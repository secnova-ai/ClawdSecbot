//go:build windows

package dintalclaw

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go_lib/core/logging"
)

// findDintalclawPIDs 在 Windows 上通过 wmic/tasklist 查找 dintalclaw 相关 Python 进程
func findDintalclawPIDs() []int {
	var pids []int

	out, err := exec.Command("wmic", "process", "where",
		"Name='python.exe' or Name='python3.exe' or Name='pythonw.exe'",
		"get", "ProcessId,CommandLine", "/format:list").Output()
	if err != nil {
		out, err = exec.Command("tasklist", "/V", "/FO", "CSV").Output()
		if err != nil {
			return pids
		}
		return findPIDsFromTasklist(string(out))
	}

	return findPIDsFromWMIC(string(out))
}

// findPIDsFromWMIC 从 wmic 输出中解析匹配的 PID
func findPIDsFromWMIC(output string) []int {
	var pids []int
	var currentCmd string
	var currentPID int

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if currentPID > 0 && currentCmd != "" {
				for _, keyword := range dintalclawProcessKeywords {
					if strings.Contains(strings.ToLower(currentCmd), strings.ToLower(keyword)) {
						pids = append(pids, currentPID)
						break
					}
				}
			}
			currentCmd = ""
			currentPID = 0
			continue
		}

		if strings.HasPrefix(line, "CommandLine=") {
			currentCmd = strings.TrimPrefix(line, "CommandLine=")
		} else if strings.HasPrefix(line, "ProcessId=") {
			pid, err := strconv.Atoi(strings.TrimPrefix(line, "ProcessId="))
			if err == nil {
				currentPID = pid
			}
		}
	}

	if currentPID > 0 && currentCmd != "" {
		for _, keyword := range dintalclawProcessKeywords {
			if strings.Contains(strings.ToLower(currentCmd), strings.ToLower(keyword)) {
				pids = append(pids, currentPID)
				break
			}
		}
	}

	return pids
}

// findPIDsFromTasklist 从 tasklist 输出中解析匹配的 PID（降级方案）
func findPIDsFromTasklist(output string) []int {
	var pids []int

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "python") {
			continue
		}
		for _, keyword := range dintalclawProcessKeywords {
			if strings.Contains(lower, strings.ToLower(keyword)) {
				fields := strings.Split(line, ",")
				if len(fields) >= 2 {
					pidStr := strings.Trim(fields[1], "\" ")
					pid, err := strconv.Atoi(pidStr)
					if err == nil {
						pids = append(pids, pid)
					}
				}
				break
			}
		}
	}

	return pids
}

// getChildPIDs 获取指定 PID 的所有子进程
func getChildPIDs(parentPID int) []int {
	var children []int

	out, err := exec.Command("wmic", "process", "where",
		fmt.Sprintf("ParentProcessId=%d", parentPID),
		"get", "ProcessId", "/format:list").Output()
	if err != nil {
		return children
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "ProcessId=") {
			pid, err := strconv.Atoi(strings.TrimPrefix(line, "ProcessId="))
			if err == nil && pid != parentPID {
				children = append(children, pid)
				grandchildren := getChildPIDs(pid)
				children = append(children, grandchildren...)
			}
		}
	}

	return children
}

// getListenersForPID 通过 netstat 获取指定 PID 的监听端口
func getListenersForPID(pid int) []Listener {
	var listeners []Listener

	out, err := exec.Command("netstat", "-ano").Output()
	if err != nil {
		return listeners
	}

	pidStr := strconv.Itoa(pid)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, "LISTENING") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		if fields[len(fields)-1] != pidStr {
			continue
		}

		addr, port := parseNetstatAddr(fields[1])
		if port > 0 {
			listeners = append(listeners, Listener{
				Address: addr,
				Port:    port,
				PID:     pid,
			})
		}
	}

	return listeners
}

