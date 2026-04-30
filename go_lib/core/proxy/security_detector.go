package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"go_lib/core/shepherd"
)

const (
	detectionBackendLLMAgent  = "llm_agent"
	detectionBackendRemoteAPI = "remote_api"
	detectionBackendReserved  = "reserved"

	defaultRemoteDetectionTimeout = 30 * time.Second
)

type securityDetectionRequest struct {
	Stage        string           `json:"stage"`
	RequestID    string           `json:"request_id,omitempty"`
	AssetName    string           `json:"asset_name,omitempty"`
	AssetID      string           `json:"asset_id,omitempty"`
	UserInput    string           `json:"user_input,omitempty"`
	ToolCalls    []ToolCallInfo   `json:"tool_calls,omitempty"`
	ToolResults  []ToolResultInfo `json:"tool_results,omitempty"`
	FinalContent string           `json:"final_content,omitempty"`
	Stream       bool             `json:"stream,omitempty"`
}

type securityDetectionResponse struct {
	Allowed         *bool  `json:"allowed,omitempty"`
	Status          string `json:"status,omitempty"`
	Action          string `json:"action,omitempty"`
	EventType       string `json:"event_type,omitempty"`
	ActionDesc      string `json:"action_desc,omitempty"`
	Reason          string `json:"reason,omitempty"`
	RiskType        string `json:"risk_type,omitempty"`
	RiskLevel       string `json:"risk_level,omitempty"`
	EvidenceSummary string `json:"evidence_summary,omitempty"`
	ToolCallID      string `json:"tool_call_id,omitempty"`
	Content         string `json:"content,omitempty"`
	Mutated         bool   `json:"mutated,omitempty"`
	WasRewritten    bool   `json:"was_rewritten,omitempty"`
	WasQuarantined  bool   `json:"was_quarantined,omitempty"`
	Usage           *Usage `json:"usage,omitempty"`
}

type securityDetector interface {
	Name() string
	Detect(ctx context.Context, req securityDetectionRequest) (*securityDetectionResponse, error)
}

type llmAgentSecurityDetector struct {
	gate *shepherd.ShepherdGate
}

func (d *llmAgentSecurityDetector) Name() string {
	return detectionBackendLLMAgent
}

func (d *llmAgentSecurityDetector) Detect(ctx context.Context, req securityDetectionRequest) (*securityDetectionResponse, error) {
	if d == nil || d.gate == nil {
		return nil, nil
	}
	switch req.Stage {
	case hookStageUserInput:
		decision, err := d.gate.CheckUserInput(ctx, req.UserInput, req.RequestID)
		return detectionResponseFromShepherdDecision(decision), err
	case hookStageToolCall, hookStageToolCallResult:
		decision, err := d.gate.CheckToolCall(ctx, req.ToolCalls, req.ToolResults, req.RequestID)
		return detectionResponseFromShepherdDecision(decision), err
	case hookStageFinalResult:
		decision, err := d.gate.CheckFinalResult(ctx, req.FinalContent, req.RequestID)
		return detectionResponseFromShepherdDecision(decision), err
	default:
		return nil, nil
	}
}

type remoteAPISecurityDetector struct {
	endpoint string
	apiKey   string
	client   *http.Client
	asset    remoteDetectionAsset
}

type remoteDetectionAsset struct {
	Name string
	ID   string
}

func (d *remoteAPISecurityDetector) Name() string {
	return detectionBackendRemoteAPI
}

func (d *remoteAPISecurityDetector) Detect(ctx context.Context, req securityDetectionRequest) (*securityDetectionResponse, error) {
	if d == nil || strings.TrimSpace(d.endpoint) == "" {
		return nil, fmt.Errorf("remote detection endpoint is empty")
	}
	req.AssetName = d.asset.Name
	req.AssetID = d.asset.ID
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal remote detection request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create remote detection request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(d.apiKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(d.apiKey))
	}

	client := d.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call remote detection: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("remote detection returned status %d", resp.StatusCode)
	}

	var result securityDetectionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode remote detection response: %w", err)
	}
	return &result, nil
}

