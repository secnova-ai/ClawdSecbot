package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteSecurityDetectorSendsStagePayloadAndParsesDecision(t *testing.T) {
	var got securityDetectionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("expected bearer token, got %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"allowed":false,"action":"BLOCK","reason":"remote blocked dangerous tool","risk_type":"HIGH_RISK_OPERATION","risk_level":"high"}`))
	}))
	defer server.Close()

	detector, err := newSecurityDetector(&ProtectionRuntimeConfig{
		DetectionBackend:        "remote_api",
		RemoteDetectionEndpoint: server.URL,
		RemoteDetectionAPIKey:   "test-key",
	}, nil, "openclaw", "openclaw:test")
	if err != nil {
		t.Fatalf("newSecurityDetector returned error: %v", err)
	}

	resp, err := detector.Detect(t.Context(), securityDetectionRequest{
		Stage:     hookStageToolCall,
		RequestID: "req-remote",
		ToolCalls: []ToolCallInfo{
			{Name: "delete_file", RawArgs: `{"path":"/tmp/demo"}`, ToolCallID: "call_1"},
		},
	})
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if resp == nil || detectionResponseAllowed(resp) {
		t.Fatalf("expected blocking response, got %+v", resp)
	}
	if got.Stage != hookStageToolCall || got.AssetID != "openclaw:test" || len(got.ToolCalls) != 1 {
		t.Fatalf("unexpected remote payload: %+v", got)
	}
}

func TestFinalResultRemoteDetectorCanRedactOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got securityDetectionRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got.Stage != hookStageFinalResult {
			t.Fatalf("expected final_result stage, got %s", got.Stage)
		}
		if !strings.Contains(got.FinalContent, "secret") {
			t.Fatalf("expected final content in request, got %+v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"allowed":false,"action":"REDACT","content":"safe redacted output","reason":"remote redacted secret","risk_type":"SENSITIVE_DATA_EXFILTRATION","risk_level":"high","mutated":true}`))
	}))
	defer server.Close()

	pp := &ProxyProtection{
		assetName: "openclaw",
		assetID:   "openclaw:test",
		securityDetector: &remoteAPISecurityDetector{
			endpoint: server.URL,
			client:   server.Client(),
			asset: remoteDetectionAsset{
				Name: "openclaw",
				ID:   "openclaw:test",
			},
		},
	}

	result := pp.runFinalResultPolicyHooks(t.Context(), finalResultPolicyContext{
		RequestID: "req-final",
		Content:   "the secret is token-123",
	})
	if !result.Handled || !result.Pass || !result.Mutated {
		t.Fatalf("expected handled redaction, got %+v", result)
	}
	if result.Content != "safe redacted output" {
		t.Fatalf("expected remote redacted content, got %q", result.Content)
	}
	if result.Decision == nil || result.Decision.Action != decisionActionRedact {
		t.Fatalf("expected redact decision, got %+v", result.Decision)
	}
}

func TestReservedSecurityDetectorIsExplicitPlaceholder(t *testing.T) {
	_, err := newSecurityDetector(&ProtectionRuntimeConfig{
		DetectionBackend: "reserved",
	}, nil, "openclaw", "openclaw:test")
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("expected explicit placeholder construction error, got %v", err)
	}
}

func TestRemoteSecurityDetectorDoesNotRequireLocalSecurityModel(t *testing.T) {
	pp, err := NewProxyProtectionFromConfig(&ProtectionConfig{
		AssetName: "openclaw",
		AssetID:   "openclaw:remote-detector",
		BotModel: &BotModelConfig{
			Provider: "openai",
			BaseURL:  "http://127.0.0.1:19090",
			APIKey:   "bot-key",
			Model:    "bot-model",
		},
		Runtime: &ProtectionRuntimeConfig{
			DetectionBackend:        "remote_api",
			RemoteDetectionEndpoint: "http://127.0.0.1:19091/detect",
		},
	}, make(chan string, 10))
	if err != nil {
		t.Fatalf("remote detector should not require local security model: %v", err)
	}
	if pp == nil || pp.shepherdGate == nil {
		t.Fatalf("expected proxy and model-free shepherd gate")
	}
	if pp.currentSecurityDetector().Name() != detectionBackendRemoteAPI {
		t.Fatalf("expected remote detector, got %s", pp.currentSecurityDetector().Name())
	}
}
