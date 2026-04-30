package shepherd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

func newGuardContextLookupTool(cache *guardContextCache) tool.BaseTool {
	return &guardContextLookupTool{cache: cache}
}

type guardContextLookupTool struct {
	cache *guardContextCache
}

func (t *guardContextLookupTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "get_full_guard_context",
		Desc: "Retrieve omitted tool_call or tool_result content by context_id when the security payload was truncated. Use this only when the preview is insufficient to classify risk. Supports offset and length for chunked reads.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"context_id": {Type: schema.String, Required: true, Desc: "The context_id shown in the truncated payload"},
			"offset":     {Type: schema.Integer, Required: false, Desc: "Zero-based character offset. Defaults to 0"},
			"length":     {Type: schema.Integer, Required: false, Desc: "Maximum characters to return. Defaults to 12000, capped at 50000"},
		}),
	}, nil
}

func (t *guardContextLookupTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		ContextID string `json:"context_id"`
		Offset    int    `json:"offset"`
		Length    int    `json:"length"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	contextID := strings.TrimSpace(args.ContextID)
	item, ok := t.cache.Lookup(contextID)
	if !ok {
		return marshalGuardContextToolResult(map[string]interface{}{
			"found":      false,
			"context_id": contextID,
			"error":      "context_id not found",
		}), nil
	}

	runes := []rune(item.Content)
	offset := args.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > len(runes) {
		offset = len(runes)
	}
	length := args.Length
	if length <= 0 {
		length = 12000
	}
	if length > guardContextToolMaxChars {
		length = guardContextToolMaxChars
	}
	end := offset + length
	if end > len(runes) {
		end = len(runes)
	}
	truncated := end < len(runes)
	result := map[string]interface{}{
		"found":          true,
		"context_id":     item.ID,
		"kind":           item.Kind,
		"owner_id":       item.OwnerID,
		"field":          item.Field,
		"total_chars":    len(runes),
		"offset":         offset,
		"returned_chars": end - offset,
		"content":        string(runes[offset:end]),
		"truncated":      truncated,
	}
	if truncated {
		result["next_offset"] = end
	}
	return marshalGuardContextToolResult(result), nil
}

func marshalGuardContextToolResult(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"found":false,"error":"marshal failed"}`
	}
	return string(b)
}
