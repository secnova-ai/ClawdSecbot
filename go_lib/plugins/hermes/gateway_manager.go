package hermes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"go_lib/core"
	"go_lib/core/cmdutil"
	"go_lib/core/logging"
	"go_lib/core/proxy"
	"go_lib/core/repository"
)

const proxyInjectedAPIKey = "botsec-proxy-key"

var gatewayRestartMu sync.Mutex

// restartGatewayFn is replaceable in tests.
var restartGatewayFn = restartHermesGateway

// GatewayRestartRequest is the minimal request for Hermes gateway restart.
type GatewayRestartRequest struct {
	AssetName      string `json:"asset_name"`
	AssetID        string `json:"asset_id,omitempty"`
	SandboxEnabled bool   `json:"sandbox_enabled"`
}

func buildGatewayRestartRequestFromDB(assetID string) *GatewayRestartRequest {
	req := &GatewayRestartRequest{
		AssetName:      hermesAssetName,
		AssetID:        strings.TrimSpace(assetID),
		SandboxEnabled: false,
	}

	repo := repository.NewProtectionRepository(nil)
	cfg, err := repo.GetProtectionConfig(req.AssetID)
	if err != nil || cfg == nil {
		return req
	}
	req.SandboxEnabled = cfg.SandboxEnabled
	return req
}

func sanitizeFileName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func computeConfigHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:8])
}

func resolveBackupFile(backupDir, assetID string) (string, error) {
	backupDir = strings.TrimSpace(backupDir)
	if backupDir == "" {
		return "", fmt.Errorf("backup dir is empty")
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}
	assetKey := sanitizeFileName(strings.TrimSpace(assetID))
	if assetKey == "" {
		assetKey = "default"
	}
	dir := filepath.Join(backupDir, "hermes", assetKey)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml.bak"), nil
}

func backupHermesConfig(configPath, backupDir, assetID string) (string, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	backupPath, err := resolveBackupFile(backupDir, assetID)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(backupPath, content, 0600); err != nil {
		return "", err
	}
	logging.Info("[HermesGateway] Backed up config: path=%s hash=%s", backupPath, computeConfigHash(content))
	return backupPath, nil
}

func restoreHermesConfig(configPath, backupPath string) error {
	content, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(configPath, content, 0600); err != nil {
		return err
	}
	logging.Info("[HermesGateway] Restored config from backup: path=%s hash=%s", backupPath, computeConfigHash(content))
	return nil
}

func restoreHermesConfigByAsset(backupDir, assetID string) map[string]interface{} {
	configPath, err := findConfigPath()
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	backupPath, err := resolveBackupFile(backupDir, assetID)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	if _, err := os.Stat(backupPath); err != nil {
		return map[string]interface{}{"success": false, "error": "backup not found"}
	}
	if err := restoreHermesConfig(configPath, backupPath); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	_, restartErr := restartGatewayFn(buildGatewayRestartRequestFromDB(assetID))
	if restartErr != nil {
		return map[string]interface{}{"success": false, "error": restartErr.Error()}
	}
	return map[string]interface{}{"success": true, "message": "restored and restarted"}
}

func hasHermesBackup(backupDir, assetID string) bool {
	backupPath, err := resolveBackupFile(backupDir, assetID)
	if err != nil {
		return false
	}
	_, err = os.Stat(backupPath)
	return err == nil
}

func updateHermesProxyModel(raw map[string]interface{}, proxyURL string, botCfg *proxy.BotModelConfig) error {
	if raw == nil {
		return fmt.Errorf("raw config is nil")
	}
	model := ensureMap(raw, "model")
	model["provider"] = "custom"
	model["base_url"] = strings.TrimSpace(proxyURL)
	model["api_key"] = proxyInjectedAPIKey

	modelID := strings.TrimSpace(botCfg.Model)
	if modelID == "" {
		return fmt.Errorf("bot model is empty")
	}
	model["default"] = modelID
	return nil
}

