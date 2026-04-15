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

// NormalizedMessage is a compact comparable representation used for
// heuristic conversation continuity checks.
type NormalizedMessage struct {
	Role    string
	Content string
}

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

func normalizeComparableMessage(msg ConversationMessage) NormalizedMessage {
	return NormalizedMessage{
		Role:    strings.TrimSpace(strings.ToLower(msg.Role)),
		Content: strings.TrimSpace(msg.Content),
	}
}

func extractRecentComparableMessages(messages []openai.ChatCompletionMessageParamUnion, limit int) []NormalizedMessage {
	all := extractComparableMessages(messages)
	if limit <= 0 || len(all) <= limit {
		return all
	}
	return cloneNormalizedMessages(all[len(all)-limit:])
}

func extractComparableMessages(messages []openai.ChatCompletionMessageParamUnion) []NormalizedMessage {
	out := make([]NormalizedMessage, 0, len(messages))
	for _, msg := range messages {
		out = append(out, normalizeComparableMessage(extractConversationMessage(msg)))
	}
	return out
}

func cloneNormalizedMessages(messages []NormalizedMessage) []NormalizedMessage {
	if len(messages) == 0 {
		return nil
	}
	out := make([]NormalizedMessage, len(messages))
	copy(out, messages)
	return out
}

func containsRecentMessageWindow(messages []NormalizedMessage, window []NormalizedMessage) bool {
	if len(window) == 0 {
		return len(messages) > 0
	}
	if len(messages) < len(window) {
		return false
	}
	maxStart := len(messages) - len(window)
	for start := 0; start <= maxStart; start++ {
		matched := true
		for i := range window {
			if messages[start+i] != window[i] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
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
	// DinTalClaw 段落标记
	dintalclawSectionRe   = regexp.MustCompile(`(?i)===\s*(SYSTEM|USER|ASSISTANT)\s*===`)
	dintalclawAssistantRe = regexp.MustCompile(`(?i)===\s*ASSISTANT\s*===`)
	dintalclawUserRe      = regexp.MustCompile(`(?i)===\s*USER\s*===`)
	dintalclawSystemRe    = regexp.MustCompile(`(?i)===\s*SYSTEM\s*===`)
)

// InlineToolUse 表示从文本中提取的内嵌工具调用
type InlineToolUse struct {
	Name    string
	RawArgs string
}

// extractAllDinTalClawSections 提取 DinTalClaw 消息中所有匹配 sectionRe 的段落内容并合并。
// DinTalClaw 可能在单条消息中包含多轮对话（多个 ASSISTANT / USER 段），
// 必须遍历全部匹配段才能完整提取 <tool_use> 或 <tool_result>。
func extractAllDinTalClawSections(content string, sectionRe *regexp.Regexp) string {
	locs := sectionRe.FindAllStringIndex(content, -1)
	if len(locs) == 0 {
		return ""
	}
	var parts []string
	for _, loc := range locs {
		body := content[loc[1]:]
		if nextLoc := dintalclawSectionRe.FindStringIndex(body); nextLoc != nil {
			body = body[:nextLoc[0]]
		}
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n")
}

// contentForToolUseExtraction 为 <tool_use> 提取准备内容。
// DinTalClaw 消息返回所有 ASSISTANT 段的合并内容；无段落标记的消息返回原文。
func contentForToolUseExtraction(content string) string {
	if !dintalclawSectionRe.MatchString(content) {
		return content
	}
	return extractAllDinTalClawSections(content, dintalclawAssistantRe)
}

// contentForToolResultExtraction 为 <tool_result> 提取准备内容。
// DinTalClaw 消息返回所有 USER 段的合并内容（自动排除 SYSTEM 段模板噪声）；无段落标记的消息返回原文。
func contentForToolResultExtraction(content string) string {
	if !dintalclawSectionRe.MatchString(content) {
		return content
	}
	return extractAllDinTalClawSections(content, dintalclawUserRe)
}

// extractInlineToolUses 从消息内容中提取 <tool_use> 标签（DinTalClaw 协议：仅 ASSISTANT 段）
func extractInlineToolUses(content string) []InlineToolUse {
	content = contentForToolUseExtraction(content)
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

// extractInlineToolResults 从消息内容中提取 <tool_result> 标签内容（DinTalClaw 协议：仅 USER 段）
func extractInlineToolResults(content string) []string {
	content = contentForToolResultExtraction(content)
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

// hasInlineToolUse 检查内容的 ASSISTANT 段是否包含 <tool_use> 标签
func hasInlineToolUse(content string) bool {
	return inlineToolUseRe.MatchString(contentForToolUseExtraction(content))
}

// hasInlineToolResult 检查内容的 USER 段是否包含 <tool_result> 标签
func hasInlineToolResult(content string) bool {
	return inlineToolResultRe.MatchString(contentForToolResultExtraction(content))
}

// generateInlineToolCallID 生成内嵌工具调用的合成 ID（请求级唯一）。
// 说明：
// - 仅使用 msgIndex/toolIndex 会在不同请求间重复，导致前端跨卡去重误判。
// - 将 requestID 纳入 ID，可保证 DinTalClaw 内嵌工具结果在跨请求场景下不被误去重。
func generateInlineToolCallID(requestID string, msgIndex, toolIndex int) string {
	normalizedRequestID := strings.TrimSpace(requestID)
	if normalizedRequestID == "" {
		normalizedRequestID = "unknown"
	}
	normalizedRequestID = strings.ReplaceAll(normalizedRequestID, " ", "_")
	return fmt.Sprintf("inline_%s_%d_%d", normalizedRequestID, msgIndex, toolIndex)
}
