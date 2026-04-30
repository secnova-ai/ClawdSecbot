package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go_lib/chatmodel-routing/adapter"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared/constant"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const defaultBaseURL = "https://api.anthropic.com/v1/messages"

// Provider implements the adapter.Provider interface for Anthropic Claude.
// It converts between OpenAI-compatible format and Anthropic's native Messages API.
type Provider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	version    string
}

// New creates a new Anthropic provider with the given API key.
func New(apiKey string) *Provider {
	return &Provider{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{},
		version:    "2023-06-01",
	}
}

// DefaultBaseURL returns the default base URL for Anthropic.
func (p *Provider) DefaultBaseURL() string {
	return defaultBaseURL
}

// GetBaseURL returns the current base URL.
func (p *Provider) GetBaseURL() string {
	return p.baseURL
}

// SetBaseURL sets a custom base URL for the provider.
func (p *Provider) SetBaseURL(url string) {
	p.baseURL = url
}

// SetHTTPClient sets a custom HTTP client for the provider.
func (p *Provider) SetHTTPClient(client *http.Client) {
	p.httpClient = client
}

// Ensure Provider implements ProviderWithBaseURL interface.
var _ adapter.ProviderWithBaseURL = (*Provider)(nil)

// ============ Interface Methods ============

// ChatCompletion handles a non-streaming chat completion request.
func (p *Provider) ChatCompletion(ctx context.Context, req *openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	return p.ChatCompletionRaw(ctx, reqBytes)
}

// ChatCompletionRaw handles a non-streaming chat completion request with raw JSON body.
func (p *Provider) ChatCompletionRaw(ctx context.Context, body []byte) (*openai.ChatCompletion, error) {
	anthropicReq, err := p.convertRequestRaw(body, false)
	if err != nil {
		return nil, fmt.Errorf("failed to convert request: %w", err)
	}

	respBody, err := p.sendRequest(ctx, anthropicReq)
	if err != nil {
		return nil, err
	}

	return p.convertResponse(respBody)
}

// ChatCompletionStream handles a streaming chat completion request.
func (p *Provider) ChatCompletionStream(ctx context.Context, req *openai.ChatCompletionNewParams) (adapter.Stream, error) {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	return p.ChatCompletionStreamRaw(ctx, reqBytes)
}

// ChatCompletionStreamRaw handles a streaming chat completion request with raw JSON body.
func (p *Provider) ChatCompletionStreamRaw(ctx context.Context, body []byte) (adapter.Stream, error) {
	anthropicReq, err := p.convertRequestRaw(body, true)
	if err != nil {
		return nil, fmt.Errorf("failed to convert request: %w", err)
	}

	resp, err := p.sendStreamRequest(ctx, anthropicReq)
	if err != nil {
		return nil, err
	}

	return &anthropicStream{
		reader: bufio.NewReader(resp.Body),
		body:   resp.Body,
	}, nil
}

// ============ HTTP Helpers ============

func (p *Provider) setHeaders(req *http.Request) {
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", p.version)
	req.Header.Set("content-type", "application/json")
}

