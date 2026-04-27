package modelfactory

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"go_lib/chatmodel-routing/adapter"
)

func TestNormalizeOpenAIBaseForModels(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://api.openai.com/v1", "https://api.openai.com/v1"},
		{"https://api.openai.com/v1/chat/completions", "https://api.openai.com/v1"},
		{"https://api.openai.com/v1/", "https://api.openai.com/v1"},
		{"https://api.custom.com", "https://api.custom.com/v1"},
	}
	for _, tt := range tests {
		if got := normalizeOpenAIBaseForModels(tt.in); got != tt.want {
			t.Errorf("normalizeOpenAIBaseForModels(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGetProviderModelsJSON_Invalid(t *testing.T) {
	got := GetProviderModelsJSON("")
	if got == "" {
		t.Fatal("expected non-empty json")
	}
	if !strings.Contains(got, `"success":false`) {
		t.Fatalf("expected failure: %s", got)
	}
}

func TestValidateModelCatalogBaseURL_RejectPrivateForNonOllama(t *testing.T) {
	err := validateModelCatalogBaseURL(
		context.Background(),
		adapter.ProviderOpenAI,
		"http://127.0.0.1:11434",
	)
	if err == nil {
		t.Fatal("expected private loopback host to be rejected for OpenAI")
	}
}

func TestValidateModelCatalogBaseURL_AllowLoopbackForOllama(t *testing.T) {
	err := validateModelCatalogBaseURL(
		context.Background(),
		adapter.ProviderOllama,
		"http://127.0.0.1:11434",
	)
	if err != nil {
		t.Fatalf("expected Ollama loopback host to be allowed, got: %v", err)
	}
}

func TestValidateModelCatalogBaseURL_AllowLoopbackForCompatibleProviders(t *testing.T) {
	cases := []adapter.ProviderName{
		adapter.ProviderOpenAICompatible,
		adapter.ProviderAnthropicCompatible,
	}
	for _, provider := range cases {
		err := validateModelCatalogBaseURL(
			context.Background(),
			provider,
			"http://127.0.0.1:8080",
		)
		if err != nil {
			t.Fatalf("%s should allow loopback host, got: %v", provider, err)
		}
	}
}

func TestSetCatalogCache_BoundedSize(t *testing.T) {
	modelCatalogMu.Lock()
	modelCatalogByKey = make(map[string]catalogCacheEntry)
	modelCatalogMu.Unlock()

	for i := 0; i < modelCatalogCacheMaxEntries+32; i++ {
		setCatalogCache(fmt.Sprintf("k-%d", i), []string{"m"}, "static", "")
	}

	modelCatalogMu.Lock()
	defer modelCatalogMu.Unlock()
	if len(modelCatalogByKey) > modelCatalogCacheMaxEntries {
		t.Fatalf("cache size should be <= %d, got %d", modelCatalogCacheMaxEntries, len(modelCatalogByKey))
	}
}
