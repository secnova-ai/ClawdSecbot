package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCollectorFileExistsExpandsTildeToUserHome 验证资产扫描的 ~ 展开始终使用用户家目录。
func TestCollectorFileExistsExpandsTildeToUserHome(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	userHomeDir := filepath.Join(t.TempDir(), "home")
	configDir := filepath.Join(userHomeDir, ".nullclaw")
	configPath := filepath.Join(configDir, "config.json")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	t.Setenv("HOME", userHomeDir)
	t.Setenv("USERPROFILE", userHomeDir)

	pm := GetPathManager()
	if err := pm.ResetForTest(workspaceDir, filepath.Join(t.TempDir(), "runtime-home")); err != nil {
		t.Fatalf("ResetForTest failed: %v", err)
	}
	t.Cleanup(func() {
		_ = pm.ResetForTest("", "")
	})

	snapshot, err := NewCollector("").Collect()
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if !snapshot.FileExists("~/.nullclaw/config.json") {
		t.Fatalf("FileExists should expand ~ to user home %q", userHomeDir)
	}
}