func (p *Provider) sendRequest(ctx context.Context, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.setHeaders(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic api error: %d - %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (p *Provider) sendStreamRequest(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.setHeaders(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic api error: %d - %s", resp.StatusCode, string(respBody))
	}

	return resp, nil
}

// ============ Request Conversion ============

// convertRequestRaw converts an OpenAI-format request to Anthropic Messages API format.
func (p *Provider) convertRequestRaw(reqBytes []byte, stream bool) ([]byte, error) {
	parsed := gjson.ParseBytes(reqBytes)

	model := parsed.Get("model").String()
	maxTokens := parsed.Get("max_tokens").Int()
	if maxTokens == 0 {
		maxTokens = int64(adapter.GetModelMaxOutputTokens(model))
	}

	// Extract system message and convert messages
	system := p.extractSystemMessage(parsed.Get("messages").Array())
	messages := p.convertMessages(parsed.Get("messages").Array())

	anthropicReq := map[string]interface{}{
		"model":      model,
		"max_tokens": maxTokens,
		"messages":   messages,
	}

	if stream {
		anthropicReq["stream"] = true
	}

	if system != "" {
		anthropicReq["system"] = system
	}

	// Forward optional parameters
	if temp := parsed.Get("temperature"); temp.Exists() {
		anthropicReq["temperature"] = temp.Float()
	}
	if topP := parsed.Get("top_p"); topP.Exists() {
		anthropicReq["top_p"] = topP.Float()
	}
	if stop := parsed.Get("stop"); stop.Exists() {
		if stop.IsArray() {
			var stops []string
			for _, s := range stop.Array() {
				stops = append(stops, s.String())
			}
			anthropicReq["stop_sequences"] = stops
		} else if stop.String() != "" {
			anthropicReq["stop_sequences"] = []string{stop.String()}
		}
	}

	// Convert tools
	if tools := parsed.Get("tools"); tools.Exists() && tools.IsArray() {
		anthropicTools := p.convertTools(tools.Array())
		if len(anthropicTools) > 0 {
			anthropicReq["tools"] = anthropicTools
		}
	}

	// Convert tool_choice
	if toolChoice := parsed.Get("tool_choice"); toolChoice.Exists() {
		if tc := p.convertToolChoice(toolChoice); tc != nil {
			anthropicReq["tool_choice"] = tc
		}
	}

	return json.Marshal(anthropicReq)
}

// extractSystemMessage collects all system messages and joins them.
func (p *Provider) extractSystemMessage(messages []gjson.Result) string {
	var parts []string
	for _, msg := range messages {
		if msg.Get("role").String() != "system" {
			continue
		}
		content := msg.Get("content")
		if content.IsArray() {
			for _, part := range content.Array() {
				if part.Get("type").String() == "text" {
					parts = append(parts, part.Get("text").String())
				}
			}
		} else if content.Exists() && content.String() != "" {
			parts = append(parts, content.String())
		}
	}
	return strings.Join(parts, "\n")
}

// convertMessages converts OpenAI messages to Anthropic format.
// Handles system extraction, tool message grouping, and role mapping.
func (p *Provider) convertMessages(messages []gjson.Result) []map[string]interface{} {
	var result []map[string]interface{}

	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		role := msg.Get("role").String()

		// Skip system messages (handled separately)
		if role == "system" {
			continue
		}

		if role == "tool" {
			// Collect consecutive tool messages into a single user message with tool_result blocks.
			// Anthropic requires tool results to be sent as a user message.
			var toolResults []map[string]interface{}
			for i < len(messages) && messages[i].Get("role").String() == "tool" {
				toolMsg := messages[i]
				toolResult := map[string]interface{}{
					"type":        "tool_result",
					"tool_use_id": toolMsg.Get("tool_call_id").String(),
				}
				toolContent := toolMsg.Get("content").String()
				if toolContent != "" {
					toolResult["content"] = toolContent
				}
				toolResults = append(toolResults, toolResult)
				i++
			}
			i-- // Compensate for outer loop increment

			result = append(result, map[string]interface{}{
				"role":    "user",
				"content": toolResults,
			})
			continue
		}

		if role == "assistant" {
			result = append(result, p.convertAssistantMessage(msg))
			continue
		}

		// User or other roles
		result = append(result, p.convertUserMessage(msg))
	}

	// Merge consecutive messages with the same role.
	// Anthropic requires strictly alternating user/assistant turns.
	return mergeConsecutiveMessages(result)
}

// convertAssistantMessage converts an OpenAI assistant message to Anthropic format.
// Handles both text content and tool_calls.
func (p *Provider) convertAssistantMessage(msg gjson.Result) map[string]interface{} {
	var contentBlocks []map[string]interface{}

	// Add text content
	content := msg.Get("content")
	if content.Exists() {
		if content.IsArray() {
			for _, part := range content.Array() {
				if part.Get("type").String() == "text" {
					contentBlocks = append(contentBlocks, map[string]interface{}{
						"type": "text",
						"text": part.Get("text").String(),
					})
				}
			}
		} else if content.String() != "" {
			contentBlocks = append(contentBlocks, map[string]interface{}{
				"type": "text",
				"text": content.String(),
			})
		}
	}

	// Convert tool_calls to tool_use content blocks
	if toolCalls := msg.Get("tool_calls"); toolCalls.Exists() && toolCalls.IsArray() {
		for _, tc := range toolCalls.Array() {
			var input interface{}
			if err := json.Unmarshal([]byte(tc.Get("function.arguments").String()), &input); err != nil {
				input = map[string]interface{}{}
			}

			contentBlocks = append(contentBlocks, map[string]interface{}{
				"type":  "tool_use",
				"id":    tc.Get("id").String(),
				"name":  tc.Get("function.name").String(),
				"input": input,
			})
		}
	}

	// Anthropic requires non-empty content
	if len(contentBlocks) == 0 {
		contentBlocks = append(contentBlocks, map[string]interface{}{
			"type": "text",
			"text": "",
		})
	}

	return map[string]interface{}{
		"role":    "assistant",
		"content": contentBlocks,
	}
}

// convertUserMessage converts an OpenAI user message to Anthropic format.
// Handles string content, array content with text and images.
func (p *Provider) convertUserMessage(msg gjson.Result) map[string]interface{} {
	content := msg.Get("content")

	if content.IsArray() {
		var contentBlocks []map[string]interface{}
		for _, part := range content.Array() {
			switch part.Get("type").String() {
			case "text":
				contentBlocks = append(contentBlocks, map[string]interface{}{
					"type": "text",
					"text": part.Get("text").String(),
				})
			case "image_url":
				imageURL := part.Get("image_url.url").String()
				if strings.HasPrefix(imageURL, "data:") {
					// Parse data URL: data:image/png;base64,xxxx
					dataParts := strings.SplitN(imageURL, ",", 2)
					if len(dataParts) == 2 {
						mimeType := strings.TrimPrefix(strings.Split(dataParts[0], ";")[0], "data:")
						contentBlocks = append(contentBlocks, map[string]interface{}{
							"type": "image",
							"source": map[string]interface{}{
								"type":       "base64",
								"media_type": mimeType,
								"data":       dataParts[1],
							},
						})
					}
				} else {
					// URL-based image
					contentBlocks = append(contentBlocks, map[string]interface{}{
						"type": "image",
						"source": map[string]interface{}{
							"type": "url",
							"url":  imageURL,
						},
					})
				}
			}
		}
		if len(contentBlocks) > 0 {
			return map[string]interface{}{
				"role":    "user",
				"content": contentBlocks,
			}
		}
	}

	// Simple string content
	return map[string]interface{}{
		"role":    "user",
		"content": content.String(),
	}
}

// convertTools converts OpenAI tools to Anthropic tools format.
func (p *Provider) convertTools(tools []gjson.Result) []map[string]interface{} {
	var result []map[string]interface{}
	for _, tool := range tools {
		if tool.Get("type").String() != "function" {
			continue
		}
		funcDef := tool.Get("function")
		anthropicTool := map[string]interface{}{
			"name": funcDef.Get("name").String(),
		}
		if desc := funcDef.Get("description"); desc.Exists() && desc.String() != "" {
			anthropicTool["description"] = desc.String()
		}
		if params := funcDef.Get("parameters"); params.Exists() {
			anthropicTool["input_schema"] = json.RawMessage(params.Raw)
		} else {
			// Anthropic requires input_schema even if empty
			anthropicTool["input_schema"] = map[string]interface{}{"type": "object"}
		}
		result = append(result, anthropicTool)
	}
	return result
}

// convertToolChoice converts OpenAI tool_choice to Anthropic format.
// OpenAI: "auto" | "none" | "required" | {"type":"function","function":{"name":"..."}}
// Anthropic: {"type":"auto"} | {"type":"any"} | {"type":"tool","name":"..."}
func (p *Provider) convertToolChoice(toolChoice gjson.Result) map[string]interface{} {
	if toolChoice.Type == gjson.String {
		switch toolChoice.String() {
		case "auto":
			return map[string]interface{}{"type": "auto"}
		case "required":
			return map[string]interface{}{"type": "any"}
		case "none":
			// Anthropic doesn't have a "none" tool_choice; omit it and remove tools instead.
			// Return nil to indicate no tool_choice should be set.
			return nil
		}
	}

	// Object form: {"type":"function","function":{"name":"func_name"}}
	if funcName := toolChoice.Get("function.name"); funcName.Exists() {
		return map[string]interface{}{
			"type": "tool",
			"name": funcName.String(),
		}
	}

	return nil
}

// mergeConsecutiveMessages merges consecutive messages with the same role.
// Anthropic requires strictly alternating user/assistant turns.
func mergeConsecutiveMessages(messages []map[string]interface{}) []map[string]interface{} {
	if len(messages) <= 1 {
		return messages
	}

	var result []map[string]interface{}
	for _, msg := range messages {
		if len(result) > 0 && result[len(result)-1]["role"] == msg["role"] {
			// Merge into previous message of the same role
			prev := result[len(result)-1]
			prevContent := ensureContentBlocks(prev["content"])
			currContent := ensureContentBlocks(msg["content"])
			prev["content"] = append(prevContent, currContent...)
		} else {
			result = append(result, msg)
		}
	}
	return result
}

// ensureContentBlocks converts content to a slice of content blocks.
// Handles both string content and existing content block arrays.
func ensureContentBlocks(content interface{}) []map[string]interface{} {
	switch c := content.(type) {
	case []map[string]interface{}:
		return c
	case string:
		if c == "" {
			return nil
		}
		return []map[string]interface{}{{"type": "text", "text": c}}
	default:
		return nil
	}
}

// ============ Response Conversion ============

// convertResponse converts an Anthropic Messages API response to OpenAI format.
func (p *Provider) convertResponse(body []byte) (*openai.ChatCompletion, error) {
	parsed := gjson.ParseBytes(body)

	// Check for error in response
	if errType := parsed.Get("error.type"); errType.Exists() {
		return nil, fmt.Errorf("anthropic error: %s - %s", errType.String(), parsed.Get("error.message").String())
	}

	id := parsed.Get("id").String()
	model := parsed.Get("model").String()
	stopReason := parsed.Get("stop_reason").String()

	// Build message from content blocks
	message := openai.ChatCompletionMessage{
		Role: constant.ValueOf[constant.Assistant](),
	}

	var textParts []string
	var reasoningParts []string
	var toolCalls []openai.ChatCompletionMessageToolCall

	for _, block := range parsed.Get("content").Array() {
		switch block.Get("type").String() {
		case "text":
			textParts = append(textParts, block.Get("text").String())
		case "thinking":
			reasoningParts = append(reasoningParts, block.Get("thinking").String())
		case "tool_use":
			argsJSON, _ := json.Marshal(block.Get("input").Value())
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCall{
				ID:   block.Get("id").String(),
				Type: "function",
				Function: openai.ChatCompletionMessageToolCallFunction{
					Name:      block.Get("name").String(),
					Arguments: string(argsJSON),
				},
			})
		}
	}

	if len(textParts) > 0 {
		message.Content = strings.Join(textParts, "")
	}
	if len(toolCalls) > 0 {
		message.ToolCalls = toolCalls
	}

	// Usage
	promptTokens := anthropicInputTokens(parsed.Get("usage"))
	completionTokens := parsed.Get("usage.output_tokens").Int()

	resp := &openai.ChatCompletion{
		ID:      id,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []openai.ChatCompletionChoice{
			{
				Index:        0,
				Message:      message,
				FinishReason: convertFinishReason(stopReason),
			},
		},
		Usage: openai.CompletionUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}

	// Inject reasoning_content if present (field not in openai-go struct, use JSON round-trip)
	if len(reasoningParts) > 0 {
		resultJSON, err := json.Marshal(resp)
		if err == nil {
			reasoningContent := strings.Join(reasoningParts, "")
			resultJSON, _ = sjson.SetBytes(resultJSON, "choices.0.message.reasoning_content", reasoningContent)
			var enriched openai.ChatCompletion
			if err := json.Unmarshal(resultJSON, &enriched); err == nil {
				resp = &enriched
			}
		}
	}

	return resp, nil
}

