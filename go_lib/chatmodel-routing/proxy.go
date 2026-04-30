package chatmodelrouting

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"go_lib/chatmodel-routing/adapter"
	"go_lib/core/logging"

	"github.com/openai/openai-go"
)

// Proxy 反向代理
type Proxy struct {
	provider   adapter.Provider
	providerMu sync.RWMutex
	filter     Filter
}

// HistoryRequestLog represents a proxy request log entry.
type HistoryRequestLog struct {
	Source  string `json:"source"`
	Method  string `json:"method"`
	URL     string `json:"url"`
	Headers string `json:"headers"`
}

const (
	// historySourceClient indicates request from client to proxy.
	historySourceClient = "client"
	// historySourceUpstream indicates request from proxy to upstream.
	historySourceUpstream = "upstream"
)

// NewProxy 创建代理实例
func NewProxy(provider adapter.Provider, filter Filter) (*Proxy, error) {
	return &Proxy{
		provider: provider,
		filter:   filter,
	}, nil
}

// UpdateProvider replaces the upstream provider at runtime.
// This allows hot-swapping the forwarding target (e.g., when the user changes
// the bot model config) without restarting the proxy server.
func (p *Proxy) UpdateProvider(provider adapter.Provider) {
	p.providerMu.Lock()
	defer p.providerMu.Unlock()
	p.provider = provider
}

// getProvider returns the current provider with read-lock protection.
func (p *Proxy) getProvider() adapter.Provider {
	p.providerMu.RLock()
	defer p.providerMu.RUnlock()
	return p.provider
}

// ListenAndServe 在指定地址监听并处理请求
func (p *Proxy) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, p)
}

// ServeHTTP 实现 http.Handler
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. 读取请求体
	var reqBody []byte
	if r.Body != nil {
		var err error
		reqBody, err = io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			p.writeOpenAIErrorResponse(w, "internal_error", err.Error(), http.StatusInternalServerError)
			return
		}
	}

	p.logHistoryRequest(historySourceClient, r)

	// 2. 解析是否流式请求（需在过滤前确定，mock 响应需区分流式/非流式）
	isStream := false
	var probe struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(reqBody, &probe); err == nil {
		isStream = probe.Stream
	}

	// 3. 过滤请求
	if p.filter != nil {
		result, pass := p.filter.FilterRequest(r.Context(), reqBody)
		if !pass {
			if result != nil && result.MockContent != "" {
				logging.Warning("[Proxy] Request blocked by filter, returning mock response")
				if isStream {
					p.writeMockStreamChatCompletion(w, result.MockContent)
				} else {
					p.writeMockChatCompletion(w, result.MockContent)
				}
			} else {
				logging.Warning("[Proxy] Request blocked by security filter")
				p.writeOpenAIErrorResponse(w, "content_filter", "Request blocked by security filter: potential prompt injection detected", http.StatusBadRequest)
			}
			return
		}
		if result != nil && len(result.ForwardBody) > 0 {
			reqBody = result.ForwardBody
			logging.Info("[Proxy] Request body rewritten by filter before upstream forwarding")
		}
	}

	// Log upstream attempt (approximate)
	p.logHistoryRequest(historySourceUpstream, r)

	// Snapshot the provider under read-lock so a concurrent UpdateProvider
	// doesn't cause a race.
	currentProvider := p.getProvider()

	if isStream {
		logging.Info("[Proxy] Forwarding stream request: %s %s", r.Method, r.URL.Path)
		stream, err := currentProvider.ChatCompletionStreamRaw(r.Context(), reqBody)
		if err != nil {
			logging.Error("[Proxy] Upstream stream error: %v", err)
			p.writeOpenAIErrorResponse(w, "upstream_error", err.Error(), http.StatusBadGateway)
			return
		}
		defer stream.Close()
		p.serveStreamResponse(r.Context(), stream, w)
	} else {
		logging.Info("[Proxy] Forwarding non-stream request: %s %s", r.Method, r.URL.Path)
		resp, err := currentProvider.ChatCompletionRaw(r.Context(), reqBody)
		if err != nil {
			logging.Error("[Proxy] Upstream error: %v", err)
			p.writeOpenAIErrorResponse(w, "upstream_error", err.Error(), http.StatusBadGateway)
			return
		}
		p.serveNonStreamResponse(r.Context(), resp, w)
	}
}

