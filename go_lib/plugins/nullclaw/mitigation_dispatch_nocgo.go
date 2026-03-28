//go:build !cgo

package nullclaw

// MitigateRiskDispatch provides a non-cgo fallback so cross-platform builds
// with CGO disabled can still compile the plugin package.
func MitigateRiskDispatch(riskInfo string) string {
	return `{"success": false, "error": "risk mitigation requires cgo-enabled build"}`
}
