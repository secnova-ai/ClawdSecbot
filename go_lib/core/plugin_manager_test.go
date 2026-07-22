package core

import (
	"path/filepath"
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
	mainPID   int
}

type claimingTestPlugin struct {
	*testPlugin
	claims []RuntimeAssetClaim
}

func (p *claimingTestPlugin) BuildRuntimeAssetClaims(assets []Asset) []RuntimeAssetClaim {
	return append([]RuntimeAssetClaim(nil), p.claims...)
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

func (p *testPlugin) GetMainProcessPID(asset Asset) (int, bool) {
	if p.mainPID <= 0 {
		return 0, false
	}
	return p.mainPID, true
}

func (p *testPlugin) AssessRisks(scannedHashes map[string]bool, assets []Asset) ([]Risk, error) {
	return nil, nil
}

func (p *testPlugin) GetVulnInfoJSON() []byte {
	return nil
}

func (p *testPlugin) CompareVulnerabilityVersion(current, target string) (int, bool) {
	switch {
	case current < target:
		return -1, true
	case current > target:
		return 1, true
	default:
		return 0, true
	}
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

type vulnAwarePlugin struct {
	testPlugin
	vulnInfo []byte
}

func (p *mitigationAwarePlugin) MitigateRisk(riskInfo string) string {
	if strings.Contains(riskInfo, `"`+"id"+`":"`+p.handledRiskID+`"`) {
		return `{"success":true}`
	}
	return `{"success":false,"error":"not implemented"}`
}

func (p *riskAssessPlugin) AssessRisks(scannedHashes map[string]bool, assets []Asset) ([]Risk, error) {
	return p.risks, nil
}

func (p *vulnAwarePlugin) GetVulnInfoJSON() []byte {
	return p.vulnInfo
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

func newTestPluginManager() *PluginManager {
	return &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}
}

func newClaimingTestPlugin(assetName, variantID, configPath string, priority int, fallback bool) *claimingTestPlugin {
	base := newTestPlugin(assetName)
	asset := Asset{
		ID:       ComputeAssetID(assetName, configPath),
		Name:     assetName,
		Metadata: map[string]string{"config_path": configPath},
	}
	base.assets = []Asset{asset}
	return &claimingTestPlugin{
		testPlugin: base,
		claims: []RuntimeAssetClaim{{
			Asset:               asset,
			RuntimeFamily:       "openclaw",
			CanonicalConfigPath: configPath,
			VariantID:           variantID,
			Priority:            priority,
			IsFallback:          fallback,
		}},
	}
}

func TestPluginManager_ScanAllAssets_ClaimsSharedRuntimeBeforeBinding(t *testing.T) {
	pm := newTestPluginManager()
	configPath := filepath.Join(t.TempDir(), "openclaw.json")
	openclaw := newClaimingTestPlugin("Openclaw", "openclaw", configPath, 10, true)
	dwtsclaw := newClaimingTestPlugin("DWTSClaw", "dwtsclaw", configPath, 90, false)

	pm.Register(openclaw)
	pm.Register(dwtsclaw)

	assets, err := pm.ScanAllAssets()
	if err != nil {
		t.Fatalf("ScanAllAssets returned error: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("expected one claimed asset, got %#v", assets)
	}
	if assets[0].Name != "DWTSClaw" || assets[0].SourcePlugin != "DWTSClaw" {
		t.Fatalf("expected DWTSClaw presentation, got %#v", assets[0])
	}
	if assets[0].ID != openclaw.assets[0].ID {
		t.Fatalf("expected fallback ID %q, got %q", openclaw.assets[0].ID, assets[0].ID)
	}
	if got := pm.GetPluginByAssetID(assets[0].ID); got != dwtsclaw {
		t.Fatalf("expected DWTSClaw to own the routed instance, got %#v", got)
	}
}

func TestPluginManager_ScanAllAssets_ReconcilesPriorFallbackOwnerBeforeBindingWinner(t *testing.T) {
	pm := newTestPluginManager()
	configPath := filepath.Join(t.TempDir(), "openclaw.json")
	openclaw := newClaimingTestPlugin("Openclaw", "openclaw", configPath, 10, true)
	dwtsclaw := newClaimingTestPlugin("DWTSClaw", "dwtsclaw", configPath, 90, false)
	dwtsclaw.assets = nil
	dwtsclaw.claims = nil
	pm.Register(openclaw)
	pm.Register(dwtsclaw)

	fallbackID := openclaw.assets[0].ID
	if _, err := pm.ScanAllAssets(); err != nil {
		t.Fatalf("first ScanAllAssets returned error: %v", err)
	}
	if got := pm.GetPluginByAssetID(fallbackID); got != openclaw {
		t.Fatalf("expected Openclaw to own the first scan, got %#v", got)
	}

	dwtsAsset := Asset{
		ID:       ComputeAssetID("DWTSClaw", configPath),
		Name:     "DWTSClaw",
		Metadata: map[string]string{"config_path": configPath},
	}
	dwtsclaw.assets = []Asset{dwtsAsset}
	dwtsclaw.claims = []RuntimeAssetClaim{{
		Asset:               dwtsAsset,
		RuntimeFamily:       "openclaw",
		CanonicalConfigPath: configPath,
		VariantID:           "dwtsclaw",
		Priority:            90,
	}}

	assets, err := pm.ScanAllAssets()
	if err != nil {
		t.Fatalf("second ScanAllAssets returned error: %v", err)
	}
	if len(assets) != 1 || assets[0].ID != fallbackID {
		t.Fatalf("expected one winner retaining fallback ID %q, got %#v", fallbackID, assets)
	}
	if got := pm.GetPluginByAssetID(fallbackID); got != dwtsclaw {
		t.Fatalf("expected DWTSClaw to replace the fallback owner, got %#v", got)
	}
	if stale := pm.getAssetsByPlugin("Openclaw"); len(stale) != 0 {
		t.Fatalf("expected losing Openclaw instances to be pruned, got %#v", stale)
	}
}

func TestPluginManager_ScanAllAssets_ClaimWinnerDoesNotDependOnRegistrationOrder(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "openclaw.json")

	scan := func(reverseRegistration bool) Asset {
		pm := newTestPluginManager()
		openclaw := newClaimingTestPlugin("Openclaw", "openclaw", configPath, 10, true)
		dwtsclaw := newClaimingTestPlugin("DWTSClaw", "dwtsclaw", configPath, 90, false)
		if reverseRegistration {
			pm.Register(dwtsclaw)
			pm.Register(openclaw)
		} else {
			pm.Register(openclaw)
			pm.Register(dwtsclaw)
		}
		assets, err := pm.ScanAllAssets()
		if err != nil {
			t.Fatalf("ScanAllAssets returned error: %v", err)
		}
		if len(assets) != 1 {
			t.Fatalf("expected one asset, got %#v", assets)
		}
		return assets[0]
	}

	first := scan(false)
	second := scan(true)
	if first.ID != second.ID || first.SourcePlugin != second.SourcePlugin || first.Name != second.Name {
		t.Fatalf("expected stable winner across registration orders, first=%#v second=%#v", first, second)
	}
}

func TestPluginManager_ScanAllAssets_ClaimedDifferentConfigPathsRemainSeparate(t *testing.T) {
	pm := newTestPluginManager()
	openclaw := newClaimingTestPlugin("Openclaw", "openclaw", filepath.Join(t.TempDir(), "first", "openclaw.json"), 10, true)
	coclaw := newClaimingTestPlugin("CoClaw", "coclaw", filepath.Join(t.TempDir(), "second", "openclaw.json"), 80, false)
	pm.Register(openclaw)
	pm.Register(coclaw)

	assets, err := pm.ScanAllAssets()
	if err != nil {
		t.Fatalf("ScanAllAssets returned error: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected two distinct config assets, got %#v", assets)
	}
	seen := map[string]bool{}
	for _, asset := range assets {
		seen[asset.SourcePlugin] = true
	}
	if !seen["Openclaw"] || !seen["CoClaw"] {
		t.Fatalf("expected both plugin presentations, got %#v", assets)
	}
}

func TestPluginManager_ScanAllAssets_NonClaimPluginStillBindsIndependently(t *testing.T) {
	pm := newTestPluginManager()
	configPath := filepath.Join(t.TempDir(), "openclaw.json")
	openclaw := newClaimingTestPlugin("Openclaw", "openclaw", configPath, 10, true)
	unrelated := newTestPlugin("QClaw")
	unrelated.assets = []Asset{{ID: "qclaw:independent", Name: "QClaw"}}
	pm.Register(openclaw)
	pm.Register(unrelated)

	assets, err := pm.ScanAllAssets()
	if err != nil {
		t.Fatalf("ScanAllAssets returned error: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("expected claimed and unrelated assets, got %#v", assets)
	}
	if got := pm.GetPluginByAssetID("qclaw:independent"); got != unrelated {
		t.Fatalf("expected unrelated plugin instance to remain independently bound, got %#v", got)
	}
}

func TestPluginManager_ScanAssetsByPlugin_UsesClaimArbitration(t *testing.T) {
	pm := newTestPluginManager()
	configPath := filepath.Join(t.TempDir(), "openclaw.json")
	openclaw := newClaimingTestPlugin("Openclaw", "openclaw", configPath, 10, true)
	dwtsclaw := newClaimingTestPlugin("DWTSClaw", "dwtsclaw", configPath, 90, false)
	pm.Register(openclaw)
	pm.Register(dwtsclaw)

	losingAssets, err := pm.ScanAssetsByPlugin("Openclaw")
	if err != nil {
		t.Fatalf("ScanAssetsByPlugin returned error for loser: %v", err)
	}
	if len(losingAssets) != 0 {
		t.Fatalf("expected losing plugin to receive no assets, got %#v", losingAssets)
	}
	winnerAssets, err := pm.ScanAssetsByPlugin("DWTSClaw")
	if err != nil {
		t.Fatalf("ScanAssetsByPlugin returned error for winner: %v", err)
	}
	if len(winnerAssets) != 1 || winnerAssets[0].SourcePlugin != "DWTSClaw" {
		t.Fatalf("expected only DWTSClaw winner asset, got %#v", winnerAssets)
	}
}

func TestPluginManager_ScanAllAssets_InvalidClaimDoesNotDropUnclaimedAsset(t *testing.T) {
	pm := newTestPluginManager()
	configPath := filepath.Join(t.TempDir(), "openclaw.json")
	plugin := newClaimingTestPlugin("Openclaw", "openclaw", configPath, 10, true)
	plugin.claims = []RuntimeAssetClaim{{
		Asset:               Asset{ID: "openclaw:missing"},
		RuntimeFamily:       "openclaw",
		CanonicalConfigPath: configPath,
		VariantID:           "openclaw",
		Priority:            10,
		IsFallback:          true,
	}}
	pm.Register(plugin)

	assets, err := pm.ScanAllAssets()
	if err != nil {
		t.Fatalf("ScanAllAssets returned error: %v", err)
	}
	if len(assets) != 1 || assets[0].ID != plugin.assets[0].ID {
		t.Fatalf("expected unclaimed scanned asset to pass through, got %#v", assets)
	}
	if got := pm.GetPluginByAssetID(plugin.assets[0].ID); got != plugin {
		t.Fatalf("expected passthrough asset to bind to its source plugin, got %#v", got)
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

func TestPluginManager_ScanAssets_AttachesMainProcessPID(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}

	p := newTestPlugin("Openclaw")
	p.mainPID = 67106
	p.assets = []Asset{
		{
			ID:           "openclaw:abc123",
			Name:         "Openclaw",
			SourcePlugin: "Openclaw",
			Metadata:     map[string]string{"pid": "111"},
			DisplaySections: []DisplaySection{
				{
					Title: "Runtime",
					Icon:  "monitor",
					Items: []DisplayItem{{Label: "PID", Value: "111", Status: "neutral"}},
				},
			},
		},
	}

	pm.Register(p)
	assets, err := pm.ScanAllAssets()
	if err != nil {
		t.Fatalf("ScanAllAssets failed: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(assets))
	}
	if got := assets[0].Metadata["main_pid"]; got != "67106" {
		t.Fatalf("expected main_pid 67106, got %q", got)
	}
	if got := assets[0].Metadata["pid"]; got != "67106" {
		t.Fatalf("expected pid 67106, got %q", got)
	}
	if got := assets[0].DisplaySections[0].Items[0].Label; got != "Main PID" {
		t.Fatalf("expected displayed label Main PID, got %q", got)
	}
	if got := assets[0].DisplaySections[0].Items[0].Value; got != "67106" {
		t.Fatalf("expected displayed main PID 67106, got %q", got)
	}
}

func TestPluginManager_ScanAssets_PrunesStaleInstancesForPlugin(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}

	p := newTestPlugin("Openclaw")
	p.assets = []Asset{
		{ID: "openclaw:old001", Name: "Openclaw", SourcePlugin: "Openclaw"},
	}
	pm.Register(p)

	if _, err := pm.ScanAllAssets(); err != nil {
		t.Fatalf("first ScanAllAssets failed: %v", err)
	}
	if got := pm.GetPluginByAssetID("openclaw:old001"); got == nil {
		t.Fatal("expected old asset instance after first scan")
	}

	p.assets = []Asset{
		{ID: "openclaw:new002", Name: "Openclaw", SourcePlugin: "Openclaw"},
	}
	if _, err := pm.ScanAllAssets(); err != nil {
		t.Fatalf("second ScanAllAssets failed: %v", err)
	}

	if got := pm.GetPluginByAssetID("openclaw:old001"); got != nil {
		t.Fatal("expected stale old asset instance to be pruned")
	}
	if got := pm.GetPluginByAssetID("openclaw:new002"); got == nil {
		t.Fatal("expected new asset instance after second scan")
	}
	if got := pm.GetAssetInstanceCount(); got != 1 {
		t.Fatalf("expected exactly 1 active asset instance, got %d", got)
	}
}

func TestPluginManager_ScanAssetsByPlugin_PrunesStaleInstancesForPlugin(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}

	p := newTestPlugin("Openclaw")
	p.assets = []Asset{
		{ID: "openclaw:old001", Name: "Openclaw", SourcePlugin: "Openclaw"},
	}
	pm.Register(p)

	if _, err := pm.ScanAssetsByPlugin("openclaw"); err != nil {
		t.Fatalf("first ScanAssetsByPlugin failed: %v", err)
	}
	if got := pm.GetPluginByAssetID("openclaw:old001"); got == nil {
		t.Fatal("expected old asset instance after first plugin scan")
	}

	p.assets = []Asset{
		{ID: "openclaw:new002", Name: "Openclaw", SourcePlugin: "Openclaw"},
	}
	if _, err := pm.ScanAssetsByPlugin("OPENCLAW"); err != nil {
		t.Fatalf("second ScanAssetsByPlugin failed: %v", err)
	}

	if got := pm.GetPluginByAssetID("openclaw:old001"); got != nil {
		t.Fatal("expected stale old asset instance to be pruned by plugin scan")
	}
	if got := pm.GetPluginByAssetID("openclaw:new002"); got == nil {
		t.Fatal("expected new asset instance after second plugin scan")
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

func TestPluginManager_MitigateRisk_RejectsAssetIDInvalid(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}
	p := &mitigationAwarePlugin{
		testPlugin:    *newTestPlugin("Openclaw"),
		handledRiskID: "logging_redact_off",
	}
	p.assets = []Asset{
		{ID: "openclaw:abc123", Name: "Openclaw", SourcePlugin: "Openclaw"},
	}
	pm.Register(p)
	if _, err := pm.ScanAllAssets(); err != nil {
		t.Fatalf("ScanAllAssets failed: %v", err)
	}

	result := pm.MitigateRisk(`{"id":"logging_redact_off","asset_id":"openclaw:missing"}`)
	if !strings.Contains(result, `"success":false`) {
		t.Fatalf("expected strict failure for unknown asset_id, got: %s", result)
	}
}

func TestPluginManager_MitigateRisk_RejectsAssetIDMissing(t *testing.T) {
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
	if !strings.Contains(result, `"asset_id is required"`) {
		t.Fatalf("expected strict asset_id required error, got: %s", result)
	}
}

func TestPluginManager_MitigateRisk_RoutesByAssetID(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}
	p := &mitigationAwarePlugin{
		testPlugin:    *newTestPlugin("Openclaw"),
		handledRiskID: "logging_redact_off",
	}
	p.assets = []Asset{
		{ID: "openclaw:abc123", Name: "Openclaw", SourcePlugin: "Openclaw"},
	}
	pm.Register(p)
	if _, err := pm.ScanAllAssets(); err != nil {
		t.Fatalf("ScanAllAssets failed: %v", err)
	}

	result := pm.MitigateRisk(`{"id":"logging_redact_off","source_plugin":"core","asset_id":"openclaw:abc123"}`)
	if !strings.Contains(result, `"success":true`) {
		t.Fatalf("expected mitigation routed by asset_id, got: %s", result)
	}
}

func TestPluginManager_MitigateRisk_UsesArgsAssetIDWhenTopLevelMissing(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}
	p := &mitigationAwarePlugin{
		testPlugin:    *newTestPlugin("Openclaw"),
		handledRiskID: "logging_redact_off",
	}
	p.assets = []Asset{
		{ID: "openclaw:abc123", Name: "Openclaw", SourcePlugin: "Openclaw"},
	}
	pm.Register(p)
	if _, err := pm.ScanAllAssets(); err != nil {
		t.Fatalf("ScanAllAssets failed: %v", err)
	}

	result := pm.MitigateRisk(`{"id":"logging_redact_off","args":{"asset_id":"openclaw:abc123"}}`)
	if !strings.Contains(result, `"success":true`) {
		t.Fatalf("expected mitigation routed by args.asset_id, got: %s", result)
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

func TestPluginManager_AssessAllRisks_IncludesAssetNameAndAssetIDInArgs(t *testing.T) {
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
					"asset_id":   "custom:id",
				},
			},
		},
	}
	p.assets = []Asset{
		{ID: "openclaw:abc123", Name: "Openclaw", SourcePlugin: "Openclaw"},
	}
	pm.Register(p)
	if _, err := pm.ScanAllAssets(); err != nil {
		t.Fatalf("ScanAllAssets failed: %v", err)
	}

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
	if got := risks[0].AssetID; got != "openclaw:abc123" {
		t.Fatalf("expected injected asset_id openclaw:abc123, got %#v", got)
	}
	if got := risks[0].Args["asset_id"]; got != "openclaw:abc123" {
		t.Fatalf("expected injected args.asset_id openclaw:abc123, got %#v", got)
	}
	if got := risks[1].Args["asset_name"]; got != "custom_asset" {
		t.Fatalf("expected existing asset_name to be kept, got %#v", got)
	}
	if got := risks[1].AssetID; got != "custom:id" {
		t.Fatalf("expected existing asset_id to be kept, got %#v", got)
	}
}

