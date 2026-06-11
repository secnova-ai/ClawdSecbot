package openclaw

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigAcceptsProviderSecretRefAPIKey(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "openclaw.json")
	if err := os.WriteFile(configPath, []byte(`{
  "agents": {
    "defaults": {
      "model": {
        "primary": "custom-llm/model-a"
      }
    }
  },
  "models": {
    "providers": {
      "custom-llm": {
        "baseUrl": "https://example.invalid/v1",
        "apiKey": {
          "source": "env",
          "id": "CUSTOM_LLM_API_KEY"
        }
      }
    }
  }
}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	config, raw, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}
	provider := config.Models.Providers["custom-llm"]
	if provider == nil {
		t.Fatal("expected custom-llm provider")
	}
	secretRef, ok := provider.APIKey.(map[string]interface{})
	if !ok {
		t.Fatalf("expected provider apiKey SecretRef object, got %T", provider.APIKey)
	}
	if secretRef["id"] != "CUSTOM_LLM_API_KEY" {
		t.Fatalf("unexpected SecretRef id: %v", secretRef["id"])
	}
	rawProvider := raw["models"].(map[string]interface{})["providers"].(map[string]interface{})["custom-llm"].(map[string]interface{})
	if _, ok := rawProvider["apiKey"].(map[string]interface{}); !ok {
		t.Fatalf("raw apiKey should remain a SecretRef object, got %T", rawProvider["apiKey"])
	}
}
