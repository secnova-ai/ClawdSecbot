package proxy

import (
	"context"
	"fmt"
	"strings"

	chatmodelrouting "go_lib/chatmodel-routing"
	"go_lib/core/shepherd"
	"go_lib/core/skillscan"

	"github.com/openai/openai-go"
)

type userInputPolicyContext struct {
	RequestID string
	Messages  []openai.ChatCompletionMessageParamUnion
}

type userInputPolicyResult struct {
	Result  *chatmodelrouting.FilterRequestResult
	Pass    bool
	Handled bool
}

type userInputPolicyHook interface {
	Name() string
	Evaluate(ctx context.Context, pp *ProxyProtection, policyCtx userInputPolicyContext) userInputPolicyResult
}

type shepherdUserInputPolicyHook struct{}

func (shepherdUserInputPolicyHook) Name() string {
	return "shepherd_user_input"
}

func (shepherdUserInputPolicyHook) Evaluate(ctx context.Context, pp *ProxyProtection, policyCtx userInputPolicyContext) userInputPolicyResult {
	if len(policyCtx.Messages) == 0 || getMessageRole(policyCtx.Messages[len(policyCtx.Messages)-1]) != "user" {
		return userInputPolicyResult{}
	}

	userText := collectUserInputText(policyCtx.Messages)
	if strings.TrimSpace(userText) == "" {
		return userInputPolicyResult{}
	}
	pp.sendSecurityFlowLog(securityFlowStageUserInput, "analysis_start: user_message_count=%d combined_chars=%d", countUserMessages(policyCtx.Messages), len(userText))
	if pp.isAuditOnlyMode() {
		logSecurityFlowInfo(securityFlowStageUserInput, "audit_only=true; skipping ShepherdGate analysis")
		pp.sendSecurityFlowLog(securityFlowStageUserInput, "audit_only=true; allowing user input without blocking")
		return userInputPolicyResult{}
	}

	if detectorResult, ok := pp.evaluateUserInputWithDetector(ctx, policyCtx.RequestID, userText); ok {
		return detectorResult
	}

	pp.sendSecurityFlowLog(securityFlowStageUserInput, "decision: action=ALLOW")
	return userInputPolicyResult{}
}

func (pp *ProxyProtection) evaluateUserInputWithDetector(ctx context.Context, requestID, userText string) (userInputPolicyResult, bool) {
	if pp == nil {
		return userInputPolicyResult{}, false
	}
	detector := pp.currentSecurityDetector()
	if detector == nil {
		return userInputPolicyResult{}, false
	}

	checkCtx := pp.ctx
	if checkCtx == nil {
		checkCtx = context.Background()
	}
	checkCtx = shepherd.WithBotID(checkCtx, pp.assetID)
	response, err := detector.Detect(checkCtx, securityDetectionRequest{
		Stage:     hookStageUserInput,
		RequestID: requestID,
		UserInput: userText,
	})
	pp.recordDetectionUsage(securityFlowStageUserInput, response)
	if err != nil {
		logSecurityFlowWarning(securityFlowStageUserInput, "semantic_analysis_failed: backend=%s err=%v action=fail_open", detector.Name(), err)
		return userInputPolicyResult{}, false
	}
	if response == nil || response.Allowed == nil {
		logSecurityFlowWarning(securityFlowStageUserInput, "semantic_analysis_empty action=fail_open")
		return userInputPolicyResult{}, false
	}
	if *response.Allowed {
		pp.sendSecurityFlowLog(securityFlowStageUserInput, "semantic_decision: backend=%s action=ALLOW reason=%s", detector.Name(), response.Reason)
		return userInputPolicyResult{}, false
	}

	policyDecision := securityPolicyDecisionFromUserInputDetection(response)
	pp.sendSecurityFlowLog(securityFlowStageUserInput, "semantic_decision: backend=%s action=%s risk_type=%s reason=%s", detector.Name(), policyDecision.Action, policyDecision.RiskType, policyDecision.Reason)
	result, pass := pp.applyRequestSecurityPolicyDecision(ctx, requestID, policyDecision)
	return userInputPolicyResult{Result: result, Pass: pass, Handled: true}, true
}

func (pp *ProxyProtection) recordDetectionUsage(stage string, response *securityDetectionResponse) {
	if pp == nil || response == nil || response.Usage == nil {
		return
	}
	pp.metricsMu.Lock()
	pp.auditTokens += response.Usage.TotalTokens
	pp.auditPromptTokens += response.Usage.PromptTokens
	pp.auditCompletionTokens += response.Usage.CompletionTokens
	pp.metricsMu.Unlock()
	pp.sendMetricsToCallback()
	pp.sendSecurityFlowLog(stage, "analysis_token_usage: total=%d prompt=%d completion=%d",
		response.Usage.TotalTokens, response.Usage.PromptTokens, response.Usage.CompletionTokens)
}

