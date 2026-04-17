package nullclaw

import (
	"bufio"
	"fmt"
	"os"
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
	systemdServiceName = "nullclaw.service"
)

var (
	installedUnitRegex = regexp.MustCompile(`(?i)Installed\s+(?:service|unit):\s*(.+\.service)`)
)

// restartNullclawGateway 统一的网关重启逻辑（幂等）。
// 完整流程：stop → install（生成 systemd unit）→ 同步沙箱配置 → daemon-reload → start。
func restartNullclawGateway(req *GatewayRestartRequest) (map[string]interface{}, error) {
	gatewayRestartMu.Lock()
	defer gatewayRestartMu.Unlock()

	logging.Info("[GatewayManager] === restartNullclawGateway called, asset=%s, assetID=%s, sandbox=%v ===",
		req.AssetName, req.AssetID, req.SandboxEnabled)

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

	// 1) 推导 nullclaw 二进制路径
	binaryPath := resolveNullclawBinaryPath()
	if binaryPath == "" {
		return nil, fmt.Errorf("nullclaw binary not found")
	}
	logging.Info("[GatewayManager] Resolved binary=%s", binaryPath)

	configPath, _ := findConfigPath()

	// 2) stop service via systemctl
	logging.Info("[GatewayManager] Step 2: Stopping service via systemctl...")
	_ = runSystemctl("stop", systemdServiceName)
	time.Sleep(800 * time.Millisecond)

	// 3) install service（生成 systemd unit file）
	logging.Info("[GatewayManager] Step 3: Installing service...")
	unitPath, installOutput, installErr := installGatewayAndGetUnitPath(binaryPath, homeDir)
	if installErr != nil {
		logging.Warning("[GatewayManager] service install failed: %v", installErr)
	}

	if unitPath == "" {
		// 无 unit file，退化为直接启动
		logging.Info("[GatewayManager] No unit file found, fallback to direct start")
		_, _ = runNullclawGatewayCommand(binaryPath, []string{"start"}, homeDir)
		return map[string]interface{}{
			"success":        true,
			"modified":       true,
			"unit":           "",
			"message":        "started without systemd unit (fallback)",
			"install_output": installOutput,
		}, nil
	}

	// 4) 根据 sandboxEnabled 同步 systemd unit 中的 LD_PRELOAD 环境变量
	logging.Info("[GatewayManager] Step 4: Syncing sandbox config, sandboxEnabled=%v, unit=%s", req.SandboxEnabled, unitPath)
	policyDir := req.PolicyDir
	if policyDir == "" {
		policyDir = core.ResolvePolicyDir(homeDir)
	}
	_ = os.MkdirAll(policyDir, 0755)
	logDir := core.ResolveSandboxLogDir(homeDir)
	_ = os.MkdirAll(logDir, 0755)

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
			logging.Warning("[GatewayManager] libsandbox_preload.so not found, sandbox injection skipped")
		} else {
			m, err := injectSandboxIntoUnit(unitPath, preloadLib, policyPath, logPath)
			if err != nil {
				return nil, fmt.Errorf("inject sandbox into unit failed: %v", err)
			}
			modified = m
		}

		if modified {
			logging.Info("[GatewayManager] Sandbox injected into unit, reloading systemd...")
			reloadSystemdUnit()
		} else {
			// 即使 unit 文件未修改，也需要确保服务运行（因为 Step 2 已经 stop 了）
			logging.Info("[GatewayManager] Unit unchanged, starting systemd service...")
			_ = runSystemctl("start", systemdServiceName)
		}
		time.Sleep(2 * time.Second)

		sandbox.StartHookLogWatcherByKey(instanceKey, logPath, func(event sandbox.HookLogEvent) {
			eventType, actionDesc, riskType, source := sandbox.MapHookEventToSecurityEvent(event)
			GetSecurityEventBuffer().AddSecurityEvent(SecurityEvent{
				EventType:  eventType,
				ActionDesc: actionDesc,
				RiskType:   riskType,
				Source:     source,
				Detail:     event.Detail,
				AssetName:  req.AssetName,
				AssetID:    req.AssetID,
			})
		})

		return map[string]interface{}{
			"success":          true,
			"modified":         modified,
			"unit":             unitPath,
			"sandbox_log_path": logPath,
			"message":          "gateway synced with sandbox protection",
		}, nil
	}

	// normal mode: remove LD_PRELOAD if present
	instanceKey := buildGatewayInstanceKey(req.AssetName, req.AssetID)
	sandbox.StopHookLogWatcherByKey(instanceKey)
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

// restartNullclawGatewaySimple 简易版网关重启
func restartNullclawGatewaySimple() error {
	binaryPath := resolveNullclawBinaryPath()
	if binaryPath == "" {
		return fmt.Errorf("nullclaw binary not found")
	}

	logging.Info("[GatewayManager] restartNullclawGatewaySimple: restarting service via systemctl")
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
		_, _ = runNullclawGatewayCommand(binaryPath, []string{"stop"}, homeDir)
		_, err := runNullclawGatewayCommand(binaryPath, []string{"start"}, homeDir)
		return err
	}
	return nil
}

