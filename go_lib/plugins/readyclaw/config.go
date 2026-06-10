package readyclaw

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go_lib/core"
)

const readyclawInitialBackupName = "readyclaw-config.initial.json"

var (
	readyclawConfigPathOverride string
	readyclawProgramDataDir     = func() string {
		return os.Getenv("ProgramData")
	}
	readyclawUserHomeDir = func() (string, error) {
		return os.UserHomeDir()
	}
)

type readyclawConfig struct {
	ConfigVersion int                    `json:"configVersion"`
	Values        map[string]interface{} `json:"values"`
}

// findConfigPath 用于定位 ReadyClaw 魔改版运行时配置，Windows 优先使用服务态 ProgramData 路径。
func findConfigPath() (string, error) {
	if path := strings.TrimSpace(readyclawConfigPathOverride); path != "" {
		return path, nil
	}

	candidates := readyclawConfigCandidates()
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("readyclaw config not found")
}

// readyclawConfigCandidates 汇总服务态和用户态候选路径，兼容 NanoClaw 安装名与 ReadyClaw 展示名不一致。
func readyclawConfigCandidates() []string {
	candidates := make([]string, 0, 4)

	if programData := strings.TrimSpace(readyclawProgramDataDir()); programData != "" {
		candidates = append(candidates,
			filepath.Join(programData, "NanoClaw", "config", "nanoclaw", "config.json"),
			filepath.Join(programData, "readyclaw", "config.json"),
			filepath.Join(programData, "nanoclaw", "config.json"),
		)
	}

	if homeDir, err := readyclawUserHomeDir(); err == nil && strings.TrimSpace(homeDir) != "" {
		candidates = append(candidates,
			filepath.Join(homeDir, ".config", "readyclaw", "config.json"),
			filepath.Join(homeDir, ".config", "nanoclaw", "config.json"),
		)
	}

	if runtime.GOOS == "darwin" {
		if homeDir, err := readyclawUserHomeDir(); err == nil && strings.TrimSpace(homeDir) != "" {
			candidates = append(candidates, filepath.Join(homeDir, "Library", "Application Support", "ReadyClaw", "config.json"))
		}
	}

	return candidates
}

// loadConfig 读取 ReadyClaw 扁平 values 配置，同时保留原始 map 以便无损写回未知字段。
func loadConfig() (*readyclawConfig, map[string]interface{}, string, error) {
	configPath, err := findConfigPath()
	if err != nil {
		return nil, nil, "", err
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, "", err
	}

	cfg, raw, err := parseReadyClawConfig(content)
	if err != nil {
		return nil, nil, "", err
	}
	return cfg, raw, configPath, nil
}

// parseReadyClawConfig 解析 ReadyClaw 配置并补齐 values map，避免空配置写回时丢字段。
func parseReadyClawConfig(content []byte) (*readyclawConfig, map[string]interface{}, error) {
	var cfg readyclawConfig
	if len(strings.TrimSpace(string(content))) == 0 {
		cfg.ConfigVersion = 1
		cfg.Values = map[string]interface{}{}
		return &cfg, map[string]interface{}{"configVersion": float64(1), "values": map[string]interface{}{}}, nil
	}

	if err := json.Unmarshal(content, &cfg); err != nil {
		return nil, nil, err
	}
	if cfg.ConfigVersion == 0 {
		cfg.ConfigVersion = 1
	}
	if cfg.Values == nil {
		cfg.Values = map[string]interface{}{}
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, nil, err
	}
	if raw == nil {
		raw = map[string]interface{}{}
	}
	if _, ok := raw["configVersion"]; !ok {
		raw["configVersion"] = float64(cfg.ConfigVersion)
	}
	if _, ok := raw["values"].(map[string]interface{}); !ok {
		raw["values"] = map[string]interface{}{}
	}
	return &cfg, raw, nil
}

// saveRawConfig 将 ReadyClaw 配置格式化写回，保留未知配置项供魔改版继续使用。
func saveRawConfig(configPath string, raw map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return os.WriteFile(configPath, content, 0644)
}

// getBackupDir 为 ReadyClaw 配置恢复提供稳定备份目录，允许代理上下文显式覆盖。
func getBackupDir(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".botsec", "backups", "readyclaw")
}

// getInitialBackupPath 返回首次开启防护时保存的原始配置路径。
func getInitialBackupPath(backupDir string) string {
	return filepath.Join(backupDir, readyclawInitialBackupName)
}

