package proxy

import (
	"context"
	"strings"
	"time"

	"go_lib/core/shepherd"

	"github.com/openai/openai-go"
)

type pendingToolCallRecovery struct {
	ToolCalls        []openai.ChatCompletionMessageToolCall
	ToolCallIDs      []string
	AssistantContent string
	RiskReason       string
	RiskType         string
	Source           string
	CreatedAt        time.Time
}

func cloneToolCalls(calls []openai.ChatCompletionMessageToolCall) []openai.ChatCompletionMessageToolCall {
	out := make([]openai.ChatCompletionMessageToolCall, len(calls))
	copy(out, calls)
	return out
}

func cloneStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizeBlockedToolCallID(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func toolCallIDsFromOpenAIToolCalls(calls []openai.ChatCompletionMessageToolCall) []string {
	ids := make([]string, 0, len(calls))
	for _, call := range calls {
		if id := normalizeBlockedToolCallID(call.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func extractRecentConversationMessagesFromParams(messages []openai.ChatCompletionMessageParamUnion, limit int) []ConversationMessage {
	if limit <= 0 {
		limit = 50
	}
	start := 0
	if len(messages) > limit {
		start = len(messages) - limit
	}
	out := make([]ConversationMessage, 0, len(messages)-start)
	for _, msg := range messages[start:] {
		out = append(out, extractConversationMessage(msg))
	}
	return out
}

func (pp *ProxyProtection) storePendingToolCallRecoveryForRequest(requestID string, toolCalls []openai.ChatCompletionMessageToolCall, assistantContent, riskReason, source string) {
	pp.storePendingToolCallRecoveryWithIDsForRequest(requestID, toolCalls, toolCallIDsFromOpenAIToolCalls(toolCalls), assistantContent, riskReason, source)
}

func (pp *ProxyProtection) storePendingToolCallRecoveryWithIDsForRequest(requestID string, toolCalls []openai.ChatCompletionMessageToolCall, toolCallIDs []string, assistantContent, riskReason, source string) {
	pp.storePendingToolCallRecoveryWithRiskForRequest(requestID, toolCalls, toolCallIDs, assistantContent, riskReason, source, "")
}

func (pp *ProxyProtection) storePendingToolCallRecoveryWithRiskForRequest(requestID string, toolCalls []openai.ChatCompletionMessageToolCall, toolCallIDs []string, assistantContent, riskReason, source, riskType string) {
	if pp == nil {
		return
	}
	chainID := pp.chainIDForRequest(requestID)
	if chainID == "" {
		chain := pp.createSecurityChain(requestID, securityChainSourceTemp, "pending_recovery_without_chain")
		if chain != nil {
			chainID = chain.ChainID
		}
	}
	if chainID == "" {
		return
	}
	now := time.Now()
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	chain := pp.chains[chainID]
	if chain == nil {
		return
	}
	chain.PendingRecovery = &pendingToolCallRecovery{
		ToolCalls:        cloneToolCalls(toolCalls),
		ToolCallIDs:      cloneStringSlice(toolCallIDs),
		AssistantContent: assistantContent,
		RiskReason:       riskReason,
		RiskType:         normalizeRecoveryRiskType(riskType),
		Source:           source,
		CreatedAt:        now,
	}
	chain.PendingRecoveryArmed = false
	chain.UpdatedAt = now
	chain.ExpiresAt = now.Add(securityChainTTL)
}

func (pp *ProxyProtection) pendingRecoveryToolCallIDsForRequest(requestID string) []string {
	chain := pp.chainByRequestID(requestID)
	if chain == nil || chain.PendingRecovery == nil {
		return nil
	}
	return cloneStringSlice(chain.PendingRecovery.ToolCallIDs)
}

func (pp *ProxyProtection) pendingRecoveryRequiresToolResultForRequest(requestID string) bool {
	return len(pp.pendingRecoveryToolCallIDsForRequest(requestID)) > 0
}

func (pp *ProxyProtection) hasPendingToolCallRecoveryForRequest(requestID string) bool {
	chain := pp.chainByRequestID(requestID)
	return chain != nil && chain.PendingRecovery != nil
}

func (pp *ProxyProtection) clearPendingToolCallRecoveryForRequest(requestID string) {
	if pp == nil {
		return
	}
	now := time.Now()
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	chainID := pp.chainIDForRequestLocked(requestID, now)
	if chainID == "" {
		return
	}
	chain := pp.chains[chainID]
	if chain == nil {
		return
	}
	chain.PendingRecovery = nil
	chain.PendingRecoveryArmed = false
	chain.UpdatedAt = now
	chain.ExpiresAt = now.Add(securityChainTTL)
}

func (pp *ProxyProtection) recoverPendingToolCallRecoveryFromHistory(requestID string, messages []openai.ChatCompletionMessageParamUnion) bool {
	if pp == nil || len(messages) == 0 {
		return false
	}
	promptIndex, promptContent := latestRecoveryPromptBeforeLatestUser(messages)
	if promptIndex < 0 {
		return false
	}

	toolCallIDs := toolCallIDsImmediatelyBefore(messages, promptIndex)
	reason := recoveryReasonFromPrompt(promptContent)
	if reason == "" {
		reason = "Recovered pending ShepherdGate confirmation from request history."
	}

	pp.ensureRecoveredPendingRecovery(requestID, toolCallIDs, promptContent, reason)
	pp.markBlockedToolCallIDsForRequest(requestID, toolCallIDs)
	pp.sendSecurityFlowLog(securityFlowStageRecovery, "recovered pending confirmation from request history: request_id=%s tool_call_ids=%v", strings.TrimSpace(requestID), toolCallIDs)
	return true
}

func (pp *ProxyProtection) ensureRecoveredPendingRecovery(requestID string, toolCallIDs []string, promptContent, reason string) {
	if pp == nil {
		return
	}
	if pp.hasPendingToolCallRecoveryForRequest(requestID) {
		return
	}
	if strings.TrimSpace(reason) == "" {
		reason = "Recovered pending ShepherdGate confirmation from request history."
	}
	pp.storePendingToolCallRecoveryWithRiskForRequest(requestID, nil, toolCallIDs, promptContent, reason, "request_history", riskTypeFromRecoveryPrompt(promptContent))
}

func normalizeRecoveryRiskType(riskType string) string {
	return strings.ToUpper(strings.TrimSpace(riskType))
}

func riskTypeFromRecoveryPrompt(content string) string {
	text := strings.ToLower(strings.TrimSpace(content))
	if text == "" {
		return ""
	}
	switch {
	case strings.Contains(text, "prompt_injection_direct") || strings.Contains(text, "直接提示词注入") || strings.Contains(text, "direct prompt injection"):
		return riskPromptInjectionDirect
	case strings.Contains(text, "prompt_injection_indirect") || strings.Contains(text, "间接提示词注入") || strings.Contains(text, "indirect prompt injection"):
		return riskPromptInjectionIndirect
	case strings.Contains(text, "sensitive_data_exfiltration") ||
		strings.Contains(text, "敏感数据外泄") ||
		strings.Contains(text, "敏感凭证") ||
		strings.Contains(text, "sensitive data exfiltration") ||
		strings.Contains(text, "credential"):
		return riskSensitiveDataExfil
	case strings.Contains(text, "high_risk_operation") || strings.Contains(text, "高危操作") || strings.Contains(text, "high-risk operation"):
		return riskHighRiskOperation
	case strings.Contains(text, "privilege_abuse") || strings.Contains(text, "权限滥用") || strings.Contains(text, "privilege abuse"):
		return riskPrivilegeAbuse
	case strings.Contains(text, "unexpected_code_execution") || strings.Contains(text, "非预期代码执行") || strings.Contains(text, "unexpected code execution"):
		return riskUnexpectedCodeExecution
	default:
		return ""
	}
}

func latestRecoveryPromptBeforeLatestUser(messages []openai.ChatCompletionMessageParamUnion) (int, string) {
	latestUserIndex := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(getMessageRole(messages[i]), "user") {
			latestUserIndex = i
			break
		}
	}
	if latestUserIndex <= 0 {
		return -1, ""
	}

	for i := latestUserIndex - 1; i >= 0; i-- {
		if !strings.EqualFold(getMessageRole(messages[i]), "assistant") {
			continue
		}
		content := extractMessageContent(messages[i])
		if isShepherdGateRecoveryPromptContent(content) {
			return i, content
		}
		return -1, ""
	}
	return -1, ""
}

func isShepherdGateRecoveryPromptContent(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" || !strings.Contains(content, "[ShepherdGate]") {
		return false
	}
	upper := strings.ToUpper(content)
	lower := strings.ToLower(content)
	return strings.Contains(upper, "NEEDS_CONFIRMATION") ||
		strings.Contains(content, "需要确认") ||
		strings.Contains(content, "继续可回复") ||
		strings.Contains(lower, "continue replies:")
}

func toolCallIDsImmediatelyBefore(messages []openai.ChatCompletionMessageParamUnion, promptIndex int) []string {
	if promptIndex <= 0 || promptIndex > len(messages) {
		return nil
	}
	ids := make([]string, 0)
	seen := make(map[string]struct{})
	for i := promptIndex - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.OfTool == nil {
			break
		}
		toolCallID := normalizeBlockedToolCallID(msg.OfTool.ToolCallID)
		if toolCallID == "" {
			continue
		}
		if _, ok := seen[toolCallID]; ok {
			continue
		}
		seen[toolCallID] = struct{}{}
		ids = append(ids, toolCallID)
	}
	return ids
}

func recoveryReasonFromPrompt(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if idx := strings.LastIndex(line, "原因:"); idx >= 0 {
			return strings.TrimSpace(line[idx+len("原因:"):])
		}
		if idx := strings.LastIndex(line, "Reason:"); idx >= 0 {
			return strings.TrimSpace(line[idx+len("Reason:"):])
		}
	}
	return ""
}

