package proxy

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

type finalResultPolicyContext struct {
	RequestID string
	Content   string
	Stream    bool
}

type finalResultPolicyResult struct {
	Content  string
	Decision *securityPolicyDecision
	Handled  bool
	Pass     bool
	Mutated  bool
}

type finalResultPolicyHook interface {
	Name() string
	Evaluate(ctx context.Context, pp *ProxyProtection, policyCtx finalResultPolicyContext) finalResultPolicyResult
}

type detectorFinalResultPolicyHook struct{}
type ruleFinalResultPolicyHook struct{}

func (detectorFinalResultPolicyHook) Name() string {
	return "detector_final_result"
}

func (ruleFinalResultPolicyHook) Name() string {
	return "rule_final_result"
}

var (
	finalResultSecretPatterns = []struct {
		pattern *regexp.Regexp
		label   string
	}{
		{regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`), "OpenAI-style API key"},
		{regexp.MustCompile(`ghp_[A-Za-z0-9_]{20,}`), "GitHub token"},
		{regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{20,}`), "Slack token"},
		{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "AWS access key"},
		{regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password)\s*[:=]\s*['"]?[A-Za-z0-9_./+=-]{12,}`), "credential-like assignment"},
	}
	finalResultDangerousAdvicePattern = regexp.MustCompile(`(?i)(rm\s+-[^\s]*r[^\s]*f\s+/|curl\s+[^|]{0,120}\|\s*(sh|bash)|wget\s+[^|]{0,120}\|\s*(sh|bash)|chmod\s+777\s+/|/etc/shadow|launchctl\s+load|crontab\s+.*\*)`)
	finalResultTrustExploitPattern    = regexp.MustCompile(`(?i)(click|press|choose|approve|confirm).{0,80}(without reviewing|safe to ignore|security warning|permission warning|直接确认|不用看|忽略.*警告|批准.*权限)`)
)

func (detectorFinalResultPolicyHook) Evaluate(ctx context.Context, pp *ProxyProtection, policyCtx finalResultPolicyContext) finalResultPolicyResult {
	content := strings.TrimSpace(policyCtx.Content)
	if content == "" || pp == nil {
		return finalResultPolicyResult{}
	}
	detector := pp.currentSecurityDetector()
	if detector == nil {
		return finalResultPolicyResult{}
	}
	response, err := detector.Detect(ctx, securityDetectionRequest{
		Stage:        hookStageFinalResult,
		RequestID:    policyCtx.RequestID,
		FinalContent: content,
		Stream:       policyCtx.Stream,
	})
	pp.recordDetectionUsage(securityFlowStageFinalResult, response)
	if err != nil {
		logSecurityFlowWarning(securityFlowStageFinalResult, "analysis_failed: backend=%s err=%v action=fail_open", detector.Name(), err)
		return finalResultPolicyResult{}
	}
	if response == nil {
		return finalResultPolicyResult{}
	}
	if detectionResponseAllowed(response) {
		if !policyCtx.Stream {
			pp.sendSecurityFlowLog(securityFlowStageFinalResult, "decision: backend=%s action=ALLOW", detector.Name())
		}
		return finalResultPolicyResult{Handled: true, Pass: true}
	}

	decision := securityPolicyDecisionFromDetectionResponse(response, hookStageFinalResult)
	if decision.Action == decisionActionRedact || decision.Action == decisionActionRewrite {
		nextContent := response.Content
		if strings.TrimSpace(nextContent) == "" {
			nextContent = content
		}
		decision.WasRewritten = true
		pp.sendSecurityFlowLog(securityFlowStageFinalResult, "decision: backend=%s action=%s risk_type=%s reason=%s", detector.Name(), decision.normalizedAction(), decision.RiskType, decision.Reason)
		return finalResultPolicyResult{
			Content:  nextContent,
			Decision: &decision,
			Handled:  true,
			Pass:     true,
			Mutated:  nextContent != content,
		}
	}

	pp.sendSecurityFlowLog(securityFlowStageFinalResult, "decision: backend=%s action=%s risk_type=%s reason=%s", detector.Name(), decision.normalizedAction(), decision.RiskType, decision.Reason)
	return finalResultPolicyResult{Decision: &decision, Handled: true, Pass: false}
}

