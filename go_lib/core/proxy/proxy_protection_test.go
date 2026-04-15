package proxy

import (
	"fmt"
	"strings"
	"testing"
)

func boolPtr(v bool) *bool {
	return &v
}

func TestBuildReActSkillRuntimeConfig_Defaults(t *testing.T) {
	cfg := buildReActSkillRuntimeConfig(nil)
	if cfg == nil {
		t.Fatalf("expected config")
	}
	if !cfg.EnableBuiltinSkills {
		t.Fatalf("expected default EnableBuiltinSkills=true")
	}
}

func TestBuildReActSkillRuntimeConfig_FromRuntime(t *testing.T) {
	runtime := &ProtectionRuntimeConfig{
		ReActEnableBuiltinSkills: boolPtr(false),
	}
	cfg := buildReActSkillRuntimeConfig(runtime)
	if cfg.EnableBuiltinSkills {
		t.Fatalf("expected EnableBuiltinSkills=false")
	}
}

// TestFormatQuotaExceededMessage_SessionZh verifies Chinese session quota exceeded message
func TestFormatQuotaExceededMessage_SessionZh(t *testing.T) {
	msg := formatQuotaExceededMessage("session", 5000, 5000)
	if !strings.Contains(msg, "[ClawSecbot]") {
		t.Errorf("Expected [ClawSecbot] prefix, got: %s", msg)
	}
	if !strings.Contains(msg, "QUOTA_EXCEEDED") {
		t.Errorf("Expected QUOTA_EXCEEDED status, got: %s", msg)
	}
	if !strings.Contains(msg, "5000/5000") {
		t.Errorf("Expected usage info 5000/5000, got: %s", msg)
	}
}

// TestFormatQuotaExceededMessage_DailyZh verifies Chinese daily quota exceeded message
func TestFormatQuotaExceededMessage_DailyZh(t *testing.T) {
	msg := formatQuotaExceededMessage("daily", 10000, 10000)
	if !strings.Contains(msg, "[ClawSecbot]") {
		t.Errorf("Expected [ClawSecbot] prefix, got: %s", msg)
	}
	if !strings.Contains(msg, "QUOTA_EXCEEDED") {
		t.Errorf("Expected QUOTA_EXCEEDED status, got: %s", msg)
	}
	if !strings.Contains(msg, "10000/10000") {
		t.Errorf("Expected usage info 10000/10000, got: %s", msg)
	}
}

// TestFormatQuotaExceededMessage_Format verifies message format contains key elements
func TestFormatQuotaExceededMessage_Format(t *testing.T) {
	tests := []struct {
		name      string
		quotaType string
		current   int
		limit     int
	}{
		{"session small", "session", 100, 100},
		{"conversation small", "conversation", 200, 300},
		{"daily large", "daily", 999999, 1000000},
		{"session zero", "session", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := formatQuotaExceededMessage(tt.quotaType, tt.current, tt.limit)
			if msg == "" {
				t.Fatal("Expected non-empty message")
			}
			if !strings.Contains(msg, "[ClawSecbot]") {
				t.Errorf("Missing [ClawSecbot] prefix in: %s", msg)
			}
			if !strings.Contains(msg, "QUOTA_EXCEEDED") {
				t.Errorf("Missing QUOTA_EXCEEDED in: %s", msg)
			}
			expectedUsage := fmt.Sprintf("%d/%d", tt.current, tt.limit)
			if !strings.Contains(msg, expectedUsage) {
				t.Errorf("Missing usage %s in: %s", expectedUsage, msg)
			}
		})
	}
}
