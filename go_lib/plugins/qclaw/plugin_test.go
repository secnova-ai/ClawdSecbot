package qclaw

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	openclawplugin "go_lib/plugins/openclaw"
)

func TestFindConfigPathPrefersOverride(t *testing.T) {
	prevConfig := qclawConfigPathOverride
	prevState := qclawStatePathOverride
	t.Cleanup(func() {
		qclawConfigPathOverride = prevConfig
		qclawStatePathOverride = prevState
	})

	qclawConfigPathOverride = `C:\tmp\openclaw.json`
	qclawStatePathOverride = `C:\tmp\qclaw.json`

	got, err := findConfigPath()
	if err != nil {
		t.Fatalf("findConfigPath returned error: %v", err)
	}
	if got != qclawConfigPathOverride {
		t.Fatalf("expected override path %q, got %q", qclawConfigPathOverride, got)
	}
}

func TestUpdateAndRestoreInjectedModelState(t *testing.T) {
	rawModel := map[string]interface{}{
		"primary": "qclaw/modelroute",
	}
	updated := updateModelField(rawModel, "clawdsecbot-openai/gpt-4o", "qclaw/modelroute").(map[string]interface{})

	if got := updated["primary"].(string); got != "clawdsecbot-openai/gpt-4o" {
		t.Fatalf("expected injected primary, got %q", got)
	}

	restored := stripInjectedModel(updated, "clawdsecbot-openai/gpt-4o").(map[string]interface{})
	if got := restored["primary"].(string); got != "qclaw/modelroute" {
		t.Fatalf("expected restored primary, got %q", got)
	}
}

func TestRemoveInjectedModelsFromList(t *testing.T) {
	raw := []interface{}{"qclaw/modelroute", "clawdsecbot-openai/gpt-4o", "deepseek/deepseek-chat"}
	got := removeInjectedModels(raw).([]interface{})
	if len(got) != 2 {
		t.Fatalf("expected 2 remaining models, got %d", len(got))
	}
}

func TestLoadState(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "qclaw.json")
	payload := map[string]interface{}{
		"authGatewayBaseUrl": "http://127.0.0.1:19000/proxy",
		"configPath":         filepath.Join(dir, "openclaw.json"),
		"port":               28789,
		"stateDir":           dir,
		"cli": map[string]interface{}{
			"nodeBinary":  `C:\Program Files\QClaw\QClaw.exe`,
			"openclawMjs": `C:\Program Files\QClaw\resources\openclaw\node_modules\openclaw\openclaw.mjs`,
			"pid":         1234,
		},
	}
	content, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(statePath, content, 0644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	state, err := loadState(statePath)
	if err != nil {
		t.Fatalf("loadState returned error: %v", err)
	}
	if state.Port != 28789 {
		t.Fatalf("expected port 28789, got %d", state.Port)
	}
	if state.CLI.PID != 1234 {
		t.Fatalf("expected pid 1234, got %d", state.CLI.PID)
	}
}

