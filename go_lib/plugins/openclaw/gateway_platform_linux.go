package openclaw

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go_lib/core"
	"go_lib/core/cmdutil"
	"go_lib/core/logging"
	"go_lib/core/sandbox"
)

const (
	systemdServiceName         = "openclaw-gateway.service"
	containerBotsecWebdPath    = "/tmp/ClawdSecbot/bin/botsec_webd"
	containerPreloadLibPath    = "/tmp/ClawdSecbot/lib/libsandbox_preload.so"
	openclawGatewayRunAllowArg = "--allow-unconfigured"
)

var (
	installedUnitRegex = regexp.MustCompile(`(?i)Installed\s+(?:service|unit):\s*(.+\.service)`)
)

// restartOpenclawGateway 统一的网关重启逻辑（幂等）。
// 完整流程：stop → install（生成 systemd unit）→ 同步沙箱配置 → daemon-reload → start。
func restartOpenclawGateway(req *GatewayRestartRequest) (map[string]interface{}, error) {
	gatewayRestartMu.Lock()
	defer gatewayRestartMu.Unlock()

	logging.Info("[GatewayManager] === restartOpenclawGateway called, asset=%s, assetID=%s, sandbox=%v ===",
		req.AssetName, req.AssetID, req.SandboxEnabled)

	cleanupGatewayManagedRuntimeState(req.AssetName, req.AssetID)

	if IsAppStoreBuild() {
		return map[string]interface{}{
			"success": true, "skipped": true, "message": "skipped: app store build",
		}, nil
	}

	var homeDir string
	pm := core.GetPathManager()
	if pm.IsInitialized() {
		homeDir = pm.GetHomeDir()
	} else {
		homeDir, _ = os.UserHomeDir()
	}
	if isContainerWebUIEnvironment() {
		if runtimeHomeDir := currentUserHomeDir(); runtimeHomeDir != "" {
			homeDir = runtimeHomeDir
		}
	}

	// 1) 推导 openclaw 二进制路径
	binaryPath := resolveOpenclawBinaryPath()
	if binaryPath == "" {
		return nil, fmt.Errorf("openclaw binary not found")
	}
	logging.Info("[GatewayManager] Resolved binary=%s", binaryPath)

	configPath, _ := findConfigPath()
	gatewayPort := resolveGatewayListenPort(configPath)

	policyDir := req.PolicyDir
	if policyDir == "" {
		policyDir = core.ResolvePolicyDir(homeDir)
	}
	_ = os.MkdirAll(policyDir, 0755)
	logDir := core.ResolveSandboxLogDir(homeDir)
	_ = os.MkdirAll(logDir, 0755)

	// 容器 WebUI / 无可用 systemd 时永远走直启 gateway 路径,
	// 避免调用 `openclaw gateway install` 在卷挂载只读时把配置写到 /tmp/.openclaw,
	// 也避免 systemctl 不可用引起的级联错误。
	if shouldUseDirectSandboxGatewayRestart(homeDir) {
		if req.SandboxEnabled {
			logging.Info("[GatewayManager] Using direct gateway restart with sandbox env (container or no systemd)")
			return startOpenclawGatewayDirectWithSandbox(req, binaryPath, configPath, homeDir, policyDir, logDir)
		}
		logging.Info("[GatewayManager] Using direct gateway restart without sandbox env (container or no systemd)")
		return startOpenclawGatewayDirectNoSandbox(req, binaryPath, configPath, homeDir, logDir)
	}

	// 2) stop gateway via systemctl
	logging.Info("[GatewayManager] Step 2: Stopping gateway via systemctl...")
	if req.SandboxEnabled {
		stopExistingGatewayListeners(gatewayPort)
	}
	_ = runSystemctl("stop", systemdServiceName)
	time.Sleep(800 * time.Millisecond)

	// 3) install gateway（生成 systemd unit file）
	logging.Info("[GatewayManager] Step 3: Installing gateway...")
	unitPath, installOutput, installErr := installGatewayAndGetUnitPath(binaryPath, homeDir)
	if installErr != nil {
		logging.Warning("[GatewayManager] gateway install failed: %v", installErr)
	}

	if unitPath == "" {
		if req.SandboxEnabled {
			logging.Info("[GatewayManager] No unit file found, fallback to direct start with sandbox env")
			startResult, startErr := startOpenclawGatewayDirectWithSandbox(req, binaryPath, configPath, homeDir, policyDir, logDir)
			if startErr != nil {
				return nil, startErr
			}
			startResult["install_output"] = installOutput
			return startResult, nil
		}
		logging.Info("[GatewayManager] No unit file found, fallback to direct start (sandbox disabled)")
		_, _ = runOpenclawGatewayCommand(binaryPath, []string{"start"}, homeDir)
		return map[string]interface{}{
			"success":        true,
			"modified":       true,
			"unit":           "",
			"message":        "started without systemd unit (sandbox disabled fallback)",
			"install_output": installOutput,
		}, nil
	}

	// 4) 根据 sandboxEnabled 同步 systemd unit 中的 LD_PRELOAD 环境变量
	logging.Info("[GatewayManager] Step 4: Syncing sandbox config, sandboxEnabled=%v, unit=%s", req.SandboxEnabled, unitPath)

	var modified bool
	if req.SandboxEnabled {
		instanceKey := buildGatewayInstanceKey(req.AssetName, req.AssetID)
		logPath := filepath.Join(logDir, fmt.Sprintf("botsec_%s_hook.log", sandbox.SanitizeAssetNamePublic(instanceKey)))
		policyPath, err := writeGatewayPolicyFile(policyDir, instanceKey, sandbox.SandboxConfig{
			AssetName:         instanceKey,
			GatewayBinaryPath: binaryPath,
			GatewayConfigPath: configPath,
			PathPermission:    req.PathPermission,
			NetworkPermission: req.NetworkPermission,
			ShellPermission:   req.ShellPermission,
		})
		if err != nil {
			return nil, fmt.Errorf("write policy failed: %v", err)
		}

		// 查找 preload 库路径
		preloadLib := findPreloadLibrary(policyDir)
		if preloadLib == "" {
			return nil, fmt.Errorf("sandbox enabled but libsandbox_preload.so not found")
		}

		m, err := injectSandboxIntoUnit(unitPath, preloadLib, policyPath, logPath)
		if err != nil {
			return nil, fmt.Errorf("inject sandbox into unit failed: %v", err)
		}
		modified = m

		if modified {
			logging.Info("[GatewayManager] Sandbox injected into unit, reloading systemd...")
			reloadSystemdUnit()
		} else {
			// 即使 unit 文件未修改，也需要确保服务运行（因为 Step 2 已经 stop 了）
			logging.Info("[GatewayManager] Unit unchanged, starting systemd service...")
			_ = runSystemctl("start", systemdServiceName)
		}
		time.Sleep(2 * time.Second)

		// Sandbox hook audit log is no longer harvested into SecurityEvents here.
		// Proxy decision sink is the sole authoritative source (see _rules/security_event.md).

		return map[string]interface{}{
			"success":          true,
			"modified":         modified,
			"unit":             unitPath,
			"sandbox_log_path": logPath,
			"message":          "gateway synced with sandbox protection",
		}, nil
	}

	// normal mode: remove LD_PRELOAD if present
	m, err := removeSandboxFromUnit(unitPath)
	if err != nil {
		logging.Warning("[GatewayManager] remove sandbox from unit failed: %v", err)
	}
	modified = m

	if modified {
		logging.Info("[GatewayManager] Sandbox removed from unit, reloading systemd...")
		reloadSystemdUnit()
	} else {
		// 即使 unit 文件未修改，也需要确保服务运行（因为 Step 2 已经 stop 了）
		logging.Info("[GatewayManager] Unit unchanged, starting systemd service...")
		_ = runSystemctl("start", systemdServiceName)
	}
	time.Sleep(2 * time.Second)

	return map[string]interface{}{
		"success":  true,
		"modified": modified,
		"unit":     unitPath,
		"message":  "gateway synced without sandbox protection",
	}, nil
}

