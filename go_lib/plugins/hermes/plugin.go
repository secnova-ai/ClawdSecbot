package hermes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/modelfactory"
	"go_lib/core/proxy"
	"go_lib/core/repository"
	"go_lib/plugin_sdk"
)

// HermesPlugin implements Hermes protection lifecycle and risk checks.
type HermesPlugin struct {
	mu sync.RWMutex

	protectionStatuses map[string]core.ProtectionStatus
}

var hermesPlugin *HermesPlugin

const hermesPluginID = "hermes"

func init() {
	hermesPlugin = &HermesPlugin{protectionStatuses: make(map[string]core.ProtectionStatus)}
	core.GetPluginManager().Register(hermesPlugin)
	logging.Info("Hermes plugin registered")
}

func (p *HermesPlugin) GetAssetName() string {
	return hermesAssetName
}

func (p *HermesPlugin) GetID() string {
	return hermesPluginID
}

func (p *HermesPlugin) RequiresBotModelConfig() bool {
	return true
}

func (p *HermesPlugin) GetManifest() plugin_sdk.PluginManifest {
	return plugin_sdk.PluginManifest{
		PluginID:    hermesPluginID,
		BotType:     strings.ToLower(hermesAssetName),
		DisplayName: hermesAssetName,
		APIVersion:  "v1",
		Capabilities: []string{
			"scan",
			"risk_assessment",
			"mitigation",
			"protection_proxy",
			"gateway_restart",
		},
		SupportedPlatforms: []string{"windows", "linux", "macos"},
	}
}