// convertFinishReason maps Anthropic stop_reason to OpenAI finish_reason.
func convertFinishReason(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "stop_sequence":
		return "stop"
	case "tool_use":
		return "tool_calls"
	default:
		return "stop"
	}
}

// ============ Streaming ============

// anthropicStream implements the adapter.Stream interface for Anthropic SSE streaming.
// It converts Anthropic's event-based SSE format to OpenAI ChatCompletionChunk format.
//
// Anthropic SSE events:
//   - message_start:       contains message id, model, input token usage
//   - content_block_start: starts a content block (text, thinking, or tool_use)
//   - content_block_delta: incremental content (text_delta, thinking_delta, input_json_delta)
//   - content_block_stop:  ends a content block
//   - message_delta:       contains stop_reason and output token usage
//   - message_stop:        signals end of message
//   - ping:                keepalive event
type anthropicStream struct {
	reader *bufio.Reader
	body   io.ReadCloser

	// State from message_start
	id    string
	model string

	// Content block tracking
	currentBlockType string // "text", "tool_use", "thinking"
	toolCallIndex    int    // tracks the index for OpenAI tool_calls array

	// Usage tracking
	inputTokens int64

	done bool
}

// Recv returns the next OpenAI-format chunk from the Anthropic SSE stream.
// Returns io.EOF when the stream is finished.
func (s *anthropicStream) Recv() (*openai.ChatCompletionChunk, error) {
	for {
		if s.done {
			return nil, io.EOF
		}

		eventType, data, err := s.readSSEEvent()
		if err != nil {
			return nil, err
		}

		chunk, err := s.processEvent(eventType, data)
		if err != nil {
			return nil, err
		}
		if chunk != nil {
			return chunk, nil
		}
		// chunk == nil means skip this event, continue reading next
	}
}

