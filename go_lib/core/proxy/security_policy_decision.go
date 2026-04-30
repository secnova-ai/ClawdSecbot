package proxy

import (
	"context"
	"fmt"
	"strings"
	"time"

	chatmodelrouting "go_lib/chatmodel-routing"
	"go_lib/core/shepherd"
)

type securityPolicyDecision struct {
	Status          string
	Action          string
	EventType       string
	ActionDesc      string
	Reason          string
	RiskType        string
	RiskLevel       string
	HookStage       string
	EvidenceSummary string
	ToolCallID      string
	WasRewritten    bool
	WasQuarantined  bool
}

func (d securityPolicyDecision) normalizedAction() string {
	if strings.TrimSpace(d.Action) != "" {
		return strings.TrimSpace(d.Action)
	}
	if strings.EqualFold(d.Status, decisionActionBlock) {
		return decisionActionBlock
	}
	return decisionActionNeedsConfirm
}

func (d securityPolicyDecision) normalizedEventType() string {
	if strings.TrimSpace(d.EventType) != "" {
		return strings.TrimSpace(d.EventType)
	}
	if d.normalizedAction() == decisionActionNeedsConfirm {
		return "needs_confirmation"
	}
	return "blocked"
}

func (d securityPolicyDecision) normalizedStatus() string {
	if strings.TrimSpace(d.Status) != "" {
		return strings.TrimSpace(d.Status)
	}
	return d.normalizedAction()
}

func (pp *ProxyProtection) formatPolicySecurityMessage(decision securityPolicyDecision) string {
	allowed := false
	var msg string
	if pp.shepherdGate != nil {
		msg = pp.shepherdGate.FormatSecurityMockReply(&shepherd.ShepherdDecision{
			Status:     decision.normalizedStatus(),
			Allowed:    &allowed,
			Reason:     decision.Reason,
			ActionDesc: decision.ActionDesc,
			RiskType:   decision.RiskType,
			Skill:      "policy_hook",
		})
	} else {
		msg = fmt.Sprintf("[ShepherdGate] Status: %s | Reason: %s", decision.normalizedStatus(), decision.Reason)
	}
	if guidance := contaminatedSessionGuidance(decision, userInputPolicyLanguage(pp)); guidance != "" {
		msg = strings.TrimSpace(msg) + "\n\n" + guidance
	}
	return msg
}

func contaminatedSessionGuidance(decision securityPolicyDecision, lang string) string {
	if decision.HookStage != hookStageUserInput {
		return ""
	}
	if strings.ToUpper(strings.TrimSpace(decision.RiskType)) != riskPromptInjectionDirect {
		return ""
	}
	if shepherd.NormalizeShepherdLanguage(lang) == "zh" {
		return "请开启新的会话恢复对话，本轮会话已被污染，继续对话将被拦截。"
	}
	return "Please start a new session to continue. This conversation has been contaminated; continuing in this session will be blocked."
}

func (pp *ProxyProtection) applyRequestSecurityPolicyDecision(ctx context.Context, requestID string, decision securityPolicyDecision) (*chatmodelrouting.FilterRequestResult, bool) {
	securityMsg := pp.formatPolicySecurityMessage(decision)
	action := decision.normalizedAction()
	riskLevel := strings.ToUpper(strings.TrimSpace(decision.RiskLevel))
	if riskLevel == "" {
		riskLevel = strings.ToUpper(strings.TrimSpace(decision.RiskType))
	}
	if riskLevel == "" {
		riskLevel = "HIGH"
	}

	if action == decisionActionNeedsConfirm && !(decision.HookStage == hookStageToolCallResult && pp.hasPendingToolCallRecoveryForRequest(requestID)) {
		pp.storePendingToolCallRecoveryWithRiskForRequest(requestID, nil, nil, "", decision.Reason, decision.HookStage, decision.RiskType)
	}
	pp.sendSecurityFlowLog(decision.HookStage, "request_decision: action=%s risk_type=%s risk_level=%s reason=%s", action, decision.RiskType, riskLevel, decision.Reason)
	pp.emitMonitorSecurityDecision(decision.normalizedStatus(), decision.Reason, true, securityMsg)
	pp.updateTruthRecord(requestID, func(r *TruthRecord) {
		r.Phase = RecordPhaseStopped
		r.CompletedAt = time.Now().Format(time.RFC3339Nano)
		r.Decision = &SecurityDecision{
			Action:     action,
			RiskLevel:  riskLevel,
			Reason:     decision.Reason,
			Confidence: 100,
		}
		applyRecordPrimaryContent(r, RecordContentSecurity, securityMsg, true)
	})

	pp.statsMu.Lock()
	pp.blockedCount++
	pp.warningCount++
	pp.statsMu.Unlock()
	pp.sendMetricsToCallback()

	actionDesc := strings.TrimSpace(decision.ActionDesc)
	if actionDesc == "" {
		actionDesc = decision.Reason
	}
	pp.emitRiskSecurityEvent(requestID, decision.normalizedEventType(), actionDesc, riskEventMetadata{
		RiskType:        decision.RiskType,
		RiskLevel:       decision.RiskLevel,
		DecisionAction:  action,
		HookStage:       decision.HookStage,
		ToolCallID:      decision.ToolCallID,
		EvidenceSummary: decision.EvidenceSummary,
		WasRewritten:    decision.WasRewritten,
		WasQuarantined:  decision.WasQuarantined,
		Reason:          decision.Reason,
	})
	pp.emitMonitorResponseReturned(decision.normalizedStatus(), securityMsg, securityMsg)
	pp.auditLogSafe("set_decision_policy_blocked", func(tracker *AuditChainTracker) {
		tracker.SetRequestDecision(requestID, action, riskLevel, decision.Reason, 100)
		tracker.FinalizeRequestOutput(requestID, securityMsg)
	})
	pp.clearRequestContext(ctx)
	pp.clearRequestRuntimeState(requestID)

	return &chatmodelrouting.FilterRequestResult{MockContent: securityMsg}, false
}

