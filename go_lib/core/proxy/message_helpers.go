package proxy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/openai/openai-go"
)

// Type definitions for ConversationMessage, ToolCallInfo, ToolResultInfo
// are in aliases.go (aliased from core/shepherd)

// extractToolCalls extracts tool call info from interface{}
func extractToolCalls(toolCallsRaw interface{}) []ToolCallInfo {
	var result []ToolCallInfo

	switch tc := toolCallsRaw.(type) {
	case []interface{}:
		for _, item := range tc {
			if info := parseToolCallItem(item); info != nil {
				result = append(result, *info)
			}
		}
	case []openai.ChatCompletionMessageToolCall:
		for _, item := range tc {
			info := ToolCallInfo{
				Name:       item.Function.Name,
				RawArgs:    item.Function.Arguments,
				ToolCallID: item.ID,
			}
			if item.Function.Arguments != "" {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(item.Function.Arguments), &args); err == nil {
					info.Arguments = args
				}
			}
			result = append(result, info)
		}
	case []openai.ChatCompletionMessageToolCallParam:
		for _, item := range tc {
			info := ToolCallInfo{
				Name:       item.Function.Name,
				RawArgs:    item.Function.Arguments,
				ToolCallID: item.ID,
			}
			if item.Function.Arguments != "" {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(item.Function.Arguments), &args); err == nil {
					info.Arguments = args
				}
			}
			result = append(result, info)
		}
	}

	return result
}

// parseToolCallItem 解析单个工具调用项
func parseToolCallItem(item interface{}) *ToolCallInfo {
	switch v := item.(type) {
	case map[string]interface{}:
		info := &ToolCallInfo{}
		if id, ok := v["id"].(string); ok {
			info.ToolCallID = id
		}
		if fn, ok := v["function"].(map[string]interface{}); ok {
			if name, ok := fn["name"].(string); ok {
				info.Name = name
			}
			if args, ok := fn["arguments"].(string); ok {
				info.RawArgs = args
				var argsMap map[string]interface{}
				if err := json.Unmarshal([]byte(args), &argsMap); err == nil {
					info.Arguments = argsMap
				}
			}
		}
		return info
	case openai.ChatCompletionMessageToolCall:
		return &ToolCallInfo{
			Name:       v.Function.Name,
			RawArgs:    v.Function.Arguments,
			ToolCallID: v.ID,
		}
	}
	return nil
}

// getMessageRole extracts role from a ChatCompletionMessageParamUnion
func getMessageRole(msg openai.ChatCompletionMessageParamUnion) string {
	switch {
	case msg.OfSystem != nil:
		return "system"
	case msg.OfUser != nil:
		return "user"
	case msg.OfAssistant != nil:
		return "assistant"
	case msg.OfTool != nil:
		return "tool"
	case msg.OfDeveloper != nil:
		return "developer"
	default:
		return "unknown"
	}
}

// extractMessageContent extracts text content from a ChatCompletionMessageParamUnion
func extractMessageContent(msg openai.ChatCompletionMessageParamUnion) string {
	switch {
	case msg.OfSystem != nil:
		if msg.OfSystem.Content.OfString.Value != "" {
			return msg.OfSystem.Content.OfString.Value
		}
		if len(msg.OfSystem.Content.OfArrayOfContentParts) > 0 {
			var parts []string
			for _, p := range msg.OfSystem.Content.OfArrayOfContentParts {
				parts = append(parts, p.Text)
			}
			return strings.Join(parts, "")
		}
	case msg.OfUser != nil:
		if msg.OfUser.Content.OfString.Value != "" {
			return msg.OfUser.Content.OfString.Value
		}
		if len(msg.OfUser.Content.OfArrayOfContentParts) > 0 {
			var parts []string
			for _, p := range msg.OfUser.Content.OfArrayOfContentParts {
				if p.OfText != nil {
					parts = append(parts, p.OfText.Text)
				}
			}
			return strings.Join(parts, "")
		}
	case msg.OfAssistant != nil:
		if msg.OfAssistant.Content.OfString.Value != "" {
			return msg.OfAssistant.Content.OfString.Value
		}
		if len(msg.OfAssistant.Content.OfArrayOfContentParts) > 0 {
			var parts []string
			for _, p := range msg.OfAssistant.Content.OfArrayOfContentParts {
				if p.OfText != nil {
					parts = append(parts, p.OfText.Text)
				}
			}
			return strings.Join(parts, "")
		}
	case msg.OfTool != nil:
		if msg.OfTool.Content.OfString.Value != "" {
			return msg.OfTool.Content.OfString.Value
		}
		if len(msg.OfTool.Content.OfArrayOfContentParts) > 0 {
			var parts []string
			for _, p := range msg.OfTool.Content.OfArrayOfContentParts {
				parts = append(parts, p.Text)
			}
			return strings.Join(parts, "")
		}
	case msg.OfDeveloper != nil:
		if msg.OfDeveloper.Content.OfString.Value != "" {
			return msg.OfDeveloper.Content.OfString.Value
		}
	}
	return ""
}

