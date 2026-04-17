package dintalclaw

// proxy_session.go contains only Dintalclaw-specific proxy session functions.

import (
	"os"

	"go_lib/core"
	"go_lib/core/logging"
)

// SyncGatewaySandboxInternal synchronizes gateway sandbox configuration.
func SyncGatewaySandboxInternal() string {
	return SyncGatewaySandboxByAssetInternal("")
}

// SyncGatewaySandboxByAssetInternal synchronizes gateway sandbox config for a specific asset instance.
func SyncGatewaySandboxByAssetInternal(assetID string) string {
	req := buildGatewayRestartRequestFromDB(assetID)
	logging.Info("[SyncGatewaySandbox] FFI called, sandbox=%v, asset=%s", req.SandboxEnabled, req.AssetName)

	result, err := restartDintalclawProcess(req)
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
		backupDir = core.ResolveBackupDir(homeDir)
	}

	exists := HasInitialBackup(backupDir)
	return toJSONString(map[string]interface{}{
		"success": true,
		"exists":  exists,
	})
}

// RestoreToInitialConfigInternal restores mykey.py to initial config state and restarts process.
func RestoreToInitialConfigInternal() string {
	logging.Info("[RestoreToInitialConfig] FFI called")

	var backupDir string
	pm := core.GetPathManager()
	if pm.IsInitialized() {
		backupDir = pm.GetBackupDir()
	} else {
		homeDir, _ := os.UserHomeDir()
		backupDir = core.ResolveBackupDir(homeDir)
	}

	result := RestoreToInitialConfig(backupDir)
	return toJSONString(result)
}
