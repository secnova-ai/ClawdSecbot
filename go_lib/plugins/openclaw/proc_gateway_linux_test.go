//go:build linux

package openclaw

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProcNetTCPListenPort(t *testing.T) {
	tests := []struct {
		field    string
		wantPort int
		wantOK   bool
	}{
		{"0100007F:4965", 18789, true},
		{"00000000:1F90", 8080, true},
		{"0100007F:ZZZZ", 0, false},
		{"bad", 0, false},
	}
	for _, tt := range tests {
		gotPort, gotOK := parseProcNetTCPListenPort(tt.field)
		if gotOK != tt.wantOK || gotPort != tt.wantPort {
			t.Fatalf("parseProcNetTCPListenPort(%q) = (%d, %v), want (%d, %v)",
				tt.field, gotPort, gotOK, tt.wantPort, tt.wantOK)
		}
	}
}

func TestCollectListenInodesFromProcNetFile(t *testing.T) {
	tmpDir := t.TempDir()
	procFile := filepath.Join(tmpDir, "tcp")
	content := "" +
		"  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 0100007F:4965 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 4242 1 0000000000000000 100 0 0 10 0\n" +
		"   1: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 9999 1 0000000000000000 100 0 0 10 0\n"
	if err := os.WriteFile(procFile, []byte(content), 0644); err != nil {
		t.Fatalf("write proc sample failed: %v", err)
	}

	inodeSet := make(map[string]struct{})
	collectListenInodesFromProcNetFile(procFile, 18789, inodeSet)
	if _, ok := inodeSet["4242"]; !ok {
		t.Fatalf("expected inode 4242 for port 18789, got %v", inodeSet)
	}
	if _, ok := inodeSet["9999"]; ok {
		t.Fatalf("did not expect inode 9999 for port 18789, got %v", inodeSet)
	}
}

func TestMatchOpenclawGatewayCmdline(t *testing.T) {
	cmdline := strings.Join([]string{
		"node",
		"/home/node/.openclaw/openclaw.mjs",
		"gateway",
		"--allow-unconfigured",
	}, "\x00")
	if !matchOpenclawGatewayCmdline([]byte(cmdline)) {
		t.Fatal("expected openclaw gateway cmdline match")
	}
	openclawOnly := strings.Join([]string{
		"openclaw",
	}, "\x00")
	if !matchOpenclawGatewayCmdline([]byte(openclawOnly)) {
		t.Fatal("expected openclaw cmdline match on gateway listener")
	}
	if matchOpenclawGatewayCmdline([]byte("nginx: worker process")) {
		t.Fatal("did not expect unrelated process match")
	}
}

// TestBuildGatewayEnvUsesProvidedHomeDir 验证 gateway 进程 HOME 使用传入的运行用户家目录。
func TestBuildGatewayEnvUsesProvidedHomeDir(t *testing.T) {
	t.Setenv("HOME", "/app")

	env := buildGatewayEnv("/home/node", []string{"LD_PRELOAD=/tmp/libsandbox_preload.so"})
	var homeValues []string
	var hasPreload bool
	for _, item := range env {
		if strings.HasPrefix(item, "HOME=") {
			homeValues = append(homeValues, item)
		}
		if item == "LD_PRELOAD=/tmp/libsandbox_preload.so" {
			hasPreload = true
		}
	}

	if len(homeValues) != 1 || homeValues[0] != "HOME=/home/node" {
		t.Fatalf("HOME env = %v, want [HOME=/home/node]", homeValues)
	}
	if !hasPreload {
		t.Fatal("expected LD_PRELOAD env to be preserved")
	}
}
