package core

import (
	"strings"
	"testing"

	"go_lib/plugin_sdk"
)

type testPlugin struct {
	assetName string
	id        string
	manifest  plugin_sdk.PluginManifest
	schema    *plugin_sdk.AssetUISchema
	assets    []Asset
}

func (p *testPlugin) GetAssetName() string {
	return p.assetName
}

func (p *testPlugin) GetID() string {
	return p.id
}

func (p *testPlugin) GetManifest() plugin_sdk.PluginManifest {
	return p.manifest
}

func (p *testPlugin) GetAssetUISchema() *plugin_sdk.AssetUISchema {
	return p.schema
}

func (p *testPlugin) RequiresBotModelConfig() bool {
	return true
}

func (p *testPlugin) ScanAssets() ([]Asset, error) {
	return p.assets, nil
}

func (p *testPlugin) AssessRisks(scannedHashes map[string]bool) ([]Risk, error) {
	return nil, nil
}

func (p *testPlugin) MitigateRisk(riskInfo string) string {
	return `{"success":true}`
}

func (p *testPlugin) StartProtection(assetID string, config ProtectionConfig) error {
	return nil
}

func (p *testPlugin) StopProtection(assetID string) error {
	return nil
}

func (p *testPlugin) GetProtectionStatus(assetID string) ProtectionStatus {
	return ProtectionStatus{}
}

type mitigationAwarePlugin struct {
	testPlugin
	handledRiskID string
}

type riskAssessPlugin struct {
	testPlugin
	risks []Risk
}

func (p *mitigationAwarePlugin) MitigateRisk(riskInfo string) string {
	if strings.Contains(riskInfo, `"`+"id"+`":"`+p.handledRiskID+`"`) {
		return `{"success":true}`
	}
	return `{"success":false,"error":"not implemented"}`
}

func (p *riskAssessPlugin) AssessRisks(scannedHashes map[string]bool) ([]Risk, error) {
	return p.risks, nil
}

func newTestPlugin(assetName string) *testPlugin {
	return &testPlugin{
		assetName: assetName,
		id:        strings.ToLower(assetName),
		manifest: plugin_sdk.PluginManifest{
			PluginID:           strings.ToLower(assetName),
			BotType:            strings.ToLower(assetName),
			DisplayName:        assetName,
			APIVersion:         "v1",
			Capabilities:       []string{"scan", "mitigation"},
			SupportedPlatforms: []string{"macos"},
		},
		schema: &plugin_sdk.AssetUISchema{
			ID:      strings.ToLower(assetName) + ".asset.v1",
			Version: "1",
		},
	}
}

func TestPluginManager_GetPluginByAssetName_CaseInsensitive(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}
	p := newTestPlugin("Openclaw")

	pm.Register(p)

	if got := pm.GetPluginByAssetName("openclaw"); got == nil || got.GetAssetName() != p.assetName {
		t.Fatalf("expected plugin by lower-case asset name, got %#v", got)
	}
	if got := pm.GetPluginByAssetName("OPENCLAW"); got == nil || got.GetAssetName() != p.assetName {
		t.Fatalf("expected plugin by upper-case asset name, got %#v", got)
	}
}

func TestPluginManager_Register_DuplicateNormalizedAssetNameIgnored(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}
	first := newTestPlugin("Openclaw")
	second := newTestPlugin("openclaw")

	pm.Register(first)
	pm.Register(second)

	if got := pm.GetPluginCount(); got != 1 {
		t.Fatalf("expected 1 registered plugin, got %d", got)
	}
}

func TestPluginManager_ScanAssets_BindsInstanceByAssetID(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}

	p := newTestPlugin("Openclaw")
	p.assets = []Asset{
		{ID: "openclaw:abc123", Name: "Openclaw", SourcePlugin: "Openclaw"},
	}

	pm.Register(p)
	assets, err := pm.ScanAllAssets()
	if err != nil {
		t.Fatalf("ScanAllAssets failed: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(assets))
	}
	if got := pm.GetAssetInstanceCount(); got != 1 {
		t.Fatalf("expected 1 asset plugin instance, got %d", got)
	}
	if got := pm.GetPluginByAssetID("openclaw:abc123"); got == nil {
		t.Fatal("expected plugin instance by assetID")
	}
}