func TestPluginManager_AssessAllRisks_SkipsPluginWithoutScannedAssets(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}
	openclaw := &riskAssessPlugin{
		testPlugin: *newTestPlugin("Openclaw"),
		risks: []Risk{
			{ID: "openclaw_risk", Title: "Openclaw Risk", Level: RiskLevelMedium},
		},
	}
	openclaw.assets = []Asset{
		{ID: "openclaw:abc123", Name: "Openclaw", SourcePlugin: "Openclaw"},
	}
	qclaw := &riskAssessPlugin{
		testPlugin: *newTestPlugin("QClaw"),
		risks: []Risk{
			{ID: "qclaw_stale_risk", Title: "QClaw stale risk", Level: RiskLevelHigh},
		},
	}

	pm.Register(openclaw)
	pm.Register(qclaw)
	if _, err := pm.ScanAllAssets(); err != nil {
		t.Fatalf("ScanAllAssets failed: %v", err)
	}

	risks, err := pm.AssessAllRisks(nil)
	if err != nil {
		t.Fatalf("AssessAllRisks failed: %v", err)
	}
	if len(risks) != 1 {
		t.Fatalf("expected only scanned plugin risks, got %d: %+v", len(risks), risks)
	}
	if got := risks[0].SourcePlugin; got != "Openclaw" {
		t.Fatalf("expected only Openclaw risk, got source=%q risk=%+v", got, risks[0])
	}
}

