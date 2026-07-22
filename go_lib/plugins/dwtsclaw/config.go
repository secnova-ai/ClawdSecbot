package dwtsclaw

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"go_lib/core"
)

const dwtsclawInitialBackupPrefix = "dwtsclaw-openclaw"

var (
	dwtsclawConfigPathOverride string
	dwtsclawUserHomeDir        = os.UserHomeDir
)

func findConfigPath() (string, error) {
	if path := strings.TrimSpace(dwtsclawConfigPathOverride); path != "" {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return "", fmt.Errorf("dwtsclaw config not found: %s", path)
		}
		return path, nil
	}

	homeDir, err := dwtsclawUserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		return "", fmt.Errorf("resolve user home for dwtsclaw config: %w", err)
	}
	configPath := filepath.Join(homeDir, ".openclaw", "openclaw.json")
	if _, err := os.Stat(configPath); err != nil {
		return "", fmt.Errorf("dwtsclaw config not found: %w", err)
	}
	return configPath, nil
}

func loadDWTSClawConfig() (map[string]interface{}, string, error) {
	configPath, err := findConfigPath()
	if err != nil {
		return nil, "", err
	}
	raw, err := loadRawConfig(configPath)
	if err != nil {
		return nil, "", err
	}
	return raw, configPath, nil
}

func loadRawConfig(path string) (map[string]interface{}, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		raw = map[string]interface{}{}
	}
	return raw, nil
}

func writeDWTSClawConfig(path string, raw map[string]interface{}) error {
	if raw == nil {
		return fmt.Errorf("dwtsclaw config is empty")
	}
	content, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return writeFileAtomically(path, content)
}

func writeFileAtomically(path string, content []byte) error {
	dir := filepath.Dir(path)
	info, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	mode := os.FileMode(0600)
	if err == nil {
		mode = info.Mode()
	}
	temp, err := os.CreateTemp(dir, ".dwtsclaw-openclaw-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func getBackupDir(explicit string) string {
	if dir := strings.TrimSpace(explicit); dir != "" {
		return dir
	}
	homeDir, err := dwtsclawUserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".botsec", "backups", "dwtsclaw")
}

func initialBackupPath(backupDir, configPath string) string {
	return filepath.Join(backupDir, backupFileStem(configPath)+".initial.json")
}

func proxyInjectionStatePath(backupDir, configPath string) string {
	return filepath.Join(backupDir, backupFileStem(configPath)+".proxy-state.json")
}

func backupFileStem(configPath string) string {
	fingerprint := core.ResolveStableConfigPathFingerprint(configPath)
	digest := sha256.Sum256([]byte(fingerprint))
	return fmt.Sprintf("%s-%x", dwtsclawInitialBackupPrefix, digest[:8])
}

// proxyInjectionState is deliberately limited to local routing metadata. It
// never contains the provider API key or the original upstream URL.
type proxyInjectionState struct {
	ProviderName string `json:"provider_name"`
	ProxyURL     string `json:"proxy_url"`
}

func ensureInitialBackups(configPath, explicitBackupDir string) (string, error) {
	backupDirs := backupDirectories(explicitBackupDir)
	if len(backupDirs) == 0 {
		return "", fmt.Errorf("backup directory is empty")
	}

	primaryPath := initialBackupPath(backupDirs[0], configPath)
	content, err := os.ReadFile(primaryPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		content, err = os.ReadFile(configPath)
		if err != nil {
			return "", err
		}
		if err := writeInitialBackup(primaryPath, content); err != nil {
			return "", err
		}
	}

	for _, backupDir := range backupDirs[1:] {
		mirrorPath := initialBackupPath(backupDir, configPath)
		if _, err := os.Stat(mirrorPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if err := writeInitialBackup(mirrorPath, content); err != nil {
			return "", err
		}
	}
	return primaryPath, nil
}

func writeInitialBackup(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0600)
}

