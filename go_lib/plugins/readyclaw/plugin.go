package readyclaw

import (
	"strings"
	"sync"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/plugin_sdk"
)

const (
	readyclawAssetName = "ReadyClaw"
	readyclawPluginID  = "readyclaw"
)

// Plugin 负责 ReadyClaw 魔改版终端资产识别与 LLM 上游地址接管。
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
	logging.Info("ReadyClaw plugin registered")
}

// GetAssetName 返回前端和策略侧使用的资产展示类型。
func (p *Plugin) GetAssetName() string { return readyclawAssetName }

// GetID 返回插件稳定 ID，用于注册、路由和实例 ID 前缀。
func (p *Plugin) GetID() string { return readyclawPluginID }

// GetManifest 声明 ReadyClaw 插件能力，供 WebBridge 和前端聚合展示。
func (p *Plugin) GetManifest() plugin_sdk.PluginManifest {
	return plugin_sdk.PluginManifest{
		PluginID:    readyclawPluginID,
		BotType:     strings.ToLower(readyclawAssetName),
		DisplayName: readyclawAssetName,
		APIVersion:  "v1",
		Capabilities: []string{
			"scan",
			"risk_assessment",
			"protection_proxy",
			"audit_log",
		},
		SupportedPlatforms: []string{"windows", "linux", "macos"},
	}
}

// GetAssetUISchema 提供 ReadyClaw 资产卡片的声明式字段。
func (p *Plugin) GetAssetUISchema() *plugin_sdk.AssetUISchema {
	return &plugin_sdk.AssetUISchema{
		ID:      "readyclaw.asset.v1",
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
					{Label: "Config Path", ValueRef: "metadata.config_path"},
					{Label: "Base URL", ValueRef: "metadata.llm_base_url"},
					{Label: "Model", ValueRef: "metadata.model_name"},
				},
			},
		},
		Actions: []plugin_sdk.AssetUIAction{
			{Action: "start_protection", LabelKey: "asset.action.start_protection", Variant: "primary"},
			{Action: "stop_protection", LabelKey: "asset.action.stop_protection", Variant: "danger"},
		},
	}
}

// RequiresBotModelConfig 返回 false，ReadyClaw 可从自身配置解析真实上游。
func (p *Plugin) RequiresBotModelConfig() bool { return false }

// ScanAssets 通过配置、进程、服务和端口多信号扫描 ReadyClaw 资产实例。
func (p *Plugin) ScanAssets() ([]core.Asset, error) {
	configPath := ""
	if _, _, path, err := loadConfig(); err == nil {
		configPath = path
	}
	return newAssetScanner(configPath).scan()
}

// GetMainProcessPID 暂不从插件内解析进程，避免与统一 detector 的运行态评分重复。
func (p *Plugin) GetMainProcessPID(asset core.Asset) (int, bool) {
	return 0, false
}

// AssessRisks 第一阶段只接入保护闭环，深度风险评估后续复用 OpenClaw 模板扩展。
func (p *Plugin) AssessRisks(scannedHashes map[string]bool, assets []core.Asset) ([]core.Risk, error) {
	_ = scannedHashes
	_ = assets
	return []core.Risk{}, nil
}

// MitigateRisk 第一阶段不实现单项修复动作。
func (p *Plugin) MitigateRisk(riskInfo string) string {
	_ = riskInfo
	return `{"success":false,"message":"ReadyClaw mitigation is not implemented yet"}`
}

// GetVulnInfoJSON 返回空漏洞定义，避免误报 NanoClaw/ReadyClaw 专项漏洞。
func (p *Plugin) GetVulnInfoJSON() []byte { return []byte("{}") }

// CompareVulnerabilityVersion 当前没有 ReadyClaw 专项版本漏洞基线。
func (p *Plugin) CompareVulnerabilityVersion(current, target string) (int, bool) {
	_ = current
	_ = target
	return 0, false
}

// StartProtection 更新插件内保护状态快照，实际代理接管由生命周期钩子完成。
func (p *Plugin) StartProtection(assetID string, config core.ProtectionConfig) error {
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

// StopProtection 清空指定 ReadyClaw 实例的保护运行态。
func (p *Plugin) StopProtection(assetID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.protectionStatuses[assetID] = core.ProtectionStatus{
		Running:      false,
		ProxyRunning: false,
	}
	return nil
}

// GetProtectionStatus 读取指定 ReadyClaw 实例的保护状态快照。
func (p *Plugin) GetProtectionStatus(assetID string) core.ProtectionStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.protectionStatuses[assetID]
}

// OnProtectionStart 在代理启动前改写 ReadyClaw 上游 LLM 地址到本地保护代理。
func (p *Plugin) OnProtectionStart(ctx *core.ProtectionContext) (map[string]interface{}, error) {
	return applyReadyClawProxyConfig(ctx)
}

// OnBeforeProxyStop 在代理停止前按首次备份恢复 ReadyClaw 原始配置。
func (p *Plugin) OnBeforeProxyStop(ctx *core.ProtectionContext) {
	backupDir := ""
	if ctx != nil {
		backupDir = ctx.BackupDir
	}
	if err := restoreReadyClawConfig(backupDir); err != nil {
		logging.Warning("[ReadyClaw] Restore config failed: %v", err)
	}
}

// ResolveProxyForwardingTarget 从 ReadyClaw 原始配置解析代理真实上游。
func (p *Plugin) ResolveProxyForwardingTarget(assetID string) (*core.ProxyForwardingTarget, error) {
	_ = assetID
	return resolveReadyClawForwardingTarget("")
}
