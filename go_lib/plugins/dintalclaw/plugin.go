package dintalclaw

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

// DintalclawPlugin 政务龙虾安全插件
// 实现core.BotPlugin接口，提供 DinTalClaw Bot 的资产发现、风险评估和防护控制
type DintalclawPlugin struct {
	mu                 sync.RWMutex
	protectionStatuses map[string]core.ProtectionStatus
}

var dintalclawPlugin *DintalclawPlugin

const dintalclawPluginID = "dintalclaw"

func init() {
	dintalclawPlugin = &DintalclawPlugin{
		protectionStatuses: make(map[string]core.ProtectionStatus),
	}
	core.GetPluginManager().Register(dintalclawPlugin)
	logging.Info("Dintalclaw plugin registered")
}

// GetAssetName 返回该插件能识别和防护的资产名称
func (p *DintalclawPlugin) GetAssetName() string {
	return dintalclawAssetName
}

// GetID 返回稳定的插件 ID
func (p *DintalclawPlugin) GetID() string {
	return dintalclawPluginID
}

// RequiresBotModelConfig 标记需要显式的 Bot 模型配置来启动防护
func (p *DintalclawPlugin) RequiresBotModelConfig() bool {
	return true
}

// GetManifest 返回插件元数据清单
func (p *DintalclawPlugin) GetManifest() plugin_sdk.PluginManifest {
	return plugin_sdk.PluginManifest{
		PluginID:    dintalclawPluginID,
		BotType:     strings.ToLower(dintalclawAssetName),
		DisplayName: "政务龙虾",
		APIVersion:  "v1",
		Capabilities: []string{
			"scan",
			"risk_assessment",
			"mitigation",
			"protection_proxy",
		},
		SupportedPlatforms: []string{"windows", "linux", "macos"},
	}
}

// GetAssetUISchema 返回资产卡片 UI 渲染结构
func (p *DintalclawPlugin) GetAssetUISchema() *plugin_sdk.AssetUISchema {
	return &plugin_sdk.AssetUISchema{
		ID:      "dintalclaw.asset.v1",
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
					{LabelKey: "asset.field.listeners", ValueRef: "metadata.runtime_listeners"},
					{LabelKey: "asset.field.config_path", ValueRef: "metadata.config_path"},
					{LabelKey: "asset.field.install_root", ValueRef: "metadata.install_root"},
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
func (p *DintalclawPlugin) ScanAssets() ([]core.Asset, error) {
	logging.Info("DintalclawPlugin: Scanning assets")
	assetScanner := NewDintalclawAssetScanner(GetConfigPath())
	return assetScanner.ScanAssets()
}

// AssessRisks 对已发现的资产进行风险评估
func (p *DintalclawPlugin) AssessRisks(scannedHashes map[string]bool, assets []core.Asset) ([]core.Risk, error) {
	logging.Info("DintalclawPlugin: Assessing risks")
	_ = assets

	risks := []core.Risk{}

	installRoot := findInstallRoot()
	if installRoot == "" {
		logging.Warning("DintalclawPlugin: Cannot find install root")
		return risks, nil
	}

	configPath := filepath.Join(installRoot, "mykey.py")
	checkConfigPermissions(configPath, &risks)
	checkMemoryDirPermissions(installRoot, &risks)
	checkProcessNotRoot(&risks)
	checkLogDirPermissions(installRoot, &risks)
	checkUnscannedSkills(scannedHashes, &risks)

	for i := range risks {
		if tmpl, ok := templates[risks[i].ID]; ok {
			risks[i].Mitigation = tmpl
		}
	}

	logging.Info("DintalclawPlugin: Risk assessment completed, found %d risks", len(risks))
	return risks, nil
}

// MitigateRisk 处理风险缓解请求
func (p *DintalclawPlugin) MitigateRisk(riskInfo string) string {
	return MitigateRiskDispatch(riskInfo)
}

func (p *DintalclawPlugin) GetVulnInfoJSON() []byte {
	return GetVulInfoJSON()
}

func (p *DintalclawPlugin) CompareVulnerabilityVersion(current, target string) (int, bool) {
	return compareDintalclawVersion(current, target)
}

// StartProtection 启动指定资产实例的防护
func (p *DintalclawPlugin) StartProtection(assetID string, config core.ProtectionConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	logging.Info("DintalclawPlugin: StartProtection called, assetID=%s", assetID)

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
func (p *DintalclawPlugin) StopProtection(assetID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	logging.Info("DintalclawPlugin: StopProtection called, assetID=%s", assetID)

	p.protectionStatuses[assetID] = core.ProtectionStatus{
		Running:      false,
		ProxyRunning: false,
	}

	return nil
}

// GetProtectionStatus 获取指定资产实例的防护状态
func (p *DintalclawPlugin) GetProtectionStatus(assetID string) core.ProtectionStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

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

// GetDintalclawPlugin 获取全局 DintalclawPlugin 实例
func GetDintalclawPlugin() *DintalclawPlugin {
	return dintalclawPlugin
}

// StartSkillSecurityScan 启动 Skill 安全分析扫描
func (p *DintalclawPlugin) StartSkillSecurityScan(skillPath, modelConfigJSON string) string {
	return StartSkillSecurityScanInternal(skillPath, modelConfigJSON)
}

// GetSkillSecurityScanLog 获取安全分析扫描日志
func (p *DintalclawPlugin) GetSkillSecurityScanLog(scanID string) string {
	return GetSkillSecurityScanLogInternal(scanID)
}

// GetSkillSecurityScanResult 获取安全分析扫描结果
func (p *DintalclawPlugin) GetSkillSecurityScanResult(scanID string) string {
	return GetSkillSecurityScanResultInternal(scanID)
}

// CancelSkillSecurityScan 取消安全分析扫描
func (p *DintalclawPlugin) CancelSkillSecurityScan(scanID string) string {
	return CancelSkillSecurityScanInternal(scanID)
}

// StartBatchSkillScan 启动批量技能扫描
func (p *DintalclawPlugin) StartBatchSkillScan() string {
	return StartBatchSkillScanInternal()
}

// GetBatchSkillScanLog 获取批量扫描日志
func (p *DintalclawPlugin) GetBatchSkillScanLog(batchID string) string {
	return GetBatchScanLogInternal(batchID)
}

// GetBatchSkillScanResults 获取批量扫描结果
func (p *DintalclawPlugin) GetBatchSkillScanResults(batchID string) string {
	return GetBatchScanResultsInternal(batchID)
}

// CancelBatchSkillScan 取消批量扫描
func (p *DintalclawPlugin) CancelBatchSkillScan(batchID string) string {
	return CancelBatchSkillScanInternal(batchID)
}

// TestModelConnection 测试模型连接
func (p *DintalclawPlugin) TestModelConnection(configJSON string) string {
	return TestModelConnectionInternal(configJSON)
}

// DeleteSkill 删除 Skill
func (p *DintalclawPlugin) DeleteSkill(skillPath string) string {
	return DeleteSkillInternal(skillPath)
}

// SyncGatewaySandbox 同步沙箱配置
func (p *DintalclawPlugin) SyncGatewaySandbox() string {
	return SyncGatewaySandboxInternal()
}

// SyncGatewaySandboxByAsset 同步指定资产的沙箱配置
func (p *DintalclawPlugin) SyncGatewaySandboxByAsset(assetID string) string {
	return SyncGatewaySandboxByAssetInternal(assetID)
}

// HasInitialBackup 检查初始备份是否存在
func (p *DintalclawPlugin) HasInitialBackup() string {
	return HasInitialBackupInternal()
}

// RestoreToInitialConfig 恢复到初始配置
func (p *DintalclawPlugin) RestoreToInitialConfig() string {
	return RestoreToInitialConfigInternal()
}

// StopAllProcesses 停掉所有 dintalclaw 相关 Python 进程（含子进程树）
func (p *DintalclawPlugin) StopAllProcesses() string {
	logging.Info("[Dintalclaw] StopAllProcesses called")
	killAllDintalclawProcesses()

	payload, _ := json.Marshal(map[string]interface{}{
		"success": true,
		"message": "all dintalclaw processes killed",
	})
	return string(payload)
}

// OnAppExit 应用退出回调
func (p *DintalclawPlugin) OnAppExit(assetID string) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	assetID = strings.TrimSpace(assetID)
	logging.Info("[Dintalclaw] OnAppExit: assetID=%s", assetID)

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
		"message":  "dintalclaw exit callback completed",
	})
	if err != nil {
		return `{"success":false,"error":"marshal error"}`
	}
	return string(payload)
}

