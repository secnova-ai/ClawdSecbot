package hermes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go_lib/core"
)

func parseJSONStringMap(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	payload := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("json unmarshal failed: %v raw=%s", err, raw)
	}
	return payload
}

func TestSyncGatewaySandboxByAssetInternal(t *testing.T) {
	oldRestart := restartGatewayFn
	restartGatewayFn = func(req *GatewayRestartRequest) (map[string]interface{}, error) {
		if req.AssetID == "hermes:error" {
			return nil, os.ErrPermission
		}
		return map[string]interface{}{"success": true, "asset_id": req.AssetID}, nil
	}
	t.Cleanup(func() { restartGatewayFn = oldRestart })

	successPayload := parseJSONStringMap(t, SyncGatewaySandboxByAssetInternal("hermes:ok"))
	if success, _ := successPayload["success"].(bool); !success {
		t.Fatalf("expected sync success payload: %+v", successPayload)
	}

	errorPayload := parseJSONStringMap(t, SyncGatewaySandboxByAssetInternal("hermes:error"))
	if success, _ := errorPayload["success"].(bool); success {
		t.Fatalf("expected sync failure payload: %+v", errorPayload)
	}
}

func TestProxySessionBackupHelpers(t *testing.T) {
	tmp := t.TempDir()
	workspace := filepath.Join(tmp, "workspace")
	home := filepath.Join(tmp, "home")
	if err := core.GetPathManager().ResetForTest(workspace, home); err != nil {
		t.Fatalf("path manager init failed: %v", err)
	}
	t.Cleanup(func() {
		_ = core.GetPathManager().ResetForTest("", "")
	})

	configPath := filepath.Join(tmp, "config.yaml")
	original := "model:\n  provider: openai\n  default: gpt-4.1\n"
	if err := os.WriteFile(configPath, []byte(original), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("model:\n  provider: custom\n"), 0o600); err != nil {
		t.Fatalf("mutate config failed: %v", err)
	}

	oldCfgPath := GetConfigPath()
	SetConfigPath(configPath)
	t.Cleanup(func() { SetConfigPath(oldCfgPath) })

	backupDir := core.GetPathManager().GetBackupDir()
	backupPath, err := resolveBackupFile(backupDir, "")
	if err != nil {
		t.Fatalf("resolve backup path failed: %v", err)
	}
	if err := os.WriteFile(backupPath, []byte(original), 0o600); err != nil {
		t.Fatalf("write backup failed: %v", err)
	}

	hasPayload := parseJSONStringMap(t, HasInitialBackupInternal())
	if success, _ := hasPayload["success"].(bool); !success {
		t.Fatalf("expected has-backup success payload: %+v", hasPayload)
	}
	if exists, _ := hasPayload["exists"].(bool); !exists {
		t.Fatalf("expected backup to exist: %+v", hasPayload)
	}

	oldRestart := restartGatewayFn
	restartGatewayFn = func(req *GatewayRestartRequest) (map[string]interface{}, error) {
		return map[string]interface{}{"success": true}, nil
	}
	t.Cleanup(func() { restartGatewayFn = oldRestart })

	restorePayload := parseJSONStringMap(t, RestoreToInitialConfigInternal())
	if success, _ := restorePayload["success"].(bool); !success {
		t.Fatalf("expected restore success payload: %+v", restorePayload)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	if string(content) != original {
		t.Fatalf("config restore mismatch: got\n%s\nwant\n%s", string(content), original)
	}

	byAsset := RestoreToInitialConfigByAsset(backupDir, "")
	if success, _ := byAsset["success"].(bool); !success {
		t.Fatalf("expected RestoreToInitialConfigByAsset success: %+v", byAsset)
	}
}
