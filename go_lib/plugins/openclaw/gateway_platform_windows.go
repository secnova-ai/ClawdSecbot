package openclaw

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go_lib/core"
	"go_lib/core/cmdutil"
	"go_lib/core/logging"
	"go_lib/core/sandbox"
)

// restartOpenclawGateway handles gateway restart on Windows.
// When sandbox_hook.dll is available, the gateway runs under MinHook-based sandbox protection.
// Falls back to direct process start/stop when sandbox is unavailable.
func restartOpenclawGateway(req *GatewayRestartRequest) (map[string]interface{}, error) {
	gatewayRestartMu.Lock()
	defer gatewayRestartMu.Unlock()

	logging.Info("[GatewayManager] === restartOpenclawGateway (Windows) called, asset=%s, assetID=%s, sandbox=%v ===",
		req.AssetName, req.AssetID, req.SandboxEnabled)

	for _, key := range buildGatewayRuntimeStateKeys(req.AssetName, req.AssetID) {
		sandbox.StopHookLogWatcherByKey(key)
	}
	cleanupGatewayManagedRuntimeState(req.AssetName, req.AssetID)

	var homeDir string
	pm := core.GetPathManager()
	if pm.IsInitialized() {
		homeDir = pm.GetHomeDir()
	} else {
		homeDir, _ = os.UserHomeDir()
	}

	binaryPath := resolveOpenclawBinaryPath()
	if binaryPath == "" {
		return nil, fmt.Errorf("openclaw binary not found")
	}
	logging.Info("[GatewayManager] Resolved binary=%s", binaryPath)

	// Stop existing gateway
	logging.Info("[GatewayManager] Step 1: Stopping gateway...")
	_, _ = runOpenclawGatewayCommand(binaryPath, []string{"stop"}, homeDir)
	time.Sleep(800 * time.Millisecond)

	// Try sandbox-protected restart if requested and supported
	if req.SandboxEnabled && sandbox.IsSandboxSupported() {
		logging.Info("[GatewayManager] Sandbox available, attempting hook protection")
		result, err := restartWithSandbox(req, binaryPath, homeDir)
		if err == nil {
			return result, nil
		}
		logging.Warning("[GatewayManager] Sandbox start failed: %v", err)
		logging.Warning("[GatewayManager] Sandbox fallback to direct mode")
	} else if req.SandboxEnabled {
		logging.Warning("[GatewayManager] Sandbox requested but sandbox_hook.dll not found")
		logging.Warning("[GatewayManager] Sandbox fallback to direct mode")
	}

	// Fallback: direct start without sandbox
	logging.Info("[GatewayManager] Step 2: Starting gateway (direct mode)...")
	_, err := runOpenclawGatewayCommand(binaryPath, []string{"start"}, homeDir)
	if err != nil {
		logging.Warning("[GatewayManager] gateway start failed: %v", err)
		return nil, fmt.Errorf("gateway start failed: %w", err)
	}

	time.Sleep(2 * time.Second)

	return map[string]interface{}{
		"success":         true,
		"modified":        true,
		"sandbox_enabled": false,
		"message":         "gateway restarted (Windows direct mode)",
	}, nil
}

// restartWithSandbox starts the gateway under MinHook sandbox protection
func restartWithSandbox(req *GatewayRestartRequest, binaryPath, homeDir string) (map[string]interface{}, error) {
	policyDir := req.PolicyDir
	if policyDir == "" {
		policyDir = core.ResolvePolicyDir(homeDir)
	}

	logDir := policyDir
	pm := core.GetPathManager()
	if pm.IsInitialized() && pm.GetLogDir() != "" {
		logDir = pm.GetLogDir()
	}

	instanceKey := buildGatewayInstanceKey(req.AssetName, req.AssetID)
	mgr := sandbox.NewSandboxManagerWithLogDir(instanceKey, policyDir, logDir)

	sandboxConfig := sandbox.SandboxConfig{
		AssetName:         instanceKey,
		GatewayBinaryPath: binaryPath,
		PathPermission:    req.PathPermission,
		NetworkPermission: req.NetworkPermission,
		ShellPermission:   req.ShellPermission,
	}

	// For sandbox mode we must inject into the long-lived real gateway process.
	// If binaryPath is a .cmd wrapper, parse and launch the underlying node command directly.
	launchBinary, gatewayArgs := resolveSandboxGatewayLaunch(binaryPath)
	sandboxConfig.GatewayBinaryPath = launchBinary
	logging.Info("[GatewayManager] Sandbox launch target: binary=%s args=%v", launchBinary, gatewayArgs)
	gatewayEnv := []string{}
	if homeDir != "" {
		gatewayEnv = append(gatewayEnv, "USERPROFILE="+homeDir)
	}

	if err := mgr.Configure(sandboxConfig, gatewayArgs, gatewayEnv); err != nil {
		return nil, fmt.Errorf("sandbox configure failed: %w", err)
	}

	if err := mgr.Start(); err != nil {
		return nil, fmt.Errorf("sandbox start failed: %w", err)
	}

	// Start hook log watcher to feed enforcement events into the security event pipeline
	logPath := filepath.Join(logDir, fmt.Sprintf("botsec_%s_hook.log", sandbox.SanitizeAssetNamePublic(instanceKey)))
	sandbox.StartHookLogWatcherByKey(instanceKey, logPath, func(event sandbox.HookLogEvent) {
		eventType, actionDesc, riskType, source := sandbox.MapHookEventToSecurityEvent(event)
		GetSecurityEventBuffer().AddSecurityEvent(SecurityEvent{
			BotID:      req.AssetID,
			EventType:  eventType,
			ActionDesc: actionDesc,
			RiskType:   riskType,
			Source:     source,
			Detail:     event.Detail,
			AssetName:  req.AssetName,
			AssetID:    req.AssetID,
		})
	})
	logging.Info("[GatewayManager] Sandbox started: mode=windows_hook, managed_pid=%d, hook_log=%s, policy_dir=%s",
		mgr.GetManagedPID(), logPath, policyDir)

	time.Sleep(2 * time.Second)

	return map[string]interface{}{
		"success":          true,
		"modified":         true,
		"sandbox_enabled":  true,
		"managed_pid":      mgr.GetManagedPID(),
		"sandbox_mode":     "windows_hook",
		"sandbox_log_path": logPath,
		"message":          "gateway restarted (Windows sandbox mode)",
	}, nil
}

