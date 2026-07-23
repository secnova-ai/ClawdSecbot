package core

import (
	"path/filepath"
	"reflect"
	"testing"
)

func newRuntimeClaimTestConfigPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name, "openclaw.json")
}

func newRuntimeClaimCandidateForTest(plugin BotPlugin, asset Asset, family, configPath, variant string, priority int, fallback bool) runtimeClaimCandidate {
	return runtimeClaimCandidate{
		plugin: plugin,
		claim: RuntimeAssetClaim{
			Asset:               asset,
			RuntimeFamily:       family,
			CanonicalConfigPath: configPath,
			VariantID:           variant,
			Priority:            priority,
			IsFallback:          fallback,
		},
	}
}

func findResolvedRuntimeAsset(t *testing.T, assets []resolvedRuntimeAsset, id string) resolvedRuntimeAsset {
	t.Helper()
	for _, asset := range assets {
		if asset.asset.ID == id {
			return asset
		}
	}
	t.Fatalf("resolved asset %q not found: %#v", id, assets)
	return resolvedRuntimeAsset{}
}

func TestResolveRuntimeAssetClaims_HigherPriorityDWTSClawWinsAndReusesFallbackID(t *testing.T) {
	configPath := newRuntimeClaimTestConfigPath(t, "shared")
	openclaw := newTestPlugin("Openclaw")
	dwtsclaw := newTestPlugin("DWTSClaw")
	winnerSections := []DisplaySection{{
		Title: "Runtime",
		Items: []DisplayItem{{Label: "Gateway", Value: "dwts", Status: "safe"}},
	}}

	resolved := resolveRuntimeAssetClaims([]runtimeClaimCandidate{
		newRuntimeClaimCandidateForTest(openclaw, Asset{
			ID:           "openclaw:shared",
			SourcePlugin: "legacy-openclaw",
			Name:         "Openclaw Gateway",
		}, "openclaw", configPath, "openclaw", 10, true),
		newRuntimeClaimCandidateForTest(dwtsclaw, Asset{
			ID:              "dwtsclaw:detected",
			SourcePlugin:    "legacy-dwtsclaw",
			Name:            "DWTSClaw Gateway",
			Metadata:        map[string]string{"preserved": "yes"},
			DisplaySections: winnerSections,
		}, " openclaw ", configPath, " dwtsclaw ", 100, false),
	})

	if len(resolved) != 1 {
		t.Fatalf("expected one resolved asset, got %#v", resolved)
	}
	if resolved[0].plugin != dwtsclaw {
		t.Fatalf("expected DWTSClaw plugin to win, got %s", resolved[0].plugin.GetAssetName())
	}
	asset := resolved[0].asset
	if asset.ID != "openclaw:shared" {
		t.Fatalf("expected fallback asset ID, got %q", asset.ID)
	}
	if asset.Name != "DWTSClaw Gateway" {
		t.Fatalf("expected winner name, got %q", asset.Name)
	}
	if !reflect.DeepEqual(asset.DisplaySections, winnerSections) {
		t.Fatalf("expected winner display sections, got %#v", asset.DisplaySections)
	}
	if asset.SourcePlugin != "DWTSClaw" {
		t.Fatalf("expected winner source plugin, got %q", asset.SourcePlugin)
	}
	if got := asset.Metadata["runtime_family"]; got != "openclaw" {
		t.Fatalf("expected runtime family, got %q", got)
	}
	if got := asset.Metadata["runtime_variant"]; got != "dwtsclaw" {
		t.Fatalf("expected runtime variant, got %q", got)
	}
	if got := asset.Metadata["runtime_config_path"]; got != ResolveStableConfigPathFingerprint(configPath) {
		t.Fatalf("expected canonical runtime config path, got %q", got)
	}
	if got := asset.Metadata["detected_variants"]; got != "dwtsclaw,openclaw" {
		t.Fatalf("expected sorted detected variants, got %q", got)
	}
	if got := asset.Metadata["preserved"]; got != "yes" {
		t.Fatalf("expected existing metadata preserved, got %q", got)
	}
	if _, ok := asset.Metadata["runtime_claim_ambiguous"]; ok {
		t.Fatalf("did not expect ambiguity marker: %#v", asset.Metadata)
	}
}

