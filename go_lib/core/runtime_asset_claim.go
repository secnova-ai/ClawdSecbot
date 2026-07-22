package core

import (
	"sort"
	"strings"
)

// RuntimeAssetClaim describes a plugin's claim over a discovered runtime asset.
type RuntimeAssetClaim struct {
	Asset               Asset
	RuntimeFamily       string
	CanonicalConfigPath string
	VariantID           string
	Priority            int
	IsFallback          bool
	Evidence            []string
}

type runtimeClaimCandidate struct {
	plugin BotPlugin
	claim  RuntimeAssetClaim
}

type resolvedRuntimeAsset struct {
	plugin BotPlugin
	asset  Asset
}

type normalizedRuntimeClaimCandidate struct {
	runtimeClaimCandidate
	runtimeFamily        string
	canonicalConfigPath  string
	variantID            string
	normalizedPluginID   string
	normalizedPluginName string
	normalizedVariantID  string
}

// resolveRuntimeAssetClaims selects one asset for each shared runtime config.
func resolveRuntimeAssetClaims(candidates []runtimeClaimCandidate) []resolvedRuntimeAsset {
	groups := make(map[string][]normalizedRuntimeClaimCandidate)
	passThrough := make([]normalizedRuntimeClaimCandidate, 0)

	for _, candidate := range candidates {
		normalized := normalizeRuntimeClaimCandidate(candidate)
		if normalized.runtimeFamily == "" || normalized.canonicalConfigPath == "" {
			passThrough = append(passThrough, normalized)
			continue
		}

		groupKey := normalized.runtimeFamily + "\x00" + normalized.canonicalConfigPath
		groups[groupKey] = append(groups[groupKey], normalized)
	}

	sort.Slice(passThrough, func(i, j int) bool {
		return runtimeClaimCandidateLess(passThrough[i], passThrough[j])
	})

	resolved := make([]resolvedRuntimeAsset, 0, len(passThrough)+len(groups))
	for _, candidate := range passThrough {
		resolved = append(resolved, resolvedRuntimeAsset{
			plugin: candidate.plugin,
			asset:  candidate.claim.Asset,
		})
	}

	groupKeys := make([]string, 0, len(groups))
	for groupKey := range groups {
		groupKeys = append(groupKeys, groupKey)
	}
	sort.Strings(groupKeys)

	for _, groupKey := range groupKeys {
		group := groups[groupKey]
		sort.Slice(group, func(i, j int) bool {
			return runtimeClaimCandidateLess(group[i], group[j])
		})
		resolved = append(resolved, resolveRuntimeClaimGroup(group))
	}

	return resolved
}

func normalizeRuntimeClaimCandidate(candidate runtimeClaimCandidate) normalizedRuntimeClaimCandidate {
	runtimeFamily := strings.TrimSpace(candidate.claim.RuntimeFamily)
	canonicalConfigPath := ResolveStableConfigPathFingerprint(strings.TrimSpace(candidate.claim.CanonicalConfigPath))
	variantID := strings.TrimSpace(candidate.claim.VariantID)

	return normalizedRuntimeClaimCandidate{
		runtimeClaimCandidate: candidate,
		runtimeFamily:         runtimeFamily,
		canonicalConfigPath:   canonicalConfigPath,
		variantID:             variantID,
		normalizedPluginID:    normalizeRuntimeClaimPluginID(candidate.plugin),
		normalizedPluginName:  normalizeRuntimeClaimPluginName(candidate.plugin),
		normalizedVariantID:   strings.ToLower(variantID),
	}
}

func normalizeRuntimeClaimPluginID(plugin BotPlugin) string {
	if plugin == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(plugin.GetID()))
}

func normalizeRuntimeClaimPluginName(plugin BotPlugin) string {
	if plugin == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(plugin.GetAssetName()))
}

func runtimeClaimCandidateLess(left, right normalizedRuntimeClaimCandidate) bool {
	if left.claim.Priority != right.claim.Priority {
		return left.claim.Priority > right.claim.Priority
	}
	if left.normalizedPluginID != right.normalizedPluginID {
		return left.normalizedPluginID < right.normalizedPluginID
	}
	if left.normalizedVariantID != right.normalizedVariantID {
		return left.normalizedVariantID < right.normalizedVariantID
	}
	if left.claim.Asset.ID != right.claim.Asset.ID {
		return left.claim.Asset.ID < right.claim.Asset.ID
	}
	if left.normalizedPluginName != right.normalizedPluginName {
		return left.normalizedPluginName < right.normalizedPluginName
	}
	if left.claim.IsFallback != right.claim.IsFallback {
		return left.claim.IsFallback
	}
	if left.runtimeFamily != right.runtimeFamily {
		return left.runtimeFamily < right.runtimeFamily
	}
	if left.canonicalConfigPath != right.canonicalConfigPath {
		return left.canonicalConfigPath < right.canonicalConfigPath
	}
	if left.variantID != right.variantID {
		return left.variantID < right.variantID
	}
	if left.claim.Asset.Name != right.claim.Asset.Name {
		return left.claim.Asset.Name < right.claim.Asset.Name
	}
	return left.claim.Asset.SourcePlugin < right.claim.Asset.SourcePlugin
}

func resolveRuntimeClaimGroup(group []normalizedRuntimeClaimCandidate) resolvedRuntimeAsset {
	winner := group[0]
	fallbackIndex := -1
	for i := range group {
		if group[i].claim.IsFallback {
			fallbackIndex = i
			break
		}
	}

	ambiguous := false
	if len(group) > 1 && group[0].claim.Priority == group[1].claim.Priority {
		if fallbackIndex >= 0 {
			winner = group[fallbackIndex]
		} else {
			ambiguous = true
		}
	}

	asset := winner.claim.Asset
	if fallbackIndex >= 0 {
		asset.ID = group[fallbackIndex].claim.Asset.ID
	}
	asset.Metadata = copyRuntimeClaimMetadata(asset.Metadata)
	asset.Metadata["runtime_family"] = winner.runtimeFamily
	asset.Metadata["runtime_variant"] = winner.variantID
	asset.Metadata["runtime_config_path"] = winner.canonicalConfigPath
	asset.Metadata["detected_variants"] = detectedRuntimeClaimVariants(group)
	if ambiguous {
		asset.Metadata["runtime_claim_ambiguous"] = "true"
	} else {
		delete(asset.Metadata, "runtime_claim_ambiguous")
	}
	asset.SourcePlugin = winner.plugin.GetAssetName()

	return resolvedRuntimeAsset{plugin: winner.plugin, asset: asset}
}

func copyRuntimeClaimMetadata(metadata map[string]string) map[string]string {
	copy := make(map[string]string, len(metadata)+5)
	for key, value := range metadata {
		copy[key] = value
	}
	return copy
}

func detectedRuntimeClaimVariants(group []normalizedRuntimeClaimCandidate) string {
	variants := make([]string, 0, len(group))
	for _, candidate := range group {
		variants = append(variants, candidate.variantID)
	}
	sort.Slice(variants, func(i, j int) bool {
		left := strings.ToLower(variants[i])
		right := strings.ToLower(variants[j])
		if left != right {
			return left < right
		}
		return variants[i] < variants[j]
	})
	return strings.Join(variants, ",")
}
