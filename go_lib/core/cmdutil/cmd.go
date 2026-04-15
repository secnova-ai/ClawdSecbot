package cmdutil

import (
	"os"
	"os/exec"
)

var devNullFile = openDevNull()

func openDevNull() *os.File {
	file, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	return file
}

// Command creates an exec.Cmd configured for background, silent execution.
func Command(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	Prepare(cmd)
	return cmd
}

// Silence redirects standard streams to the null device for detached/background work.
func Silence(cmd *exec.Cmd) {
	if cmd == nil || devNullFile == nil {
		return
	}
	cmd.Stdin = devNullFile
	cmd.Stdout = devNullFile
	cmd.Stderr = devNullFile
}

// BackgroundCommand creates a detached command with stdio redirected to the null device.
func BackgroundCommand(name string, arg ...string) *exec.Cmd {
	cmd := Command(name, arg...)
	Silence(cmd)
	return cmd
}