type reservedSecurityDetector struct{}

func (reservedSecurityDetector) Name() string {
	return detectionBackendReserved
}

func (reservedSecurityDetector) Detect(context.Context, securityDetectionRequest) (*securityDetectionResponse, error) {
	return nil, fmt.Errorf("security detection backend %q is reserved for future implementation", detectionBackendReserved)
}

func newSecurityDetector(runtime *ProtectionRuntimeConfig, gate *shepherd.ShepherdGate, assetName, assetID string) (securityDetector, error) {
	backend := normalizedDetectionBackend(runtimeValue(runtime, "DetectionBackend", os.Getenv("BOTSEC_DETECTION_BACKEND")))
	switch backend {
	case detectionBackendLLMAgent:
		return &llmAgentSecurityDetector{gate: gate}, nil
	case detectionBackendRemoteAPI:
		endpoint := runtimeValue(runtime, "RemoteDetectionEndpoint", os.Getenv("BOTSEC_REMOTE_DETECTION_ENDPOINT"))
		if strings.TrimSpace(endpoint) == "" {
			return nil, fmt.Errorf("remote detection endpoint is required when detection_backend=%s", detectionBackendRemoteAPI)
		}
		timeout := remoteDetectionTimeout(runtime)
		return &remoteAPISecurityDetector{
			endpoint: strings.TrimSpace(endpoint),
			apiKey:   runtimeValue(runtime, "RemoteDetectionAPIKey", os.Getenv("BOTSEC_REMOTE_DETECTION_API_KEY")),
			client:   &http.Client{Timeout: timeout},
			asset: remoteDetectionAsset{
				Name: strings.TrimSpace(assetName),
				ID:   strings.TrimSpace(assetID),
			},
		}, nil
	case detectionBackendReserved:
		return nil, fmt.Errorf("security detection backend %q is reserved for future implementation", detectionBackendReserved)
	default:
		return nil, fmt.Errorf("unsupported security detection backend: %s", backend)
	}
}

func shouldUpdateSecurityDetector(runtime *ProtectionRuntimeConfig) bool {
	if strings.TrimSpace(os.Getenv("BOTSEC_DETECTION_BACKEND")) != "" {
		return true
	}
	if runtime == nil {
		return false
	}
	return strings.TrimSpace(runtime.DetectionBackend) != "" ||
		strings.TrimSpace(runtime.RemoteDetectionEndpoint) != "" ||
		strings.TrimSpace(runtime.RemoteDetectionAPIKey) != "" ||
		runtime.RemoteDetectionTimeoutSeconds > 0
}

