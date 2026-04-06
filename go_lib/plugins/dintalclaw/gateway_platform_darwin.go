//go:build darwin

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

// findDintalclawPIDs 在 macOS 上通过 ps 查找 dintalclaw 相关 Python 进程
func findDintalclawPIDs() []int {
	var pids []int

	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		return pids
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		for _, keyword := range dintalclawProcessKeywords {
			if strings.Contains(line, keyword) {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					pid, err := strconv.Atoi(fields[1])
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

	out, err := exec.Command("pgrep", "-P", strconv.Itoa(parentPID)).Output()
	if err != nil {
		return children
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		pid, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
		if err == nil {
			children = append(children, pid)
			grandchildren := getChildPIDs(pid)
			children = append(children, grandchildren...)
		}
	}

	return children
}

// getListenersForPID 通过 lsof 获取指定 PID 的监听端口
func getListenersForPID(pid int) []Listener {
	var listeners []Listener

	out, err := exec.Command("lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-a", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return listeners
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "COMMAND") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}

		nameField := fields[8]
		addr, port := parseLsofAddr(nameField)
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

// getProcessNameForPID 在 macOS 上获取进程对应的脚本名称
func getProcessNameForPID(pid int) string {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return ""
	}
	cmd := strings.TrimSpace(string(out))
	if strings.Contains(cmd, "launch.pyw") {
		return "launch.pyw"
	}
	if strings.Contains(cmd, "stapp.py") {
		return "stapp.py"
	}
	if strings.Contains(cmd, "agentmain.py") {
		return "agentmain.py"
	}
	commOut, commErr := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if commErr == nil {
		comm := strings.TrimSpace(string(commOut))
		if comm != "" {
			return filepath.Base(comm)
		}
	}
	return ""
}

// parseLsofAddr 解析 lsof 输出的地址字段（如 "*:8501" 或 "127.0.0.1:8080"）
func parseLsofAddr(nameField string) (string, int) {
	idx := strings.LastIndex(nameField, ":")
	if idx < 0 {
		return "", 0
	}

	addr := nameField[:idx]
	portStr := nameField[idx+1:]

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0
	}

	if addr == "*" {
		addr = "0.0.0.0"
	}
	if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
		addr = addr[1 : len(addr)-1]
	}

	return addr, port
}

// killAllDintalclawProcesses 停止所有 dintalclaw 相关进程（先子后父）
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

// restartDintalclawProcess 在 macOS 上重启 dintalclaw 进程
func restartDintalclawProcess(req *GatewayRestartRequest) (map[string]interface{}, error) {
	gatewayRestartMu.Lock()
	defer gatewayRestartMu.Unlock()

	logging.Info("[GatewayManager] === restartDintalclawProcess (Darwin) called, asset=%s ===", req.AssetName)

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
		"message": "dintalclaw process restarted (Darwin)",
	}, nil
}

// buildLaunchCommand 根据 LaunchMode 和安装目录构建启动命令
func buildLaunchCommand(root string, mode LaunchMode) *exec.Cmd {
	launchScript := filepath.Join(root, "launch.pyw")
	stappScript := filepath.Join(root, "stapp.py")
	agentScript := filepath.Join(root, "agentmain.py")

	switch mode {
	case LaunchModeGUI:
		if _, err := os.Stat(launchScript); err == nil {
			return exec.Command("python3", launchScript)
		}
		logging.Warning("[GatewayManager] LaunchMode=gui but launch.pyw not found, falling back to auto")
	case LaunchModeBrowser:
		if _, err := os.Stat(stappScript); err == nil {
			return exec.Command("python3", "-m", "streamlit", "run", stappScript)
		}
		logging.Warning("[GatewayManager] LaunchMode=browser but stapp.py not found, falling back to auto")
	case LaunchModeCLI:
		if _, err := os.Stat(agentScript); err == nil {
			return exec.Command("python3", agentScript)
		}
		logging.Warning("[GatewayManager] LaunchMode=cli but agentmain.py not found, falling back to auto")
	}

	if _, err := os.Stat(launchScript); err == nil {
		return exec.Command("python3", launchScript)
	}
	if _, err := os.Stat(stappScript); err == nil {
		return exec.Command("python3", "-m", "streamlit", "run", stappScript)
	}
	if _, err := os.Stat(agentScript); err == nil {
		return exec.Command("python3", agentScript)
	}

	return nil
}
