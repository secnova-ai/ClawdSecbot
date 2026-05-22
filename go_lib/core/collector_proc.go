package core

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go_lib/core/logging"
)

// readProcNetTCPPorts 通过 Linux /proc 网络表采集监听端口，供无 ss 的容器兜底使用。
func readProcNetTCPPorts() ([]int, error) {
	portMap := make(map[int]bool)
	for _, procPath := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		content, err := os.ReadFile(procPath)
		if err != nil {
			logging.Warning("Failed to read %s: %v", procPath, err)
			continue
		}
		for _, port := range parseProcNetTCPPorts(string(content)) {
			portMap[port] = true
		}
	}

	ports := make([]int, 0, len(portMap))
	for port := range portMap {
		ports = append(ports, port)
	}
	if len(ports) == 0 {
		return []int{}, nil
	}
	logging.Debug("Collected %d listening ports via /proc/net/tcp: %v", len(ports), ports)
	return ports, nil
}

// parseProcNetTCPPorts 解析 /proc/net/tcp 和 /proc/net/tcp6 中的监听端口。
func parseProcNetTCPPorts(content string) []int {
	portMap := make(map[int]bool)
	scanner := bufio.NewScanner(strings.NewReader(content))
	if scanner.Scan() {
	}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || fields[3] != "0A" {
			continue
		}
		parts := strings.Split(fields[1], ":")
		if len(parts) != 2 {
			continue
		}
		port64, err := strconv.ParseInt(parts[1], 16, 32)
		if err != nil || port64 <= 0 {
			continue
		}
		portMap[int(port64)] = true
	}

	ports := make([]int, 0, len(portMap))
	for port := range portMap {
		ports = append(ports, port)
	}
	return ports
}

// readProcRunningProcesses 通过 Linux /proc 采集进程列表，供无 ps 的容器兜底使用。
func readProcRunningProcesses(procRoot string) ([]SystemProcess, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, err
	}

	procs := make([]SystemProcess, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}

		pidDir := filepath.Join(procRoot, entry.Name())
		comm, _ := os.ReadFile(filepath.Join(pidDir, "comm"))
		cmdline, _ := os.ReadFile(filepath.Join(pidDir, "cmdline"))
		exePath, _ := os.Readlink(filepath.Join(pidDir, "exe"))
		procs = append(procs, buildProcSystemProcess(pid, string(comm), cmdline, exePath))
	}

	logging.Debug("Collected %d processes via /proc", len(procs))
	return procs, nil
}

// buildProcSystemProcess 将 /proc 进程文件内容转换为系统进程快照。
func buildProcSystemProcess(pid int, comm string, cmdline []byte, exePath string) SystemProcess {
	name := strings.TrimSpace(comm)
	cmd := strings.TrimSpace(strings.ReplaceAll(string(cmdline), "\x00", " "))
	if cmd == "" {
		cmd = name
	}
	pathValue := strings.TrimSpace(exePath)
	if pathValue == "" {
		pathValue = name
	}
	return SystemProcess{
		Pid:  int32(pid),
		Name: name,
		Cmd:  fmt.Sprintf("%d %s %s", pid, name, cmd),
		Path: pathValue,
	}
}