func normalizedDetectionBackend(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "shepherd", "shepherd_gate", "llm", "agent", detectionBackendLLMAgent:
		return detectionBackendLLMAgent
	case "remote", "api", "remote_api", "remote_interface":
		return detectionBackendRemoteAPI
	case "reserved", "placeholder", "future":
		return detectionBackendReserved
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func runtimeValue(runtime *ProtectionRuntimeConfig, field, fallback string) string {
	if runtime == nil {
		return fallback
	}
	switch field {
	case "DetectionBackend":
		if strings.TrimSpace(runtime.DetectionBackend) != "" {
			return runtime.DetectionBackend
		}
	case "RemoteDetectionEndpoint":
		if strings.TrimSpace(runtime.RemoteDetectionEndpoint) != "" {
			return runtime.RemoteDetectionEndpoint
		}
	case "RemoteDetectionAPIKey":
		if strings.TrimSpace(runtime.RemoteDetectionAPIKey) != "" {
			return runtime.RemoteDetectionAPIKey
		}
	}
	return fallback
}

func remoteDetectionTimeout(runtime *ProtectionRuntimeConfig) time.Duration {
	if runtime != nil && runtime.RemoteDetectionTimeoutSeconds > 0 {
		return time.Duration(runtime.RemoteDetectionTimeoutSeconds) * time.Second
	}
	if raw := strings.TrimSpace(os.Getenv("BOTSEC_REMOTE_DETECTION_TIMEOUT_SECONDS")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultRemoteDetectionTimeout
}

func detectionResponseFromShepherdDecision(decision *shepherd.ShepherdDecision) *securityDetectionResponse {
	if decision == nil {
		return nil
	}
	return &securityDetectionResponse{
		Allowed:    decision.Allowed,
		Status:     decision.Status,
		ActionDesc: decision.ActionDesc,
		Reason:     decision.Reason,
		RiskType:   decision.RiskType,
		Usage:      decision.Usage,
	}
}

func shepherdDecisionFromDetectionResponse(resp *securityDetectionResponse) *shepherd.ShepherdDecision {
	if resp == nil {
		return nil
	}
	status := strings.TrimSpace(resp.Status)
	if status == "" && strings.TrimSpace(resp.Action) != "" {
		status = normalizeDetectionAction(resp.Action, resp.Status)
	}
	return &shepherd.ShepherdDecision{
		Status:     status,
		Allowed:    resp.Allowed,
		Reason:     resp.Reason,
		ActionDesc: resp.ActionDesc,
		RiskType:   resp.RiskType,
		Usage:      resp.Usage,
	}
}

func securityPolicyDecisionFromDetectionResponse(resp *securityDetectionResponse, stage string) securityPolicyDecision {
	if resp == nil {
		return securityPolicyDecision{}
	}
	action := normalizeDetectionAction(resp.Action, resp.Status)
	riskType := strings.TrimSpace(resp.RiskType)
	if riskType == "" {
		riskType = riskHighRiskOperation
	}
	riskLevel := strings.TrimSpace(resp.RiskLevel)
	if riskLevel == "" {
		riskLevel = riskLevelHigh
	}
	reason := strings.TrimSpace(resp.Reason)
	if reason == "" {
		reason = "Risk detected by security detector."
	}
	actionDesc := strings.TrimSpace(resp.ActionDesc)
	if actionDesc == "" {
		actionDesc = "Risk detected by security detector"
	}
	return securityPolicyDecision{
		Status:          action,
		Action:          action,
		EventType:       resp.EventType,
		ActionDesc:      actionDesc,
		Reason:          reason,
		RiskType:        riskType,
		RiskLevel:       riskLevel,
		HookStage:       stage,
		EvidenceSummary: truncateString(resp.EvidenceSummary, 240),
		ToolCallID:      strings.TrimSpace(resp.ToolCallID),
		WasRewritten:    resp.WasRewritten || resp.Mutated,
		WasQuarantined:  resp.WasQuarantined,
	}
}

func normalizeDetectionAction(action, status string) string {
	raw := strings.ToUpper(strings.TrimSpace(action))
	if raw == "" {
		raw = strings.ToUpper(strings.TrimSpace(status))
	}
	switch raw {
	case "ALLOWED", decisionActionAllow:
		return decisionActionAllow
	case "BLOCKED", decisionActionBlock:
		return decisionActionBlock
	case "NEEDS_CONFIRM", "NEEDS_CONFIRMATION", "CONFIRM":
		return decisionActionNeedsConfirm
	case "REDACTED", decisionActionRedact:
		return decisionActionRedact
	case "REWRITTEN", decisionActionRewrite:
		return decisionActionRewrite
	default:
		return decisionActionNeedsConfirm
	}
}

func detectionResponseAllowed(resp *securityDetectionResponse) bool {
	if resp == nil {
		return true
	}
	if resp.Allowed != nil {
		return *resp.Allowed
	}
	return normalizeDetectionAction(resp.Action, resp.Status) == decisionActionAllow
}

func (pp *ProxyProtection) currentSecurityDetector() securityDetector {
	if pp == nil {
		return nil
	}
	pp.configMu.RLock()
	detector := pp.securityDetector
	pp.configMu.RUnlock()
	if detector != nil {
		return detector
	}
	if pp.shepherdGate != nil {
		return &llmAgentSecurityDetector{gate: pp.shepherdGate}
	}
	return nil
}

func (pp *ProxyProtection) isAuditOnlyMode() bool {
	if pp == nil {
		return false
	}
	pp.configMu.RLock()
	defer pp.configMu.RUnlock()
	return pp.auditOnly
}
