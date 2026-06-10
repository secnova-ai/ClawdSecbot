package coclaw

import (
	_ "embed"
	"fmt"
	"strings"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/scanner"
)

//go:embed coclaw.json
var coclawRulesJSON []byte

type assetScanner struct {
	configPath string
	collector  core.Collector
}

func newAssetScanner(configPath string) *assetScanner {
	return &assetScanner{configPath: configPath}
}

func (s *assetScanner) withCollector(c core.Collector) *assetScanner {
	s.collector = c
	return s
}

func (s *assetScanner) scan() ([]core.Asset, error) {
	return scanner.ScanSingleMergedAsset(scanner.PluginAssetScanOptions{
		AssetName:  coclawAssetName,
		AssetType:  "Service",
		ConfigPath: stableConfigFingerprint(s.configPath),
		Collector:  s.collector,
		RulesJSON:  coclawRulesJSON,
		Enrich:     s.enrichAsset,
	})
}

func (s *assetScanner) enrichAsset(asset *core.Asset) {
	cfg, _, configPath, err := loadConfig()
	if err != nil {
		logging.Warning("[CoClaw] config enrichment skipped: %v", err)
		return
	}
	if asset.Metadata == nil {
		asset.Metadata = map[string]string{}
	}
	asset.Metadata["config_path"] = configPath

	primary, _ := readPrimaryFromConfig(cfg)
	if primary != "" {
		asset.Metadata["model_primary"] = primary
	}

	bind := strings.TrimSpace(cfg.Gateway.Bind)
	if bind == "" {
		bind = strings.TrimSpace(cfg.Gateway.Host)
	}
	if bind == "" {
		bind = "127.0.0.1"
	}
	port := cfg.Gateway.Port
	if port == 0 {
		port = 18790
	}
	asset.Metadata["gateway_bind"] = bind
	asset.Metadata["gateway_port"] = fmt.Sprintf("%d", port)

	asset.DisplaySections = []core.DisplaySection{
		{
			Title: "Gateway",
			Icon:  "globe",
			Items: []core.DisplayItem{
				{Label: "Bind", Value: bind, Status: safeStatus(bind == "127.0.0.1" || bind == "loopback" || bind == "::1")},
				{Label: "Port", Value: asset.Metadata["gateway_port"], Status: "neutral"},
			},
		},
		{
			Title: "Config",
			Icon:  "file",
			Items: []core.DisplayItem{
				{Label: "Path", Value: configPath, Status: "neutral"},
				{Label: "Primary Model", Value: primary, Status: "neutral"},
			},
		},
	}
}

func safeStatus(ok bool) string {
	if ok {
		return "safe"
	}
	return "danger"
}

func readPrimaryFromConfig(cfg *coclawConfig) (string, bool) {
	if cfg == nil {
		return "", false
	}
	switch model := cfg.Agents.Defaults.Model.(type) {
	case string:
		model = strings.TrimSpace(model)
		return model, model != ""
	case map[string]interface{}:
		primary, ok := model["primary"].(string)
		primary = strings.TrimSpace(primary)
		return primary, ok && primary != ""
	default:
		return "", false
	}
}
