package scanner

import (
	"encoding/json"
	"testing"

	"go_lib/core"
)

func TestParseAssetFinderRulesJSON(t *testing.T) {
	rulesJSON := []byte(`[
		{
			"code": "test_rule",
			"name": "Test Rule",
			"life_cycle": 1,
			"desc": "test",
			"expression": {
				"lang": "json_match",
				"expr": "{\"ports\":[3000]}"
			}
		}
	]`)

	rules, err := ParseAssetFinderRulesJSON(rulesJSON)
	if err != nil {
		t.Fatalf("ParseAssetFinderRulesJSON failed: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Code != "test_rule" {
		t.Fatalf("expected code=test_rule, got %s", rules[0].Code)
	}
}

func TestScanSingleMergedAsset(t *testing.T) {
	collector := &mockCollector{
		snapshot: core.SystemSnapshot{
			OpenPorts: []int{3000},
			RunningProcesses: []core.SystemProcess{
				{Pid: 502, Name: "nullclaw", Cmd: "nullclaw gateway", Path: "/usr/local/bin/nullclaw"},
			},
			Services: []string{},
			FileExists: func(path string) bool {
				return path == "~/.nullclaw/config.json"
			},
		},
	}

	rules := []core.AssetFinderRule{
		{
			Code:      "rule_port_process",
			Name:      "Port and Process Detection",
			LifeCycle: core.RuleLifeCycleRuntime,
			Desc:      "Detects via port and process",
			Expression: core.RuleExpression{
				Lang: "json_match",
				Expr: `{"ports": [3000], "process_keywords": ["nullclaw"]}`,
			},
		},
		{
			Code:      "rule_config",
			Name:      "Config File Detection",
			LifeCycle: core.RuleLifeCycleStatic,
			Desc:      "Detects via config file",
			Expression: core.RuleExpression{
				Lang: "json_match",
				Expr: `{"file_paths": ["~/.nullclaw/config.json"]}`,
			},
		},
	}
	rulesJSON, err := json.Marshal(rules)
	if err != nil {
		t.Fatalf("marshal rules failed: %v", err)
	}

	assets, err := ScanSingleMergedAsset(PluginAssetScanOptions{
		AssetName: "Nullclaw",
		AssetType: "Service",
		Collector: collector,
		RulesJSON: rulesJSON,
		Enrich: func(asset *core.Asset) {
			if asset.Metadata == nil {
				asset.Metadata = make(map[string]string)
			}
			asset.Metadata["config_path"] = "~/.nullclaw/config.json"
			asset.DisplaySections = []core.DisplaySection{
				{
					Title: "Config",
					Icon:  "file",
					Items: []core.DisplayItem{{Label: "Path", Value: "~/.nullclaw/config.json", Status: "neutral"}},
				},
			}
		},
	})
	if err != nil {
		t.Fatalf("ScanSingleMergedAsset failed: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(assets))
	}
	if assets[0].Name != "Nullclaw" {
		t.Fatalf("expected asset name Nullclaw, got %s", assets[0].Name)
	}
	if assets[0].SourcePlugin != "Nullclaw" {
		t.Fatalf("expected source plugin Nullclaw, got %s", assets[0].SourcePlugin)
	}
	if assets[0].ID == "" {
		t.Fatal("expected non-empty asset ID")
	}
	if assets[0].Metadata["config_path"] != "~/.nullclaw/config.json" {
		t.Fatalf("expected config_path metadata, got %s", assets[0].Metadata["config_path"])
	}
}

func TestScanSingleMergedAsset_BadRulesJSONReturnsEmpty(t *testing.T) {
	assets, err := ScanSingleMergedAsset(PluginAssetScanOptions{
		AssetName: "Openclaw",
		RulesJSON: []byte("{bad json"),
	})
	if err != nil {
		t.Fatalf("expected nil error for compatibility, got %v", err)
	}
	if len(assets) != 0 {
		t.Fatalf("expected empty assets on rule parse failure, got %d", len(assets))
	}
}
