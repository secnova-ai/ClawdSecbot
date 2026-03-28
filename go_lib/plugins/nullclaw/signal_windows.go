//go:build windows

package nullclaw

import (
	"os/signal"
	"syscall"
)

func resetPluginSignals() {
	signal.Reset(syscall.SIGINT, syscall.SIGTERM)
}