func (pp *ProxyProtection) armPendingRecoveryFromRequest(ctx context.Context, requestID string, messages []openai.ChatCompletionMessageParamUnion) bool {
	contextMessages := extractRecentConversationMessagesFromParams(messages, 80)
	if len(contextMessages) == 0 {
		return false
	}
	return pp.armPendingRecoveryFromContext(ctx, requestID, contextMessages)
}

func (pp *ProxyProtection) armPendingRecoveryFromContext(ctx context.Context, requestID string, contextMessages []ConversationMessage) bool {
	if pp == nil {
		return false
	}
	now := time.Now()
	pp.chainMu.Lock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	chainID := pp.chainIDForRequestLocked(requestID, now)
	chain := pp.chains[chainID]
	if chain == nil || chain.PendingRecovery == nil {
		pp.chainMu.Unlock()
		return false
	}
	if chain.PendingRecoveryArmed {
		pp.chainMu.Unlock()
		logSecurityFlowInfo(securityFlowStageRecovery, "pending recovery already armed; waiting for injection")
		return true
	}
	snapshot := *chain.PendingRecovery
	snapshot.ToolCalls = cloneToolCalls(chain.PendingRecovery.ToolCalls)
	snapshot.ToolCallIDs = cloneStringSlice(chain.PendingRecovery.ToolCallIDs)
	pp.chainMu.Unlock()

	if pp.shepherdGate == nil {
		logSecurityFlowWarning(securityFlowStageRecovery, "shepherdGate is nil; skipping recovery intent analysis")
		return false
	}

	toolCalls := extractToolCalls(snapshot.ToolCalls)
	intentDecision, err := pp.shepherdGate.EvaluateRecoveryIntent(ctx, contextMessages, toolCalls, snapshot.RiskReason)
	if intentDecision != nil && intentDecision.Usage != nil {
		pp.metricsMu.Lock()
		pp.auditTokens += intentDecision.Usage.TotalTokens
		pp.auditPromptTokens += intentDecision.Usage.PromptTokens
		pp.auditCompletionTokens += intentDecision.Usage.CompletionTokens
		pp.metricsMu.Unlock()
		pp.sendMetricsToCallback()
		pp.sendSecurityFlowLog(securityFlowStageRecovery, "analysis_token_usage: total=%d prompt=%d completion=%d",
			intentDecision.Usage.TotalTokens, intentDecision.Usage.PromptTokens, intentDecision.Usage.CompletionTokens)
	}
	if err != nil {
		logSecurityFlowWarning(securityFlowStageRecovery, "intent analysis failed: %v", err)
		return false
	}
	if intentDecision == nil {
		logSecurityFlowWarning(securityFlowStageRecovery, "intent analysis returned nil decision")
		return false
	}

	intent := shepherd.NormalizeRecoveryIntent(intentDecision.Intent)
	logSecurityFlowInfo(securityFlowStageRecovery, "intent_decision: intent=%s reason=%s", intent, intentDecision.Reason)

	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(time.Now())
	chainID = pp.chainIDForRequestLocked(requestID, time.Now())
	chain = pp.chains[chainID]
	if chain == nil || chain.PendingRecovery == nil || !chain.PendingRecovery.CreatedAt.Equal(snapshot.CreatedAt) {
		logSecurityFlowInfo(securityFlowStageRecovery, "pending recovery changed before applying intent decision; skipping")
		return false
	}

	switch intent {
	case "CONFIRM":
		now = time.Now()
		chain.PendingRecoveryArmed = true
		if len(snapshot.ToolCallIDs) == 0 && len(snapshot.ToolCalls) == 0 {
			riskType := normalizeRecoveryRiskType(snapshot.RiskType)
			if riskType == "" {
				riskType = riskTypeFromRecoveryPrompt(snapshot.AssistantContent)
			}
			if riskType != "" {
				chain.ConfirmedToolCallRiskType = riskType
				chain.ConfirmedToolCallUntil = now.Add(securityChainTTL)
				logSecurityFlowInfo(securityFlowStageRecovery, "user confirmation grants next matching tool_call once: risk_type=%s", riskType)
			}
		}
		chain.UpdatedAt = now
		chain.ExpiresAt = chain.UpdatedAt.Add(securityChainTTL)
		logSecurityFlowInfo(securityFlowStageRecovery, "user confirmation recognized; recovery armed")
		return true
	case "REJECT":
		chain.PendingRecovery = nil
		chain.PendingRecoveryArmed = false
		chain.UpdatedAt = time.Now()
		chain.ExpiresAt = chain.UpdatedAt.Add(securityChainTTL)
		logSecurityFlowInfo(securityFlowStageRecovery, "user rejection recognized; pending recovery cleared")
		return false
	default:
		logSecurityFlowInfo(securityFlowStageRecovery, "no clear confirmation or rejection detected; keeping pending recovery")
		return false
	}
}
