package openclaw

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

const clawdsecbotModelPrefix = "clawdsecbot-"

// proxyInjectedAPIKey is a placeholder written to bot config files.
// The real API key is injected by the LLM proxy at forwarding time
// and must never appear in bot config files.
const proxyInjectedAPIKey = "botsec-proxy-key"

// stripClawdsecbotPrefix removes clawdsecbot- prefix from a model ID.
func stripClawdsecbotPrefix(modelID string) string {
	trimmed := strings.TrimSpace(modelID)
	if strings.HasPrefix(trimmed, clawdsecbotModelPrefix) {
		return strings.TrimPrefix(trimmed, clawdsecbotModelPrefix)
	}
	return trimmed
}

func hasClawdsecbotPrefix(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), clawdsecbotModelPrefix)
}

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
			result = append(result, text)
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

// parsePrimaryModelFromConfig extracts base and new model IDs.
func parsePrimaryModelFromConfig(config *OpenclawConfig) (string, string, error) {
	primaryModel, err := getPrimaryModelFromConfig(config)
	if err != nil {
		return "", "", err
	}
	baseModel := stripClawdsecbotPrefix(primaryModel)
	if baseModel == "" {
		return "", "", fmt.Errorf("primary model is empty")
	}
	newModel := fmt.Sprintf("clawdsecbot-%s", baseModel)
	return baseModel, newModel, nil
}

// providerNameFromBotConfig maps bot model provider to provider name.
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

// buildBotModelIdentity builds provider name and model IDs from bot config.
func buildBotModelIdentity(botConfig *BotModelConfig) (string, string, string, error) {
	if botConfig == nil {
		return "", "", "", fmt.Errorf("bot config is nil")
	}
	modelID := strings.TrimSpace(botConfig.Model)
	if modelID == "" {
		return "", "", "", fmt.Errorf("bot model name is empty")
	}
	providerName := providerNameFromBotConfig(botConfig)

	// Provider 必须在配置中明确指定
	// 不再默认使用 openai 以避免配置错误

	providerName = normalizeProviderName(providerName)
	if providerName == "" {
		return "", "", "", fmt.Errorf("provider name is empty")
	}

	// baseModel 应为用户配置的原始模型 ID，不做修改或添加前缀
	baseModel := modelID

	// newModel 是用于路由的内部 ID，必须包含 provider 前缀
	// 以确保 GetCurrentModelProvider 能正确识别 provider
	// 格式: clawdsecbot-{provider}/{baseModel}
	newModel := fmt.Sprintf("clawdsecbot-%s/%s", providerName, baseModel)

	return providerName, baseModel, newModel, nil
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
			modelMap["primary"] = modelStr
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

// ensureProviderModelEntry ensures provider models list has the bot model.
// 用指定的 modelID 替换已有的 models 列表
func ensureProviderModelEntry(providerMap map[string]interface{}, modelID string) []interface{} {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil
	}

	// 创建仅包含单个模型条目的 models 列表
	modelsValue := []interface{}{
		map[string]interface{}{
			"id":   modelID,
			"name": modelID,
		},
	}

	// 替换已有列表
	providerMap["models"] = modelsValue
	return modelsValue
}

// extractRealModelID extracts the real model ID from baseModel by stripping provider prefix.
func extractRealModelID(baseModel, providerName string) string {
	prefix := providerName + "/"
	if len(baseModel) > len(prefix) && strings.EqualFold(baseModel[:len(prefix)], prefix) {
		return baseModel[len(prefix):]
	}
	// 前缀不匹配时直接信任输入就是真实模型 ID
	return baseModel
}

