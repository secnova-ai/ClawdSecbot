package readyclaw

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go_lib/core"
)

func TestAssessRisksReportsUnsupportedProtocolWithoutOpenClawRisks(t *testing.T) {
	prevConfigOverride := readyclawConfigPathOverride
	t.Cleanup(func() { readyclawConfigPathOverride = prevConfigOverride })

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	readyclawConfigPathOverride = configPath

	writeReadyClawFullConfig(t, configPath, map[string]interface{}{
		"LLM_PROTOCOL":   "azure",
		"LLM_BASE_URL":   "https://api.example.com/v1/chat/completions",
		"LLM_MODEL_NAME": "gpt-5.4",
	})

	p := &Plugin{}
	risks, err := p.AssessRisks(nil, nil)
	if err != nil {
		t.Fatalf("AssessRisks returned error: %v", err)
	}

	if !hasReadyClawRisk(risks, "readyclaw_unsupported_llm_protocol") {
		t.Fatalf("expected unsupported protocol risk, got %#v", risks)
	}
	for _, risk := range risks {
		if strings.HasPrefix(risk.ID, "openclaw_") ||
			strings.HasPrefix(risk.ID, "gateway_") ||
			strings.HasPrefix(risk.ID, "sandbox_") {
			t.Fatalf("ReadyClaw should not emit OpenClaw-style risk %q", risk.ID)
		}
	}
}

func TestAssessRisksReportsMissingModelName(t *testing.T) {
	prevConfigOverride := readyclawConfigPathOverride
	t.Cleanup(func() { readyclawConfigPathOverride = prevConfigOverride })

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	readyclawConfigPathOverride = configPath

	writeReadyClawFullConfig(t, configPath, map[string]interface{}{
		"LLM_BASE_URL": "https://api.example.com/v1/chat/completions",
	})

	p := &Plugin{}
	risks, err := p.AssessRisks(nil, nil)
	if err != nil {
		t.Fatalf("AssessRisks returned error: %v", err)
	}
	if !hasReadyClawRisk(risks, "readyclaw_model_missing") {
		t.Fatalf("expected missing model risk, got %#v", risks)
	}
}

func TestAssessRisksIgnoresHealthyReadyClawConfig(t *testing.T) {
	prevConfigOverride := readyclawConfigPathOverride
	t.Cleanup(func() { readyclawConfigPathOverride = prevConfigOverride })

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	readyclawConfigPathOverride = configPath

	writeReadyClawFullConfig(t, configPath, map[string]interface{}{
		"LLM_BASE_URL":   "https://api.example.com/v1/chat/completions",
		"LLM_MODEL_NAME": "gpt-5.4",
	})

	p := &Plugin{}
	risks, err := p.AssessRisks(nil, nil)
	if err != nil {
		t.Fatalf("AssessRisks returned error: %v", err)
	}
	if len(risks) != 0 {
		t.Fatalf("expected no risks for healthy ReadyClaw config, got %#v", risks)
	}
}

func TestAssessRisksReportsUnreadableConfig(t *testing.T) {
	prevConfigOverride := readyclawConfigPathOverride
	t.Cleanup(func() { readyclawConfigPathOverride = prevConfigOverride })

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	readyclawConfigPathOverride = configPath
	if err := os.WriteFile(configPath, []byte("{"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	p := &Plugin{}
	risks, err := p.AssessRisks(nil, nil)
	if err != nil {
		t.Fatalf("AssessRisks returned error: %v", err)
	}
	if !hasReadyClawRisk(risks, "readyclaw_config_unreadable") {
		t.Fatalf("expected unreadable config risk, got %#v", risks)
	}
}

func hasReadyClawRisk(risks []core.Risk, id string) bool {
	for _, risk := range risks {
		if risk.ID == id {
			return true
		}
	}
	return false
}
