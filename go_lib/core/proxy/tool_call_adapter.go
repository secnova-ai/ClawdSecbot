package proxy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

type normalizedToolPayload struct {
	Calls   []ToolCallInfo
	Results []ToolResultInfo
}

type rawToolCallAdapter interface {
	Name() string
	Extract(raw string) normalizedToolPayload
}

var responseToolCallAdapters = []rawToolCallAdapter{
	openAIMCPToolCallAdapter{},
}

func extractRawResponseToolPayload(raw string) normalizedToolPayload {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return normalizedToolPayload{}
	}
	var out normalizedToolPayload
	for _, adapter := range responseToolCallAdapters {
		payload := adapter.Extract(raw)
		out.Calls = append(out.Calls, payload.Calls...)
		out.Results = append(out.Results, payload.Results...)
	}
	return out
}

type openAIMCPToolCallAdapter struct{}

func (openAIMCPToolCallAdapter) Name() string {
	return "openai_mcp"
}

func (openAIMCPToolCallAdapter) Extract(raw string) normalizedToolPayload {
	output := gjson.Get(raw, "output")
	if !output.IsArray() {
		return normalizedToolPayload{}
	}

	var payload normalizedToolPayload
	output.ForEach(func(_, item gjson.Result) bool {
		itemType := strings.TrimSpace(item.Get("type").String())
		if itemType != "mcp_call" && itemType != "mcp_approval_request" {
			return true
		}

		originalID := strings.TrimSpace(item.Get("id").String())
		serverLabel := strings.TrimSpace(item.Get("server_label").String())
		toolCallID := normalizeAdapterToolCallID("mcp", serverLabel, originalID)
		name := strings.TrimSpace(item.Get("name").String())
		rawArgs := rawJSONOrString(item.Get("arguments"))
		if toolCallID == "" || name == "" {
			return true
		}

		call := ToolCallInfo{
			Name:               name,
			RawArgs:            rawArgs,
			ToolCallID:         toolCallID,
			OriginalToolCallID: originalID,
			Protocol:           "mcp",
			ServerLabel:        serverLabel,
		}
		if rawArgs != "" {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(rawArgs), &args); err == nil {
				call.Arguments = args
			}
		}
		payload.Calls = append(payload.Calls, call)

		if itemType == "mcp_call" {
			if content, ok := mcpCallResultContent(item); ok {
				payload.Results = append(payload.Results, ToolResultInfo{
					ToolCallID:         toolCallID,
					OriginalToolCallID: originalID,
					FuncName:           name,
					Content:            content,
					Protocol:           "mcp",
					ServerLabel:        serverLabel,
				})
			}
		}
		return true
	})
	return payload
}

func normalizeAdapterToolCallID(protocol, serverLabel, rawID string) string {
	rawID = strings.TrimSpace(rawID)
	if rawID == "" {
		return ""
	}
	protocol = strings.TrimSpace(protocol)
	serverLabel = strings.TrimSpace(serverLabel)
	if protocol == "" || serverLabel == "" {
		return rawID
	}
	if strings.HasPrefix(rawID, protocol+":") {
		return rawID
	}
	return fmt.Sprintf("%s:%s:%s", protocol, sanitizeToolCallIDPart(serverLabel), rawID)
}

func sanitizeToolCallIDPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "\t", "_")
	value = strings.ReplaceAll(value, "\n", "_")
	return value
}

func rawJSONOrString(value gjson.Result) string {
	if !value.Exists() {
		return ""
	}
	if value.Type == gjson.String {
		return strings.TrimSpace(value.String())
	}
	return strings.TrimSpace(value.Raw)
}

func mcpCallResultContent(item gjson.Result) (string, bool) {
	if output := rawJSONOrString(item.Get("output")); output != "" {
		return output, true
	}
	if errValue := item.Get("error"); errValue.Exists() {
		if content := rawJSONOrString(errValue); content != "" {
			return content, true
		}
	}
	return "", false
}

func toolCallRefsFromInfos(infos []ToolCallInfo) []toolCallRef {
	refs := make([]toolCallRef, 0, len(infos))
	for _, info := range infos {
		refs = append(refs, toolCallRef{
			ID:       info.ToolCallID,
			FuncName: info.Name,
			RawArgs:  info.RawArgs,
		})
	}
	return refs
}

func toolResultMapFromInfos(infos []ToolResultInfo) map[string]string {
	results := make(map[string]string, len(infos))
	for _, info := range infos {
		if id := strings.TrimSpace(info.ToolCallID); id != "" {
			results[id] = info.Content
		}
	}
	return results
}
