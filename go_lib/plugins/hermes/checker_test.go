package hermes

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go_lib/core"
)

func riskIDs(risks []core.Risk) map[string]bool {
	ids := make(map[string]bool, len(risks))
	for _, r := range risks {
		ids[r.ID] = true
	}
	return ids
}

func TestCheckPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission mode checks are skipped on windows")
	}

	tmp := t.TempDir()
	if err := os.Chmod(tmp, 0o755); err != nil {
		t.Fatalf("chmod dir failed: %v", err)
	}
	configPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(configPath, []byte("model:\n  provider: openai\n"), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	var risks []core.Risk
	checkPermissions(configPath, &risks)
	ids := riskIDs(risks)
	if !ids["config_perm_unsafe"] {
		t.Fatalf("expected config_perm_unsafe risk, got %+v", risks)
	}
	if !ids["config_dir_perm_unsafe"] {
		t.Fatalf("expected config_dir_perm_unsafe risk, got %+v", risks)
	}
}

func TestCheckTerminalBackend(t *testing.T) {
	tests := []struct {
		name     string
		backend  string
		wantRisk bool
	}{
		{name: "local", backend: "local", wantRisk: true},
		{name: "empty", backend: "", wantRisk: true},
		{name: "remote", backend: "remote", wantRisk: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &HermesConfig{}
			cfg.Terminal.Backend = tt.backend
			var risks []core.Risk
			checkTerminalBackend(cfg, &risks)
			got := len(risks) > 0
			if got != tt.wantRisk {
				t.Fatalf("risk mismatch: got=%v want=%v risks=%+v", got, tt.wantRisk, risks)
			}
		})
	}
}

func TestCheckApprovalsMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		wantRisk bool
	}{
		{name: "off", mode: "off", wantRisk: true},
		{name: "never", mode: "never", wantRisk: true},
		{name: "yolo", mode: "yolo", wantRisk: true},
		{name: "manual", mode: "manual", wantRisk: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &HermesConfig{}
			cfg.Approvals.Mode = tt.mode
			var risks []core.Risk
			checkApprovalsMode(cfg, &risks)
			got := len(risks) > 0
			if got != tt.wantRisk {
				t.Fatalf("risk mismatch: got=%v want=%v risks=%+v", got, tt.wantRisk, risks)
			}
		})
	}
}

func TestCheckRedactSecrets(t *testing.T) {
	falseValue := false

	cfg := &HermesConfig{}
	cfg.Security.RedactSecrets = &falseValue
	var risks []core.Risk
	checkRedactSecrets(cfg, nil, &risks)
	if len(risks) != 1 || risks[0].ID != "redact_secrets_disabled" {
		t.Fatalf("expected redact_secrets_disabled from struct, got %+v", risks)
	}

	cfg = &HermesConfig{}
	raw := map[string]interface{}{"security": map[string]interface{}{"redact_secrets": false}}
	risks = nil
	checkRedactSecrets(cfg, raw, &risks)
	if len(risks) != 1 || risks[0].ID != "redact_secrets_disabled" {
		t.Fatalf("expected redact_secrets_disabled from raw fallback, got %+v", risks)
	}
}

func TestCheckModelBaseURL(t *testing.T) {
	cfg := &HermesConfig{}
	cfg.Model.Provider = "custom"
	cfg.Model.BaseURL = "https://example.com/v1"
	var risks []core.Risk
	checkModelBaseURL(cfg, &risks)
	if len(risks) != 1 || risks[0].ID != "model_base_url_public" {
		t.Fatalf("expected model_base_url_public, got %+v", risks)
	}

	cfg.Model.BaseURL = "http://127.0.0.1:18080"
	risks = nil
	checkModelBaseURL(cfg, &risks)
	if len(risks) != 0 {
		t.Fatalf("expected no risk for loopback endpoint, got %+v", risks)
	}
}

func TestIsLocalURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{url: "http://localhost:8080", want: true},
		{url: "http://127.0.0.1:8080", want: true},
		{url: "http://[::1]:8080", want: true},
		{url: "https://example.com", want: false},
		{url: "not-a-url", want: false},
	}

	for _, tt := range tests {
		if got := isLocalURL(tt.url); got != tt.want {
			t.Fatalf("isLocalURL(%q)=%v want=%v", tt.url, got, tt.want)
		}
	}
}
