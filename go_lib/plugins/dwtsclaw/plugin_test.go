package dwtsclaw

import "testing"

func TestPluginManifestAndModelConfigPolicy(t *testing.T) {
	if plugin == nil {
		t.Fatal("expected global DWTSClaw plugin")
	}
	if plugin.GetID() != dwtsclawPluginID {
		t.Fatalf("plugin ID = %q", plugin.GetID())
	}
	if plugin.GetAssetName() != dwtsclawAssetName {
		t.Fatalf("asset name = %q", plugin.GetAssetName())
	}
	if plugin.RequiresBotModelConfig() {
		t.Fatal("DWTSClaw should resolve the forwarding target from openclaw.json")
	}
	manifest := plugin.GetManifest()
	if manifest.PluginID != dwtsclawPluginID || manifest.DisplayName != dwtsclawAssetName {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}
