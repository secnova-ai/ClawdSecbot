package chatmodelrouting

import (
	"context"
	"encoding/json"

	"github.com/openai/openai-go"
)

// FilterRequestResult carries request-filter decisions back to the HTTP proxy.
type FilterRequestResult struct {
	// MockContent is used when the request is blocked and the proxy should
	// return a model-shaped assistant response instead of an HTTP error.
	MockContent string
	// ForwardBody replaces the raw upstream request body when the filter allows
	// the request but rewrites payload content such as redacted messages.
	ForwardBody []byte
}

// OnRequest 请求过滤回调，接收解析后的 ChatCompletionNewParams 和原始请求体
// 返回 (*FilterRequestResult, bool)：bool=true 放行，bool=false 拦截；拦截时 result 可携带 mock 内容
type OnRequest func(ctx context.Context, req *openai.ChatCompletionNewParams, rawBody []byte) (*FilterRequestResult, bool)

// OnResponse 非流式响应过滤回调，接收解析后的 ChatCompletion（可原地修改）
type OnResponse func(ctx context.Context, resp *openai.ChatCompletion) (pass bool)

// OnStreamChunk 流式响应过滤回调，接收解析后的 ChatCompletionChunk（可原地修改）
type OnStreamChunk func(ctx context.Context, chunk *openai.ChatCompletionChunk) (pass bool)

// Filter 安全过滤器接口
type Filter interface {
	// FilterRequest 过滤请求体；返回 (*FilterRequestResult, bool)：bool=true 放行，bool=false 拦截
	FilterRequest(ctx context.Context, body []byte) (*FilterRequestResult, bool)
	// FilterResponse 过滤非流式响应（可原地修改 resp）
	FilterResponse(ctx context.Context, resp *openai.ChatCompletion) (pass bool)
	// FilterStreamChunk 过滤流式响应 chunk（可原地修改 chunk）
	FilterStreamChunk(ctx context.Context, chunk *openai.ChatCompletionChunk) (pass bool)
}

// NewCallbackFilter 创建基于回调的过滤器
func NewCallbackFilter(onRequest OnRequest, onResponse OnResponse, onStreamChunk OnStreamChunk) Filter {
	return &callbackFilter{
		onRequest:     onRequest,
		onResponse:    onResponse,
		onStreamChunk: onStreamChunk,
	}
}

type callbackFilter struct {
	onRequest     OnRequest
	onResponse    OnResponse
	onStreamChunk OnStreamChunk
}

func (f *callbackFilter) FilterRequest(ctx context.Context, body []byte) (*FilterRequestResult, bool) {
	if f.onRequest == nil {
		return nil, true
	}

	var parsed openai.ChatCompletionNewParams
	if err := json.Unmarshal(body, &parsed); err != nil {
		// 无法解析为 OpenAI 格式则放行
		return nil, true
	}

	return f.onRequest(ctx, &parsed, body)
}

func (f *callbackFilter) FilterResponse(ctx context.Context, resp *openai.ChatCompletion) (pass bool) {
	if f.onResponse == nil {
		return true
	}
	return f.onResponse(ctx, resp)
}

func (f *callbackFilter) FilterStreamChunk(ctx context.Context, chunk *openai.ChatCompletionChunk) (pass bool) {
	if f.onStreamChunk == nil {
		return true
	}
	return f.onStreamChunk(ctx, chunk)
}
