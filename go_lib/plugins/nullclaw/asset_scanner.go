package nullclaw

import (
	"fmt"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/scanner"
)

// nullclawAssetName Nullclaw资产名称常量
const nullclawAssetName = "Nullclaw"

// NullclawAssetScanner Nullclaw资产扫描器
// 实现Nullclaw Bot的资产发现和属性采集逻辑
// 负责：加载Nullclaw专用检测规则 → 执行扫描 → 按实例分组 → 丰富资产属性
type NullclawAssetScanner struct {
	// configPath 从Flutter传入的授权配置路径
	configPath string
	// collector 可注入的系统信息采集器（nil时使用平台默认采集器）
	collector core.Collector
}

// NewNullclawAssetScanner 创建Nullclaw资产扫描器实例
func NewNullclawAssetScanner(configPath string) *NullclawAssetScanner {
	return &NullclawAssetScanner{
		configPath: configPath,
	}
}

// WithCollector 设置自定义采集器（主要用于单元测试注入模拟采集器）
func (s *NullclawAssetScanner) WithCollector(c core.Collector) *NullclawAssetScanner {
	s.collector = c
	return s
}

// ScanAssets 执行Nullclaw资产扫描
// 完整流程：
//  1. 加载Nullclaw检测规则（从嵌入的nullclaw.json）
//  2. 通过核心扫描服务执行系统检测
//  3. 合并同名规则匹配结果（端口+进程去重），保留多实例
//  4. 使用Nullclaw配置文件信息丰富资产属性
//  5. 为每个实例计算唯一指纹 ID
func (s *NullclawAssetScanner) ScanAssets() ([]core.Asset, error) {
	return scanner.ScanSingleMergedAsset(scanner.PluginAssetScanOptions{
		AssetName:  nullclawAssetName,
		AssetType:  "Service",
		ConfigPath: s.configPath,
		Collector:  s.collector,
		RulesJSON:  nullclawRulesJSON,
		Enrich:     s.enrichAssetWithConfig,
	})
}

// loadRules 从嵌入的JSON文件加载Nullclaw资产检测规则
func (s *NullclawAssetScanner) loadRules() ([]core.AssetFinderRule, error) {
	return scanner.ParseAssetFinderRulesJSON(nullclawRulesJSON)
}

// enrichAssetWithConfig 使用Nullclaw配置文件信息丰富资产属性
// 读取并解析配置文件，提取Gateway、Auth、Sandbox、Logging等关键配置,
// 同时构建 DisplaySections 供 UI 层通用渲染
func (s *NullclawAssetScanner) enrichAssetWithConfig(asset *core.Asset) {
	configPath, err := findConfigPath()
	if err != nil {
		logging.Warning("Cannot find Nullclaw config, skip enrichment: %v", err)
		return
	}

	config, _, err := loadConfig(configPath)
	if err != nil {
		logging.Warning("Cannot load Nullclaw config, skip enrichment: %v", err)
		return
	}

	if asset.Metadata == nil {
		asset.Metadata = make(map[string]string)
	}

	// Config file path
	asset.Metadata["config_path"] = configPath

	// Gateway host/bind address
	host := config.Gateway.Host
	if host == "" {
		host = config.Gateway.Bind
	}
	if host == "" {
		host = "127.0.0.1"
	}
	asset.Metadata["gateway_host"] = host

	// Gateway port
	gatewayPort := "3000"
	if config.Gateway.Port > 0 {
		gatewayPort = fmt.Sprintf("%d", config.Gateway.Port)
	}
	asset.Metadata["gateway_port"] = gatewayPort

	// Gateway pairing/public bind state
	asset.Metadata["require_pairing"] = fmt.Sprintf("%v", config.Gateway.RequirePairing)
	asset.Metadata["allow_public_bind"] = fmt.Sprintf("%v", config.Gateway.AllowPublicBind)

	// Sandbox backend and workspace-only constraints
	sandboxBackend := config.Security.Sandbox.Backend
	if sandboxBackend == "" {
		sandboxBackend = "auto"
	}
	asset.Metadata["sandbox_backend"] = sandboxBackend
	asset.Metadata["workspace_only"] = fmt.Sprintf("%v", config.Autonomy.WorkspaceOnly)

	// Audit settings
	asset.Metadata["audit_enabled"] = fmt.Sprintf("%v", config.Security.Audit.Enabled)

	// Build DisplaySections for generic UI rendering
	isBindSafe := host == "127.0.0.1" || host == "localhost" || host == "::1" || host == "loopback"
	isPairingRequired := config.Gateway.RequirePairing
	isSandboxEnabled := sandboxBackend != "none" && sandboxBackend != "off" && sandboxBackend != "disabled"
	isAuditEnabled := config.Security.Audit.Enabled

	bindStatus := "danger"
	if isBindSafe {
		bindStatus = "safe"
	}
	pairingStatus := "danger"
	if isPairingRequired {
		pairingStatus = "safe"
	}
	sandboxStatus := "warning"
	if isSandboxEnabled {
		sandboxStatus = "safe"
	}
	auditStatus := "danger"
	if isAuditEnabled {
		auditStatus = "safe"
	}
	publicBindDisplay := "false"
	if config.Gateway.AllowPublicBind {
		publicBindDisplay = "true"
	}
	workspaceOnlyDisplay := "false"
	if config.Autonomy.WorkspaceOnly {
		workspaceOnlyDisplay = "true"
	}

	asset.DisplaySections = []core.DisplaySection{
		{
			Title: "Gateway Configuration",
			Icon:  "globe",
			Items: []core.DisplayItem{
				{Label: "Host", Value: host, Status: bindStatus},
				{Label: "Port", Value: gatewayPort, Status: "neutral"},
				{Label: "Pairing Required", Value: fmt.Sprintf("%v", config.Gateway.RequirePairing), Status: pairingStatus},
				{Label: "Allow Public Bind", Value: publicBindDisplay, Status: bindStatus},
			},
		},
		{
			Title: "Sandbox",
			Icon:  "box",
			Items: []core.DisplayItem{
				{Label: "Backend", Value: sandboxBackend, Status: sandboxStatus},
				{Label: "Workspace Only", Value: workspaceOnlyDisplay, Status: sandboxStatus},
			},
		},
		{
			Title: "Audit",
			Icon:  "file-text",
			Items: []core.DisplayItem{
				{Label: "Enabled", Value: fmt.Sprintf("%v", config.Security.Audit.Enabled), Status: auditStatus},
			},
		},
	}

	if configPath != "" {
		asset.DisplaySections = append(asset.DisplaySections, core.DisplaySection{
			Title: "Config",
			Icon:  "file",
			Items: []core.DisplayItem{
				{Label: "Path", Value: configPath, Status: "neutral"},
			},
		})
	}
}
