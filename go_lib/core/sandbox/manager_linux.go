package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"go_lib/core/logging"
)

// preloadLibSearchPaths 定义 LD_PRELOAD 沙箱库的搜索路径列表
var preloadLibSearchPaths = []string{
	"/usr/lib/clawdsecbot/libsandbox_preload.so",
	"/usr/local/lib/libsandbox_preload.so",
	"/usr/lib/libsandbox_preload.so",
	"/opt/sandbox/lib/libsandbox_preload.so",
}

// isSandboxSupportedOnPlatform checks if LD_PRELOAD sandbox is available on Linux
func isSandboxSupportedOnPlatform() bool {
	for _, p := range preloadLibSearchPaths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// platformConfigure handles Linux-specific sandbox configuration
func (m *SandboxManager) platformConfigure() error {
	// Search for preload library in standard paths and policy directory
	searchPaths := append(preloadLibSearchPaths, filepath.Join(m.policyDir, "libsandbox_preload.so"))
	for _, p := range searchPaths {
		if _, err := os.Stat(p); err == nil {
			m.preloadLib = p
			logging.Info("Found LD_PRELOAD sandbox library: %s", p)
			return nil
		}
	}
	return fmt.Errorf("libsandbox_preload.so not found, searched: %v", searchPaths)
}

// buildSandboxCommand builds the Linux LD_PRELOAD sandboxed command
func (m *SandboxManager) buildSandboxCommand() (*exec.Cmd, string, error) {
	// Generate preload policy JSON
	preloadConfig := buildPreloadConfig(m.config)
	policyJSON, err := preloadConfig.ToPolicyJSON()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate preload policy: %w", err)
	}

	// Ensure policy directory exists
	if err := os.MkdirAll(m.policyDir, 0700); err != nil {
		return nil, "", fmt.Errorf("failed to create policy directory: %w", err)
	}

	// Write policy file
	policyPath := filepath.Join(m.policyDir, fmt.Sprintf("botsec_%s_preload.json", sanitizeAssetName(m.config.AssetName)))
	if err := os.WriteFile(policyPath, policyJSON, 0600); err != nil {
		return nil, "", fmt.Errorf("failed to write preload policy: %w", err)
	}

	logDir := m.logDir
	if logDir == "" {
		logDir = m.policyDir
	}
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, "", fmt.Errorf("failed to create sandbox log directory: %w", err)
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("botsec_%s_hook.log", sanitizeAssetName(m.config.AssetName)))

	// Build: openclaw <gatewayArgs...> with LD_PRELOAD environment variables
	cmd := exec.Command("openclaw", m.gatewayArgs...)
	cmd.Env = append(os.Environ(), m.gatewayEnv...)

	cmd.Env = append(cmd.Env,
		fmt.Sprintf("LD_PRELOAD=%s", m.preloadLib),
		fmt.Sprintf("SANDBOX_POLICY_FILE=%s", policyPath),
		fmt.Sprintf("SANDBOX_LOG_FILE=%s", logPath),
	)

	setSysProcAttr(cmd)

	return cmd, policyPath, nil
}

// platformPostStart is a no-op on Linux (LD_PRELOAD handles everything)
func (m *SandboxManager) platformPostStart(cmd *exec.Cmd) error {
	return nil
}

// GeneratePlatformPolicy generates Linux LD_PRELOAD sandbox policy content
func GeneratePlatformPolicy(config SandboxConfig) (string, error) {
	preloadConfig := buildPreloadConfig(config)
	data, err := preloadConfig.ToPolicyJSON()
	if err != nil {
		return "", err
	}
	return string(data), nil
}