// getProcessNameForPID 在 Windows 上获取进程对应的脚本名称
func getProcessNameForPID(pid int) string {
	out, err := exec.Command("wmic", "process", "where",
		fmt.Sprintf("ProcessId=%d", pid),
		"get", "Name,CommandLine", "/format:list").Output()
	if err != nil {
		return ""
	}
	var cmdLine string
	var imageName string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "CommandLine=") {
			cmdLine = strings.TrimPrefix(line, "CommandLine=")
		}
		if strings.HasPrefix(line, "Name=") {
			imageName = strings.TrimPrefix(line, "Name=")
		}
	}

	cmd := strings.ToLower(cmdLine)
	if strings.Contains(cmd, "launch.pyw") {
		return "launch.pyw"
	}
	if strings.Contains(cmd, "stapp.py") {
		return "stapp.py"
	}
	if strings.Contains(cmd, "agentmain.py") {
		return "agentmain.py"
	}
	if imageName != "" {
		return imageName
	}
	return ""
}

// parseNetstatAddr 解析 netstat 输出的地址字段（如 "0.0.0.0:8501"）
func parseNetstatAddr(addrField string) (string, int) {
	idx := strings.LastIndex(addrField, ":")
	if idx < 0 {
		return "", 0
	}

	addr := addrField[:idx]
	portStr := addrField[idx+1:]

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0
	}

	return addr, port
}

// killAllDintalclawProcesses 停止所有 dintalclaw 相关进程（先子后父，强制 taskkill）
func killAllDintalclawProcesses() {
	mainPIDs := findDintalclawPIDs()
	if len(mainPIDs) == 0 {
		return
	}

	var allPIDs []int
	for _, pid := range mainPIDs {
		children := getChildPIDs(pid)
		allPIDs = append(allPIDs, children...)
	}
	allPIDs = append(allPIDs, mainPIDs...)

	for _, pid := range allPIDs {
		logging.Info("[GatewayManager] Killing PID=%d via taskkill /F", pid)
		_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F").Run()
	}

	time.Sleep(2 * time.Second)
}

// restartDintalclawProcess 在 Windows 上重启 dintalclaw 进程
func restartDintalclawProcess(req *GatewayRestartRequest) (map[string]interface{}, error) {
	gatewayRestartMu.Lock()
	defer gatewayRestartMu.Unlock()

	logging.Info("[GatewayManager] === restartDintalclawProcess (Windows) called, asset=%s ===", req.AssetName)

	killAllDintalclawProcesses()

	root := findInstallRoot()
	if root == "" {
		return map[string]interface{}{
			"success": true,
			"message": "processes killed but install root not found for restart",
		}, nil
	}

	cmd := buildLaunchCommand(root, req.LaunchMode)

	if cmd != nil {
		cmd.Dir = root

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
	}

	time.Sleep(2 * time.Second)

	return map[string]interface{}{
		"success": true,
		"message": "dintalclaw process restarted (Windows)",
	}, nil
}

// buildLaunchCommand 根据 LaunchMode 和安装目录构建启动命令（Windows 使用 pythonw 启动 GUI 脚本）
func buildLaunchCommand(root string, mode LaunchMode) *exec.Cmd {
	launchScript := filepath.Join(root, "launch.pyw")
	stappScript := filepath.Join(root, "stapp.py")
	agentScript := filepath.Join(root, "agentmain.py")

	switch mode {
	case LaunchModeGUI:
		if _, err := os.Stat(launchScript); err == nil {
			return exec.Command("pythonw", launchScript)
		}
		logging.Warning("[GatewayManager] LaunchMode=gui but launch.pyw not found, falling back to auto")
	case LaunchModeBrowser:
		if _, err := os.Stat(stappScript); err == nil {
			return exec.Command("python", "-m", "streamlit", "run", stappScript)
		}
		logging.Warning("[GatewayManager] LaunchMode=browser but stapp.py not found, falling back to auto")
	case LaunchModeCLI:
		if _, err := os.Stat(agentScript); err == nil {
			return exec.Command("python", agentScript)
		}
		logging.Warning("[GatewayManager] LaunchMode=cli but agentmain.py not found, falling back to auto")
	}

	if _, err := os.Stat(launchScript); err == nil {
		return exec.Command("pythonw", launchScript)
	}
	if _, err := os.Stat(stappScript); err == nil {
		return exec.Command("python", "-m", "streamlit", "run", stappScript)
	}
	if _, err := os.Stat(agentScript); err == nil {
		return exec.Command("python", agentScript)
	}

	return nil
}
