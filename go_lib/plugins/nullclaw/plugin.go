package nullclaw

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"go_lib/core"
	"go_lib/core/logging"
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
// Nullclaw can resolve forwarding target from its own runtime config,
// so bot model config is optional.
func (p *NullclawPlugin) RequiresBotModelConfig() bool {
	return false
}

// ResolveProxyForwardingTarget resolves proxy forwarding provider/base_url/api_key
// from current Nullclaw runtime config.
func (p *NullclawPlugin) ResolveProxyForwardingTarget(assetID string) (*core.ProxyForwardingTarget, error) {
	_ = assetID
	botConfig, err := resolveBotModelFromActiveConfig()
	if err != nil {
		return nil, err
	}
	return &core.ProxyForwardingTarget{
		Provider: botConfig.Provider,
		BaseURL:  botConfig.BaseURL,
		APIKey:   botConfig.APIKey,
	}, nil
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

// AssessRisks 对已发现的资产进行风险评估
func (p *NullclawPlugin) AssessRisks(scannedHashes map[string]bool) ([]core.Risk, error) {
	logging.Info("NullclawPlugin: Assessing risks")

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
	proxy := GetProxyProtectionByAsset(nullclawAssetName, assetID)
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

// OnBeforeProxyStop 实现防护停止前钩子
// 在代理停止前执行，恢复原始配置
func (p *NullclawPlugin) OnBeforeProxyStop(ctx *core.ProtectionContext) {
	logging.Info("[Nullclaw] OnBeforeProxyStop: assetID=%s", ctx.AssetID)

	// 获取备份目录
	backupDir := ctx.BackupDir
	if backupDir == "" {
		homeDir, _ := os.UserHomeDir()
		backupDir = filepath.Join(homeDir, ".botsec", "backups")
	}

	// 恢复原始配置
	result := RestoreToInitialConfigByAsset(backupDir, ctx.AssetID)
	if result.Success {
		logging.Info("[Nullclaw] Config restored to initial state: %s", result.Message)
	} else {
		logging.Warning("[Nullclaw] Config restore failed: %s", result.Error)
	}
}

func extractProviderAPIKey(rawConfig map[string]interface{}, providerName string) string {
	if rawConfig == nil {
		return ""
	}
	modelsMap, ok := rawConfig["models"].(map[string]interface{})
	if !ok {
		return ""
	}
	providersMap, ok := modelsMap["providers"].(map[string]interface{})
	if !ok {
		return ""
	}
	providerMap, ok := providersMap[providerName].(map[string]interface{})
	if !ok {
		return ""
	}

	raw, ok := providerMap["api_key"]
	if !ok {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		// Structured credentials (for example service account JSON) are
		// intentionally ignored here because forwarding providers expect
		// plain API keys/tokens.
		return ""
	}
}

func resolveBackupDir() string {
	pm := core.GetPathManager()
	if pm.IsInitialized() {
		return pm.GetBackupDir()
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".botsec", "backups")
}

func isLikelyLocalProxyURL(baseURL string) bool {
	raw := strings.TrimSpace(baseURL)
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return false
	}

	host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	if host != "127.0.0.1" && host != "localhost" && host != "::1" && host != "loopback" {
		return false
	}

	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		return false
	}
	// Proxy manager allocates ports in [13436, 13535].
	return port >= 13436 && port <= 13535
}

func resolveBotModelFromConfigPath(configPath string) (*BotModelConfig, error) {
	config, rawConfig, err := loadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config failed: %w", err)
	}
	primaryModel, err := getPrimaryModelFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("read primary model failed: %w", err)
	}

	providerName, baseURL, err := GetCurrentModelProvider(config, rawConfig)
	if err != nil {
		return nil, fmt.Errorf("resolve current model provider failed: %w", err)
	}
	if strings.TrimSpace(providerName) == "" {
		return nil, fmt.Errorf("resolved provider is empty")
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("resolved base_url is empty")
	}

	return &BotModelConfig{
		Provider: providerName,
		BaseURL:  baseURL,
		APIKey:   extractProviderAPIKey(rawConfig, providerName),
		Model:    primaryModel,
	}, nil
}

func resolveBotModelFromActiveConfig() (*BotModelConfig, error) {
	configPath, err := findConfigPath()
	if err != nil {
		return nil, fmt.Errorf("find config path failed: %w", err)
	}

	active, err := resolveBotModelFromConfigPath(configPath)
	if err != nil {
		return nil, err
	}

	// If active config already points to a likely local protection proxy
	// endpoint (e.g. leftover from an unclean shutdown), prefer the initial
	// backup to avoid proxy->proxy self-looping.
	if isLikelyLocalProxyURL(active.BaseURL) {
		backupDir := resolveBackupDir()
		if hasInitialBackup(backupDir) {
			backupPath := getInitialBackupPath(backupDir)
			if restored, restoreErr := resolveBotModelFromConfigPath(backupPath); restoreErr == nil {
				if !isLikelyLocalProxyURL(restored.BaseURL) {
					logging.Warning("[Nullclaw] Active base_url looks like local proxy (%s), fallback to initial backup (%s)",
						active.BaseURL, restored.BaseURL)
					return restored, nil
				}
				logging.Warning("[Nullclaw] Initial backup base_url is also local proxy-like: %s", restored.BaseURL)
			} else {
				logging.Warning("[Nullclaw] Failed to resolve bot model from initial backup: %v", restoreErr)
			}
		} else {
			logging.Warning("[Nullclaw] Active base_url looks like local proxy (%s) but no initial backup found", active.BaseURL)
		}
	}

	return active, nil
}

// OnProtectionStart 实现防护启动钩子
// 在代理启动前执行，执行 gateway 配置更新和进程重启
func (p *NullclawPlugin) OnProtectionStart(ctx *core.ProtectionContext) (map[string]interface{}, error) {
	logging.Info("[Nullclaw] OnProtectionStart: assetID=%s, proxyPort=%d", ctx.AssetID, ctx.ProxyPort)

	// Nullclaw does not require DB bot model config.
	// Always resolve forwarding target from active runtime config.
	botModelConfig, err := resolveBotModelFromActiveConfig()
	if err != nil {
		logging.Error("[Nullclaw] Failed to resolve forwarding target from active config: %v", err)
		return nil, fmt.Errorf("failed to resolve forwarding target from active config: %w", err)
	}
	logging.Info("[Nullclaw] Using active config forwarding target: provider=%s model=%s",
		botModelConfig.Provider, botModelConfig.Model)

	// 执行 gateway 操作：更新配置 + 重启进程
	logging.Info("[Nullclaw] Starting gateway with proxyPort=%d", ctx.ProxyPort)
	return startGatewayWithProxy(ctx.ProxyPort, botModelConfig, ctx.BackupDir, ctx.AssetID)
}