func TestGetSkillsDirsIncludesExpectedQClawDirs(t *testing.T) {
	prevConfig := qclawConfigPathOverride
	prevState := qclawStatePathOverride
	prevHomeDir := userCurrentHomeDir
	t.Cleanup(func() {
		qclawConfigPathOverride = prevConfig
		qclawStatePathOverride = prevState
		userCurrentHomeDir = prevHomeDir
	})

	dir := t.TempDir()
	homeDir := filepath.Join(dir, "home")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	configPath := filepath.Join(dir, "openclaw.json")
	statePath := filepath.Join(dir, "qclaw.json")
	qclawConfigPathOverride = configPath
	qclawStatePathOverride = statePath
	userCurrentHomeDir = func() (string, error) { return homeDir, nil }

	content := []byte(`{
  "agents": {
    "defaults": {}
  },
  "skills": {
    "load": {}
  }
}`)
	if err := os.WriteFile(configPath, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	stateContent := []byte(`{
  "cli": {
    "openclawMjs": "C:\\Program Files\\QClaw\\resources\\openclaw\\node_modules\\openclaw\\openclaw.mjs"
  }
}`)
	if err := os.WriteFile(statePath, stateContent, 0644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	dirs, err := getSkillsDirs()
	if err != nil {
		t.Fatalf("getSkillsDirs returned error: %v", err)
	}

	expected := map[string]bool{
		strings.ToLower(filepath.Join(dir, "skills")):                                              true,
		strings.ToLower(filepath.Clean(`C:\Program Files\QClaw\resources\openclaw\config\skills`)): true,
	}

	for _, skillDir := range dirs {
		delete(expected, strings.ToLower(skillDir))
	}
	if len(expected) != 0 {
		t.Fatalf("missing expected skill dirs: %+v", expected)
	}
}

func TestWithOpenclawOverridesRestoresPreviousValues(t *testing.T) {
	prevConfig := qclawConfigPathOverride
	prevHomeDir := userCurrentHomeDir
	oldOpenclawConfig := openclawplugin.GetConfigPath()
	oldAppStore := openclawplugin.IsAppStoreBuild()
	t.Cleanup(func() {
		qclawConfigPathOverride = prevConfig
		userCurrentHomeDir = prevHomeDir
		openclawplugin.SetConfigPath(oldOpenclawConfig)
		openclawplugin.SetAppStoreBuild(oldAppStore)
	})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "openclaw.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	qclawConfigPathOverride = configPath
	userCurrentHomeDir = func() (string, error) { return dir, nil }

	openclawplugin.SetConfigPath("C:\\existing")
	openclawplugin.SetAppStoreBuild(true)

	restore, err := withOpenclawOverrides()
	if err != nil {
		t.Fatalf("withOpenclawOverrides returned error: %v", err)
	}

	if got := openclawplugin.GetConfigPath(); got != dir {
		t.Fatalf("expected temporary config path %q, got %q", dir, got)
	}
	if openclawplugin.IsAppStoreBuild() {
		t.Fatalf("expected temporary app store flag to be false")
	}

	restore()

	if got := openclawplugin.GetConfigPath(); got != "C:\\existing" {
		t.Fatalf("expected restored config path, got %q", got)
	}
	if !openclawplugin.IsAppStoreBuild() {
		t.Fatalf("expected restored app store flag to be true")
	}
}

func TestQClawAppDataAndLogsDir(t *testing.T) {
	homeDir := filepath.Join(string(filepath.Separator), "Users", "tester")

	gotAppData := qclawAppDataDir(homeDir)
	gotLogs := qclawLogsDir(homeDir)

	var wantAppData string
	switch runtime.GOOS {
	case "darwin":
		wantAppData = filepath.Join(homeDir, "Library", "Application Support", "QClaw")
	case "windows":
		wantAppData = filepath.Join(homeDir, "AppData", "Roaming", "QClaw")
	default:
		wantAppData = filepath.Join(homeDir, ".config", "QClaw")
	}

	if gotAppData != wantAppData {
		t.Fatalf("expected app data dir %q, got %q", wantAppData, gotAppData)
	}
	if gotLogs != filepath.Join(wantAppData, "logs") {
		t.Fatalf("expected logs dir under %q, got %q", wantAppData, gotLogs)
	}
}

func TestFindStatePathFallsBackToAppDataDir(t *testing.T) {
	prevState := qclawStatePathOverride
	prevHomeDir := userCurrentHomeDir
	t.Cleanup(func() {
		qclawStatePathOverride = prevState
		userCurrentHomeDir = prevHomeDir
	})

	dir := t.TempDir()
	homeDir := filepath.Join(dir, "home")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}

	statePath := filepath.Join(qclawAppDataDir(homeDir), "qclaw.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(statePath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	userCurrentHomeDir = func() (string, error) { return homeDir, nil }

	got, err := findStatePath()
	if err != nil {
		t.Fatalf("findStatePath returned error: %v", err)
	}
	if got != statePath {
		t.Fatalf("expected state path %q, got %q", statePath, got)
	}
}

func TestQClawDefaultInstallPaths(t *testing.T) {
	nodeBinary := qclawDefaultNodeBinaryPath()
	openclawMjs := qclawDefaultOpenclawMjsPath()
	builtinSkills := qclawDefaultBuiltinSkillsDir()

	switch runtime.GOOS {
	case "darwin":
		if nodeBinary != "/Applications/QClaw.app/Contents/MacOS/QClaw" {
			t.Fatalf("unexpected darwin node binary path: %q", nodeBinary)
		}
		if openclawMjs != "/Applications/QClaw.app/Contents/Resources/openclaw/node_modules/openclaw/openclaw.mjs" {
			t.Fatalf("unexpected darwin openclaw.mjs path: %q", openclawMjs)
		}
		if builtinSkills != "/Applications/QClaw.app/Contents/Resources/openclaw/config/skills" {
			t.Fatalf("unexpected darwin builtin skills path: %q", builtinSkills)
		}
	case "windows":
		if !strings.HasSuffix(strings.ToLower(nodeBinary), strings.ToLower(`QClaw\QClaw.exe`)) {
			t.Fatalf("unexpected windows node binary path: %q", nodeBinary)
		}
		if !strings.HasSuffix(strings.ToLower(openclawMjs), strings.ToLower(`QClaw\resources\openclaw\node_modules\openclaw\openclaw.mjs`)) {
			t.Fatalf("unexpected windows openclaw.mjs path: %q", openclawMjs)
		}
		if !strings.HasSuffix(strings.ToLower(builtinSkills), strings.ToLower(`QClaw\resources\openclaw\config\skills`)) {
			t.Fatalf("unexpected windows builtin skills path: %q", builtinSkills)
		}
	default:
		if nodeBinary != "" || openclawMjs != "" || builtinSkills != "" {
			t.Fatalf("expected empty defaults on %s, got %q %q %q", runtime.GOOS, nodeBinary, openclawMjs, builtinSkills)
		}
	}
}
