package qclaw

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/repository"
	"go_lib/plugin_sdk"
	openclawplugin "go_lib/plugins/openclaw"
)

type Plugin struct {
	mu                 sync.RWMutex
	protectionStatuses map[string]core.ProtectionStatus
}

var plugin *Plugin

func init() {
	plugin = &Plugin{
		protectionStatuses: make(map[string]core.ProtectionStatus),
	}
	core.GetPluginManager().Register(plugin)
	logging.Info("QClaw plugin registered")
}

func (p *Plugin) GetAssetName() string { return qclawAssetName }
func (p *Plugin) GetID() string        { return qclawPluginID }

func (p *Plugin) GetManifest() plugin_sdk.PluginManifest {
	return plugin_sdk.PluginManifest{
		PluginID:    qclawPluginID,
		BotType:     strings.ToLower(qclawAssetName),
		DisplayName: qclawAssetName,
		APIVersion:  "v1",
		Capabilities: []string{
			"scan",
			"risk_assessment",
			"mitigation",
			"protection_proxy",
			"sandbox",
			"audit_log",
		},
		SupportedPlatforms: []string{"windows", "linux", "macos"},
	}
}

func (p *Plugin) GetAssetUISchema() *plugin_sdk.AssetUISchema {
	return &plugin_sdk.AssetUISchema{
		ID:      "qclaw.asset.v1",
		Version: "1",
		Badges: []plugin_sdk.AssetUIBadge{
			{LabelKey: "asset.badge.bot_type", ValueRef: "source_plugin", Tone: "info"},
		},
		StatusChips: []plugin_sdk.AssetUIStatusChip{
			{LabelKey: "asset.status.protection", ValueRef: "metadata.protection_status", Tone: "neutral"},
		},
		Sections: []plugin_sdk.AssetUISection{
			{
				Type:     "kv_list",
				LabelKey: "asset.section.runtime",
				Items: []plugin_sdk.AssetUIField{
					{LabelKey: "asset.field.port", ValueRef: "metadata.gateway_port"},
					{LabelKey: "asset.field.config_path", ValueRef: "metadata.config_path"},
					{LabelKey: "asset.field.logs_path", ValueRef: "metadata.logs_dir"},
					{LabelKey: "asset.field.state_path", ValueRef: "metadata.state_path"},
					{LabelKey: "asset.field.version", ValueRef: "version"},
				},
			},
		},
		Actions: []plugin_sdk.AssetUIAction{
			{Action: "open_config", LabelKey: "asset.action.open_config", Variant: "secondary"},
			{Action: "start_protection", LabelKey: "asset.action.start_protection", Variant: "primary"},
			{Action: "stop_protection", LabelKey: "asset.action.stop_protection", Variant: "danger"},
		},
	}
}

func (p *Plugin) RequiresBotModelConfig() bool { return true }

func (p *Plugin) ScanAssets() ([]core.Asset, error) {
	configPath, _ := findConfigPath()
	return newAssetScanner(filepath.Dir(configPath)).scan()
}

func (p *Plugin) GetMainProcessPID(asset core.Asset) (int, bool) {
	return 0, false
}

func (p *Plugin) AssessRisks(scannedHashes map[string]bool, assets []core.Asset) ([]core.Risk, error) {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return nil, err
	}
	defer restoreOverrides()

	risks, err := openclawplugin.GetOpenclawPlugin().AssessRisks(scannedHashes, assets)
	if err != nil {
		return nil, err
	}

	risks = removeRiskByID(risks, "skills_not_scanned")
	if skillRisk := buildUnscannedSkillsRisk(scannedHashes); skillRisk != nil {
		risks = append(risks, *skillRisk)
	}

	for i := range risks {
		risks[i].SourcePlugin = qclawAssetName
	}
	return risks, nil
}

func (p *Plugin) MitigateRisk(riskInfo string) string {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restoreOverrides()
	return openclawplugin.GetOpenclawPlugin().MitigateRisk(riskInfo)
}

func (p *Plugin) GetVulnInfoJSON() []byte {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return []byte("{}")
	}
	defer restoreOverrides()
	return openclawplugin.GetOpenclawPlugin().GetVulnInfoJSON()
}

func (p *Plugin) CompareVulnerabilityVersion(current, target string) (int, bool) {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return 0, false
	}
	defer restoreOverrides()
	return openclawplugin.GetOpenclawPlugin().CompareVulnerabilityVersion(current, target)
}