// restartOpenclawGatewaySimple 简易版网关重启
func restartOpenclawGatewaySimple() error {
	binaryPath := resolveOpenclawBinaryPath()
	if binaryPath == "" {
		return fmt.Errorf("openclaw binary not found")
	}

	logging.Info("[GatewayManager] restartOpenclawGatewaySimple: restarting via systemctl")
	if err := runSystemctl("restart", systemdServiceName); err != nil {
		// 降级为直接命令
		logging.Warning("[GatewayManager] systemctl restart failed: %v, fallback to direct command", err)
		var homeDir string
		pm := core.GetPathManager()
		if pm.IsInitialized() {
			homeDir = pm.GetHomeDir()
		} else {
			homeDir, _ = os.UserHomeDir()
		}
		_, _ = runOpenclawGatewayCommand(binaryPath, []string{"stop"}, homeDir)
		_, err := runOpenclawGatewayCommand(binaryPath, []string{"start"}, homeDir)
		return err
	}
	return nil
}

// runOpenclawGatewayCommand 直接执行 openclaw gateway 命令
func runOpenclawGatewayCommand(binaryPath string, args []string, homeDir string) (string, error) {
	if binaryPath == "" {
		return "", fmt.Errorf("binary path is empty")
	}

	cmdArgs := append([]string{"gateway"}, args...)
	cmd := cmdutil.Command(binaryPath, cmdArgs...)
	if homeDir != "" {
		cmd.Env = append(os.Environ(), "HOME="+homeDir)
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), nil
	}

	// 降级：通过 bash 执行
	fullCmd := binaryPath
	for _, a := range cmdArgs {
		fullCmd += " " + a
	}
	bashCmd := cmdutil.Command("/bin/bash", "-l", "-c", fullCmd)
	if homeDir != "" {
		bashCmd.Env = append(os.Environ(), "HOME="+homeDir)
	}
	bashOut, bashErr := bashCmd.CombinedOutput()
	if bashErr == nil {
		return string(bashOut), nil
	}

	return string(out), err
}

