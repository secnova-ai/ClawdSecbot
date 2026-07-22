package dwtsclaw

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/plugin_sdk"
	openclawplugin "go_lib/plugins/openclaw"
)

const (
	dwtsclawAssetName = "DWTSClaw"
	dwtsclawPluginID  = "dwtsclaw"
)

// Plugin owns DWTSClaw-specific discovery and config takeover while reusing
// the compatible OpenClaw risk and skill implementations.
type Plugin struct {
	mu                 sync.RWMutex
	protectionStatuses map[string]core.ProtectionStatus
}

var plugin *Plugin

func init() {
	plugin = &Plugin{protectionStatuses: make(map[string]core.ProtectionStatus)}
	core.GetPluginManager().Register(plugin)
	logging.Info("DWTSClaw plugin registered")
}

func (p *Plugin) GetAssetName() string { return dwtsclawAssetName }
func (p *Plugin) GetID() string        { return dwtsclawPluginID }

func (p *Plugin) GetManifest() plugin_sdk.PluginManifest {
	return plugin_sdk.PluginManifest{
		PluginID:    dwtsclawPluginID,
		BotType:     strings.ToLower(dwtsclawAssetName),
		DisplayName: dwtsclawAssetName,
		APIVersion:  "v1",
		Capabilities: []string{
			"scan",
			"risk_assessment",
			"mitigation",
			"protection_proxy",
			"sandbox",
			"audit_log",
		},
		SupportedPlatforms: []string{"windows"},
	}
}

func (p *Plugin) GetAssetUISchema() *plugin_sdk.AssetUISchema {
	return &plugin_sdk.AssetUISchema{
		ID:      "dwtsclaw.asset.v1",
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
					{LabelKey: "asset.field.version", ValueRef: "version"},
					{LabelKey: "asset.field.port", ValueRef: "metadata.gateway_port"},
					{LabelKey: "asset.field.config_path", ValueRef: "metadata.config_path"},
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

// RequiresBotModelConfig is false because DWTSClaw's active upstream is read
// from the pre-takeover OpenClaw provider in openclaw.json.
func (p *Plugin) RequiresBotModelConfig() bool { return false }

func (p *Plugin) ScanAssets() ([]core.Asset, error) {
	return newAssetScanner().scan()
}

func (p *Plugin) GetMainProcessPID(asset core.Asset) (int, bool) {
	if asset.Metadata == nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(asset.Metadata["pid"]))
	return pid, err == nil && pid > 0
}

func (p *Plugin) AssessRisks(scannedHashes map[string]bool, assets []core.Asset) ([]core.Risk, error) {
	risks, err := openclawplugin.GetOpenclawPlugin().AssessRisks(scannedHashes, assets)
	if err != nil {
		return nil, err
	}
	for i := range risks {
		risks[i].SourcePlugin = dwtsclawAssetName
	}
	return risks, nil
}

func (p *Plugin) GetVulnInfoJSON() []byte {
	return openclawplugin.GetOpenclawPlugin().GetVulnInfoJSON()
}

func (p *Plugin) CompareVulnerabilityVersion(current, target string) (int, bool) {
	return openclawplugin.GetOpenclawPlugin().CompareVulnerabilityVersion(current, target)
}

func (p *Plugin) MitigateRisk(riskInfo string) string {
	return openclawplugin.GetOpenclawPlugin().MitigateRisk(riskInfo)
}

func (p *Plugin) StartProtection(assetID string, config core.ProtectionConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.protectionStatuses[strings.TrimSpace(assetID)] = core.ProtectionStatus{
		Running:       config.ProxyEnabled,
		ProxyRunning:  config.ProxyEnabled,
		ProxyPort:     config.ProxyPort,
		SandboxActive: config.SandboxEnabled,
		AuditOnly:     config.AuditOnly,
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
	if err := restoreProxyConfig(ctx); err != nil {
		logging.Warning("[DWTSClaw] Restore config failed: %v", err)
	}
}

func (p *Plugin) ResolveProxyForwardingTarget(assetID string) (*core.ProxyForwardingTarget, error) {
	_ = assetID
	return resolveProxyForwardingTarget("")
}

func (p *Plugin) StartSkillSecurityScan(skillPath, modelConfigJSON string) string {
	return openclawplugin.GetOpenclawPlugin().StartSkillSecurityScan(skillPath, modelConfigJSON)
}

func (p *Plugin) GetSkillSecurityScanLog(scanID string) string {
	return openclawplugin.GetOpenclawPlugin().GetSkillSecurityScanLog(scanID)
}

func (p *Plugin) GetSkillSecurityScanResult(scanID string) string {
	return openclawplugin.GetOpenclawPlugin().GetSkillSecurityScanResult(scanID)
}

func (p *Plugin) CancelSkillSecurityScan(scanID string) string {
	return openclawplugin.GetOpenclawPlugin().CancelSkillSecurityScan(scanID)
}

func (p *Plugin) StartBatchSkillScan() string {
	return openclawplugin.GetOpenclawPlugin().StartBatchSkillScan()
}

func (p *Plugin) GetBatchSkillScanLog(batchID string) string {
	return openclawplugin.GetOpenclawPlugin().GetBatchSkillScanLog(batchID)
}

func (p *Plugin) GetBatchSkillScanResults(batchID string) string {
	return openclawplugin.GetOpenclawPlugin().GetBatchSkillScanResults(batchID)
}

func (p *Plugin) CancelBatchSkillScan(batchID string) string {
	return openclawplugin.GetOpenclawPlugin().CancelBatchSkillScan(batchID)
}

func (p *Plugin) TestModelConnection(configJSON string) string {
	return openclawplugin.GetOpenclawPlugin().TestModelConnection(configJSON)
}

func (p *Plugin) DeleteSkill(skillPath string) string {
	return openclawplugin.GetOpenclawPlugin().DeleteSkill(skillPath)
}

func (p *Plugin) SyncGatewaySandbox() string {
	return openclawplugin.GetOpenclawPlugin().SyncGatewaySandbox()
}

func (p *Plugin) SyncGatewaySandboxByAsset(assetID string) string {
	return openclawplugin.GetOpenclawPlugin().SyncGatewaySandboxByAsset(assetID)
}

func (p *Plugin) HasInitialBackup() string {
	configPath, err := findConfigPath()
	if err != nil {
		return marshalPluginResult(map[string]interface{}{"success": false, "error": err.Error()})
	}
	_, backupPath, err := loadInitialBackup(configPath, "")
	return marshalPluginResult(map[string]interface{}{"success": err == nil, "backup_path": backupPath})
}

func (p *Plugin) RestoreBotDefaultState(assetID string) string {
	err := restoreProxyConfig(&core.ProtectionContext{AssetID: strings.TrimSpace(assetID)})
	if err != nil {
		return marshalPluginResult(map[string]interface{}{"success": false, "error": err.Error()})
	}
	return marshalPluginResult(map[string]interface{}{"success": true, "asset_id": strings.TrimSpace(assetID)})
}

func (p *Plugin) RestoreToInitialConfig() string {
	return p.RestoreBotDefaultState("")
}

func (p *Plugin) OnAppExit(assetID string) string {
	return p.RestoreBotDefaultState(assetID)
}

func marshalPluginResult(payload map[string]interface{}) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return `{"success":false,"error":"marshal error"}`
	}
	return string(data)
}
