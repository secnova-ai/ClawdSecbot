//go:build linux

package dintalclaw

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go_lib/core/cmdutil"
	"go_lib/core/logging"
	"go_lib/core/sandbox"
)

// findDintalclawPIDs 在 Linux 上通过 /proc 查找 dintalclaw 相关 Python 进程
func findDintalclawPIDs() []int {
	var pids []int

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return pids
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}

		cmdStr := strings.ReplaceAll(string(cmdline), "\x00", " ")
		for _, keyword := range dintalclawProcessKeywords {
			if strings.Contains(cmdStr, keyword) {
				pids = append(pids, pid)
				break
			}
		}
	}

	return pids
}

// getChildPIDs 获取指定 PID 的所有子进程
func getChildPIDs(parentPID int) []int {
	var children []int

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return children
	}

	parentStr := strconv.Itoa(parentPID)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == parentPID {
			continue
		}

		statPath := filepath.Join("/proc", entry.Name(), "stat")
		data, err := os.ReadFile(statPath)
		if err != nil {
			continue
		}

		fields := strings.Fields(string(data))
		if len(fields) >= 4 && fields[3] == parentStr {
			children = append(children, pid)
			grandchildren := getChildPIDs(pid)
			children = append(children, grandchildren...)
		}
	}

	return children
}

// getListenersForPID 从 /proc/net/tcp 和 /proc/net/tcp6 获取指定 PID 的监听端口
func getListenersForPID(pid int) []Listener {
	var listeners []Listener

	inodes := getPIDSocketInodes(pid)
	if len(inodes) == 0 {
		return listeners
	}

	inodeSet := make(map[string]bool)
	for _, inode := range inodes {
		inodeSet[inode] = true
	}

	for _, tcpFile := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		parseTCPListeners(tcpFile, inodeSet, pid, &listeners)
	}

	return listeners
}

// getProcessNameForPID 在 Linux 上获取进程对应的脚本名称
func getProcessNameForPID(pid int) string {
	cmdlinePath := filepath.Join("/proc", strconv.Itoa(pid), "cmdline")
	data, err := os.ReadFile(cmdlinePath)
	if err != nil {
		commData, commErr := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm"))
		if commErr == nil {
			return strings.TrimSpace(string(commData))
		}
		return ""
	}
	cmd := strings.ReplaceAll(string(data), "\x00", " ")
	if strings.Contains(cmd, "launch.pyw") {
		return "launch.pyw"
	}
	if strings.Contains(cmd, "stapp.py") {
		return "stapp.py"
	}
	if strings.Contains(cmd, "agentmain.py") {
		return "agentmain.py"
	}
	exePath, exeErr := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	if exeErr == nil {
		base := filepath.Base(exePath)
		if base != "" {
			return base
		}
	}
	commData, commErr := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm"))
	if commErr == nil {
		return strings.TrimSpace(string(commData))
	}
	return ""
}

// getPIDSocketInodes 获取进程持有的 socket inode 列表
func getPIDSocketInodes(pid int) []string {
	var inodes []string
	fdDir := filepath.Join("/proc", strconv.Itoa(pid), "fd")
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return inodes
	}

	for _, entry := range entries {
		link, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
		if err != nil {
			continue
		}
		if strings.HasPrefix(link, "socket:[") && strings.HasSuffix(link, "]") {
			inode := link[8 : len(link)-1]
			inodes = append(inodes, inode)
		}
	}

	return inodes
}

// parseTCPListeners 解析 /proc/net/tcp[6] 文件，匹配 inode 提取监听信息
func parseTCPListeners(tcpFile string, inodeSet map[string]bool, pid int, listeners *[]Listener) {
	f, err := os.Open(tcpFile)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Scan() // skip header

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}

		// st=0A means LISTEN
		if fields[3] != "0A" {
			continue
		}

		inode := fields[9]
		if !inodeSet[inode] {
			continue
		}

		addr, port, err := parseHexAddr(fields[1])
		if err != nil {
			continue
		}

		*listeners = append(*listeners, Listener{
			Address: addr,
			Port:    port,
			PID:     pid,
		})
	}
}

