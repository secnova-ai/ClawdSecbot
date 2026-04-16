package hermes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindConfigPath_FromOverrideDir(t *testing.T) {
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, ".hermes")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("model:\n  provider: openai\n"), 0600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	old := GetConfigPath()
	SetConfigPath(tmp)
	t.Cleanup(func() { SetConfigPath(old) })

	got, err := findConfigPath()
	if err != nil {
		t.Fatalf("findConfigPath failed: %v", err)
	}
	if got != cfgPath {
		t.Fatalf("config path mismatch: got=%s want=%s", got, cfgPath)
	}
}