// ensureProviderForBotModel ensures models.providers has bot provider config.
func ensureProviderForBotModel(rawConfig map[string]interface{}, botConfig *BotModelConfig, providerName string, baseModel string) (map[string]interface{}, map[string]interface{}, error) {
	if strings.TrimSpace(providerName) == "" {
		return nil, nil, fmt.Errorf("provider name is empty")
	}
	if strings.TrimSpace(baseModel) == "" {
		return nil, nil, fmt.Errorf("bot model is empty")
	}

	realModelID := extractRealModelID(baseModel, providerName)
	if realModelID == "" {
		return nil, nil, fmt.Errorf("real model id extraction failed")
	}

	// 使用 clawdsecbot-{provider} 作为 key，与 primary model 引用对齐
	providerKey := fmt.Sprintf("clawdsecbot-%s", providerName)
	modelsMap := ensureMapValue(rawConfig, "models")
	providersMap := ensureMapValue(modelsMap, "providers")

	// 记录之前的 provider 状态用于日志
	previousProvider := map[string]interface{}{}
	if existing, ok := providersMap[providerKey].(map[string]interface{}); ok {
		for key, value := range existing {
			previousProvider[key] = value
		}
	}

	// 始终构建全新的 provider 配置，确保覆盖已有状态
	providerMap := buildDefaultProviderConfig(providerName, realModelID)

	// Proxy injects the real API key when forwarding LLM requests.
	// The value here is just a placeholder to satisfy OpenClaw config validation.
	providerMap["apiKey"] = proxyInjectedAPIKey

	providersMap[providerKey] = providerMap
	modelsMap["providers"] = providersMap
	rawConfig["models"] = modelsMap
	return previousProvider, providerMap, nil
}

// extractProviderFromDefaultsPrimary extracts provider from agents.defaults.model.primary.
func extractProviderFromDefaultsPrimary(rawConfig map[string]interface{}) string {
	agentsMap, ok := rawConfig["agents"].(map[string]interface{})
	if !ok {
		return ""
	}
	defaultsMap, ok := agentsMap["defaults"].(map[string]interface{})
	if !ok {
		return ""
	}
	modelValue, exists := defaultsMap["model"]
	if !exists {
		return ""
	}
	switch modelValue := modelValue.(type) {
	case string:
		if strings.TrimSpace(modelValue) == "" {
			return ""
		}
		return extractProviderFromModel(modelValue)
	case map[string]interface{}:
		if primary, ok := modelValue["primary"].(string); ok && strings.TrimSpace(primary) != "" {
			return extractProviderFromModel(primary)
		}
	}
	return ""
}

// normalizeTargetProviderName trims and normalizes provider for baseUrl updates.
func normalizeTargetProviderName(providerName string) string {
	normalized := normalizeProviderName(providerName)
	const sectorPrefix = "clawdsector-"
	if strings.HasPrefix(normalized, sectorPrefix) {
		return strings.TrimPrefix(normalized, sectorPrefix)
	}
	return normalized
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
	}
	previousPrimary := modelMap["primary"]
	modelMap["primary"] = newModel
	defaultsMap["model"] = modelMap
	agentsMap["defaults"] = defaultsMap
	rawConfig["agents"] = agentsMap
	return previousPrimary, newModel, nil
}

func readMapValue(parent map[string]interface{}, key string) (map[string]interface{}, bool) {
	if parent == nil {
		return nil, false
	}
	value, ok := parent[key].(map[string]interface{})
	return value, ok
}

func containsString(values []string, candidate string) bool {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == candidate {
			return true
		}
	}
	return false
}

func lastString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[len(values)-1])
}

func collectModelKeys(value interface{}) []string {
	switch typed := value.(type) {
	case []interface{}:
		return readStringSlice(typed)
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			clean := strings.TrimSpace(key)
			if clean != "" {
				keys = append(keys, clean)
			}
		}
		sort.Strings(keys)
		return keys
	default:
		return nil
	}
}

