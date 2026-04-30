package shepherd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go_lib/core/logging"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// ==================== record_security_event ====================

// NewRecordSecurityEventTool creates the record_security_event tool.
func NewRecordSecurityEventTool(assetName, assetID string, requestID ...string) tool.BaseTool {
	reqID := ""
	if len(requestID) > 0 {
		reqID = requestID[0]
	}
	return &recordSecurityEventTool{
		assetName: strings.TrimSpace(assetName),
		assetID:   strings.TrimSpace(assetID),
		requestID: strings.TrimSpace(reqID),
	}
}

type recordSecurityEventTool struct {
	assetName string
	assetID   string
	requestID string
}

func (t *recordSecurityEventTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "record_security_event",
		Desc: `Record a security event during analysis. Call this tool to log notable actions:
- tool_execution: a tool was executed (allowed)
- blocked: a tool call was blocked or intercepted due to risk
- other: any other security-relevant observation
Provide action_desc in the user's language describing what happened, and risk_type categorizing the risk.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"event_type":  {Type: schema.String, Required: true, Desc: "Event type: tool_execution | blocked | other"},
			"action_desc": {Type: schema.String, Required: true, Desc: "Human-readable description of the action, in the user's language"},
			"risk_type":   {Type: schema.String, Required: false, Desc: "Risk category, e.g. '数据外泄', '权限提升', '文件篡改', 'prompt注入'. Empty if safe."},
			"detail":      {Type: schema.String, Required: false, Desc: "Additional detail or context"},
		}),
	}, nil
}

func (t *recordSecurityEventTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		EventType  string `json:"event_type"`
		ActionDesc string `json:"action_desc"`
		RiskType   string `json:"risk_type"`
		Detail     string `json:"detail"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	eventType := strings.TrimSpace(args.EventType)
	if eventType == "" {
		return "", fmt.Errorf("event_type is required")
	}
	switch eventType {
	case "tool_execution", "blocked", "other":
		// ok
	default:
		eventType = "other"
	}

	actionDesc := strings.TrimSpace(args.ActionDesc)
	if actionDesc == "" {
		return "", fmt.Errorf("action_desc is required")
	}

	event := SecurityEvent{
		BotID:      botIDFromContext(ctx),
		EventType:  eventType,
		ActionDesc: actionDesc,
		RiskType:   strings.TrimSpace(args.RiskType),
		Detail:     strings.TrimSpace(args.Detail),
		Source:     "react_agent",
		AssetName:  t.assetName,
		AssetID:    t.assetID,
		RequestID:  t.requestID,
	}
	securityEventBuffer.AddSecurityEvent(event)

	logging.ShepherdGateInfo("%s[react][Tool:record_security_event][-] type=%s action=%s risk=%s",
		shepherdFlowLogPrefix,
		event.EventType, shortenForLog(event.ActionDesc, 120), event.RiskType)

	result := map[string]interface{}{
		"recorded": true,
		"id":       event.ID,
	}
	b, _ := json.Marshal(result)
	return string(b), nil
}
