package proxy

import (
	"fmt"
	"strings"

	"go_lib/core/logging"
)

const (
	securityFlowLogPrefix = "[ShepherdGate][Flow]"

	securityFlowStageRequest        = "request"
	securityFlowStageUserInput      = "user_input"
	securityFlowStageToolCall       = "tool_call"
	securityFlowStageToolCallResult = "tool_call_result"
	securityFlowStageRequestRewrite = "request_rewrite"
	securityFlowStageFinalResult    = "final_result"
	securityFlowStageRecovery       = "recovery"
	securityFlowStageQuarantine     = "quarantine"
	securityFlowStageChain          = "chain"
)

func formatSecurityFlowLog(stage, format string, args ...interface{}) string {
	stage = strings.TrimSpace(stage)
	if stage == "" {
		stage = "unknown"
	}
	return fmt.Sprintf("%s[%s] %s", securityFlowLogPrefix, stage, fmt.Sprintf(format, args...))
}

func (pp *ProxyProtection) sendSecurityFlowLog(stage, format string, args ...interface{}) {
	if pp == nil {
		return
	}
	pp.sendTerminalLog(formatSecurityFlowLog(stage, format, args...))
}

func logSecurityFlowInfo(stage, format string, args ...interface{}) {
	logging.Info("%s", formatSecurityFlowLog(stage, format, args...))
}

func logSecurityFlowWarning(stage, format string, args ...interface{}) {
	logging.Warning("%s", formatSecurityFlowLog(stage, format, args...))
}

func logSecurityFlowError(stage, format string, args ...interface{}) {
	logging.Error("%s", formatSecurityFlowLog(stage, format, args...))
}
