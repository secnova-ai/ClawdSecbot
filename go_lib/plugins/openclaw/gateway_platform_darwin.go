package openclaw

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go_lib/core"
	"go_lib/core/cmdutil"
	"go_lib/core/logging"
	"go_lib/core/sandbox"
)

var (
	installedPlistRegex = regexp.MustCompile(`Installed LaunchAgent:\s*(.+\.plist)`)
)

// restartOpenclawGateway 统一的网关重启逻辑（幂等）。
// 完整流程：stop → install（生成 plist）→ 同步沙箱配置 → reload/start。
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

	// 获取用户主目录（优先使用PathManager）
	var homeDir string
	pm := core.GetPathManager()
	if pm.IsInitialized() {
		homeDir = pm.GetHomeDir()
	} else {
		homeDir, _ = os.UserHomeDir()
	}

	// 1) 基于资产发现/系统环境推导 openclaw 二进制路径 + 配置路径
	binaryPath := resolveOpenclawBinaryPath()
	if binaryPath == "" {
		return nil, fmt.Errorf("openclaw binary not found")
	}
	logging.Info("[GatewayManager] Resolved binary=%s", binaryPath)

	configPath, _ := findConfigPath()
	// configPath 允许为空：有些场景仅需要 install/stop/start

	// 2) stop gateway（忽略错误，保持幂等）
	logging.Info("[GatewayManager] Step 2: Stopping gateway...")
	_, _ = runOpenclawGatewayCommand(binaryPath, []string{"stop"}, homeDir)
	time.Sleep(800 * time.Millisecond)

	// 2.5) unload 已有的 LaunchAgent，防止 install 时 launchctl bootstrap 冲突
	logging.Info("[GatewayManager] Step 2.5: Unloading existing LaunchAgent...")
	unloadExistingOpenclawLaunchAgent(homeDir)

	// 3) install gateway 并解析 plist 路径（幂等：重新生成 LaunchAgent plist）
	logging.Info("[GatewayManager] Step 3: Installing gateway and resolving plist path...")
	plistPath, installOutput, installErr := installGatewayAndGetPlistPath(binaryPath, homeDir)
	if installErr != nil {
		logging.Warning("[GatewayManager] gateway install failed: %v", installErr)
	}

	if plistPath == "" {
		// 无 LaunchAgent，退化为普通 start
		logging.Info("[GatewayManager] No plist found, fallback to direct start")
		_, _ = runOpenclawGatewayCommand(binaryPath, []string{"start"}, homeDir)
		return map[string]interface{}{
			"success":        true,
			"modified":       true,
			"plist":          "",
			"message":        "started without launchagent (fallback)",
			"install_output": installOutput,
		}, nil
	}

	// 4) 根据 sandboxEnabled 同步 plist 中 sandbox-exec 包装
	logging.Info("[GatewayManager] Step 4: Syncing sandbox config, sandboxEnabled=%v, plist=%s", req.SandboxEnabled, plistPath)
	policyDir := req.PolicyDir
	if policyDir == "" {
		policyDir = filepath.Join(homeDir, ".botsec", "policies")
	}
	_ = os.MkdirAll(policyDir, 0755)

	var modified bool
	if req.SandboxEnabled {
		instanceKey := buildGatewayInstanceKey(req.AssetName, req.AssetID)
		policyPath, policyModified, err := writeGatewayPolicyFile(policyDir, instanceKey, sandbox.SandboxConfig{
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

		m, err := injectSandboxIntoPlist(plistPath, policyPath)
		if err != nil {
			return nil, fmt.Errorf("inject sandbox failed: %v", err)
		}
		modified = m

		if modified || policyModified {
			logging.Info("[GatewayManager] Sandbox updated (plist_modified=%v, policy_modified=%v), reloading LaunchAgent...", modified, policyModified)
			if err := reloadLaunchAgent(plistPath, homeDir); err != nil {
				logging.Warning("[GatewayManager] reload launchagent failed: %v", err)
				_, _ = runOpenclawGatewayCommand(binaryPath, []string{"start"}, homeDir)
			}
			time.Sleep(2 * time.Second)
		}

		return map[string]interface{}{
			"success":  true,
			"modified": modified,
			"plist":    plistPath,
			"message":  "gateway synced with sandbox protection",
		}, nil
	}

	// normal mode: remove sandbox-exec wrapper if present
	m, err := removeSandboxFromPlist(plistPath)
	if err != nil {
		logging.Warning("[GatewayManager] remove sandbox from plist failed: %v", err)
	}
	modified = m

	if modified {
		logging.Info("[GatewayManager] Sandbox removed, reloading LaunchAgent...")
		if err := reloadLaunchAgent(plistPath, homeDir); err != nil {
			logging.Warning("[GatewayManager] reload launchagent failed: %v", err)
			_, _ = runOpenclawGatewayCommand(binaryPath, []string{"start"}, homeDir)
		}
		time.Sleep(2 * time.Second)
	}

	return map[string]interface{}{
		"success":  true,
		"modified": modified,
		"plist":    plistPath,
		"message":  "gateway synced without sandbox protection",
	}, nil
}

// restartOpenclawGatewaySimple 简易版网关重启：查找 openclaw 二进制并执行 stop + start
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

	logging.Info("[GatewayManager] restartOpenclawGatewaySimple: stop gateway")
	_, _ = runOpenclawGatewayCommand(binaryPath, []string{"stop"}, homeDir)

	logging.Info("[GatewayManager] restartOpenclawGatewaySimple: start gateway")
	_, err := runOpenclawGatewayCommand(binaryPath, []string{"start"}, homeDir)
	return err
}

