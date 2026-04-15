//go:build !windows

package sandbox

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go_lib/core/cmdutil"
)

// KillProcess kills a specific process by PID
func KillProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return process.Kill()
	}

	time.Sleep(2 * time.Second)

	if IsProcessRunning(pid) {
		return process.Kill()
	}

	return nil
}

// IsProcessRunning checks if a process is still running
func IsProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// FindProcessesByName finds all processes matching a name/path pattern
func FindProcessesByName(pattern string) ([]int, error) {
	cmd := cmdutil.Command("pgrep", "-f", pattern)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}

	var pids []int
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		pids = append(pids, pid)
	}

	return pids, nil
}

// GetProcessInfo returns information about a process
func GetProcessInfo(pid int) (string, error) {
	cmd := cmdutil.Command("ps", "-p", strconv.Itoa(pid), "-o", "pid,ppid,comm,args")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// gracefulTerminate sends SIGTERM to a process for graceful shutdown
func gracefulTerminate(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}

// setSysProcAttr sets Unix-specific process attributes (process group)
func setSysProcAttr(cmd *exec.Cmd) {
	cmdutil.Prepare(cmd)
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}
