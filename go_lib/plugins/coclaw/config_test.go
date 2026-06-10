package coclaw

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFindConfigPathPrefersProgramDataRuntimeConfig(t *testing.T) {
	previousOverride := configPathOverride
	previousUserHomeDir := userHomeDir
	t.Cleanup(func() {
		configPathOverride = previousOverride
		userHomeDir = previousUserHomeDir
	})

	homeDir := t.TempDir()
	programData := t.TempDir()
	userHomeDir = func() (string, error) {
		return homeDir, nil
	}
	t.Setenv("ProgramData", programData)

	configPath := filepath.Join(programData, "CoClaw", "config", "openclaw.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"gateway":{"port":18790}}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := findConfigPath()
	if err != nil {
		t.Fatalf("findConfigPath returned error: %v", err)
	}
	if got != configPath {
		t.Fatalf("expected ProgramData config %q, got %q", configPath, got)
	}
}

func TestHijackProviderAliasPreservesSecretRefProvider(t *testing.T) {
	secretRef := map[string]interface{}{
		"source": "env",
		"id":     "CUSTOM_LLM_API_KEY",
	}
	raw := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model": map[string]interface{}{
					"primary": "custom-llm/Qwen3.5-397B-A17B-FP8",
				},
			},
		},
		"models": map[string]interface{}{
			"providers": map[string]interface{}{
				"custom-llm": map[string]interface{}{
					"baseUrl": "http://172.16.0.6:3000/v1/",
					"apiKey":  secretRef,
				},
			},
		},
	}

	result, changed, err := hijackProviderAlias(raw, "http://127.0.0.1:13436/v1")
	if err != nil {
		t.Fatalf("hijackProviderAlias returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected config to change")
	}
	if result.ProxyProviderName != "clawdsecbot-custom-llm" {
		t.Fatalf("unexpected proxy provider: %q", result.ProxyProviderName)
	}
	if result.ProxyPrimary != "clawdsecbot-custom-llm/Qwen3.5-397B-A17B-FP8" {
		t.Fatalf("unexpected proxy primary: %q", result.ProxyPrimary)
	}

	providers := raw["models"].(map[string]interface{})["providers"].(map[string]interface{})
	originalProvider := providers["custom-llm"].(map[string]interface{})
	if originalProvider["baseUrl"] != "http://172.16.0.6:3000/v1/" {
		t.Fatalf("original provider baseUrl changed: %v", originalProvider["baseUrl"])
	}
	if !reflect.DeepEqual(originalProvider["apiKey"], secretRef) {
		t.Fatal("original provider SecretRef apiKey changed")
	}

	proxyProvider := providers["clawdsecbot-custom-llm"].(map[string]interface{})
	if proxyProvider["baseUrl"] != "http://127.0.0.1:13436/v1" {
		t.Fatalf("proxy provider baseUrl = %v", proxyProvider["baseUrl"])
	}
	if !reflect.DeepEqual(proxyProvider["apiKey"], secretRef) {
		t.Fatal("proxy provider must copy SecretRef apiKey")
	}

	model := raw["agents"].(map[string]interface{})["defaults"].(map[string]interface{})["model"].(map[string]interface{})
	if model["primary"] != "clawdsecbot-custom-llm/Qwen3.5-397B-A17B-FP8" {
		t.Fatalf("primary = %v", model["primary"])
	}
}

func TestRestoreProviderAliasRemovesProxyAndRestoresPrimary(t *testing.T) {
	raw := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model": map[string]interface{}{
					"primary": "clawdsecbot-custom-llm/Qwen3.5-397B-A17B-FP8",
				},
			},
		},
		"models": map[string]interface{}{
			"providers": map[string]interface{}{
				"custom-llm": map[string]interface{}{
					"baseUrl": "http://172.16.0.6:3000/v1/",
				},
				"clawdsecbot-custom-llm": map[string]interface{}{
					"baseUrl": "http://127.0.0.1:13436/v1",
				},
			},
		},
	}
	backup := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model": map[string]interface{}{
					"primary": "custom-llm/Qwen3.5-397B-A17B-FP8",
				},
			},
		},
	}

	result, changed, err := restoreProviderAlias(raw, backup)
	if err != nil {
		t.Fatalf("restoreProviderAlias returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected config to change")
	}
	if result.RestoredPrimary != "custom-llm/Qwen3.5-397B-A17B-FP8" {
		t.Fatalf("unexpected restored primary: %q", result.RestoredPrimary)
	}

	model := raw["agents"].(map[string]interface{})["defaults"].(map[string]interface{})["model"].(map[string]interface{})
	if model["primary"] != "custom-llm/Qwen3.5-397B-A17B-FP8" {
		t.Fatalf("primary = %v", model["primary"])
	}
	providers := raw["models"].(map[string]interface{})["providers"].(map[string]interface{})
	if _, exists := providers["clawdsecbot-custom-llm"]; exists {
		t.Fatal("proxy provider should be removed")
	}
	if _, exists := providers["custom-llm"]; !exists {
		t.Fatal("original provider should remain")
	}
}