// runOpenclawGatewayCommand 通过 shell 执行 openclaw gateway 命令
// macOS 需要 source shell 配置以获取正确的 PATH（openclaw 可能通过 homebrew 等安装）
func runOpenclawGatewayCommand(binaryPath string, args []string, homeDir string) (string, error) {
	if binaryPath == "" {
		return "", fmt.Errorf("binary path is empty")
	}

	cmdArgs := append([]string{"gateway"}, args...)
	fullCmd := binaryPath
	for _, a := range cmdArgs {
		fullCmd += " " + a
	}

	shells := []struct {
		bin  string
		args []string
	}{
		{"/bin/zsh", []string{"-c", "source ~/.zshrc 2>/dev/null; " + fullCmd}},
		{"/bin/zsh", []string{"-l", "-c", fullCmd}},
		{"/bin/bash", []string{"-c", "source ~/.bashrc 2>/dev/null; source ~/.bash_profile 2>/dev/null; " + fullCmd}},
	}

	var lastErr error
	var lastOutput string
	for _, sh := range shells {
		cmd := cmdutil.Command(sh.bin, sh.args...)
		if homeDir != "" {
			cmd.Env = append(os.Environ(), "HOME="+homeDir)
		}
		out, err := cmd.CombinedOutput()
		lastOutput = string(out)
		if err == nil {
			return lastOutput, nil
		}
		lastErr = err
		logging.Info("[GatewayManager] shell %s failed: %v, output: %s", sh.bin, err, strings.TrimSpace(lastOutput))
	}
	return lastOutput, lastErr
}

func installGatewayAndGetPlistPath(binaryPath string, homeDir string) (plistPath string, output string, err error) {
	output, err = runOpenclawGatewayCommand(binaryPath, []string{"install"}, homeDir)
	if output == "" && err != nil {
		return "", output, err
	}

	m := installedPlistRegex.FindStringSubmatch(output)
	if len(m) >= 2 {
		plistPath = strings.TrimSpace(m[1])
		plistPath = expandHome(plistPath, homeDir)
	}
	return plistPath, output, err
}

func writeGatewayPolicyFile(policyDir string, assetName string, cfg sandbox.SandboxConfig) (string, bool, error) {
	if err := os.MkdirAll(policyDir, 0755); err != nil {
		return "", false, err
	}

	fileName := "botsec_" + sanitizeFileName(assetName) + ".sb"
	policyPath := filepath.Join(policyDir, fileName)

	policy := sandbox.NewSeatbeltPolicy(cfg)
	content, err := policy.GeneratePolicy()
	if err != nil {
		return "", false, err
	}

	oldContent, err := os.ReadFile(policyPath)
	if err != nil && !os.IsNotExist(err) {
		return "", false, err
	}

	modified, err := writeIfChanged(policyPath, oldContent, []byte(content))
	if err != nil {
		return "", false, err
	}

	logging.Info("[GatewayManager] Policy file ready: %s (modified=%v)", policyPath, modified)
	return policyPath, modified, nil
}

