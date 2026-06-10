package readyclaw

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go_lib/core"
)

func TestFindConfigPathPrefersProgramDataRuntimeConfig(t *testing.T) {
	prevProgramData := readyclawProgramDataDir
	prevHomeDir := readyclawUserHomeDir
	prevConfigOverride := readyclawConfigPathOverride
	t.Cleanup(func() {
		readyclawProgramDataDir = prevProgramData
		readyclawUserHomeDir = prevHomeDir
		readyclawConfigPathOverride = prevConfigOverride
	})

	dir := t.TempDir()
	programData := filepath.Join(dir, "ProgramData")
	homeDir := filepath.Join(dir, "Users", "tester")
	programDataConfig := filepath.Join(programData, "NanoClaw", "config", "nanoclaw", "config.json")
	userConfig := filepath.Join(homeDir, ".config", "nanoclaw", "config.json")

	writeReadyClawTestConfig(t, programDataConfig, "https://program-data.example.com/v1/chat/completions")
	writeReadyClawTestConfig(t, userConfig, "https://home.example.com/v1/chat/completions")

	readyclawProgramDataDir = func() string { return programData }
	readyclawUserHomeDir = func() (string, error) { return homeDir, nil }
	readyclawConfigPathOverride = ""

	got, err := findConfigPath()
	if err != nil {
		t.Fatalf("findConfigPath returned error: %v", err)
	}
	if got != programDataConfig {
		t.Fatalf("expected ProgramData config %q, got %q", programDataConfig, got)
	}
}

func TestApplyProxyConfigUpdatesBaseURLAndPreservesValues(t *testing.T) {
	prevConfigOverride := readyclawConfigPathOverride
	t.Cleanup(func() { readyclawConfigPathOverride = prevConfigOverride })

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	backupDir := filepath.Join(dir, "backup")
	readyclawConfigPathOverride = configPath

	writeReadyClawFullConfig(t, configPath, map[string]interface{}{
		"API_TOKEN":      "ready-token",
		"LLM_BASE_URL":   "https://api.example.com/v1/chat/completions",
		"LLM_API_KEY":    "sk-real",
		"LLM_MODEL_NAME": "gpt-5.4",
		"IMAGE_API_KEY":  "sk-image",
		"IMAGE_BASE_URL": "https://image.example.com/v1",
		"IMAGE_MODEL":    "dall-e-3",
	})

	result, err := applyReadyClawProxyConfig(&core.ProtectionContext{
		AssetID:   "readyclaw:test",
		ProxyPort: 18081,
		BackupDir: backupDir,
	})
	if err != nil {
		t.Fatalf("applyReadyClawProxyConfig returned error: %v", err)
	}
	if result["proxy_url"] != "http://127.0.0.1:18081/v1/chat/completions" {
		t.Fatalf("unexpected proxy_url: %#v", result["proxy_url"])
	}
	if result["original_base_url"] != "https://api.example.com/v1/chat/completions" {
		t.Fatalf("unexpected original_base_url: %#v", result["original_base_url"])
	}

	raw := readReadyClawRawConfig(t, configPath)
	values := raw["values"].(map[string]interface{})
	if values["LLM_BASE_URL"] != "http://127.0.0.1:18081/v1/chat/completions" {
		t.Fatalf("expected hijacked LLM_BASE_URL, got %#v", values["LLM_BASE_URL"])
	}
	if values["LLM_API_KEY"] != "sk-real" || values["LLM_MODEL_NAME"] != "gpt-5.4" {
		t.Fatalf("expected existing LLM credentials/model to be preserved, got %#v", values)
	}
	if values["IMAGE_API_KEY"] != "sk-image" || values["IMAGE_BASE_URL"] != "https://image.example.com/v1" {
		t.Fatalf("expected image settings to be preserved, got %#v", values)
	}
	if _, exists := values["LLM_PROTOCOL"]; exists {
		t.Fatalf("LLM_PROTOCOL should not be added when ReadyClaw omits it")
	}
	if _, err := os.Stat(filepath.Join(backupDir, "readyclaw-config.initial.json")); err != nil {
		t.Fatalf("expected initial backup to be created: %v", err)
	}
}