func TestResolveRuntimeAssetClaims_SingleValidClaimKeepsOriginalID(t *testing.T) {
	configPath := newRuntimeClaimTestConfigPath(t, "single")
	plugin := newTestPlugin("DWTSClaw")

	resolved := resolveRuntimeAssetClaims([]runtimeClaimCandidate{
		newRuntimeClaimCandidateForTest(plugin, Asset{
			ID:   "dwtsclaw:original",
			Name: "DWTSClaw Gateway",
		}, " openclaw ", configPath, " dwtsclaw ", 100, false),
	})

	if len(resolved) != 1 {
		t.Fatalf("expected one resolved asset, got %#v", resolved)
	}
	asset := resolved[0].asset
	if asset.ID != "dwtsclaw:original" {
		t.Fatalf("expected original ID, got %q", asset.ID)
	}
	if asset.SourcePlugin != "DWTSClaw" {
		t.Fatalf("expected source plugin DWTSClaw, got %q", asset.SourcePlugin)
	}
	if got := asset.Metadata["runtime_variant"]; got != "dwtsclaw" {
		t.Fatalf("expected trimmed variant, got %q", got)
	}
}

func TestResolveRuntimeAssetClaims_DistinctCanonicalConfigPathsRemainSeparate(t *testing.T) {
	firstPath := newRuntimeClaimTestConfigPath(t, "first")
	secondPath := newRuntimeClaimTestConfigPath(t, "second")
	plugin := newTestPlugin("Openclaw")

	resolved := resolveRuntimeAssetClaims([]runtimeClaimCandidate{
		newRuntimeClaimCandidateForTest(plugin, Asset{ID: "openclaw:first", Name: "First"}, "openclaw", firstPath, "openclaw", 1, false),
		newRuntimeClaimCandidateForTest(plugin, Asset{ID: "openclaw:second", Name: "Second"}, "openclaw", secondPath, "openclaw", 1, false),
	})

	if len(resolved) != 2 {
		t.Fatalf("expected two assets for distinct configs, got %#v", resolved)
	}
	first := findResolvedRuntimeAsset(t, resolved, "openclaw:first")
	if got := first.asset.Metadata["runtime_config_path"]; got != ResolveStableConfigPathFingerprint(firstPath) {
		t.Fatalf("expected first canonical path, got %q", got)
	}
	second := findResolvedRuntimeAsset(t, resolved, "openclaw:second")
	if got := second.asset.Metadata["runtime_config_path"]; got != ResolveStableConfigPathFingerprint(secondPath) {
		t.Fatalf("expected second canonical path, got %q", got)
	}
}

func TestResolveRuntimeAssetClaims_TiedHighestPriorityPrefersFallback(t *testing.T) {
	configPath := newRuntimeClaimTestConfigPath(t, "tied-fallback")
	dwtsclaw := newTestPlugin("DWTSClaw")
	openclaw := newTestPlugin("Openclaw")

	resolved := resolveRuntimeAssetClaims([]runtimeClaimCandidate{
		newRuntimeClaimCandidateForTest(dwtsclaw, Asset{ID: "dwtsclaw:detected", Name: "DWTSClaw"}, "openclaw", configPath, "dwtsclaw", 100, false),
		newRuntimeClaimCandidateForTest(openclaw, Asset{ID: "openclaw:fallback", Name: "Openclaw"}, "openclaw", configPath, "openclaw", 100, true),
	})

	if len(resolved) != 1 {
		t.Fatalf("expected one resolved asset, got %#v", resolved)
	}
	if resolved[0].plugin != openclaw {
		t.Fatalf("expected fallback plugin to win tie, got %s", resolved[0].plugin.GetAssetName())
	}
	if resolved[0].asset.ID != "openclaw:fallback" {
		t.Fatalf("expected fallback ID, got %q", resolved[0].asset.ID)
	}
	if _, ok := resolved[0].asset.Metadata["runtime_claim_ambiguous"]; ok {
		t.Fatalf("fallback tie must not be ambiguous: %#v", resolved[0].asset.Metadata)
	}
}

