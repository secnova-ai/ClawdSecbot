package dwtsclaw

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go_lib/core"
)

type failingGatewayConfigPatcher struct{}

func (failingGatewayConfigPatcher) Patch(configPath string, patch map[string]interface{}) error {
	return errors.New("gateway unavailable")
}

type recordingGatewayConfigPatcher struct {
	configPath string
	patch      map[string]interface{}
}

func (p *recordingGatewayConfigPatcher) Patch(configPath string, patch map[string]interface{}) error {
	p.configPath = configPath
	p.patch = patch
	return nil
}

func TestApplyProxyConfigFallsBackToFileAndRestoresOnlyInjectedProvider(t *testing.T) {
	configPath, backupDir := writeDWTSClawTestConfig(t)
	previousConfigPath := dwtsclawConfigPathOverride
	previousPatcher := gatewayConfigPatcher
	t.Cleanup(func() {
		dwtsclawConfigPathOverride = previousConfigPath
		gatewayConfigPatcher = previousPatcher
	})
	dwtsclawConfigPathOverride = configPath
	gatewayConfigPatcher = failingGatewayConfigPatcher{}

	result, err := applyProxyConfig(&core.ProtectionContext{ProxyPort: 13436, BackupDir: backupDir})
	if err != nil {
		t.Fatalf("apply proxy config: %v", err)
	}
	if result["method"] != "direct_file_update" || result["gateway_reload"] != "pending" {
		t.Fatalf("unexpected apply result: %#v", result)
	}

	raw := readDWTSClawConfig(t, configPath)
	providers := raw["models"].(map[string]interface{})["providers"].(map[string]interface{})
	qwen := providers["qwen"].(map[string]interface{})
	if qwen["baseUrl"] != "http://127.0.0.1:13436/v1" {
		t.Fatalf("qwen base URL = %v", qwen["baseUrl"])
	}
	other := providers["other"].(map[string]interface{})
	other["baseUrl"] = "https://user-change.invalid/v1"
	if err := writeDWTSClawConfig(configPath, raw); err != nil {
		t.Fatalf("write user change: %v", err)
	}

	if err := restoreProxyConfig(&core.ProtectionContext{BackupDir: backupDir}); err != nil {
		t.Fatalf("restore proxy config: %v", err)
	}
	restored := readDWTSClawConfig(t, configPath)
	restoredProviders := restored["models"].(map[string]interface{})["providers"].(map[string]interface{})
	if got := restoredProviders["qwen"].(map[string]interface{})["baseUrl"]; got != "https://qwen.example/v1" {
		t.Fatalf("restored qwen base URL = %v", got)
	}
	if got := restoredProviders["other"].(map[string]interface{})["baseUrl"]; got != "https://user-change.invalid/v1" {
		t.Fatalf("unrelated user change was not preserved: %v", got)
	}
}

func TestRestoreProxyConfigPreservesActiveProviderChangedToAnotherLocalProxy(t *testing.T) {
	configPath, backupDir := writeDWTSClawTestConfig(t)
	previousConfigPath := dwtsclawConfigPathOverride
	previousPatcher := gatewayConfigPatcher
	t.Cleanup(func() {
		dwtsclawConfigPathOverride = previousConfigPath
		gatewayConfigPatcher = previousPatcher
	})
	dwtsclawConfigPathOverride = configPath
	gatewayConfigPatcher = failingGatewayConfigPatcher{}

	if _, err := applyProxyConfig(&core.ProtectionContext{ProxyPort: 13436, BackupDir: backupDir}); err != nil {
		t.Fatalf("apply proxy config: %v", err)
	}
	raw := readDWTSClawConfig(t, configPath)
	providers := raw["models"].(map[string]interface{})["providers"].(map[string]interface{})
	providers["qwen"].(map[string]interface{})["baseUrl"] = "http://127.0.0.1:60017/v1"
	if err := writeDWTSClawConfig(configPath, raw); err != nil {
		t.Fatalf("write user change: %v", err)
	}

	if err := restoreProxyConfig(&core.ProtectionContext{BackupDir: backupDir}); err != nil {
		t.Fatalf("restore proxy config: %v", err)
	}
	restored := readDWTSClawConfig(t, configPath)
	restoredProviders := restored["models"].(map[string]interface{})["providers"].(map[string]interface{})
	if got := restoredProviders["qwen"].(map[string]interface{})["baseUrl"]; got != "http://127.0.0.1:60017/v1" {
		t.Fatalf("user local provider change was not preserved: %v", got)
	}
}

