//go:build !windows

package qclaw

// readQClawExecutableVersion returns empty on non-Windows platforms.
func readQClawExecutableVersion(exePath string) string {
	return ""
}
