package hermes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go_lib/core"
	"go_lib/core/repository"
)

var (
	_ core.ModelConnectionCapability      = (*HermesPlugin)(nil)
	_ core.GatewaySandboxCapability       = (*HermesPlugin)(nil)
	_ core.ApplicationLifecycleCapability = (*HermesPlugin)(nil)
)

func parseJSONMap(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	payload := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("json unmarshal failed: %v raw=%s", err, raw)
	}
	return payload
}

func TestHermesPlugin_MetadataAndUISchema(t *testing.T) {
	p := GetHermesPlugin()
	if p == nil {
		t.Fatal("expected global hermes plugin")
	}
	if p.GetID() != "hermes" {
		t.Fatalf("unexpected plugin id: %s", p.GetID())
	}
	if p.GetAssetName() != hermesAssetName {
		t.Fatalf("unexpected asset name: %s", p.GetAssetName())
	}
	if !p.RequiresBotModelConfig() {
		t.Fatal("expected RequiresBotModelConfig=true")
	}

	manifest := p.GetManifest()
	if manifest.PluginID != "hermes" {
		t.Fatalf("unexpected manifest plugin id: %+v", manifest)
	}
	if manifest.BotType != "hermes" {
		t.Fatalf("unexpected manifest bot type: %+v", manifest)
	}
	if len(manifest.Capabilities) == 0 {
		t.Fatalf("expected non-empty capabilities: %+v", manifest)
	}

	schema := p.GetAssetUISchema()
	if schema == nil || schema.ID == "" {
		t.Fatalf("invalid ui schema: %+v", schema)
	}
	if len(schema.Actions) == 0 {
		t.Fatalf("expected ui actions in schema: %+v", schema)
	}
}

func TestHermesPlugin_ProtectionStatusLifecycle(t *testing.T) {
	p := &HermesPlugin{protectionStatuses: map[string]core.ProtectionStatus{}}
	assetID := "hermes:test-status"
	cfg := core.ProtectionConfig{
		ProxyEnabled:   true,
		ProxyPort:      18181,
		SandboxEnabled: true,
		AuditOnly:      true,
	}

	if err := p.StartProtection(assetID, cfg); err != nil {
		t.Fatalf("StartProtection failed: %v", err)
	}
	status := p.GetProtectionStatus(assetID)
	if !status.Running || !status.ProxyRunning {
		t.Fatalf("expected running status after start: %+v", status)
	}
	if status.ProxyPort != 18181 || !status.SandboxActive || !status.AuditOnly {
		t.Fatalf("unexpected status after start: %+v", status)
	}

	if err := p.StopProtection(assetID); err != nil {
		t.Fatalf("StopProtection failed: %v", err)
	}
	status = p.GetProtectionStatus(assetID)
	if status.Running || status.ProxyRunning {
		t.Fatalf("expected stopped status: %+v", status)
	}

	if err := p.StartProtection(assetID, cfg); err != nil {
		t.Fatalf("StartProtection failed: %v", err)
	}
	payload := parseJSONMap(t, p.OnAppExit(assetID))
	if success, _ := payload["success"].(bool); !success {
		t.Fatalf("expected OnAppExit success: %+v", payload)
	}
	status = p.GetProtectionStatus(assetID)
	if status.Running || status.ProxyRunning {
		t.Fatalf("expected OnAppExit to stop running status: %+v", status)
	}
}

func TestParseBotModelConfig(t *testing.T) {
	if got := parseBotModelConfig(nil); got != nil {
		t.Fatalf("expected nil parse result for nil config, got %+v", got)
	}

	cfg := &repository.ProtectionConfig{
		BotModelConfig: &repository.BotModelConfigData{
			Provider:  "minimax",
			BaseURL:   "https://api.minimax.io/anthropic",
			APIKey:    "sk-test",
			Model:     "MiniMax-M2.7-coding-plan",
			SecretKey: "secret-test",
		},
	}
	bot := parseBotModelConfig(cfg)
	if bot == nil {
		t.Fatal("expected non-nil bot config")
	}
	if bot.Provider != "minimax" || bot.Model != "MiniMax-M2.7-coding-plan" || bot.SecretKey != "secret-test" {
		t.Fatalf("unexpected parsed bot config: %+v", bot)
	}
}