func TestPluginManager_AssessAllRisks_AppendsVersionMatchedVulnerabilities(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}
	p := &vulnAwarePlugin{
		testPlugin: *newTestPlugin("Openclaw"),
		vulnInfo: []byte(`[
			{
				"risk_id": "openclaw_cve-xxxx-xxxxx",
				"check_point": {"operation": "<", "version": "2026.3.8"},
				"mitigation": {
					"type": "suggestion",
					"risk": "High",
					"title": "Upgrade Openclaw",
					"description": "Known vulnerable build.",
					"suggestions": "Update Version"
				}
			}
		]`),
	}
	p.assets = []Asset{
		{ID: "openclaw:abc123", Name: "Openclaw", SourcePlugin: "Openclaw", Version: "2026.3.7"},
	}

	pm.Register(p)
	if _, err := pm.ScanAllAssets(); err != nil {
		t.Fatalf("ScanAllAssets failed: %v", err)
	}

	risks, err := pm.AssessAllRisks(nil)
	if err != nil {
		t.Fatalf("AssessAllRisks failed: %v", err)
	}
	if len(risks) != 1 {
		t.Fatalf("expected 1 vulnerability risk, got %d", len(risks))
	}
	if got := risks[0].ID; got != "openclaw_cve-xxxx-xxxxx" {
		t.Fatalf("unexpected risk id: %s", got)
	}
	if got := risks[0].AssetID; got != "openclaw:abc123" {
		t.Fatalf("expected vulnerability asset_id openclaw:abc123, got %s", got)
	}
}

