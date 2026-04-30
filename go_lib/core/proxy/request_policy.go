package proxy

import (
	"context"
	"fmt"
	"strings"
	"time"

	chatmodelrouting "go_lib/chatmodel-routing"

	"github.com/openai/openai-go"
)

type requestPolicyState struct {
	ConversationUsage int
	RecentMsgCount    int
	IsContinuation    bool
}

type requestPolicyDecision struct {
	HookName          string
	Reason            string
	MockContent       string
	Current           int
	Limit             int
	SendLogKey        string
	ActionDesc        string
	AuditStartSource  string
	AuditFinishSource string
	BlockLog          string
	SetConversation   bool
}

type requestPolicyHook interface {
	Name() string
	Evaluate(pp *ProxyProtection, req *openai.ChatCompletionNewParams, modelName string, state *requestPolicyState) *requestPolicyDecision
}

type quotaRequestPolicyHook struct{}

func (quotaRequestPolicyHook) Name() string {
	return "quota"
}

func (quotaRequestPolicyHook) Evaluate(pp *ProxyProtection, req *openai.ChatCompletionNewParams, modelName string, state *requestPolicyState) *requestPolicyDecision {
	pp.configMu.RLock()
	sessionLimit := pp.singleSessionTokenLimit
	dailyLimit := pp.dailyTokenLimit
	initialUsage := pp.initialDailyUsage
	pp.configMu.RUnlock()

	conversationUsage, recentMsgCount, isContinuation := pp.evaluateConversationWindow(req)
	state.ConversationUsage = conversationUsage
	state.RecentMsgCount = recentMsgCount
	state.IsContinuation = isContinuation

	if sessionLimit > 0 {
		if isContinuation {
			pp.sendTerminalLog(fmt.Sprintf("Conversation quota continuing: recent_messages=%d usage=%d/%d", recentMsgCount, conversationUsage, sessionLimit))
		} else {
			pp.sendTerminalLog(fmt.Sprintf("Conversation quota reset: recent_messages=%d usage=%d/%d", recentMsgCount, conversationUsage, sessionLimit))
		}
		if conversationUsage >= sessionLimit {
			reason := fmt.Sprintf("Conversation token quota exceeded (%d/%d)", conversationUsage, sessionLimit)
			return &requestPolicyDecision{
				HookName:          "quota_conversation",
				Reason:            reason,
				MockContent:       formatQuotaExceededMessage("conversation", conversationUsage, sessionLimit),
				Current:           conversationUsage,
				Limit:             sessionLimit,
				SendLogKey:        "proxy_session_quota_exceeded",
				ActionDesc:        "Conversation token quota exceeded",
				AuditStartSource:  "start_from_request_quota_conversation",
				AuditFinishSource: "set_decision_quota_conversation",
				BlockLog:          fmt.Sprintf(">>> %s, request blocked <<<", reason),
				SetConversation:   true,
			}
		}
	}

	if dailyLimit > 0 {
		pp.metricsMu.Lock()
		runtimeUsage := pp.totalTokens - pp.baselineTotalTokens
		if runtimeUsage < 0 {
			runtimeUsage = 0
		}
		currentTotal := initialUsage + runtimeUsage
		pp.metricsMu.Unlock()

		pp.sendTerminalLog(fmt.Sprintf("📊 每日 Token 用量: %d / %d (当日基线: %d, 本次会话新增: %d)", currentTotal, dailyLimit, initialUsage, runtimeUsage))
		if currentTotal >= dailyLimit {
			reason := fmt.Sprintf("每日 Token 配额已用尽 (%d/%d)", currentTotal, dailyLimit)
			return &requestPolicyDecision{
				HookName:          "quota_daily",
				Reason:            reason,
				MockContent:       formatQuotaExceededMessage("daily", currentTotal, dailyLimit),
				Current:           currentTotal,
				Limit:             dailyLimit,
				SendLogKey:        "proxy_quota_exceeded",
				ActionDesc:        "Daily token quota exceeded",
				AuditStartSource:  "start_from_request_quota_daily",
				AuditFinishSource: "set_decision_quota_daily",
				BlockLog:          fmt.Sprintf(">>> %s,已拦截请求 <<<", reason),
			}
		}
	}

	return nil
}

