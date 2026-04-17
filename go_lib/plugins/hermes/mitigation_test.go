package hermes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func buildMitigationRequest(
	t *testing.T,
	riskID string,
	args map[string]interface{},
	formData map[string]interface{},
) string {
	t.Helper()
	req := map[string]interface{}{
		"id":        riskID,
		"args":      args,
		"form_data": formData,
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to build mitigation request: %v", err)
	}
	return string(raw)
}

func parseMitigationPayload(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	payload := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("failed to parse mitigation payload: %v raw=%s", err, raw)
	}
	return payload
}

func TestMitigateRiskDispatch_InvalidAndUnknown(t *testing.T) {
	payload := parseMitigationPayload(t, MitigateRiskDispatch("{"))
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("expected invalid json failure: %+v", payload)
	}

	payload = parseMitigationPayload(t, MitigateRiskDispatch(`{"id":"unknown_risk"}`))
	if success, _ := payload["success"].(bool); success {
		t.Fatalf("expected not implemented failure: %+v", payload)
	}
}

func TestConfigPermMitigation_ApplyAndSkip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod mode assertion is not stable on windows")
	}

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("model:\n  provider: openai\n"), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	skipReq := buildMitigationRequest(
		t,
		"config_perm_unsafe",
		map[string]interface{}{"path": cfgPath},
		map[string]interface{}{"fix_permission": false},
	)
	skipPayload := parseMitigationPayload(t, MitigateRiskDispatch(skipReq))
	if success, _ := skipPayload["success"].(bool); !success {
		t.Fatalf("expected skip to succeed: %+v", skipPayload)
	}

	applyReq := buildMitigationRequest(
		t,
		"config_perm_unsafe",
		map[string]interface{}{"path": cfgPath},
		map[string]interface{}{"fix_permission": true},
	)
	applyPayload := parseMitigationPayload(t, MitigateRiskDispatch(applyReq))
	if success, _ := applyPayload["success"].(bool); !success {
		t.Fatalf("expected apply to succeed: %+v", applyPayload)
	}

	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("expected 0600 after mitigation, got %o", perm)
	}
}

func TestConfigDirPermMitigation_Apply(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod mode assertion is not stable on windows")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}

	req := buildMitigationRequest(
		t,
		"config_dir_perm_unsafe",
		map[string]interface{}{"path": dir},
		map[string]interface{}{"fix_permission": true},
	)
	payload := parseMitigationPayload(t, MitigateRiskDispatch(req))
	if success, _ := payload["success"].(bool); !success {
		t.Fatalf("expected apply to succeed: %+v", payload)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("expected 0700 after mitigation, got %o", perm)
	}
}

func TestRedactSecretsMitigation_EnableAndSkip(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	initial := "model:\n  provider: openai\nsecurity:\n  redact_secrets: false\n"
	if err := os.WriteFile(cfgPath, []byte(initial), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	skipReq := buildMitigationRequest(
		t,
		"redact_secrets_disabled",
		map[string]interface{}{"config_path": cfgPath},
		map[string]interface{}{"enable_redaction": false},
	)
	skipPayload := parseMitigationPayload(t, MitigateRiskDispatch(skipReq))
	if success, _ := skipPayload["success"].(bool); !success {
		t.Fatalf("expected skip to succeed: %+v", skipPayload)
	}

	applyReq := buildMitigationRequest(
		t,
		"redact_secrets_disabled",
		map[string]interface{}{"config_path": cfgPath},
		map[string]interface{}{"enable_redaction": true},
	)
	applyPayload := parseMitigationPayload(t, MitigateRiskDispatch(applyReq))
	if success, _ := applyPayload["success"].(bool); !success {
		t.Fatalf("expected enable to succeed: %+v", applyPayload)
	}

	_, raw, err := loadConfig(cfgPath)
	if err != nil {
		t.Fatalf("reload config failed: %v", err)
	}
	if value, ok := getNestedBool(raw, "security", "redact_secrets"); !ok || !value {
		t.Fatalf("expected redact_secrets=true, got value=%v ok=%v raw=%+v", value, ok, raw)
	}
}

func TestApprovalsModeMitigation_ValidateAndApply(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	initial := "approvals:\n  mode: off\n"
	if err := os.WriteFile(cfgPath, []byte(initial), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	invalidReq := buildMitigationRequest(
		t,
		"approvals_mode_disabled",
		map[string]interface{}{"config_path": cfgPath},
		map[string]interface{}{"mode": "invalid"},
	)
	invalidPayload := parseMitigationPayload(t, MitigateRiskDispatch(invalidReq))
	if success, _ := invalidPayload["success"].(bool); success {
		t.Fatalf("expected invalid mode failure: %+v", invalidPayload)
	}
	if errMsg, _ := invalidPayload["error"].(string); !strings.Contains(errMsg, "invalid approvals mode") {
		t.Fatalf("unexpected invalid mode error: %+v", invalidPayload)
	}

	applyReq := buildMitigationRequest(
		t,
		"approvals_mode_disabled",
		map[string]interface{}{"config_path": cfgPath},
		map[string]interface{}{"mode": "smart"},
	)
	applyPayload := parseMitigationPayload(t, MitigateRiskDispatch(applyReq))
	if success, _ := applyPayload["success"].(bool); !success {
		t.Fatalf("expected smart mode apply success: %+v", applyPayload)
	}

	cfg, _, err := loadConfig(cfgPath)
	if err != nil {
		t.Fatalf("reload config failed: %v", err)
	}
	if cfg.Approvals.Mode != "smart" {
		t.Fatalf("expected approvals.mode=smart, got %q", cfg.Approvals.Mode)
	}

	defaultReq := buildMitigationRequest(
		t,
		"approvals_mode_disabled",
		map[string]interface{}{"config_path": cfgPath},
		map[string]interface{}{},
	)
	defaultPayload := parseMitigationPayload(t, MitigateRiskDispatch(defaultReq))
	if success, _ := defaultPayload["success"].(bool); !success {
		t.Fatalf("expected default mode apply success: %+v", defaultPayload)
	}
	cfg, _, err = loadConfig(cfgPath)
	if err != nil {
		t.Fatalf("reload config failed: %v", err)
	}
	if cfg.Approvals.Mode != "manual" {
		t.Fatalf("expected approvals.mode=manual by default, got %q", cfg.Approvals.Mode)
	}
}