func (p *Plugin) StartProtection(assetID string, cfg core.ProtectionConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.protectionStatuses[assetID] = core.ProtectionStatus{
		Running:       cfg.ProxyEnabled,
		ProxyRunning:  cfg.ProxyEnabled,
		ProxyPort:     cfg.ProxyPort,
		SandboxActive: cfg.SandboxEnabled,
		AuditOnly:     cfg.AuditOnly,
	}
	return nil
}

func (p *Plugin) StopProtection(assetID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.protectionStatuses[assetID] = core.ProtectionStatus{
		Running:      false,
		ProxyRunning: false,
	}
	return nil
}

func (p *Plugin) GetProtectionStatus(assetID string) core.ProtectionStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.protectionStatuses[assetID]
}

func (p *Plugin) OnProtectionStart(ctx *core.ProtectionContext) (map[string]interface{}, error) {
	result, err := applyProxyConfig(ctx)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (p *Plugin) OnBeforeProxyStop(ctx *core.ProtectionContext) {
	if err := restoreBotDefaultState(); err != nil {
		logging.Warning("[QClaw] Restore bot default state failed: %v", err)
	}
}

func (p *Plugin) StartSkillSecurityScan(skillPath, modelConfigJSON string) string {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restoreOverrides()
	return openclawplugin.GetOpenclawPlugin().StartSkillSecurityScan(skillPath, modelConfigJSON)
}

func (p *Plugin) GetSkillSecurityScanLog(scanID string) string {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restoreOverrides()
	return openclawplugin.GetOpenclawPlugin().GetSkillSecurityScanLog(scanID)
}

func (p *Plugin) GetSkillSecurityScanResult(scanID string) string {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restoreOverrides()
	return openclawplugin.GetOpenclawPlugin().GetSkillSecurityScanResult(scanID)
}

func (p *Plugin) CancelSkillSecurityScan(scanID string) string {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restoreOverrides()
	return openclawplugin.GetOpenclawPlugin().CancelSkillSecurityScan(scanID)
}

func (p *Plugin) StartBatchSkillScan() string {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restoreOverrides()
	return openclawplugin.GetOpenclawPlugin().StartBatchSkillScan()
}

func (p *Plugin) GetBatchSkillScanLog(batchID string) string {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restoreOverrides()
	return openclawplugin.GetOpenclawPlugin().GetBatchSkillScanLog(batchID)
}

func (p *Plugin) GetBatchSkillScanResults(batchID string) string {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restoreOverrides()
	return openclawplugin.GetOpenclawPlugin().GetBatchSkillScanResults(batchID)
}

func (p *Plugin) CancelBatchSkillScan(batchID string) string {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restoreOverrides()
	return openclawplugin.GetOpenclawPlugin().CancelBatchSkillScan(batchID)
}

func (p *Plugin) TestModelConnection(configJSON string) string {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restoreOverrides()
	return openclawplugin.GetOpenclawPlugin().TestModelConnection(configJSON)
}

func (p *Plugin) DeleteSkill(skillPath string) string {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restoreOverrides()
	return openclawplugin.GetOpenclawPlugin().DeleteSkill(skillPath)
}

func (p *Plugin) SyncGatewaySandbox() string {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restoreOverrides()
	return openclawplugin.GetOpenclawPlugin().SyncGatewaySandbox()
}

func (p *Plugin) SyncGatewaySandboxByAsset(assetID string) string {
	restoreOverrides, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restoreOverrides()
	return openclawplugin.GetOpenclawPlugin().SyncGatewaySandboxByAsset(assetID)
}

func (p *Plugin) HasInitialBackup() string {
	backupDir := getBackupDir("")
	_, err := os.Stat(getInitialBackupPath(backupDir))
	return marshalResult(map[string]interface{}{"success": err == nil, "backup_path": getInitialBackupPath(backupDir)})
}

func (p *Plugin) RestoreToInitialConfig() string {
	configPath, err := findConfigPath()
	if err != nil {
		return marshalError(err)
	}
	backupPath := getInitialBackupPath(getBackupDir(""))
	content, err := os.ReadFile(backupPath)
	if err != nil {
		return marshalError(err)
	}
	if err := os.WriteFile(configPath, content, 0644); err != nil {
		return marshalError(err)
	}
	return marshalResult(map[string]interface{}{"success": true, "config_path": configPath, "backup_path": backupPath})
}

func (p *Plugin) OnAppExit(assetID string) string {
	if err := restoreBotDefaultState(); err != nil {
		return marshalError(err)
	}
	return marshalResult(map[string]interface{}{"success": true, "asset_id": assetID})
}

func (p *Plugin) RestoreBotDefaultState(assetID string) string {
	if err := restoreBotDefaultState(); err != nil {
		return marshalError(err)
	}
	return marshalResult(map[string]interface{}{"success": true, "asset_id": assetID})
}

func applyProxyConfig(ctx *core.ProtectionContext) (map[string]interface{}, error) {
	config, raw, configPath, err := loadConfig()
	if err != nil {
		return nil, err
	}
	state, _ := loadQClawRuntimeState()

	repo := repository.NewProtectionRepository(nil)
	record, err := repo.GetProtectionConfig(strings.TrimSpace(ctx.AssetID))
	if err != nil {
		return nil, fmt.Errorf("failed to get protection config: %w", err)
	}
	if record == nil || record.BotModelConfig == nil {
		return nil, fmt.Errorf("bot model config is required")
	}

	backupDir := getBackupDir(ctx.BackupDir)
	backupPath, err := ensureInitialBackup(configPath, backupDir)
	if err != nil {
		return nil, fmt.Errorf("backup failed: %w", err)
	}

	providerName := normalizeProvider(record.BotModelConfig.Provider)
	baseModel := normalizeBaseModel(record.BotModelConfig.Model, providerName)
	if providerName == "" || baseModel == "" {
		return nil, fmt.Errorf("invalid bot model configuration")
	}

	providerKey := "clawdsecbot-" + providerName
	newModel := providerKey + "/" + baseModel
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", ctx.ProxyPort)

	modelsMap := ensureMap(raw, "models")
	providersMap := ensureMap(modelsMap, "providers")
	providersMap[providerKey] = map[string]interface{}{
		"baseUrl": proxyURL,
		"apiKey":  fallbackString(record.BotModelConfig.APIKey, "no apiKey"),
		"api":     "openai-completions",
		"models": []interface{}{
			map[string]interface{}{
				"id":   baseModel,
				"name": baseModel,
			},
		},
	}

	agentsMap := ensureMap(raw, "agents")
	defaultsMap := ensureMap(agentsMap, "defaults")
	currentPrimary := readPrimary(defaultsMap["model"])
	defaultsMap["model"] = updateModelField(defaultsMap["model"], newModel, currentPrimary)
	if existingModels, ok := defaultsMap["models"]; ok {
		defaultsMap["models"] = appendModel(existingModels, newModel)
	}

	if err := saveRawConfig(configPath, raw); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success":         true,
		"config_path":     configPath,
		"backup_path":     backupPath,
		"provider_name":   providerName,
		"original_model":  currentPrimary,
		"forwarded_model": newModel,
		"proxy_url":       proxyURL,
		"gateway_port":    config.Gateway.Port,
		"state_pid":       statePID(state),
		"state_port":      statePort(state),
		"state_dir":       stateDir(state),
	}, nil
}

