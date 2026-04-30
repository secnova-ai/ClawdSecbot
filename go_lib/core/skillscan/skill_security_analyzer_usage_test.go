package skillscan

import (
	"errors"
	"testing"

	"go_lib/skillagent"
)

func TestAttachSkillAgentUsage(t *testing.T) {
	result := &SkillAnalysisResult{}
	execution := &skillagent.ExecutionResult{
		PromptTokens:     21,
		CompletionTokens: 9,
		TokensUsed:       30,
	}

	attachSkillAgentUsage(result, execution)

	if result.PromptTokens != 21 || result.CompletionTokens != 9 || result.TotalTokens != 30 {
		t.Fatalf("expected skill scan usage to be attached, got=%+v", result)
	}
}

func TestUsageFromAnalysisError(t *testing.T) {
	err := skillAnalysisUsageError("failed", errors.New("boom"), &skillagent.ExecutionResult{
		PromptTokens:     13,
		CompletionTokens: 5,
		TokensUsed:       18,
	})

	promptTokens, completionTokens, totalTokens, ok := UsageFromAnalysisError(err)
	if !ok || promptTokens != 13 || completionTokens != 5 || totalTokens != 18 {
		t.Fatalf("expected usage from analysis error, got ok=%v prompt=%d completion=%d total=%d", ok, promptTokens, completionTokens, totalTokens)
	}
}
