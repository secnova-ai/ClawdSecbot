package nullclaw

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go_lib/core/logging"
)

// ensureMapValue ensures the value under key is a map and returns it.
func ensureMapValue(parent map[string]interface{}, key string) map[string]interface{} {
	if parent == nil {
		return map[string]interface{}{}
	}
	value, ok := parent[key].(map[string]interface{})
	if ok {
		return value
	}
	value = map[string]interface{}{}
	parent[key] = value
	return value
}

// readStringSlice converts a raw interface value into a string slice.
func readStringSlice(value interface{}) []string {
	rawList, ok := value.([]interface{})
	if !ok {
		return []string{}
	}
	result := make([]string, 0, len(rawList))
	for _, item := range rawList {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, strings.TrimSpace(text))
		}
	}
	return result
}

// writeStringSlice converts a string slice to an interface slice for JSON.
func writeStringSlice(values []string) []interface{} {
	result := make([]interface{}, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

// addUniqueString appends a value to slice if it does not already exist.
func addUniqueString(values []string, candidate string) []string {
	clean := strings.TrimSpace(candidate)
	if clean == "" {
		return values
	}
	for _, value := range values {
		if value == clean {
			return values
		}
	}
	return append(values, clean)
}

func providerNameFromBotConfig(botConfig *BotModelConfig) string {
	if botConfig == nil {
		return ""
	}
	normalized := normalizeProviderName(botConfig.Provider)
	switch normalized {
	case "claude":
		return "anthropic"
	case "gemini":
		return "google"
	case "zhipu":
		return "zai"
	default:
		return normalized
	}
}

func extractRealModelID(baseModel, providerName string) string {
	baseModel = strings.TrimSpace(baseModel)
	providerName = strings.TrimSpace(providerName)
	if baseModel == "" {
		return ""
	}
	prefix := providerName + "/"
	if providerName != "" && strings.HasPrefix(strings.ToLower(baseModel), strings.ToLower(prefix)) {
		return strings.TrimSpace(baseModel[len(prefix):])
	}
	return baseModel
}

// buildBotModelIdentity builds provider and model identity for nullclaw config.
// Returns: providerName, realModelID, fullPrimaryModel(provider/model).
func buildBotModelIdentity(botConfig *BotModelConfig) (string, string, string, error) {
	if botConfig == nil {
		return "", "", "", fmt.Errorf("bot config is nil")
	}
	modelID := strings.TrimSpace(botConfig.Model)
	if modelID == "" {
		return "", "", "", fmt.Errorf("bot model name is empty")
	}

	providerName := providerNameFromBotConfig(botConfig)
	if providerName == "" {
		providerName = normalizeProviderName(extractProviderFromModel(modelID))
	}
	if providerName == "" {
		providerName = normalizeProviderName(inferProviderFromModel(modelID))
	}
	if providerName == "" {
		return "", "", "", fmt.Errorf("provider name is empty")
	}

	realModelID := extractRealModelID(modelID, providerName)
	if realModelID == "" {
		return "", "", "", fmt.Errorf("real model id is empty")
	}

	newPrimary := providerName + "/" + realModelID
	return providerName, realModelID, newPrimary, nil
}

// updateAgentsDefaultsFallbacks updates agents.defaults.model.fallbacks.
func updateAgentsDefaultsFallbacks(rawConfig map[string]interface{}, fallbackModel string) (interface{}, []string, error) {
	if strings.TrimSpace(fallbackModel) == "" {
		return nil, nil, fmt.Errorf("fallback model is empty")
	}
	agentsMap := ensureMapValue(rawConfig, "agents")
	defaultsMap := ensureMapValue(agentsMap, "defaults")
	modelValue := defaultsMap["model"]
	modelMap, ok := modelValue.(map[string]interface{})
	if !ok {
		modelMap = map[string]interface{}{}
		if modelStr, ok := modelValue.(string); ok && strings.TrimSpace(modelStr) != "" {
			modelMap["primary"] = strings.TrimSpace(modelStr)
		}
	}
	fallbacksValue := modelMap["fallbacks"]
	if fallbacksValue == nil {
		fallbacksValue = defaultsMap["fallbacks"]
	}
	fallbacks := readStringSlice(fallbacksValue)
	fallbacks = addUniqueString(fallbacks, fallbackModel)
	modelMap["fallbacks"] = writeStringSlice(fallbacks)
	defaultsMap["model"] = modelMap
	agentsMap["defaults"] = defaultsMap
	rawConfig["agents"] = agentsMap
	return fallbacksValue, fallbacks, nil
}

// ensureProviderModelEntry ensures provider models list has exactly the target model.
func ensureProviderModelEntry(providerMap map[string]interface{}, modelID string) []interface{} {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil
	}

	modelsValue := []interface{}{
		map[string]interface{}{
			"id":   modelID,
			"name": modelID,
		},
	}
	providerMap["models"] = modelsValue
	return modelsValue
}

func cloneStringInterfaceMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// ensureProviderForBotModel ensures models.providers has bot provider config.
func ensureProviderForBotModel(rawConfig map[string]interface{}, botConfig *BotModelConfig, providerName string, modelID string) (map[string]interface{}, map[string]interface{}, error) {
	providerName = normalizeProviderName(providerName)
	if providerName == "" {
		return nil, nil, fmt.Errorf("provider name is empty")
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, nil, fmt.Errorf("bot model is empty")
	}

	modelsMap := ensureMapValue(rawConfig, "models")
	providersMap := ensureMapValue(modelsMap, "providers")

	previousProvider := map[string]interface{}{}
	if existing, ok := providersMap[providerName].(map[string]interface{}); ok {
		previousProvider = cloneStringInterfaceMap(existing)
	}

	providerMap := buildDefaultProviderConfig(providerName, modelID)
	if len(previousProvider) > 0 {
		for key, value := range previousProvider {
			providerMap[key] = value
		}
	}

	// Keep provider model list aligned with the active bot model.
	ensureProviderModelEntry(providerMap, modelID)

	if botConfig != nil {
		if baseURL := strings.TrimSpace(botConfig.BaseURL); baseURL != "" {
			providerMap["base_url"] = baseURL
		}
		if apiKey := strings.TrimSpace(botConfig.APIKey); apiKey != "" {
			providerMap["api_key"] = apiKey
		}
	}

	if _, ok := providerMap["base_url"]; !ok {
		if defaultBaseURL := getDefaultBaseURL(providerName); defaultBaseURL != "" {
			providerMap["base_url"] = defaultBaseURL
		}
	}

	providersMap[providerName] = providerMap
	modelsMap["providers"] = providersMap
	rawConfig["models"] = modelsMap
	return previousProvider, providerMap, nil
}

// updateAgentsDefaultsModels updates agents.defaults.models when non-empty.
func updateAgentsDefaultsModels(rawConfig map[string]interface{}, newModel string) (interface{}, interface{}, error) {
	agentsMap := ensureMapValue(rawConfig, "agents")
	defaultsMap := ensureMapValue(agentsMap, "defaults")
	modelsValue := defaultsMap["models"]
	switch modelsValue := modelsValue.(type) {
	case []interface{}:
		modelList := readStringSlice(modelsValue)
		if len(modelList) == 0 {
			return modelsValue, modelsValue, nil
		}
		modelList = addUniqueString(modelList, newModel)
		defaultsMap["models"] = writeStringSlice(modelList)
		agentsMap["defaults"] = defaultsMap
		rawConfig["agents"] = agentsMap
		return modelsValue, defaultsMap["models"], nil
	case map[string]interface{}:
		if len(modelsValue) == 0 {
			return modelsValue, modelsValue, nil
		}
		if _, exists := modelsValue[newModel]; !exists {
			modelsValue[newModel] = map[string]interface{}{}
		}
		defaultsMap["models"] = modelsValue
		agentsMap["defaults"] = defaultsMap
		rawConfig["agents"] = agentsMap
		return modelsValue, defaultsMap["models"], nil
	default:
		return nil, nil, nil
	}
}

// setAgentsDefaultsPrimary updates agents.defaults.model.primary.
func setAgentsDefaultsPrimary(rawConfig map[string]interface{}, newModel string) (interface{}, interface{}, error) {
	if strings.TrimSpace(newModel) == "" {
		return nil, nil, fmt.Errorf("primary model is empty")
	}
	agentsMap := ensureMapValue(rawConfig, "agents")
	defaultsMap := ensureMapValue(agentsMap, "defaults")
	modelValue := defaultsMap["model"]
	modelMap, ok := modelValue.(map[string]interface{})
	if !ok {
		modelMap = map[string]interface{}{}
		if modelStr, ok := modelValue.(string); ok && strings.TrimSpace(modelStr) != "" {
			modelMap["fallbacks"] = []interface{}{strings.TrimSpace(modelStr)}
		}
	}
	previousPrimary := modelMap["primary"]
	modelMap["primary"] = newModel
	defaultsMap["model"] = modelMap
	agentsMap["defaults"] = defaultsMap
	rawConfig["agents"] = agentsMap
	return previousPrimary, newModel, nil
}

// buildDefaultProviderConfig constructs a provider config with defaults.
func buildDefaultProviderConfig(providerName string, modelID string) map[string]interface{} {
	provider := map[string]interface{}{}
	if defaultBaseURL := getDefaultBaseURL(providerName); defaultBaseURL != "" {
		provider["base_url"] = defaultBaseURL
	}
	if strings.TrimSpace(modelID) != "" {
		provider["models"] = []interface{}{
			map[string]interface{}{
				"id":   modelID,
				"name": modelID,
			},
		}
	}
	return provider
}

// ==================== 配置备份/恢复 ====================

// InitialBackupFileName 初始备份文件名（固定名称，只备份一次）
const InitialBackupFileName = "config.json.initial"

func backupNullclawConfigOnce(configPath string, backupDir string) (string, bool, error) {
	if strings.TrimSpace(backupDir) == "" {
		return "", false, fmt.Errorf("backup directory is empty")
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", false, fmt.Errorf("create backup directory failed: %w", err)
	}

	initialBackupPath := filepath.Join(backupDir, InitialBackupFileName)
	if _, err := os.Stat(initialBackupPath); err == nil {
		return initialBackupPath, false, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", false, fmt.Errorf("read config failed: %w", err)
	}
	if err := os.WriteFile(initialBackupPath, data, 0600); err != nil {
		return "", false, fmt.Errorf("write initial backup failed: %w", err)
	}

	return initialBackupPath, true, nil
}

func hasInitialBackup(backupDir string) bool {
	if strings.TrimSpace(backupDir) == "" {
		return false
	}
	initialBackupPath := filepath.Join(backupDir, InitialBackupFileName)
	_, err := os.Stat(initialBackupPath)
	return err == nil
}

func getInitialBackupPath(backupDir string) string {
	return filepath.Join(backupDir, InitialBackupFileName)
}

// backupNullclawConfig keeps old function name for compatibility.
func backupNullclawConfig(configPath string, backupDir string) (string, error) {
	backupPath, _, err := backupNullclawConfigOnce(configPath, backupDir)
	return backupPath, err
}

// findLatestNullclawBackup returns the latest backup path if it exists.
func findLatestNullclawBackup(backupDir string) (string, bool, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return "", false, fmt.Errorf("read backup directory failed: %w", err)
	}
	var backupNames []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "config.json.bak-") {
			backupNames = append(backupNames, name)
		}
	}
	if len(backupNames) == 0 {
		return "", false, nil
	}
	sort.Slice(backupNames, func(i, j int) bool {
		return backupNames[i] > backupNames[j]
	})
	return filepath.Join(backupDir, backupNames[0]), true, nil
}

