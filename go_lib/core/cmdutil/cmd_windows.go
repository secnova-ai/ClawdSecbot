//go:build windows

package cmdutil

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// Prepare hides console windows for child processes on Windows.
func Prepare(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
