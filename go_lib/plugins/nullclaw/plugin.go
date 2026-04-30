package nullclaw

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/repository"
	"go_lib/plugin_sdk"
)

// NullclawPlugin Nullclaw安全插件
// 实现core.BotPlugin接口，提供Nullclaw Bot的资产发现、风险评估和防护控制
type NullclawPlugin struct {
	mu sync.RWMutex

	// protectionStatuses tracks per-instance protection state keyed by asset ID
	protectionStatuses map[string]core.ProtectionStatus
}

// 全局NullclawPlugin实例
var nullclawPlugin *NullclawPlugin

const nullclawPluginID = "nullclaw"

func init() {
	// 创建插件实例并注册到PluginManager
	nullclawPlugin = &NullclawPlugin{
		protectionStatuses: make(map[string]core.ProtectionStatus),
	}
	core.GetPluginManager().Register(nullclawPlugin)
	logging.Info("Nullclaw plugin registered")
}

// GetAssetName 返回该插件能识别和防护的资产名称
func (p *NullclawPlugin) GetAssetName() string {
	return nullclawAssetName
}

// GetID returns a stable plugin ID for host-side metadata aggregation.
func (p *NullclawPlugin) GetID() string {
	return nullclawPluginID
}

// RequiresBotModelConfig reports whether Nullclaw protection depends on
// explicit bot model configuration.
//
// Nullclaw currently relies on explicit bot model config from protection settings.
func (p *NullclawPlugin) RequiresBotModelConfig() bool {
	return true
}