// parseHexAddr 解析 /proc/net/tcp 中的十六进制地址:端口
func parseHexAddr(hexAddrPort string) (string, int, error) {
	parts := strings.Split(hexAddrPort, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid format: %s", hexAddrPort)
	}

	port, err := strconv.ParseInt(parts[1], 16, 32)
	if err != nil {
		return "", 0, err
	}

	hexAddr := parts[0]
	var addr string

	switch len(hexAddr) {
	case 8:
		bytes, err := hex.DecodeString(hexAddr)
		if err != nil {
			return "", 0, err
		}
		addr = net.IPv4(bytes[3], bytes[2], bytes[1], bytes[0]).String()
	case 32:
		bytes, err := hex.DecodeString(hexAddr)
		if err != nil {
			return "", 0, err
		}
		ip := make(net.IP, 16)
		for i := 0; i < 4; i++ {
			ip[i*4] = bytes[i*4+3]
			ip[i*4+1] = bytes[i*4+2]
			ip[i*4+2] = bytes[i*4+1]
			ip[i*4+3] = bytes[i*4]
		}
		addr = ip.String()
	default:
		addr = "0.0.0.0"
	}

	return addr, int(port), nil
}

// killAllDintalclawProcesses 停止所有 dintalclaw 相关进程（先子后父）
func killAllDintalclawProcesses() {
	mainPIDs := findDintalclawPIDs()
	if len(mainPIDs) == 0 {
		return
	}

	// 收集完整进程树（子进程在前，主进程在后，便于先子后父终止）
	var allPIDs []int
	for _, pid := range mainPIDs {
		children := getChildPIDs(pid)
		allPIDs = append(allPIDs, children...)
	}
	allPIDs = append(allPIDs, mainPIDs...)

	for _, pid := range allPIDs {
		logging.Info("[GatewayManager] Sending SIGINT to PID=%d", pid)
		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Signal(os.Interrupt)
		}
	}

	time.Sleep(2 * time.Second)

	for _, pid := range allPIDs {
		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Kill()
		}
	}
	time.Sleep(500 * time.Millisecond)
}

