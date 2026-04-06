//go:build !cgo

package dintalclaw

// MitigateRiskDispatch non-cgo fallback
func MitigateRiskDispatch(riskInfo string) string {
	return `{"success": false, "error": "risk mitigation requires cgo-enabled build"}`
}
