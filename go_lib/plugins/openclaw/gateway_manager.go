package openclaw

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/repository"
	"go_lib/core/sandbox"
)

// gatewayRestartMu 保证同一时刻只有一个网关重启流程在执行，
// 防止多路径并发调用（如 ApplyOpenclawConfig + SyncOpenclawGateway）导致冲突。
var gatewayRestartMu sync.Mutex

// GatewayRestartRequest 网关重启/同步请求
type GatewayRestartRequest struct {
	AssetName         string                          `json:"asset_name"`
	AssetID           string                          `json:"asset_id,omitempty"`
	SandboxEnabled    bool                            `json:"sandbox_enabled"`
	PathPermission    sandbox.PathPermissionConfig    `json:"path_permission"`
	NetworkPermission sandbox.NetworkPermissionConfig `json:"network_permission"`
	ShellPermission   sandbox.ShellPermissionConfig   `json:"shell_permission"`
	PolicyDir         string                          `json:"policy_dir"`
}

// buildGatewayRestartRequestFromDB 从数据库读取防护配置，构造 GatewayRestartRequest。
// 确保所有重启场景（编辑配置、启动恢复、更新 bot 模型）都使用数据库中最新的沙箱设置。
func buildGatewayRestartRequestFromDB(assetID string) *GatewayRestartRequest {
	req := &GatewayRestartRequest{
		AssetName: openclawAssetName,
		AssetID:   strings.TrimSpace(assetID),
	}

	repo := repository.NewProtectionRepository(nil)
	config, err := repo.GetProtectionConfig(openclawAssetName, req.AssetID)
	if err != nil {
		logging.Warning("[GatewayManager] Failed to read protection config from DB: %v, using defaults", err)
		return req
	}
	if config == nil {
		logging.Info("[GatewayManager] No protection config in DB, using defaults")
		return req
	}

	req.SandboxEnabled = config.SandboxEnabled

	// 解析权限 JSON（数据库中以 JSON 字符串存储）
	if config.PathPermission != "" {
		var pp sandbox.PathPermissionConfig
		if err := json.Unmarshal([]byte(config.PathPermission), &pp); err == nil {
			req.PathPermission = pp
		} else {
			logging.Warning("[GatewayManager] Failed to parse path_permission: %v", err)
		}
	}
	if config.NetworkPermission != "" {
		var np sandbox.NetworkPermissionConfig
		if err := json.Unmarshal([]byte(config.NetworkPermission), &np); err == nil {
			req.NetworkPermission = np
		} else {
			logging.Warning("[GatewayManager] Failed to parse network_permission: %v", err)
		}
	}
	if config.ShellPermission != "" {
		var sp sandbox.ShellPermissionConfig
		if err := json.Unmarshal([]byte(config.ShellPermission), &sp); err == nil {
			req.ShellPermission = sp
		} else {
			logging.Warning("[GatewayManager] Failed to parse shell_permission: %v", err)
		}
	}

	// policyDir 使用PathManager（优先）或默认位置
	var policyDir string
	pm := core.GetPathManager()
	if pm.IsInitialized() {
		policyDir = pm.GetPolicyDir()
	} else {
		homeDir, _ := os.UserHomeDir()
		policyDir = filepath.Join(homeDir, ".botsec", "policies")
	}
	req.PolicyDir = policyDir

	logging.Info("[GatewayManager] Built request from DB: sandbox=%v, asset=%s, assetID=%s",
		req.SandboxEnabled, req.AssetName, req.AssetID)
	return req
}

