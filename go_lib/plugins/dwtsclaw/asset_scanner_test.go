package dwtsclaw

import (
	"os"
	"path/filepath"
	"testing"

	"go_lib/core"
)

type scannerTestCollector struct {
	snapshot core.SystemSnapshot
}

func (c *scannerTestCollector) Collect() (core.SystemSnapshot, error) {
	return c.snapshot, nil
}

func TestAssetScannerDetectsDWTSClawFromInstallEvidence(t *testing.T) {
	previousConfigPath := dwtsclawConfigPathOverride
	previousInstallRoot := dwtsclawInstallRootOverride
	t.Cleanup(func() {
		dwtsclawConfigPathOverride = previousConfigPath
		dwtsclawInstallRootOverride = previousInstallRoot
	})

	dir := t.TempDir()
	configPath := filepath.Join(dir, ".openclaw", "openclaw.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"agents":{"defaults":{"model":"qwen/model-a"}},"models":{"providers":{"qwen":{"baseUrl":"https://example.invalid/v1","apiKey":"test-key"}}}}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	installRoot := filepath.Join(dir, "DWTSClaw")
	entry := filepath.Join(installRoot, "resources", "cfmind", "openclaw.mjs")
	if err := os.MkdirAll(filepath.Dir(entry), 0755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "DWTSClaw.exe"), nil, 0644); err != nil {
		t.Fatalf("write executable marker: %v", err)
	}
	if err := os.WriteFile(entry, nil, 0644); err != nil {
		t.Fatalf("write gateway entry marker: %v", err)
	}

	dwtsclawConfigPathOverride = configPath
	dwtsclawInstallRootOverride = installRoot
	assets, err := newAssetScanner().withCollector(&scannerTestCollector{}).scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("asset count = %d, want 1", len(assets))
	}
	asset := assets[0]
	if asset.Name != dwtsclawAssetName || asset.SourcePlugin != dwtsclawAssetName {
		t.Fatalf("unexpected asset identity: %#v", asset)
	}
	if asset.Metadata["config_path"] != core.ResolveStableConfigPathFingerprint(configPath) {
		t.Fatalf("config path = %q", asset.Metadata["config_path"])
	}
	if asset.Metadata["install_root"] != installRoot {
		t.Fatalf("install root = %q", asset.Metadata["install_root"])
	}
	if asset.Metadata["product_evidence"] != "install-root" {
		t.Fatalf("product evidence = %q", asset.Metadata["product_evidence"])
	}
}

func TestAssetScannerDoesNotClaimGenericOpenclawConfig(t *testing.T) {
	previousConfigPath := dwtsclawConfigPathOverride
	previousInstallRoot := dwtsclawInstallRootOverride
	t.Cleanup(func() {
		dwtsclawConfigPathOverride = previousConfigPath
		dwtsclawInstallRootOverride = previousInstallRoot
	})

	configPath := filepath.Join(t.TempDir(), "openclaw.json")
	if err := os.WriteFile(configPath, []byte(`{"models":{"providers":{"qwen":{"baseUrl":"https://example.invalid/v1"}}}}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	dwtsclawConfigPathOverride = configPath
	dwtsclawInstallRootOverride = filepath.Join(t.TempDir(), "not-dwtsclaw")

	assets, err := newAssetScanner().withCollector(&scannerTestCollector{}).scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(assets) != 0 {
		t.Fatalf("asset count = %d, want 0", len(assets))
	}
}

func TestInstallRootFromCFMindProcessPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "DWTSClaw")
	processPath := filepath.Join(root, "resources", "cfmind", "cfmind.exe")
	if got := installRootFromProcessPath(processPath); got != root {
		t.Fatalf("install root = %q, want %q", got, root)
	}
}
