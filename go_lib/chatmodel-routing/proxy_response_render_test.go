package chatmodelrouting

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"go_lib/chatmodel-routing/adapter"

	"github.com/openai/openai-go"
)

type testStream struct {
	chunks []*openai.ChatCompletionChunk
	index  int
}

type bodyCaptureProvider struct {
	body []byte
}

func (p *bodyCaptureProvider) ChatCompletion(ctx context.Context, req *openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return nil, nil
}

func (p *bodyCaptureProvider) ChatCompletionRaw(ctx context.Context, body []byte) (*openai.ChatCompletion, error) {
	p.body = append([]byte(nil), body...)
	return &openai.ChatCompletion{}, nil
}

func (p *bodyCaptureProvider) ChatCompletionStream(ctx context.Context, req *openai.ChatCompletionNewParams) (adapter.Stream, error) {
	return nil, nil
}

func (p *bodyCaptureProvider) ChatCompletionStreamRaw(ctx context.Context, body []byte) (adapter.Stream, error) {
	p.body = append([]byte(nil), body...)
	return &testStream{}, nil
}

func (s *testStream) Recv() (*openai.ChatCompletionChunk, error) {
	if s.index >= len(s.chunks) {
		return nil, io.EOF
	}
	chunk := s.chunks[s.index]
	s.index++
	return chunk, nil
}

func (s *testStream) Close() error {
	return nil
}

