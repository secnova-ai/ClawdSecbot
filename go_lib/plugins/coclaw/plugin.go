package coclaw

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/plugin_sdk"
	openclawplugin "go_lib/plugins/openclaw"
)

const (
	coclawAssetName = "CoClaw"
	coclawPluginID  = "coclaw"
)

type Plugin struct {
	mu                 sync.RWMutex
	protectionStatuses map[string]core.ProtectionStatus
}

var plugin *Plugin

func init() {
	plugin = &Plugin{protectionStatuses: make(map[string]core.ProtectionStatus)}
	core.GetPluginManager().Register(plugin)
	logging.Info("CoClaw plugin registered")
}

func (p *Plugin) GetAssetName() string { return coclawAssetName }
func (p *Plugin) GetID() string        { return coclawPluginID }

func (p *Plugin) GetManifest() plugin_sdk.PluginManifest {
	return plugin_sdk.PluginManifest{
		PluginID:    coclawPluginID,
		BotType:     strings.ToLower(coclawAssetName),
		DisplayName: coclawAssetName,
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
		ID:      "coclaw.asset.v1",
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
					{LabelKey: "asset.field.model", ValueRef: "metadata.model_primary"},
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

func (p *Plugin) RequiresBotModelConfig() bool { return false }

func (p *Plugin) ScanAssets() ([]core.Asset, error) {
	configPath, _ := findConfigPath()
	return newAssetScanner(configPath).scan()
}

func (p *Plugin) GetMainProcessPID(asset core.Asset) (int, bool) {
	return 0, false
}

func (p *Plugin) AssessRisks(scannedHashes map[string]bool, assets []core.Asset) ([]core.Risk, error) {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return nil, err
	}
	defer restore()

	risks, err := openclawplugin.GetOpenclawPlugin().AssessRisks(scannedHashes, assets)
	if err != nil {
		return nil, err
	}
	for i := range risks {
		risks[i].SourcePlugin = coclawAssetName
	}
	return risks, nil
}

func (p *Plugin) GetVulnInfoJSON() []byte {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return []byte("{}")
	}
	defer restore()
	return openclawplugin.GetOpenclawPlugin().GetVulnInfoJSON()
}

func (p *Plugin) CompareVulnerabilityVersion(current, target string) (int, bool) {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return 0, false
	}
	defer restore()
	return openclawplugin.GetOpenclawPlugin().CompareVulnerabilityVersion(current, target)
}

func (p *Plugin) MitigateRisk(riskInfo string) string {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restore()
	return openclawplugin.GetOpenclawPlugin().MitigateRisk(riskInfo)
}

func (p *Plugin) StartProtection(assetID string, cfg core.ProtectionConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.protectionStatuses[strings.TrimSpace(assetID)] = core.ProtectionStatus{
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
	p.protectionStatuses[strings.TrimSpace(assetID)] = core.ProtectionStatus{}
	return nil
}

func (p *Plugin) GetProtectionStatus(assetID string) core.ProtectionStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.protectionStatuses[strings.TrimSpace(assetID)]
}

func (p *Plugin) OnProtectionStart(ctx *core.ProtectionContext) (map[string]interface{}, error) {
	return applyProxyConfig(ctx)
}

func (p *Plugin) OnBeforeProxyStop(ctx *core.ProtectionContext) {
	if err := restoreBotDefaultState(ctx); err != nil {
		logging.Warning("[CoClaw] restore failed: %v", err)
	}
}

func (p *Plugin) StartSkillSecurityScan(skillPath, modelConfigJSON string) string {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restore()
	return openclawplugin.GetOpenclawPlugin().StartSkillSecurityScan(skillPath, modelConfigJSON)
}

func (p *Plugin) GetSkillSecurityScanLog(scanID string) string {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restore()
	return openclawplugin.GetOpenclawPlugin().GetSkillSecurityScanLog(scanID)
}

func (p *Plugin) GetSkillSecurityScanResult(scanID string) string {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restore()
	return openclawplugin.GetOpenclawPlugin().GetSkillSecurityScanResult(scanID)
}

func (p *Plugin) CancelSkillSecurityScan(scanID string) string {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restore()
	return openclawplugin.GetOpenclawPlugin().CancelSkillSecurityScan(scanID)
}

