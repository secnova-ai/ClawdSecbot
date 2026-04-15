package sandbox

import (
	"fmt"
	"os"
	"os/exec"

	"go_lib/core/cmdutil"
	"go_lib/core/logging"
)

// isSandboxSupportedOnPlatform checks if macOS sandbox-exec is available
func isSandboxSupportedOnPlatform() bool {
	path, err := exec.LookPath("sandbox-exec")
	if err != nil {
		logging.Warning("sandbox-exec not found: %v", err)
		return false
	}

	info, err := os.Stat(path)
	if err != nil {
		logging.Warning("Cannot stat sandbox-exec: %v", err)
		return false
	}

	if info.Mode()&0111 == 0 {
		logging.Warning("sandbox-exec is not executable")
		return false
	}

	return true
}

// platformConfigure handles macOS-specific sandbox configuration
func (m *SandboxManager) platformConfigure() error {
	// macOS: Seatbelt 策略在 Start 时生成，无需预配置
	return nil
}

// buildSandboxCommand builds the macOS sandbox-exec command
func (m *SandboxManager) buildSandboxCommand() (*exec.Cmd, string, error) {
	// Generate and save Seatbelt policy
	policy := NewSeatbeltPolicy(m.config)
	policyPath, err := policy.SavePolicyFile(m.policyDir)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate policy: %w", err)
	}

	// Verify policy file was created with correct permissions
	if info, err := os.Stat(policyPath); err != nil {
		os.Remove(policyPath)
		return nil, "", fmt.Errorf("policy file not created: %w", err)
	} else if info.Mode().Perm() != 0600 {
		logging.Warning("Policy file has unexpected permissions: %o", info.Mode().Perm())
	}

	// Build sandbox-exec command: sandbox-exec -f xx.sb openclaw gateway start
	args := []string{"-f", policyPath, "openclaw"}
	args = append(args, m.gatewayArgs...)

	cmd := cmdutil.BackgroundCommand("sandbox-exec", args...)
	cmd.Env = append(os.Environ(), m.gatewayEnv...)

	setSysProcAttr(cmd)

	return cmd, policyPath, nil
}

// platformPostStart is a no-op on macOS (sandbox-exec handles everything)
func (m *SandboxManager) platformPostStart(cmd *exec.Cmd) error {
	return nil
}

// GeneratePlatformPolicy generates macOS Seatbelt sandbox policy content
func GeneratePlatformPolicy(config SandboxConfig) (string, error) {
	policy := NewSeatbeltPolicy(config)
	return policy.GeneratePolicy()
}