// startOpenclawGatewayDirectWithSandbox 在无 systemd 的容器内通过环境变量直启沙箱网关。
func startOpenclawGatewayDirectWithSandbox(req *GatewayRestartRequest, binaryPath string, configPath string, homeDir string, policyDir string, logDir string) (map[string]interface{}, error) {
	if err := ensureContainerRuntimeWritableDirs(); err != nil {
		return nil, fmt.Errorf("ensure container writable dirs failed: %v", err)
	}

	instanceKey := buildGatewayInstanceKey(req.AssetName, req.AssetID)
	logPath := filepath.Join(logDir, fmt.Sprintf("botsec_%s_hook.log", sandbox.SanitizeAssetNamePublic(instanceKey)))
	policyPath, err := writeGatewayPolicyFile(policyDir, instanceKey, sandbox.SandboxConfig{
		AssetName:         instanceKey,
		GatewayBinaryPath: binaryPath,
		GatewayConfigPath: configPath,
		PathPermission:    req.PathPermission,
		NetworkPermission: req.NetworkPermission,
		ShellPermission:   req.ShellPermission,
	})
	if err != nil {
		return nil, fmt.Errorf("write policy failed: %v", err)
	}

	preloadLib := findPreloadLibrary(policyDir)
	if preloadLib == "" {
		return nil, fmt.Errorf("sandbox enabled but libsandbox_preload.so not found")
	}

	gatewayPort := resolveGatewayListenPort(configPath)
	stopExistingGatewayListeners(gatewayPort)
	waitGatewayPortReleased(gatewayPort, 3*time.Second)
	cleanupOpenclawGatewayLockFiles(homeDir, configPath)

	sandboxEnv := []string{
		"LD_PRELOAD=" + preloadLib,
		"SANDBOX_POLICY_FILE=" + policyPath,
		"SANDBOX_LOG_FILE=" + logPath,
	}
	managedPID, err := launchDetachedOpenclawGatewayWithEnv(binaryPath, homeDir, sandboxEnv)
	if err != nil {
		return nil, fmt.Errorf("direct sandbox gateway start failed: %v", err)
	}
	logging.Info("[GatewayManager] Detached sandbox gateway started pid=%d (gateway run)", managedPID)

	if err := waitForGatewayListenerHasPreload(gatewayPort, preloadLib, managedPID, 15*time.Second); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"success":          true,
		"modified":         true,
		"unit":             "",
		"managed_pid":      managedPID,
		"sandbox_log_path": logPath,
		"message":          "gateway started directly with sandbox protection (gateway run)",
	}, nil
}

