package openclaw

import (
	"encoding/json"
	"fmt"

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
	logging.Info("Starting Openclaw asset scan...")

	// 1. 确定采集器
	collector := s.collector
	if collector == nil {
		collector = core.NewCollector(s.configPath)
	}

	// 2. 创建扫描服务并加载规则
	assetScanner := scanner.NewAssetScanner(collector)
	rules, err := s.loadRules()
	if err != nil {
		logging.Error("Failed to load Openclaw rules: %v", err)
		return []core.Asset{}, nil
	}
	assetScanner.LoadRules(rules)

	// 3. 执行扫描
	rawAssets, err := assetScanner.Scan()
	if err != nil {
		logging.Error("Openclaw asset scan failed: %v", err)
		return []core.Asset{}, nil
	}

	if len(rawAssets) == 0 {
		logging.Info("No Openclaw assets detected")
		return []core.Asset{}, nil
	}

	// 4. Merge rule-matched results into a single Openclaw asset per config path.
	// MergeAssetsByName consolidates ports/processes from multiple detection rules
	// (e.g. port-based + config-file-based) that belong to the same logical instance.
	mergedAsset := scanner.MergeAssetsByName(rawAssets, openclawAssetName, "Service")
	if mergedAsset == nil {
		logging.Info("No Openclaw assets after merge")
		return []core.Asset{}, nil
	}

	// 5. Enrich with config, build display sections, compute ID
	s.enrichAssetWithConfig(mergedAsset)

	mergedAsset.SourcePlugin = openclawAssetName
	mergedAsset.ID = core.ComputeAssetID(
		mergedAsset.Name,
		mergedAsset.Metadata["config_path"],
		mergedAsset.Ports,
		mergedAsset.ProcessPaths,
	)

	logging.Info("Openclaw asset scan completed, id=%s, ports=%v, processes=%v",
		mergedAsset.ID, mergedAsset.Ports, mergedAsset.ProcessPaths)
	return []core.Asset{*mergedAsset}, nil
}

// loadRules 从嵌入的JSON文件加载Openclaw资产检测规则
func (s *OpenclawAssetScanner) loadRules() ([]core.AssetFinderRule, error) {
	var rules []core.AssetFinderRule
	if err := json.Unmarshal(openclawRulesJSON, &rules); err != nil {
		return nil, err
	}
	return rules, nil
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
}
