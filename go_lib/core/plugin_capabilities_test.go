package core

import (
	"fmt"
	"strings"
	"testing"
)

type gatewayCapabilityTestPlugin struct {
	*testPlugin
	syncCalls        int
	syncByAssetCalls []string
}

func newGatewayCapabilityTestPlugin(assetName string) *gatewayCapabilityTestPlugin {
	return &gatewayCapabilityTestPlugin{
		testPlugin:       newTestPlugin(assetName),
		syncByAssetCalls: make([]string, 0, 1),
	}
}

func (p *gatewayCapabilityTestPlugin) SyncGatewaySandbox() string {
	p.syncCalls++
	return fmt.Sprintf(`{"success":true,"mode":"plugin","plugin":"%s"}`, strings.ToLower(p.GetAssetName()))
}

func (p *gatewayCapabilityTestPlugin) SyncGatewaySandboxByAsset(assetID string) string {
	p.syncByAssetCalls = append(p.syncByAssetCalls, assetID)
	return fmt.Sprintf(`{"success":true,"mode":"asset","plugin":"%s","asset_id":"%s"}`, strings.ToLower(p.GetAssetName()), assetID)
}

func (p *gatewayCapabilityTestPlugin) HasInitialBackup() string {
	return `{"success":true,"has_backup":false}`
}

func (p *gatewayCapabilityTestPlugin) RestoreToInitialConfig() string {
	return `{"success":true}`
}

func resetCapabilityTestPluginManager(t *testing.T) *PluginManager {
	t.Helper()
	pm := GetPluginManager()

	pm.mu.Lock()
	oldRegistered := pm.registeredPlugins
	oldInstances := pm.instances
	pm.registeredPlugins = make(map[string]BotPlugin)
	pm.instances = make(map[string]*AssetPluginInstance)
	pm.mu.Unlock()

	t.Cleanup(func() {
		pm.mu.Lock()
		pm.registeredPlugins = oldRegistered
		pm.instances = oldInstances
		pm.mu.Unlock()
	})

	return pm
}

func TestSyncGatewaySandboxByAssetAndPlugin_PrioritizesAssetID(t *testing.T) {
	pm := resetCapabilityTestPluginManager(t)

	openPlugin := newGatewayCapabilityTestPlugin("Openclaw")
	nullPlugin := newGatewayCapabilityTestPlugin("Nullclaw")
	pm.Register(openPlugin)
	pm.Register(nullPlugin)
	pm.bindAssetInstance(nullPlugin, Asset{
		ID:           "nullclaw:asset-1",
		SourcePlugin: "Nullclaw",
	})

	result := SyncGatewaySandboxByAssetAndPlugin("Openclaw", "nullclaw:asset-1")

	if !strings.Contains(result, `"plugin":"nullclaw"`) {
		t.Fatalf("expected routing by asset_id to nullclaw plugin, got: %s", result)
	}
	if len(nullPlugin.syncByAssetCalls) != 1 || nullPlugin.syncByAssetCalls[0] != "nullclaw:asset-1" {
		t.Fatalf("expected nullclaw SyncGatewaySandboxByAsset called once with asset_id, got: %#v", nullPlugin.syncByAssetCalls)
	}
	if openPlugin.syncCalls != 0 || len(openPlugin.syncByAssetCalls) != 0 {
		t.Fatalf("expected openclaw plugin not called, got sync=%d byAsset=%d", openPlugin.syncCalls, len(openPlugin.syncByAssetCalls))
	}
}

func TestSyncGatewaySandboxByAssetAndPlugin_EmptyAssetIDUsesPluginScope(t *testing.T) {
	pm := resetCapabilityTestPluginManager(t)

	openPlugin := newGatewayCapabilityTestPlugin("Openclaw")
	pm.Register(openPlugin)

	result := SyncGatewaySandboxByAssetAndPlugin("Openclaw", "")

	if !strings.Contains(result, `"mode":"plugin"`) {
		t.Fatalf("expected plugin-scope sync when asset_id empty, got: %s", result)
	}
	if openPlugin.syncCalls != 1 {
		t.Fatalf("expected SyncGatewaySandbox called once, got: %d", openPlugin.syncCalls)
	}
	if len(openPlugin.syncByAssetCalls) != 0 {
		t.Fatalf("expected SyncGatewaySandboxByAsset not called, got: %#v", openPlugin.syncByAssetCalls)
	}
}

func TestSyncGatewaySandboxByAssetAndPlugin_AssetIDMissingBindingReturnsError(t *testing.T) {
	pm := resetCapabilityTestPluginManager(t)

	openPlugin := newGatewayCapabilityTestPlugin("Openclaw")
	pm.Register(openPlugin)

	result := SyncGatewaySandboxByAssetAndPlugin("Openclaw", "missing-asset-id")
	if !strings.Contains(result, "no plugin found for asset_id: missing-asset-id") {
		t.Fatalf("expected asset_id binding error, got: %s", result)
	}
}
