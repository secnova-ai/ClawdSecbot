package hermes

import (
	"os"
	"path/filepath"
	"strings"

	"go_lib/core"
	"go_lib/core/logging"
)

// SyncGatewaySandboxByAssetInternal restarts Hermes gateway with latest config.
func SyncGatewaySandboxByAssetInternal(assetID string) string {
	req := buildGatewayRestartRequestFromDB(strings.TrimSpace(assetID))
	result, err := restartGatewayFn(req)
	if err != nil {
		logging.Warning("[Hermes] sync gateway failed: %v", err)
		return toJSONString(map[string]interface{}{"success": false, "error": err.Error()})
	}
	return toJSONString(result)
}

// HasInitialBackupInternal checks whether backup exists in backup dir.
func HasInitialBackupInternal() string {
	backupDir := ""
	if pm := core.GetPathManager(); pm.IsInitialized() {
		backupDir = pm.GetBackupDir()
	}
	if backupDir == "" {
		homeDir, _ := os.UserHomeDir()
		backupDir = filepath.Join(homeDir, ".botsec", "backups")
	}
	return toJSONString(map[string]interface{}{"success": true, "exists": hasHermesBackup(backupDir, "")})
}

// RestoreToInitialConfigInternal restores default backup target.
func RestoreToInitialConfigInternal() string {
	backupDir := ""
	if pm := core.GetPathManager(); pm.IsInitialized() {
		backupDir = pm.GetBackupDir()
	}
	if backupDir == "" {
		homeDir, _ := os.UserHomeDir()
		backupDir = filepath.Join(homeDir, ".botsec", "backups")
	}
	return toJSONString(restoreHermesConfigByAsset(backupDir, ""))
}

// RestoreToInitialConfigByAsset restores config for specific asset backup.
func RestoreToInitialConfigByAsset(backupDir, assetID string) map[string]interface{} {
	return restoreHermesConfigByAsset(backupDir, assetID)
}
