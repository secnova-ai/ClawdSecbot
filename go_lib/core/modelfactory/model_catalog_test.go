package modelfactory

import (
	"strings"
	"testing"
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
