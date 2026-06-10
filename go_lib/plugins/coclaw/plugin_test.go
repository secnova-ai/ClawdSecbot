package coclaw

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go_lib/core"
	openclawplugin "go_lib/plugins/openclaw"
)

func TestPluginManifestAndModelConfigPolicy(t *testing.T) {
	if plugin == nil {
		t.Fatal("expected global CoClaw plugin")
	}
	if plugin.GetID() != coclawPluginID {
		t.Fatalf("unexpected plugin id: %q", plugin.GetID())
	}
	if plugin.GetAssetName() != coclawAssetName {
		t.Fatalf("unexpected asset name: %q", plugin.GetAssetName())
	}
	if plugin.RequiresBotModelConfig() {
		t.Fatal("CoClaw should resolve forwarding target from its own config")
	}
	if plugin.GetManifest().PluginID != coclawPluginID {
		t.Fatalf("unexpected manifest plugin id: %q", plugin.GetManifest().PluginID)
	}
}

func TestApplyProxyConfigAndRestoreDefaultState(t *testing.T) {
	previousOverride := configPathOverride
	t.Cleanup(func() {
		configPathOverride = previousOverride
	})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "openclaw.json")
	backupDir := filepath.Join(dir, "backup")
	configPathOverride = configPath

	content := []byte(`{
  "agents": {
    "defaults": {
      "model": {
        "primary": "custom-llm/Qwen3.5-397B-A17B-FP8"
      }
    }
  },
  "models": {
    "providers": {
      "custom-llm": {
        "baseUrl": "http://172.16.0.6:3000/v1/",
        "apiKey": {
          "source": "env",
          "id": "CUSTOM_LLM_API_KEY"
        }
      }
    }
  },
  "gateway": {
    "port": 18790
  }
}`)
	if err := os.WriteFile(configPath, content, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := applyProxyConfig(&core.ProtectionContext{ProxyPort: 13436, BackupDir: backupDir})
	if err != nil {
		t.Fatalf("applyProxyConfig returned error: %v", err)
	}
	if result["method"] != "provider_alias_hijack" {
		t.Fatalf("unexpected method: %v", result["method"])
	}

	raw := readConfigMap(t, configPath)
	model := raw["agents"].(map[string]interface{})["defaults"].(map[string]interface{})["model"].(map[string]interface{})
	if model["primary"] != "clawdsecbot-custom-llm/Qwen3.5-397B-A17B-FP8" {
		t.Fatalf("primary = %v", model["primary"])
	}
	providers := raw["models"].(map[string]interface{})["providers"].(map[string]interface{})
	if _, ok := providers["clawdsecbot-custom-llm"]; !ok {
		t.Fatal("expected proxy provider to be written")
	}

	if err := restoreBotDefaultState(&core.ProtectionContext{BackupDir: backupDir}); err != nil {
		t.Fatalf("restoreBotDefaultState returned error: %v", err)
	}
	restored := readConfigMap(t, configPath)
	restoredModel := restored["agents"].(map[string]interface{})["defaults"].(map[string]interface{})["model"].(map[string]interface{})
	if restoredModel["primary"] != "custom-llm/Qwen3.5-397B-A17B-FP8" {
		t.Fatalf("restored primary = %v", restoredModel["primary"])
	}
	restoredProviders := restored["models"].(map[string]interface{})["providers"].(map[string]interface{})
	if _, ok := restoredProviders["clawdsecbot-custom-llm"]; ok {
		t.Fatal("proxy provider should be removed after restore")
	}
}

func TestWithOpenclawOverridesRestoresPreviousValues(t *testing.T) {
	previousOverride := configPathOverride
	previousOpenClawConfig := openclawplugin.GetConfigPath()
	previousAppStoreBuild := openclawplugin.IsAppStoreBuild()
	t.Cleanup(func() {
		configPathOverride = previousOverride
		openclawplugin.SetConfigPath(previousOpenClawConfig)
		openclawplugin.SetAppStoreBuild(previousAppStoreBuild)
	})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "openclaw.json")
	configPathOverride = configPath
	if err := os.WriteFile(configPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

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
		t.Fatal("expected app store mode to be disabled during override")
	}

	restore()

	if got := openclawplugin.GetConfigPath(); got != "C:\\existing" {
		t.Fatalf("expected restored config path, got %q", got)
	}
	if !openclawplugin.IsAppStoreBuild() {
		t.Fatal("expected app store mode to be restored")
	}
}

func readConfigMap(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(content, &raw); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return raw
}
