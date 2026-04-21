package api

import "strings"

// canonicalRiskID normalizes risk aliases for stable export keys.
func canonicalRiskID(riskID string) string {
	switch strings.TrimSpace(riskID) {
	case "gateway_bind_unsafe":
		return "riskNonLoopbackBinding"
	case "riskNoAuth":
		return "gateway_auth_disabled"
	case "riskWeakPassword":
		return "gateway_weak_password"
	case "openclaw_1click_rce_vulnerability", "nullclaw_1click_rce_vulnerability":
		return "riskOneClickRce"
	default:
		return strings.TrimSpace(riskID)
	}
}

func riskTitleKey(riskID string) string {
	switch canonicalRiskID(riskID) {
	case "riskNonLoopbackBinding":
		return "riskNonLoopbackBinding"
	case "gateway_auth_disabled":
		return "riskNoAuth"
	case "gateway_auth_password_mode":
		return "riskGatewayAuthPasswordMode"
	case "gateway_weak_password":
		return "riskWeakPassword"
	case "gateway_weak_token":
		return "riskGatewayWeakToken"
	case "riskAllPluginsAllowed":
		return "riskAllPluginsAllowed"
	case "riskControlUiEnabled":
		return "riskControlUiEnabled"
	case "riskRunningAsRoot":
		return "riskRunningAsRoot"
	case "config_perm_unsafe":
		return "riskConfigPermUnsafe"
	case "config_dir_perm_unsafe":
		return "riskConfigDirPermUnsafe"
	case "sandbox_disabled_default":
		return "riskSandboxDisabledDefault"
	case "sandbox_disabled_agent":
		return "riskSandboxDisabledAgent"
	case "logging_redact_off":
		return "riskLoggingRedactOff"
	case "audit_disabled":
		return "riskAuditDisabled"
	case "autonomy_workspace_unrestricted":
		return "riskAutonomyWorkspaceUnrestricted"
	case "log_dir_perm_unsafe":
		return "riskLogDirPermUnsafe"
	case "plaintext_secrets":
		return "riskPlaintextSecrets"
	case "memory_dir_perm_unsafe":
		return "riskMemoryDirPermUnsafe"
	case "process_running_as_root":
		return "riskProcessRunningAsRoot"
	case "skill_agent_risk":
		return "riskSkillAgentRisk"
	case "skills_not_scanned":
		return "riskSkillsNotScanned"
	case "riskOneClickRce":
		return "riskOneClickRce"
	case "terminal_backend_local":
		return "riskTerminalBackendLocal"
	case "approvals_mode_disabled":
		return "riskApprovalsModeDisabled"
	case "redact_secrets_disabled":
		return "riskRedactSecretsDisabled"
	case "model_base_url_public":
		return "riskModelBaseUrlPublic"
	case "riskSkillSecurityIssue":
		return "riskSkillSecurityIssue"
	default:
		return canonicalRiskID(riskID)
	}
}

func riskDescriptionKey(riskID string) string {
	switch canonicalRiskID(riskID) {
	case "riskNonLoopbackBinding":
		return "riskNonLoopbackBindingDesc"
	case "gateway_auth_disabled":
		return "riskNoAuthDesc"
	case "gateway_auth_password_mode":
		return "riskGatewayAuthPasswordModeDesc"
	case "gateway_weak_password":
		return "riskWeakPasswordDesc"
	case "gateway_weak_token":
		return "riskGatewayWeakTokenDesc"
	case "riskAllPluginsAllowed":
		return "riskAllPluginsAllowedDesc"
	case "riskControlUiEnabled":
		return "riskControlUiEnabledDesc"
	case "riskRunningAsRoot":
		return "riskRunningAsRootDesc"
	case "config_perm_unsafe":
		return "riskConfigPermUnsafeDesc"
	case "config_dir_perm_unsafe":
		return "riskConfigDirPermUnsafeDesc"
	case "sandbox_disabled_default":
		return "riskSandboxDisabledDefaultDesc"
	case "sandbox_disabled_agent":
		return "riskSandboxDisabledAgentDesc"
	case "logging_redact_off":
		return "riskLoggingRedactOffDesc"
	case "audit_disabled":
		return "riskAuditDisabledDesc"
	case "autonomy_workspace_unrestricted":
		return "riskAutonomyWorkspaceUnrestrictedDesc"
	case "log_dir_perm_unsafe":
		return "riskLogDirPermUnsafeDesc"
	case "plaintext_secrets":
		return "riskPlaintextSecretsDesc"
	case "memory_dir_perm_unsafe":
		return "riskMemoryDirPermUnsafeDesc"
	case "process_running_as_root":
		return "riskProcessRunningAsRootDesc"
	case "skill_agent_risk":
		return "riskSkillAgentRiskDesc"
	case "skills_not_scanned":
		return "riskSkillsNotScannedDesc"
	case "riskOneClickRce":
		return "riskOneClickRceDesc"
	case "terminal_backend_local":
		return "riskTerminalBackendLocalDesc"
	case "approvals_mode_disabled":
		return "riskApprovalsModeDisabledDesc"
	case "redact_secrets_disabled":
		return "riskRedactSecretsDisabledDesc"
	case "model_base_url_public":
		return "riskModelBaseUrlPublicDesc"
	case "riskSkillSecurityIssue":
		return "riskSkillSecurityIssueDesc"
	default:
		key := strings.TrimSpace(canonicalRiskID(riskID))
		if key == "" {
			return ""
		}
		return key + "Desc"
	}
}

func replaceMitigationDescWithKey(infos []MitigationInfo, descKey string) []MitigationInfo {
	if len(infos) == 0 || strings.TrimSpace(descKey) == "" {
		return infos
	}
	out := make([]MitigationInfo, 0, len(infos))
	for _, info := range infos {
		info.Desc = descKey
		out = append(out, info)
	}
	return out
}