// startOpenclawGatewayDirectNoSandbox 在容器 / 无 systemd 场景下,
// 不带任何 LD_PRELOAD 沙箱环境变量直启 openclaw gateway run。
// 用于关闭防护后恢复默认运行,严禁调用 `openclaw gateway install`,
// 防止其在卷挂载只读时把新配置回退到 /tmp/.openclaw 造成路径漂移。
func startOpenclawGatewayDirectNoSandbox(req *GatewayRestartRequest, binaryPath string, configPath string, homeDir string, logDir string) (map[string]interface{}, error) {
	_ = req
	if err := ensureContainerRuntimeWritableDirs(); err != nil {
		return nil, fmt.Errorf("ensure container writable dirs failed: %v", err)
	}

	gatewayPort := resolveGatewayListenPort(configPath)
	stopExistingGatewayListeners(gatewayPort)
	waitGatewayPortReleased(gatewayPort, 3*time.Second)
	cleanupOpenclawGatewayLockFiles(homeDir, configPath)

	managedPID, err := launchDetachedOpenclawGatewayWithEnv(binaryPath, homeDir, nil)
	if err != nil {
		return nil, fmt.Errorf("direct gateway start failed: %v", err)
	}
	logging.Info("[GatewayManager] Detached gateway started pid=%d (gateway run, no sandbox)", managedPID)

	if err := waitForGatewayListener(gatewayPort, managedPID, 15*time.Second); err != nil {
		return nil, err
	}

	runLogPath := openclawGatewayRunLogPath(homeDir, logDir)
	return map[string]interface{}{
		"success":     true,
		"modified":    true,
		"unit":        "",
		"managed_pid": managedPID,
		"run_log":     runLogPath,
		"message":     "gateway started directly without sandbox (gateway run)",
	}, nil
}

// shouldUseDirectSandboxGatewayRestart 判断是否在容器 WebUI 或无可用 systemd user 单元时走直启沙箱路径。
func shouldUseDirectSandboxGatewayRestart(homeDir string) bool {
	if _, err := os.Stat(containerBotsecWebdPath); err == nil {
		return true
	}
	return !isSystemdUserGatewayAvailable(homeDir)
}

// isSystemdUserGatewayAvailable 检查用户级 systemd 是否已加载 openclaw-gateway 单元。
func isSystemdUserGatewayAvailable(homeDir string) bool {
	if findSystemdUnitPath(homeDir) == "" {
		return false
	}
	cmd := cmdutil.Command("systemctl", "--user", "show", "-p", "LoadState", systemdServiceName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "LoadState=loaded")
}

// launchDetachedOpenclawGatewayWithEnv 后台启动 openclaw gateway run, 与 botsec_webd 脱钩。
// 容器 WebUI 场景也保持使用 openclaw gateway run, 避免重启路径和日志路径漂移。
func launchDetachedOpenclawGatewayWithEnv(binaryPath string, homeDir string, extraEnv []string) (int, error) {
	if binaryPath == "" {
		return 0, fmt.Errorf("binary path is empty")
	}

	env := buildGatewayEnv(homeDir, extraEnv)

	logging.Info("[GatewayManager] Using openclaw CLI command: %s gateway run %s", binaryPath, openclawGatewayRunAllowArg)
	cmd := cmdutil.Command(binaryPath, "gateway", "run", openclawGatewayRunAllowArg)
	cmd.Stdin = nil

	runLogPath := openclawGatewayRunLogPath(homeDir, "")
	attachGatewayRunLog(cmd, runLogPath)

	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	go func() {
		_ = cmd.Wait()
	}()
	return cmd.Process.Pid, nil
}

// buildGatewayEnv 构建用于 gateway 进程的环境变量列表。
// 容器环境下 HOME 使用当前运行用户的家目录, 最后追加 extraEnv (如 LD_PRELOAD 等沙箱变量)。
func buildGatewayEnv(homeDir string, extraEnv []string) []string {
	baseEnv := os.Environ()
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return append(baseEnv, extraEnv...)
	}

	env := make([]string, 0, len(baseEnv)+len(extraEnv)+1)
	for _, e := range baseEnv {
		if strings.HasPrefix(e, "HOME=") {
			continue
		}
		env = append(env, e)
	}
	env = append(env, "HOME="+homeDir)
	env = append(env, extraEnv...)
	return env
}

