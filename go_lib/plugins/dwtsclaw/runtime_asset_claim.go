package dwtsclaw

import (
	"strings"

	"go_lib/core"
)

const dwtsclawRuntimePriority = 90

// BuildRuntimeAssetClaims claims an OpenClaw-compatible config only after the
// scanner found DWTSClaw-specific process or installation evidence.
func (p *Plugin) BuildRuntimeAssetClaims(assets []core.Asset) []core.RuntimeAssetClaim {
	claims := make([]core.RuntimeAssetClaim, 0, len(assets))
	for _, asset := range assets {
		if asset.Metadata == nil {
			continue
		}
		configPath := core.ResolveStableConfigPathFingerprint(strings.TrimSpace(asset.Metadata["config_path"]))
		evidence := sortedStrings(strings.Split(asset.Metadata["product_evidence"], ","))
		if configPath == "" || len(evidence) == 0 {
			continue
		}
		claims = append(claims, core.RuntimeAssetClaim{
			Asset:               asset,
			RuntimeFamily:       "openclaw",
			CanonicalConfigPath: configPath,
			VariantID:           "dwtsclaw",
			Priority:            dwtsclawRuntimePriority,
			Evidence:            evidence,
		})
	}
	return claims
}
