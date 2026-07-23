package openclaw

import (
	"path/filepath"
	"reflect"
	"testing"

	"go_lib/core"
)

func TestOpenclawPluginBuildRuntimeAssetClaims_ClaimsCompatibleConfigAsFallback(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "openclaw.json")
	rawConfigPath := "  " + configPath + "  "
	asset := core.Asset{
		ID:   "openclaw-asset-id",
		Name: "OpenClaw Gateway",
		Metadata: map[string]string{
			"config_path": rawConfigPath,
			"marker":      "preserved",
		},
	}

	claims := (&OpenclawPlugin{}).BuildRuntimeAssetClaims([]core.Asset{asset})
	if len(claims) != 1 {
		t.Fatalf("claim count = %d, want 1", len(claims))
	}

	claim := claims[0]
	if claim.RuntimeFamily != "openclaw" {
		t.Fatalf("runtime family = %q, want %q", claim.RuntimeFamily, "openclaw")
	}
	if claim.VariantID != "openclaw" {
		t.Fatalf("variant ID = %q, want %q", claim.VariantID, "openclaw")
	}
	if claim.Priority != 10 {
		t.Fatalf("priority = %d, want 10", claim.Priority)
	}
	if !claim.IsFallback {
		t.Fatal("claim should be a fallback")
	}
	if !reflect.DeepEqual(claim.Evidence, []string{"openclaw-compatible-config"}) {
		t.Fatalf("evidence = %#v", claim.Evidence)
	}

	wantCanonicalPath := core.ResolveStableConfigPathFingerprint(configPath)
	if claim.CanonicalConfigPath != wantCanonicalPath {
		t.Fatalf("canonical config path = %q, want %q", claim.CanonicalConfigPath, wantCanonicalPath)
	}
	if claim.Asset.ID != asset.ID {
		t.Fatalf("claim asset ID = %q, want %q", claim.Asset.ID, asset.ID)
	}
	if claim.Asset.Name != asset.Name {
		t.Fatalf("claim asset name = %q, want %q", claim.Asset.Name, asset.Name)
	}
	if claim.Asset.Metadata["config_path"] != rawConfigPath || claim.Asset.Metadata["marker"] != "preserved" {
		t.Fatalf("claim asset metadata = %#v, want original metadata", claim.Asset.Metadata)
	}
}

func TestOpenclawPluginBuildRuntimeAssetClaims_SkipsAssetsWithoutConfigPath(t *testing.T) {
	claims := (&OpenclawPlugin{}).BuildRuntimeAssetClaims([]core.Asset{
		{ID: "missing-metadata"},
		{ID: "empty-config-path", Metadata: map[string]string{"config_path": "   "}},
	})

	if len(claims) != 0 {
		t.Fatalf("claim count = %d, want 0", len(claims))
	}
}
