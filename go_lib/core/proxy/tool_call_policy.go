package proxy

import (
	"context"
	"encoding/json"
	"strings"

	"go_lib/core/shepherd"

	"github.com/openai/openai-go"
)

type toolCallPolicyContext struct {
	RequestID string
	ToolCalls []openai.ChatCompletionMessageToolCall
}

type toolCallPolicyResult struct {
	Decision *securityPolicyDecision
	Handled  bool
	Pass     bool
}

type toolCallPolicyHook interface {
	Name() string
	Evaluate(ctx context.Context, pp *ProxyProtection, policyCtx toolCallPolicyContext) toolCallPolicyResult
}

type shepherdToolCallPolicyHook struct{}

func (shepherdToolCallPolicyHook) Name() string {
	return "shepherd_tool_call"
}

func (shepherdToolCallPolicyHook) Evaluate(ctx context.Context, pp *ProxyProtection, policyCtx toolCallPolicyContext) toolCallPolicyResult {
	if len(policyCtx.ToolCalls) == 0 || pp == nil || pp.shepherdGate == nil {
		return toolCallPolicyResult{}
	}
	pp.sendSecurityFlowLog(securityFlowStageToolCall, "analysis_start: tool_call_count=%d", len(policyCtx.ToolCalls))

	toolCallInfos := make([]ToolCallInfo, 0, len(policyCtx.ToolCalls))
	for _, tc := range policyCtx.ToolCalls {
		info := ToolCallInfo{
			Name:       tc.Function.Name,
			RawArgs:    tc.Function.Arguments,
			ToolCallID: tc.ID,
		}
		if pp.toolValidator != nil {
			info.IsSensitive = pp.toolValidator.IsSensitive(tc.Function.Name)
		}
		if tc.Function.Arguments != "" {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil {
				info.Arguments = args
			}
		}
		toolCallInfos = append(toolCallInfos, info)
	}

	_, _, chainMeta, hasChain := pp.securityChainContext(policyCtx.RequestID)
	if hasChain && chainMeta.Degraded {
		pp.sendSecurityFlowLog(securityFlowStageChain, "chain_degraded_context: request_id=%s instruction_chain_id=%s reason=%s", policyCtx.RequestID, chainMeta.ChainID, chainMeta.DegradeReason)
	}

	detector := pp.currentSecurityDetector()
	if detector == nil {
		logSecurityFlowError(securityFlowStageToolCall, "analysis_failed: detector_missing action=fail_open")
		return toolCallPolicyResult{}
	}

	checkCtx := pp.ctx
	if checkCtx == nil {
		checkCtx = context.Background()
	}
	checkCtx = shepherd.WithBotID(checkCtx, pp.assetID)
	response, err := detector.Detect(checkCtx, securityDetectionRequest{
		Stage:     hookStageToolCall,
		RequestID: policyCtx.RequestID,
		ToolCalls: toolCallInfos,
	})

	pp.statsMu.Lock()
	pp.analysisCount++
	pp.statsMu.Unlock()
	pp.sendMetricsToCallback()

	pp.recordDetectionUsage(securityFlowStageToolCall, response)

	if err != nil {
		logSecurityFlowError(securityFlowStageToolCall, "analysis_failed: backend=%s err=%v action=fail_open", detector.Name(), err)
		return toolCallPolicyResult{}
	}
	if response == nil || response.Allowed == nil || detectionResponseAllowed(response) {
		pp.sendSecurityFlowLog(securityFlowStageToolCall, "decision: backend=%s action=ALLOW", detector.Name())
		return toolCallPolicyResult{}
	}

	policyDecision := securityPolicyDecisionFromToolCallDetection(response, toolCallInfos)
	if pp.consumeConfirmedToolCallGrantForRequest(policyCtx.RequestID, policyDecision) {
		pp.sendSecurityFlowLog(securityFlowStageToolCall, "decision: action=ALLOW reason=confirmed_matching_tool_call risk_type=%s", policyDecision.RiskType)
		return toolCallPolicyResult{}
	}
	pp.sendSecurityFlowLog(securityFlowStageToolCall, "decision: action=%s risk_type=%s reason=%s", policyDecision.normalizedAction(), policyDecision.RiskType, policyDecision.Reason)
	return toolCallPolicyResult{
		Decision: &policyDecision,
		Handled:  true,
		Pass:     false,
	}
}