func TestServeHTTP_UsesForwardBodyFromFilter(t *testing.T) {
	originalBody := `{"model":"gpt-test","messages":[{"role":"user","content":"secret"}]}`
	forwardBody := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"redacted"}]}`)
	provider := &bodyCaptureProvider{}
	filter := NewCallbackFilter(
		func(ctx context.Context, req *openai.ChatCompletionNewParams, rawBody []byte) (*FilterRequestResult, bool) {
			if !strings.Contains(string(rawBody), "secret") {
				t.Fatalf("expected filter to see original body, got %s", string(rawBody))
			}
			return &FilterRequestResult{ForwardBody: forwardBody}, true
		},
		nil,
		nil,
	)
	p, err := NewProxy(provider, filter)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(originalBody))
	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := string(provider.body); got != string(forwardBody) {
		t.Fatalf("expected provider to receive rewritten body %s, got %s", string(forwardBody), got)
	}
}

func TestServeNonStreamResponse_RewritesRawToolCallIDWithMutatedResponse(t *testing.T) {
	raw := `{
	  "id":"chatcmpl_test",
	  "object":"chat.completion",
	  "created":1,
	  "model":"gpt-test",
	  "choices":[
	    {
	      "index":0,
	      "message":{
	        "role":"assistant",
	        "content":"",
	        "reasoning_content":"keep_me",
	        "tool_calls":[
	          {"id":"","type":"function","function":{"name":"search","arguments":"{}"}}
	        ]
	      },
	      "finish_reason":"tool_calls"
	    }
	  ],
	  "usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
	}`

	var resp openai.ChatCompletion
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	resp.Choices[0].Message.ToolCalls[0].ID = "tc1"

	p := &Proxy{}
	rr := httptest.NewRecorder()
	p.serveNonStreamResponse(context.Background(), &resp, rr)

	body := rr.Body.String()
	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(body, `"id":"tc1"`) {
		t.Fatalf("expected rewritten tool_call_id in response body, got: %s", body)
	}
	if !strings.Contains(body, `"reasoning_content":"keep_me"`) {
		t.Fatalf("expected extra field from raw response to be preserved, got: %s", body)
	}
}

func TestServeNonStreamResponse_RewritesRawAssistantContentWithMutatedResponse(t *testing.T) {
	raw := `{
	  "id":"chatcmpl_test",
	  "object":"chat.completion",
	  "created":1,
	  "model":"gpt-test",
	  "choices":[
	    {
	      "index":0,
	      "message":{
	        "role":"assistant",
	        "content":"secret",
	        "reasoning_content":"keep_me"
	      },
	      "finish_reason":"stop"
	    }
	  ],
	  "usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
	}`

	var resp openai.ChatCompletion
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	resp.Choices[0].Message.Content = "redacted"

	p := &Proxy{}
	rr := httptest.NewRecorder()
	p.serveNonStreamResponse(context.Background(), &resp, rr)

	body := rr.Body.String()
	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if strings.Contains(body, `"content":"secret"`) {
		t.Fatalf("expected raw assistant content to be rewritten, got: %s", body)
	}
	if !strings.Contains(body, `"content":"redacted"`) {
		t.Fatalf("expected redacted assistant content in response body, got: %s", body)
	}
	if !strings.Contains(body, `"reasoning_content":"keep_me"`) {
		t.Fatalf("expected extra field from raw response to be preserved, got: %s", body)
	}
}

func TestServeStreamResponse_RewritesRawToolCallIDWithMutatedChunk(t *testing.T) {
	raw := `{
	  "id":"chatcmpl_chunk",
	  "object":"chat.completion.chunk",
	  "created":1,
	  "model":"gpt-test",
	  "choices":[
	    {
	      "index":0,
	      "delta":{
	        "tool_calls":[
	          {"index":0,"id":"","type":"function","function":{"name":"search","arguments":"{}"}}
	        ],
	        "reasoning_content":"keep_chunk_field"
	      },
	      "finish_reason":""
	    }
	  ]
	}`

	var chunk openai.ChatCompletionChunk
	if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
		t.Fatalf("failed to unmarshal chunk: %v", err)
	}
	chunk.Choices[0].Delta.ToolCalls[0].ID = "tc2"

	p := &Proxy{}
	rr := httptest.NewRecorder()
	stream := &testStream{
		chunks: []*openai.ChatCompletionChunk{
			&chunk,
		},
	}
	p.serveStreamResponse(context.Background(), stream, rr)

	body := rr.Body.String()
	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(body, `"id":"tc2"`) {
		t.Fatalf("expected rewritten tool_call_id in stream body, got: %s", body)
	}
	if !strings.Contains(body, `"reasoning_content":"keep_chunk_field"`) {
		t.Fatalf("expected extra field from raw chunk to be preserved, got: %s", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected [DONE] terminator, got: %s", body)
	}
}

func TestServeStreamResponse_RewritesRawDeltaContentWithMutatedChunk(t *testing.T) {
	raw := `{
	  "id":"chatcmpl_chunk",
	  "object":"chat.completion.chunk",
	  "created":1,
	  "model":"gpt-test",
	  "choices":[
	    {
	      "index":0,
	      "delta":{
	        "content":"secret",
	        "reasoning_content":"keep_chunk_field"
	      },
	      "finish_reason":""
	    }
	  ]
	}`

	var chunk openai.ChatCompletionChunk
	if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
		t.Fatalf("failed to unmarshal chunk: %v", err)
	}
	chunk.Choices[0].Delta.Content = "redacted"

	p := &Proxy{}
	rr := httptest.NewRecorder()
	stream := &testStream{
		chunks: []*openai.ChatCompletionChunk{
			&chunk,
		},
	}
	p.serveStreamResponse(context.Background(), stream, rr)

	body := rr.Body.String()
	if rr.Code != 200 {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if strings.Contains(body, `"content":"secret"`) {
		t.Fatalf("expected raw stream content to be rewritten, got: %s", body)
	}
	if !strings.Contains(body, `"content":"redacted"`) {
		t.Fatalf("expected redacted stream content in body, got: %s", body)
	}
	if !strings.Contains(body, `"reasoning_content":"keep_chunk_field"`) {
		t.Fatalf("expected extra field from raw chunk to be preserved, got: %s", body)
	}
}

func TestServeStreamResponse_DropsBufferedToolCallsWhenFilterBlocks(t *testing.T) {
	p := &Proxy{
		filter: NewCallbackFilter(nil, nil, func(ctx context.Context, chunk *openai.ChatCompletionChunk) bool {
			if !streamChunkHasFinishReason(chunk) {
				return true
			}
			chunk.Choices[0].Delta.Content = "blocked by ShepherdGate"
			chunk.Choices[0].Delta.ToolCalls = nil
			chunk.Choices[0].FinishReason = "stop"
			return false
		}),
	}
	rr := httptest.NewRecorder()
	stream := &testStream{
		chunks: []*openai.ChatCompletionChunk{
			{
				Choices: []openai.ChatCompletionChunkChoice{
					{
						Delta: openai.ChatCompletionChunkChoiceDelta{
							ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
								{
									Index: 0,
									ID:    "call_danger",
									Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
										Name:      "exec",
										Arguments: `{"command":"bash /Users/kidbei/Desktop/poc/test.sh secret"}`,
									},
								},
							},
						},
					},
				},
			},
			{
				Choices: []openai.ChatCompletionChunkChoice{
					{FinishReason: "tool_calls"},
				},
			},
		},
	}

	p.serveStreamResponse(context.Background(), stream, rr)

	body := rr.Body.String()
	if !strings.Contains(body, "blocked by ShepherdGate") {
		t.Fatalf("expected security response, got: %s", body)
	}
	if strings.Contains(body, "test.sh") || strings.Contains(body, "call_danger") || strings.Contains(body, `"name":"exec"`) {
		t.Fatalf("blocked stream leaked buffered tool_call chunks: %s", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected [DONE] terminator, got: %s", body)
	}
}

func TestServeStreamResponse_ToolCallBuffersAreRequestLocalUnderConcurrency(t *testing.T) {
	type contextKey string
	const blockKey contextKey = "block"

	p := &Proxy{
		filter: NewCallbackFilter(nil, nil, func(ctx context.Context, chunk *openai.ChatCompletionChunk) bool {
			if !streamChunkHasFinishReason(chunk) {
				return true
			}
			if block, _ := ctx.Value(blockKey).(bool); block {
				chunk.Choices[0].Delta.Content = "blocked by ShepherdGate"
				chunk.Choices[0].Delta.ToolCalls = nil
				chunk.Choices[0].FinishReason = "stop"
				return false
			}
			return true
		}),
	}

	blockedRecorder := httptest.NewRecorder()
	allowedRecorder := httptest.NewRecorder()
	blockedStream := &testStream{chunks: []*openai.ChatCompletionChunk{
		streamToolCallChunk("call_blocked", "exec", `{"command":"bash secret.sh"}`),
		streamFinishChunk("tool_calls"),
	}}
	allowedStream := &testStream{chunks: []*openai.ChatCompletionChunk{
		streamToolCallChunk("call_allowed", "search", `{"query":"public"}`),
		streamFinishChunk("tool_calls"),
	}}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		p.serveStreamResponse(context.WithValue(context.Background(), blockKey, true), blockedStream, blockedRecorder)
	}()
	go func() {
		defer wg.Done()
		p.serveStreamResponse(context.WithValue(context.Background(), blockKey, false), allowedStream, allowedRecorder)
	}()
	wg.Wait()

	blockedBody := blockedRecorder.Body.String()
	if !strings.Contains(blockedBody, "blocked by ShepherdGate") {
		t.Fatalf("expected blocked response, got: %s", blockedBody)
	}
	if strings.Contains(blockedBody, "secret.sh") || strings.Contains(blockedBody, "call_blocked") {
		t.Fatalf("blocked response leaked buffered tool call: %s", blockedBody)
	}

	allowedBody := allowedRecorder.Body.String()
	if !strings.Contains(allowedBody, "call_allowed") || !strings.Contains(allowedBody, `"name":"search"`) {
		t.Fatalf("expected allowed response to flush its own buffered tool call, got: %s", allowedBody)
	}
	if strings.Contains(allowedBody, "secret.sh") || strings.Contains(allowedBody, "call_blocked") {
		t.Fatalf("allowed response contains data from concurrent blocked request: %s", allowedBody)
	}
}

func streamToolCallChunk(id, name, arguments string) *openai.ChatCompletionChunk {
	return &openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Delta: openai.ChatCompletionChunkChoiceDelta{
					ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
						{
							Index: 0,
							ID:    id,
							Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
								Name:      name,
								Arguments: arguments,
							},
						},
					},
				},
			},
		},
	}
}

func streamFinishChunk(reason string) *openai.ChatCompletionChunk {
	return &openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{FinishReason: reason},
		},
	}
}