func (p *Proxy) serveNonStreamResponse(ctx context.Context, resp *openai.ChatCompletion, w http.ResponseWriter) {
	if p.filter != nil {
		if !p.filter.FilterResponse(ctx, resp) {
			p.writeOpenAIErrorResponse(w, "content_filter", "Response blocked by security filter", http.StatusBadGateway)
			return
		}
	}

	var content []byte
	if raw := resp.RawJSON(); raw != "" {
		rewritten, changed, err := rewriteRawResponseToolCallIDs(raw, resp)
		if err != nil {
			logging.Warning("[Proxy] Failed to rewrite non-stream raw JSON tool_call IDs: %v", err)
			content, err = json.Marshal(resp)
			if err != nil {
				p.writeOpenAIErrorResponse(w, "internal_error", "Failed to marshal response", http.StatusInternalServerError)
				return
			}
		} else if changed {
			content = []byte(rewritten)
		} else {
			content = []byte(raw)
		}
	} else {
		var err error
		content, err = json.Marshal(resp)
		if err != nil {
			p.writeOpenAIErrorResponse(w, "internal_error", "Failed to marshal response", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func (p *Proxy) serveStreamResponse(ctx context.Context, stream adapter.Stream, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	flusher, _ := w.(http.Flusher)
	sawFinishReason := false
	bufferingToolCalls := false
	var bufferedToolCallChunks [][]byte

	for {
		chunk, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				// Some providers may end stream without an explicit finish_reason chunk.
				// Trigger a synthetic terminal chunk so filter logic can finalize accounting.
				if p.filter != nil && !sawFinishReason {
					syntheticChunk := &openai.ChatCompletionChunk{
						Choices: []openai.ChatCompletionChunkChoice{
							{
								Index:        0,
								Delta:        openai.ChatCompletionChunkChoiceDelta{},
								FinishReason: "stop",
							},
						},
					}
					if !p.filter.FilterStreamChunk(ctx, syntheticChunk) {
						_ = p.writeStreamChunk(w, syntheticChunk, flusher)
						w.Write([]byte("data: [DONE]\n\n"))
						if flusher != nil {
							flusher.Flush()
						}
						return
					}
				}
				if bufferingToolCalls {
					p.writeBufferedStreamChunks(w, bufferedToolCallChunks, flusher)
				}
				// Send [DONE]
				w.Write([]byte("data: [DONE]\n\n"))
				if flusher != nil {
					flusher.Flush()
				}
				return
			}
			logging.Error("[Proxy] Stream recv error: %v", err)
			// Ensure in-flight stream can still be finalized in filter side.
			if p.filter != nil {
				_ = p.filter.FilterStreamChunk(ctx, &openai.ChatCompletionChunk{
					Choices: []openai.ChatCompletionChunkChoice{
						{
							Index:        0,
							Delta:        openai.ChatCompletionChunkChoiceDelta{},
							FinishReason: "error",
						},
					},
				})
			}
			return
		}

		for _, choice := range chunk.Choices {
			if choice.FinishReason != "" {
				sawFinishReason = true
				break
			}
		}

		// filter 回调仅做分析（日志/审计/指标），不修改 chunk
		if p.filter != nil {
			pass := p.filter.FilterStreamChunk(ctx, chunk)
			if !pass {
				// Drop buffered tool_call chunks; only forward the security response.
				_ = p.writeStreamChunk(w, chunk, flusher)
				w.Write([]byte("data: [DONE]\n\n"))
				if flusher != nil {
					flusher.Flush()
				}
				return
			}
		}

		if streamChunkHasToolCallDelta(chunk) {
			bufferingToolCalls = true
		}
		if bufferingToolCalls {
			chunkBytes, marshalErr := marshalStreamChunk(chunk)
			if marshalErr != nil {
				logging.Warning("[Proxy] Failed to marshal buffered stream chunk: %v", marshalErr)
				continue
			}
			bufferedToolCallChunks = append(bufferedToolCallChunks, chunkBytes)
			if streamChunkHasFinishReason(chunk) {
				p.writeBufferedStreamChunks(w, bufferedToolCallChunks, flusher)
				bufferedToolCallChunks = nil
				bufferingToolCalls = false
			}
			continue
		}

		_ = p.writeStreamChunk(w, chunk, flusher)
	}
}

func (p *Proxy) writeBufferedStreamChunks(w http.ResponseWriter, chunks [][]byte, flusher http.Flusher) {
	for _, chunkBytes := range chunks {
		w.Write([]byte(fmt.Sprintf("data: %s\n\n", string(chunkBytes))))
	}
	if flusher != nil && len(chunks) > 0 {
		flusher.Flush()
	}
}

func (p *Proxy) writeStreamChunk(w http.ResponseWriter, chunk *openai.ChatCompletionChunk, flusher http.Flusher) error {
	chunkBytes, err := marshalStreamChunk(chunk)
	if err != nil {
		logging.Warning("[Proxy] Failed to marshal stream chunk: %v", err)
		return err
	}
	w.Write([]byte(fmt.Sprintf("data: %s\n\n", string(chunkBytes))))
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

func marshalStreamChunk(chunk *openai.ChatCompletionChunk) ([]byte, error) {
	if chunk == nil {
		return json.Marshal(chunk)
	}
	if raw := chunk.RawJSON(); raw != "" {
		rewritten, changed, rewriteErr := rewriteRawChunkToolCallIDs(raw, chunk)
		if rewriteErr != nil {
			logging.Warning("[Proxy] Failed to rewrite stream raw JSON tool_call IDs: %v", rewriteErr)
			return json.Marshal(chunk)
		}
		if changed {
			return []byte(rewritten), nil
		}
		return []byte(raw), nil
	}
	return json.Marshal(chunk)
}

func streamChunkHasToolCallDelta(chunk *openai.ChatCompletionChunk) bool {
	if chunk == nil {
		return false
	}
	for _, choice := range chunk.Choices {
		if len(choice.Delta.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

func streamChunkHasFinishReason(chunk *openai.ChatCompletionChunk) bool {
	if chunk == nil {
		return false
	}
	for _, choice := range chunk.Choices {
		if choice.FinishReason != "" {
			return true
		}
	}
	return false
}

func rewriteRawResponseToolCallIDs(raw string, resp *openai.ChatCompletion) (string, bool, error) {
	if strings.TrimSpace(raw) == "" || resp == nil {
		return raw, false, nil
	}
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return "", false, err
	}
	choices, ok := root["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return raw, false, nil
	}

	changed := false
	for i, choice := range choices {
		if i >= len(resp.Choices) {
			break
		}
		respToolCalls := resp.Choices[i].Message.ToolCalls

		choiceMap, ok := choice.(map[string]interface{})
		if !ok {
			return "", false, fmt.Errorf("raw choices[%d] is not an object", i)
		}
		messageMap, ok := choiceMap["message"].(map[string]interface{})
		if !ok {
			return "", false, fmt.Errorf("raw choices[%d].message is not an object", i)
		}
		respContent := resp.Choices[i].Message.Content
		if respContent != "" && stringValue(messageMap["content"]) != respContent {
			messageMap["content"] = respContent
			changed = true
		}
		rawToolCalls, ok := messageMap["tool_calls"].([]interface{})
		if !ok && len(respToolCalls) > 0 {
			return "", false, fmt.Errorf("raw choices[%d].message.tool_calls is not an array", i)
		}

		for j := range respToolCalls {
			respID := strings.TrimSpace(respToolCalls[j].ID)
			if respID == "" {
				continue
			}
			if j >= len(rawToolCalls) {
				return "", false, fmt.Errorf("raw choices[%d].message.tool_calls is shorter than parsed response", i)
			}
			rawToolCall, ok := rawToolCalls[j].(map[string]interface{})
			if !ok {
				return "", false, fmt.Errorf("raw choices[%d].message.tool_calls[%d] is not an object", i, j)
			}
			rawID := strings.TrimSpace(stringValue(rawToolCall["id"]))
			if rawID == respID {
				continue
			}
			rawToolCall["id"] = respID
			changed = true
		}
	}

	if !changed {
		return raw, false, nil
	}
	out, err := json.Marshal(root)
	if err != nil {
		return "", false, err
	}
	return string(out), true, nil
}

func rewriteRawChunkToolCallIDs(raw string, chunk *openai.ChatCompletionChunk) (string, bool, error) {
	if strings.TrimSpace(raw) == "" || chunk == nil {
		return raw, false, nil
	}
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return "", false, err
	}
	choices, ok := root["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return raw, false, nil
	}

	changed := false
	for i, choice := range choices {
		if i >= len(chunk.Choices) {
			break
		}
		respToolCalls := chunk.Choices[i].Delta.ToolCalls

		choiceMap, ok := choice.(map[string]interface{})
		if !ok {
			return "", false, fmt.Errorf("raw chunk choices[%d] is not an object", i)
		}
		deltaMap, ok := choiceMap["delta"].(map[string]interface{})
		if !ok {
			return "", false, fmt.Errorf("raw chunk choices[%d].delta is not an object", i)
		}
		respContent := chunk.Choices[i].Delta.Content
		if respContent != "" && stringValue(deltaMap["content"]) != respContent {
			deltaMap["content"] = respContent
			changed = true
		}
		if len(respToolCalls) == 0 {
			continue
		}
		rawToolCalls, ok := deltaMap["tool_calls"].([]interface{})
		if !ok {
			return "", false, fmt.Errorf("raw chunk choices[%d].delta.tool_calls is not an array", i)
		}

		for j := range respToolCalls {
			respID := strings.TrimSpace(respToolCalls[j].ID)
			if respID == "" {
				continue
			}
			if j >= len(rawToolCalls) {
				return "", false, fmt.Errorf("raw chunk choices[%d].delta.tool_calls is shorter than parsed response", i)
			}
			rawToolCall, ok := rawToolCalls[j].(map[string]interface{})
			if !ok {
				return "", false, fmt.Errorf("raw chunk choices[%d].delta.tool_calls[%d] is not an object", i, j)
			}
			rawID := strings.TrimSpace(stringValue(rawToolCall["id"]))
			if rawID == respID {
				continue
			}
			rawToolCall["id"] = respID
			changed = true
		}
	}

	if !changed {
		return raw, false, nil
	}
	out, err := json.Marshal(root)
	if err != nil {
		return "", false, err
	}
	return string(out), true, nil
}

func stringValue(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	default:
		return ""
	}
}

// writeMockChatCompletion 返回非流式 mock ChatCompletion 响应
func (p *Proxy) writeMockChatCompletion(w http.ResponseWriter, content string) {
	now := time.Now().Unix()
	mockResp := map[string]interface{}{
		"id":      fmt.Sprintf("chatcmpl-mock-%d", now),
		"object":  "chat.completion",
		"created": now,
		"model":   "clawsecbot-system",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(mockResp)
}

// writeMockStreamChatCompletion 返回流式 mock ChatCompletion 响应（SSE 格式）
func (p *Proxy) writeMockStreamChatCompletion(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	flusher, _ := w.(http.Flusher)

	now := time.Now().Unix()
	mockChunk := map[string]interface{}{
		"id":      fmt.Sprintf("chatcmpl-mock-%d", now),
		"object":  "chat.completion.chunk",
		"created": now,
		"model":   "clawsecbot-system",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"delta": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
	}

	chunkBytes, _ := json.Marshal(mockChunk)
	w.Write([]byte(fmt.Sprintf("data: %s\n\n", string(chunkBytes))))
	if flusher != nil {
		flusher.Flush()
	}

	w.Write([]byte("data: [DONE]\n\n"))
	if flusher != nil {
		flusher.Flush()
	}
}

// writeOpenAIErrorResponse writes an OpenAI API compatible error response
func (p *Proxy) writeOpenAIErrorResponse(w http.ResponseWriter, errType, message string, statusCode int) {
	errorResp := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    errType,
			"code":    errType,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(errorResp)
}

// logHistoryRequest logs proxy request details into history log.
func (p *Proxy) logHistoryRequest(source string, req *http.Request) {
	entry := HistoryRequestLog{
		Source:  source,
		Method:  req.Method,
		URL:     req.URL.String(),
		Headers: cloneHeaders(req.Header),
	}
	direction := "unknown"
	switch entry.Source {
	case historySourceClient:
		direction = "recv from"
	case historySourceUpstream:
		direction = "send to"
	}
	logging.HistoryInfo("[Request %s %s] method=%s url=%s headers=%s", direction, entry.Source, entry.Method, entry.URL, entry.Headers)
}

// cloneHeaders builds a readable header string for logging.
func cloneHeaders(headers http.Header) string {
	if headers == nil {
		return "{}"
	}
	keyHeaders := map[string]struct{}{
		"authorization":  {},
		"content-length": {},
		"content-type":   {},
	}
	maskedValues := make(map[string][]string, len(keyHeaders))
	for key, values := range headers {
		if _, ok := keyHeaders[strings.ToLower(key)]; !ok {
			continue
		}
		items := make([]string, 0, len(values))
		for _, value := range values {
			items = append(items, maskHeaderValue(key, value))
		}
		maskedValues[key] = items
	}
	if len(maskedValues) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(maskedValues))
	for key := range maskedValues {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	builder.WriteString("{")
	for index, key := range keys {
		values := maskedValues[key]
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(strings.Join(values, ","))
		if index < len(keys)-1 {
			builder.WriteString("; ")
		}
	}
	builder.WriteString("}")
	return builder.String()
}

// maskHeaderValue masks header values for logging.
func maskHeaderValue(key, value string) string {
	lowerKey := strings.ToLower(key)
	if lowerKey == "authorization" {
		if value == "" {
			return ""
		}
		if len(value) <= 8 {
			return "****"
		}
		return value[:4] + "****" + value[len(value)-4:]
	}
	return value
}
