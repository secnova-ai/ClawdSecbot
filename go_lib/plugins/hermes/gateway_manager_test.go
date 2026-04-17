package hermes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go_lib/core/proxy"
)

func TestStartGatewayWithProxy_UpdatesConfigAndCreatesBackup(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	original := "model:\n  provider: openai\n  default: gpt-4.1\nterminal:\n  backend: local\n"
	if err := os.WriteFile(cfgPath, []byte(original), 0600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldCfgPath := GetConfigPath()
	SetConfigPath(cfgPath)
	t.Cleanup(func() { SetConfigPath(oldCfgPath) })

	oldRestartFn := restartGatewayFn
	restartGatewayFn = func(req *GatewayRestartRequest) (map[string]interface{}, error) {
		return map[string]interface{}{"success": true, "asset_id": req.AssetID}, nil
	}
	t.Cleanup(func() { restartGatewayFn = oldRestartFn })

	result, err := startGatewayWithProxy(18080, &proxy.BotModelConfig{Provider: "openai", Model: "gpt-4.1"}, tmp, "hermes:test")
	if err != nil {
		t.Fatalf("startGatewayWithProxy failed: %v", err)
	}
	if success, _ := result["success"].(bool); !success {
		t.Fatalf("expected success result, got: %+v", result)
	}

	updated, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read updated config failed: %v", err)
	}
	updatedStr := string(updated)
	if !strings.Contains(updatedStr, "provider: custom") {
		t.Fatalf("expected provider custom, got:\n%s", updatedStr)
	}
	if !strings.Contains(updatedStr, "base_url: http://127.0.0.1:18080") {
		t.Fatalf("expected proxy base_url, got:\n%s", updatedStr)
	}
	if !strings.Contains(updatedStr, "api_key: botsec-proxy-key") {
		t.Fatalf("expected proxy api_key placeholder, got:\n%s", updatedStr)
	}

	backupPath, err := resolveBackupFile(tmp, "hermes:test")
	if err != nil {
		t.Fatalf("resolveBackupFile failed: %v", err)
	}
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup failed: %v", err)
	}
	if string(backupData) != original {
		t.Fatalf("backup content mismatch")
	}
}

func TestRestoreHermesConfigByAsset_RestoresOriginalContent(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	original := "model:\n  provider: openai\n  default: gpt-4.1\n"
	if err := os.WriteFile(cfgPath, []byte(original), 0600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldCfgPath := GetConfigPath()
	SetConfigPath(cfgPath)
	t.Cleanup(func() { SetConfigPath(oldCfgPath) })

	oldRestartFn := restartGatewayFn
	restartGatewayFn = func(req *GatewayRestartRequest) (map[string]interface{}, error) {
		return map[string]interface{}{"success": true}, nil
	}
	t.Cleanup(func() { restartGatewayFn = oldRestartFn })

	if _, err := startGatewayWithProxy(18081, &proxy.BotModelConfig{Provider: "openai", Model: "gpt-4.1"}, tmp, "hermes:test-restore"); err != nil {
		t.Fatalf("startGatewayWithProxy failed: %v", err)
	}

	if err := os.WriteFile(cfgPath, []byte("model:\n  provider: custom\n  default: mutated\n"), 0600); err != nil {
		t.Fatalf("mutate config failed: %v", err)
	}

	result := restoreHermesConfigByAsset(tmp, "hermes:test-restore")
	if success, _ := result["success"].(bool); !success {
		t.Fatalf("restore failed: %+v", result)
	}
	content, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	if string(content) != original {
		t.Fatalf("restore mismatch: got\n%s\nwant\n%s", string(content), original)
	}
}
