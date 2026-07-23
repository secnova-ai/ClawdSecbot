package dwtsclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type gatewayConfigPatchContract interface {
	Patch(configPath string, patch map[string]interface{}) error
}

type liveGatewayConfigPatcher struct{}

var gatewayConfigPatcher gatewayConfigPatchContract = liveGatewayConfigPatcher{}

func (liveGatewayConfigPatcher) Patch(configPath string, patch map[string]interface{}) error {
	runtime, err := discoverGatewayRuntime()
	if err != nil {
		return err
	}
	snapshot, err := invokeGatewayRPC(runtime, "config.get", map[string]interface{}{})
	if err != nil {
		return err
	}
	baseHash := findGatewayString(snapshot, "baseHash")
	if baseHash == "" {
		baseHash = findGatewayString(snapshot, "hash")
	}
	if baseHash == "" {
		return fmt.Errorf("gateway config.get did not return baseHash")
	}
	rawPatch, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	_, err = invokeGatewayRPC(runtime, "config.patch", map[string]interface{}{
		"raw":      string(rawPatch),
		"baseHash": baseHash,
		"note":     "ClawdSecbot updated the active provider base URL",
	})
	return err
}

type gatewayRuntime struct {
	executable string
	entry      string
}

func discoverGatewayRuntime() (gatewayRuntime, error) {
	for _, root := range candidateInstallRoots() {
		if !isDWTSClawInstallRoot(root) {
			continue
		}
		return gatewayRuntime{
			executable: filepath.Join(root, "DWTSClaw.exe"),
			entry:      filepath.Join(root, "resources", "cfmind", "openclaw.mjs"),
		}, nil
	}
	return gatewayRuntime{}, fmt.Errorf("dwtsclaw runtime not found")
}

func invokeGatewayRPC(runtime gatewayRuntime, method string, params map[string]interface{}) (map[string]interface{}, error) {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, runtime.executable,
		runtime.entry,
		"gateway",
		"call",
		method,
		"--params",
		string(paramsJSON),
		"--json",
	)
	cmd.Dir = filepath.Dir(runtime.entry)
	cmd.Env = append(os.Environ(), "ELECTRON_RUN_AS_NODE=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gateway %s failed: %w", method, err)
	}
	var response map[string]interface{}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("gateway %s returned invalid JSON", method)
	}
	return response, nil
}

func findGatewayString(value interface{}, key string) string {
	switch typed := value.(type) {
	case map[string]interface{}:
		if raw, ok := typed[key].(string); ok && strings.TrimSpace(raw) != "" {
			return strings.TrimSpace(raw)
		}
		for _, child := range typed {
			if found := findGatewayString(child, key); found != "" {
				return found
			}
		}
	case []interface{}:
		for _, child := range typed {
			if found := findGatewayString(child, key); found != "" {
				return found
			}
		}
	}
	return ""
}