// extractConversationMessage converts a ChatCompletionMessageParamUnion to a ConversationMessage
func extractConversationMessage(msg openai.ChatCompletionMessageParamUnion) ConversationMessage {
	role := getMessageRole(msg)
	content := extractMessageContent(msg)

	cm := ConversationMessage{
		Role:    role,
		Content: content,
	}

	if msg.OfAssistant != nil {
		if len(msg.OfAssistant.ToolCalls) > 0 {
			cm.ToolCalls = sdkParamToolCallsToInterface(msg.OfAssistant.ToolCalls)
		}
	}
	if msg.OfTool != nil {
		cm.ToolCallID = msg.OfTool.ToolCallID
	}

	return cm
}

// sdkToolCallsToInterface converts SDK tool calls to interface{} for ConversationMessage.ToolCalls
func sdkToolCallsToInterface(toolCalls []openai.ChatCompletionMessageToolCall) interface{} {
	return toolCalls
}

// sdkParamToolCallsToInterface converts SDK param tool calls to interface{} for ConversationMessage.ToolCalls
func sdkParamToolCallsToInterface(toolCalls []openai.ChatCompletionMessageToolCallParam) interface{} {
	return toolCalls
}

// ============================================================
// DinTalClaw 文本嵌入式工具协议（Inline Tool Protocol）解析
// ============================================================

var (
	inlineToolUseRe    = regexp.MustCompile(`(?s)<tool_use>\s*(.*?)\s*</tool_use>`)
	inlineToolResultRe = regexp.MustCompile(`(?s)<tool_result>\s*(.*?)\s*</tool_result>`)
	// 匹配 DinTalClaw 模板中的 SYSTEM->USER 区间，用于忽略其中示例工具协议。
	inlineSystemToUserRe = regexp.MustCompile(`(?is)===\s*system\s*===.*?(===\s*user\s*===)`)
)

// InlineToolUse 表示从文本中提取的内嵌工具调用
type InlineToolUse struct {
	Name    string
	RawArgs string
}

// stripInlineProtocolTemplateNoise 忽略 DinTalClaw 模板中 SYSTEM 到 USER 之间的示例工具内容。
func stripInlineProtocolTemplateNoise(content string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	return inlineSystemToUserRe.ReplaceAllString(content, "$1")
}

// extractInlineToolUses 从消息内容中提取 <tool_use> 标签（DinTalClaw 协议）
func extractInlineToolUses(content string) []InlineToolUse {
	content = stripInlineProtocolTemplateNoise(content)
	matches := inlineToolUseRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	var result []InlineToolUse
	for _, m := range matches {
		body := strings.TrimSpace(m[1])
		if body == "" {
			continue
		}
		var parsed struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(body), &parsed); err == nil && parsed.Name != "" {
			result = append(result, InlineToolUse{
				Name:    parsed.Name,
				RawArgs: string(parsed.Args),
			})
		} else {
			result = append(result, InlineToolUse{
				Name:    "inline_tool",
				RawArgs: body,
			})
		}
	}
	return result
}

// extractInlineToolResults 从消息内容中提取 <tool_result> 标签内容（DinTalClaw 协议）
func extractInlineToolResults(content string) []string {
	content = stripInlineProtocolTemplateNoise(content)
	matches := inlineToolResultRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	var result []string
	for _, m := range matches {
		body := strings.TrimSpace(m[1])
		if body != "" {
			result = append(result, body)
		}
	}
	return result
}

// hasInlineToolUse 检查内容是否包含 <tool_use> 标签
func hasInlineToolUse(content string) bool {
	content = stripInlineProtocolTemplateNoise(content)
	return inlineToolUseRe.MatchString(content)
}

// hasInlineToolResult 检查内容是否包含 <tool_result> 标签
func hasInlineToolResult(content string) bool {
	content = stripInlineProtocolTemplateNoise(content)
	return inlineToolResultRe.MatchString(content)
}

// generateInlineToolCallID 生成内嵌工具调用的合成 ID
func generateInlineToolCallID(msgIndex, toolIndex int) string {
	return fmt.Sprintf("inline_%d_%d", msgIndex, toolIndex)
}
