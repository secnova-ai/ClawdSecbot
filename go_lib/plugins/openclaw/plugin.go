package openclaw

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
)

// OpenclawPlugin Openclaw安全插件
// 实现core.BotPlugin接口，提供Openclaw Bot的资产发现、风险评估和防护控制
type OpenclawPlugin struct {
	mu sync.RWMutex

	// protectionStatuses tracks per-instance protection state keyed by asset ID
	protectionStatuses map[string]core.ProtectionStatus
}

// 全局OpenclawPlugin实例
var openclawPlugin *OpenclawPlugin

const openclawPluginID = "openclaw"

func init() {
	// 创建插件实例并注册到PluginManager
	openclawPlugin = &OpenclawPlugin{
		protectionStatuses: make(map[string]core.ProtectionStatus),
	}
	core.GetPluginManager().Register(openclawPlugin)
	logging.Info("Openclaw plugin registered")
}

// GetAssetName 返回该插件能识别和防护的资产名称
func (p *OpenclawPlugin) GetAssetName() string {
	return openclawAssetName
}

// GetID returns a stable plugin ID for host-side metadata aggregation.
func (p *OpenclawPlugin) GetID() string {
	return openclawPluginID
}

// RequiresBotModelConfig reports whether Openclaw protection depends on
// explicit bot model configuration.
func (p *OpenclawPlugin) RequiresBotModelConfig() bool {
	return true
}

