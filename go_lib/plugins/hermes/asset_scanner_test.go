package hermes

import (
	"testing"

	"go_lib/core"
)

func TestRewriteStableAssetID_UsesConfigPathOnly(t *testing.T) {
	scanner := NewHermesAssetScanner("")
	asset := &core.Asset{
		ID: "volatile",
		Metadata: map[string]string{
			"config_path": "/Users/test/.hermes/config.yaml",
		},
		Ports:        []int{7777, 9999},
		ProcessPaths: []string{"/usr/local/bin/hermes"},
	}

	scanner.rewriteStableAssetID(asset)
	if asset.ID == "" {
		t.Fatal("expected non-empty stable asset id")
	}
	want := core.ComputeAssetID(hermesAssetName, "/Users/test/.hermes/config.yaml", nil, nil)
	if asset.ID != want {
		t.Fatalf("asset id mismatch: got=%s want=%s", asset.ID, want)
	}
}
