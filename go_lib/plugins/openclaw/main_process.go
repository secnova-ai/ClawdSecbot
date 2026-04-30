package openclaw

import (
	"bufio"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"go_lib/core"
	"go_lib/core/cmdutil"
	"go_lib/core/logging"
)

type openclawProcessCandidate struct {
	pid   int
	ppid  int
	score int
}

type openclawProcessMatchConstraints struct {
	gatewayPort string
	configDir   string
}

func resolveOpenclawMainProcessPID(asset core.Asset) (int, bool) {
	constraints := buildOpenclawProcessMatchConstraints(asset)

	if runtime.GOOS == "darwin" {
		output, err := cmdutil.Command("launchctl", "list").Output()
		if err != nil {
			logging.Warning("Openclaw main PID lookup via launchctl failed: %v", err)
		} else {
			for _, pid := range parseOpenclawLaunchctlPIDs(string(output)) {
				if processMatchesOpenclawAsset(pid, constraints) {
					return pid, true
				}
			}
		}
	}

	if runtime.GOOS == "linux" {
		for _, serviceName := range []string{"openclaw-gateway.service", "moltbot-gateway.service", "clawdbot-gateway.service"} {
			output, err := cmdutil.Command("systemctl", "--user", "show", serviceName, "-p", "MainPID", "--value").Output()
			if err != nil {
				continue
			}
			if pid, ok := parseOpenclawSystemdMainPID(string(output)); ok {
				if processMatchesOpenclawAsset(pid, constraints) {
					return pid, true
				}
			}
		}
	}

	if runtime.GOOS == "windows" {
		return 0, false
	}

	output, err := cmdutil.Command("ps", "-eo", "pid,ppid,comm,args").Output()
	if err != nil {
		logging.Warning("Openclaw main PID lookup via ps failed: %v", err)
		return 0, false
	}
	return parseOpenclawPSMainPID(string(output), constraints)
}

func buildOpenclawProcessMatchConstraints(asset core.Asset) openclawProcessMatchConstraints {
	constraints := openclawProcessMatchConstraints{
		gatewayPort: strings.TrimSpace(asset.Metadata["gateway_port"]),
	}
	if constraints.gatewayPort == "" && len(asset.Ports) == 1 && asset.Ports[0] > 0 {
		constraints.gatewayPort = strconv.Itoa(asset.Ports[0])
	}

	configPath := strings.TrimSpace(asset.Metadata["config_path"])
	if configPath != "" {
		constraints.configDir = filepath.Dir(configPath)
	}
	return constraints
}

func processMatchesOpenclawAsset(pid int, constraints openclawProcessMatchConstraints) bool {
	if pid <= 0 {
		return false
	}

	output, err := cmdutil.Command("ps", "-p", strconv.Itoa(pid), "-o", "pid,ppid,comm,args").Output()
	if err != nil {
		logging.Warning("Openclaw main PID lookup failed to inspect PID %d: %v", pid, err)
		return false
	}
	_, ok := parseOpenclawPSMainPID(string(output), constraints)
	return ok
}

func parseOpenclawLaunchctlPIDs(output string) []int {
	pids := make([]int, 0)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}

		label := strings.ToLower(fields[len(fields)-1])
		if !isOpenclawServiceLabel(label) {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}

func parseOpenclawSystemdMainPID(output string) (int, bool) {
	pid, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func parseOpenclawPSMainPID(output string, constraints openclawProcessMatchConstraints) (int, bool) {
	var best *openclawProcessCandidate
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(strings.ToUpper(line), "PID ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			continue
		}
		ppid, _ := strconv.Atoi(fields[1])
		cmdLine := strings.Join(fields[3:], " ")
		score, ok := scoreOpenclawMainCommand(cmdLine, ppid, constraints)
		if !ok {
			continue
		}

		candidate := &openclawProcessCandidate{pid: pid, ppid: ppid, score: score}
		if best == nil || candidate.score > best.score || candidate.score == best.score && candidate.pid < best.pid {
			best = candidate
		}
	}

	if best == nil {
		return 0, false
	}
	return best.pid, true
}

func scoreOpenclawMainCommand(cmdLine string, ppid int, constraints openclawProcessMatchConstraints) (int, bool) {
	lower := strings.ToLower(strings.TrimSpace(cmdLine))
	if lower == "" {
		return 0, false
	}
	if strings.Contains(lower, " rg ") || strings.Contains(lower, "/rg ") ||
		strings.Contains(lower, " grep ") || strings.Contains(lower, "/grep ") {
		return 0, false
	}
	if !containsAny(lower, []string{"openclaw", "moltbot", "clawdbot"}) {
		return 0, false
	}
	if !strings.Contains(lower, "gateway") {
		return 0, false
	}
	if !matchesOpenclawAssetConstraints(lower, constraints) {
		return 0, false
	}

	score := 1
	if ppid == 1 {
		score += 10
	}
	if strings.Contains(lower, "dist/index.js") {
		score += 5
	}
	if strings.Contains(lower, "node_modules/openclaw") {
		score += 4
	}
	if strings.Contains(lower, " gateway ") || strings.HasSuffix(lower, " gateway") {
		score += 3
	}
	if strings.Contains(lower, " --port ") {
		score += 2
	}
	if strings.TrimSpace(constraints.gatewayPort) != "" {
		score += 20
	}
	if strings.TrimSpace(constraints.configDir) != "" {
		score += 5
	}
	return score, true
}

func matchesOpenclawAssetConstraints(lowerCmdLine string, constraints openclawProcessMatchConstraints) bool {
	port := strings.TrimSpace(constraints.gatewayPort)
	if port != "" && !commandLineHasGatewayPort(lowerCmdLine, port) {
		return false
	}

	configDir := strings.ToLower(strings.TrimSpace(constraints.configDir))
	if port == "" && configDir != "" && !strings.Contains(lowerCmdLine, configDir) {
		return false
	}

	return true
}

func commandLineHasGatewayPort(lowerCmdLine, port string) bool {
	return strings.Contains(lowerCmdLine, " --port "+port) ||
		strings.Contains(lowerCmdLine, " --port="+port) ||
		strings.Contains(lowerCmdLine, fmt.Sprintf(":%s", port))
}

func isOpenclawServiceLabel(label string) bool {
	label = strings.ToLower(strings.TrimSpace(label))
	return containsAny(label, []string{"openclaw", "moltbot", "clawdbot"}) && strings.Contains(label, "gateway")
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
