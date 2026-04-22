package proxy

import (
	"encoding/json"
	"sync"

	"github.com/openai/openai-go"
)

// StreamBuffer accumulates stream chunks and tool calls for analysis
type StreamBuffer struct {
	mu              sync.Mutex
	requestID       string
	requestMessages []ConversationMessage                  // Original request messages
	contentChunks   []string                               // Accumulated content from stream deltas
	toolCalls       []openai.ChatCompletionMessageToolCall // Accumulated tool calls
	toolsRaw        json.RawMessage                        // Tools definitions from request (raw JSON for token estimation)
	loggedToolCalls map[int]bool                           // Tool calls already emitted to UI during streaming
	started         bool                                   // Whether monitor_upstream_stream_started has been emitted

	// Token usage for current request (set when usage is received in stream)
	promptTokens     int
	completionTokens int
	totalTokens      int
}

type StreamToolCallUpdate struct {
	Index    int
	ToolCall openai.ChatCompletionMessageToolCall
}

// NewStreamBuffer creates a new stream buffer
func NewStreamBuffer() *StreamBuffer {
	return &StreamBuffer{
		contentChunks:   make([]string, 0),
		toolCalls:       make([]openai.ChatCompletionMessageToolCall, 0),
		loggedToolCalls: make(map[int]bool),
	}
}

// SetRequest initializes buffer with request data
func (sb *StreamBuffer) SetRequest(requestID string, req *openai.ChatCompletionNewParams, rawBody []byte) {
	if sb == nil {
		return
	}
	sb.mu.Lock()
	defer sb.mu.Unlock()

	sb.requestID = requestID
	sb.requestMessages = make([]ConversationMessage, 0, len(req.Messages))
	for _, msg := range req.Messages {
		cm := extractConversationMessage(msg)
		sb.requestMessages = append(sb.requestMessages, cm)
	}
	// Store tool definitions raw JSON for token estimation
	if len(req.Tools) > 0 {
		if toolsBytes, err := json.Marshal(req.Tools); err == nil {
			sb.toolsRaw = toolsBytes
		}
	}
}

func (sb *StreamBuffer) RequestID() string {
	if sb == nil {
		return ""
	}
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.requestID
}

// AppendContent appends content from a stream delta
func (sb *StreamBuffer) AppendContent(content string) {
	if sb == nil {
		return
	}
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.contentChunks = append(sb.contentChunks, content)
}

// MergeStreamToolCall merges incremental stream tool call data from a chunk delta
func (sb *StreamBuffer) MergeStreamToolCall(stc openai.ChatCompletionChunkChoiceDeltaToolCall) {
	if sb == nil {
		return
	}
	sb.mu.Lock()
	defer sb.mu.Unlock()

	idx := int(stc.Index)
	for len(sb.toolCalls) <= idx {
		sb.toolCalls = append(sb.toolCalls, openai.ChatCompletionMessageToolCall{
			Type: "function",
		})
	}

	tc := &sb.toolCalls[idx]
	if stc.ID != "" {
		tc.ID = stc.ID
	}
	if stc.Type != "" {
		tc.Type = "function"
	}
	if stc.Function.Name != "" {
		tc.Function.Name += stc.Function.Name
	}
	if stc.Function.Arguments != "" {
		tc.Function.Arguments += stc.Function.Arguments
	}
}

// ConsumeNewlyReadyToolCalls returns tool calls whose names became available
// during streaming and marks them as emitted to avoid duplicate UI events.
func (sb *StreamBuffer) ConsumeNewlyReadyToolCalls() []StreamToolCallUpdate {
	if sb == nil {
		return nil
	}
	sb.mu.Lock()
	defer sb.mu.Unlock()

	if len(sb.toolCalls) == 0 {
		return nil
	}

	ready := make([]StreamToolCallUpdate, 0)
	for idx, tc := range sb.toolCalls {
		if tc.Function.Name == "" || sb.loggedToolCalls[idx] {
			continue
		}
		sb.loggedToolCalls[idx] = true
		ready = append(ready, StreamToolCallUpdate{
			Index:    idx,
			ToolCall: tc,
		})
	}
	return ready
}

// HasToolCalls returns true if buffer has tool calls
func (sb *StreamBuffer) HasToolCalls() bool {
	if sb == nil {
		return false
	}
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return len(sb.toolCalls) > 0
}

// Clear resets the buffer for next request/response cycle
func (sb *StreamBuffer) Clear() {
	if sb == nil {
		return
	}
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.contentChunks = make([]string, 0)
	sb.toolCalls = make([]openai.ChatCompletionMessageToolCall, 0)
	sb.requestMessages = make([]ConversationMessage, 0)
	sb.loggedToolCalls = make(map[int]bool)
	sb.promptTokens = 0
	sb.completionTokens = 0
	sb.totalTokens = 0
	sb.started = false
}

// ClearAll resets the entire buffer including request data
func (sb *StreamBuffer) ClearAll() {
	if sb == nil {
		return
	}
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.requestID = ""
	sb.requestMessages = nil
	sb.contentChunks = make([]string, 0)
	sb.toolCalls = make([]openai.ChatCompletionMessageToolCall, 0)
	sb.toolsRaw = nil
	sb.loggedToolCalls = make(map[int]bool)
	sb.promptTokens = 0
	sb.completionTokens = 0
	sb.totalTokens = 0
	sb.started = false
}

// truncateString truncates a string to maxLen runes and adds "..." if truncated
func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// estimateTokenCount estimates the number of tokens in a text string
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

// calculateRequestTokensFromRaw calculates tokens for a list of messages and raw tools JSON
func calculateRequestTokensFromRaw(messages []ConversationMessage, toolsRaw json.RawMessage) int {
	count := 0
	for _, msg := range messages {
		count += estimateTokenCount(msg.Role)
		count += estimateTokenCount(msg.Content)
		if msg.ToolCalls != nil {
			if tcBytes, err := json.Marshal(msg.ToolCalls); err == nil {
				count += estimateTokenCount(string(tcBytes))
			}
		}
	}

	if len(toolsRaw) > 0 {
		count += estimateTokenCount(string(toolsRaw))
	}

	return count
}
