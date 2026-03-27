package openclaw

import "testing"

func TestRestoreInjectedOpenclawBotStateRemovesInjectedEntries(t *testing.T) {
	rawConfig := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model": map[string]interface{}{
					"primary": "clawdsecbot-minimax/MiniMax-M2.7",
					"fallbacks": []interface{}{
						"minimax/MiniMax-M2.5",
						"clawdsecbot-openai/Pro/zai-org/GLM-4.7",
						"clawdsecbot-zai/glm-4.7",
					},
				},
				"models": map[string]interface{}{
					"clawdsecbot-minimax/MiniMax-M2.7":       map[string]interface{}{},
					"clawdsecbot-openai/Pro/zai-org/GLM-4.7": map[string]interface{}{},
					"clawdsecbot-zai/glm-4.7":                map[string]interface{}{},
					"minimax/MiniMax-M2.5":                   map[string]interface{}{"alias": "Minimax"},
					"anthropic/claude-sonnet-4-6":            map[string]interface{}{"alias": "Sonnet"},
				},
			},
		},
		"models": map[string]interface{}{
			"providers": map[string]interface{}{
				"clawdsecbot-minimax": map[string]interface{}{"baseUrl": "http://127.0.0.1:13436"},
				"clawdsecbot-openai":  map[string]interface{}{"baseUrl": "http://127.0.0.1:13436"},
				"clawdsecbot-zai":     map[string]interface{}{"baseUrl": "http://127.0.0.1:13437"},
				"minimax":             map[string]interface{}{"baseUrl": "https://api.minimax.io/anthropic"},
			},
		},
	}

	targetPrimary, removedProviders, removedModels, removedFallbacks, changed, err := restoreInjectedOpenclawBotState(
		rawConfig,
		"clawdsecbot-minimax/MiniMax-M2.7",
	)
	if err != nil {
		t.Fatalf("restoreInjectedOpenclawBotState returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected changes to be applied")
	}
	if targetPrimary != "minimax/MiniMax-M2.5" {
		t.Fatalf("expected primary model to restore to minimax/MiniMax-M2.5, got %s", targetPrimary)
	}

	if len(removedProviders) != 3 {
		t.Fatalf("expected 3 injected providers removed, got %v", removedProviders)
	}
	if len(removedModels) != 3 {
		t.Fatalf("expected 3 injected models removed, got %v", removedModels)
	}
	if len(removedFallbacks) != 2 {
		t.Fatalf("expected 2 injected fallbacks removed, got %v", removedFallbacks)
	}

	agentsMap := rawConfig["agents"].(map[string]interface{})
	defaultsMap := agentsMap["defaults"].(map[string]interface{})
	modelMap := defaultsMap["model"].(map[string]interface{})
	if got := modelMap["primary"].(string); got != "minimax/MiniMax-M2.5" {
		t.Fatalf("expected updated primary model, got %s", got)
	}

	fallbacks := readStringSlice(modelMap["fallbacks"])
	if len(fallbacks) != 1 || fallbacks[0] != "minimax/MiniMax-M2.5" {
		t.Fatalf("unexpected restored fallbacks: %v", fallbacks)
	}

	models := defaultsMap["models"].(map[string]interface{})
	if _, exists := models["clawdsecbot-minimax/MiniMax-M2.7"]; exists {
		t.Fatal("expected injected model to be removed from agents.defaults.models")
	}
	if _, exists := models["minimax/MiniMax-M2.5"]; !exists {
		t.Fatal("expected original model to remain in agents.defaults.models")
	}

	providers := rawConfig["models"].(map[string]interface{})["providers"].(map[string]interface{})
	if _, exists := providers["clawdsecbot-minimax"]; exists {
		t.Fatal("expected injected provider to be removed from models.providers")
	}
	if _, exists := providers["minimax"]; !exists {
		t.Fatal("expected original provider to remain in models.providers")
	}
}

func TestRestoreInjectedOpenclawBotStateNoOpWhenAlreadyDefault(t *testing.T) {
	rawConfig := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model": map[string]interface{}{
					"primary": "minimax/MiniMax-M2.5",
					"fallbacks": []interface{}{
						"anthropic/claude-sonnet-4-6",
					},
				},
				"models": map[string]interface{}{
					"anthropic/claude-sonnet-4-6": map[string]interface{}{},
					"minimax/MiniMax-M2.5":        map[string]interface{}{},
				},
			},
		},
		"models": map[string]interface{}{
			"providers": map[string]interface{}{
				"anthropic": map[string]interface{}{},
				"minimax":   map[string]interface{}{},
			},
		},
	}

	targetPrimary, removedProviders, removedModels, removedFallbacks, changed, err := restoreInjectedOpenclawBotState(
		rawConfig,
		"minimax/MiniMax-M2.5",
	)
	if err != nil {
		t.Fatalf("restoreInjectedOpenclawBotState returned error: %v", err)
	}
	if changed {
		t.Fatal("expected no config mutation when already in default state")
	}
	if targetPrimary != "minimax/MiniMax-M2.5" {
		t.Fatalf("expected primary to stay unchanged, got %s", targetPrimary)
	}
	if len(removedProviders) != 0 || len(removedModels) != 0 || len(removedFallbacks) != 0 {
		t.Fatalf("expected no injected entries to be removed, got providers=%v models=%v fallbacks=%v", removedProviders, removedModels, removedFallbacks)
	}
}
