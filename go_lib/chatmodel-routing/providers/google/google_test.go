package google

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

// Unit Tests for Logic Verification (No API Call)
func TestConvertMessages_Logic(t *testing.T) {
	p := New("test")

	// Test Tool Call ID Mapping and Signature Decoding
	msgs := []map[string]interface{}{
		{
			"role":    "user",
			"content": "Hi",
		},
		{
			"role": "assistant",
			"tool_calls": []map[string]interface{}{
				{
					"id":   "call_123:::SIG:::signature_abc",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "my_func",
						"arguments": "{}",
					},
				},
			},
		},
		{
			"role":         "tool",
			"tool_call_id": "call_123:::SIG:::signature_abc",
			"content":      "result",
		},
	}

	msgBytes, _ := json.Marshal(msgs)
	parsed := gjson.ParseBytes(msgBytes)

	contents := p.convertMessages(parsed.Array())

	if len(contents) != 3 {
		t.Fatalf("Expected 3 contents, got %d", len(contents))
	}

	// Check Assistant Message (Index 1) - Signature Extraction
	asstParts := contents[1]["parts"].([]map[string]interface{})
	// funcCall := asstParts[0]["functionCall"].(map[string]interface{}) // No longer inside functionCall
	part := asstParts[0]
	if sig, ok := part["thoughtSignature"]; !ok || sig != "signature_abc" {
		t.Errorf("Expected signature 'signature_abc', got %v", sig)
	}

	// Check Tool Message (Index 2) - Name Mapping
	toolParts := contents[2]["parts"].([]map[string]interface{})
	funcResp := toolParts[0]["functionResponse"].(map[string]interface{})
	if name := funcResp["name"]; name != "my_func" {
		t.Errorf("Expected function name 'my_func', got %v", name)
	}
}

func TestConvertResponse_Logic(t *testing.T) {
	p := New("test")

	// Mock Gemini Response with Signature
	geminiResp := `{
		"candidates": [{
			"content": {
				"parts": [{
					"functionCall": {
						"name": "my_func",
						"args": {},
						"thoughtSignature": "new_sig_123"
					}
				}]
			}
		}]
	}`

	// We need to implement a dummy method or just test the logic indirectly if possible,
	// but convertResponse is exported (capitalized in my memory? No, it's lower case `convertResponse`).
	// Wait, in google.go it is `func (p *Provider) convertResponse(...)`.
	// Since I am in package `google`, I can call it.

	resp, err := p.convertResponse([]byte(geminiResp), "model")
	if err != nil {
		t.Fatalf("convertResponse failed: %v", err)
	}

	id := resp.Choices[0].Message.ToolCalls[0].ID
	if !strings.Contains(id, ":::SIG:::new_sig_123") {
		t.Errorf("Expected ID to contain signature, got %s", id)
	}
}

func TestConvertResponse_PreservesOfficialUsageTotal(t *testing.T) {
	p := New("test")

	geminiResp := `{
		"candidates": [{
			"content": {
				"parts": [{"text": "done"}]
			}
		}],
		"usageMetadata": {
			"promptTokenCount": 10,
			"candidatesTokenCount": 5,
			"thoughtsTokenCount": 3,
			"totalTokenCount": 18
		}
	}`

	resp, err := p.convertResponse([]byte(geminiResp), "model")
	if err != nil {
		t.Fatalf("convertResponse failed: %v", err)
	}

	if resp.Usage.PromptTokens != 10 {
		t.Fatalf("expected prompt tokens 10, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 5 {
		t.Fatalf("expected completion tokens 5, got %d", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 18 {
		t.Fatalf("expected provider total tokens 18, got %d", resp.Usage.TotalTokens)
	}
}

func TestStreamingID_Logic(t *testing.T) {
	// Need to test geminiStream.Recv logic which adds unique IDs and signature.
	// This is harder to mock without a real reader.
	// I'll skip this for unit test and rely on integration test or logic inspection.
	// But I can create a dummy reader.

	mockBody := `data: {"candidates": [{"content": {"parts": [{"functionCall": {"name": "foo", "args": {}, "thoughtSignature": "sig1"}}]}}]}
`

	stream := &geminiStream{
		reader:        bufio.NewReader(strings.NewReader(mockBody)),
		body:          &dummyCloser{},
		model:         "model",
		id:            "id",
		toolCallIndex: 0,
	}

	chunk, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv failed: %v", err)
	}

	if len(chunk.Choices) == 0 {
		t.Fatal("No choices")
	}

	toolCall := chunk.Choices[0].Delta.ToolCalls[0]
	if !strings.Contains(toolCall.ID, ":::SIG:::sig1") {
		t.Errorf("Expected ID to contain signature, got %s", toolCall.ID)
	}
}

type dummyCloser struct{}

func (d *dummyCloser) Close() error                     { return nil }
func (d *dummyCloser) Read(p []byte) (n int, err error) { return 0, io.EOF }