func TestResolveForwardingTargetUsesInitialBackupWhenCurrentConfigIsHijacked(t *testing.T) {
	prevConfigOverride := readyclawConfigPathOverride
	t.Cleanup(func() { readyclawConfigPathOverride = prevConfigOverride })

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	backupDir := filepath.Join(dir, "backup")
	readyclawConfigPathOverride = configPath

	writeReadyClawTestConfig(t, configPath, "https://api.example.com/v1/chat/completions")
	if _, err := applyReadyClawProxyConfig(&core.ProtectionContext{ProxyPort: 18081, BackupDir: backupDir}); err != nil {
		t.Fatalf("applyReadyClawProxyConfig returned error: %v", err)
	}

	target, err := resolveReadyClawForwardingTarget(backupDir)
	if err != nil {
		t.Fatalf("resolveReadyClawForwardingTarget returned error: %v", err)
	}
	if target.Provider != "openai" {
		t.Fatalf("expected provider openai, got %q", target.Provider)
	}
	if target.BaseURL != "https://api.example.com/v1/chat/completions" {
		t.Fatalf("expected original base URL from backup, got %q", target.BaseURL)
	}
	if target.APIKey != "" {
		t.Fatalf("expected APIKey to stay empty for request-header forwarding, got %q", target.APIKey)
	}
}

func TestRestoreReadyClawConfigRestoresInitialBackup(t *testing.T) {
	prevConfigOverride := readyclawConfigPathOverride
	t.Cleanup(func() { readyclawConfigPathOverride = prevConfigOverride })

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	backupDir := filepath.Join(dir, "backup")
	readyclawConfigPathOverride = configPath

	writeReadyClawTestConfig(t, configPath, "https://api.example.com/v1/chat/completions")
	if _, err := applyReadyClawProxyConfig(&core.ProtectionContext{ProxyPort: 18081, BackupDir: backupDir}); err != nil {
		t.Fatalf("applyReadyClawProxyConfig returned error: %v", err)
	}
	if err := restoreReadyClawConfig(backupDir); err != nil {
		t.Fatalf("restoreReadyClawConfig returned error: %v", err)
	}

	raw := readReadyClawRawConfig(t, configPath)
	values := raw["values"].(map[string]interface{})
	if values["LLM_BASE_URL"] != "https://api.example.com/v1/chat/completions" {
		t.Fatalf("expected restored LLM_BASE_URL, got %#v", values["LLM_BASE_URL"])
	}
}

func TestPluginUsesReadyClawIdentityAndNoBotModelRequirement(t *testing.T) {
	p := &Plugin{}
	if p.GetID() != "readyclaw" {
		t.Fatalf("expected plugin ID readyclaw, got %q", p.GetID())
	}
	if p.GetAssetName() != "ReadyClaw" {
		t.Fatalf("expected asset name ReadyClaw, got %q", p.GetAssetName())
	}
	if p.RequiresBotModelConfig() {
		t.Fatalf("ReadyClaw should resolve forwarding target from runtime config")
	}
	if p.GetManifest().DisplayName != "ReadyClaw" {
		t.Fatalf("unexpected manifest display name: %q", p.GetManifest().DisplayName)
	}
}

func writeReadyClawTestConfig(t *testing.T, path string, baseURL string) {
	t.Helper()
	writeReadyClawFullConfig(t, path, map[string]interface{}{
		"LLM_BASE_URL":   baseURL,
		"LLM_API_KEY":    "sk-real",
		"LLM_MODEL_NAME": "gpt-5.4",
	})
}

func writeReadyClawFullConfig(t *testing.T, path string, values map[string]interface{}) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	payload := map[string]interface{}{
		"configVersion": 1,
		"values":        values,
	}
	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, append(content, '\n'), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func readReadyClawRawConfig(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(content, &raw); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	return raw
}
