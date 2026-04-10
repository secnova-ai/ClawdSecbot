package nullclaw

import (
	"strings"
	"testing"
)

// TestEnsureProviderForBotModel_DoesNotWriteRealAPIKey verifies that the real API key
// from BotModelConfig is never written to the nullclaw.json provider config.
// Instead, a placeholder value (proxyInjectedAPIKey) is used, because the LLM proxy
// injects the real key at forwarding time.
func TestEnsureProviderForBotModel_DoesNotWriteRealAPIKey(t *testing.T) {
	rawConfig := map[string]interface{}{
		"agents": map[string]interface{}{},
		"models": map[string]interface{}{},
	}

	botConfig := &BotModelConfig{
		Provider:  "anthropic",
		BaseURL:   "https://api.anthropic.com",
		APIKey:    "sk-ant-real-secret-key-12345",
		Model:     "claude-sonnet-4-20250514",
		SecretKey: "",
	}

	previousProvider, providerMap, err := ensureProviderForBotModel(rawConfig, botConfig, "anthropic", "claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("ensureProviderForBotModel returned error: %v", err)
	}

	// Verify the api_key field is the placeholder, not the real key
	// Note: Nullclaw uses snake_case "api_key" (OpenClaw uses camelCase "apiKey")
	apiKeyValue, ok := providerMap["api_key"].(string)
	if !ok {
		t.Fatal("expected api_key to be a string")
	}
	if apiKeyValue != proxyInjectedAPIKey {
		t.Fatalf("expected api_key to be %q, got %q", proxyInjectedAPIKey, apiKeyValue)
	}
	if strings.Contains(apiKeyValue, "sk-ant-real") {
		t.Fatal("real API key was written to provider config, this is a security leak")
	}

	// previousProvider should be empty since no provider existed before
	if len(previousProvider) != 0 {
		t.Fatalf("expected empty previousProvider, got %v", previousProvider)
	}
}

// TestEnsureProviderForBotModel_OverwritesPreviousProviderKey verifies that even if
// the previousProvider config contained a real API key, it gets replaced by the placeholder.
func TestEnsureProviderForBotModel_OverwritesPreviousProviderKey(t *testing.T) {
	rawConfig := map[string]interface{}{
		"agents": map[string]interface{}{},
		"models": map[string]interface{}{
			"providers": map[string]interface{}{
				"anthropic": map[string]interface{}{
					"api":        "anthropic-messages",
					"api_key":    "sk-ant-leaked-key-should-not-persist",
					"base_url":   "https://api.anthropic.com",
					"models":     []interface{}{"claude-3-opus"},
				},
			},
		},
	}

	botConfig := &BotModelConfig{
		Provider: "anthropic",
		BaseURL:  "https://api.anthropic.com",
		APIKey:   "sk-ant-new-key-from-db",
		Model:    "claude-sonnet-4-20250514",
	}

	_, providerMap, err := ensureProviderForBotModel(rawConfig, botConfig, "anthropic", "claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("ensureProviderForBotModel returned error: %v", err)
	}

	// The api_key should be the placeholder, not the old leaked key or the new DB key
	apiKeyValue, ok := providerMap["api_key"].(string)
	if !ok {
		t.Fatal("expected api_key to be a string")
	}
	if apiKeyValue != proxyInjectedAPIKey {
		t.Fatalf("expected api_key to be %q, got %q", proxyInjectedAPIKey, apiKeyValue)
	}
	if strings.Contains(apiKeyValue, "sk-ant") && apiKeyValue != proxyInjectedAPIKey {
		t.Fatal("real API key (old or new) was written to provider config, this is a security leak")
	}
}
