package hermes

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go_lib/core/proxy"
)

func writeHermesBinary(t *testing.T, script string) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("gateway command script tests are unix-only")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "hermes")
	content := "#!/bin/sh\n" + script + "\n"
	if err := os.WriteFile(bin, []byte(content), 0o755); err != nil {
		t.Fatalf("write binary failed: %v", err)
	}
	return dir, bin
}

func TestRunHermesGatewayCommand(t *testing.T) {
	_, bin := writeHermesBinary(t, "echo \"HOME=$HOME ARGS=$*\"; exit 0")
	out, err := runHermesGatewayCommand(bin, []string{"restart"}, "/tmp/hermes-home")
	if err != nil {
		t.Fatalf("runHermesGatewayCommand failed: %v", err)
	}
	if !strings.Contains(out, "HOME=/tmp/hermes-home") {
		t.Fatalf("expected HOME env in output, got: %s", out)
	}
	if !strings.Contains(out, "ARGS=gateway restart") {
		t.Fatalf("expected gateway restart args in output, got: %s", out)
	}
}

func TestRunHermesGatewayCommand_EmptyBinary(t *testing.T) {
	if _, err := runHermesGatewayCommand("", []string{"restart"}, ""); err == nil {
		t.Fatal("expected empty binary error")
	}
}

func TestRestartHermesGateway_RestartSuccess(t *testing.T) {
	dir, _ := writeHermesBinary(t, strings.Join([]string{
		"if [ \"$1\" = \"gateway\" ] && [ \"$2\" = \"restart\" ]; then",
		"  echo restart-ok",
		"  exit 0",
		"fi",
		"exit 1",
	}, "\n"))

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", dir); err != nil {
		t.Fatalf("set PATH failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	result, err := restartHermesGateway(&GatewayRestartRequest{AssetID: "hermes:restart", SandboxEnabled: true})
	if err != nil {
		t.Fatalf("restartHermesGateway failed: %v", err)
	}
	if success, _ := result["success"].(bool); !success {
		t.Fatalf("expected restart success result: %+v", result)
	}
	if result["command"] != "gateway restart" {
		t.Fatalf("expected gateway restart command result: %+v", result)
	}
}

func TestRestartHermesGateway_FallbackStopStart(t *testing.T) {
	dir, _ := writeHermesBinary(t, strings.Join([]string{
		"if [ \"$1\" = \"gateway\" ] && [ \"$2\" = \"restart\" ]; then",
		"  echo restart-failed",
		"  exit 1",
		"fi",
		"if [ \"$1\" = \"gateway\" ] && [ \"$2\" = \"stop\" ]; then",
		"  echo stop-ok",
		"  exit 0",
		"fi",
		"if [ \"$1\" = \"gateway\" ] && [ \"$2\" = \"start\" ]; then",
		"  echo start-ok",
		"  exit 0",
		"fi",
		"exit 1",
	}, "\n"))

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", dir); err != nil {
		t.Fatalf("set PATH failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	result, err := restartHermesGateway(&GatewayRestartRequest{AssetID: "hermes:fallback"})
	if err != nil {
		t.Fatalf("restartHermesGateway fallback failed: %v", err)
	}
	if success, _ := result["success"].(bool); !success {
		t.Fatalf("expected fallback success result: %+v", result)
	}
	if result["command"] != "gateway stop/start" {
		t.Fatalf("expected stop/start command result: %+v", result)
	}
}

func TestRestartHermesGateway_NotFound(t *testing.T) {
	oldPath := os.Getenv("PATH")
	empty := t.TempDir()
	if err := os.Setenv("PATH", empty); err != nil {
		t.Fatalf("set PATH failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	if _, err := restartHermesGateway(&GatewayRestartRequest{}); err == nil {
		t.Fatal("expected hermes binary not found error")
	}
}

func TestStartGatewayWithProxy_ValidationErrors(t *testing.T) {
	old := IsAppStoreBuild()
	SetAppStoreBuild(true)
	result, err := startGatewayWithProxy(18080, &proxy.BotModelConfig{Model: "MiniMax-M2.7-coding-plan"}, t.TempDir(), "hermes:test")
	if err != nil {
		t.Fatalf("app store branch should not return error, got: %v", err)
	}
	if success, _ := result["success"].(bool); success {
		t.Fatalf("expected app store branch to fail: %+v", result)
	}
	SetAppStoreBuild(false)
	SetAppStoreBuild(old)

	if _, err := startGatewayWithProxy(0, &proxy.BotModelConfig{Model: "x"}, t.TempDir(), "hermes:test"); err == nil {
		t.Fatal("expected invalid port error")
	}
	if _, err := startGatewayWithProxy(18080, nil, t.TempDir(), "hermes:test"); err == nil {
		t.Fatal("expected nil bot config error")
	}
	if _, err := startGatewayWithProxy(18080, &proxy.BotModelConfig{}, t.TempDir(), "hermes:test"); err == nil {
		t.Fatal("expected empty bot model error")
	}
	if _, err := startGatewayWithProxy(18080, &proxy.BotModelConfig{Model: "x"}, "", "hermes:test"); err == nil {
		t.Fatal("expected empty backup dir error")
	}
}

func TestUpdateHermesProxyModelAndToJSONString(t *testing.T) {
	if err := updateHermesProxyModel(nil, "http://127.0.0.1:18080", &proxy.BotModelConfig{Model: "x"}); err == nil {
		t.Fatal("expected nil raw config error")
	}

	raw := map[string]interface{}{}
	if err := updateHermesProxyModel(raw, "http://127.0.0.1:18080", &proxy.BotModelConfig{Model: "MiniMax-M2.7-coding-plan"}); err != nil {
		t.Fatalf("updateHermesProxyModel failed: %v", err)
	}
	model, _ := raw["model"].(map[string]interface{})
	if model["provider"] != "custom" || model["default"] != "MiniMax-M2.7-coding-plan" {
		t.Fatalf("unexpected updated model block: %+v", model)
	}

	payload := toJSONString(map[string]interface{}{"success": true})
	if !strings.Contains(payload, `"success":true`) {
		t.Fatalf("unexpected toJSONString payload: %s", payload)
	}
}

func TestBuildGatewayRestartRequestFromDB_Default(t *testing.T) {
	req := buildGatewayRestartRequestFromDB("hermes:asset")
	if req.AssetName != hermesAssetName {
		t.Fatalf("unexpected asset name: %+v", req)
	}
	if req.AssetID != "hermes:asset" {
		t.Fatalf("unexpected asset id: %+v", req)
	}
}