func removeInjectedProviderEntries(rawConfig map[string]interface{}) []string {
	modelsMap, ok := readMapValue(rawConfig, "models")
	if !ok {
		return nil
	}
	providersMap, ok := readMapValue(modelsMap, "providers")
	if !ok {
		return nil
	}

	removed := make([]string, 0)
	for key := range providersMap {
		if hasClawdsecbotPrefix(key) {
			removed = append(removed, strings.TrimSpace(key))
			delete(providersMap, key)
		}
	}
	if len(removed) == 0 {
		return nil
	}

	sort.Strings(removed)
	modelsMap["providers"] = providersMap
	rawConfig["models"] = modelsMap
	return removed
}

func removeInjectedDefaultsModels(rawConfig map[string]interface{}) ([]string, []string) {
	agentsMap, ok := readMapValue(rawConfig, "agents")
	if !ok {
		return nil, nil
	}
	defaultsMap, ok := readMapValue(agentsMap, "defaults")
	if !ok {
		return nil, nil
	}

	removed := make([]string, 0)
	switch modelsValue := defaultsMap["models"].(type) {
	case []interface{}:
		models := readStringSlice(modelsValue)
		remaining := make([]string, 0, len(models))
		for _, model := range models {
			if hasClawdsecbotPrefix(model) {
				removed = append(removed, model)
				continue
			}
			remaining = append(remaining, model)
		}
		if len(removed) == 0 {
			return nil, remaining
		}
		defaultsMap["models"] = writeStringSlice(remaining)
		agentsMap["defaults"] = defaultsMap
		rawConfig["agents"] = agentsMap
		return removed, remaining
	case map[string]interface{}:
		for key := range modelsValue {
			if hasClawdsecbotPrefix(key) {
				removed = append(removed, strings.TrimSpace(key))
				delete(modelsValue, key)
			}
		}
		remaining := collectModelKeys(modelsValue)
		if len(removed) == 0 {
			return nil, remaining
		}
		sort.Strings(removed)
		defaultsMap["models"] = modelsValue
		agentsMap["defaults"] = defaultsMap
		rawConfig["agents"] = agentsMap
		return removed, remaining
	default:
		return nil, nil
	}
}

func removeInjectedFallbacks(rawConfig map[string]interface{}) ([]string, []string) {
	agentsMap, ok := readMapValue(rawConfig, "agents")
	if !ok {
		return nil, nil
	}
	defaultsMap, ok := readMapValue(agentsMap, "defaults")
	if !ok {
		return nil, nil
	}

	modelMap, modelAsMap := defaultsMap["model"].(map[string]interface{})
	targetParent := defaultsMap
	targetKey := "fallbacks"
	if modelAsMap {
		targetParent = modelMap
	}

	rawFallbacks, exists := targetParent[targetKey]
	if !exists {
		return nil, nil
	}

	fallbacks := readStringSlice(rawFallbacks)
	removed := make([]string, 0)
	remaining := make([]string, 0, len(fallbacks))
	for _, fallback := range fallbacks {
		if hasClawdsecbotPrefix(fallback) {
			removed = append(removed, fallback)
			continue
		}
		remaining = append(remaining, fallback)
	}
	if len(removed) == 0 {
		return nil, remaining
	}

	targetParent[targetKey] = writeStringSlice(remaining)
	if modelAsMap {
		defaultsMap["model"] = modelMap
	} else {
		defaultsMap = targetParent
	}
	agentsMap["defaults"] = defaultsMap
	rawConfig["agents"] = agentsMap
	return removed, remaining
}

func determineRestoredPrimaryModel(currentPrimary string, remainingModels []string, remainingFallbacks []string) string {
	strippedPrimary := stripClawdsecbotPrefix(currentPrimary)
	if containsString(remainingModels, strippedPrimary) {
		return strippedPrimary
	}

	// Go map iteration does not preserve the original JSON object order. Since
	// protection startup stores the previous primary in fallbacks, prefer the
	// last surviving fallback before falling back to the remaining model set.
	for i := len(remainingFallbacks) - 1; i >= 0; i-- {
		fallback := strings.TrimSpace(remainingFallbacks[i])
		if fallback == "" {
			continue
		}
		if len(remainingModels) == 0 || containsString(remainingModels, fallback) {
			return fallback
		}
	}

	if len(remainingModels) > 0 {
		return lastString(remainingModels)
	}
	if strippedPrimary != "" && !hasClawdsecbotPrefix(strippedPrimary) {
		return strippedPrimary
	}
	return ""
}