// Close closes the underlying response body.
func (s *anthropicStream) Close() error {
	return s.body.Close()
}

// readSSEEvent reads the next complete SSE event from the stream.
// Returns the event type and data payload.
func (s *anthropicStream) readSSEEvent() (eventType string, data string, err error) {
	var currentEvent, currentData string

	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// If we have partial event data, return it
				if currentData != "" {
					return currentEvent, currentData, nil
				}
				return "", "", io.EOF
			}
			return "", "", err
		}

		line = strings.TrimRight(line, "\r\n")

		// Empty line signals end of an SSE event
		if line == "" {
			if currentEvent != "" || currentData != "" {
				return currentEvent, currentData, nil
			}
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			currentData = strings.TrimPrefix(line, "data: ")
		}
		// Ignore other lines (e.g., comments starting with ":")
	}
}

// processEvent converts an Anthropic SSE event to an OpenAI ChatCompletionChunk.
// Returns nil chunk to indicate the event should be skipped.
func (s *anthropicStream) processEvent(eventType, data string) (*openai.ChatCompletionChunk, error) {
	parsed := gjson.Parse(data)

	switch eventType {
	case "message_start":
		s.id = parsed.Get("message.id").String()
		s.model = parsed.Get("message.model").String()
		s.inputTokens = anthropicInputTokens(parsed.Get("message.usage"))

		// Return initial chunk with assistant role
		return &openai.ChatCompletionChunk{
			ID:      s.id,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   s.model,
			Choices: []openai.ChatCompletionChunkChoice{
				{
					Index: 0,
					Delta: openai.ChatCompletionChunkChoiceDelta{
						Role: "assistant",
					},
				},
			},
		}, nil

	case "content_block_start":
		s.currentBlockType = parsed.Get("content_block.type").String()

		if s.currentBlockType == "tool_use" {
			// Send tool call start chunk with function name and ID
			return &openai.ChatCompletionChunk{
				ID:      s.id,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   s.model,
				Choices: []openai.ChatCompletionChunkChoice{
					{
						Index: 0,
						Delta: openai.ChatCompletionChunkChoiceDelta{
							ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
								{
									Index: int64(s.toolCallIndex),
									ID:    parsed.Get("content_block.id").String(),
									Type:  "function",
									Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
										Name:      parsed.Get("content_block.name").String(),
										Arguments: "",
									},
								},
							},
						},
					},
				},
			}, nil
		}

		// No chunk needed for text/thinking block start
		return nil, nil

	case "content_block_delta":
		return s.processContentDelta(parsed)

	case "content_block_stop":
		if s.currentBlockType == "tool_use" {
			s.toolCallIndex++
		}
		return nil, nil

	case "message_delta":
		stopReason := parsed.Get("delta.stop_reason").String()
		outputTokens := parsed.Get("usage.output_tokens").Int()

		return &openai.ChatCompletionChunk{
			ID:      s.id,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   s.model,
			Choices: []openai.ChatCompletionChunkChoice{
				{
					Index:        0,
					Delta:        openai.ChatCompletionChunkChoiceDelta{},
					FinishReason: convertFinishReason(stopReason),
				},
			},
			Usage: openai.CompletionUsage{
				PromptTokens:     s.inputTokens,
				CompletionTokens: outputTokens,
				TotalTokens:      s.inputTokens + outputTokens,
			},
		}, nil

	case "message_stop":
		s.done = true
		return nil, nil

	case "ping":
		return nil, nil

	case "error":
		errMsg := parsed.Get("error.message").String()
		if errMsg == "" {
			errMsg = data
		}
		return nil, fmt.Errorf("anthropic stream error: %s", errMsg)
	}

	// Unknown event type, skip
	return nil, nil
}