// restartDintalclawProcess 在 Linux 上重启 dintalclaw 进程
func restartDintalclawProcess(req *GatewayRestartRequest) (map[string]interface{}, error) {
	gatewayRestartMu.Lock()
	defer gatewayRestartMu.Unlock()

	logging.Info("[GatewayManager] === restartDintalclawProcess (Linux) called, asset=%s, sandbox=%v ===",
		req.AssetName, req.SandboxEnabled)

	instanceKey := buildGatewayInstanceKey(req.AssetName, req.AssetID)

	killAllDintalclawProcesses()

	root := findInstallRoot()
	if root == "" {
		return map[string]interface{}{
			"success": true,
			"message": "processes killed but install root not found for restart",
		}, nil
	}

	cmd := buildLaunchCommand(root, req.LaunchMode)
	if cmd == nil {
		return map[string]interface{}{
			"success": true,
			"message": "processes killed but no launch script found",
		}, nil
	}
	cmd.Dir = root

	homeDir, _ := os.UserHomeDir()
	sandboxLogPath := ""

	// sandbox 注入：通过进程环境变量 LD_PRELOAD 实现权限管控
	if req.SandboxEnabled {
		policyDir := req.PolicyDir
		if policyDir == "" {
			policyDir = filepath.Join(homeDir, ".botsec", "policies")
		}
		_ = os.MkdirAll(policyDir, 0755)
		logDir := filepath.Join(homeDir, ".botsec", "logs")
		_ = os.MkdirAll(logDir, 0755)
		sandboxLogPath = filepath.Join(logDir, fmt.Sprintf("botsec_%s_hook.log",
			sandbox.SanitizeAssetNamePublic(instanceKey)))

		configPath, _ := findConfigPathForDintalclaw()
		policyPath, err := writeDintalclawPolicyFile(policyDir, instanceKey, sandbox.SandboxConfig{
			AssetName:         instanceKey,
			GatewayBinaryPath: "python3",
			GatewayConfigPath: configPath,
			PathPermission:    req.PathPermission,
			NetworkPermission: req.NetworkPermission,
			ShellPermission:   req.ShellPermission,
		})
		if err != nil {
			logging.Warning("[GatewayManager] Write sandbox policy failed: %v, starting without sandbox", err)
		} else {
			preloadLib := findDintalclawPreloadLibrary(policyDir)
			if preloadLib == "" {
				logging.Warning("[GatewayManager] libsandbox_preload.so not found, starting without sandbox")
			} else {
				env := os.Environ()
				env = append(env,
					fmt.Sprintf("LD_PRELOAD=%s", preloadLib),
					fmt.Sprintf("SANDBOX_POLICY_FILE=%s", policyPath),
					fmt.Sprintf("SANDBOX_LOG_FILE=%s", sandboxLogPath),
				)
				cmd.Env = env
				logging.Info("[GatewayManager] Sandbox injected: LD_PRELOAD=%s, policy=%s, log=%s",
					preloadLib, policyPath, sandboxLogPath)
			}
		}
	} else {
		sandbox.StopHookLogWatcherByKey(instanceKey)
	}

	stdinR, stdinW, pipeErr := os.Pipe()
	if pipeErr == nil {
		cmd.Stdin = stdinR
	}

	if err := cmd.Start(); err != nil {
		logging.Warning("[GatewayManager] Failed to start process: %v", err)
		if stdinR != nil {
			stdinR.Close()
		}
		if stdinW != nil {
			stdinW.Close()
		}
		return nil, fmt.Errorf("start process failed: %w", err)
	}
	logging.Info("[GatewayManager] Started dintalclaw process PID=%d", cmd.Process.Pid)

	if stdinR != nil {
		stdinR.Close()
	}

	go func() {
		_ = cmd.Wait()
		if stdinW != nil {
			stdinW.Close()
		}
	}()

	time.Sleep(2 * time.Second)

	// 启动 sandbox 日志监控
	if req.SandboxEnabled && sandboxLogPath != "" {
		sandbox.StartHookLogWatcherByKey(instanceKey, sandboxLogPath, func(event sandbox.HookLogEvent) {
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
	}

	result := map[string]interface{}{
		"success": true,
		"message": "dintalclaw process restarted (Linux)",
	}
	if req.SandboxEnabled && sandboxLogPath != "" {
		result["sandbox_log_path"] = sandboxLogPath
	}
	return result, nil
}

// writeDintalclawPolicyFile 生成 LD_PRELOAD 沙箱策略文件
func writeDintalclawPolicyFile(policyDir string, assetName string, cfg sandbox.SandboxConfig) (string, error) {
	if err := os.MkdirAll(policyDir, 0755); err != nil {
		return "", err
	}
	content, err := sandbox.GeneratePlatformPolicy(cfg)
	if err != nil {
		return "", err
	}
	fileName := "botsec_" + sanitizeFileName(assetName) + "_preload.json"
	policyPath := filepath.Join(policyDir, fileName)
	if err := os.WriteFile(policyPath, []byte(content), 0644); err != nil {
		return "", err
	}
	logging.Info("[GatewayManager] Dintalclaw policy file written: %s", policyPath)
	return policyPath, nil
}

// findDintalclawPreloadLibrary 查找 LD_PRELOAD 沙箱库
func findDintalclawPreloadLibrary(policyDir string) string {
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

// buildLaunchCommand 根据 LaunchMode 和安装目录构建启动命令
func buildLaunchCommand(root string, mode LaunchMode) *exec.Cmd {
	launchScript := filepath.Join(root, "launch.pyw")
	stappScript := filepath.Join(root, "stapp.py")
	agentScript := filepath.Join(root, "agentmain.py")

	switch mode {
	case LaunchModeGUI:
		if _, err := os.Stat(launchScript); err == nil {
			return cmdutil.BackgroundCommand("python3", launchScript)
		}
		logging.Warning("[GatewayManager] LaunchMode=gui but launch.pyw not found, falling back to auto")
	case LaunchModeBrowser:
		if _, err := os.Stat(stappScript); err == nil {
			return cmdutil.BackgroundCommand("python3", "-m", "streamlit", "run", stappScript)
		}
		logging.Warning("[GatewayManager] LaunchMode=browser but stapp.py not found, falling back to auto")
	case LaunchModeCLI:
		if _, err := os.Stat(agentScript); err == nil {
			return cmdutil.BackgroundCommand("python3", agentScript)
		}
		logging.Warning("[GatewayManager] LaunchMode=cli but agentmain.py not found, falling back to auto")
	}

	if _, err := os.Stat(launchScript); err == nil {
		return cmdutil.BackgroundCommand("python3", launchScript)
	}
	if _, err := os.Stat(stappScript); err == nil {
		return cmdutil.BackgroundCommand("python3", "-m", "streamlit", "run", stappScript)
	}
	if _, err := os.Stat(agentScript); err == nil {
		return cmdutil.BackgroundCommand("python3", agentScript)
	}

	return nil
}