func (ruleFinalResultPolicyHook) Evaluate(ctx context.Context, pp *ProxyProtection, policyCtx finalResultPolicyContext) finalResultPolicyResult {
	_ = ctx
	content := strings.TrimSpace(policyCtx.Content)
	if content == "" {
		return finalResultPolicyResult{}
	}
	if !policyCtx.Stream {
		pp.sendSecurityFlowLog(securityFlowStageFinalResult, "analysis_start: stream=%v content_chars=%d", policyCtx.Stream, len(content))
	}

	if strings.Contains(content, blockedToolResultPlaceholder) ||
		strings.Contains(content, "Tool result was blocked and withheld due to security risk") {
		decision := &securityPolicyDecision{
			Status:          decisionActionBlock,
			Action:          decisionActionBlock,
			ActionDesc:      "Final result references quarantined tool result",
			Reason:          "Final answer references a quarantined tool result and must not be returned.",
			RiskType:        riskContextPoisoning,
			RiskLevel:       riskLevelHigh,
			HookStage:       hookStageFinalResult,
			EvidenceSummary: truncateString(redactSecurityEvidence(content), 160),
		}
		pp.sendSecurityFlowLog(securityFlowStageFinalResult, "decision: action=%s risk_type=%s reason=%s", decision.normalizedAction(), decision.RiskType, decision.Reason)
		return finalResultPolicyResult{Decision: decision, Handled: true, Pass: false}
	}

	if pp != nil && pp.shepherdGate != nil {
		if matched, rule := matchesFinalResultSemanticRule(content, pp.shepherdGate.GetUserRules()); matched {
			action := normalizeRuleAction(rule.Action)
			if action == decisionActionAllow {
				return finalResultPolicyResult{}
			}
			riskType := strings.TrimSpace(rule.RiskType)
			if riskType == "" {
				riskType = riskHighRiskOperation
			}
			decision := &securityPolicyDecision{
				Status:          action,
				Action:          action,
				ActionDesc:      "Final result matches user-defined semantic rule",
				Reason:          fmt.Sprintf("Final answer matches user-defined rule: %s", rule.Description),
				RiskType:        riskType,
				RiskLevel:       riskLevelHigh,
				HookStage:       hookStageFinalResult,
				EvidenceSummary: truncateString(rule.Description, 160),
			}
			pp.sendSecurityFlowLog(securityFlowStageFinalResult, "decision: action=%s risk_type=%s reason=%s", decision.normalizedAction(), decision.RiskType, decision.Reason)
			return finalResultPolicyResult{Decision: decision, Handled: true, Pass: false}
		}
	}

	if evidence := finalResultDangerousAdvicePattern.FindString(content); strings.TrimSpace(evidence) != "" {
		decision := &securityPolicyDecision{
			Status:          decisionActionBlock,
			Action:          decisionActionBlock,
			ActionDesc:      "Final result contains dangerous operation guidance",
			Reason:          "Final answer contains high-risk destructive, persistence, or external execution guidance.",
			RiskType:        riskHighRiskOperation,
			RiskLevel:       riskLevelHigh,
			HookStage:       hookStageFinalResult,
			EvidenceSummary: truncateString(redactSecurityEvidence(evidence), 160),
		}
		pp.sendSecurityFlowLog(securityFlowStageFinalResult, "decision: action=%s risk_type=%s reason=%s", decision.normalizedAction(), decision.RiskType, decision.Reason)
		return finalResultPolicyResult{Decision: decision, Handled: true, Pass: false}
	}

	if evidence := finalResultTrustExploitPattern.FindString(content); strings.TrimSpace(evidence) != "" {
		decision := &securityPolicyDecision{
			Status:          decisionActionBlock,
			Action:          decisionActionBlock,
			ActionDesc:      "Final result may induce unsafe user confirmation",
			Reason:          "Final answer appears to encourage approving or ignoring a security warning.",
			RiskType:        riskHumanTrustExploitation,
			RiskLevel:       riskLevelHigh,
			HookStage:       hookStageFinalResult,
			EvidenceSummary: truncateString(redactSecurityEvidence(evidence), 160),
		}
		pp.sendSecurityFlowLog(securityFlowStageFinalResult, "decision: action=%s risk_type=%s reason=%s", decision.normalizedAction(), decision.RiskType, decision.Reason)
		return finalResultPolicyResult{Decision: decision, Handled: true, Pass: false}
	}

	redacted, labels, evidence := redactFinalResultSecrets(content)
	if redacted != content {
		decision := &securityPolicyDecision{
			Status:          decisionActionRedact,
			Action:          decisionActionRedact,
			EventType:       "redacted",
			ActionDesc:      "Final result sensitive data redacted",
			Reason:          fmt.Sprintf("Final answer contained sensitive data and was redacted: %s", strings.Join(labels, ", ")),
			RiskType:        riskSensitiveDataExfil,
			RiskLevel:       riskLevelHigh,
			HookStage:       hookStageFinalResult,
			EvidenceSummary: truncateString(redactSecurityEvidence(evidence), 160),
			WasRewritten:    true,
		}
		pp.sendSecurityFlowLog(securityFlowStageFinalResult, "decision: action=%s risk_type=%s labels=%s", decision.normalizedAction(), decision.RiskType, strings.Join(labels, ", "))
		return finalResultPolicyResult{
			Content:  redacted,
			Decision: decision,
			Handled:  true,
			Pass:     true,
			Mutated:  true,
		}
	}

	if !policyCtx.Stream {
		pp.sendSecurityFlowLog(securityFlowStageFinalResult, "decision: action=ALLOW stream=%v", policyCtx.Stream)
	}
	return finalResultPolicyResult{}
}

