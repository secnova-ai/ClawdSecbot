package modelfactory

import (
	"strings"
	"testing"

	"go_lib/core/repository"
)

func TestValidateSecurityModelConfig_ARKRequiresModelNotEndpoint(t *testing.T) {
	cfg := &repository.SecurityModelConfig{
		Provider: "ark",
		APIKey:   "test-key",
		Model:    "",
		Endpoint: "",
	}

	err := ValidateSecurityModelConfig(cfg)
	if err == nil {
		t.Fatal("expected error when ARK model is empty")
	}
	if !strings.Contains(err.Error(), "ARK model name is required") {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg.Model = "ep-123456"
	err = ValidateSecurityModelConfig(cfg)
	if err != nil {
		t.Fatalf("expected ARK config to be valid without endpoint, got: %v", err)
	}
}
