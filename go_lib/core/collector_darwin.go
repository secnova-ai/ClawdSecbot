package core

import (
	"bufio"
	"strconv"
	"strings"

	"go_lib/core/cmdutil"
	"go_lib/core/logging"
)

// getOpenPorts 使用 lsof 获取开放的 TCP 监听端口
func (c *platformCollector) getOpenPorts() ([]int, error) {
	logging.Debug("开始执行 lsof 命令获取开放端口...")
	// lsof -iTCP -sTCP:LISTEN -P -n
	cmd := cmdutil.Command("lsof", "-iTCP", "-sTCP:LISTEN", "-P", "-n")
	output, err := cmd.Output()
	if err != nil {
		logging.Error("执行 lsof 命令失败: %v", err)
		return nil, err
	}
	logging.Debug("lsof 命令执行成功,输出长度: %d 字节", len(output))

	portMap := make(map[int]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	// 跳过标题行
	if scanner.Scan() {
	}

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		// lsof 输出格式: COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE NAME
		// 示例: moltbot 12345 user 12u IPv4 0x... 0t0 TCP 127.0.0.1:18789 (LISTEN)
		if len(fields) >= 9 {
			addrField := fields[8] // 127.0.0.1:18789
			// 提取端口
			parts := strings.Split(addrField, ":")
			if len(parts) >= 2 {
				portStr := parts[len(parts)-1]
				if port, err := strconv.Atoi(portStr); err == nil {
					portMap[port] = true
				}
			}
		}
	}

	var ports []int
	for p := range portMap {
		ports = append(ports, p)
	}
	logging.Debug("解析到 %d 个端口: %v", len(ports), ports)
	return ports, nil
}

// getRunningProcesses 使用 ps 获取运行中的进程
func (c *platformCollector) getRunningProcesses() ([]SystemProcess, error) {
	logging.Debug("开始执行 ps 命令获取运行进程...")
	// ps -eo pid,comm,args
	cmd := cmdutil.Command("ps", "-eo", "pid,comm,args")
	output, err := cmd.Output()
	if err != nil {
		logging.Error("执行 ps 命令失败: %v", err)
		return nil, err
	}
	logging.Debug("ps 命令执行成功,输出长度: %d 字节", len(output))

	var procs []SystemProcess
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	if scanner.Scan() { // 跳过标题
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// 简单的字段分割,注意 args 可能包含空格
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		pid, _ := strconv.Atoi(parts[0])
		comm := parts[1]
		cmdLine := line

		procs = append(procs, SystemProcess{
			Pid:  int32(pid),
			Name: comm,
			Cmd:  cmdLine,
			Path: comm,
		})
	}
	logging.Debug("解析到 %d 个进程", len(procs))
	return procs, nil
}

// getServices 使用 launchctl 获取服务列表
func (c *platformCollector) getServices() ([]string, error) {
	logging.Debug("开始执行 launchctl 命令获取服务列表...")
	// launchctl list
	cmd := cmdutil.Command("launchctl", "list")
	output, err := cmd.Output()
	if err != nil {
		logging.Error("执行 launchctl 命令失败: %v", err)
		return nil, err
	}
	logging.Debug("launchctl 命令执行成功,输出长度: %d 字节", len(output))

	var services []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	if scanner.Scan() { // 跳过标题
	}

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		// launchctl list 输出: PID Status Label
		if len(fields) >= 3 {
			services = append(services, fields[2])
		} else if len(fields) == 2 {
			// 某些可能没有 PID
			services = append(services, fields[1])
		}
	}
	logging.Debug("解析到 %d 个服务", len(services))
	return services, nil
}
