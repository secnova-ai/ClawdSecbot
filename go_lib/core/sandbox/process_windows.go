//go:build windows

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"go_lib/core/cmdutil"
	"go_lib/core/logging"
)

// KillProcess kills a specific process by PID on Windows
func KillProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	// Windows does not support SIGTERM; use taskkill for graceful stop first
	cmd := cmdutil.Command("taskkill", "/PID", strconv.Itoa(pid))
	if err := cmd.Run(); err != nil {
		logging.Warning("taskkill graceful failed for PID %d: %v, forcing kill", pid, err)
		return process.Kill()
	}

	time.Sleep(2 * time.Second)

	if IsProcessRunning(pid) {
		return process.Kill()
	}

	return nil
}

// IsProcessRunning checks if a process is still running on Windows
func IsProcessRunning(pid int) bool {
	cmd := cmdutil.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), strconv.Itoa(pid))
}

// FindProcessesByName finds all processes matching a name/path pattern on Windows
func FindProcessesByName(pattern string) ([]int, error) {
	// Use tasklist and filter by image name or full command line via WMIC
	cmd := cmdutil.Command("wmic", "process", "where",
		fmt.Sprintf("CommandLine like '%%%s%%'", pattern),
		"get", "ProcessId", "/format:list")
	output, err := cmd.Output()
	if err != nil {
		// Fallback to tasklist if WMIC fails
		return findProcessesByTasklist(pattern)
	}

	var pids []int
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ProcessId=") {
			pidStr := strings.TrimPrefix(line, "ProcessId=")
			pidStr = strings.TrimSpace(pidStr)
			pid, err := strconv.Atoi(pidStr)
			if err != nil {
				continue
			}
			pids = append(pids, pid)
		}
	}

	return pids, nil
}

// findProcessesByTasklist is a fallback for finding processes
func findProcessesByTasklist(pattern string) ([]int, error) {
	cmd := cmdutil.Command("tasklist", "/FO", "CSV", "/NH")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var pids []int
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lowerLine := strings.ToLower(line)
		if !strings.Contains(lowerLine, strings.ToLower(pattern)) {
			continue
		}
		// CSV: "name","PID",...
		fields := strings.Split(line, ",")
		if len(fields) < 2 {
			continue
		}
		pidStr := strings.Trim(fields[1], "\" ")
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		pids = append(pids, pid)
	}

	return pids, nil
}

// GetProcessInfo returns information about a process on Windows
func GetProcessInfo(pid int) (string, error) {
	cmd := cmdutil.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/V", "/FO", "LIST")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// gracefulTerminate sends a termination signal on Windows (uses taskkill)
func gracefulTerminate(process *os.Process) error {
	cmd := cmdutil.Command("taskkill", "/PID", strconv.Itoa(process.Pid))
	return cmd.Run()
}

// setSysProcAttr keeps child processes hidden on Windows.
func setSysProcAttr(cmd *exec.Cmd) {
	cmdutil.Prepare(cmd)
}