func (pp *ProxyProtection) runRequestPolicyHooks(req *openai.ChatCompletionNewParams, modelName string) (requestPolicyState, *requestPolicyDecision) {
	state := requestPolicyState{}
	hooks := []requestPolicyHook{
		quotaRequestPolicyHook{},
	}
	for _, hook := range hooks {
		if decision := hook.Evaluate(pp, req, modelName, &state); decision != nil {
			if decision.HookName == "" {
				decision.HookName = hook.Name()
			}
			return state, decision
		}
	}
	return state, nil
}

func (pp *ProxyProtection) applyRequestPolicyDecision(
	ctx context.Context,
	req *openai.ChatCompletionNewParams,
	rawBody []byte,
	stream bool,
	modelName string,
	state requestPolicyState,
	decision *requestPolicyDecision,
) (*chatmodelrouting.FilterRequestResult, bool) {
	if decision == nil {
		return nil, true
	}
	if decision.BlockLog == "" {
		decision.BlockLog = fmt.Sprintf(">>> %s, request blocked <<<", decision.Reason)
	}
	pp.sendTerminalLog(decision.BlockLog)

	pp.auditMu.Lock()
	pp.requestStartTime = time.Now()
	pp.currentRequestID = fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), pp.requestCount+1)
	requestID := pp.currentRequestID
	pp.auditMu.Unlock()
	pp.bindRequestContext(ctx, requestID)
	chain := pp.prepareSecurityChainForRequest(requestID, req.Messages, nil)
	chainID := ""
	if chain != nil {
		chainID = chain.ChainID
	}
	pp.createRequestRuntimeState(requestID, chainID, req, rawBody)
	contextMessages := make([]ConversationMessage, 0, len(req.Messages))
	lastUserMessage := ""
	for _, msg := range req.Messages {
		cm := extractConversationMessage(msg)
		contextMessages = append(contextMessages, cm)
		if strings.EqualFold(cm.Role, "user") {
			lastUserMessage = cm.Content
		}
	}
	pp.updateSecurityChainContext(requestID, contextMessages, lastUserMessage)
	pp.auditLogSafe(decision.AuditStartSource, func(tracker *AuditChainTracker) {
		tracker.StartFromRequest(requestID, pp.assetName, pp.assetID, modelName, req.Messages)
		tracker.SetRequestInstructionChainID(requestID, chainID)
	})

	pp.sendLog(decision.SendLogKey, map[string]interface{}{
		"current": decision.Current,
		"limit":   decision.Limit,
		"model":   modelName,
	})

	pp.updateTruthRecord(requestID, func(r *TruthRecord) {
		r.Model = modelName
		r.MessageCount = len(req.Messages)
		r.Phase = RecordPhaseStopped
		r.CompletedAt = time.Now().Format(time.RFC3339Nano)
		r.FinishReason = "quota_exceeded"
		if decision.SetConversation {
			r.ConversationTokens = state.ConversationUsage
		}
		r.InstructionChainID = chainID
		r.DailyTokens = pp.currentDailyTokenUsage()
		r.OutputContent = truncateToBytes(decision.MockContent, maxRecordOutputBytes)
		appendRequestMessagesToTruthRecord(r, req)
		appendAssistantMessageToTruthRecord(r, decision.MockContent)
		r.Decision = &SecurityDecision{
			Action:     "BLOCK",
			RiskLevel:  "QUOTA",
			Reason:     decision.Reason,
			Confidence: 100,
		}
		applyRecordPrimaryContent(r, RecordContentSecurity, decision.MockContent, true)
	})

	pp.emitMonitorRequestCreated(req, rawBody, stream)
	pp.emitMonitorSecurityDecision("QUOTA_EXCEEDED", decision.Reason, true, decision.MockContent)
	pp.emitSecurityEvent(requestID, "blocked", decision.ActionDesc, "QUOTA", decision.Reason)
	pp.emitMonitorResponseReturned("QUOTA_EXCEEDED", decision.MockContent, decision.MockContent)
	pp.auditLogSafe(decision.AuditFinishSource, func(tracker *AuditChainTracker) {
		tracker.SetRequestDecision(requestID, "BLOCK", "QUOTA", decision.Reason, 100)
		tracker.FinalizeRequestOutput(requestID, decision.MockContent)
	})
	pp.clearRequestContext(ctx)
	pp.clearRequestRuntimeState(requestID)
	return &chatmodelrouting.FilterRequestResult{MockContent: decision.MockContent}, false
}
