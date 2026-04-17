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

type lifecycleCapabilityTestPlugin struct {
	*testPlugin
	onAppExitCalls      []string
	restoreDefaultCalls []string
}

func newGatewayCapabilityTestPlugin(assetName string) *gatewayCapabilityTestPlugin {
	return &gatewayCapabilityTestPlugin{
		testPlugin:       newTestPlugin(assetName),
		syncByAssetCalls: make([]string, 0, 1),
	}
}

func newLifecycleCapabilityTestPlugin(assetName string) *lifecycleCapabilityTestPlugin {
	return &lifecycleCapabilityTestPlugin{
		testPlugin:          newTestPlugin(assetName),
		onAppExitCalls:      make([]string, 0, 1),
		restoreDefaultCalls: make([]string, 0, 1),
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

func (p *lifecycleCapabilityTestPlugin) OnAppExit(assetID string) string {
	p.onAppExitCalls = append(p.onAppExitCalls, assetID)
	return fmt.Sprintf(`{"success":true,"mode":"asset","plugin":"%s","asset_id":"%s"}`, strings.ToLower(p.GetAssetName()), assetID)
}

func (p *lifecycleCapabilityTestPlugin) RestoreBotDefaultState(assetID string) string {
	p.restoreDefaultCalls = append(p.restoreDefaultCalls, assetID)
	return fmt.Sprintf(`{"success":true,"mode":"asset","plugin":"%s","asset_id":"%s"}`, strings.ToLower(p.GetAssetName()), assetID)
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

func TestNotifyAppExitByPlugin_PrioritizesAssetID(t *testing.T) {
	pm := resetCapabilityTestPluginManager(t)

	openPlugin := newLifecycleCapabilityTestPlugin("Openclaw")
	nullPlugin := newLifecycleCapabilityTestPlugin("Nullclaw")
	pm.Register(openPlugin)
	pm.Register(nullPlugin)
	pm.bindAssetInstance(nullPlugin, Asset{
		ID:           "nullclaw:asset-1",
		SourcePlugin: "Nullclaw",
	})

	result := NotifyAppExitByPlugin("Openclaw", "nullclaw:asset-1")

	if !strings.Contains(result, `"plugin":"nullclaw"`) {
		t.Fatalf("expected routing by asset_id to nullclaw plugin, got: %s", result)
	}
	if len(nullPlugin.onAppExitCalls) != 1 || nullPlugin.onAppExitCalls[0] != "nullclaw:asset-1" {
		t.Fatalf("expected nullclaw OnAppExit called once with asset_id, got: %#v", nullPlugin.onAppExitCalls)
	}
	if len(openPlugin.onAppExitCalls) != 0 {
		t.Fatalf("expected openclaw OnAppExit not called, got: %#v", openPlugin.onAppExitCalls)
	}
}

func TestNotifyAppExitByPlugin_AssetIDMissingBindingReturnsError(t *testing.T) {
	pm := resetCapabilityTestPluginManager(t)
	pm.Register(newLifecycleCapabilityTestPlugin("Openclaw"))

	result := NotifyAppExitByPlugin("Openclaw", "missing-asset-id")
	if !strings.Contains(result, "no plugin found for asset_id: missing-asset-id") {
		t.Fatalf("expected asset_id binding error, got: %s", result)
	}
}

func TestRestoreBotDefaultStateByPlugin_PrioritizesAssetID(t *testing.T) {
	pm := resetCapabilityTestPluginManager(t)

	openPlugin := newLifecycleCapabilityTestPlugin("Openclaw")
	nullPlugin := newLifecycleCapabilityTestPlugin("Nullclaw")
	pm.Register(openPlugin)
	pm.Register(nullPlugin)
	pm.bindAssetInstance(nullPlugin, Asset{
		ID:           "nullclaw:asset-1",
		SourcePlugin: "Nullclaw",
	})

	result := RestoreBotDefaultStateByPlugin("Openclaw", "nullclaw:asset-1")

	if !strings.Contains(result, `"plugin":"nullclaw"`) {
		t.Fatalf("expected routing by asset_id to nullclaw plugin, got: %s", result)
	}
	if len(nullPlugin.restoreDefaultCalls) != 1 || nullPlugin.restoreDefaultCalls[0] != "nullclaw:asset-1" {
		t.Fatalf("expected nullclaw RestoreBotDefaultState called once with asset_id, got: %#v", nullPlugin.restoreDefaultCalls)
	}
	if len(openPlugin.restoreDefaultCalls) != 0 {
		t.Fatalf("expected openclaw RestoreBotDefaultState not called, got: %#v", openPlugin.restoreDefaultCalls)
	}
}

type skillDeleteCapabilityTestPlugin struct {
	*testPlugin
	ownedPrefix string
	calls       int
}

func newSkillDeleteCapabilityTestPlugin(assetName, ownedPrefix string) *skillDeleteCapabilityTestPlugin {
	return &skillDeleteCapabilityTestPlugin{
		testPlugin:  newTestPlugin(assetName),
		ownedPrefix: strings.ToLower(ownedPrefix),
	}
}

func (p *skillDeleteCapabilityTestPlugin) DeleteSkill(skillPath string) string {
	p.calls++
	if strings.HasPrefix(strings.ToLower(skillPath), p.ownedPrefix) {
		return fmt.Sprintf(`{"success":true,"plugin":"%s"}`, strings.ToLower(p.GetAssetName()))
	}
	return fmt.Sprintf(`{"success":false,"error":"skill path is not within skills directory (%s)"}`, strings.ToLower(p.GetAssetName()))
}

func TestDeleteSkillByPlugin_AutoRoutesBySkillPath(t *testing.T) {
	pm := resetCapabilityTestPluginManager(t)

	openPlugin := newSkillDeleteCapabilityTestPlugin("Openclaw", "/openclaw/skills/")
	dintalPlugin := newSkillDeleteCapabilityTestPlugin("Dintalclaw", "/dintalclaw/memory/")
	pm.Register(openPlugin)
	pm.Register(dintalPlugin)

	result := DeleteSkillByPlugin("", "/dintalclaw/memory/security/skill-a")

	if !strings.Contains(result, `"success":true`) || !strings.Contains(strings.ToLower(result), `"plugin":"dintalclaw"`) {
		t.Fatalf("expected auto routed delete to dintalclaw plugin, got: %s", result)
	}
	// 自动路由命中后应尽早返回，未命中的插件调用次数不做强约束（0 或 1 均可）。
	if openPlugin.calls > 1 {
		t.Fatalf("expected openclaw delete attempted at most once, got: %d", openPlugin.calls)
	}
	if dintalPlugin.calls != 1 {
		t.Fatalf("expected dintalclaw delete attempted once, got: %d", dintalPlugin.calls)
	}
}

func TestDeleteSkillByPlugin_AutoRouteAllFailReturnsError(t *testing.T) {
	pm := resetCapabilityTestPluginManager(t)

	openPlugin := newSkillDeleteCapabilityTestPlugin("Openclaw", "/openclaw/skills/")
	dintalPlugin := newSkillDeleteCapabilityTestPlugin("Dintalclaw", "/dintalclaw/memory/")
	pm.Register(openPlugin)
	pm.Register(dintalPlugin)

	result := DeleteSkillByPlugin("", "/unknown/root/skill-a")

	if !strings.Contains(result, `"success":false`) {
		t.Fatalf("expected failure when no plugin owns path, got: %s", result)
	}
	if !strings.Contains(result, "failed to route delete_skill by path") {
		t.Fatalf("expected routing failure reason in error, got: %s", result)
	}
}