func TestResolveRuntimeAssetClaims_TiedHighestPriorityWithoutFallbackUsesPluginIDAndMarksAmbiguous(t *testing.T) {
	configPath := newRuntimeClaimTestConfigPath(t, "tied-no-fallback")
	zulu := newTestPlugin("ZuluClaw")
	alpha := newTestPlugin("AlphaClaw")

	resolved := resolveRuntimeAssetClaims([]runtimeClaimCandidate{
		newRuntimeClaimCandidateForTest(zulu, Asset{ID: "zulu:detected", Name: "ZuluClaw"}, "openclaw", configPath, "zulu", 100, false),
		newRuntimeClaimCandidateForTest(alpha, Asset{ID: "alpha:detected", Name: "AlphaClaw"}, "openclaw", configPath, "alpha", 100, false),
	})

	if len(resolved) != 1 {
		t.Fatalf("expected one resolved asset, got %#v", resolved)
	}
	if resolved[0].plugin != alpha {
		t.Fatalf("expected lowest normalized plugin ID to win, got %s", resolved[0].plugin.GetID())
	}
	if resolved[0].asset.ID != "alpha:detected" {
		t.Fatalf("expected winner original ID, got %q", resolved[0].asset.ID)
	}
	if got := resolved[0].asset.Metadata["runtime_claim_ambiguous"]; got != "true" {
		t.Fatalf("expected ambiguity marker, got %q", got)
	}
}

func TestResolveRuntimeAssetClaims_EqualPrimaryTieBreakersUsePluginNameDeterministically(t *testing.T) {
	configPath := newRuntimeClaimTestConfigPath(t, "same-primary-tie-breakers")
	zulu := newTestPlugin("ZuluClaw")
	zulu.id = "shared-plugin"
	alpha := newTestPlugin("AlphaClaw")
	alpha.id = "shared-plugin"

	zuluCandidate := newRuntimeClaimCandidateForTest(zulu, Asset{ID: "shared:asset", Name: "Shared"}, "openclaw", configPath, "shared", 100, false)
	alphaCandidate := newRuntimeClaimCandidateForTest(alpha, Asset{ID: "shared:asset", Name: "Shared"}, "openclaw", configPath, "shared", 100, false)

	first := resolveRuntimeAssetClaims([]runtimeClaimCandidate{zuluCandidate, alphaCandidate})
	second := resolveRuntimeAssetClaims([]runtimeClaimCandidate{alphaCandidate, zuluCandidate})
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected one result for both orders, first=%#v second=%#v", first, second)
	}
	if first[0].plugin != alpha || second[0].plugin != alpha {
		t.Fatalf("expected AlphaClaw to win deterministic final tie-break, first=%s second=%s", first[0].plugin.GetAssetName(), second[0].plugin.GetAssetName())
	}
	if first[0].asset.Metadata["runtime_claim_ambiguous"] != "true" || second[0].asset.Metadata["runtime_claim_ambiguous"] != "true" {
		t.Fatalf("expected ambiguous metadata on equal-priority non-fallback candidates, first=%#v second=%#v", first[0].asset.Metadata, second[0].asset.Metadata)
	}
}

