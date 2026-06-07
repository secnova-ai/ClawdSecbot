package coclaw

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"go_lib/core"
	openclawplugin "go_lib/plugins/openclaw"
)

var (
	configPathOverride string
	overrideMu         sync.Mutex
	userHomeDir        = func() (string, error) {
		return os.UserHomeDir()
	}
)

type coclawConfig struct {
	Agents struct {
		Defaults struct {
			Model interface{} `json:"model"`
		} `json:"defaults"`
	} `json:"agents"`
	Models struct {
		Providers map[string]map[string]interface{} `json:"providers"`
	} `json:"models"`
	Gateway struct {
		Bind string `json:"bind"`
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"gateway"`
}

// findConfigPath 定位 CoClaw 的 OpenClaw 兼容配置文件，优先使用测试或宿主传入的覆盖路径。
func findConfigPath() (string, error) {
	if strings.TrimSpace(configPathOverride) != "" {
		return configPathOverride, nil
	}
	homeDir, err := userHomeDir()
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Join(homeDir, ".coclaw", "openclaw.json"),
		filepath.Join(homeDir, ".coclaw", "coclaw.json"),
	}
	if appData := coclawAppDataDir(homeDir); appData != "" {
		candidates = append(candidates,
			filepath.Join(appData, "openclaw.json"),
			filepath.Join(appData, "coclaw.json"),
		)
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("coclaw config not found")
}

func coclawAppDataDir(homeDir string) string {
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "CoClaw")
	case "windows":
		return filepath.Join(homeDir, "AppData", "Roaming", "CoClaw")
	default:
		return filepath.Join(homeDir, ".config", "CoClaw")
	}
}

func loadConfig() (*coclawConfig, map[string]interface{}, string, error) {
	configPath, err := findConfigPath()
	if err != nil {
		return nil, nil, "", err
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, "", err
	}
	var cfg coclawConfig
	if err := json.Unmarshal(content, &cfg); err != nil {
		return nil, nil, "", err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, nil, "", err
	}
	return &cfg, raw, configPath, nil
}

func saveRawConfig(configPath string, raw map[string]interface{}) error {
	content, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return os.WriteFile(configPath, content, 0644)
}

func backupDir(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".botsec", "backups", "coclaw")
}

func initialBackupPath(dir string) string {
	return filepath.Join(dir, "coclaw.json.initial")
}

func ensureInitialBackup(configPath string, dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", fmt.Errorf("backup directory is empty")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	backupPath := initialBackupPath(dir)
	if _, err := os.Stat(backupPath); err == nil {
		return backupPath, nil
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(backupPath, content, 0644); err != nil {
		return "", err
	}
	return backupPath, nil
}

func loadBackupRaw(path string) (map[string]interface{}, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func stableConfigFingerprint(path string) string {
	return core.ResolveStableConfigPathFingerprint(path)
}

// withOpenclawOverrides 临时让通用 OpenClaw 能力读取 CoClaw 的兼容配置目录。
func withOpenclawOverrides() (func(), error) {
	configPath, err := findConfigPath()
	if err != nil {
		return nil, err
	}

	overrideMu.Lock()
	previousConfigPath := openclawplugin.GetConfigPath()
	previousAppStoreBuild := openclawplugin.IsAppStoreBuild()

	openclawplugin.SetConfigPath(filepath.Dir(configPath))
	openclawplugin.SetAppStoreBuild(false)

	return func() {
		openclawplugin.SetConfigPath(previousConfigPath)
		openclawplugin.SetAppStoreBuild(previousAppStoreBuild)
		overrideMu.Unlock()
	}, nil
}