// runNullclawGatewayCommand 直接执行 nullclaw service 命令
func runNullclawGatewayCommand(binaryPath string, args []string, homeDir string) (string, error) {
	if binaryPath == "" {
		return "", fmt.Errorf("binary path is empty")
	}

	cmdArgs := append([]string{"service"}, args...)
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

func installGatewayAndGetUnitPath(binaryPath string, homeDir string) (unitPath string, output string, err error) {
	output, err = runNullclawGatewayCommand(binaryPath, []string{"install"}, homeDir)

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

// findSystemdUnitPath 在标准位置查找 nullclaw systemd unit 文件
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

// injectSandboxIntoUnit 向 systemd unit 文件中注入 LD_PRELOAD 环境变量
func injectSandboxIntoUnit(unitPath string, preloadLib string, policyPath string, logPath string) (bool, error) {
	contentBytes, err := os.ReadFile(unitPath)
	if err != nil {
		return false, err
	}
	content := string(contentBytes)

	ldPreloadLine := fmt.Sprintf("Environment=LD_PRELOAD=%s", preloadLib)
	policyLine := fmt.Sprintf("Environment=SANDBOX_POLICY_FILE=%s", policyPath)
	logLine := fmt.Sprintf("Environment=SANDBOX_LOG_FILE=%s", logPath)

	// 检查是否已注入
	hasPreload := strings.Contains(content, "Environment=LD_PRELOAD=")
	hasPolicy := strings.Contains(content, "Environment=SANDBOX_POLICY_FILE=")
	hasLog := strings.Contains(content, "Environment=SANDBOX_LOG_FILE=")

	if hasPreload && hasPolicy && hasLog {
		// 更新已有的环境变量
		lines := strings.Split(content, "\n")
		changed := false
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "Environment=LD_PRELOAD=") && trimmed != ldPreloadLine {
				lines[i] = ldPreloadLine
				changed = true
			}
			if strings.HasPrefix(trimmed, "Environment=SANDBOX_POLICY_FILE=") && trimmed != policyLine {
				lines[i] = policyLine
				changed = true
			}
			if strings.HasPrefix(trimmed, "Environment=SANDBOX_LOG_FILE=") && trimmed != logLine {
				lines[i] = logLine
				changed = true
			}
		}
		if !changed {
			return false, nil
		}
		newContent := strings.Join(lines, "\n")
		return writeIfChanged(unitPath, contentBytes, []byte(newContent))
	}

	// 需要添加新的环境变量行，插入到 [Service] 段中 ExecStart 之前
	var newLines []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	injected := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// 在 ExecStart 行之前插入环境变量
		if !injected && strings.HasPrefix(trimmed, "ExecStart=") {
			if !hasPreload {
				newLines = append(newLines, ldPreloadLine)
			}
			if !hasPolicy {
				newLines = append(newLines, policyLine)
			}
			if !hasLog {
				newLines = append(newLines, logLine)
			}
			injected = true
		}
		newLines = append(newLines, line)
	}

	// 如果未找到 ExecStart，追加到 [Service] 段末尾
	if !injected {
		for i, line := range newLines {
			if strings.TrimSpace(line) == "[Service]" {
				insertIdx := i + 1
				insert := []string{}
				if !hasPreload {
					insert = append(insert, ldPreloadLine)
				}
				if !hasPolicy {
					insert = append(insert, policyLine)
				}
				if !hasLog {
					insert = append(insert, logLine)
				}
				newLines = append(newLines[:insertIdx], append(insert, newLines[insertIdx:]...)...)
				break
			}
		}
	}

	newContent := strings.Join(newLines, "\n")
	return writeIfChanged(unitPath, contentBytes, []byte(newContent))
}

// removeSandboxFromUnit 从 systemd unit 文件中移除 LD_PRELOAD 环境变量
func removeSandboxFromUnit(unitPath string) (bool, error) {
	contentBytes, err := os.ReadFile(unitPath)
	if err != nil {
		return false, err
	}
	content := string(contentBytes)

	if !strings.Contains(content, "Environment=LD_PRELOAD=") &&
		!strings.Contains(content, "Environment=SANDBOX_POLICY_FILE=") &&
		!strings.Contains(content, "Environment=SANDBOX_LOG_FILE=") {
		return false, nil
	}

	var newLines []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Environment=LD_PRELOAD=") ||
			strings.HasPrefix(trimmed, "Environment=SANDBOX_POLICY_FILE=") ||
			strings.HasPrefix(trimmed, "Environment=SANDBOX_LOG_FILE=") {
			continue // 跳过这些行
		}
		newLines = append(newLines, line)
	}

	newContent := strings.Join(newLines, "\n")
	return writeIfChanged(unitPath, contentBytes, []byte(newContent))
}

// reloadSystemdUnit 重新加载 systemd 配置并重启 nullclaw gateway 服务
func reloadSystemdUnit() {
	cmd := cmdutil.Command("systemctl", "--user", "daemon-reload")
	if out, err := cmd.CombinedOutput(); err != nil {
		logging.Warning("[GatewayManager] systemctl daemon-reload failed: %v, output: %s", err, strings.TrimSpace(string(out)))
	}
	_ = runSystemctl("restart", systemdServiceName)
}