func TestPluginManager_GetProtectionStatus_ResolvedByAssetID(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}

	p := newTestPlugin("Openclaw")
	p.assets = []Asset{
		{ID: "openclaw:abc123", Name: "Openclaw", SourcePlugin: "Openclaw"},
	}

	pm.Register(p)
	if _, err := pm.ScanAllAssets(); err != nil {
		t.Fatalf("ScanAllAssets failed: %v", err)
	}

	if _, err := pm.GetProtectionStatus("openclaw:abc123"); err != nil {
		t.Fatalf("expected resolve by assetID, got error: %v", err)
	}
}

func TestPluginManager_MitigateRisk_RejectsSourcePluginInvalid(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}
	p := &mitigationAwarePlugin{
		testPlugin:    *newTestPlugin("Openclaw"),
		handledRiskID: "logging_redact_off",
	}
	pm.Register(p)

	result := pm.MitigateRisk(`{"id":"logging_redact_off","source_plugin":"core"}`)
	if !strings.Contains(result, `"success":false`) {
		t.Fatalf("expected strict failure for unknown source plugin, got: %s", result)
	}
}

func TestPluginManager_MitigateRisk_RejectsSourcePluginMissing(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}
	p := &mitigationAwarePlugin{
		testPlugin:    *newTestPlugin("Openclaw"),
		handledRiskID: "logging_redact_off",
	}
	pm.Register(p)

	result := pm.MitigateRisk(`{"id":"logging_redact_off"}`)
	if !strings.Contains(result, `"source_plugin is required"`) {
		t.Fatalf("expected strict source_plugin required error, got: %s", result)
	}
}

func TestPluginManager_MitigateRisk_RoutesBySourcePlugin(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}
	p := &mitigationAwarePlugin{
		testPlugin:    *newTestPlugin("Openclaw"),
		handledRiskID: "logging_redact_off",
	}
	pm.Register(p)

	result := pm.MitigateRisk(`{"id":"logging_redact_off","source_plugin":"openclaw"}`)
	if !strings.Contains(result, `"success":true`) {
		t.Fatalf("expected mitigation routed by source_plugin, got: %s", result)
	}
}

func TestPluginManager_GetAllPluginInfos_IncludesManifestAndSchema(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}
	p := newTestPlugin("Openclaw")

	pm.Register(p)
	infos := pm.GetAllPluginInfos()
	if len(infos) != 1 {
		t.Fatalf("expected 1 plugin info, got %d", len(infos))
	}

	info := infos[0]
	if info.ID != "openclaw" {
		t.Fatalf("expected plugin id openclaw, got %q", info.ID)
	}
	if !info.RequiresBotModelConfig {
		t.Fatalf("expected requires_bot_model_config=true, got false")
	}
	if info.Manifest == nil || info.Manifest.PluginID != "openclaw" {
		t.Fatalf("expected manifest plugin_id openclaw, got %#v", info.Manifest)
	}
	if info.AssetUISchema == nil || info.AssetUISchema.ID != "openclaw.asset.v1" {
		t.Fatalf("expected schema id openclaw.asset.v1, got %#v", info.AssetUISchema)
	}

	// Ensure schema is cloned.
	info.AssetUISchema.ID = "mutated"
	infos2 := pm.GetAllPluginInfos()
	if infos2[0].AssetUISchema == nil || infos2[0].AssetUISchema.ID != "openclaw.asset.v1" {
		t.Fatalf("expected original schema to remain intact, got %#v", infos2[0].AssetUISchema)
	}
}

func TestPluginManager_AssessAllRisks_IncludesAssetNameInArgs(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}
	p := &riskAssessPlugin{
		testPlugin: *newTestPlugin("Openclaw"),
		risks: []Risk{
			{
				ID:    "sample_risk",
				Title: "Sample Risk",
				Level: RiskLevelMedium,
			},
			{
				ID:    "sample_risk_with_args",
				Title: "Sample Risk with Args",
				Level: RiskLevelHigh,
				Args: map[string]interface{}{
					"asset_name": "custom_asset",
				},
			},
		},
	}
	pm.Register(p)

	risks, err := pm.AssessAllRisks(nil)
	if err != nil {
		t.Fatalf("AssessAllRisks failed: %v", err)
	}
	if len(risks) != 2 {
		t.Fatalf("expected 2 risks, got %d", len(risks))
	}

	if got := risks[0].SourcePlugin; got != "Openclaw" {
		t.Fatalf("expected source plugin Openclaw, got %q", got)
	}
	if got := risks[0].Args["asset_name"]; got != "Openclaw" {
		t.Fatalf("expected injected asset_name Openclaw, got %#v", got)
	}
	if got := risks[1].Args["asset_name"]; got != "custom_asset" {
		t.Fatalf("expected existing asset_name to be kept, got %#v", got)
	}
}