func TestHermesPlugin_AssessRisksAndMitigate(t *testing.T) {
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, ".hermes")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	content := strings.Join([]string{
		"model:",
		"  provider: custom",
		"  default: MiniMax-M2.7-coding-plan",
		"  base_url: https://example.com/v1",
		"terminal:",
		"  backend: local",
		"approvals:",
		"  mode: off",
		"security:",
		"  redact_secrets: false",
		"",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldCfgPath := GetConfigPath()
	SetConfigPath(cfgPath)
	t.Cleanup(func() { SetConfigPath(oldCfgPath) })

	p := &HermesPlugin{protectionStatuses: map[string]core.ProtectionStatus{}}
	risks, err := p.AssessRisks(nil, nil)
	if err != nil {
		t.Fatalf("AssessRisks failed: %v", err)
	}
	ids := riskIDs(risks)
	for _, id := range []string{"terminal_backend_local", "approvals_mode_disabled", "redact_secrets_disabled", "model_base_url_public"} {
		if !ids[id] {
			t.Fatalf("expected risk %s, got %+v", id, risks)
		}
	}
	for _, risk := range risks {
		if path, ok := risk.Args["config_path"].(string); !ok || path != cfgPath {
			t.Fatalf("expected config_path in risk args: %+v", risk)
		}
		if tmpl, ok := templates[risk.ID]; ok && tmpl != nil && risk.Mitigation == nil {
			t.Fatalf("expected mitigation template for risk %s", risk.ID)
		}
	}

	result := parseJSONMap(t, p.MitigateRisk(`{"id":"unknown"}`))
	if success, _ := result["success"].(bool); success {
		t.Fatalf("expected mitigation failure for unknown risk: %+v", result)
	}
}

func TestHermesPlugin_OnProtectionStart_ErrorAndSuccess(t *testing.T) {
	p := &HermesPlugin{protectionStatuses: map[string]core.ProtectionStatus{}}
	if _, err := p.OnProtectionStart(nil); err == nil {
		t.Fatal("expected nil context error")
	}

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	if err := repository.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	t.Cleanup(func() { _ = repository.CloseDB() })

	ctxMissing := &core.ProtectionContext{AssetID: "hermes:missing", ProxyPort: 18080, BackupDir: tmp}
	if _, err := p.OnProtectionStart(ctxMissing); err == nil {
		t.Fatal("expected missing protection config error")
	}

	repo := repository.NewProtectionRepository(nil)
	assetID := "hermes:asset-start"
	if err := repo.SaveProtectionConfig(&repository.ProtectionConfig{
		AssetName: hermesAssetName,
		AssetID:   assetID,
		Enabled:   true,
		BotModelConfig: &repository.BotModelConfigData{
			Provider: "minimax",
			BaseURL:  "https://api.minimax.io/anthropic",
			APIKey:   "sk-test",
			Model:    "MiniMax-M2.7-coding-plan",
		},
	}); err != nil {
		t.Fatalf("save protection config failed: %v", err)
	}

	cfgPath := filepath.Join(tmp, "config.yaml")
	original := "model:\n  provider: openai\n  default: gpt-4.1\n"
	if err := os.WriteFile(cfgPath, []byte(original), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldCfgPath := GetConfigPath()
	SetConfigPath(cfgPath)
	t.Cleanup(func() { SetConfigPath(oldCfgPath) })

	oldRestart := restartGatewayFn
	restartCalls := 0
	restartGatewayFn = func(req *GatewayRestartRequest) (map[string]interface{}, error) {
		restartCalls++
		return map[string]interface{}{"success": true, "asset_id": req.AssetID}, nil
	}
	t.Cleanup(func() { restartGatewayFn = oldRestart })

	ctx := &core.ProtectionContext{AssetID: assetID, ProxyPort: 19090, BackupDir: tmp}
	result, err := p.OnProtectionStart(ctx)
	if err != nil {
		t.Fatalf("OnProtectionStart failed: %v", err)
	}
	if success, _ := result["success"].(bool); !success {
		t.Fatalf("expected start success result: %+v", result)
	}

	updated, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read updated config failed: %v", err)
	}
	if !strings.Contains(string(updated), "provider: custom") {
		t.Fatalf("expected config to be rewritten to proxy mode, got:\n%s", string(updated))
	}

	p.OnBeforeProxyStop(ctx)
	restored, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read restored config failed: %v", err)
	}
	if string(restored) != original {
		t.Fatalf("expected original config after OnBeforeProxyStop, got:\n%s", string(restored))
	}
	if restartCalls < 2 {
		t.Fatalf("expected restart called at least twice (start + restore), got %d", restartCalls)
	}
}

func TestHermesPlugin_RestoreBotDefaultStateAndModelConnection(t *testing.T) {
	p := &HermesPlugin{protectionStatuses: map[string]core.ProtectionStatus{}}

	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	home := filepath.Join(tmp, "home")
	if err := core.GetPathManager().ResetForTest(workspace, home); err != nil {
		t.Fatalf("path manager init failed: %v", err)
	}
	t.Cleanup(func() {
		_ = core.GetPathManager().ResetForTest("", "")
	})

	cfgPath := filepath.Join(tmp, "config.yaml")
	original := "model:\n  provider: openai\n  default: gpt-4.1\n"
	if err := os.WriteFile(cfgPath, []byte(original), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte("model:\n  provider: custom\n"), 0o600); err != nil {
		t.Fatalf("mutate config failed: %v", err)
	}

	assetID := "hermes:restore"
	backupPath, err := resolveBackupFile(core.GetPathManager().GetBackupDir(), assetID)
	if err != nil {
		t.Fatalf("resolve backup path failed: %v", err)
	}
	if err := os.WriteFile(backupPath, []byte(original), 0o600); err != nil {
		t.Fatalf("write backup failed: %v", err)
	}

	oldCfgPath := GetConfigPath()
	SetConfigPath(cfgPath)
	t.Cleanup(func() { SetConfigPath(oldCfgPath) })

	oldRestart := restartGatewayFn
	restartGatewayFn = func(req *GatewayRestartRequest) (map[string]interface{}, error) {
		return map[string]interface{}{"success": true}, nil
	}
	t.Cleanup(func() { restartGatewayFn = oldRestart })

	payload := parseJSONMap(t, p.RestoreBotDefaultState(assetID))
	if success, _ := payload["success"].(bool); !success {
		t.Fatalf("expected restore success: %+v", payload)
	}
	content, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read restored config failed: %v", err)
	}
	if string(content) != original {
		t.Fatalf("restore content mismatch: got\n%s\nwant\n%s", string(content), original)
	}

	connectionPayload := parseJSONMap(t, p.TestModelConnection("{"))
	if success, _ := connectionPayload["success"].(bool); success {
		t.Fatalf("expected invalid json model connection failure, got %+v", connectionPayload)
	}
}
