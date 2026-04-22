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

	// 使用 RawJSON 透传原始 JSON（保留 reasoning_content 等非标准字段）
	// filter 回调仅做分析（日志/审计/指标），不修改 resp，因此直接使用原始 JSON 即可
	var content []byte
	if raw := resp.RawJSON(); raw != "" {
		content = []byte(raw)
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

	for {
		chunk, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				// Some providers may end stream without an explicit finish_reason chunk.
				// Trigger a synthetic terminal chunk so filter logic can finalize accounting.
				if p.filter != nil && !sawFinishReason {
					_ = p.filter.FilterStreamChunk(ctx, &openai.ChatCompletionChunk{
						Choices: []openai.ChatCompletionChunkChoice{
							{
								Index:        0,
								Delta:        openai.ChatCompletionChunkChoiceDelta{},
								FinishReason: "stop",
							},
						},
					})
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
				// 被拦截：发送当前 chunk 后关闭流
				chunkBytes, marshalErr := json.Marshal(chunk)
				if marshalErr == nil {
					w.Write([]byte(fmt.Sprintf("data: %s\n\n", string(chunkBytes))))
				}
				w.Write([]byte("data: [DONE]\n\n"))
				if flusher != nil {
					flusher.Flush()
				}
				return
			}
		}

		// 使用 RawJSON 透传原始 JSON（保留 reasoning_content 等非标准字段）
		var chunkBytes []byte
		if raw := chunk.RawJSON(); raw != "" {
			chunkBytes = []byte(raw)
		} else {
			var marshalErr error
			chunkBytes, marshalErr = json.Marshal(chunk)
			if marshalErr != nil {
				continue
			}
		}

		sseData := fmt.Sprintf("data: %s\n\n", string(chunkBytes))
		w.Write([]byte(sseData))
		if flusher != nil {
			flusher.Flush()
		}
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
