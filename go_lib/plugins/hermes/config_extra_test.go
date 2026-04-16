package hermes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindConfigPath_FromEnvHermesConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("model:\n  provider: openai\n"), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldOverride := GetConfigPath()
	oldEnv := os.Getenv("HERMES_CONFIG")
	oldHome := os.Getenv("HERMES_HOME")
	SetConfigPath("")
	if err := os.Setenv("HERMES_CONFIG", cfgPath); err != nil {
		t.Fatalf("set env failed: %v", err)
	}
	if err := os.Unsetenv("HERMES_HOME"); err != nil {
		t.Fatalf("unset env failed: %v", err)
	}
	t.Cleanup(func() {
		SetConfigPath(oldOverride)
		_ = os.Setenv("HERMES_CONFIG", oldEnv)
		_ = os.Setenv("HERMES_HOME", oldHome)
	})

	got, err := findConfigPath()
	if err != nil {
		t.Fatalf("findConfigPath failed: %v", err)
	}
	if got != cfgPath {
		t.Fatalf("config path mismatch: got=%s want=%s", got, cfgPath)
	}
}

func TestFindConfigPath_FromEnvHermesHome(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("model:\n  provider: openai\n"), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	oldOverride := GetConfigPath()
	oldEnv := os.Getenv("HERMES_CONFIG")
	oldHome := os.Getenv("HERMES_HOME")
	SetConfigPath("")
	if err := os.Unsetenv("HERMES_CONFIG"); err != nil {
		t.Fatalf("unset env failed: %v", err)
	}
	if err := os.Setenv("HERMES_HOME", tmp); err != nil {
		t.Fatalf("set env failed: %v", err)
	}
	t.Cleanup(func() {
		SetConfigPath(oldOverride)
		_ = os.Setenv("HERMES_CONFIG", oldEnv)
		_ = os.Setenv("HERMES_HOME", oldHome)
	})

	got, err := findConfigPath()
	if err != nil {
		t.Fatalf("findConfigPath failed: %v", err)
	}
	if got != cfgPath {
		t.Fatalf("config path mismatch: got=%s want=%s", got, cfgPath)
	}
}

func TestEnsureMapAndNestedGetters(t *testing.T) {
	raw := map[string]interface{}{}
	security := ensureMap(raw, "security")
	security["redact_secrets"] = true
	if v := getNestedString(raw, "security", "missing"); v != "" {
		t.Fatalf("expected empty missing string, got %q", v)
	}
	if b, ok := getNestedBool(raw, "security", "redact_secrets"); !ok || !b {
		t.Fatalf("expected nested bool true, got value=%v ok=%v", b, ok)
	}

	raw2 := map[string]interface{}{
		"model": map[string]interface{}{"default": "MiniMax-M2.7-coding-plan"},
	}
	if got := getNestedString(raw2, "model", "default"); got != "MiniMax-M2.7-coding-plan" {
		t.Fatalf("nested string mismatch: %q", got)
	}

	raw3 := map[string]interface{}{
		"security": map[interface{}]interface{}{"redact_secrets": false},
	}
	m := ensureMap(raw3, "security")
	if _, ok := m["redact_secrets"]; !ok {
		t.Fatalf("expected map conversion to preserve redact_secrets key: %+v", m)
	}
}

func TestSetAppStoreBuild(t *testing.T) {
	old := IsAppStoreBuild()
	SetAppStoreBuild(true)
	if !IsAppStoreBuild() {
		t.Fatal("expected app store build flag to be true")
	}
	SetAppStoreBuild(false)
	if IsAppStoreBuild() {
		t.Fatal("expected app store build flag to be false")
	}
	SetAppStoreBuild(old)
}