func (pp *ProxyProtection) applyResponseSecurityPolicyDecision(ctx context.Context, requestID string, decision securityPolicyDecision, clearContext bool) bool {
	securityMsg := pp.formatPolicySecurityMessage(decision)
	action := decision.normalizedAction()
	riskLevel := strings.ToUpper(strings.TrimSpace(decision.RiskLevel))
	if riskLevel == "" {
		riskLevel = strings.ToUpper(strings.TrimSpace(decision.RiskType))
	}
	if riskLevel == "" {
		riskLevel = "HIGH"
	}

	if action == decisionActionNeedsConfirm {
		var toolCallIDs []string
		if toolCallID := normalizeBlockedToolCallID(decision.ToolCallID); toolCallID != "" {
			toolCallIDs = []string{toolCallID}
			pp.markBlockedToolCallIDsForRequest(requestID, toolCallIDs)
		}
		pp.storePendingToolCallRecoveryWithRiskForRequest(requestID, nil, toolCallIDs, "", decision.Reason, decision.HookStage, decision.RiskType)
	}
	pp.sendSecurityFlowLog(decision.HookStage, "response_decision: action=%s risk_type=%s risk_level=%s reason=%s", action, decision.RiskType, riskLevel, decision.Reason)
	pp.emitMonitorSecurityDecision(decision.normalizedStatus(), decision.Reason, true, securityMsg)
	pp.updateTruthRecord(requestID, func(r *TruthRecord) {
		r.Phase = RecordPhaseStopped
		r.CompletedAt = time.Now().Format(time.RFC3339Nano)
		r.Decision = &SecurityDecision{
			Action:     action,
			RiskLevel:  riskLevel,
			Reason:     decision.Reason,
			Confidence: 100,
		}
		applyRecordPrimaryContent(r, RecordContentSecurity, securityMsg, true)
	})

	pp.statsMu.Lock()
	pp.blockedCount++
	pp.warningCount++
	pp.statsMu.Unlock()
	pp.sendMetricsToCallback()

	actionDesc := strings.TrimSpace(decision.ActionDesc)
	if actionDesc == "" {
		actionDesc = decision.Reason
	}
	pp.emitRiskSecurityEvent(requestID, decision.normalizedEventType(), actionDesc, riskEventMetadata{
		RiskType:        decision.RiskType,
		RiskLevel:       decision.RiskLevel,
		DecisionAction:  action,
		HookStage:       decision.HookStage,
		ToolCallID:      decision.ToolCallID,
		EvidenceSummary: decision.EvidenceSummary,
		WasRewritten:    decision.WasRewritten,
		WasQuarantined:  decision.WasQuarantined,
		Reason:          decision.Reason,
	})
	pp.emitMonitorResponseReturned(decision.normalizedStatus(), securityMsg, securityMsg)
	pp.auditLogSafe("set_decision_response_policy_blocked", func(tracker *AuditChainTracker) {
		tracker.SetRequestDecision(requestID, action, riskLevel, decision.Reason, 100)
		tracker.FinalizeRequestOutput(requestID, securityMsg)
	})
	if clearContext {
		pp.clearRequestContext(ctx)
	}
	pp.clearRequestRuntimeState(requestID)
	return false
}