func TestPluginManager_AssessRisksByPlugin_IncludesAssetNameAndAssetIDInArgs(t *testing.T) {
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
					"asset_id":   "custom:id",
				},
			},
		},
	}
	p.assets = []Asset{
		{ID: "openclaw:abc123", Name: "Openclaw", SourcePlugin: "Openclaw"},
	}
	pm.Register(p)
	if _, err := pm.ScanAssetsByPlugin("openclaw"); err != nil {
		t.Fatalf("ScanAssetsByPlugin failed: %v", err)
	}

	risks, err := pm.AssessRisksByPlugin("OPENCLAW", nil)
	if err != nil {
		t.Fatalf("AssessRisksByPlugin failed: %v", err)
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
	if got := risks[0].AssetID; got != "openclaw:abc123" {
		t.Fatalf("expected injected asset_id openclaw:abc123, got %#v", got)
	}
	if got := risks[0].Args["asset_id"]; got != "openclaw:abc123" {
		t.Fatalf("expected injected args.asset_id openclaw:abc123, got %#v", got)
	}
	if got := risks[1].Args["asset_name"]; got != "custom_asset" {
		t.Fatalf("expected existing asset_name to be kept, got %#v", got)
	}
	if got := risks[1].AssetID; got != "custom:id" {
		t.Fatalf("expected existing asset_id to be kept, got %#v", got)
	}
}