// GetManifest returns canonical plugin manifest metadata.
func (p *OpenclawPlugin) GetManifest() plugin_sdk.PluginManifest {
	return plugin_sdk.PluginManifest{
		PluginID:    openclawPluginID,
		BotType:     strings.ToLower(openclawAssetName),
		DisplayName: openclawAssetName,
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

// GetAssetUISchema returns the canonical schema for Openclaw asset rendering.
func (p *OpenclawPlugin) GetAssetUISchema() *plugin_sdk.AssetUISchema {
	return &plugin_sdk.AssetUISchema{
		ID:      "openclaw.asset.v1",
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
func (p *OpenclawPlugin) ScanAssets() ([]core.Asset, error) {
	logging.Info("OpenclawPlugin: Scanning assets")
	assetScanner := NewOpenclawAssetScanner(GetConfigPath())
	return assetScanner.ScanAssets()
}

// AssessRisks 对已发现的资产进行风险评估
func (p *OpenclawPlugin) AssessRisks(scannedHashes map[string]bool) ([]core.Risk, error) {
	logging.Info("OpenclawPlugin: Assessing risks")

	risks := []core.Risk{}

	configPath, err := findConfigPath()
	if err != nil {
		logging.Warning("OpenclawPlugin: Cannot find config path: %v", err)
		return risks, nil
	}

	// 执行各项风险检查
	checkPermissions(configPath, &risks)

	config, rawConfig, err := loadConfig(configPath)
	if err != nil {
		logging.Warning("OpenclawPlugin: Cannot load config: %v", err)
		return risks, nil
	}

	checkNetworkExposure(*config, &risks)
	checkSandbox(*config, rawConfig, &risks)
	checkLogging(*config, configPath, &risks)
	checkDangerousGatewayFlags(*config, rawConfig, &risks)

	version := getOpenClawVersion()
	checkOneClickRCEVulnerabilityByVersion(version, &risks)
	checkConfigPatchLevelByVersion(version, &risks)

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

	logging.Info("OpenclawPlugin: Risk assessment completed, found %d risks", len(risks))
	return risks, nil
}

// MitigateRisk handles risk mitigation requests for Openclaw-specific risks.
func (p *OpenclawPlugin) MitigateRisk(riskInfo string) string {
	return MitigateRiskDispatch(riskInfo)
}

// StartProtection 启动指定资产实例的防护
// 注意：实际的代理启动通过StartProtectionProxy FFI完成，此方法用于更新插件状态
func (p *OpenclawPlugin) StartProtection(assetID string, config core.ProtectionConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	logging.Info("OpenclawPlugin: StartProtection called, assetID=%s, config=%+v", assetID, config)

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
func (p *OpenclawPlugin) StopProtection(assetID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	logging.Info("OpenclawPlugin: StopProtection called, assetID=%s", assetID)

	p.protectionStatuses[assetID] = core.ProtectionStatus{
		Running:      false,
		ProxyRunning: false,
	}

	return nil
}

// GetProtectionStatus 获取指定资产实例的防护状态
func (p *OpenclawPlugin) GetProtectionStatus(assetID string) core.ProtectionStatus {
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

// GetOpenclawPlugin 获取全局OpenclawPlugin实例
func GetOpenclawPlugin() *OpenclawPlugin {
	return openclawPlugin
}

// StartSkillSecurityScan implements core.SkillScanCapability.
func (p *OpenclawPlugin) StartSkillSecurityScan(skillPath, modelConfigJSON string) string {
	return StartSkillSecurityScanInternal(skillPath, modelConfigJSON)
}

// GetSkillSecurityScanLog implements core.SkillScanCapability.
func (p *OpenclawPlugin) GetSkillSecurityScanLog(scanID string) string {
	return GetSkillSecurityScanLogInternal(scanID)
}

// GetSkillSecurityScanResult implements core.SkillScanCapability.
func (p *OpenclawPlugin) GetSkillSecurityScanResult(scanID string) string {
	return GetSkillSecurityScanResultInternal(scanID)
}

// CancelSkillSecurityScan implements core.SkillScanCapability.
func (p *OpenclawPlugin) CancelSkillSecurityScan(scanID string) string {
	return CancelSkillSecurityScanInternal(scanID)
}

// StartBatchSkillScan implements core.SkillScanCapability.
func (p *OpenclawPlugin) StartBatchSkillScan() string {
	return StartBatchSkillScanInternal()
}

// GetBatchSkillScanLog implements core.SkillScanCapability.
func (p *OpenclawPlugin) GetBatchSkillScanLog(batchID string) string {
	return GetBatchScanLogInternal(batchID)
}

// GetBatchSkillScanResults implements core.SkillScanCapability.
func (p *OpenclawPlugin) GetBatchSkillScanResults(batchID string) string {
	return GetBatchScanResultsInternal(batchID)
}

// CancelBatchSkillScan implements core.SkillScanCapability.
func (p *OpenclawPlugin) CancelBatchSkillScan(batchID string) string {
	return CancelBatchSkillScanInternal(batchID)
}

// TestModelConnection implements core.ModelConnectionCapability.
func (p *OpenclawPlugin) TestModelConnection(configJSON string) string {
	return TestModelConnectionInternal(configJSON)
}

// DeleteSkill implements core.SkillManagementCapability.
func (p *OpenclawPlugin) DeleteSkill(skillPath string) string {
	return DeleteSkillInternal(skillPath)
}

// SyncGatewaySandbox implements core.GatewaySandboxCapability.
func (p *OpenclawPlugin) SyncGatewaySandbox() string {
	return SyncGatewaySandboxInternal()
}

// SyncGatewaySandboxByAsset implements core.GatewaySandboxCapability.
func (p *OpenclawPlugin) SyncGatewaySandboxByAsset(assetID string) string {
	return SyncGatewaySandboxByAssetInternal(assetID)
}

// HasInitialBackup implements core.GatewaySandboxCapability.
func (p *OpenclawPlugin) HasInitialBackup() string {
	return HasInitialBackupInternal()
}

// RestoreToInitialConfig implements core.GatewaySandboxCapability.
func (p *OpenclawPlugin) RestoreToInitialConfig() string {
	return RestoreToInitialConfigInternal()
}

// OnAppExit implements core.ApplicationLifecycleCapability.
func (p *OpenclawPlugin) OnAppExit(assetID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	assetID = strings.TrimSpace(assetID)
	logging.Info("[Openclaw] OnAppExit: assetID=%s", assetID)

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
		"message":  "openclaw exit callback completed",
	})
	if err != nil {
		return `{"success":false,"error":"marshal error"}`
	}
	return string(payload)
}

// RestoreBotDefaultState implements core.ApplicationLifecycleCapability.
func (p *OpenclawPlugin) RestoreBotDefaultState(assetID string) string {
	result := RestoreBotDefaultStateByAsset(strings.TrimSpace(assetID))
	payload, err := json.Marshal(result)
	if err != nil {
		return `{"success":false,"error":"marshal error"}`
	}
	return string(payload)
}

// OnBeforeProxyStop 实现防护停止前钩子
// 在代理停止前执行，恢复原始配置
func (p *OpenclawPlugin) OnBeforeProxyStop(ctx *core.ProtectionContext) {
	logging.Info("[Openclaw] OnBeforeProxyStop: assetID=%s", ctx.AssetID)

	// 获取备份目录
	backupDir := ctx.BackupDir
	if backupDir == "" {
		homeDir, _ := os.UserHomeDir()
		backupDir = filepath.Join(homeDir, ".botsec", "backups")
	}

	_ = backupDir // 保留兼容路径推导，当前退出恢复不依赖初始整文件备份。

	// 恢复 Bot 默认配置，仅移除注入的 clawdsecbot-* 路由项。
	result := RestoreBotDefaultStateByAsset(ctx.AssetID)
	if result.Success {
		logging.Info("[Openclaw] Bot default state restored: %s", result.Message)
	} else {
		logging.Warning("[Openclaw] Bot default state restore failed: %s", result.Error)
	}
}

// parseBotModelConfig 从 repository.ProtectionConfig 解析 BotModelConfig
func parseBotModelConfig(config *repository.ProtectionConfig) *BotModelConfig {
	if config == nil || config.BotModelConfig == nil {
		logging.Warning("[Openclaw] BotModelConfig is nil")
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
func (p *OpenclawPlugin) OnProtectionStart(ctx *core.ProtectionContext) (map[string]interface{}, error) {
	logging.Info("[Openclaw] OnProtectionStart: assetID=%s, proxyPort=%d", ctx.AssetID, ctx.ProxyPort)

	// 从数据库读取 BotModel 配置
	repo := repository.NewProtectionRepository(nil)
	config, err := repo.GetProtectionConfig(ctx.AssetID)
	if err != nil {
		logging.Error("[Openclaw] Failed to get protection config from DB: %v", err)
		return nil, fmt.Errorf("failed to get protection config: %w", err)
	}
	if config == nil {
		logging.Error("[Openclaw] No protection config found in DB for Openclaw/%s", ctx.AssetID)
		return nil, fmt.Errorf("no protection config found")
	}

	// 解析 BotModel 配置
	botModelConfig := parseBotModelConfig(config)
	if botModelConfig == nil {
		logging.Error("[Openclaw] Failed to parse bot model config")
		return nil, fmt.Errorf("failed to parse bot model config")
	}

	// 执行 gateway 操作：更新配置 + 重启进程
	logging.Info("[Openclaw] Starting gateway with proxyPort=%d", ctx.ProxyPort)
	return startGatewayWithProxy(ctx.ProxyPort, botModelConfig, ctx.BackupDir, ctx.AssetID)
}
