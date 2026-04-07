package qclaw

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	openclawplugin "go_lib/plugins/openclaw"
)

const (
	qclawAssetName = "QClaw"
	qclawPluginID  = "qclaw"
)

var (
	qclawConfigPathOverride string
	qclawStatePathOverride  string
	qclawOverrideMu         sync.Mutex
	userCurrentHomeDir      = func() (string, error) {
		return os.UserHomeDir()
	}
)

type qclawConfig struct {
	Agents struct {
		Defaults struct {
			Model     interface{} `json:"model"`
			Workspace string      `json:"workspace"`
		} `json:"defaults"`
	} `json:"agents"`
	Models struct {
		Providers map[string]map[string]interface{} `json:"providers"`
	} `json:"models"`
	Skills struct {
		Load struct {
			ExtraDirs []string `json:"extraDirs"`
		} `json:"load"`
	} `json:"skills"`
	Gateway struct {
		Bind string `json:"bind"`
		Host string `json:"host"`
		Port int    `json:"port"`
		Auth struct {
			Mode     string `json:"mode"`
			Enabled  bool   `json:"enabled"`
			Token    string `json:"token"`
			Password string `json:"password"`
		} `json:"auth"`
	} `json:"gateway"`
	Logging struct {
		RedactSensitive string `json:"redactSensitive"`
	} `json:"logging"`
}

type qclawState struct {
	AuthGatewayBaseURL string `json:"authGatewayBaseUrl"`
	ConfigPath         string `json:"configPath"`
	Port               int    `json:"port"`
	Platform           string `json:"platform"`
	StateDir           string `json:"stateDir"`
	CLI                struct {
		NodeBinary  string `json:"nodeBinary"`
		OpenclawMjs string `json:"openclawMjs"`
		PID         int    `json:"pid"`
	} `json:"cli"`
}

func setOpenclawOverrides() {
	if configPath, err := findConfigPath(); err == nil {
		openclawplugin.SetConfigPath(filepath.Dir(configPath))
	}
	openclawplugin.SetAppStoreBuild(false)
}

func withOpenclawOverrides() (func(), error) {
	configPath, err := findConfigPath()
	if err != nil {
		return nil, err
	}

	qclawOverrideMu.Lock()
	prevConfigPath := openclawplugin.GetConfigPath()
	prevAppStoreBuild := openclawplugin.IsAppStoreBuild()

	openclawplugin.SetConfigPath(filepath.Dir(configPath))
	openclawplugin.SetAppStoreBuild(false)

	return func() {
		openclawplugin.SetConfigPath(prevConfigPath)
		openclawplugin.SetAppStoreBuild(prevAppStoreBuild)
		qclawOverrideMu.Unlock()
	}, nil
}

func findConfigPath() (string, error) {
	if strings.TrimSpace(qclawConfigPathOverride) != "" {
		return qclawConfigPathOverride, nil
	}

	if statePath, err := findStatePath(); err == nil {
		state, err := loadState(statePath)
		if err == nil && strings.TrimSpace(state.ConfigPath) != "" {
			if _, err := os.Stat(state.ConfigPath); err == nil {
				return state.ConfigPath, nil
			}
		}
	}

	homeDir, err := userCurrentHomeDir()
	if err != nil {
		return "", err
	}

	candidates := []string{
		filepath.Join(homeDir, ".qclaw", "openclaw.json"),
		filepath.Join(homeDir, ".openclaw", "openclaw.json"),
	}
	if appDataDir := qclawAppDataDir(homeDir); appDataDir != "" {
		candidates = append(candidates, filepath.Join(appDataDir, "openclaw.json"))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("qclaw config not found")
}

func findStatePath() (string, error) {
	if strings.TrimSpace(qclawStatePathOverride) != "" {
		return qclawStatePathOverride, nil
	}

	homeDir, err := userCurrentHomeDir()
	if err != nil {
		return "", err
	}

	candidate := filepath.Join(homeDir, ".qclaw", "qclaw.json")
	candidates := []string{candidate}
	if appDataDir := qclawAppDataDir(homeDir); appDataDir != "" {
		candidates = append(candidates, filepath.Join(appDataDir, "qclaw.json"))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("qclaw state not found")
}

func expandQClawPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}

	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		if homeDir, err := userCurrentHomeDir(); err == nil {
			return filepath.Join(homeDir, path[2:])
		}
	}

	return os.ExpandEnv(path)
}

func loadConfig() (*qclawConfig, map[string]interface{}, string, error) {
	configPath, err := findConfigPath()
	if err != nil {
		return nil, nil, "", err
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, "", err
	}

	var parsed qclawConfig
	if err := json.Unmarshal(content, &parsed); err != nil {
		return nil, nil, "", err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, nil, "", err
	}

	return &parsed, raw, configPath, nil
}

func loadState(path string) (*qclawState, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var state qclawState
	if err := json.Unmarshal(content, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func saveRawConfig(configPath string, raw map[string]interface{}) error {
	content, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return os.WriteFile(configPath, content, 0644)
}

func getBackupDir(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".botsec", "backups", "qclaw")
}

func getInitialBackupPath(backupDir string) string {
	return filepath.Join(backupDir, "openclaw.json.initial")
}

func qclawAppDataDir(homeDir string) string {
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return ""
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "QClaw")
	case "windows":
		return filepath.Join(homeDir, "AppData", "Roaming", "QClaw")
	default:
		return filepath.Join(homeDir, ".config", "QClaw")
	}
}

func qclawLogsDir(homeDir string) string {
	appDataDir := qclawAppDataDir(homeDir)
	if appDataDir == "" {
		return ""
	}
	return filepath.Join(appDataDir, "logs")
}

func ensureInitialBackup(configPath, backupDir string) (string, error) {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}

	backupPath := getInitialBackupPath(backupDir)
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
