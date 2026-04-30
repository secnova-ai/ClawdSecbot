package proxy

import (
	"context"
	"fmt"

	chatmodelrouting "go_lib/chatmodel-routing"

	"github.com/openai/openai-go"
	"github.com/tidwall/sjson"
)

const blockedToolResultPlaceholder = "[ClawdSecbot] This tool result was quarantined by security policy because its tool_call_id was previously blocked. The original content is withheld."

type requestRewriteContext struct {
	RequestID string
	RawBody   []byte
	Messages  []openai.ChatCompletionMessageParamUnion
}

type requestRewriteResult struct {
	Result  *chatmodelrouting.FilterRequestResult
	Rewrote bool
}

type requestRewriteHook interface {
	Name() string
	Rewrite(ctx context.Context, pp *ProxyProtection, rewriteCtx requestRewriteContext) requestRewriteResult
}

type blockedToolResultRewriteHook struct{}

func (blockedToolResultRewriteHook) Name() string {
	return "blocked_tool_result_rewrite"
}

func (blockedToolResultRewriteHook) Rewrite(ctx context.Context, pp *ProxyProtection, rewriteCtx requestRewriteContext) requestRewriteResult {
	_ = ctx
	if pp == nil || len(rewriteCtx.RawBody) == 0 || len(rewriteCtx.Messages) == 0 {
		return requestRewriteResult{}
	}

	forwardBody := rewriteCtx.RawBody
	rewrittenIDs := make([]string, 0)
	for i, msg := range rewriteCtx.Messages {
		if msg.OfTool == nil {
			continue
		}
		toolCallID := normalizeBlockedToolCallID(msg.OfTool.ToolCallID)
		if toolCallID == "" || !pp.isBlockedToolCallIDForRequest(rewriteCtx.RequestID, toolCallID) {
			continue
		}
		nextBody, err := sjson.SetBytes(forwardBody, fmt.Sprintf("messages.%d.content", i), blockedToolResultPlaceholder)
		if err != nil {
			logSecurityFlowWarning(securityFlowStageRequestRewrite, "rewrite_failed: request_id=%s index=%d tool_call_id=%s err=%v", rewriteCtx.RequestID, i, toolCallID, err)
			continue
		}
		forwardBody = nextBody
		rewrittenIDs = append(rewrittenIDs, toolCallID)
	}

	if len(rewrittenIDs) == 0 {
		return requestRewriteResult{}
	}

	pp.sendSecurityFlowLog(securityFlowStageRequestRewrite, "rewrote historical blocked tool results before upstream forwarding: count=%d tool_call_ids=%v", len(rewrittenIDs), rewrittenIDs)
	pp.sendLog("proxy_blocked_tool_result_rewritten", map[string]interface{}{
		"request_id": rewriteCtx.RequestID,
		"count":      len(rewrittenIDs),
		"tool_ids":   rewrittenIDs,
	})
	pp.emitRiskSecurityEvent(rewriteCtx.RequestID, "rewritten", "Historical blocked tool result rewritten", riskEventMetadata{
		RiskType:        riskContextPoisoning,
		RiskLevel:       riskLevelHigh,
		DecisionAction:  decisionActionRewrite,
		HookStage:       hookStageRequestRewrite,
		ToolCallID:      rewrittenIDs[0],
		EvidenceSummary: fmt.Sprintf("rewritten_tool_call_ids=%v", rewrittenIDs),
		WasRewritten:    true,
		Reason:          "Historical tool result was quarantined earlier and rewritten before upstream forwarding.",
	})
	logSecurityFlowInfo(securityFlowStageRequestRewrite, "rewrite_complete: request_id=%s count=%d", rewriteCtx.RequestID, len(rewrittenIDs))

	return requestRewriteResult{
		Result:  &chatmodelrouting.FilterRequestResult{ForwardBody: forwardBody},
		Rewrote: true,
	}
}

func (pp *ProxyProtection) runRequestRewriteHooks(ctx context.Context, rewriteCtx requestRewriteContext) requestRewriteResult {
	hooks := []requestRewriteHook{
		blockedToolResultRewriteHook{},
	}
	var merged *chatmodelrouting.FilterRequestResult
	for _, hook := range hooks {
		result := hook.Rewrite(ctx, pp, rewriteCtx)
		if !result.Rewrote {
			continue
		}
		if merged == nil {
			merged = result.Result
		} else if result.Result != nil && len(result.Result.ForwardBody) > 0 {
			merged.ForwardBody = result.Result.ForwardBody
		}
		if result.Result != nil && len(result.Result.ForwardBody) > 0 {
			rewriteCtx.RawBody = result.Result.ForwardBody
		}
	}
	if merged == nil {
		return requestRewriteResult{}
	}
	return requestRewriteResult{Result: merged, Rewrote: true}
}
