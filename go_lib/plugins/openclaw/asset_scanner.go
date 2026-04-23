package openclaw

import (
	"fmt"
	"strings"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/scanner"
)

// openclawAssetName Openclaw资产名称常量
const openclawAssetName = "Openclaw"

// OpenclawAssetScanner Openclaw资产扫描器
// 实现Openclaw Bot的资产发现和属性采集逻辑
// 负责：加载Openclaw专用检测规则 → 执行扫描 → 按实例分组 → 丰富资产属性
type OpenclawAssetScanner struct {
	// configPath 从Flutter传入的授权配置路径
	configPath string
	// collector 可注入的系统信息采集器（nil时使用平台默认采集器）
	collector core.Collector
}

// NewOpenclawAssetScanner 创建Openclaw资产扫描器实例
func NewOpenclawAssetScanner(configPath string) *OpenclawAssetScanner {
	return &OpenclawAssetScanner{
		configPath: configPath,
	}
}

// WithCollector 设置自定义采集器（主要用于单元测试注入模拟采集器）
func (s *OpenclawAssetScanner) WithCollector(c core.Collector) *OpenclawAssetScanner {
	s.collector = c
	return s
}

// ScanAssets 执行Openclaw资产扫描
// 完整流程：
//  1. 加载Openclaw检测规则（从嵌入的openclaw.json）
//  2. 通过核心扫描服务执行系统检测
//  3. 合并同名规则匹配结果（端口+进程去重），保留多实例
//  4. 使用Openclaw配置文件信息丰富资产属性
//  5. 为每个实例计算唯一指纹 ID
func (s *OpenclawAssetScanner) ScanAssets() ([]core.Asset, error) {
	return scanner.ScanSingleMergedAsset(scanner.PluginAssetScanOptions{
		AssetName:  openclawAssetName,
		AssetType:  "Service",
		ConfigPath: s.configPath,
		Collector:  s.collector,
		RulesJSON:  openclawRulesJSON,
		Enrich:     s.enrichAssetWithConfig,
	})
}

// loadRules 从嵌入的JSON文件加载Openclaw资产检测规则
func (s *OpenclawAssetScanner) loadRules() ([]core.AssetFinderRule, error) {
	return scanner.ParseAssetFinderRulesJSON(openclawRulesJSON)
}

// enrichAssetWithConfig 使用Openclaw配置文件信息丰富资产属性
// 读取并解析配置文件，提取Gateway、Auth、Sandbox、Logging等关键配置,
// 同时构建 DisplaySections 供 UI 层通用渲染
func (s *OpenclawAssetScanner) enrichAssetWithConfig(asset *core.Asset) {
	configPath, err := findConfigPath()
	if err != nil {
		logging.Warning("Cannot find Openclaw config, skip enrichment: %v", err)
		return
	}

	config, _, err := loadConfig(configPath)
	if err != nil {
		logging.Warning("Cannot load Openclaw config, skip enrichment: %v", err)
		return
	}

	if asset.Metadata == nil {
		asset.Metadata = make(map[string]string)
	}

	if version := resolveOpenClawVersion(asset.ProcessPaths, configPath); version != "" {
		asset.Version = version
	}

	// Config file path
	asset.Metadata["config_path"] = configPath

	// Gateway bind address
	bind := config.Gateway.Bind
	if bind == "" {
		bind = config.Gateway.Host
	}
	if bind == "" {
		bind = "127.0.0.1"
	}
	asset.Metadata["gateway_bind"] = bind

	// Gateway port
	gatewayPort := "18789"
	if config.Gateway.Port > 0 {
		gatewayPort = fmt.Sprintf("%d", config.Gateway.Port)
	}
	asset.Metadata["gateway_port"] = gatewayPort

	// Auth mode
	authMode := "disabled"
	if config.Gateway.Auth.Mode != "" {
		authMode = config.Gateway.Auth.Mode
	} else if config.Gateway.Auth.Enabled {
		authMode = "enabled"
	}
	asset.Metadata["auth_mode"] = authMode

	// Sandbox mode
	sandboxMode := config.Agents.Defaults.Sandbox.Mode
	if sandboxMode == "" {
		sandboxMode = "none"
	}
	asset.Metadata["sandbox_mode"] = sandboxMode

	// Logging redaction
	redact := config.Logging.RedactSensitive
	if redact == "" {
		redact = "on"
	}
	asset.Metadata["logging_redact"] = redact

	// Build DisplaySections for generic UI rendering
	isBindSafe := bind == "127.0.0.1" || bind == "loopback" || bind == "::1"
	isAuthConfigured := authMode != "disabled"
	isSandboxEnabled := sandboxMode != "none" && sandboxMode != ""
	isRedactEnabled := redact != "off"

	bindStatus := "danger"
	if isBindSafe {
		bindStatus = "safe"
	}
	authStatus := "danger"
	if isAuthConfigured {
		authStatus = "safe"
	}
	sandboxStatus := "warning"
	if isSandboxEnabled {
		sandboxStatus = "safe"
	}
	redactStatus := "danger"
	if isRedactEnabled {
		redactStatus = "safe"
	}

	authDisplay := authMode
	switch authMode {
	case "token":
		authDisplay = "Token"
	case "password":
		authDisplay = "Password"
	case "enabled":
		authDisplay = "Enabled"
	case "disabled":
		authDisplay = "Disabled"
	}

	// Normalize loopback address for display
	displayBind := bind
	if bind == "loopback" {
		displayBind = "127.0.0.1"
	}

	asset.DisplaySections = []core.DisplaySection{
		{
			Title: "Gateway Configuration",
			Icon:  "globe",
			Items: []core.DisplayItem{
				{Label: "Bind", Value: displayBind, Status: bindStatus},
				{Label: "Port", Value: gatewayPort, Status: "neutral"},
				{Label: "Auth", Value: authDisplay, Status: authStatus},
			},
		},
		{
			Title: "Sandbox",
			Icon:  "box",
			Items: []core.DisplayItem{
				{Label: "Mode", Value: sandboxMode, Status: sandboxStatus},
			},
		},
		{
			Title: "Logging",
			Icon:  "file-text",
			Items: []core.DisplayItem{
				{Label: "Redact", Value: redact, Status: redactStatus},
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

	if runtimeSection := buildRuntimeSection(asset); runtimeSection != nil {
		asset.DisplaySections = append(asset.DisplaySections, *runtimeSection)
	}
}

func buildRuntimeSection(asset *core.Asset) *core.DisplaySection {
	if asset == nil {
		return nil
	}

	items := make([]core.DisplayItem, 0, 2)
	if pid := strings.TrimSpace(asset.Metadata["pid"]); pid != "" {
		items = append(items, core.DisplayItem{Label: "PID", Value: pid, Status: "neutral"})
	}
	if len(asset.ProcessPaths) > 0 && strings.TrimSpace(asset.ProcessPaths[0]) != "" {
		items = append(items, core.DisplayItem{
			Label:  "Image Path",
			Value:  strings.TrimSpace(asset.ProcessPaths[0]),
			Status: "neutral",
		})
	}
	if len(items) == 0 {
		return nil
	}

	return &core.DisplaySection{
		Title: "Runtime",
		Icon:  "monitor",
		Items: items,
	}
}