// RestoreBotDefaultState 恢复 Bot 默认配置状态
func (p *DintalclawPlugin) RestoreBotDefaultState(assetID string) string {
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

// OnBeforeProxyStop 防护停止前钩子，恢复原始配置
func (p *DintalclawPlugin) OnBeforeProxyStop(ctx *core.ProtectionContext) {
	logging.Info("[Dintalclaw] OnBeforeProxyStop: assetID=%s", ctx.AssetID)

	backupDir := ctx.BackupDir
	if backupDir == "" {
		homeDir, _ := os.UserHomeDir()
		backupDir = core.ResolveBackupDir(homeDir)
	}

	result := RestoreToInitialConfigByAsset(backupDir, ctx.AssetID)
	if result.Success {
		logging.Info("[Dintalclaw] Config restored to initial state: %s", result.Message)
	} else {
		logging.Warning("[Dintalclaw] Config restore failed: %s", result.Error)
	}
}

// parseBotModelConfig 从 repository.ProtectionConfig 解析 BotModelConfig
func parseBotModelConfig(config *repository.ProtectionConfig) *BotModelConfig {
	if config == nil || config.BotModelConfig == nil {
		logging.Warning("[Dintalclaw] BotModelConfig is nil")
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

// OnProtectionStart 防护启动钩子，注入代理配置并重启 Bot 进程
func (p *DintalclawPlugin) OnProtectionStart(ctx *core.ProtectionContext) (map[string]interface{}, error) {
	logging.Info("[Dintalclaw] OnProtectionStart: assetID=%s, proxyPort=%d", ctx.AssetID, ctx.ProxyPort)

	repo := repository.NewProtectionRepository(nil)
	config, err := repo.GetProtectionConfig(ctx.AssetID)
	if err != nil {
		logging.Error("[Dintalclaw] Failed to get protection config from DB: %v", err)
		return nil, fmt.Errorf("failed to get protection config: %w", err)
	}
	if config == nil {
		logging.Error("[Dintalclaw] No protection config found in DB for %s/%s", dintalclawAssetName, ctx.AssetID)
		return nil, fmt.Errorf("no protection config found")
	}

	botModelConfig := parseBotModelConfig(config)
	if botModelConfig == nil {
		logging.Error("[Dintalclaw] Failed to parse bot model config")
		return nil, fmt.Errorf("failed to parse bot model config")
	}

	logging.Info("[Dintalclaw] Starting process with proxyPort=%d", ctx.ProxyPort)
	return startProcessWithProxy(ctx.ProxyPort, botModelConfig, ctx.BackupDir, ctx.AssetID)
}
