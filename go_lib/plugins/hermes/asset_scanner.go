package hermes

import (
	"strings"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/scanner"
)

const hermesAssetName = "Hermes"

// HermesAssetScanner discovers local Hermes runtime and config.
type HermesAssetScanner struct {
	configPath string
	collector  core.Collector
}

func NewHermesAssetScanner(configPath string) *HermesAssetScanner {
	return &HermesAssetScanner{configPath: configPath}
}

func (s *HermesAssetScanner) WithCollector(c core.Collector) *HermesAssetScanner {
	s.collector = c
	return s
}

func (s *HermesAssetScanner) ScanAssets() ([]core.Asset, error) {
	assets, err := scanner.ScanSingleMergedAsset(scanner.PluginAssetScanOptions{
		AssetName:  hermesAssetName,
		AssetType:  "Service",
		ConfigPath: s.configPath,
		Collector:  s.collector,
		RulesJSON:  hermesRulesJSON,
		Enrich:     s.enrichAssetWithConfig,
	})
	for i := range assets {
		s.rewriteStableAssetID(&assets[i])
	}
	return assets, err
}

func (s *HermesAssetScanner) enrichAssetWithConfig(asset *core.Asset) {
	configPath, err := findConfigPath()
	if err != nil {
		logging.Warning("[HermesScanner] config not found: %v", err)
		return
	}
	cfg, raw, err := loadConfig(configPath)
	if err != nil {
		logging.Warning("[HermesScanner] config load failed: %v", err)
		return
	}

	if asset.Metadata == nil {
		asset.Metadata = make(map[string]string)
	}
	asset.Metadata["config_path"] = configPath
	asset.Metadata["model_provider"] = strings.TrimSpace(cfg.Model.Provider)
	asset.Metadata["model_default"] = strings.TrimSpace(cfg.Model.Default)
	asset.Metadata["model_base_url"] = strings.TrimSpace(cfg.Model.BaseURL)
	asset.Metadata["terminal_backend"] = strings.TrimSpace(cfg.Terminal.Backend)
	asset.Metadata["approvals_mode"] = strings.TrimSpace(cfg.Approvals.Mode)
	if version := getHermesVersion(); version != "" {
		asset.Version = version
	}
	if cfg.Security.RedactSecrets != nil {
		if *cfg.Security.RedactSecrets {
			asset.Metadata["redact_secrets"] = "true"
		} else {
			asset.Metadata["redact_secrets"] = "false"
		}
	} else {
		if v, ok := getNestedBool(raw, "security", "redact_secrets"); ok {
			if v {
				asset.Metadata["redact_secrets"] = "true"
			} else {
				asset.Metadata["redact_secrets"] = "false"
			}
		}
	}

	asset.DisplaySections = []core.DisplaySection{
		{
			Title: "Model",
			Icon:  "cpu",
			Items: []core.DisplayItem{
				{Label: "Provider", Value: fallbackString(asset.Metadata["model_provider"], "auto"), Status: "neutral"},
				{Label: "Default", Value: fallbackString(asset.Metadata["model_default"], "(empty)"), Status: "neutral"},
				{Label: "Base URL", Value: fallbackString(asset.Metadata["model_base_url"], "(default)"), Status: "neutral"},
			},
		},
		{
			Title: "Runtime Safety",
			Icon:  "shield",
			Items: []core.DisplayItem{
				{Label: "Terminal Backend", Value: fallbackString(asset.Metadata["terminal_backend"], "local"), Status: terminalBackendStatus(asset.Metadata["terminal_backend"])},
				{Label: "Approvals", Value: fallbackString(asset.Metadata["approvals_mode"], "default"), Status: approvalModeStatus(asset.Metadata["approvals_mode"])},
				{Label: "Redact Secrets", Value: fallbackString(asset.Metadata["redact_secrets"], "unknown"), Status: redactStatus(asset.Metadata["redact_secrets"])},
			},
		},
		{
			Title: "Config",
			Icon:  "file",
			Items: []core.DisplayItem{{Label: "Path", Value: configPath, Status: "neutral"}},
		},
	}

	if runtimeSection := buildRuntimeSection(asset); runtimeSection != nil {
		asset.DisplaySections = append(asset.DisplaySections, *runtimeSection)
	}
}

// rewriteStableAssetID keeps Hermes instance identity stable across runtime drift.
// Hermes gateway ports and process paths can vary between restarts, so the
// instance key is derived from the config path fingerprint only.
func (s *HermesAssetScanner) rewriteStableAssetID(asset *core.Asset) {
	if asset == nil {
		return
	}
	configPath := strings.TrimSpace(asset.Metadata["config_path"])
	if configPath == "" {
		configPath = strings.TrimSpace(s.configPath)
	}
	previous := strings.TrimSpace(asset.ID)
	asset.ID = core.ComputeAssetID(hermesAssetName, configPath)
	if previous != "" && previous != asset.ID {
		logging.Info("[HermesScanner] rewrote volatile asset_id %s -> %s", previous, asset.ID)
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
		items = append(items, core.DisplayItem{Label: "Image Path", Value: strings.TrimSpace(asset.ProcessPaths[0]), Status: "neutral"})
	}
	if len(items) == 0 {
		return nil
	}
	return &core.DisplaySection{Title: "Runtime", Icon: "monitor", Items: items}
}

func fallbackString(value, fallback string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return fallback
	}
	return v
}

func terminalBackendStatus(v string) string {
	backend := strings.ToLower(strings.TrimSpace(v))
	if backend == "remote" {
		return "safe"
	}
	if backend == "local" || backend == "" {
		return "warning"
	}
	return "neutral"
}

func approvalModeStatus(v string) string {
	mode := strings.ToLower(strings.TrimSpace(v))
	if mode == "off" || mode == "never" || mode == "yolo" {
		return "danger"
	}
	if mode == "manual" || mode == "smart" {
		return "safe"
	}
	if mode == "auto" || mode == "default" || mode == "on-request" || mode == "on_request" {
		return "safe"
	}
	return "neutral"
}

func redactStatus(v string) string {
	normalized := strings.ToLower(strings.TrimSpace(v))
	if normalized == "true" || normalized == "on" {
		return "safe"
	}
	if normalized == "false" || normalized == "off" {
		return "danger"
	}
	return "neutral"
}
