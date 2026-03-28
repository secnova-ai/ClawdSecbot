package scanner

import (
	"encoding/json"
	"fmt"
	"strings"

	"go_lib/core"
	"go_lib/core/logging"
)

// PluginAssetScanOptions defines shared scan pipeline options used by plugins.
type PluginAssetScanOptions struct {
	AssetName  string
	AssetType  string
	ConfigPath string
	Collector  core.Collector
	RulesJSON  []byte
	Enrich     func(asset *core.Asset)
}

// ParseAssetFinderRulesJSON parses plugin-embedded rule JSON bytes.
func ParseAssetFinderRulesJSON(rulesJSON []byte) ([]core.AssetFinderRule, error) {
	if len(rulesJSON) == 0 {
		return nil, fmt.Errorf("rules json is empty")
	}
	var rules []core.AssetFinderRule
	if err := json.Unmarshal(rulesJSON, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// ScanSingleMergedAsset runs the shared plugin asset scan pipeline:
//  1. build collector
//  2. parse/load rules
//  3. scan snapshot
//  4. merge rule-matched assets into one logical asset
//  5. plugin-specific enrichment
//  6. fill source plugin and deterministic asset ID
//
// For compatibility with existing plugin behavior, rule/scan errors are logged
// and converted to empty results instead of bubbling up.
func ScanSingleMergedAsset(opts PluginAssetScanOptions) ([]core.Asset, error) {
	assetName := strings.TrimSpace(opts.AssetName)
	if assetName == "" {
		return nil, fmt.Errorf("asset name is required")
	}

	logging.Info("Starting %s asset scan...", assetName)

	collector := opts.Collector
	if collector == nil {
		collector = core.NewCollector(opts.ConfigPath)
	}

	assetScanner := NewAssetScanner(collector)
	rules, err := ParseAssetFinderRulesJSON(opts.RulesJSON)
	if err != nil {
		logging.Error("Failed to load %s rules: %v", assetName, err)
		return []core.Asset{}, nil
	}
	assetScanner.LoadRules(rules)

	rawAssets, err := assetScanner.Scan()
	if err != nil {
		logging.Error("%s asset scan failed: %v", assetName, err)
		return []core.Asset{}, nil
	}
	if len(rawAssets) == 0 {
		logging.Info("No %s assets detected", assetName)
		return []core.Asset{}, nil
	}

	assetType := strings.TrimSpace(opts.AssetType)
	if assetType == "" {
		assetType = "Service"
	}
	mergedAsset := MergeAssetsByName(rawAssets, assetName, assetType)
	if mergedAsset == nil {
		logging.Info("No %s assets after merge", assetName)
		return []core.Asset{}, nil
	}

	if opts.Enrich != nil {
		opts.Enrich(mergedAsset)
	}

	if mergedAsset.Metadata == nil {
		mergedAsset.Metadata = make(map[string]string)
	}
	mergedAsset.SourcePlugin = assetName
	configPathFingerprint := strings.TrimSpace(mergedAsset.Metadata["config_path"])
	if configPathFingerprint == "" {
		configPathFingerprint = strings.TrimSpace(opts.ConfigPath)
	}
	mergedAsset.ID = core.ComputeAssetID(
		assetName,
		configPathFingerprint,
		mergedAsset.Ports,
		mergedAsset.ProcessPaths,
	)

	logging.Info("%s asset scan completed, id=%s, ports=%v, processes=%v",
		assetName, mergedAsset.ID, mergedAsset.Ports, mergedAsset.ProcessPaths)

	return []core.Asset{*mergedAsset}, nil
}
