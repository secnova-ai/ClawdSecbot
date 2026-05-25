package openclaw

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go_lib/core/logging"
)

// openclawGatewayCmdKeywords 用于识别 Openclaw 系网关进程，避免误杀同端口其它服务。
var openclawGatewayCmdKeywords = []string{
	"openclaw",
	"moltbot",
	"clawdbot",
	"openclaw.mjs",
}

// parseProcNetTCPListenPort 从 /proc/net/tcp local_address 字段解析监听端口(十进制)。
func parseProcNetTCPListenPort(addrPortField string) (int, bool) {
	parts := strings.Split(addrPortField, ":")
	if len(parts) != 2 {
		return 0, false
	}
	port, err := strconv.ParseInt(parts[1], 16, 32)
	if err != nil || port <= 0 || port > 65535 {
		return 0, false
	}
	return int(port), true
}

// collectListenInodesOnTCPPort 收集在指定 TCP 端口上处于 LISTEN 状态的 socket inode。
func collectListenInodesOnTCPPort(port int, inodeSet map[string]struct{}) {
	for _, procPath := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		collectListenInodesFromProcNetFile(procPath, port, inodeSet)
	}
}

func collectListenInodesFromProcNetFile(procNetTCP string, port int, inodeSet map[string]struct{}) {
	content, err := os.ReadFile(procNetTCP)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 || fields[3] != "0A" {
			continue
		}
		listenPort, ok := parseProcNetTCPListenPort(fields[1])
		if !ok || listenPort != port {
			continue
		}
		inodeSet[fields[9]] = struct{}{}
	}
}

// findListenPIDsOnTCPPort 通过 inode 反查监听指定 TCP 端口的进程 PID。
func findListenPIDsOnTCPPort(port int) []int {
	if port <= 0 {
		return nil
	}
	inodeSet := make(map[string]struct{})
	collectListenInodesOnTCPPort(port, inodeSet)
	if len(inodeSet) == 0 {
		return nil
	}

	pidSet := make(map[int]struct{})
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		if processHasSocketInodes(pid, inodeSet) {
			pidSet[pid] = struct{}{}
		}
	}

	pids := make([]int, 0, len(pidSet))
	for pid := range pidSet {
		pids = append(pids, pid)
	}
	return pids
}

// findOpenclawGatewayListenPIDs 返回监听指定端口且 cmdline 符合 Openclaw 网关特征的 PID。
func findOpenclawGatewayListenPIDs(port int) []int {
	allPIDs := findListenPIDsOnTCPPort(port)
	if len(allPIDs) == 0 {
		return nil
	}
	filtered := make([]int, 0, len(allPIDs))
	for _, pid := range allPIDs {
		if isOpenclawGatewayProcess(pid) {
			filtered = append(filtered, pid)
		}
	}
	if len(filtered) == 0 && len(allPIDs) > 0 {
		logging.Warning(
			"[GatewayManager] Port %d has %d listener(s) but none match openclaw gateway cmdline; skip SIGTERM to avoid killing unrelated processes",
			port,
			len(allPIDs),
		)
	}
	return filtered
}

// matchOpenclawGatewayCmdline 判断 cmdline 是否属于 Openclaw 网关进程。
func matchOpenclawGatewayCmdline(cmdline []byte) bool {
	cmd := strings.ToLower(strings.ReplaceAll(string(cmdline), "\x00", " "))
	if !strings.Contains(cmd, "gateway") {
		return false
	}
	for _, keyword := range openclawGatewayCmdKeywords {
		if strings.Contains(cmd, keyword) {
			return true
		}
	}
	return false
}

// isOpenclawGatewayProcess 根据 /proc/<pid>/cmdline 判断是否为 Openclaw 网关进程。
func isOpenclawGatewayProcess(pid int) bool {
	cmdlinePath := filepath.Join("/proc", strconv.Itoa(pid), "cmdline")
	data, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return false
	}
	return matchOpenclawGatewayCmdline(data)
}

func processHasSocketInodes(pid int, inodeSet map[string]struct{}) bool {
	fdDir := filepath.Join("/proc", strconv.Itoa(pid), "fd")
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		link, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
		if err != nil || !strings.HasPrefix(link, "socket:[") || !strings.HasSuffix(link, "]") {
			continue
		}
		inode := link[len("socket:[") : len(link)-1]
		if _, ok := inodeSet[inode]; ok {
			return true
		}
	}
	return false
}

// stopExistingGatewayListeners 停止占用网关端口的 Openclaw 进程；SIGTERM 后若仍监听则 SIGKILL。
func stopExistingGatewayListeners(port int) {
	if port <= 0 {
		return
	}
	signalGatewayListenersOnPort(port, syscall.SIGTERM)
	time.Sleep(800 * time.Millisecond)
	remaining := findOpenclawGatewayListenPIDs(port)
	if len(remaining) == 0 {
		return
	}
	logging.Warning("[GatewayManager] %d openclaw gateway listener(s) still on port %d after SIGTERM, sending SIGKILL", len(remaining), port)
	signalGatewayListenersOnPort(port, syscall.SIGKILL)
	time.Sleep(300 * time.Millisecond)
}

func signalGatewayListenersOnPort(port int, sig syscall.Signal) {
	for _, pid := range findOpenclawGatewayListenPIDs(port) {
		logging.Info("[GatewayManager] Signaling gateway listener pid=%d on port=%d signal=%v", pid, port, sig)
		if err := syscall.Kill(pid, sig); err != nil {
			logging.Warning("[GatewayManager] Failed to signal pid=%d on port=%d: %v", pid, port, err)
		}
	}
}

// verifyGatewayListenerHasPreload 检查监听端口的 Openclaw 进程是否携带 LD_PRELOAD。
func verifyGatewayListenerHasPreload(port int, preloadLib string) {
	pids := findOpenclawGatewayListenPIDs(port)
	if len(pids) == 0 {
		logging.Warning("[GatewayManager] No openclaw gateway listener on port %d to verify LD_PRELOAD", port)
		return
	}
	preloadBase := filepath.Base(strings.TrimSpace(preloadLib))
	for _, pid := range pids {
		envPath := filepath.Join("/proc", strconv.Itoa(pid), "environ")
		envData, err := os.ReadFile(envPath)
		if err != nil {
			logging.Warning("[GatewayManager] Cannot read environ for pid=%d: %v", pid, err)
			continue
		}
		env := strings.ReplaceAll(string(envData), "\x00", "\n")
		if strings.Contains(env, "LD_PRELOAD=") && (preloadBase == "" || strings.Contains(env, preloadBase)) {
			logging.Info("[GatewayManager] Verified LD_PRELOAD on gateway listener pid=%d", pid)
			continue
		}
		logging.Warning(
			"[GatewayManager] Gateway listener pid=%d on port %d missing expected LD_PRELOAD (%s); sandbox may not be active on this process",
			pid, port, preloadLib,
		)
	}
}