func backupDirectories(explicit string) []string {
	dirs := make([]string, 0, 2)
	for _, dir := range []string{explicit, getBackupDir("")} {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		alreadyAdded := false
		for _, existing := range dirs {
			if existing == dir {
				alreadyAdded = true
				break
			}
		}
		if !alreadyAdded {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

func loadInitialBackup(configPath, explicitBackupDir string) (map[string]interface{}, string, error) {
	for _, backupDir := range backupDirectories(explicitBackupDir) {
		backupPath := initialBackupPath(backupDir, configPath)
		raw, err := loadRawConfig(backupPath)
		if err == nil {
			return raw, backupPath, nil
		}
		if !os.IsNotExist(err) {
			return nil, "", err
		}
	}
	return nil, "", os.ErrNotExist
}

func writeProxyInjectionState(configPath, explicitBackupDir, providerName, proxyURL string) error {
	state := proxyInjectionState{
		ProviderName: strings.TrimSpace(providerName),
		ProxyURL:     strings.TrimSpace(proxyURL),
	}
	if state.ProviderName == "" || state.ProxyURL == "" {
		return fmt.Errorf("dwtsclaw proxy injection state is incomplete")
	}
	content, err := json.Marshal(state)
	if err != nil {
		return err
	}
	content = append(content, '\n')
	for _, backupDir := range backupDirectories(explicitBackupDir) {
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			return err
		}
		if err := writeFileAtomically(proxyInjectionStatePath(backupDir, configPath), content); err != nil {
			return err
		}
	}
	return nil
}

func loadProxyInjectionState(configPath, explicitBackupDir string) (proxyInjectionState, error) {
	for _, backupDir := range backupDirectories(explicitBackupDir) {
		content, err := os.ReadFile(proxyInjectionStatePath(backupDir, configPath))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return proxyInjectionState{}, err
		}
		var state proxyInjectionState
		if err := json.Unmarshal(content, &state); err != nil {
			return proxyInjectionState{}, err
		}
		state.ProviderName = strings.TrimSpace(state.ProviderName)
		state.ProxyURL = strings.TrimSpace(state.ProxyURL)
		if state.ProviderName == "" || state.ProxyURL == "" {
			return proxyInjectionState{}, fmt.Errorf("dwtsclaw proxy injection state is incomplete")
		}
		return state, nil
	}
	return proxyInjectionState{}, os.ErrNotExist
}

func removeProxyInjectionState(configPath, explicitBackupDir string) error {
	var firstErr error
	for _, backupDir := range backupDirectories(explicitBackupDir) {
		err := os.Remove(proxyInjectionStatePath(backupDir, configPath))
		if err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func applyProxyConfig(ctx *core.ProtectionContext) (map[string]interface{}, error) {
	if ctx == nil {
		return nil, fmt.Errorf("protection context is nil")
	}
	if ctx.ProxyPort < 1 || ctx.ProxyPort > 65535 {
		return nil, fmt.Errorf("invalid proxy port: %d", ctx.ProxyPort)
	}

	raw, configPath, err := loadDWTSClawConfig()
	if err != nil {
		return nil, err
	}
	providerName, provider, err := selectForwardingProvider(raw)
	if err != nil {
		return nil, err
	}
	originalBaseURL := mapString(provider, "baseUrl")
	if originalBaseURL == "" {
		return nil, fmt.Errorf("dwtsclaw provider %q has no baseUrl", providerName)
	}

	backupPath, err := ensureInitialBackups(configPath, ctx.BackupDir)
	if err != nil {
		return nil, fmt.Errorf("backup failed: %w", err)
	}
	proxyURL := buildProxyURL(ctx.ProxyPort)
	if err := writeProxyInjectionState(configPath, ctx.BackupDir, providerName, proxyURL); err != nil {
		return nil, fmt.Errorf("record proxy injection state: %w", err)
	}
	patch := buildProviderBaseURLPatch(providerName, proxyURL)
	if err := gatewayConfigPatcher.Patch(configPath, patch); err == nil {
		return map[string]interface{}{
			"success":           true,
			"method":            "gateway_config_patch",
			"gateway_reload":    "hot",
			"config_path":       configPath,
			"backup_path":       backupPath,
			"provider_name":     providerName,
			"original_base_url": originalBaseURL,
			"proxy_url":         proxyURL,
		}, nil
	}

	provider["baseUrl"] = proxyURL
	if err := writeDWTSClawConfig(configPath, raw); err != nil {
		_ = removeProxyInjectionState(configPath, ctx.BackupDir)
		return nil, err
	}
	return map[string]interface{}{
		"success":           true,
		"method":            "direct_file_update",
		"gateway_reload":    "pending",
		"config_path":       configPath,
		"backup_path":       backupPath,
		"provider_name":     providerName,
		"original_base_url": originalBaseURL,
		"proxy_url":         proxyURL,
	}, nil
}

func restoreProxyConfig(ctx *core.ProtectionContext) error {
	raw, configPath, err := loadDWTSClawConfig()
	if err != nil {
		return err
	}
	backupDir := ""
	if ctx != nil {
		backupDir = ctx.BackupDir
	}
	backupRaw, _, err := loadInitialBackup(configPath, backupDir)
	if err != nil {
		return err
	}
	state, err := loadProxyInjectionState(configPath, backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	providers, ok := providersMap(raw)
	if !ok {
		return nil
	}
	backupProviders, ok := providersMap(backupRaw)
	if !ok {
		return nil
	}

	provider, ok := providers[state.ProviderName].(map[string]interface{})
	if !ok {
		return removeProxyInjectionState(configPath, backupDir)
	}
	backupProvider, ok := backupProviders[state.ProviderName].(map[string]interface{})
	if !ok {
		return removeProxyInjectionState(configPath, backupDir)
	}
	if mapString(provider, "baseUrl") != state.ProxyURL {
		return removeProxyInjectionState(configPath, backupDir)
	}
	originalBaseURL := mapString(backupProvider, "baseUrl")
	if originalBaseURL == "" {
		return fmt.Errorf("dwtsclaw backup provider %q has no baseUrl", state.ProviderName)
	}

	patch := buildProviderBaseURLPatch(state.ProviderName, originalBaseURL)
	if err := gatewayConfigPatcher.Patch(configPath, patch); err == nil {
		return removeProxyInjectionState(configPath, backupDir)
	}
	provider["baseUrl"] = originalBaseURL
	if err := writeDWTSClawConfig(configPath, raw); err != nil {
		return err
	}
	return removeProxyInjectionState(configPath, backupDir)
}

func resolveProxyForwardingTarget(backupDir string) (*core.ProxyForwardingTarget, error) {
	_, configPath, err := loadDWTSClawConfig()
	if err != nil {
		return nil, err
	}
	backupDirs := backupDirectories(backupDir)
	paths := make([]string, 0, len(backupDirs)+1)
	for _, backupDir := range backupDirs {
		paths = append(paths, initialBackupPath(backupDir, configPath))
	}
	paths = append(paths, configPath)
	for _, path := range paths {
		raw, err := loadRawConfig(path)
		if err != nil {
			continue
		}
		_, provider, err := selectForwardingProvider(raw)
		if err != nil {
			continue
		}
		baseURL := mapString(provider, "baseUrl")
		if baseURL == "" || isDWTSClawLocalProxyURL(baseURL) {
			continue
		}
		return &core.ProxyForwardingTarget{
			Provider: "openai",
			BaseURL:  baseURL,
			APIKey:   mapString(provider, "apiKey"),
		}, nil
	}
	return nil, fmt.Errorf("dwtsclaw upstream provider is empty or already points to the local proxy")
}

func selectForwardingProvider(raw map[string]interface{}) (string, map[string]interface{}, error) {
	providers, ok := providersMap(raw)
	if !ok || len(providers) == 0 {
		return "", nil, fmt.Errorf("dwtsclaw models.providers is empty")
	}
	primary := readPrimaryModel(raw)
	if primary != "" {
		if providerName := providerNameFromPrimary(primary, providers); providerName != "" {
			provider, ok := providers[providerName].(map[string]interface{})
			if ok {
				return providerName, provider, nil
			}
		}
	}
	if len(providers) == 1 {
		for providerName, value := range providers {
			provider, ok := value.(map[string]interface{})
			if ok {
				return providerName, provider, nil
			}
		}
	}
	return "", nil, fmt.Errorf("cannot identify active DWTSClaw provider from agents.defaults.model")
}

func providersMap(raw map[string]interface{}) (map[string]interface{}, bool) {
	models, ok := raw["models"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	providers, ok := models["providers"].(map[string]interface{})
	return providers, ok
}

func readPrimaryModel(raw map[string]interface{}) string {
	agents, ok := raw["agents"].(map[string]interface{})
	if !ok {
		return ""
	}
	defaults, ok := agents["defaults"].(map[string]interface{})
	if !ok {
		return ""
	}
	switch model := defaults["model"].(type) {
	case string:
		return strings.TrimSpace(model)
	case map[string]interface{}:
		return mapString(model, "primary")
	default:
		return ""
	}
}

func providerNameFromPrimary(primary string, providers map[string]interface{}) string {
	primary = strings.TrimSpace(primary)
	if primary == "" {
		return ""
	}
	if slash := strings.Index(primary, "/"); slash > 0 {
		providerName := primary[:slash]
		if _, ok := providers[providerName]; ok {
			return providerName
		}
	}
	if _, ok := providers[primary]; ok {
		return primary
	}

	matches := make([]string, 0, 1)
	for providerName, value := range providers {
		provider, ok := value.(map[string]interface{})
		if !ok || !providerContainsModel(provider, primary) {
			continue
		}
		matches = append(matches, providerName)
	}
	if len(matches) == 1 {
		return matches[0]
	}
	return ""
}

func providerContainsModel(provider map[string]interface{}, modelID string) bool {
	models, ok := provider["models"].([]interface{})
	if !ok {
		return false
	}
	for _, value := range models {
		switch model := value.(type) {
		case string:
			if strings.TrimSpace(model) == modelID {
				return true
			}
		case map[string]interface{}:
			if mapString(model, "id") == modelID {
				return true
			}
		}
	}
	return false
}

func mapString(values map[string]interface{}, key string) string {
	value := values[key]
	stringValue, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(stringValue)
}

func buildProviderBaseURLPatch(providerName, baseURL string) map[string]interface{} {
	return map[string]interface{}{
		"models": map[string]interface{}{
			"providers": map[string]interface{}{
				providerName: map[string]interface{}{"baseUrl": baseURL},
			},
		},
	}
}

func buildProxyURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d/v1", port)
}

func isDWTSClawLocalProxyURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return false
	}
	if parsed.Port() == "" {
		return false
	}
	_, err = strconv.Atoi(parsed.Port())
	return err == nil && strings.HasPrefix(strings.TrimRight(parsed.Path, "/"), "/v1")
}

func readGatewayPort(configPath string) int {
	content, err := os.ReadFile(filepath.Join(filepath.Dir(configPath), "gateway-port.json"))
	if err != nil {
		return 0
	}
	var portFile struct {
		Port int `json:"port"`
	}
	if err := json.Unmarshal(content, &portFile); err != nil || portFile.Port < 1 || portFile.Port > 65535 {
		return 0
	}
	return portFile.Port
}

func sortedStrings(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			unique[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
