package proxy

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	chatmodelrouting "go_lib/chatmodel-routing"
	"go_lib/core/shepherd"
)

type toolResultPolicyContext struct {
	RequestID                string
	HasToolResultMessages    bool
	LatestAssistantToolCalls []toolCallRef
	ToolResultsMap           map[string]string
}

type toolResultPolicyResult struct {
	Result  *chatmodelrouting.FilterRequestResult
	Pass    bool
	Handled bool
}

type toolResultPolicyHook interface {
	Name() string
	Evaluate(ctx context.Context, pp *ProxyProtection, policyCtx toolResultPolicyContext) toolResultPolicyResult
}

type shepherdToolResultPolicyHook struct{}

func (shepherdToolResultPolicyHook) Name() string {
	return "shepherd_tool_result"
}

func toolCallIDsFromRefs(refs []toolCallRef) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if id := strings.TrimSpace(ref.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func toolCallIDsFromToolResults(results []ToolResultInfo) []string {
	ids := make([]string, 0, len(results))
	for _, result := range results {
		if id := strings.TrimSpace(result.ToolCallID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func toolResultContentByToolCallID(toolResults map[string]string, toolCallID string) (string, string, bool) {
	rawToolCallID := strings.TrimSpace(toolCallID)
	if rawToolCallID == "" || len(toolResults) == 0 {
		return "", "", false
	}
	if content, ok := toolResults[rawToolCallID]; ok {
		return content, rawToolCallID, true
	}
	matchID := normalizeBlockedToolCallID(rawToolCallID)
	if matchID == "" {
		return "", "", false
	}
	found := false
	foundID := ""
	foundContent := ""
	for rawID, content := range toolResults {
		candidateID := strings.TrimSpace(rawID)
		if normalizeBlockedToolCallID(candidateID) != matchID {
			continue
		}
		if found {
			return "", "", false
		}
		found = true
		foundID = candidateID
		foundContent = content
	}
	return foundContent, foundID, found
}

func (shepherdToolResultPolicyHook) Evaluate(ctx context.Context, pp *ProxyProtection, policyCtx toolResultPolicyContext) toolResultPolicyResult {
	if !policyCtx.HasToolResultMessages || pp.shepherdGate == nil {
		return toolResultPolicyResult{}
	}

	chain := pp.chainByRequestID(policyCtx.RequestID)
	armed := chain != nil && chain.PendingRecoveryArmed

	if armed {
		toolCallIDs := append(pp.pendingRecoveryToolCallIDsForRequest(policyCtx.RequestID), toolCallIDsFromRefs(policyCtx.LatestAssistantToolCalls)...)
		cleared := pp.clearBlockedToolCallIDsForRequest(policyCtx.RequestID, toolCallIDs)
		pp.clearPendingToolCallRecoveryForRequest(policyCtx.RequestID)
		pp.sendSecurityFlowLog(securityFlowStageRecovery, "recovery is armed; skipping tool_result analysis and allowing request_id=%s cleared_blocked_tool_call_ids=%d", policyCtx.RequestID, cleared)
		pp.sendLog("proxy_tool_result_recovery_allowed", map[string]interface{}{
			"armed": true,
			"ids":   toolCallIDs,
		})
		pp.emitMonitorSecurityDecision("RECOVERY_ALLOWED", "user confirmed recovery", false, "")
		return toolResultPolicyResult{}
	}

	pp.configMu.RLock()
	auditOnlyForShepherd := pp.auditOnly
	pp.configMu.RUnlock()

	if auditOnlyForShepherd {
		logSecurityFlowInfo(securityFlowStageToolCallResult, "audit_only=true; skipping ShepherdGate analysis")
		pp.sendSecurityFlowLog(securityFlowStageToolCallResult, "audit_only=true; allowing tool results without blocking")
		return toolResultPolicyResult{}
	}

	toolCallInfos := make([]ToolCallInfo, 0, len(policyCtx.LatestAssistantToolCalls))
	for _, tcRef := range policyCtx.LatestAssistantToolCalls {
		info := ToolCallInfo{
			Name:       tcRef.FuncName,
			RawArgs:    tcRef.RawArgs,
			ToolCallID: tcRef.ID,
		}
		if pp.toolValidator != nil {
			info.IsSensitive = pp.toolValidator.IsSensitive(tcRef.FuncName)
		}
		if tcRef.RawArgs != "" {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tcRef.RawArgs), &args); err == nil {
				info.Arguments = args
			}
		}
		toolCallInfos = append(toolCallInfos, info)
	}

	toolResultInfos := make([]ToolResultInfo, 0, len(policyCtx.ToolResultsMap))
	matchedToolResultIDs := make(map[string]struct{})
	for _, tcRef := range policyCtx.LatestAssistantToolCalls {
		if content, resultID, ok := toolResultContentByToolCallID(policyCtx.ToolResultsMap, tcRef.ID); ok {
			matchedToolResultIDs[resultID] = struct{}{}
			toolResultInfos = append(toolResultInfos, ToolResultInfo{
				ToolCallID: resultID,
				FuncName:   tcRef.FuncName,
				Content:    content,
			})
		}
	}
	for resultID, content := range policyCtx.ToolResultsMap {
		resultID = strings.TrimSpace(resultID)
		if resultID == "" {
			continue
		}
		if _, ok := matchedToolResultIDs[resultID]; ok {
			continue
		}
		toolResultInfos = append(toolResultInfos, ToolResultInfo{
			ToolCallID: resultID,
			FuncName:   "tool_result",
			Content:    content,
		})
	}

	for _, tr := range toolResultInfos {
		if !isClawdSecbotSandboxBlockedToolResult(tr.Content) {
			continue
		}
		if !pp.markSandboxBlockedToolResultIfFirst(tr.ToolCallID) {
			continue
		}

		pp.sendSecurityFlowLog(securityFlowStageToolCallResult,
			"sandbox already blocked tool result; skipping duplicate confirmation: tool=%s tool_call_id=%s",
			tr.FuncName,
			tr.ToolCallID,
		)
		pp.sendLog("proxy_tool_result_sandbox_blocked", map[string]interface{}{
			"tool_id":  tr.ToolCallID,
			"tool":     tr.FuncName,
			"detected": true,
		})
		pp.markBlockedToolCallIDsForRequest(policyCtx.RequestID, []string{tr.ToolCallID})

		sandboxReason := "tool result already blocked by ClawdSecbot sandbox"
		pp.emitMonitorSecurityDecision(
			"SANDBOX_BLOCKED",
			sandboxReason,
			false,
			"",
		)
		pp.emitSecurityEvent(policyCtx.RequestID, "blocked", "Sandbox blocked tool execution", "SANDBOX_BLOCKED", sandboxReason)
		pp.auditLogSafe("set_decision_sandbox_blocked", func(tracker *AuditChainTracker) {
			tracker.SetRequestDecision(
				policyCtx.RequestID,
				"BLOCK",
				"SANDBOX_BLOCKED",
				sandboxReason,
				100,
			)
		})
		return toolResultPolicyResult{Pass: true, Handled: true}
	}

	toolNames := make([]string, 0, len(toolCallInfos))
	for _, tc := range toolCallInfos {
		toolNames = append(toolNames, tc.Name)
	}
	pp.updateTruthRecord(policyCtx.RequestID, func(r *TruthRecord) {
		// Tool names/count will be computed from ToolCalls by frontend getters
	})
	pp.sendSecurityFlowLog(securityFlowStageToolCallResult, "analysis_start: tool_result_count=%d tools=%s", len(toolResultInfos), strings.Join(toolNames, ", "))

	_, _, chainMeta, hasChain := pp.securityChainContext(policyCtx.RequestID)
	if hasChain && chainMeta.Degraded {
		pp.sendSecurityFlowLog(securityFlowStageChain, "chain_degraded_context: request_id=%s instruction_chain_id=%s reason=%s", policyCtx.RequestID, chainMeta.ChainID, chainMeta.DegradeReason)
	}

	detector := pp.currentSecurityDetector()
	if detector == nil {
		logSecurityFlowError(securityFlowStageToolCallResult, "analysis_failed: detector_missing action=fail_open")
		return toolResultPolicyResult{}
	}
	securityModel := ""
	if pp.shepherdGate != nil {
		securityModel = pp.shepherdGate.GetModelName()
	}
	logSecurityFlowInfo(securityFlowStageToolCallResult, "deep_analysis_triggered: backend=%s tool_calls=%d tool_results=%d security_model=%s", detector.Name(), len(toolCallInfos), len(toolResultInfos), securityModel)

	// Use the proxy lifecycle context instead of the request context so a
	// client-side disconnect does not cancel security analysis mid-flight.
	checkCtx := pp.ctx
	if checkCtx == nil {
		checkCtx = context.Background()
	}
	checkCtx = shepherd.WithBotID(checkCtx, pp.assetID)
	response, err := detector.Detect(checkCtx, securityDetectionRequest{
		Stage:       hookStageToolCallResult,
		RequestID:   policyCtx.RequestID,
		ToolCalls:   toolCallInfos,
		ToolResults: toolResultInfos,
	})

	pp.statsMu.Lock()
	pp.analysisCount++
	pp.statsMu.Unlock()
	pp.sendMetricsToCallback()

	pp.recordDetectionUsage(securityFlowStageToolCallResult, response)

	if err != nil {
		logSecurityFlowError(securityFlowStageToolCallResult, "analysis_failed: backend=%s err=%v action=fail_open", detector.Name(), err)
		return toolResultPolicyResult{}
	}
	if response == nil || detectionResponseAllowed(response) {
		decision := shepherdDecisionFromDetectionResponse(response)
		if decision == nil {
			allowed := true
			decision = &shepherd.ShepherdDecision{Status: "ALLOWED", Allowed: &allowed}
		}
		logSecurityFlowInfo(securityFlowStageToolCallResult, "decision: status=ALLOWED tools=%s", strings.Join(toolNames, ", "))
		pp.sendSecurityFlowLog(securityFlowStageToolCallResult, "decision: status=ALLOWED tools=%s", strings.Join(toolNames, ", "))
		pp.sendLog("proxy_tool_result_decision", map[string]interface{}{
			"status":      decision.Status,
			"reason":      decision.Reason,
			"blocked":     false,
			"skill":       decision.Skill,
			"action_desc": decision.ActionDesc,
			"risk_type":   decision.RiskType,
		})
		pp.emitMonitorSecurityDecision(decision.Status, decision.Reason, false, "")
		pp.updateTruthRecord(policyCtx.RequestID, func(r *TruthRecord) {
			r.Decision = &SecurityDecision{
				Action: "ALLOW",
				Reason: decision.Reason,
			}
		})
		pp.auditLogSafe("set_decision_shepherd_allowed", func(tracker *AuditChainTracker) {
			tracker.SetRequestDecision(policyCtx.RequestID, "ALLOW", "", decision.Reason, 0)
		})
		return toolResultPolicyResult{}
	}
	decision := shepherdDecisionFromDetectionResponse(response)
	if decision == nil {
		decision = &shepherd.ShepherdDecision{
			Status:     normalizeDetectionAction(response.Action, response.Status),
			Allowed:    response.Allowed,
			Reason:     response.Reason,
			ActionDesc: response.ActionDesc,
			RiskType:   response.RiskType,
		}
	}

	logSecurityFlowInfo(securityFlowStageToolCallResult, "decision: status=%s reason=%s", decision.Status, decision.Reason)
	pp.sendSecurityFlowLog(securityFlowStageToolCallResult, "decision: status=%s reason=%s", decision.Status, decision.Reason)
	pp.sendLog("proxy_tool_result_decision", map[string]interface{}{
		"status":      decision.Status,
		"reason":      decision.Reason,
		"blocked":     true,
		"skill":       decision.Skill,
		"action_desc": decision.ActionDesc,
		"risk_type":   decision.RiskType,
	})

	toolCallIDs := toolCallIDsFromToolResults(toolResultInfos)
	pp.markBlockedToolCallIDsForRequest(policyCtx.RequestID, toolCallIDs)
	pp.storePendingToolCallRecoveryWithRiskForRequest(policyCtx.RequestID, nil, toolCallIDs, "", decision.Reason, "tool_result", decision.RiskType)

	securityMsg := pp.shepherdGate.FormatSecurityMockReply(decision)
	pp.emitMonitorSecurityDecision(decision.Status, decision.Reason, true, securityMsg)
	recordAction := "BLOCK"
	recordRiskLevel := "BLOCKED"
	if decision.Status == "NEEDS_CONFIRMATION" {
		recordAction = "NEEDS_CONFIRMATION"
		recordRiskLevel = "NEEDS_CONFIRMATION"
	}
	pp.updateTruthRecord(policyCtx.RequestID, func(r *TruthRecord) {
		r.Phase = RecordPhaseStopped
		r.CompletedAt = time.Now().Format(time.RFC3339Nano)
		r.Decision = &SecurityDecision{
			Action:     recordAction,
			RiskLevel:  recordRiskLevel,
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

	shepherdActionDesc := strings.TrimSpace(decision.ActionDesc)
	if shepherdActionDesc == "" {
		shepherdActionDesc = decision.Reason
	}
	shepherdRiskType := strings.TrimSpace(decision.RiskType)
	if shepherdRiskType == "" {
		shepherdRiskType = decision.Status
	}
	shepherdDetail := decision.Reason
	if strings.Contains(decision.Reason, shepherd.PostValidationOverrideTag) {
		shepherdDetail = "post_validation_override | " + decision.Reason
	}
	shepherdEventType := "blocked"
	if decision.Status == "NEEDS_CONFIRMATION" {
		shepherdEventType = "needs_confirmation"
	}
	pp.emitSecurityEvent(policyCtx.RequestID, shepherdEventType, shepherdActionDesc, shepherdRiskType, shepherdDetail)
	pp.emitMonitorResponseReturned(decision.Status, securityMsg, securityMsg)
	pp.auditLogSafe("set_decision_shepherd_blocked", func(tracker *AuditChainTracker) {
		tracker.SetRequestDecision(policyCtx.RequestID, recordAction, recordRiskLevel, decision.Reason, 100)
		tracker.FinalizeRequestOutput(policyCtx.RequestID, securityMsg)
	})
	pp.clearRequestContext(ctx)
	pp.clearRequestRuntimeState(policyCtx.RequestID)
	return toolResultPolicyResult{
		Result:  &chatmodelrouting.FilterRequestResult{MockContent: securityMsg},
		Pass:    false,
		Handled: true,
	}
}

func (pp *ProxyProtection) runToolResultPolicyHooks(ctx context.Context, policyCtx toolResultPolicyContext) toolResultPolicyResult {
	hooks := []toolResultPolicyHook{
		shepherdToolResultPolicyHook{},
	}
	for _, hook := range hooks {
		result := hook.Evaluate(ctx, pp, policyCtx)
		if result.Handled {
			return result
		}
	}
	return toolResultPolicyResult{}
}