func anthropicInputTokens(usage gjson.Result) int64 {
	if !usage.Exists() {
		return 0
	}
	return usage.Get("input_tokens").Int() +
		usage.Get("cache_creation_input_tokens").Int() +
		usage.Get("cache_read_input_tokens").Int()
}

// processContentDelta handles content_block_delta events.
func (s *anthropicStream) processContentDelta(parsed gjson.Result) (*openai.ChatCompletionChunk, error) {
	deltaType := parsed.Get("delta.type").String()

	switch deltaType {
	case "text_delta":
		return &openai.ChatCompletionChunk{
			ID:      s.id,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   s.model,
			Choices: []openai.ChatCompletionChunkChoice{
				{
					Index: 0,
					Delta: openai.ChatCompletionChunkChoiceDelta{
						Content: parsed.Get("delta.text").String(),
					},
				},
			},
		}, nil

	case "thinking_delta":
		thinking := parsed.Get("delta.thinking").String()

		// Build base chunk then inject reasoning_content via JSON round-trip,
		// since ChatCompletionChunkChoiceDelta doesn't have a ReasoningContent field.
		chunk := &openai.ChatCompletionChunk{
			ID:      s.id,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   s.model,
			Choices: []openai.ChatCompletionChunkChoice{
				{Index: 0, Delta: openai.ChatCompletionChunkChoiceDelta{}},
			},
		}
		chunkJSON, err := json.Marshal(chunk)
		if err == nil {
			chunkJSON, _ = sjson.SetBytes(chunkJSON, "choices.0.delta.reasoning_content", thinking)
			var enriched openai.ChatCompletionChunk
			if err := json.Unmarshal(chunkJSON, &enriched); err == nil {
				return &enriched, nil
			}
		}
		// Fallback: return chunk without reasoning (shouldn't happen)
		return chunk, nil

	case "input_json_delta":
		return &openai.ChatCompletionChunk{
			ID:      s.id,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   s.model,
			Choices: []openai.ChatCompletionChunkChoice{
				{
					Index: 0,
					Delta: openai.ChatCompletionChunkChoiceDelta{
						ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
							{
								Index: int64(s.toolCallIndex),
								Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
									Arguments: parsed.Get("delta.partial_json").String(),
								},
							},
						},
					},
				},
			},
		}, nil
	}

	// Unknown delta type, skip
	return nil, nil
}