func TestPluginManager_AssessRisksByPlugin_AppendsVersionMatchedVulnerabilities(t *testing.T) {
	pm := &PluginManager{
		registeredPlugins: make(map[string]BotPlugin),
		instances:         make(map[string]*AssetPluginInstance),
	}
	p := &vulnAwarePlugin{
		testPlugin: *newTestPlugin("Openclaw"),
		vulnInfo: []byte(`[
			{
				"risk_id": "openclaw_cve-xxxx-xxxxx",
				"check_point": {"operation": "<", "version": "2026.3.8"},
				"mitigation": {
					"type": "suggestion",
					"risk": "High",
					"title": "Upgrade Openclaw",
					"description": "Known vulnerable build.",
					"suggestions": "Update Version"
				}
			}
		]`),
	}
	p.assets = []Asset{
		{ID: "openclaw:abc123", Name: "Openclaw", SourcePlugin: "Openclaw", Version: "2026.3.7"},
	}

	pm.Register(p)
	if _, err := pm.ScanAssetsByPlugin("openclaw"); err != nil {
		t.Fatalf("ScanAssetsByPlugin failed: %v", err)
	}

	risks, err := pm.AssessRisksByPlugin("openclaw", nil)
	if err != nil {
		t.Fatalf("AssessRisksByPlugin failed: %v", err)
	}
	if len(risks) != 1 {
		t.Fatalf("expected 1 vulnerability risk, got %d", len(risks))
	}
	if got := risks[0].ID; got != "openclaw_cve-xxxx-xxxxx" {
		t.Fatalf("unexpected risk id: %s", got)
	}
	if got := risks[0].AssetID; got != "openclaw:abc123" {
		t.Fatalf("expected vulnerability asset_id openclaw:abc123, got %s", got)
	}
	if got := risks[0].Args["asset_id"]; got != "openclaw:abc123" {
		t.Fatalf("expected vulnerability args.asset_id openclaw:abc123, got %#v", got)
	}
}
