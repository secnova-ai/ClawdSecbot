package openclaw

import (
	"strings"

	"go_lib/core"
)

const openclawRuntimeFamily = "openclaw"

// BuildRuntimeAssetClaims marks native Openclaw as the compatibility fallback
// for an Openclaw-compatible runtime configuration.
func (p *OpenclawPlugin) BuildRuntimeAssetClaims(assets []core.Asset) []core.RuntimeAssetClaim {
	claims := make([]core.RuntimeAssetClaim, 0, len(assets))
	for _, asset := range assets {
		configPath := ""
		if asset.Metadata != nil {
			configPath = strings.TrimSpace(asset.Metadata["config_path"])
		}
		configPath = core.ResolveStableConfigPathFingerprint(configPath)
		if configPath == "" {
			continue
		}

		claims = append(claims, core.RuntimeAssetClaim{
			Asset:               asset,
			RuntimeFamily:       openclawRuntimeFamily,
			CanonicalConfigPath: configPath,
			VariantID:           "openclaw",
			Priority:            10,
			IsFallback:          true,
			Evidence:            []string{"openclaw-compatible-config"},
		})
	}
	return claims
}