func TestApplyProxyConfigUsesGatewayPatchBeforeFileFallback(t *testing.T) {
	configPath, backupDir := writeDWTSClawTestConfig(t)
	previousConfigPath := dwtsclawConfigPathOverride
	previousPatcher := gatewayConfigPatcher
	t.Cleanup(func() {
		dwtsclawConfigPathOverride = previousConfigPath
		gatewayConfigPatcher = previousPatcher
	})
	dwtsclawConfigPathOverride = configPath
	patcher := &recordingGatewayConfigPatcher{}
	gatewayConfigPatcher = patcher

	result, err := applyProxyConfig(&core.ProtectionContext{ProxyPort: 18081, BackupDir: backupDir})
	if err != nil {
		t.Fatalf("apply proxy config: %v", err)
	}
	if result["method"] != "gateway_config_patch" || result["gateway_reload"] != "hot" {
		t.Fatalf("unexpected apply result: %#v", result)
	}
	if patcher.configPath != configPath {
		t.Fatalf("patch config path = %q, want %q", patcher.configPath, configPath)
	}
	providers := patcher.patch["models"].(map[string]interface{})["providers"].(map[string]interface{})
	if got := providers["qwen"].(map[string]interface{})["baseUrl"]; got != "http://127.0.0.1:18081/v1" {
		t.Fatalf("gateway patch base URL = %v", got)
	}
}

func TestResolveForwardingTargetUsesInitialBackup(t *testing.T) {
	configPath, backupDir := writeDWTSClawTestConfig(t)
	previousConfigPath := dwtsclawConfigPathOverride
	previousPatcher := gatewayConfigPatcher
	t.Cleanup(func() {
		dwtsclawConfigPathOverride = previousConfigPath
		gatewayConfigPatcher = previousPatcher
	})
	dwtsclawConfigPathOverride = configPath
	gatewayConfigPatcher = failingGatewayConfigPatcher{}

	if _, err := applyProxyConfig(&core.ProtectionContext{ProxyPort: 18081, BackupDir: backupDir}); err != nil {
		t.Fatalf("apply proxy config: %v", err)
	}
	target, err := resolveProxyForwardingTarget(backupDir)
	if err != nil {
		t.Fatalf("resolve forwarding target: %v", err)
	}
	if target.Provider != "openai" || target.BaseURL != "https://qwen.example/v1" || target.APIKey != "qwen-key" {
		t.Fatalf("unexpected forwarding target: %#v", target)
	}
}

func TestResolveForwardingTargetFindsMirrorBackupAfterCoreLifecycleStart(t *testing.T) {
	configPath, coreBackupDir := writeDWTSClawTestConfig(t)
	previousConfigPath := dwtsclawConfigPathOverride
	previousPatcher := gatewayConfigPatcher
	previousHomeDir := dwtsclawUserHomeDir
	t.Cleanup(func() {
		dwtsclawConfigPathOverride = previousConfigPath
		gatewayConfigPatcher = previousPatcher
		dwtsclawUserHomeDir = previousHomeDir
	})
	dwtsclawConfigPathOverride = configPath
	homeDir := filepath.Join(t.TempDir(), "home")
	dwtsclawUserHomeDir = func() (string, error) { return homeDir, nil }
	gatewayConfigPatcher = failingGatewayConfigPatcher{}

	if _, err := applyProxyConfig(&core.ProtectionContext{ProxyPort: 18081, BackupDir: coreBackupDir}); err != nil {
		t.Fatalf("apply proxy config: %v", err)
	}
	target, err := resolveProxyForwardingTarget("")
	if err != nil {
		t.Fatalf("resolve forwarding target: %v", err)
	}
	if target.BaseURL != "https://qwen.example/v1" {
		t.Fatalf("unexpected forwarding target: %#v", target)
	}
}

func TestBackupDirectoriesKeepsLifecycleDirectoryFirst(t *testing.T) {
	previousHomeDir := dwtsclawUserHomeDir
	t.Cleanup(func() {
		dwtsclawUserHomeDir = previousHomeDir
	})
	dir := t.TempDir()
	dwtsclawUserHomeDir = func() (string, error) { return filepath.Join(dir, "home"), nil }
	explicit := filepath.Join(dir, "z-lifecycle-backups")
	dirs := backupDirectories(explicit)
	if len(dirs) != 2 {
		t.Fatalf("backup directories = %#v", dirs)
	}
	if dirs[0] != explicit {
		t.Fatalf("primary backup directory = %q, want %q", dirs[0], explicit)
	}
}

func writeDWTSClawTestConfig(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "openclaw.json")
	content := []byte(`{
  "agents": {"defaults": {"model": {"primary": "qwen/model-a"}}},
  "models": {
    "providers": {
      "qwen": {"baseUrl": "https://qwen.example/v1", "apiKey": "qwen-key", "api": "openai-completions"},
      "other": {"baseUrl": "https://other.example/v1", "apiKey": "other-key", "api": "openai-completions"}
    }
  }
}`)
	if err := os.WriteFile(configPath, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath, filepath.Join(dir, "backups")
}

func readDWTSClawConfig(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(content, &raw); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return raw
}
