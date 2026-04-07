package core

import (
	"bufio"
	"strconv"
	"strings"

	"go_lib/core/cmdutil"
	"go_lib/core/logging"
)

// getOpenPorts 使用 ss 获取开放的 TCP 监听端口
func (c *platformCollector) getOpenPorts() ([]int, error) {
	logging.Debug("开始执行 ss 命令获取开放端口...")
	// ss -tlnp: TCP listening, numeric, show process
	cmd := cmdutil.Command("ss", "-tlnp")
	output, err := cmd.Output()
	if err != nil {
		logging.Error("执行 ss 命令失败: %v", err)
		return nil, err
	}
	logging.Debug("ss 命令执行成功,输出长度: %d 字节", len(output))

	portMap := make(map[int]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	// 跳过标题行
	if scanner.Scan() {
	}

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		// ss -tlnp 输出格式: State Recv-Q Send-Q Local Address:Port Peer Address:Port Process
		// 示例: LISTEN 0 128 127.0.0.1:8080 0.0.0.0:*
		// 示例: LISTEN 0 128 [::1]:8080 [::]:*
		if len(fields) >= 4 {
			localAddr := fields[3] // 127.0.0.1:8080 或 [::1]:8080
			port := extractPortFromAddr(localAddr)
			if port > 0 {
				portMap[port] = true
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

// extractPortFromAddr 从 ss 输出的地址字段中提取端口号
// 支持 IPv4 (127.0.0.1:8080) 和 IPv6 ([::1]:8080) 格式
func extractPortFromAddr(addr string) int {
	// 查找最后一个冒号的位置作为端口分隔符
	lastColon := strings.LastIndex(addr, ":")
	if lastColon < 0 || lastColon == len(addr)-1 {
		return 0
	}
	portStr := addr[lastColon+1:]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}
	return port
}

// getRunningProcesses 使用 ps 获取运行中的进程
func (c *platformCollector) getRunningProcesses() ([]SystemProcess, error) {
	logging.Debug("开始执行 ps 命令获取运行进程...")
	// ps -eo pid,comm,args (Linux 和 macOS 通用)
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

// getServices 使用 systemctl 获取用户级服务列表
func (c *platformCollector) getServices() ([]string, error) {
	logging.Debug("开始执行 systemctl 命令获取用户级服务列表...")
	// systemctl --user list-units --type=service --all --no-pager --no-legend
	cmd := cmdutil.Command("systemctl", "--user", "list-units", "--type=service", "--all", "--no-pager", "--no-legend")
	output, err := cmd.Output()
	if err != nil {
		logging.Error("执行 systemctl 命令失败: %v", err)
		return nil, err
	}
	logging.Debug("systemctl 命令执行成功,输出长度: %d 字节", len(output))

	var services []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// --no-legend 输出格式: UNIT LOAD ACTIVE SUB DESCRIPTION...
		// 示例: openclaw-gateway.service loaded active running Openclaw Gateway
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			unitName := fields[0]
			// 去掉 .service 后缀以保持与 macOS launchctl 输出风格一致
			unitName = strings.TrimSuffix(unitName, ".service")
			services = append(services, unitName)
		}
	}
	logging.Debug("解析到 %d 个服务", len(services))
	return services, nil
}
