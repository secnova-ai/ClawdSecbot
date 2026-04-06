//go:build !linux && !darwin

package dintalclaw

// dintalclawAnyPIDRunsAsRoot 非 Linux/macOS 不做 Unix root 检测（如 Windows）
func dintalclawAnyPIDRunsAsRoot() bool {
	return false
}