func restoreBotDefaultState() error {
	_, raw, configPath, err := loadConfig()
	if err != nil {
		return err
	}

	modelsMap, ok := raw["models"].(map[string]interface{})
	if ok {
		if providersMap, ok := modelsMap["providers"].(map[string]interface{}); ok {
			for key := range providersMap {
				if strings.HasPrefix(strings.TrimSpace(key), "clawdsecbot-") {
					delete(providersMap, key)
				}
			}
		}
	}

	agentsMap, ok := raw["agents"].(map[string]interface{})
	if ok {
		if defaultsMap, ok := agentsMap["defaults"].(map[string]interface{}); ok {
			currentPrimary := readPrimary(defaultsMap["model"])
			defaultsMap["model"] = stripInjectedModel(defaultsMap["model"], currentPrimary)
			if existingModels, ok := defaultsMap["models"]; ok {
				defaultsMap["models"] = removeInjectedModels(existingModels)
			}
		}
	}

	return saveRawConfig(configPath, raw)
}

func removeRiskByID(risks []core.Risk, riskID string) []core.Risk {
	if len(risks) == 0 {
		return risks
	}

	filtered := make([]core.Risk, 0, len(risks))
	for _, risk := range risks {
		if risk.ID == riskID {
			continue
		}
		filtered = append(filtered, risk)
	}
	return filtered
}

func normalizeProvider(provider string) string {
	provider = strings.TrimSpace(strings.ToLower(provider))
	switch provider {
	case "claude":
		return "anthropic"
	case "gemini":
		return "google"
	default:
		return provider
	}
}