// ensureInitialBackup 只保存第一次接管前的配置，避免多次开启后把代理地址误当作原始上游。
func ensureInitialBackup(configPath, backupDir string) (string, error) {
	if strings.TrimSpace(backupDir) == "" {
		return "", fmt.Errorf("backup dir is empty")
	}
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

// ensureValuesMap 获取并补齐扁平 values 对象，ReadyClaw 的 LLM 字段都存放在这里。
func ensureValuesMap(raw map[string]interface{}) map[string]interface{} {
	if values, ok := raw["values"].(map[string]interface{}); ok {
		return values
	}
	values := map[string]interface{}{}
	raw["values"] = values
	return values
}

// applyReadyClawProxyConfig 将 ReadyClaw 上游 LLM 地址改为本地保护代理入口。
func applyReadyClawProxyConfig(ctx *core.ProtectionContext) (map[string]interface{}, error) {
	if ctx == nil {
		return nil, fmt.Errorf("protection context is nil")
	}

	cfg, raw, configPath, err := loadConfig()
	if err != nil {
		return nil, err
	}

	protocol := strings.TrimSpace(valueString(cfg.Values, "LLM_PROTOCOL"))
	if protocol != "" && !strings.EqualFold(protocol, "openai") {
		return nil, fmt.Errorf("readyclaw protocol %q is not supported yet", protocol)
	}

	backupDir := getBackupDir(ctx.BackupDir)
	backupPath, err := ensureInitialBackup(configPath, backupDir)
	if err != nil {
		return nil, fmt.Errorf("backup failed: %w", err)
	}

	values := ensureValuesMap(raw)
	originalBaseURL := strings.TrimSpace(valueString(values, "LLM_BASE_URL"))
	proxyURL := buildReadyClawProxyURL(ctx.ProxyPort)
	values["LLM_BASE_URL"] = proxyURL

	if err := saveRawConfig(configPath, raw); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success":           true,
		"config_path":       configPath,
		"backup_path":       backupPath,
		"provider_name":     "openai",
		"original_base_url": originalBaseURL,
		"proxy_url":         proxyURL,
	}, nil
}

// restoreReadyClawConfig 使用初始备份恢复 ReadyClaw 原始配置。
func restoreReadyClawConfig(backupDir string) error {
	configPath, err := findConfigPath()
	if err != nil {
		return err
	}

	backupPath := getInitialBackupPath(getBackupDir(backupDir))
	content, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, content, 0644)
}

// resolveReadyClawForwardingTarget 从初始备份优先解析真实上游，避免代理自循环。
func resolveReadyClawForwardingTarget(backupDir string) (*core.ProxyForwardingTarget, error) {
	if target, err := resolveReadyClawForwardingTargetFromPath(getInitialBackupPath(getBackupDir(backupDir))); err == nil {
		return target, nil
	}

	_, _, configPath, err := loadConfig()
	if err != nil {
		return nil, err
	}
	return resolveReadyClawForwardingTargetFromPath(configPath)
}

// resolveReadyClawForwardingTargetFromPath 解析单个配置文件中的真实上游模型地址。
func resolveReadyClawForwardingTargetFromPath(path string) (*core.ProxyForwardingTarget, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg, _, err := parseReadyClawConfig(content)
	if err != nil {
		return nil, err
	}

	protocol := strings.TrimSpace(valueString(cfg.Values, "LLM_PROTOCOL"))
	if protocol == "" {
		protocol = "openai"
	}
	if !strings.EqualFold(protocol, "openai") {
		return nil, fmt.Errorf("readyclaw protocol %q is not supported yet", protocol)
	}

	baseURL := strings.TrimSpace(valueString(cfg.Values, "LLM_BASE_URL"))
	if baseURL == "" || isReadyClawProxyURL(baseURL) {
		return nil, fmt.Errorf("readyclaw upstream base URL is empty or already points to local proxy")
	}

	return &core.ProxyForwardingTarget{
		Provider: "openai",
		BaseURL:  baseURL,
		APIKey:   "",
	}, nil
}

// buildReadyClawProxyURL 与本机 ReadyClaw OpenAI provider 行为对齐，显式写入 chat completions 路径。
func buildReadyClawProxyURL(proxyPort int) string {
	return fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", proxyPort)
}

// isReadyClawProxyURL 判断当前配置是否已经指向本地代理，避免把代理地址当成真实上游。
func isReadyClawProxyURL(baseURL string) bool {
	normalized := strings.ToLower(strings.TrimSpace(baseURL))
	return strings.HasPrefix(normalized, "http://127.0.0.1:") ||
		strings.HasPrefix(normalized, "http://localhost:")
}

// valueString 兼容 ReadyClaw values 中 string 与非 string 简单值读取。
func valueString(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	value := values[key]
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprintf("%v", value)
	}
}