func isFileContentSameMD5(firstPath string, secondPath string) (bool, error) {
	firstHash, err := computeFileMD5(firstPath)
	if err != nil {
		return false, err
	}
	secondHash, err := computeFileMD5(secondPath)
	if err != nil {
		return false, err
	}
	return firstHash == secondHash, nil
}

func computeFileMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file failed: %w", err)
	}
	defer file.Close()
	hasher := md5.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("hash file failed: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func trimNullclawBackups(backupDir string, maxCount int) error {
	if maxCount <= 0 {
		return fmt.Errorf("invalid backup limit: %d", maxCount)
	}
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("read backup directory failed: %w", err)
	}
	var backupNames []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "config.json.bak-") {
			backupNames = append(backupNames, name)
		}
	}
	if len(backupNames) <= maxCount {
		return nil
	}
	sort.Slice(backupNames, func(i, j int) bool {
		return backupNames[i] > backupNames[j]
	})
	for _, name := range backupNames[maxCount:] {
		backupPath := filepath.Join(backupDir, name)
		if err := os.Remove(backupPath); err != nil {
			return fmt.Errorf("remove backup failed: %w", err)
		}
	}
	return nil
}

func restoreNullclawConfig(configPath string, backupPath string) error {
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("read backup failed: %w", err)
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("restore config failed: %w", err)
	}
	return nil
}

