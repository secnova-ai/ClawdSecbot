package coclaw

import (
	"strings"

	"go_lib/core"
)

// BuildRuntimeAssetClaims gives CoClaw product evidence precedence over the
// generic Openclaw compatibility fallback for the same runtime configuration.
func (p *Plugin) BuildRuntimeAssetClaims(assets []core.Asset) []core.RuntimeAssetClaim {
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
			RuntimeFamily:       "openclaw",
			CanonicalConfigPath: configPath,
			VariantID:           "coclaw",
			Priority:            80,
			Evidence:            []string{"coclaw-config-or-process"},
		})
	}
	return claims
}
