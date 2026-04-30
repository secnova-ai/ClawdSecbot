package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertRequestRawMovesSystemMessageAndSetsDefaults(t *testing.T) {
	provider := New("test-key")
	body := []byte(`{
		"model":"claude-test",
		"messages":[
			{"role":"system","content":"follow policy"},
			{"role":"user","content":"hello"}
		]
	}`)

	converted, err := provider.convertRequestRaw(body, false)
	if err != nil {
		t.Fatalf("convertRequestRaw failed: %v", err)
	}

	if got := gjson.GetBytes(converted, "system").String(); got != "follow policy" {
		t.Fatalf("Expected system prompt to be moved, got %q in %s", got, converted)
	}
	if got := gjson.GetBytes(converted, "messages.0.role").String(); got != "user" {
		t.Fatalf("Expected first Anthropic message to be user, got %q in %s", got, converted)
	}
	if got := gjson.GetBytes(converted, "max_tokens").Int(); got == 0 {
		t.Fatalf("Expected max_tokens default, got %d in %s", got, converted)
	}
}

func TestConvertRequestRawEnablesStream(t *testing.T) {
	provider := New("test-key")

	converted, err := provider.convertRequestRaw([]byte(`{"model":"claude-test","messages":[]}`), true)
	if err != nil {
		t.Fatalf("convertRequestRaw failed: %v", err)
	}
	if got := gjson.GetBytes(converted, "stream").Bool(); !got {
		t.Fatalf("Expected stream=true in %s", converted)
	}
}

func TestProviderBaseURLAccessors(t *testing.T) {
	provider := New("test-key")
	if provider.DefaultBaseURL() != defaultBaseURL {
		t.Fatalf("Expected default base URL %q, got %q", defaultBaseURL, provider.DefaultBaseURL())
	}
	if provider.GetBaseURL() != defaultBaseURL {
		t.Fatalf("Expected initial base URL %q, got %q", defaultBaseURL, provider.GetBaseURL())
	}

	customURL := "https://example.test/v1/messages"
	provider.SetBaseURL(customURL)
	if provider.GetBaseURL() != customURL {
		t.Fatalf("Expected custom base URL %q, got %q", customURL, provider.GetBaseURL())
	}
}

func TestConvertResponseMapsAnthropicMessage(t *testing.T) {
	provider := New("test-key")
	resp, err := provider.convertResponse([]byte(`{
		"id":"msg_test",
		"type":"message",
		"model":"claude-test",
		"role":"assistant",
		"content":[{"type":"text","text":"hello"}],
		"stop_reason":"end_turn",
		"usage":{"input_tokens":3,"output_tokens":4}
	}`))
	if err != nil {
		t.Fatalf("convertResponse failed: %v", err)
	}
	if resp.ID != "msg_test" {
		t.Fatalf("Expected response ID msg_test, got %q", resp.ID)
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "hello" {
		body, _ := json.Marshal(resp)
		t.Fatalf("Expected assistant content hello, got %s", body)
	}
}

func TestAnthropicInputTokensIncludesPromptCacheUsage(t *testing.T) {
	usage := gjson.Parse(`{
		"input_tokens": 50,
		"cache_creation_input_tokens": 200,
		"cache_read_input_tokens": 100000,
		"output_tokens": 12
	}`)

	if got := anthropicInputTokens(usage); got != 100250 {
		t.Fatalf("expected total input tokens 100250, got %d", got)
	}
}

func TestConvertResponse_IncludesPromptCacheUsage(t *testing.T) {
	p := New("test")
	resp, err := p.convertResponse([]byte(`{
		"id": "msg_1",
		"model": "claude-test",
		"role": "assistant",
		"content": [{"type": "text", "text": "done"}],
		"stop_reason": "end_turn",
		"usage": {
			"input_tokens": 50,
			"cache_creation_input_tokens": 200,
			"cache_read_input_tokens": 100000,
			"output_tokens": 12
		}
	}`))
	if err != nil {
		t.Fatalf("convertResponse failed: %v", err)
	}

	if resp.Usage.PromptTokens != 100250 {
		t.Fatalf("expected prompt tokens 100250, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 12 {
		t.Fatalf("expected completion tokens 12, got %d", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 100262 {
		t.Fatalf("expected total tokens 100262, got %d", resp.Usage.TotalTokens)
	}
}

func TestAnthropicStream_IncludesPromptCacheUsage(t *testing.T) {
	stream := &anthropicStream{}
	if _, err := stream.processEvent("message_start", `{
		"message": {
			"id": "msg_1",
			"model": "claude-test",
			"usage": {
				"input_tokens": 50,
				"cache_creation_input_tokens": 200,
				"cache_read_input_tokens": 100000
			}
		}
	}`); err != nil {
		t.Fatalf("message_start failed: %v", err)
	}

	chunk, err := stream.processEvent("message_delta", `{
		"delta": {"stop_reason": "end_turn"},
		"usage": {"output_tokens": 12}
	}`)
	if err != nil {
		t.Fatalf("message_delta failed: %v", err)
	}
	if chunk == nil {
		t.Fatalf("expected usage chunk")
	}
	if chunk.Usage.PromptTokens != 100250 {
		t.Fatalf("expected prompt tokens 100250, got %d", chunk.Usage.PromptTokens)
	}
	if chunk.Usage.TotalTokens != 100262 {
		t.Fatalf("expected total tokens 100262, got %d", chunk.Usage.TotalTokens)
	}
}