func restoreInjectedOpenclawBotState(rawConfig map[string]interface{}, currentPrimary string) (string, []string, []string, []string, bool, error) {
	removedProviders := removeInjectedProviderEntries(rawConfig)
	removedModels, remainingModels := removeInjectedDefaultsModels(rawConfig)
	removedFallbacks, remainingFallbacks := removeInjectedFallbacks(rawConfig)

	targetPrimary := determineRestoredPrimaryModel(currentPrimary, remainingModels, remainingFallbacks)
	if targetPrimary == "" {
		return "", removedProviders, removedModels, removedFallbacks, false, fmt.Errorf("no remaining model available for primary")
	}

	if _, _, err := setAgentsDefaultsPrimary(rawConfig, targetPrimary); err != nil {
		return "", removedProviders, removedModels, removedFallbacks, false, err
	}

	changed := len(removedProviders) > 0 || len(removedModels) > 0 || len(removedFallbacks) > 0 ||
		strings.TrimSpace(currentPrimary) != strings.TrimSpace(targetPrimary)

	return targetPrimary, removedProviders, removedModels, removedFallbacks, changed, nil
}

// buildDefaultProviderConfig constructs a provider config with defaults.
func buildDefaultProviderConfig(providerName string, modelID string) map[string]interface{} {
	modelID = strings.TrimSpace(modelID)

	provider := map[string]interface{}{
		"api":    "openai-completions",
		"models": []interface{}{},
	}
	if defaultBaseURL := getDefaultBaseURL(providerName); defaultBaseURL != "" {
		provider["baseUrl"] = defaultBaseURL
	}
	if modelID != "" {
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
const InitialBackupFileName = "openclaw.json.initial"

// backupOpenclawConfigOnce 仅在首次启动代理时备份配置文件。
// 如果初始备份文件已存在，则跳过备份；否则创建初始备份。
// 返回值：备份文件路径、是否为新创建的备份、错误
func backupOpenclawConfigOnce(configPath string, backupDir string) (string, bool, error) {
	if strings.TrimSpace(backupDir) == "" {
		return "", false, fmt.Errorf("backup directory is empty")
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", false, fmt.Errorf("create backup directory failed: %w", err)
	}

	initialBackupPath := filepath.Join(backupDir, InitialBackupFileName)

	// 检查初始备份是否已存在
	if _, err := os.Stat(initialBackupPath); err == nil {
		// 初始备份已存在，跳过备份
		return initialBackupPath, false, nil
	}

	// 初始备份不存在，创建新备份
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", false, fmt.Errorf("read config failed: %w", err)
	}
	if err := os.WriteFile(initialBackupPath, data, 0644); err != nil {
		return "", false, fmt.Errorf("write initial backup failed: %w", err)
	}

	return initialBackupPath, true, nil
}

// hasInitialBackup 检查初始备份文件是否存在
func hasInitialBackup(backupDir string) bool {
	if strings.TrimSpace(backupDir) == "" {
		return false
	}
	initialBackupPath := filepath.Join(backupDir, InitialBackupFileName)
	_, err := os.Stat(initialBackupPath)
	return err == nil
}

// getInitialBackupPath 获取初始备份文件路径
func getInitialBackupPath(backupDir string) string {
	return filepath.Join(backupDir, InitialBackupFileName)
}

// backupOpenclawConfig saves a backup copy of openclaw config to backupDir.
// Deprecated: 此函数已废弃，使用 backupOpenclawConfigOnce 代替。
// 保留此函数仅用于兼容性，内部实际调用 backupOpenclawConfigOnce。
func backupOpenclawConfig(configPath string, backupDir string) (string, error) {
	backupPath, _, err := backupOpenclawConfigOnce(configPath, backupDir)
	return backupPath, err
}

// findLatestOpenclawBackup returns the latest backup path if it exists.
func findLatestOpenclawBackup(backupDir string) (string, bool, error) {
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
		if strings.HasPrefix(name, "openclaw.json.bak-") {
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

// isFileContentSameMD5 compares two files by MD5 hash.
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

// computeFileMD5 returns the MD5 hash of a file.
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

// trimOpenclawBackups keeps only the latest maxCount backup files.
func trimOpenclawBackups(backupDir string, maxCount int) error {
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
		if strings.HasPrefix(name, "openclaw.json.bak-") {
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

// restoreOpenclawConfig restores openclaw config from a backup file.
func restoreOpenclawConfig(configPath string, backupPath string) error {
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("read backup failed: %w", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
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

type RestoreBotDefaultStateResult struct {
	Success          bool     `json:"success"`
	Message          string   `json:"message,omitempty"`
	Error            string   `json:"error,omitempty"`
	ConfigPath       string   `json:"config_path,omitempty"`
	PrimaryModel     string   `json:"primary_model,omitempty"`
	RemovedProviders []string `json:"removed_providers,omitempty"`
	RemovedModels    []string `json:"removed_models,omitempty"`
	RemovedFallbacks []string `json:"removed_fallbacks,omitempty"`
	Restored         bool     `json:"restored,omitempty"`
	GatewayRestart   bool     `json:"gateway_restart,omitempty"`
}

// RestoreBotDefaultStateByAsset removes injected clawdsecbot-* model entries
// and restores OpenClaw to a usable unprotected state without rolling the full
// config file back to its initial backup.
func RestoreBotDefaultStateByAsset(assetID string) RestoreBotDefaultStateResult {
	logging.Info("[RestoreBotDefaultState] Starting targeted OpenClaw restore...")

	configPath, err := findConfigPath()
	if err != nil {
		return RestoreBotDefaultStateResult{
			Success: false,
			Error:   fmt.Sprintf("config not found: %v", err),
		}
	}

	config, rawConfig, err := loadConfig(configPath)
	if err != nil {
		return RestoreBotDefaultStateResult{
			Success:    false,
			Error:      fmt.Sprintf("load config failed: %v", err),
			ConfigPath: configPath,
		}
	}

	currentPrimary, err := getPrimaryModelFromConfig(config)
	if err != nil {
		return RestoreBotDefaultStateResult{
			Success:    false,
			Error:      fmt.Sprintf("read primary model failed: %v", err),
			ConfigPath: configPath,
		}
	}

	targetPrimary, removedProviders, removedModels, removedFallbacks, changed, err := restoreInjectedOpenclawBotState(rawConfig, currentPrimary)
	if err != nil {
		return RestoreBotDefaultStateResult{
			Success:          false,
			Error:            fmt.Sprintf("restore bot default state failed: %v", err),
			ConfigPath:       configPath,
			RemovedProviders: removedProviders,
			RemovedModels:    removedModels,
			RemovedFallbacks: removedFallbacks,
		}
	}

	if !changed {
		return RestoreBotDefaultStateResult{
			Success:          true,
			Message:          "bot config already in default state",
			ConfigPath:       configPath,
			PrimaryModel:     targetPrimary,
			RemovedProviders: removedProviders,
			RemovedModels:    removedModels,
			RemovedFallbacks: removedFallbacks,
			Restored:         false,
			GatewayRestart:   false,
		}
	}

	if err := saveConfig(configPath, rawConfig); err != nil {
		return RestoreBotDefaultStateResult{
			Success:          false,
			Error:            fmt.Sprintf("save config failed: %v", err),
			ConfigPath:       configPath,
			PrimaryModel:     targetPrimary,
			RemovedProviders: removedProviders,
			RemovedModels:    removedModels,
			RemovedFallbacks: removedFallbacks,
		}
	}

	req := &GatewayRestartRequest{
		AssetName:      openclawAssetName,
		AssetID:        strings.TrimSpace(assetID),
		SandboxEnabled: false,
	}

	gatewayResult, gatewayErr := restartOpenclawGateway(req)
	gatewayRestart := gatewayErr == nil
	if gatewayErr != nil {
		logging.Warning("[RestoreBotDefaultState] Gateway restart failed: %v", gatewayErr)
	} else {
		logging.Info("[RestoreBotDefaultState] Gateway restarted: %v", gatewayResult)
	}

	return RestoreBotDefaultStateResult{
		Success:          true,
		Message:          "bot config restored to default state",
		ConfigPath:       configPath,
		PrimaryModel:     targetPrimary,
		RemovedProviders: removedProviders,
		RemovedModels:    removedModels,
		RemovedFallbacks: removedFallbacks,
		Restored:         true,
		GatewayRestart:   gatewayRestart,
	}
}

// RestoreToInitialConfig 恢复 openclaw.json 到初始配置状态并重启 gateway。
// 此函数执行以下步骤：
// 1. 检查初始备份文件是否存在
// 2. 将 openclaw.json 恢复到初始状态
// 3. 重启 openclaw gateway（不启用沙箱）
func RestoreToInitialConfigByAsset(backupDir string, assetID string) RestoreToInitialConfigResult {
	logging.Info("[RestoreToInitialConfig] Starting restore to initial config...")

	// 检查初始备份是否存在
	if !hasInitialBackup(backupDir) {
		return RestoreToInitialConfigResult{
			Success: false,
			Error:   "initial backup not found",
		}
	}

	// 获取配置文件路径
	configPath, err := findConfigPath()
	if err != nil {
		return RestoreToInitialConfigResult{
			Success: false,
			Error:   fmt.Sprintf("config not found: %v", err),
		}
	}

	// 获取初始备份路径
	initialBackupPath := getInitialBackupPath(backupDir)

	// 恢复配置文件
	if err := restoreOpenclawConfig(configPath, initialBackupPath); err != nil {
		return RestoreToInitialConfigResult{
			Success:    false,
			Error:      fmt.Sprintf("restore failed: %v", err),
			ConfigPath: configPath,
			BackupPath: initialBackupPath,
		}
	}
	logging.Info("[RestoreToInitialConfig] Config restored from %s to %s", initialBackupPath, configPath)

	// 重启 openclaw gateway（不启用沙箱）
	req := &GatewayRestartRequest{
		AssetName:      openclawAssetName,
		AssetID:        strings.TrimSpace(assetID),
		SandboxEnabled: false, // 恢复时不启用沙箱
	}

	gatewayResult, gatewayErr := restartOpenclawGateway(req)
	gatewayRestart := gatewayErr == nil
	if gatewayErr != nil {
		logging.Warning("[RestoreToInitialConfig] Gateway restart failed: %v", gatewayErr)
	} else {
		logging.Info("[RestoreToInitialConfig] Gateway restarted: %v", gatewayResult)
	}

	return RestoreToInitialConfigResult{
		Success:        true,
		Message:        "config restored to initial state",
		ConfigPath:     configPath,
		BackupPath:     initialBackupPath,
		GatewayRestart: gatewayRestart,
	}
}

// RestoreToInitialConfig 恢复 openclaw.json 到初始配置状态并重启 gateway（兼容旧调用，不区分资产实例）。
func RestoreToInitialConfig(backupDir string) RestoreToInitialConfigResult {
	return RestoreToInitialConfigByAsset(backupDir, "")
}

// HasInitialBackup 检查是否存在初始备份（对外暴露）
func HasInitialBackup(backupDir string) bool {
	return hasInitialBackup(backupDir)
}