func normalizeBaseModel(model, provider string) string {
	model = strings.TrimSpace(model)
	prefix := provider + "/"
	if strings.HasPrefix(strings.ToLower(model), strings.ToLower(prefix)) {
		return strings.TrimSpace(model[len(prefix):])
	}
	return model
}

func ensureMap(parent map[string]interface{}, key string) map[string]interface{} {
	if raw, ok := parent[key].(map[string]interface{}); ok {
		return raw
	}
	child := make(map[string]interface{})
	parent[key] = child
	return child
}

func readPrimary(raw interface{}) string {
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]interface{}:
		if primary, ok := typed["primary"].(string); ok {
			return strings.TrimSpace(primary)
		}
	}
	return ""
}

func updateModelField(raw interface{}, primary, previousPrimary string) interface{} {
	modelMap, ok := raw.(map[string]interface{})
	if !ok {
		modelMap = map[string]interface{}{}
	}
	modelMap["primary"] = primary

	var fallbacks []interface{}
	if existing, ok := modelMap["fallbacks"].([]interface{}); ok {
		fallbacks = append(fallbacks, existing...)
	}
	if strings.TrimSpace(previousPrimary) != "" {
		already := false
		for _, item := range fallbacks {
			if text, ok := item.(string); ok && strings.TrimSpace(text) == previousPrimary {
				already = true
				break
			}
		}
		if !already {
			fallbacks = append(fallbacks, previousPrimary)
		}
	}
	if len(fallbacks) > 0 {
		modelMap["fallbacks"] = fallbacks
	}
	return modelMap
}

func appendModel(raw interface{}, model string) interface{} {
	switch typed := raw.(type) {
	case []interface{}:
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) == model {
				return typed
			}
		}
		return append(typed, model)
	case map[string]interface{}:
		if _, exists := typed[model]; !exists {
			typed[model] = map[string]interface{}{}
		}
		return typed
	default:
		return raw
	}
}

func stripInjectedModel(raw interface{}, currentPrimary string) interface{} {
	modelMap, ok := raw.(map[string]interface{})
	if !ok {
		return raw
	}

	if strings.HasPrefix(strings.TrimSpace(currentPrimary), "clawdsecbot-") {
		if fallbacks, ok := modelMap["fallbacks"].([]interface{}); ok {
			for i := len(fallbacks) - 1; i >= 0; i-- {
				if text, ok := fallbacks[i].(string); ok && !strings.HasPrefix(strings.TrimSpace(text), "clawdsecbot-") {
					modelMap["primary"] = strings.TrimSpace(text)
					break
				}
			}
		}
	}

	if fallbacks, ok := modelMap["fallbacks"].([]interface{}); ok {
		filtered := make([]interface{}, 0, len(fallbacks))
		for _, item := range fallbacks {
			text, ok := item.(string)
			if !ok || strings.HasPrefix(strings.TrimSpace(text), "clawdsecbot-") {
				continue
			}
			filtered = append(filtered, text)
		}
		modelMap["fallbacks"] = filtered
	}

	return modelMap
}

func removeInjectedModels(raw interface{}) interface{} {
	switch typed := raw.(type) {
	case []interface{}:
		filtered := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok || strings.HasPrefix(strings.TrimSpace(text), "clawdsecbot-") {
				continue
			}
			filtered = append(filtered, text)
		}
		return filtered
	case map[string]interface{}:
		for key := range typed {
			if strings.HasPrefix(strings.TrimSpace(key), "clawdsecbot-") {
				delete(typed, key)
			}
		}
		return typed
	default:
		return raw
	}
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func marshalResult(payload map[string]interface{}) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return `{"success":false,"error":"marshal error"}`
	}
	return string(data)
}

func marshalError(err error) string {
	return marshalResult(map[string]interface{}{
		"success": false,
		"error":   err.Error(),
	})
}

func loadQClawRuntimeState() (*qclawState, error) {
	statePath, err := findStatePath()
	if err != nil {
		return nil, err
	}
	return loadState(statePath)
}

func statePID(state *qclawState) int {
	if state == nil {
		return 0
	}
	return state.CLI.PID
}

func statePort(state *qclawState) int {
	if state == nil {
		return 0
	}
	return state.Port
}

func stateDir(state *qclawState) string {
	if state == nil {
		return ""
	}
	return state.StateDir
}