func injectSandboxIntoPlist(plistPath string, policyPath string) (bool, error) {
	contentBytes, err := os.ReadFile(plistPath)
	if err != nil {
		return false, err
	}
	content := string(contentBytes)

	if strings.Contains(content, "sandbox-exec") {
		re := regexp.MustCompile(`(<string>/usr/bin/sandbox-exec</string>\s*<string>-f</string>\s*<string>)([^<]+)(</string>)`)
		m := re.FindStringSubmatch(content)
		if len(m) == 4 && m[2] == policyPath {
			return false, nil
		}
		newContent := re.ReplaceAllString(content, fmt.Sprintf("$1%s$3", policyPath))
		if newContent == content {
			return false, fmt.Errorf("sandbox-exec exists but policy path not found in plist")
		}
		return writeIfChanged(plistPath, contentBytes, []byte(newContent))
	}

	re := regexp.MustCompile(`(?s)(<key>ProgramArguments</key>\s*<array>\s*)(\s*)(<string>)([^<]+)(</string>)`)
	loc := re.FindStringSubmatchIndex(content)
	if loc == nil {
		return false, fmt.Errorf("ProgramArguments not found in plist")
	}

	sub := re.FindStringSubmatch(content)
	indent := ""
	if len(sub) >= 3 {
		indent = sub[2]
	}

	replace := fmt.Sprintf("${1}%s<string>/usr/bin/sandbox-exec</string>\n%s<string>-f</string>\n%s<string>%s</string>\n%s${3}${4}${5}",
		indent, indent, indent, policyPath, indent,
	)
	replace = strings.ReplaceAll(replace, "${1}", "$1")
	replace = strings.ReplaceAll(replace, "${3}", "$3")
	replace = strings.ReplaceAll(replace, "${4}", "$4")
	replace = strings.ReplaceAll(replace, "${5}", "$5")
	newContent := re.ReplaceAllString(content, replace)
	if newContent == content {
		return false, fmt.Errorf("failed to inject sandbox-exec into plist")
	}

	return writeIfChanged(plistPath, contentBytes, []byte(newContent))
}

func removeSandboxFromPlist(plistPath string) (bool, error) {
	contentBytes, err := os.ReadFile(plistPath)
	if err != nil {
		return false, err
	}
	content := string(contentBytes)

	if !strings.Contains(content, "sandbox-exec") {
		return false, nil
	}

	re := regexp.MustCompile(`(?s)\s*<string>/usr/bin/sandbox-exec</string>\s*<string>-f</string>\s*<string>[^<]+</string>`)
	newContent := re.ReplaceAllString(content, "")
	if newContent == content {
		return false, fmt.Errorf("sandbox-exec exists but removal pattern not matched")
	}

	return writeIfChanged(plistPath, contentBytes, []byte(newContent))
}

func unloadExistingOpenclawLaunchAgent(homeDir string) {
	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	entries, err := os.ReadDir(launchAgentsDir)
	if err != nil {
		logging.Info("[GatewayManager] Cannot read LaunchAgents dir: %v", err)
		return
	}

	env := os.Environ()
	if homeDir != "" {
		env = append(env, "HOME="+homeDir)
	}
	env = append(env, "PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin")

	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if strings.Contains(name, "openclaw") && strings.HasSuffix(name, ".plist") {
			plistPath := filepath.Join(launchAgentsDir, entry.Name())
			logging.Info("[GatewayManager] Unloading existing LaunchAgent: %s", plistPath)
			cmd := cmdutil.Command("/bin/launchctl", "unload", plistPath)
			cmd.Env = env
			if out, err := cmd.CombinedOutput(); err != nil {
				logging.Info("[GatewayManager] launchctl unload (ignored): %v, output: %s", err, strings.TrimSpace(string(out)))
			} else {
				logging.Info("[GatewayManager] launchctl unload success: %s", plistPath)
			}
		}
	}
}

func reloadLaunchAgent(plistPath string, homeDir string) error {
	env := os.Environ()
	if homeDir != "" {
		env = append(env, "HOME="+homeDir)
	}
	env = append(env, "PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin")

	unloadCmd := cmdutil.Command("/bin/launchctl", "unload", plistPath)
	unloadCmd.Env = env
	_ = unloadCmd.Run()

	cmd := cmdutil.Command("/bin/launchctl", "load", plistPath)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl load failed: %v, output: %s", err, strings.TrimSpace(string(out)))
	}
	logging.Info("[GatewayManager] LaunchAgent reloaded: %s", plistPath)
	return nil
}