func securityPolicyDecisionFromToolCallDetection(response *securityDetectionResponse, toolCalls []ToolCallInfo) securityPolicyDecision {
	decision := shepherdDecisionFromDetectionResponse(response)
	if decision != nil && strings.TrimSpace(response.Action) == "" {
		return securityPolicyDecisionFromToolCallLLM(decision, toolCalls)
	}
	result := securityPolicyDecisionFromDetectionResponse(response, hookStageToolCall)
	if result.ToolCallID == "" && len(toolCalls) > 0 {
		result.ToolCallID = toolCalls[0].ToolCallID
	}
	if result.EvidenceSummary == "" {
		evidenceParts := make([]string, 0, len(toolCalls))
		for _, tc := range toolCalls {
			evidenceParts = append(evidenceParts, strings.TrimSpace(tc.Name+" "+tc.RawArgs))
		}
		result.EvidenceSummary = truncateString(redactSecurityEvidence(strings.Join(evidenceParts, "\n")), 240)
	}
	return result
}

func securityPolicyDecisionFromToolCallLLM(decision *shepherd.ShepherdDecision, toolCalls []ToolCallInfo) securityPolicyDecision {
	action := decisionActionNeedsConfirm
	if strings.EqualFold(strings.TrimSpace(decision.Status), decisionActionBlock) {
		action = decisionActionBlock
	}
	riskType := strings.TrimSpace(decision.RiskType)
	if riskType == "" {
		riskType = riskHighRiskOperation
	}
	reason := strings.TrimSpace(decision.Reason)
	if reason == "" {
		reason = "Tool call risk detected by ShepherdGate ReAct analysis."
	}
	actionDesc := strings.TrimSpace(decision.ActionDesc)
	if actionDesc == "" {
		actionDesc = "Tool call risk detected by ShepherdGate ReAct analysis"
	}
	toolCallID := ""
	evidenceParts := make([]string, 0, len(toolCalls))
	for _, tc := range toolCalls {
		if toolCallID == "" {
			toolCallID = tc.ToolCallID
		}
		evidenceParts = append(evidenceParts, strings.TrimSpace(tc.Name+" "+tc.RawArgs))
	}
	return securityPolicyDecision{
		Status:          action,
		Action:          action,
		ActionDesc:      actionDesc,
		Reason:          reason,
		RiskType:        riskType,
		RiskLevel:       riskLevelHigh,
		HookStage:       hookStageToolCall,
		ToolCallID:      toolCallID,
		EvidenceSummary: truncateString(redactSecurityEvidence(strings.Join(evidenceParts, "\n")), 240),
	}
}

func normalizeRuleAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "block", "blocked":
		return decisionActionBlock
	case "allow", "allowed":
		return decisionActionAllow
	case "redact":
		return decisionActionRedact
	default:
		return decisionActionNeedsConfirm
	}
}

type shepherdRuleView struct {
	Description string
	Action      string
	RiskType    string
}

func semanticRuleMatchesText(rule, text string) bool {
	rule = strings.TrimSpace(rule)
	text = strings.TrimSpace(text)
	if rule == "" || text == "" {
		return false
	}
	if !strings.Contains(rule, "*") {
		if strings.Contains(text, rule) {
			return true
		}
		return semanticRuleKeywordMatches(rule, text)
	}
	parts := strings.Split(rule, "*")
	cursor := 0
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.Index(text[cursor:], part)
		if idx < 0 {
			return false
		}
		cursor += idx + len(part)
	}
	return true
}

func semanticRuleKeywordMatches(rule, text string) bool {
	switch {
	case strings.Contains(rule, "删除") || strings.Contains(rule, "delete") || strings.Contains(rule, "remove"):
		return strings.Contains(text, "删除") || strings.Contains(text, "delete") || strings.Contains(text, "remove")
	case strings.Contains(rule, "邮件") || strings.Contains(rule, "mail") || strings.Contains(rule, "email"):
		return strings.Contains(text, "邮件") || strings.Contains(text, "mail") || strings.Contains(text, "email")
	case strings.Contains(rule, "ssh") || strings.Contains(rule, "key") || strings.Contains(rule, "密钥"):
		return strings.Contains(text, "ssh") || strings.Contains(text, "key") || strings.Contains(text, "密钥")
	case strings.Contains(rule, "cookie"):
		return strings.Contains(text, "cookie")
	case strings.Contains(rule, "配置") || strings.Contains(rule, "config"):
		return strings.Contains(text, "配置") || strings.Contains(text, "config")
	default:
		return false
	}
}

func semanticRuleAppliesTo(appliesTo []string, stage string) bool {
	if len(appliesTo) == 0 {
		return true
	}
	stage = strings.ToLower(strings.TrimSpace(stage))
	for _, item := range appliesTo {
		if strings.ToLower(strings.TrimSpace(item)) == stage {
			return true
		}
	}
	return false
}

func (pp *ProxyProtection) runToolCallPolicyHooks(ctx context.Context, policyCtx toolCallPolicyContext) toolCallPolicyResult {
	hooks := []toolCallPolicyHook{
		shepherdToolCallPolicyHook{},
	}
	for _, hook := range hooks {
		result := hook.Evaluate(ctx, pp, policyCtx)
		if result.Handled {
			return result
		}
	}
	return toolCallPolicyResult{}
}