func matchesFinalResultSemanticRule(content string, rules *UserRules) (bool, shepherdRuleView) {
	if rules == nil {
		return false, shepherdRuleView{}
	}
	text := strings.ToLower(content)
	for _, rule := range rules.SemanticRules {
		if !rule.Enabled || !semanticRuleAppliesTo(rule.AppliesTo, hookStageFinalResult) {
			continue
		}
		ruleText := strings.ToLower(strings.TrimSpace(rule.ID + " " + rule.Description))
		if ruleText == "" {
			continue
		}
		if semanticRuleMatchesText(ruleText, text) {
			return true, shepherdRuleView{
				Description: rule.Description,
				Action:      rule.Action,
				RiskType:    rule.RiskType,
			}
		}
	}
	return false, shepherdRuleView{}
}

func redactFinalResultSecrets(content string) (string, []string, string) {
	redacted := content
	labels := make([]string, 0)
	evidence := ""
	for _, item := range finalResultSecretPatterns {
		match := item.pattern.FindString(redacted)
		if match == "" {
			continue
		}
		if evidence == "" {
			evidence = match
		}
		labels = append(labels, item.label)
		redacted = item.pattern.ReplaceAllString(redacted, "[REDACTED_SECRET]")
	}
	return redacted, labels, evidence
}

func (pp *ProxyProtection) recordFinalResultPolicyEvent(requestID string, decision securityPolicyDecision) {
	actionDesc := strings.TrimSpace(decision.ActionDesc)
	if actionDesc == "" {
		actionDesc = decision.Reason
	}
	pp.emitRiskSecurityEvent(requestID, decision.normalizedEventType(), actionDesc, riskEventMetadata{
		RiskType:        decision.RiskType,
		RiskLevel:       decision.RiskLevel,
		DecisionAction:  decision.normalizedAction(),
		HookStage:       decision.HookStage,
		EvidenceSummary: decision.EvidenceSummary,
		WasRewritten:    decision.WasRewritten,
		Reason:          decision.Reason,
	})
	pp.emitMonitorSecurityDecision(decision.normalizedStatus(), decision.Reason, false, "")
	pp.statsMu.Lock()
	pp.warningCount++
	pp.statsMu.Unlock()
	pp.sendMetricsToCallback()
}

func (pp *ProxyProtection) runFinalResultPolicyHooks(ctx context.Context, policyCtx finalResultPolicyContext) finalResultPolicyResult {
	hooks := []finalResultPolicyHook{
		detectorFinalResultPolicyHook{},
		ruleFinalResultPolicyHook{},
	}
	for _, hook := range hooks {
		result := hook.Evaluate(ctx, pp, policyCtx)
		if result.Handled {
			return result
		}
	}
	return finalResultPolicyResult{}
}