// GetManifest returns canonical plugin manifest metadata.
func (p *NullclawPlugin) GetManifest() plugin_sdk.PluginManifest {
	return plugin_sdk.PluginManifest{
		PluginID:    nullclawPluginID,
		BotType:     strings.ToLower(nullclawAssetName),
		DisplayName: nullclawAssetName,
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

// GetAssetUISchema returns the canonical schema for Nullclaw asset rendering.
func (p *NullclawPlugin) GetAssetUISchema() *plugin_sdk.AssetUISchema {
	return &plugin_sdk.AssetUISchema{
		ID:      "nullclaw.asset.v1",
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
					{LabelKey: "asset.field.port", ValueRef: "port"},
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

// ScanAssets 执行资产扫描，返回检测到的资产列表
func (p *NullclawPlugin) ScanAssets() ([]core.Asset, error) {
	logging.Info("NullclawPlugin: Scanning assets")
	assetScanner := NewNullclawAssetScanner(GetConfigPath())
	return assetScanner.ScanAssets()
}

func (p *NullclawPlugin) GetMainProcessPID(asset core.Asset) (int, bool) {
	return 0, false
}

// AssessRisks 对已发现的资产进行风险评估
func (p *NullclawPlugin) AssessRisks(scannedHashes map[string]bool, assets []core.Asset) ([]core.Risk, error) {
	logging.Info("NullclawPlugin: Assessing risks")
	_ = assets

	risks := []core.Risk{}

	configPath, err := findConfigPath()
	if err != nil {
		logging.Warning("NullclawPlugin: Cannot find config path: %v", err)
		return risks, nil
	}

	// 执行各项风险检查
	checkPermissions(configPath, &risks)

	config, rawConfig, err := loadConfig(configPath)
	if err != nil {
		logging.Warning("NullclawPlugin: Cannot load config: %v", err)
		return risks, nil
	}

	checkNetworkExposure(*config, &risks)
	checkSandbox(*config, rawConfig, &risks)
	checkLogging(*config, configPath, &risks)
	// 已隐藏：配置文件中明文密钥检测，不再向用户展示此项风险
	// checkCredentialsInConfig(configPath, &risks)

	// 检查未扫描的skills
	checkUnscannedSkills(scannedHashes, &risks)

	// 注入缓解建议模板
	for i := range risks {
		if tmpl, ok := templates[risks[i].ID]; ok {
			risks[i].Mitigation = tmpl
		}
	}

	logging.Info("NullclawPlugin: Risk assessment completed, found %d risks", len(risks))
	return risks, nil
}

// MitigateRisk handles risk mitigation requests for Nullclaw-specific risks.
func (p *NullclawPlugin) MitigateRisk(riskInfo string) string {
	return MitigateRiskDispatch(riskInfo)
}

func (p *NullclawPlugin) GetVulnInfoJSON() []byte {
	return GetVulInfoJSON()
}

func (p *NullclawPlugin) CompareVulnerabilityVersion(current, target string) (int, bool) {
	return compareNullclawVersion(current, target)
}

// StartProtection 启动指定资产实例的防护
// 注意：实际的代理启动通过StartProtectionProxy FFI完成，此方法用于更新插件状态
func (p *NullclawPlugin) StartProtection(assetID string, config core.ProtectionConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	logging.Info("NullclawPlugin: StartProtection called, assetID=%s, config=%+v", assetID, config)

	p.protectionStatuses[assetID] = core.ProtectionStatus{
		Running:       config.ProxyEnabled,
		ProxyRunning:  config.ProxyEnabled,
		ProxyPort:     config.ProxyPort,
		SandboxActive: config.SandboxEnabled,
		AuditOnly:     config.AuditOnly,
	}

	return nil
}

// StopProtection 停止指定资产实例的防护
// 注意：实际的代理停止通过StopProtectionProxy FFI完成，此方法用于更新插件状态
func (p *NullclawPlugin) StopProtection(assetID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	logging.Info("NullclawPlugin: StopProtection called, assetID=%s", assetID)

	p.protectionStatuses[assetID] = core.ProtectionStatus{
		Running:      false,
		ProxyRunning: false,
	}

	return nil
}

// GetProtectionStatus 获取指定资产实例的防护状态
func (p *NullclawPlugin) GetProtectionStatus(assetID string) core.ProtectionStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Sync with live proxy state
	proxy := GetProxyProtectionByAsset(assetID)
	if proxy != nil {
		running := proxy.IsRunning()
		cached := p.protectionStatuses[assetID]
		return core.ProtectionStatus{
			Running:       running,
			ProxyRunning:  running,
			ProxyPort:     proxy.GetPort(),
			SandboxActive: cached.SandboxActive,
			AuditOnly:     cached.AuditOnly,
		}
	}

	return p.protectionStatuses[assetID]
}

// GetNullclawPlugin 获取全局NullclawPlugin实例
func GetNullclawPlugin() *NullclawPlugin {
	return nullclawPlugin
}

// StartSkillSecurityScan implements core.SkillScanCapability.
func (p *NullclawPlugin) StartSkillSecurityScan(skillPath, modelConfigJSON string) string {
	return StartSkillSecurityScanInternal(skillPath, modelConfigJSON)
}

// GetSkillSecurityScanLog implements core.SkillScanCapability.
func (p *NullclawPlugin) GetSkillSecurityScanLog(scanID string) string {
	return GetSkillSecurityScanLogInternal(scanID)
}

// GetSkillSecurityScanResult implements core.SkillScanCapability.
func (p *NullclawPlugin) GetSkillSecurityScanResult(scanID string) string {
	return GetSkillSecurityScanResultInternal(scanID)
}

// CancelSkillSecurityScan implements core.SkillScanCapability.
func (p *NullclawPlugin) CancelSkillSecurityScan(scanID string) string {
	return CancelSkillSecurityScanInternal(scanID)
}

// StartBatchSkillScan implements core.SkillScanCapability.
func (p *NullclawPlugin) StartBatchSkillScan() string {
	return StartBatchSkillScanInternal()
}

// GetBatchSkillScanLog implements core.SkillScanCapability.
func (p *NullclawPlugin) GetBatchSkillScanLog(batchID string) string {
	return GetBatchScanLogInternal(batchID)
}

// GetBatchSkillScanResults implements core.SkillScanCapability.
func (p *NullclawPlugin) GetBatchSkillScanResults(batchID string) string {
	return GetBatchScanResultsInternal(batchID)
}

// CancelBatchSkillScan implements core.SkillScanCapability.
func (p *NullclawPlugin) CancelBatchSkillScan(batchID string) string {
	return CancelBatchSkillScanInternal(batchID)
}

// TestModelConnection implements core.ModelConnectionCapability.
func (p *NullclawPlugin) TestModelConnection(configJSON string) string {
	return TestModelConnectionInternal(configJSON)
}

// DeleteSkill implements core.SkillManagementCapability.
func (p *NullclawPlugin) DeleteSkill(skillPath string) string {
	return DeleteSkillInternal(skillPath)
}

// SyncGatewaySandbox implements core.GatewaySandboxCapability.
func (p *NullclawPlugin) SyncGatewaySandbox() string {
	return SyncGatewaySandboxInternal()
}

// SyncGatewaySandboxByAsset implements core.GatewaySandboxCapability.
func (p *NullclawPlugin) SyncGatewaySandboxByAsset(assetID string) string {
	return SyncGatewaySandboxByAssetInternal(assetID)
}

// HasInitialBackup implements core.GatewaySandboxCapability.
func (p *NullclawPlugin) HasInitialBackup() string {
	return HasInitialBackupInternal()
}

// RestoreToInitialConfig implements core.GatewaySandboxCapability.
func (p *NullclawPlugin) RestoreToInitialConfig() string {
	return RestoreToInitialConfigInternal()
}

// OnAppExit implements core.ApplicationLifecycleCapability.
func (p *NullclawPlugin) OnAppExit(assetID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	assetID = strings.TrimSpace(assetID)
	logging.Info("[Nullclaw] OnAppExit: assetID=%s", assetID)

	if assetID != "" {
		if status, ok := p.protectionStatuses[assetID]; ok {
			status.Running = false
			status.ProxyRunning = false
			p.protectionStatuses[assetID] = status
		}
	}

	payload, err := json.Marshal(map[string]interface{}{
		"success":  true,
		"asset_id": assetID,
		"message":  "nullclaw exit callback completed",
	})
	if err != nil {
		return `{"success":false,"error":"marshal error"}`
	}
	return string(payload)
}

// RestoreBotDefaultState implements core.ApplicationLifecycleCapability.
// Nullclaw currently falls back to the existing initial-backup restoration flow.
func (p *NullclawPlugin) RestoreBotDefaultState(assetID string) string {
	assetID = strings.TrimSpace(assetID)
	backupDir := ""
	homeDir, err := os.UserHomeDir()
	if err == nil {
		backupDir = core.ResolveBackupDir(homeDir)
	}
	result := RestoreToInitialConfigByAsset(backupDir, assetID)
	payload, err := json.Marshal(result)
	if err != nil {
		return `{"success":false,"error":"marshal error"}`
	}
	return string(payload)
}

// OnBeforeProxyStop 实现防护停止前钩子
// 在代理停止前执行，恢复原始配置
func (p *NullclawPlugin) OnBeforeProxyStop(ctx *core.ProtectionContext) {
	logging.Info("[Nullclaw] OnBeforeProxyStop: assetID=%s", ctx.AssetID)

	// 获取备份目录
	backupDir := ctx.BackupDir
	if backupDir == "" {
		homeDir, _ := os.UserHomeDir()
		backupDir = core.ResolveBackupDir(homeDir)
	}

	// 恢复原始配置
	result := RestoreToInitialConfigByAsset(backupDir, ctx.AssetID)
	if result.Success {
		logging.Info("[Nullclaw] Config restored to initial state: %s", result.Message)
	} else {
		logging.Warning("[Nullclaw] Config restore failed: %s", result.Error)
	}
}

// parseBotModelConfig 从 repository.ProtectionConfig 解析 BotModelConfig
func parseBotModelConfig(config *repository.ProtectionConfig) *BotModelConfig {
	if config == nil || config.BotModelConfig == nil {
		logging.Warning("[Nullclaw] BotModelConfig is nil")
		return nil
	}

	return &BotModelConfig{
		Provider:  config.BotModelConfig.Provider,
		BaseURL:   config.BotModelConfig.BaseURL,
		APIKey:    config.BotModelConfig.APIKey,
		Model:     config.BotModelConfig.Model,
		SecretKey: config.BotModelConfig.SecretKey,
	}
}

// OnProtectionStart 实现防护启动钩子
// 在代理启动前执行，执行 gateway 配置更新和进程重启
func (p *NullclawPlugin) OnProtectionStart(ctx *core.ProtectionContext) (map[string]interface{}, error) {
	logging.Info("[Nullclaw] OnProtectionStart: assetID=%s, proxyPort=%d", ctx.AssetID, ctx.ProxyPort)

	// 从数据库读取 BotModel 配置
	repo := repository.NewProtectionRepository(nil)
	config, err := repo.GetProtectionConfig(ctx.AssetID)
	if err != nil {
		logging.Error("[Nullclaw] Failed to get protection config from DB: %v", err)
		return nil, fmt.Errorf("failed to get protection config: %w", err)
	}
	if config == nil {
		logging.Error("[Nullclaw] No protection config found in DB for Nullclaw/%s", ctx.AssetID)
		return nil, fmt.Errorf("no protection config found")
	}

	// 解析 BotModel 配置
	botModelConfig := parseBotModelConfig(config)
	if botModelConfig == nil {
		logging.Error("[Nullclaw] Failed to parse bot model config")
		return nil, fmt.Errorf("failed to parse bot model config")
	}

	// 执行 gateway 操作：更新配置 + 重启进程
	logging.Info("[Nullclaw] Starting gateway with proxyPort=%d", ctx.ProxyPort)
	return startGatewayWithProxy(ctx.ProxyPort, botModelConfig, ctx.BackupDir, ctx.AssetID)
}
