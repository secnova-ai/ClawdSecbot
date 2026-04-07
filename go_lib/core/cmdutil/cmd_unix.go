//go:build !windows

package cmdutil

import (
	"os/exec"
	"syscall"
)

// Prepare detaches child processes from the current terminal/session on Unix.
func Prepare(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
}
