package skillagent

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestTokenUsageFromMessageReadsResponseMeta(t *testing.T) {
	msg := &schema.Message{
		ResponseMeta: &schema.ResponseMeta{
			Usage: &schema.TokenUsage{
				PromptTokens:     12,
				CompletionTokens: 8,
				TotalTokens:      20,
			},
		},
	}

	got := tokenUsageFromMessage(msg)
	if got.PromptTokens != 12 || got.CompletionTokens != 8 || got.TotalTokens != 20 {
		t.Fatalf("unexpected token usage: %+v", got)
	}
}

func TestTokenUsageFromMessageReadsExtra(t *testing.T) {
	msg := &schema.Message{
		Extra: map[string]any{
			"usage": map[string]any{
				"prompt_tokens":     7,
				"completion_tokens": 3,
			},
		},
	}

	got := tokenUsageFromMessage(msg)
	if got.PromptTokens != 7 || got.CompletionTokens != 3 || got.TotalTokens != 10 {
		t.Fatalf("unexpected token usage: %+v", got)
	}
}

func TestNormalizeTokenUsageFillsTotal(t *testing.T) {
	got := normalizeTokenUsage(TokenUsage{PromptTokens: 12, CompletionTokens: 8})
	if got.TotalTokens != 20 {
		t.Fatalf("expected total tokens to be filled, got=%+v", got)
	}
}