// startGatewayWithProxy 在代理启动时自动调用，完成以下步骤：
// 1. 更新 openclaw.json 配置（将 Bot 模型配置指向本地代理）
// 2. 重启 openclaw gateway
// 这是内部方法，由 StartProtectionProxy 自动调用，不对外暴露 FFI。
func startGatewayWithProxy(proxyPort int, botModelConfig *BotModelConfig, backupDir string, assetID string) (map[string]interface{}, error) {
	logging.Info("[GatewayManager] === startGatewayWithProxy called, proxyPort=%d, assetID=%s ===", proxyPort, strings.TrimSpace(assetID))

	result := map[string]interface{}{
		"success": false,
	}

	if IsAppStoreBuild() {
		logging.Info("[GatewayManager] Early return: App Store build, cannot update openclaw config")
		result["error"] = "app store build cannot update openclaw config"
		return result, nil
	}

	if proxyPort < 1 || proxyPort > 65535 {
		logging.Warning("[GatewayManager] Early return: invalid proxy port (%d)", proxyPort)
		result["error"] = "invalid proxy port"
		return result, nil
	}
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", proxyPort)

	if botModelConfig == nil {
		logging.Warning("[GatewayManager] Early return: bot model config is nil")
		result["error"] = "bot model config is nil"
		return result, nil
	}

	logging.Info("[GatewayManager] Received bot config: Provider=%s, Model=%s, proxyPort=%d",
		botModelConfig.Provider, botModelConfig.Model, proxyPort)

	providerName, baseModel, newModel, err := buildBotModelIdentity(botModelConfig)
	if err != nil {
		logging.Error("[GatewayManager] Build identity failed: %v", err)
		result["error"] = fmt.Sprintf("invalid bot model identity: %v", err)
		return result, err
	}
	logging.Info("[GatewayManager] Identity built: provider=%s, baseModel=%s, newModel=%s",
		providerName, baseModel, newModel)

	// Step 1: 查找 openclaw.json 并备份
	if backupDir == "" {
		result["error"] = "backup directory is empty"
		return result, fmt.Errorf("backup directory is empty")
	}

	configPath, err := findConfigPath()
	if err != nil {
		result["error"] = fmt.Sprintf("config not found: %v", err)
		return result, err
	}
	result["config_path"] = configPath

	backupPath, err := backupOpenclawConfig(configPath, backupDir)
	if err != nil {
		errMsg := fmt.Sprintf("backup failed: %v", err)
		logging.Error("[GatewayManager] %s", errMsg)
		result["error"] = errMsg
		return result, err
	}
	result["backup_path"] = backupPath
	logging.Info("[GatewayManager] Step 1: backup config from %s to %s", configPath, backupPath)

	restoreOnError := func(applyErr error) (map[string]interface{}, error) {
		logging.Error("[GatewayManager] Apply failed: %v, restore start from %s", applyErr, backupPath)
		if restoreErr := restoreOpenclawConfig(configPath, backupPath); restoreErr != nil {
			restoreErrMsg := fmt.Sprintf("restore failed: %v", restoreErr)
			logging.Error("[GatewayManager] %s", restoreErrMsg)
			result["error"] = fmt.Sprintf("%v; %s", applyErr, restoreErrMsg)
			result["restore_failed"] = true
			result["restore_error"] = restoreErr.Error()
			return result, fmt.Errorf("%v; %v", applyErr, restoreErr)
		}
		logging.Info("[GatewayManager] Restore success")
		result["error"] = applyErr.Error()
		result["restore_failed"] = false
		return result, applyErr
	}

	config, rawConfig, err := loadConfig(configPath)
	if err != nil {
		return restoreOnError(fmt.Errorf("load config failed: %w", err))
	}

	currentPrimary, err := getPrimaryModelFromConfig(config)
	if err != nil {
		return restoreOnError(fmt.Errorf("read primary model failed: %w", err))
	}

	// Step 2: models.providers - 确保 provider 存在
	previousProvider, updatedProvider, err := ensureProviderForBotModel(rawConfig, botModelConfig, providerName, baseModel)
	if err != nil {
		return restoreOnError(fmt.Errorf("ensure provider failed: %w", err))
	}
	logging.Info("[GatewayManager] Step 2 models.providers old=%v, new=%v", previousProvider, updatedProvider)

	// Step 3: 设置 provider baseUrl 为本地代理地址
	if updatedProvider != nil {
		updatedProvider["baseUrl"] = proxyURL
	}
	logging.Info("[GatewayManager] Step 3 models.providers change baseUrl to local proxy address: %s", proxyURL)

	// Step 4: agents.defaults.model.fallbacks
	previousFallbacks, updatedFallbacks, err := updateAgentsDefaultsFallbacks(rawConfig, currentPrimary)
	if err != nil {
		return restoreOnError(fmt.Errorf("update fallbacks failed: %w", err))
	}
	logging.Info("[GatewayManager] Step 4 agents.defaults.model.fallbacks old=%v, new=%v", previousFallbacks, updatedFallbacks)

	// Step 5: agents.defaults.models
	previousModels, updatedModels, err := updateAgentsDefaultsModels(rawConfig, newModel)
	if err != nil {
		return restoreOnError(fmt.Errorf("update models whitelist failed: %w", err))
	}
	logging.Info("[GatewayManager] Step 5 agents.defaults.models old=%v, new=%v", previousModels, updatedModels)

	// Step 6: agents.defaults.model.primary
	previousPrimary, updatedPrimary, err := setAgentsDefaultsPrimary(rawConfig, newModel)
	if err != nil {
		return restoreOnError(fmt.Errorf("update primary failed: %w", err))
	}
	logging.Info("[GatewayManager] Step 6 agents.defaults.model.primary old=%v, new=%v", previousPrimary, updatedPrimary)

	// Step 7: 保存配置
	if err := saveConfig(configPath, rawConfig); err != nil {
		return restoreOnError(fmt.Errorf("save config failed: %w", err))
	}

	// 验证文件内容是否正确写入
	verifyData, verifyErr := os.ReadFile(configPath)
	if verifyErr != nil {
		logging.Error("[GatewayManager] Verify read failed: %v", verifyErr)
	} else {
		verifyHash := md5.Sum(verifyData)
		verifyHashStr := hex.EncodeToString(verifyHash[:])
		logging.Info("[GatewayManager] Saved config MD5: %s", verifyHashStr)
	}

	// Step 8: 从数据库读取防护配置，调用统一的网关重启方法（含 install + 沙箱同步）
	logging.Info("[GatewayManager] Step 8: restarting openclaw gateway with full config...")
	gatewayReq := buildGatewayRestartRequestFromDB(assetID)
	restartResult, restartErr := restartOpenclawGateway(gatewayReq)
	if restartErr != nil {
		logging.Warning("[GatewayManager] Step 8: gateway restart failed: %v", restartErr)
		result["gateway_restart_error"] = restartErr.Error()
	} else {
		logging.Info("[GatewayManager] Step 8: gateway restarted successfully: %v", restartResult)
		result["gateway_restarted"] = true
	}

	result["success"] = true
	result["new_model"] = newModel
	result["old_model"] = currentPrimary
	logging.Info("[GatewayManager] === startGatewayWithProxy completed: new_model=%s, old_model=%s ===", newModel, currentPrimary)
	return result, nil
}