func startGatewayWithProxy(proxyPort int, botModelConfig *proxy.BotModelConfig, backupDir string, assetID string) (map[string]interface{}, error) {
	result := map[string]interface{}{"success": false}

	if IsAppStoreBuild() {
		result["error"] = "app store build cannot update hermes config"
		return result, nil
	}
	if proxyPort < 1 || proxyPort > 65535 {
		result["error"] = "invalid proxy port"
		return result, fmt.Errorf("invalid proxy port: %d", proxyPort)
	}
	if botModelConfig == nil {
		result["error"] = "bot model config is nil"
		return result, fmt.Errorf("bot model config is nil")
	}
	if strings.TrimSpace(botModelConfig.Model) == "" {
		result["error"] = "bot model is empty"
		return result, fmt.Errorf("bot model is empty")
	}
	if strings.TrimSpace(backupDir) == "" {
		result["error"] = "backup directory is empty"
		return result, fmt.Errorf("backup directory is empty")
	}

	configPath, err := findConfigPath()
	if err != nil {
		result["error"] = fmt.Sprintf("config not found: %v", err)
		return result, err
	}
	result["config_path"] = configPath

	backupPath, err := backupHermesConfig(configPath, backupDir, assetID)
	if err != nil {
		result["error"] = fmt.Sprintf("backup failed: %v", err)
		return result, err
	}
	result["backup_path"] = backupPath

	cfg, raw, err := loadConfig(configPath)
	if err != nil {
		_ = restoreHermesConfig(configPath, backupPath)
		result["error"] = fmt.Sprintf("load config failed: %v", err)
		return result, err
	}
	_ = cfg

	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", proxyPort)
	if err := updateHermesProxyModel(raw, proxyURL, botModelConfig); err != nil {
		_ = restoreHermesConfig(configPath, backupPath)
		result["error"] = fmt.Sprintf("update config failed: %v", err)
		return result, err
	}

	if err := saveConfig(configPath, raw); err != nil {
		_ = restoreHermesConfig(configPath, backupPath)
		result["error"] = fmt.Sprintf("save config failed: %v", err)
		return result, err
	}

	restartResult, restartErr := restartGatewayFn(buildGatewayRestartRequestFromDB(assetID))
	if restartErr != nil {
		_ = restoreHermesConfig(configPath, backupPath)
		result["error"] = fmt.Sprintf("gateway restart failed: %v", restartErr)
		result["restart_result"] = restartResult
		return result, restartErr
	}

	result["success"] = true
	result["proxy_url"] = proxyURL
	result["gateway_result"] = restartResult
	return result, nil
}

func resolveHermesBinaryPath() string {
	if p, err := exec.LookPath("hermes"); err == nil {
		return p
	}
	return ""
}

func runHermesGatewayCommand(binaryPath string, args []string, homeDir string) (string, error) {
	if strings.TrimSpace(binaryPath) == "" {
		return "", fmt.Errorf("hermes binary not found")
	}
	cmdArgs := append([]string{"gateway"}, args...)
	cmd := cmdutil.Command(binaryPath, cmdArgs...)
	if strings.TrimSpace(homeDir) != "" {
		envKey := "HOME"
		if runtime.GOOS == "windows" {
			envKey = "USERPROFILE"
		}
		cmd.Env = append(os.Environ(), envKey+"="+homeDir)
	}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func restartHermesGateway(req *GatewayRestartRequest) (map[string]interface{}, error) {
	gatewayRestartMu.Lock()
	defer gatewayRestartMu.Unlock()

	if req == nil {
		req = &GatewayRestartRequest{AssetName: hermesAssetName}
	}
	binaryPath := resolveHermesBinaryPath()
	if binaryPath == "" {
		return nil, fmt.Errorf("hermes binary not found in PATH")
	}

	homeDir := ""
	if pm := core.GetPathManager(); pm.IsInitialized() {
		homeDir = pm.GetHomeDir()
	}
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}

	// Prefer explicit restart, fallback to stop+start.
	output, err := runHermesGatewayCommand(binaryPath, []string{"restart"}, homeDir)
	if err == nil {
		return map[string]interface{}{
			"success":         true,
			"asset_id":        strings.TrimSpace(req.AssetID),
			"sandbox_enabled": req.SandboxEnabled,
			"command":         "gateway restart",
			"output":          output,
		}, nil
	}

	stopOut, _ := runHermesGatewayCommand(binaryPath, []string{"stop"}, homeDir)
	startOut, startErr := runHermesGatewayCommand(binaryPath, []string{"start"}, homeDir)
	if startErr != nil {
		return map[string]interface{}{
			"success":       false,
			"restart_error": err.Error(),
			"stop_output":   stopOut,
			"start_output":  startOut,
		}, fmt.Errorf("restart failed: %w", startErr)
	}

	return map[string]interface{}{
		"success":         true,
		"asset_id":        strings.TrimSpace(req.AssetID),
		"sandbox_enabled": req.SandboxEnabled,
		"command":         "gateway stop/start",
		"stop_output":     stopOut,
		"start_output":    startOut,
	}, nil
}

func toJSONString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"success":false,"error":"marshal error"}`
	}
	return string(b)
}
