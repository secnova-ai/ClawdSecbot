package dwtsclaw

import (
	"path/filepath"
	"reflect"
	"testing"

	"go_lib/core"
)

func TestPluginBuildRuntimeAssetClaimsRequiresProductEvidence(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "openclaw.json")
	asset := core.Asset{
		ID: "dwtsclaw:test",
		Metadata: map[string]string{
			"config_path":      "  " + configPath + "  ",
			"product_evidence": "install-root,process:dwtsclaw",
		},
	}

	claims := (&Plugin{}).BuildRuntimeAssetClaims([]core.Asset{asset})
	if len(claims) != 1 {
		t.Fatalf("claim count = %d, want 1", len(claims))
	}
	claim := claims[0]
	if claim.RuntimeFamily != "openclaw" || claim.VariantID != "dwtsclaw" {
		t.Fatalf("unexpected claim identity: %#v", claim)
	}
	if claim.Priority != dwtsclawRuntimePriority || claim.IsFallback {
		t.Fatalf("unexpected claim priority/fallback: %#v", claim)
	}
	if got, want := claim.CanonicalConfigPath, core.ResolveStableConfigPathFingerprint(configPath); got != want {
		t.Fatalf("canonical config path = %q, want %q", got, want)
	}
	if got, want := claim.Evidence, []string{"install-root", "process:dwtsclaw"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("evidence = %#v, want %#v", got, want)
	}
}

func TestPluginBuildRuntimeAssetClaimsSkipsGenericOpenclawConfig(t *testing.T) {
	claims := (&Plugin{}).BuildRuntimeAssetClaims([]core.Asset{{
		ID:       "dwtsclaw:test",
		Metadata: map[string]string{"config_path": filepath.Join(t.TempDir(), "openclaw.json")},
	}})
	if len(claims) != 0 {
		t.Fatalf("claim count = %d, want 0", len(claims))
	}
}
