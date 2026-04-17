package core

import (
	"encoding/json"

	"go_lib/core/logging"
)

// ========== FFI helpers used by main ==========

// MarshalJSON serializes a Go value to JSON.
func MarshalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"success":false,"error":"marshal error"}`
	}
	return string(b)
}

// SuccessResult builds a successful FFI response.
func SuccessResult(data interface{}) map[string]interface{} {
	return map[string]interface{}{
		"success": true,
		"data":    data,
	}
}

// ErrorResult builds an error FFI response.
func ErrorResult(err error) map[string]interface{} {
	return map[string]interface{}{
		"success": false,
		"error":   err.Error(),
	}
}

// ========== Global initialization ==========

// Initialize configures the shared path manager.
// workspaceDir is the app data base directory provided by Flutter.
// homeDir is the user home directory for discovery and policy paths.
func Initialize(workspaceDir, homeDir, sandboxDir string) (map[string]interface{}, error) {
	logging.Info(
		"Initializing global path manager: workspaceDir=%s, homeDir=%s, sandboxDir=%s",
		workspaceDir,
		homeDir,
		sandboxDir,
	)

	pm := GetPathManager()
	if err := pm.InitializeWithSandbox(workspaceDir, homeDir, sandboxDir); err != nil {
		logging.Error("Failed to initialize path manager: %v", err)
		return nil, err
	}

	return map[string]interface{}{
		"success":           true,
		"workspace_dir":     pm.GetWorkspaceDir(),
		"home_dir":          pm.GetHomeDir(),
		"sandbox_dir":       pm.GetSandboxDir(),
		"log_dir":           pm.GetLogDir(),
		"sandbox_log_dir":   pm.GetSandboxLogDir(),
		"backup_dir":        pm.GetBackupDir(),
		"policy_dir":        pm.GetPolicyDir(),
		"react_skill_dir":   pm.GetReActSkillDir(),
		"scan_skill_dir":    pm.GetScanSkillDir(),
		"db_path":           pm.GetDBPath(),
		"version_file_path": pm.GetVersionFilePath(),
	}, nil
}

// InitLogging initializes Go loggers.
// When logDir is empty, the path is derived from PathManager.
func InitLogging(logDir string) (map[string]interface{}, error) {
	if logDir == "" {
		pm := GetPathManager()
		if !pm.IsInitialized() {
			return map[string]interface{}{
				"success": false,
				"error":   "logDir is empty and PathManager not initialized",
			}, nil
		}
		logDir = pm.GetLogDir()
	}

	// Initialize the main logger.
	if err := logging.InitLogger(logDir, logging.INFO); err != nil {
		return nil, err
	}

	// Initialize the history logger.
	if err := logging.InitHistoryLogger(logDir, logging.INFO); err != nil {
		return nil, err
	}

	// Initialize the ShepherdGate logger.
	if err := logging.InitShepherdGateLogger(logDir, logging.INFO); err != nil {
		return nil, err
	}

	logging.Info("Logging system initialized, log dir: %s", logDir)

	return map[string]interface{}{
		"success": true,
		"log_dir": logDir,
	}, nil
}

// GetPaths returns all derived path information.
func GetPaths() map[string]interface{} {
	pm := GetPathManager()
	if !pm.IsInitialized() {
		return map[string]interface{}{
			"success": false,
			"error":   "PathManager not initialized",
		}
	}

	return map[string]interface{}{
		"success":           true,
		"workspace_dir":     pm.GetWorkspaceDir(),
		"home_dir":          pm.GetHomeDir(),
		"sandbox_dir":       pm.GetSandboxDir(),
		"log_dir":           pm.GetLogDir(),
		"sandbox_log_dir":   pm.GetSandboxLogDir(),
		"backup_dir":        pm.GetBackupDir(),
		"policy_dir":        pm.GetPolicyDir(),
		"react_skill_dir":   pm.GetReActSkillDir(),
		"scan_skill_dir":    pm.GetScanSkillDir(),
		"db_path":           pm.GetDBPath(),
		"version_file_path": pm.GetVersionFilePath(),
	}
}

// ========== 插件管理函数 ==========

// GetRegisteredPlugins 获取所有已注册的插件信息
func GetRegisteredPlugins() map[string]interface{} {
	pm := GetPluginManager()
	infos := pm.GetAllPluginInfos()

	return map[string]interface{}{
		"success": true,
		"data":    infos,
		"count":   len(infos),
	}
}

// ========== 资产扫描函数 ==========

// ScanAllAssets 使用所有插件扫描资产
func ScanAllAssets() (map[string]interface{}, error) {
	logging.Info("Core: Scanning all assets")

	pm := GetPluginManager()
	assets, err := pm.ScanAllAssets()
	if err != nil {
		logging.Error("Core: Scan all assets failed: %v", err)
		return nil, err
	}

	logging.Info("Core: Scan completed, found %d assets", len(assets))
	return map[string]interface{}{
		"success": true,
		"data":    assets,
		"count":   len(assets),
	}, nil
}

// AssessAllRisks 使用所有插件评估风险
func AssessAllRisks(scannedHashes map[string]bool) (map[string]interface{}, error) {
	logging.Info("Core: Assessing all risks")

	pm := GetPluginManager()
	risks, err := pm.AssessAllRisks(scannedHashes)
	if err != nil {
		logging.Error("Core: Assess all risks failed: %v", err)
		return nil, err
	}

	logging.Info("Core: Risk assessment completed, found %d risks", len(risks))
	return map[string]interface{}{
		"success": true,
		"data":    risks,
		"count":   len(risks),
	}, nil
}

// AssessAllRisksFromString 从 JSON 字符串解析 scannedHashes 并评估风险
// JSON 格式: ["hash1", "hash2", ...]
func AssessAllRisksFromString(scannedHashesJSON string) (map[string]interface{}, error) {
	var hashList []string
	if err := json.Unmarshal([]byte(scannedHashesJSON), &hashList); err != nil {
		hashList = nil
	}

	hashSet := make(map[string]bool)
	for _, h := range hashList {
		hashSet[h] = true
	}

	return AssessAllRisks(hashSet)
}

// ========== 风险缓解路由函数 ==========

// MitigateRiskByPlugin routes a mitigation request to the correct plugin via PluginManager.
// riskInfoJSON must contain a "source_plugin" field to identify the target plugin.
func MitigateRiskByPlugin(riskInfoJSON string) string {
	pm := GetPluginManager()
	return pm.MitigateRisk(riskInfoJSON)
}

// ========== 防护控制函数 ==========

// StartProtectionByAsset starts protection for the specified asset instance.
// assetID: deterministic instance ID from ComputeAssetID, identifies the specific instance
// configJSON: JSON protection config string
func StartProtectionByAsset(assetID string, configJSON string) error {
	var config ProtectionConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return err
	}

	logging.Info("Core: Starting protection for asset: id=%s", assetID)

	pm := GetPluginManager()
	if err := pm.StartProtection(assetID, config); err != nil {
		logging.Error("Core: Start protection failed: %v", err)
		return err
	}

	logging.Info("Core: Protection started for asset: id=%s", assetID)
	return nil
}

// StopProtectionByAsset stops protection for the specified asset instance.
func StopProtectionByAsset(assetID string) error {
	logging.Info("Core: Stopping protection for asset: id=%s", assetID)

	pm := GetPluginManager()
	if err := pm.StopProtection(assetID); err != nil {
		logging.Error("Core: Stop protection failed: %v", err)
		return err
	}

	logging.Info("Core: Protection stopped for asset: id=%s", assetID)
	return nil
}

// GetProtectionStatusByAsset returns the protection status for the specified asset instance.
func GetProtectionStatusByAsset(assetID string) (ProtectionStatus, error) {
	pm := GetPluginManager()
	return pm.GetProtectionStatus(assetID)
}

// GetAllProtectionStatuses 获取所有资产的防护状态
func GetAllProtectionStatuses() map[string]ProtectionStatus {
	pm := GetPluginManager()
	return pm.GetAllProtectionStatus()
}