// attachGatewayRunLog 将 gateway 进程的 stdout/stderr 重定向到日志文件。
func attachGatewayRunLog(cmd *exec.Cmd, runLogPath string) {
	if runLogPath == "" {
		cmdutil.Silence(cmd)
		return
	}
	if err := os.MkdirAll(filepath.Dir(runLogPath), 0755); err != nil {
		logging.Warning("[GatewayManager] mkdir gateway run log dir failed: %v", err)
		cmdutil.Silence(cmd)
		return
	}
	logFile, err := os.OpenFile(runLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		logging.Warning("[GatewayManager] open gateway run log failed: %v", err)
		cmdutil.Silence(cmd)
		return
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
}

// isContainerWebUIEnvironment 检测是否在 ClawdSecbot 容器 WebUI 环境。
func isContainerWebUIEnvironment() bool {
	_, err := os.Stat(containerBotsecWebdPath)
	return err == nil
}

// currentUserHomeDir 返回当前运行用户的家目录, 优先使用系统用户数据库而非 HOME 环境变量。
func currentUserHomeDir() string {
	if usr, err := user.Current(); err == nil && usr != nil {
		if homeDir := strings.TrimSpace(usr.HomeDir); homeDir != "" {
			return homeDir
		}
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		return strings.TrimSpace(homeDir)
	}
	return ""
}

// openclawGatewayRunLogPath 推导 openclaw gateway run 的 stdout/stderr 日志文件路径。
func openclawGatewayRunLogPath(homeDir string, logDir string) string {
	dir := strings.TrimSpace(logDir)
	if dir == "" {
		dir = core.ResolveSandboxLogDir(homeDir)
	}
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	return filepath.Join(dir, "openclaw_gateway_run.log")
}

// waitGatewayPortReleased 等待网关端口真正释放, 避免 SIGKILL 后 300ms 还未来得及回收 socket。
func waitGatewayPortReleased(port int, timeout time.Duration) {
	if port <= 0 || timeout <= 0 {
		return
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(findListenPIDsOnTCPPort(port)) == 0 {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	logging.Warning("[GatewayManager] Port %d still has listeners after %s", port, timeout.String())
}

// cleanupOpenclawGatewayLockFiles 清理 openclaw 残留的 pid / lock 文件,
// 防止 `openclaw gateway run` 检测到旧实例文件直接拒绝启动。
func cleanupOpenclawGatewayLockFiles(homeDir string, configPath string) {
	candidates := make([]string, 0, 4)
	if cfgDir := strings.TrimSpace(filepath.Dir(configPath)); cfgDir != "" && cfgDir != "." {
		candidates = append(candidates,
			filepath.Join(cfgDir, ".gateway.pid"),
			filepath.Join(cfgDir, "gateway.lock"),
			filepath.Join(cfgDir, "gateway.pid"),
		)
	}
	if hd := strings.TrimSpace(homeDir); hd != "" {
		candidates = append(candidates,
			filepath.Join(hd, ".openclaw", ".gateway.pid"),
			filepath.Join(hd, ".openclaw", "gateway.lock"),
			filepath.Join(hd, ".openclaw", "gateway.pid"),
		)
	}
	for _, path := range candidates {
		if path == "" {
			continue
		}
		if err := os.Remove(path); err == nil {
			logging.Info("[GatewayManager] Removed stale gateway lockfile: %s", path)
		}
	}
}

// waitForGatewayListener 轮询等待网关监听端口就绪, 不校验 LD_PRELOAD。
func waitForGatewayListener(port int, managedPID int, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(findListenPIDsOnTCPPort(port)) > 0 {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	processState := "unknown"
	if managedPID > 0 {
		processState = "exited"
		if _, err := os.Stat(filepath.Join("/proc", fmt.Sprintf("%d", managedPID))); err == nil {
			processState = "alive"
		}
	}
	return fmt.Errorf(
		"no openclaw gateway listener on port %d after %s (managed_pid=%d,%s); see openclaw_gateway_run.log",
		port, timeout.String(), managedPID, processState,
	)
}

// waitForGatewayListenerHasPreload 轮询等待网关监听就绪并携带预期 LD_PRELOAD, 避免固定 sleep 误判。
func waitForGatewayListenerHasPreload(port int, preloadLib string, managedPID int, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := verifyGatewayListenerHasPreload(port, preloadLib); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}

	listeners := findListenPIDsOnTCPPort(port)
	processState := "unknown"
	if managedPID > 0 {
		processState = "exited"
		if _, err := os.Stat(filepath.Join("/proc", fmt.Sprintf("%d", managedPID))); err == nil {
			processState = "alive"
		}
	}
	return fmt.Errorf(
		"gateway listener verify timeout after %s (managed_pid=%d,%s,listener_pids=%v): %w",
		timeout.String(), managedPID, processState, listeners, lastErr,
	)
}

// resolveGatewayListenPort 从 Openclaw 配置解析网关监听端口，失败时回退默认 18789。
func resolveGatewayListenPort(configPath string) int {
	const defaultPort = 18789
	if strings.TrimSpace(configPath) == "" {
		return defaultPort
	}
	config, _, err := loadConfig(configPath)
	if err != nil || config == nil || config.Gateway.Port <= 0 {
		return defaultPort
	}
	return config.Gateway.Port
}

// ensureContainerRuntimeWritableDirs 创建容器代理运行所需的可写目录。
func ensureContainerRuntimeWritableDirs() error {
	for _, dir := range []string{"/tmp/botsec_web_workspace", "/tmp/.botsec"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func installGatewayAndGetUnitPath(binaryPath string, homeDir string) (unitPath string, output string, err error) {
	output, err = runOpenclawGatewayCommand(binaryPath, []string{"install"}, homeDir)
	if output == "" && err != nil {
		return "", output, err
	}

	// 尝试从 install 输出中解析 unit 文件路径
	m := installedUnitRegex.FindStringSubmatch(output)
	if len(m) >= 2 {
		unitPath = strings.TrimSpace(m[1])
		unitPath = expandHome(unitPath, homeDir)
	}

	// 如果解析失败，尝试标准路径
	if unitPath == "" {
		unitPath = findSystemdUnitPath(homeDir)
	}

	return unitPath, output, err
}

// writeGatewayPolicyFile 生成 LD_PRELOAD 沙箱策略文件
func writeGatewayPolicyFile(policyDir string, assetName string, cfg sandbox.SandboxConfig) (string, error) {
	if err := os.MkdirAll(policyDir, 0755); err != nil {
		return "", err
	}

	// 生成 LD_PRELOAD 策略 JSON
	content, err := sandbox.GeneratePlatformPolicy(cfg)
	if err != nil {
		return "", err
	}

	fileName := "botsec_" + sanitizeFileName(assetName) + "_preload.json"
	policyPath := filepath.Join(policyDir, fileName)

	if err := os.WriteFile(policyPath, []byte(content), 0644); err != nil {
		return "", err
	}

	logging.Info("[GatewayManager] Policy file written: %s", policyPath)
	return policyPath, nil
}

// === Linux systemd 辅助函数 ===

// runSystemctl 执行 systemctl --user 命令
func runSystemctl(action string, service string) error {
	cmd := cmdutil.Command("systemctl", "--user", action, service)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logging.Warning("[GatewayManager] systemctl --user %s %s failed: %v, output: %s",
			action, service, err, strings.TrimSpace(string(out)))
		return err
	}
	logging.Info("[GatewayManager] systemctl --user %s %s success", action, service)
	return nil
}

// findSystemdUnitPath 在标准位置查找 openclaw systemd unit 文件
func findSystemdUnitPath(homeDir string) string {
	searchDirs := []string{
		filepath.Join(homeDir, ".config", "systemd", "user"),
		"/etc/systemd/user",
		"/usr/lib/systemd/user",
	}
	for _, dir := range searchDirs {
		unitPath := filepath.Join(dir, systemdServiceName)
		if _, err := os.Stat(unitPath); err == nil {
			return unitPath
		}
	}
	return ""
}

// findPreloadLibrary 查找 LD_PRELOAD 沙箱库
func findPreloadLibrary(policyDir string) string {
	paths := []string{
		containerPreloadLibPath,
		"/usr/lib/clawdsecbot/libsandbox_preload.so",
		"/usr/local/lib/libsandbox_preload.so",
		"/usr/lib/libsandbox_preload.so",
		"/opt/sandbox/lib/libsandbox_preload.so",
		filepath.Join(policyDir, "libsandbox_preload.so"),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// leadingWhitespace 返回行首连续的空格与制表符, 用于替换 unit 行时保留缩进, 避免破坏 systemd 解析
func leadingWhitespace(line string) string {
	trimmedLeft := strings.TrimLeft(line, " \t")
	return line[:len(line)-len(trimmedLeft)]
}

// injectSandboxIntoUnit 向 systemd unit 文件中注入 LD_PRELOAD/SANDBOX_POLICY_FILE 环境变量
func injectSandboxIntoUnit(unitPath string, preloadLib string, policyPath string, logPath string) (bool, error) {
	contentBytes, err := os.ReadFile(unitPath)
	if err != nil {
		return false, err
	}
	content := string(contentBytes)

	ldPreloadLine := fmt.Sprintf("Environment=LD_PRELOAD=%s", preloadLib)
	policyLine := fmt.Sprintf("Environment=SANDBOX_POLICY_FILE=%s", policyPath)
	logLine := fmt.Sprintf("Environment=SANDBOX_LOG_FILE=%s", logPath)

	envPrefixes := []struct {
		prefix  string
		desired string
	}{
		{"Environment=LD_PRELOAD=", ldPreloadLine},
		{"Environment=SANDBOX_POLICY_FILE=", policyLine},
		{"Environment=SANDBOX_LOG_FILE=", logLine},
	}

	// 检查已有的环境变量行
	allPresent := true
	for _, ep := range envPrefixes {
		if !strings.Contains(content, ep.prefix) {
			allPresent = false
			break
		}
	}

	if allPresent {
		lines := strings.Split(content, "\n")
		changed := false
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			for _, ep := range envPrefixes {
				if strings.HasPrefix(trimmed, ep.prefix) && trimmed != ep.desired {
					lines[i] = leadingWhitespace(line) + ep.desired
					changed = true
				}
			}
		}
		if !changed {
			return false, nil
		}
		newContent := strings.Join(lines, "\n")
		return writeIfChanged(unitPath, contentBytes, []byte(newContent))
	}

	// 需要添加缺失的环境变量行，插入到 [Service] 段中 ExecStart 之前
	var newLines []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	injected := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if !injected && strings.HasPrefix(trimmed, "ExecStart=") {
			for _, ep := range envPrefixes {
				if !strings.Contains(content, ep.prefix) {
					newLines = append(newLines, ep.desired)
				}
			}
			injected = true
		}
		newLines = append(newLines, line)
	}

	if !injected {
		for i, line := range newLines {
			if strings.TrimSpace(line) == "[Service]" {
				insertIdx := i + 1
				var insert []string
				for _, ep := range envPrefixes {
					if !strings.Contains(content, ep.prefix) {
						insert = append(insert, ep.desired)
					}
				}
				newLines = append(newLines[:insertIdx], append(insert, newLines[insertIdx:]...)...)
				break
			}
		}
	}

	newContent := strings.Join(newLines, "\n")
	return writeIfChanged(unitPath, contentBytes, []byte(newContent))
}

// removeSandboxFromUnit 从 systemd unit 文件中移除沙箱相关环境变量
func removeSandboxFromUnit(unitPath string) (bool, error) {
	contentBytes, err := os.ReadFile(unitPath)
	if err != nil {
		return false, err
	}
	content := string(contentBytes)

	sandboxPrefixes := []string{
		"Environment=LD_PRELOAD=",
		"Environment=SANDBOX_POLICY_FILE=",
		"Environment=SANDBOX_LOG_FILE=",
	}

	hasAny := false
	for _, prefix := range sandboxPrefixes {
		if strings.Contains(content, prefix) {
			hasAny = true
			break
		}
	}
	if !hasAny {
		return false, nil
	}

	var newLines []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		skip := false
		for _, prefix := range sandboxPrefixes {
			if strings.HasPrefix(trimmed, prefix) {
				skip = true
				break
			}
		}
		if !skip {
			newLines = append(newLines, line)
		}
	}

	newContent := strings.Join(newLines, "\n")
	return writeIfChanged(unitPath, contentBytes, []byte(newContent))
}

// reloadSystemdUnit 重新加载 systemd 配置并重启 openclaw gateway 服务
func reloadSystemdUnit() {
	cmd := cmdutil.Command("systemctl", "--user", "daemon-reload")
	if out, err := cmd.CombinedOutput(); err != nil {
		logging.Warning("[GatewayManager] systemctl daemon-reload failed: %v, output: %s", err, strings.TrimSpace(string(out)))
	}
	_ = runSystemctl("restart", systemdServiceName)
}
