package skillagent

import (
	"encoding/json"

	"github.com/cloudwego/eino/schema"
)

// TokenUsage records model token usage for SkillAgent execution.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

func normalizeTokenUsage(usage TokenUsage) TokenUsage {
	if usage.PromptTokens < 0 {
		usage.PromptTokens = 0
	}
	if usage.CompletionTokens < 0 {
		usage.CompletionTokens = 0
	}
	if usage.TotalTokens < 0 {
		usage.TotalTokens = 0
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return usage
}

func tokenUsageFromMessage(msg *schema.Message) TokenUsage {
	if msg == nil {
		return TokenUsage{}
	}
	if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
		usage := normalizeTokenUsage(TokenUsage{
			PromptTokens:     msg.ResponseMeta.Usage.PromptTokens,
			CompletionTokens: msg.ResponseMeta.Usage.CompletionTokens,
			TotalTokens:      msg.ResponseMeta.Usage.TotalTokens,
		})
		if usage.TotalTokens > 0 {
			return usage
		}
	}
	return tokenUsageFromExtra(msg.Extra)
}

func tokenUsageFromExtra(extra map[string]any) TokenUsage {
	if extra == nil {
		return TokenUsage{}
	}
	usageVal, ok := extra["usage"]
	if !ok {
		usageVal, ok = extra["Usage"]
	}
	if !ok {
		return TokenUsage{}
	}
	if usageMap, ok := usageVal.(map[string]any); ok {
		return normalizeTokenUsage(TokenUsage{
			PromptTokens:     getIntFromMap(usageMap, "prompt_tokens"),
			CompletionTokens: getIntFromMap(usageMap, "completion_tokens"),
			TotalTokens:      getIntFromMap(usageMap, "total_tokens"),
		})
	}
	if jsonBytes, err := json.Marshal(usageVal); err == nil {
		var usage TokenUsage
		if err := json.Unmarshal(jsonBytes, &usage); err == nil {
			return normalizeTokenUsage(usage)
		}
	}
	return TokenUsage{}
}

func getIntFromMap(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func addTokenUsage(left, right TokenUsage) TokenUsage {
	left = normalizeTokenUsage(left)
	right = normalizeTokenUsage(right)
	return normalizeTokenUsage(TokenUsage{
		PromptTokens:     left.PromptTokens + right.PromptTokens,
		CompletionTokens: left.CompletionTokens + right.CompletionTokens,
		TotalTokens:      left.TotalTokens + right.TotalTokens,
	})
}

func estimateSkillAgentUsage(instruction, userMessage, output string) TokenUsage {
	promptTokens := estimateTokenCount(instruction) + estimateTokenCount(userMessage)
	completionTokens := estimateTokenCount(output)
	return normalizeTokenUsage(TokenUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	})
}

func estimateTokenCount(text string) int {
	if text == "" {
		return 0
	}
	tokenCount := 0.0
	for _, r := range text {
		if r < 128 {
			tokenCount += 0.25
		} else {
			tokenCount += 1.5
		}
	}
	count := int(tokenCount)
	if tokenCount > float64(count) {
		count++
	}
	if count == 0 && len(text) > 0 {
		return 1
	}
	return count
}