func securityPolicyDecisionFromUserInputDetection(response *securityDetectionResponse) securityPolicyDecision {
	if strings.TrimSpace(response.Action) != "" {
		return securityPolicyDecisionFromDetectionResponse(response, hookStageUserInput)
	}
	decision := shepherdDecisionFromDetectionResponse(response)
	if decision == nil {
		return securityPolicyDecisionFromDetectionResponse(response, hookStageUserInput)
	}
	return securityPolicyDecisionFromUserInputLLM(decision)
}

func securityPolicyDecisionFromUserInputLLM(decision *shepherd.ShepherdDecision) securityPolicyDecision {
	riskType := strings.TrimSpace(decision.RiskType)
	if riskType == "" {
		riskType = riskHighRiskOperation
	}
	action := decisionActionNeedsConfirm
	if isUserInputPromptInjectionRisk(riskType) {
		action = decisionActionBlock
	}
	reason := strings.TrimSpace(decision.Reason)
	if reason == "" {
		reason = "User input risk detected by ShepherdGate semantic analysis."
	}
	actionDesc := strings.TrimSpace(decision.ActionDesc)
	if actionDesc == "" {
		actionDesc = "User input risk detected by ShepherdGate semantic analysis"
	}
	return securityPolicyDecision{
		Status:          action,
		Action:          action,
		ActionDesc:      actionDesc,
		Reason:          reason,
		RiskType:        riskType,
		RiskLevel:       riskLevelHigh,
		HookStage:       hookStageUserInput,
		EvidenceSummary: truncateString(reason, 160),
	}
}

func isUserInputPromptInjectionRisk(riskType string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(riskType))
	if normalized == riskPromptInjectionDirect {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(riskType))
	return strings.Contains(lower, "prompt") && strings.Contains(lower, "inject")
}

func userInputPolicyLanguage(pp *ProxyProtection) string {
	if pp != nil && pp.shepherdGate != nil {
		return pp.shepherdGate.EffectiveLanguage()
	}
	return shepherd.NormalizeShepherdLanguage(skillscan.GetLanguageFromAppSettings())
}

func countUserMessages(messages []openai.ChatCompletionMessageParamUnion) int {
	count := 0
	for _, msg := range messages {
		if msg.OfUser != nil {
			count++
		}
	}
	return count
}

func collectUserInputText(messages []openai.ChatCompletionMessageParamUnion) string {
	parts := make([]string, 0)
	for i, msg := range messages {
		if msg.OfUser == nil {
			continue
		}
		content := strings.TrimSpace(extractMessageContent(msg))
		if content == "" {
			continue
		}
		if isInjectedUserContext(content) {
			continue
		}
		parts = append(parts, fmt.Sprintf("[user:%d] %s", i, content))
	}
	return strings.Join(parts, "\n")
}

func isInjectedUserContext(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}
	normalized := strings.ToLower(content)
	markers := []string{
		"user's conversation history (from memory system)",
		"important: the following are facts from previous conversations with this user.",
		"available follow-up tools:",
		"task_summary(taskid=",
		"memory_timeline(chunkid=",
		"call memory_search with a shorter or rephrased query",
		"must call memory_search",
		"quoted_notes",
		"end_quoted_notes",
	}
	hits := 0
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			hits++
		}
	}
	if hits >= 2 {
		return true
	}
	if !strings.Contains(normalized, "memory system") {
		return false
	}
	recallMarkers := []string{
		"memory_search",
		"previous conversations",
		"conversation history",
		"quoted_notes",
		"end_quoted_notes",
		"follow-up tools",
		"task_summary(",
		"memory_timeline(",
	}
	for _, marker := range recallMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func (pp *ProxyProtection) runUserInputPolicyHooks(ctx context.Context, policyCtx userInputPolicyContext) userInputPolicyResult {
	if pp == nil || !pp.isUserInputDetectionEnabled() {
		logSecurityFlowInfo(securityFlowStageUserInput, "user_input_detection_enabled=false; skipping user input analysis")
		return userInputPolicyResult{}
	}
	hooks := []userInputPolicyHook{
		shepherdUserInputPolicyHook{},
	}
	for _, hook := range hooks {
		result := hook.Evaluate(ctx, pp, policyCtx)
		if result.Handled {
			return result
		}
	}
	return userInputPolicyResult{}
}

func (pp *ProxyProtection) isUserInputDetectionEnabled() bool {
	if pp == nil {
		return false
	}
	pp.configMu.RLock()
	defer pp.configMu.RUnlock()
	if !pp.userInputDetectionSet {
		return true
	}
	return pp.userInputDetection
}
