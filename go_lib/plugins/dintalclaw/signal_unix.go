//go:build !windows

package dintalclaw

import (
	"os/signal"
	"syscall"
)

func resetPluginSignals() {
	signal.Reset(syscall.SIGINT, syscall.SIGTERM, syscall.SIGTSTP)
}