// resolveSandboxGatewayLaunch resolves the executable/args used for sandbox mode.
// Priority:
// 1) Parse gateway.cmd and execute node + dist/index.js gateway ... directly
// 2) Fallback to foreground gateway run through the provided binary path
func resolveSandboxGatewayLaunch(binaryPath string) (string, []string) {
	if binaryPath == "" {
		return "openclaw", []string{"gateway", "run"}
	}

	ext := strings.ToLower(filepath.Ext(binaryPath))
	if ext == ".cmd" || ext == ".bat" {
		if parsedBinary, parsedArgs, ok := parseGatewayWrapperCommand(binaryPath); ok {
			return parsedBinary, parsedArgs
		}
	}

	return binaryPath, []string{"gateway", "run"}
}

func parseGatewayWrapperCommand(cmdPath string) (string, []string, bool) {
	f, err := os.Open(cmdPath)
	if err != nil {
		return "", nil, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(strings.ToLower(line), "rem ") || strings.HasPrefix(line, "@") {
			continue
		}

		// Expected line example:
		// "C:\Program Files\nodejs\node.exe" C:\...\dist\index.js gateway --port 18789
		bin, args, ok := parseQuotedCommandLine(line)
		if !ok || len(args) < 2 {
			continue
		}

		lowerBin := strings.ToLower(bin)
		if !strings.HasSuffix(lowerBin, "node.exe") {
			continue
		}
		if strings.ToLower(filepath.Base(args[0])) != "index.js" {
			continue
		}
		if strings.ToLower(args[1]) != "gateway" {
			continue
		}

		return bin, args, true
	}

	return "", nil, false
}

func parseQuotedCommandLine(line string) (string, []string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil, false
	}

	if strings.HasPrefix(line, "\"") {
		end := strings.Index(line[1:], "\"")
		if end < 0 {
			return "", nil, false
		}
		bin := line[1 : 1+end]
		rest := strings.TrimSpace(line[2+end:])
		args := []string{}
		if rest != "" {
			args = strings.Fields(rest)
		}
		return bin, args, true
	}

	parts := strings.Fields(line)
	if len(parts) == 0 {
		return "", nil, false
	}
	return parts[0], parts[1:], true
}

// restartOpenclawGatewaySimple performs a basic stop + start
func restartOpenclawGatewaySimple() error {
	binaryPath := resolveOpenclawBinaryPath()
	if binaryPath == "" {
		return fmt.Errorf("openclaw binary not found")
	}

	var homeDir string
	pm := core.GetPathManager()
	if pm.IsInitialized() {
		homeDir = pm.GetHomeDir()
	} else {
		homeDir, _ = os.UserHomeDir()
	}

	logging.Info("[GatewayManager] restartOpenclawGatewaySimple (Windows): stop gateway")
	_, _ = runOpenclawGatewayCommand(binaryPath, []string{"stop"}, homeDir)

	logging.Info("[GatewayManager] restartOpenclawGatewaySimple (Windows): start gateway")
	_, err := runOpenclawGatewayCommand(binaryPath, []string{"start"}, homeDir)
	return err
}

// runOpenclawGatewayCommand executes openclaw gateway command on Windows
func runOpenclawGatewayCommand(binaryPath string, args []string, homeDir string) (string, error) {
	if binaryPath == "" {
		return "", fmt.Errorf("binary path is empty")
	}

	cmdArgs := append([]string{"gateway"}, args...)
	cmd := cmdutil.Command(binaryPath, cmdArgs...)
	if homeDir != "" {
		cmd.Env = append(os.Environ(), "USERPROFILE="+homeDir)
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), nil
	}

	// Fallback: try via cmd.exe
	fullCmd := binaryPath
	for _, a := range cmdArgs {
		fullCmd += " " + a
	}
	cmdExe := cmdutil.Command("cmd.exe", "/C", fullCmd)
	if homeDir != "" {
		cmdExe.Env = append(os.Environ(), "USERPROFILE="+homeDir)
	}
	cmdOut, cmdErr := cmdExe.CombinedOutput()
	if cmdErr == nil {
		return string(cmdOut), nil
	}

	logging.Warning("[GatewayManager] cmd.exe fallback failed: %v, output: %s", cmdErr, strings.TrimSpace(string(cmdOut)))
	return string(out), err
}
