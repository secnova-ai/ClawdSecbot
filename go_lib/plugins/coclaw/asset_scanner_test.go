package coclaw

import (
	"os"
	"path/filepath"
	"testing"

	"go_lib/core"
)

type testCollector struct {
	snapshot core.SystemSnapshot
}

func (c *testCollector) Collect() (core.SystemSnapshot, error) {
	return c.snapshot, nil
}

func TestAssetScannerDetectsCoClawConfig(t *testing.T) {
	previousOverride := configPathOverride
	t.Cleanup(func() {
		configPathOverride = previousOverride
	})

	dir := t.TempDir()
	configDir := filepath.Join(dir, ".coclaw")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configPath := filepath.Join(configDir, "openclaw.json")
	configPathOverride = configPath
	if err := os.WriteFile(configPath, []byte(`{
  "agents": {"defaults": {"model": {"primary": "custom-llm/model-a"}}},
  "models": {"providers": {"custom-llm": {"baseUrl": "https://example.invalid/v1"}}},
  "gateway": {"bind": "127.0.0.1", "port": 18790}
}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	assets, err := newAssetScanner(dir).withCollector(&testCollector{
		snapshot: core.SystemSnapshot{
			FileExists: func(path string) bool {
				return path == "~/.coclaw"
			},
		},
	}).scan()
	if err != nil {
		t.Fatalf("scan returned error: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("expected one asset, got %d", len(assets))
	}
	if assets[0].SourcePlugin != coclawAssetName {
		t.Fatalf("source plugin = %q", assets[0].SourcePlugin)
	}
	expectedConfigPath := core.ResolveStableConfigPathFingerprint(configPath)
	if assets[0].Metadata["config_path"] != expectedConfigPath {
		t.Fatalf("config_path = %q, want %q", assets[0].Metadata["config_path"], expectedConfigPath)
	}
	if assets[0].Metadata["gateway_port"] != "18790" {
		t.Fatalf("gateway_port = %q", assets[0].Metadata["gateway_port"])
	}
	if assets[0].Metadata["model_primary"] != "custom-llm/model-a" {
		t.Fatalf("model_primary = %q", assets[0].Metadata["model_primary"])
	}
}