// RestoreToInitialConfigResult 恢复初始配置的结果
type RestoreToInitialConfigResult struct {
	Success        bool   `json:"success"`
	Message        string `json:"message,omitempty"`
	Error          string `json:"error,omitempty"`
	ConfigPath     string `json:"config_path,omitempty"`
	BackupPath     string `json:"backup_path,omitempty"`
	GatewayRestart bool   `json:"gateway_restart,omitempty"`
}

// RestoreToInitialConfigByAsset restores config.json to initial backup and restarts gateway.
func RestoreToInitialConfigByAsset(backupDir string, assetID string) RestoreToInitialConfigResult {
	logging.Info("[RestoreToInitialConfig] Starting restore to initial config...")

	if !hasInitialBackup(backupDir) {
		return RestoreToInitialConfigResult{Success: false, Error: "initial backup not found"}
	}

	configPath, err := findConfigPath()
	if err != nil {
		return RestoreToInitialConfigResult{Success: false, Error: fmt.Sprintf("config not found: %v", err)}
	}

	initialBackupPath := getInitialBackupPath(backupDir)
	if err := restoreNullclawConfig(configPath, initialBackupPath); err != nil {
		return RestoreToInitialConfigResult{
			Success:    false,
			Error:      fmt.Sprintf("restore failed: %v", err),
			ConfigPath: configPath,
			BackupPath: initialBackupPath,
		}
	}
	logging.Info("[RestoreToInitialConfig] Config restored from %s to %s", initialBackupPath, configPath)

	req := &GatewayRestartRequest{
		AssetName:      nullclawAssetName,
		AssetID:        strings.TrimSpace(assetID),
		SandboxEnabled: false,
	}

	_, gatewayErr := restartNullclawGateway(req)
	gatewayRestart := gatewayErr == nil
	if gatewayErr != nil {
		logging.Warning("[RestoreToInitialConfig] Gateway restart failed: %v", gatewayErr)
	}

	return RestoreToInitialConfigResult{
		Success:        true,
		Message:        "config restored to initial state",
		ConfigPath:     configPath,
		BackupPath:     initialBackupPath,
		GatewayRestart: gatewayRestart,
	}
}

// RestoreToInitialConfig restores config to initial backup (compat wrapper).
func RestoreToInitialConfig(backupDir string) RestoreToInitialConfigResult {
	return RestoreToInitialConfigByAsset(backupDir, "")
}

// HasInitialBackup checks whether initial backup exists.
func HasInitialBackup(backupDir string) bool {
	return hasInitialBackup(backupDir)
}