func TestResolveRuntimeAssetClaims_EmptyFamilyOrConfigPathPassThroughUnchanged(t *testing.T) {
	configPath := newRuntimeClaimTestConfigPath(t, "pass-through")
	emptyFamilyPlugin := newTestPlugin("FamilyEmpty")
	emptyPathPlugin := newTestPlugin("PathEmpty")
	emptyFamilyAsset := Asset{
		ID:           "family-empty:asset",
		SourcePlugin: "original-family",
		Name:         "Family Empty",
		Metadata:     map[string]string{"preserved": "family"},
	}
	emptyPathAsset := Asset{
		ID:           "path-empty:asset",
		SourcePlugin: "original-path",
		Name:         "Path Empty",
		Metadata:     map[string]string{"preserved": "path"},
	}

	resolved := resolveRuntimeAssetClaims([]runtimeClaimCandidate{
		newRuntimeClaimCandidateForTest(emptyPathPlugin, emptyPathAsset, "openclaw", " \t ", "path-empty", 10, false),
		newRuntimeClaimCandidateForTest(emptyFamilyPlugin, emptyFamilyAsset, " \t ", configPath, "family-empty", 10, false),
	})

	if len(resolved) != 2 {
		t.Fatalf("expected two pass-through assets, got %#v", resolved)
	}
	gotFamily := findResolvedRuntimeAsset(t, resolved, emptyFamilyAsset.ID)
	if gotFamily.plugin != emptyFamilyPlugin || !reflect.DeepEqual(gotFamily.asset, emptyFamilyAsset) {
		t.Fatalf("expected empty-family asset unchanged, got %#v", gotFamily)
	}
	gotPath := findResolvedRuntimeAsset(t, resolved, emptyPathAsset.ID)
	if gotPath.plugin != emptyPathPlugin || !reflect.DeepEqual(gotPath.asset, emptyPathAsset) {
		t.Fatalf("expected empty-path asset unchanged, got %#v", gotPath)
	}
}

func TestResolveRuntimeAssetClaims_MetadataIsStableAndUsesWinningPlugin(t *testing.T) {
	configPath := newRuntimeClaimTestConfigPath(t, "stable-metadata")
	openclaw := newTestPlugin("Openclaw")
	dwtsclaw := newTestPlugin("DWTSClaw")
	sections := []DisplaySection{{Title: "Runtime", Items: []DisplayItem{{Label: "Mode", Value: "managed", Status: "neutral"}}}}
	fallback := newRuntimeClaimCandidateForTest(openclaw, Asset{
		ID:           "openclaw:stable",
		Name:         "Openclaw Gateway",
		SourcePlugin: "stale-openclaw",
	}, "openclaw", configPath, " alpha ", 1, true)
	winner := newRuntimeClaimCandidateForTest(dwtsclaw, Asset{
		ID:              "dwtsclaw:stable",
		Name:            "DWTSClaw Gateway",
		SourcePlugin:    "stale-dwtsclaw",
		DisplaySections: sections,
	}, "openclaw", configPath, " zeta ", 100, false)

	first := resolveRuntimeAssetClaims([]runtimeClaimCandidate{fallback, winner})
	second := resolveRuntimeAssetClaims([]runtimeClaimCandidate{winner, fallback})
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected one asset from both runs, got first=%#v second=%#v", first, second)
	}
	if !reflect.DeepEqual(first[0].asset.Metadata, second[0].asset.Metadata) {
		t.Fatalf("expected stable metadata, first=%#v second=%#v", first[0].asset.Metadata, second[0].asset.Metadata)
	}
	asset := first[0].asset
	if asset.SourcePlugin != "DWTSClaw" {
		t.Fatalf("expected winning source plugin, got %q", asset.SourcePlugin)
	}
	if asset.ID != "openclaw:stable" {
		t.Fatalf("expected fallback ID reuse, got %q", asset.ID)
	}
	if got := asset.Metadata["detected_variants"]; got != "alpha,zeta" {
		t.Fatalf("expected sorted variants, got %q", got)
	}
	if got := asset.Metadata["runtime_family"]; got != "openclaw" {
		t.Fatalf("expected runtime family, got %q", got)
	}
	if got := asset.Metadata["runtime_variant"]; got != "zeta" {
		t.Fatalf("expected winning runtime variant, got %q", got)
	}
	if got := asset.Metadata["runtime_config_path"]; got != ResolveStableConfigPathFingerprint(configPath) {
		t.Fatalf("expected canonical config path, got %q", got)
	}
	if !reflect.DeepEqual(asset.DisplaySections, sections) {
		t.Fatalf("expected winner display sections, got %#v", asset.DisplaySections)
	}
}
