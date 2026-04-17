package dintalclaw

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/repository"
	"go_lib/core/sandbox"
)

var gatewayRestartMu sync.Mutex

// LaunchMode 控制 dintalclaw 进程的启动路径
//   - "gui"     : python3 launch.pyw （前端桌面场景，默认）
//   - "browser" : python3 -m streamlit run stapp.py （浏览器/无头场景）
//   - "cli"     : python3 agentmain.py （命令行场景）
//   - ""        : 自动检测，按 launch.pyw > stapp.py > agentmain.py 优先级
type LaunchMode string

const (
	LaunchModeGUI     LaunchMode = "gui"
	LaunchModeBrowser LaunchMode = "browser"
	LaunchModeCLI     LaunchMode = "cli"
	LaunchModeAuto    LaunchMode = ""
)

// GatewayRestartRequest 进程重启请求
type GatewayRestartRequest struct {
	AssetName         string                          `json:"asset_name"`
	AssetID           string                          `json:"asset_id,omitempty"`
	SandboxEnabled    bool                            `json:"sandbox_enabled"`
	LaunchMode        LaunchMode                      `json:"launch_mode,omitempty"`
	PathPermission    sandbox.PathPermissionConfig    `json:"path_permission"`
	NetworkPermission sandbox.NetworkPermissionConfig `json:"network_permission"`
	ShellPermission   sandbox.ShellPermissionConfig   `json:"shell_permission"`
	PolicyDir         string                          `json:"policy_dir"`
}

// buildGatewayRestartRequestFromDB 从数据库读取防护配置，构造重启请求
func buildGatewayRestartRequestFromDB(assetID string) *GatewayRestartRequest {
	req := &GatewayRestartRequest{
		AssetName: dintalclawAssetName,
		AssetID:   strings.TrimSpace(assetID),
	}

	repo := repository.NewProtectionRepository(nil)
	config, err := repo.GetProtectionConfig(req.AssetID)
	if err != nil {
		logging.Warning("[GatewayManager] Failed to read protection config from DB: %v, using defaults", err)
		return req
	}
	if config == nil {
		logging.Info("[GatewayManager] No protection config in DB, using defaults")
		return req
	}

	req.SandboxEnabled = config.SandboxEnabled

	if config.PathPermission != "" {
		var pp sandbox.PathPermissionConfig
		if err := json.Unmarshal([]byte(config.PathPermission), &pp); err == nil {
			req.PathPermission = pp
		}
	}
	if config.NetworkPermission != "" {
		var np sandbox.NetworkPermissionConfig
		if err := json.Unmarshal([]byte(config.NetworkPermission), &np); err == nil {
			req.NetworkPermission = np
		}
	}
	if config.ShellPermission != "" {
		var sp sandbox.ShellPermissionConfig
		if err := json.Unmarshal([]byte(config.ShellPermission), &sp); err == nil {
			req.ShellPermission = sp
		}
	}

	var policyDir string
	pm := core.GetPathManager()
	if pm.IsInitialized() {
		policyDir = pm.GetPolicyDir()
	} else {
		homeDir, _ := os.UserHomeDir()
		policyDir = core.ResolvePolicyDir(homeDir)
	}
	req.PolicyDir = policyDir

	logging.Info("[GatewayManager] Built request from DB: sandbox=%v, asset=%s, assetID=%s",
		req.SandboxEnabled, req.AssetName, req.AssetID)
	return req
}

// startProcessWithProxy 代理启动时调用：
// 1. 备份 mykey.py
// 2. 注入 oai_config0 代理块
// 3. 重启 dintalclaw 进程
func startProcessWithProxy(proxyPort int, botModelConfig *BotModelConfig, backupDir string, assetID string) (map[string]interface{}, error) {
	logging.Info("[GatewayManager] === startProcessWithProxy called, proxyPort=%d, assetID=%s ===", proxyPort, strings.TrimSpace(assetID))

	result := map[string]interface{}{
		"success": false,
	}

	if proxyPort < 1 || proxyPort > 65535 {
		result["error"] = "invalid proxy port"
		return result, nil
	}
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", proxyPort)

	if botModelConfig == nil {
		result["error"] = "bot model config is nil"
		return result, nil
	}

	logging.Info("[GatewayManager] Received bot config: Provider=%s, Model=%s, proxyPort=%d",
		botModelConfig.Provider, botModelConfig.Model, proxyPort)

	if backupDir == "" {
		result["error"] = "backup directory is empty"
		return result, fmt.Errorf("backup directory is empty")
	}

	configPath, err := findConfigPathForDintalclaw()
	if err != nil {
		result["error"] = fmt.Sprintf("config not found: %v", err)
		return result, err
	}
	result["config_path"] = configPath

	backupPath, err := backupDintalclawConfig(configPath, backupDir)
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
		if restoreErr := restoreDintalclawConfig(configPath, backupPath); restoreErr != nil {
			result["error"] = fmt.Sprintf("%v; restore failed: %v", applyErr, restoreErr)
			result["restore_failed"] = true
			return result, fmt.Errorf("%v; %v", applyErr, restoreErr)
		}
		logging.Info("[GatewayManager] Restore success")
		result["error"] = applyErr.Error()
		result["restore_failed"] = false
		return result, applyErr
	}

	apiKey := ""
	if botModelConfig.APIKey != "" {
		apiKey = botModelConfig.APIKey
	}
	modelName := strings.TrimSpace(botModelConfig.Model)
	if err := injectOaiConfig0(configPath, proxyURL, apiKey, modelName); err != nil {
		return restoreOnError(fmt.Errorf("inject oai_config0 failed: %w", err))
	}
	logging.Info("[GatewayManager] Step 2: injected oai_config0 with proxy=%s", proxyURL)

	verifyData, verifyErr := os.ReadFile(configPath)
	if verifyErr != nil {
		logging.Error("[GatewayManager] Verify read failed: %v", verifyErr)
	} else {
		verifyHash := md5.Sum(verifyData)
		logging.Info("[GatewayManager] Saved config MD5: %s", hex.EncodeToString(verifyHash[:]))
	}

	logging.Info("[GatewayManager] Step 3: restarting dintalclaw process...")
	gatewayReq := buildGatewayRestartRequestFromDB(assetID)
	restartResult, restartErr := restartDintalclawProcess(gatewayReq)
	if restartErr != nil {
		logging.Warning("[GatewayManager] Step 3: process restart failed: %v", restartErr)
		result["restart_error"] = restartErr.Error()
	} else {
		logging.Info("[GatewayManager] Step 3: process restarted successfully: %v", restartResult)
		result["process_restarted"] = true
	}

	result["success"] = true
	logging.Info("[GatewayManager] === startProcessWithProxy completed ===")
	return result, nil
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
	return dintalclawAssetName
}
