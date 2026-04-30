package proxy

import (
	"encoding/json"
	"strings"
)

const (
	riskPromptInjectionDirect   = "PROMPT_INJECTION_DIRECT"
	riskPromptInjectionIndirect = "PROMPT_INJECTION_INDIRECT"
	riskSensitiveDataExfil      = "SENSITIVE_DATA_EXFILTRATION"
	riskHighRiskOperation       = "HIGH_RISK_OPERATION"
	riskPrivilegeAbuse          = "PRIVILEGE_ABUSE"
	riskUnexpectedCodeExecution = "UNEXPECTED_CODE_EXECUTION"
	riskContextPoisoning        = "CONTEXT_POISONING"
	riskSupplyChain             = "SUPPLY_CHAIN_RISK"
	riskHumanTrustExploitation  = "HUMAN_TRUST_EXPLOITATION"
	riskCascadingFailure        = "CASCADING_FAILURE"
	riskSandboxBlocked          = "SANDBOX_BLOCKED"
	riskQuota                   = "QUOTA"
	hookStageUserInput          = "user_input"
	hookStageToolCall           = "tool_call"
	hookStageToolCallResult     = "tool_call_result"
	hookStageRequestRewrite     = "request_rewrite"
	hookStageFinalResult        = "final_result"
	decisionActionAllow         = "ALLOW"
	decisionActionNeedsConfirm  = "NEEDS_CONFIRMATION"
	decisionActionBlock         = "BLOCK"
	decisionActionRewrite       = "REWRITE"
	decisionActionRedact        = "REDACT"
	riskLevelLow                = "low"
	riskLevelMedium             = "medium"
	riskLevelHigh               = "high"
	riskLevelCritical           = "critical"
)

type riskEventMetadata struct {
	RiskType           string   `json:"risk_type"`
	RiskLevel          string   `json:"risk_level,omitempty"`
	OWASPAgenticIDs    []string `json:"owasp_agentic_ids,omitempty"`
	DecisionAction     string   `json:"decision_action,omitempty"`
	HookStage          string   `json:"hook_stage,omitempty"`
	ToolCallID         string   `json:"tool_call_id,omitempty"`
	RequestID          string   `json:"request_id,omitempty"`
	AssetID            string   `json:"asset_id,omitempty"`
	InstructionChainID string   `json:"instruction_chain_id,omitempty"`
	ChainSource        string   `json:"chain_source,omitempty"`
	ChainDegraded      bool     `json:"chain_degraded,omitempty"`
	ChainDegradeReason string   `json:"chain_degrade_reason,omitempty"`
	EvidenceSummary    string   `json:"evidence_summary,omitempty"`
	WasRewritten       bool     `json:"was_rewritten,omitempty"`
	WasQuarantined     bool     `json:"was_quarantined,omitempty"`
	Reason             string   `json:"reason,omitempty"`
}

func owaspAgenticIDsForRisk(riskType string) []string {
	switch strings.ToUpper(strings.TrimSpace(riskType)) {
	case riskPromptInjectionDirect:
		return []string{"ASI01"}
	case riskPromptInjectionIndirect:
		return []string{"ASI01", "ASI06"}
	case riskSensitiveDataExfil:
		return []string{"ASI02", "ASI03"}
	case riskHighRiskOperation:
		return []string{"ASI02"}
	case riskPrivilegeAbuse:
		return []string{"ASI03"}
	case riskUnexpectedCodeExecution:
		return []string{"ASI05"}
	case riskContextPoisoning:
		return []string{"ASI06"}
	case riskSupplyChain:
		return []string{"ASI04"}
	case riskHumanTrustExploitation:
		return []string{"ASI09"}
	case riskCascadingFailure:
		return []string{"ASI08"}
	default:
		return nil
	}
}

func buildRiskEventDetail(meta riskEventMetadata) string {
	if len(meta.OWASPAgenticIDs) == 0 {
		meta.OWASPAgenticIDs = owaspAgenticIDsForRisk(meta.RiskType)
	}
	meta.EvidenceSummary = redactSecurityEvidence(meta.EvidenceSummary)
	data, err := json.Marshal(meta)
	if err != nil {
		return strings.TrimSpace(meta.Reason)
	}
	return string(data)
}

func (pp *ProxyProtection) emitRiskSecurityEvent(requestID, eventType, actionDesc string, meta riskEventMetadata) {
	meta.RequestID = strings.TrimSpace(requestID)
	meta.AssetID = strings.TrimSpace(pp.assetID)
	chainMeta := pp.chainMetadataForRequest(requestID)
	meta.InstructionChainID = chainMeta.ChainID
	meta.ChainSource = chainMeta.Source
	meta.ChainDegraded = chainMeta.Degraded
	meta.ChainDegradeReason = chainMeta.DegradeReason
	if meta.RiskType == "" {
		meta.RiskType = riskHighRiskOperation
	}
	detail := buildRiskEventDetail(meta)
	pp.emitSecurityEvent(requestID, eventType, actionDesc, meta.RiskType, detail)
}
