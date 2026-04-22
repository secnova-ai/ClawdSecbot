package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"go_lib/core/cmdutil"
	"go_lib/core/logging"
)

// hookDLLPath caches the resolved path to sandbox_hook.dll
var hookDLLPath string

// isSandboxSupportedOnPlatform checks if sandbox_hook.dll is available on Windows
func isSandboxSupportedOnPlatform() bool {
	homeDir, _ := os.UserHomeDir()
	policyDir := filepath.Join(homeDir, ".botsec", "policies")
	p := findHookDLL(policyDir)
	if p != "" {
		logging.Info("[Sandbox] Found hook DLL: %s", p)
		return true
	}
	logging.Info("[Sandbox] sandbox_hook.dll not found, sandbox unavailable")
	return false
}

// platformConfigure handles Windows-specific sandbox configuration
func (m *SandboxManager) platformConfigure() error {
	p := findHookDLL(m.policyDir)
	if p == "" {
		return fmt.Errorf("sandbox_hook.dll not found, searched policy dir: %s", m.policyDir)
	}
	hookDLLPath = p
	logging.Info("[Sandbox] Configured hook DLL: %s", hookDLLPath)
	return nil
}

// buildSandboxCommand builds the Windows sandboxed command with MinHook DLL injection.
// The process is created suspended so the hook DLL can be injected before execution.
func (m *SandboxManager) buildSandboxCommand() (*exec.Cmd, string, error) {
	if hookDLLPath == "" {
		return nil, "", fmt.Errorf("hook DLL path not configured, call Configure first")
	}

	// Generate policy JSON (reuse the same format as Linux preload)
	preloadConfig := buildHookConfig(m.config)
	policyJSON, err := preloadConfig.ToPolicyJSON()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate hook policy: %w", err)
	}

	if err := os.MkdirAll(m.policyDir, 0700); err != nil {
		return nil, "", fmt.Errorf("failed to create policy directory: %w", err)
	}

	policyPath := filepath.Join(m.policyDir, fmt.Sprintf("botsec_%s_hook.json", sanitizeAssetName(m.config.AssetName)))
	if err := os.WriteFile(policyPath, policyJSON, 0600); err != nil {
		return nil, "", fmt.Errorf("failed to write hook policy: %w", err)
	}

	logDir := m.logDir
	if logDir == "" {
		// Backward-compatible fallback when caller does not provide an explicit log dir.
		logDir = m.policyDir
	}
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, "", fmt.Errorf("failed to create log directory: %w", err)
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("botsec_%s_hook.log", sanitizeAssetName(m.config.AssetName)))

	bin := "openclaw"
	if m.config.GatewayBinaryPath != "" {
		bin = m.config.GatewayBinaryPath
	}
	cmd := cmdutil.BackgroundCommand(bin, m.gatewayArgs...)
	cmd.Env = append(os.Environ(), m.gatewayEnv...)
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("SANDBOX_POLICY_FILE=%s", policyPath),
		fmt.Sprintf("SANDBOX_LOG_FILE=%s", logPath),
		fmt.Sprintf("SANDBOX_HOOK_DLL=%s", hookDLLPath),
	)
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createSuspended

	return cmd, policyPath, nil
}

// platformPostStart injects the hook DLL into the suspended process and resumes it
func (m *SandboxManager) platformPostStart(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return fmt.Errorf("process not started")
	}

	pid := cmd.Process.Pid
	logging.Info("[Sandbox] Injecting hook DLL into gateway process PID %d", pid)

	if err := injectDLL(pid, hookDLLPath); err != nil {
		logging.Error("[Sandbox] DLL injection failed for PID %d: %v", pid, err)
		_ = cmd.Process.Kill()
		return fmt.Errorf("DLL injection failed: %w", err)
	}

	if err := resumeProcessThreads(pid); err != nil {
		logging.Error("[Sandbox] Failed to resume process PID %d: %v", pid, err)
		_ = cmd.Process.Kill()
		return fmt.Errorf("process resume failed: %w", err)
	}

	logging.Info("[Sandbox] Gateway process PID %d running with hook protection", pid)
	return nil
}

// GeneratePlatformPolicy generates Windows hook sandbox policy content
func GeneratePlatformPolicy(config SandboxConfig) (string, error) {
	hookConfig := buildHookConfig(config)
	data, err := hookConfig.ToPolicyJSON()
	if err != nil {
		return "", err
	}
	return string(data), nil
}
