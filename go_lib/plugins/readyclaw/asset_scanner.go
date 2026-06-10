package readyclaw

import (
	_ "embed"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/scanner"
)

//go:embed readyclaw.json
var readyclawRulesJSON []byte

type assetScanner struct {
	configPath string
	collector  core.Collector
}

// newAssetScanner 构造 ReadyClaw 多信号资产扫描器，复用社区版 OpenClaw-like 插件扫描管线。
func newAssetScanner(configPath string) *assetScanner {
	return &assetScanner{configPath: configPath}
}

// withCollector 仅供测试注入采集器，生产路径使用默认系统采集器。
func (s *assetScanner) withCollector(c core.Collector) *assetScanner {
	s.collector = c
	return s
}

// scan 执行 ReadyClaw 规则扫描并合并成稳定资产实例。
func (s *assetScanner) scan() ([]core.Asset, error) {
	return scanner.ScanSingleMergedAsset(scanner.PluginAssetScanOptions{
		AssetName:  readyclawAssetName,
		AssetType:  "Service",
		ConfigPath: s.configPath,
		Collector:  s.collector,
		RulesJSON:  readyclawRulesJSON,
		Enrich:     s.enrichAsset,
	})
}

// enrichAsset 读取 ReadyClaw 扁平配置，补齐前端资产卡片需要的展示字段。
func (s *assetScanner) enrichAsset(asset *core.Asset) {
	if asset.Metadata == nil {
		asset.Metadata = make(map[string]string)
	}

	cfg, _, configPath, err := loadConfig()
	if err != nil {
		logging.Warning("[ReadyClaw] Cannot load config for enrichment: %v", err)
		if s.configPath != "" {
			asset.Metadata["config_path"] = s.configPath
		}
		return
	}

	asset.Metadata["config_path"] = configPath
	asset.Metadata["llm_base_url"] = valueString(cfg.Values, "LLM_BASE_URL")
	asset.Metadata["model_name"] = valueString(cfg.Values, "LLM_MODEL_NAME")
	asset.Metadata["protection_status"] = "unknown"
	if asset.ServiceName == "" {
		asset.ServiceName = "nanoclaw"
	}
	if len(asset.Ports) == 0 {
		asset.Ports = []int{29555, 3456}
	}

	asset.DisplaySections = []core.DisplaySection{
		{
			Title: "Gateway",
			Icon:  "globe",
			Items: []core.DisplayItem{
				{Label: "API Port", Value: "29555", Status: "neutral"},
				{Label: "LLM Gateway Port", Value: "3456", Status: "neutral"},
				{Label: "Base URL", Value: asset.Metadata["llm_base_url"], Status: "neutral"},
			},
		},
		{
			Title: "Config",
			Icon:  "file",
			Items: []core.DisplayItem{
				{Label: "Path", Value: configPath, Status: "neutral"},
				{Label: "Model", Value: asset.Metadata["model_name"], Status: "neutral"},
			},
		},
	}
}
