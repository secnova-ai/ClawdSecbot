package nullclaw

// proxy_session.go contains only Nullclaw-specific proxy session functions.
// Generic proxy session management has been moved to core/proxy/proxy_session.go.

import (
	"os"
	"path/filepath"

	"go_lib/core"
	"go_lib/core/logging"
)

// SyncGatewaySandboxInternal synchronizes gateway sandbox configuration.
// Reads the latest config from DB and restarts gateway to apply new sandbox policy.
func SyncGatewaySandboxInternal() string {
	return SyncGatewaySandboxByAssetInternal("")
}

// SyncGatewaySandboxByAssetInternal synchronizes gateway sandbox config for a specific asset instance.
func SyncGatewaySandboxByAssetInternal(assetID string) string {
	req := buildGatewayRestartRequestFromDB(assetID)
	logging.Info("[SyncGatewaySandbox] FFI called, sandbox=%v, asset=%s", req.SandboxEnabled, req.AssetName)

	result, err := restartNullclawGateway(req)
	if err != nil {
		logging.Error("[SyncGatewaySandbox] Failed: %v", err)
		return toJSONString(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
	}

	logging.Info("[SyncGatewaySandbox] Completed: %v", result)
	return toJSONString(result)
}

// HasInitialBackupInternal checks if an initial config backup exists.
func HasInitialBackupInternal() string {
	var backupDir string
	pm := core.GetPathManager()
	if pm.IsInitialized() {
		backupDir = pm.GetBackupDir()
	} else {
		homeDir, _ := os.UserHomeDir()
		backupDir = filepath.Join(homeDir, ".botsec", "backups")
	}

	exists := HasInitialBackup(backupDir)
	return toJSONString(map[string]interface{}{
		"success": true,
		"exists":  exists,
	})
}

// RestoreToInitialConfigInternal restores nullclaw.json to initial config state and restarts gateway.
func RestoreToInitialConfigInternal() string {
	logging.Info("[RestoreToInitialConfig] FFI called")

	var backupDir string
	pm := core.GetPathManager()
	if pm.IsInitialized() {
		backupDir = pm.GetBackupDir()
	} else {
		homeDir, _ := os.UserHomeDir()
		backupDir = filepath.Join(homeDir, ".botsec", "backups")
	}

	result := RestoreToInitialConfig(backupDir)
	return toJSONString(result)
}