func (p *Plugin) StartBatchSkillScan() string {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restore()
	return openclawplugin.GetOpenclawPlugin().StartBatchSkillScan()
}

func (p *Plugin) GetBatchSkillScanLog(batchID string) string {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restore()
	return openclawplugin.GetOpenclawPlugin().GetBatchSkillScanLog(batchID)
}

func (p *Plugin) GetBatchSkillScanResults(batchID string) string {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restore()
	return openclawplugin.GetOpenclawPlugin().GetBatchSkillScanResults(batchID)
}

func (p *Plugin) CancelBatchSkillScan(batchID string) string {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restore()
	return openclawplugin.GetOpenclawPlugin().CancelBatchSkillScan(batchID)
}

func (p *Plugin) TestModelConnection(configJSON string) string {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restore()
	return openclawplugin.GetOpenclawPlugin().TestModelConnection(configJSON)
}

func (p *Plugin) DeleteSkill(skillPath string) string {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restore()
	return openclawplugin.GetOpenclawPlugin().DeleteSkill(skillPath)
}

func (p *Plugin) SyncGatewaySandbox() string {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restore()
	return openclawplugin.GetOpenclawPlugin().SyncGatewaySandbox()
}

func (p *Plugin) SyncGatewaySandboxByAsset(assetID string) string {
	restore, err := withOpenclawOverrides()
	if err != nil {
		return marshalError(err)
	}
	defer restore()
	return openclawplugin.GetOpenclawPlugin().SyncGatewaySandboxByAsset(assetID)
}

func (p *Plugin) HasInitialBackup() string {
	path := initialBackupPath(backupDir(""))
	_, err := loadBackupRaw(path)
	return marshalResult(map[string]interface{}{"success": err == nil, "backup_path": path})
}

func (p *Plugin) RestoreBotDefaultState(assetID string) string {
	err := restoreBotDefaultState(&core.ProtectionContext{AssetID: strings.TrimSpace(assetID)})
	if err != nil {
		return marshalError(err)
	}
	return marshalResult(map[string]interface{}{"success": true, "asset_id": strings.TrimSpace(assetID)})
}

func (p *Plugin) OnAppExit(assetID string) string {
	return p.RestoreBotDefaultState(assetID)
}

func applyProxyConfig(ctx *core.ProtectionContext) (map[string]interface{}, error) {
	if ctx == nil {
		return nil, fmt.Errorf("protection context is nil")
	}
	cfg, raw, configPath, err := loadConfig()
	if err != nil {
		return nil, err
	}
	backupPath, err := ensureInitialBackup(configPath, backupDir(ctx.BackupDir))
	if err != nil {
		return nil, fmt.Errorf("backup failed: %w", err)
	}

	proxyURL := fmt.Sprintf("http://127.0.0.1:%d/v1", ctx.ProxyPort)
	hijack, changed, err := hijackProviderAlias(raw, proxyURL)
	if err != nil {
		return nil, err
	}
	if changed {
		if err := saveRawConfig(configPath, raw); err != nil {
			return nil, err
		}
	}

	return map[string]interface{}{
		"success":          true,
		"changed":          changed,
		"config_path":      configPath,
		"backup_path":      backupPath,
		"method":           "provider_alias_hijack",
		"original_primary": hijack.OriginalPrimary,
		"proxy_primary":    hijack.ProxyPrimary,
		"proxy_provider":   hijack.ProxyProviderName,
		"gateway_port":     cfg.Gateway.Port,
	}, nil
}

func restoreBotDefaultState(ctx *core.ProtectionContext) error {
	_, raw, configPath, err := loadConfig()
	if err != nil {
		return err
	}
	dir := ""
	if ctx != nil {
		dir = ctx.BackupDir
	}
	backupRaw, err := loadBackupRaw(initialBackupPath(backupDir(dir)))
	if err != nil {
		return err
	}
	_, changed, err := restoreProviderAlias(raw, backupRaw)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return saveRawConfig(configPath, raw)
}

func marshalResult(payload map[string]interface{}) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return `{"success":false,"error":"marshal error"}`
	}
	return string(data)
}

func marshalError(err error) string {
	if err == nil {
		return marshalResult(map[string]interface{}{"success": false, "error": "unknown error"})
	}
	return marshalResult(map[string]interface{}{"success": false, "error": err.Error()})
}