func (p *HermesPlugin) GetAssetUISchema() *plugin_sdk.AssetUISchema {
	return &plugin_sdk.AssetUISchema{
		ID:      "hermes.asset.v1",
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
					{LabelKey: "asset.field.config_path", ValueRef: "metadata.config_path"},
					{LabelKey: "asset.field.model", ValueRef: "metadata.model_default"},
					{LabelKey: "asset.field.port", ValueRef: "port"},
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

func (p *HermesPlugin) ScanAssets() ([]core.Asset, error) {
	logging.Info("[Hermes] scanning assets")
	return NewHermesAssetScanner(GetConfigPath()).ScanAssets()
}

func (p *HermesPlugin) GetMainProcessPID(asset core.Asset) (int, bool) {
	return 0, false
}

func (p *HermesPlugin) AssessRisks(scannedHashes map[string]bool, assets []core.Asset) ([]core.Risk, error) {
	_ = scannedHashes
	_ = assets
	logging.Info("[Hermes] assessing risks")

	risks := []core.Risk{}
	configPath, err := findConfigPath()
	if err != nil {
		logging.Warning("[Hermes] config not found for risk check: %v", err)
		return risks, nil
	}

	checkPermissions(configPath, &risks)
	cfg, raw, err := loadConfig(configPath)
	if err != nil {
		logging.Warning("[Hermes] config load failed for risk check: %v", err)
		return risks, nil
	}

	checkTerminalBackend(cfg, &risks)
	checkApprovalsMode(cfg, &risks)
	checkRedactSecrets(cfg, raw, &risks)
	checkModelBaseURL(cfg, &risks)

	for i := range risks {
		if risks[i].Args == nil {
			risks[i].Args = map[string]interface{}{}
		}
		risks[i].Args["config_path"] = configPath
		if tmpl, ok := templates[risks[i].ID]; ok {
			risks[i].Mitigation = tmpl
		}
	}

	logging.Info("[Hermes] risk assessment completed: count=%d", len(risks))
	return risks, nil
}

func (p *HermesPlugin) MitigateRisk(riskInfo string) string {
	return MitigateRiskDispatch(riskInfo)
}

func (p *HermesPlugin) GetVulnInfoJSON() []byte {
	return GetVulInfoJSON()
}

func (p *HermesPlugin) CompareVulnerabilityVersion(current, target string) (int, bool) {
	return compareHermesVersion(current, target)
}

func (p *HermesPlugin) StartProtection(assetID string, config core.ProtectionConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.protectionStatuses[assetID] = core.ProtectionStatus{
		Running:       config.ProxyEnabled,
		ProxyRunning:  config.ProxyEnabled,
		ProxyPort:     config.ProxyPort,
		SandboxActive: config.SandboxEnabled,
		AuditOnly:     config.AuditOnly,
	}
	return nil
}

func (p *HermesPlugin) StopProtection(assetID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.protectionStatuses[assetID] = core.ProtectionStatus{Running: false, ProxyRunning: false}
	return nil
}

func (p *HermesPlugin) GetProtectionStatus(assetID string) core.ProtectionStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	live := proxy.GetProxyProtectionByAsset(assetID)
	if live != nil {
		running := live.IsRunning()
		cached := p.protectionStatuses[assetID]
		return core.ProtectionStatus{
			Running:       running,
			ProxyRunning:  running,
			ProxyPort:     live.GetPort(),
			SandboxActive: cached.SandboxActive,
			AuditOnly:     cached.AuditOnly,
		}
	}
	return p.protectionStatuses[assetID]
}

func parseBotModelConfig(config *repository.ProtectionConfig) *proxy.BotModelConfig {
	if config == nil || config.BotModelConfig == nil {
		return nil
	}
	return &proxy.BotModelConfig{
		Provider:  config.BotModelConfig.Provider,
		BaseURL:   config.BotModelConfig.BaseURL,
		APIKey:    config.BotModelConfig.APIKey,
		Model:     config.BotModelConfig.Model,
		SecretKey: config.BotModelConfig.SecretKey,
	}
}

// OnProtectionStart updates Hermes config to point model traffic at local proxy and restarts gateway.
func (p *HermesPlugin) OnProtectionStart(ctx *core.ProtectionContext) (map[string]interface{}, error) {
	if ctx == nil {
		return nil, fmt.Errorf("protection context is nil")
	}
	repo := repository.NewProtectionRepository(nil)
	cfg, err := repo.GetProtectionConfig(strings.TrimSpace(ctx.AssetID))
	if err != nil {
		return nil, fmt.Errorf("load protection config failed: %w", err)
	}
	if cfg == nil {
		return nil, fmt.Errorf("protection config not found")
	}
	botCfg := parseBotModelConfig(cfg)
	if botCfg == nil {
		return nil, fmt.Errorf("bot model config not found")
	}
	return startGatewayWithProxy(ctx.ProxyPort, botCfg, ctx.BackupDir, ctx.AssetID)
}

// OnBeforeProxyStop restores original Hermes config backup and restarts gateway.
func (p *HermesPlugin) OnBeforeProxyStop(ctx *core.ProtectionContext) {
	if ctx == nil {
		return
	}
	backupDir := strings.TrimSpace(ctx.BackupDir)
	if backupDir == "" {
		if pm := core.GetPathManager(); pm.IsInitialized() {
			backupDir = pm.GetBackupDir()
		}
	}
	if backupDir == "" {
		homeDir, _ := os.UserHomeDir()
		backupDir = filepath.Join(homeDir, ".botsec", "backups")
	}
	result := restoreHermesConfigByAsset(backupDir, ctx.AssetID)
	if success, _ := result["success"].(bool); !success {
		logging.Warning("[Hermes] restore on stop failed: %+v", result)
	}
}

// SyncGatewaySandbox implements core.GatewaySandboxCapability.
func (p *HermesPlugin) SyncGatewaySandbox() string {
	return SyncGatewaySandboxByAssetInternal("")
}

// SyncGatewaySandboxByAsset implements core.GatewaySandboxCapability.
func (p *HermesPlugin) SyncGatewaySandboxByAsset(assetID string) string {
	return SyncGatewaySandboxByAssetInternal(assetID)
}

// HasInitialBackup implements core.GatewaySandboxCapability.
func (p *HermesPlugin) HasInitialBackup() string {
	return HasInitialBackupInternal()
}

// RestoreToInitialConfig implements core.GatewaySandboxCapability.
func (p *HermesPlugin) RestoreToInitialConfig() string {
	return RestoreToInitialConfigInternal()
}

// TestModelConnection implements core.ModelConnectionCapability.
func (p *HermesPlugin) TestModelConnection(configJSON string) string {
	return modelfactory.TestModelConnectionInternal(configJSON)
}

// OnAppExit implements core.ApplicationLifecycleCapability.
func (p *HermesPlugin) OnAppExit(assetID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	assetID = strings.TrimSpace(assetID)
	if assetID != "" {
		if status, ok := p.protectionStatuses[assetID]; ok {
			status.Running = false
			status.ProxyRunning = false
			p.protectionStatuses[assetID] = status
		}
	}
	payload, _ := json.Marshal(map[string]interface{}{"success": true, "asset_id": assetID})
	return string(payload)
}

// RestoreBotDefaultState implements core.ApplicationLifecycleCapability.
func (p *HermesPlugin) RestoreBotDefaultState(assetID string) string {
	assetID = strings.TrimSpace(assetID)
	backupDir := ""
	if pm := core.GetPathManager(); pm.IsInitialized() {
		backupDir = pm.GetBackupDir()
	}
	if backupDir == "" {
		homeDir, _ := os.UserHomeDir()
		backupDir = filepath.Join(homeDir, ".botsec", "backups")
	}
	payload, _ := json.Marshal(restoreHermesConfigByAsset(backupDir, assetID))
	return string(payload)
}

func GetHermesPlugin() *HermesPlugin {
	return hermesPlugin
}