// === 跨平台辅助函数 ===

func resolveOpenclawBinaryPath() string {
	if p, err := exec.LookPath("openclaw"); err == nil {
		logging.Info("[GatewayManager] resolveOpenclawBinaryPath: found: %s", p)
		return p
	} else {
		logging.Warning("[GatewayManager] resolveOpenclawBinaryPath: openclaw not found in PATH: %v", err)
	}
	return ""
}

func expandHome(p string, home string) string {
	if strings.HasPrefix(p, "~/") && home != "" {
		return filepath.Join(home, strings.TrimPrefix(p, "~/"))
	}
	return p
}

func sanitizeFileName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func buildGatewayInstanceKey(assetName, assetID string) string {
	_ = assetName
	id := strings.TrimSpace(assetID)
	if id != "" {
		return id
	}
	return openclawAssetName
}

func buildGatewayRuntimeStateKeys(assetName, assetID string) []string {
	keys := make([]string, 0, 2)
	appendKey := func(key string) {
		key = strings.TrimSpace(key)
		if key == "" || containsString(keys, key) {
			return
		}
		keys = append(keys, key)
	}

	appendKey(buildGatewayInstanceKey(assetName, assetID))
	appendKey(assetName)

	return keys
}

func cleanupGatewayManagedRuntimeState(assetName, assetID string) {
	for _, key := range buildGatewayRuntimeStateKeys(assetName, assetID) {
		sandbox.RemoveProcessMonitor(key)
		sandbox.RemoveSandboxManagerByKey(key)
	}
}

func writeIfChanged(path string, oldBytes []byte, newBytes []byte) (bool, error) {
	if bytes.Equal(oldBytes, newBytes) {
		return false, nil
	}
	if err := os.WriteFile(path, newBytes, 0644); err != nil {
		return false, err
	}
	return true, nil
}
