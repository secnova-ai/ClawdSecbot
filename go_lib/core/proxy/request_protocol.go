package proxy

import (
	"strings"

	"github.com/openai/openai-go"
)

type toolCallRef struct {
	ID       string
	FuncName string
	RawArgs  string
}

// requestProtocolAnalysis is the normalized request-side protocol view used by
// protection hooks. It keeps OpenAI tool calls and inline DinTalClaw tool tags
// behind the same IDs/results shape before policy code runs.
type requestProtocolAnalysis struct {
	ToolCallsInHistory      []toolCallRef
	LatestAssistantToolCall []toolCallRef
	LatestAssistantIndex    int
	HasToolResultMessages   bool
	ToolResultIndices       []int
	IsInlineToolProtocol    bool
	InlineToolResults       map[string]string
	LatestRoundToolCallIDs  map[string]bool
	TerminalLogs            []string
}

func analyzeRequestProtocol(requestID string, messages []openai.ChatCompletionMessageParamUnion) requestProtocolAnalysis {
	result := requestProtocolAnalysis{
		LatestAssistantIndex:   -1,
		LatestRoundToolCallIDs: make(map[string]bool),
	}

	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.OfAssistant == nil || len(msg.OfAssistant.ToolCalls) == 0 {
			continue
		}
		for _, tc := range msg.OfAssistant.ToolCalls {
			result.LatestAssistantToolCall = append(result.LatestAssistantToolCall, toolCallRef{
				ID:       tc.ID,
				FuncName: tc.Function.Name,
				RawArgs:  tc.Function.Arguments,
			})
		}
		result.LatestAssistantIndex = i
		result.TerminalLogs = append(result.TerminalLogs, formatSecurityFlowLog(securityFlowStageToolCall, "latest assistant tool_calls found: index=%d count=%d", i, len(result.LatestAssistantToolCall)))
		for idx, tc := range result.LatestAssistantToolCall {
			result.TerminalLogs = append(result.TerminalLogs, formatSecurityFlowLog(securityFlowStageToolCall, "latest assistant tool_call: index=%d id=%s", idx, tc.ID))
		}

		hasToolsFollowing := false
		for j := i + 1; j < len(messages); j++ {
			if messages[j].OfTool != nil {
				hasToolsFollowing = true
			} else if messages[j].OfUser != nil {
				result.TerminalLogs = append(result.TerminalLogs, formatSecurityFlowLog(securityFlowStageToolCallResult, "tool results already consumed by later user message: index=%d", j))
				result.LatestAssistantToolCall = nil
				result.LatestAssistantIndex = -1
				break
			}
		}
		if hasToolsFollowing && result.LatestAssistantIndex >= 0 {
			result.TerminalLogs = append(result.TerminalLogs, formatSecurityFlowLog(securityFlowStageToolCallResult, "tool results follow latest assistant tool_calls; detection will run"))
		}
		break
	}
	if len(result.LatestAssistantToolCall) == 0 {
		result.TerminalLogs = append(result.TerminalLogs, formatSecurityFlowLog(securityFlowStageToolCall, "no assistant tool_calls require detection"))
	}

	analyzeInlineToolProtocol(requestID, messages, &result)
	analyzeLatestRoundToolCallIDs(messages, &result)
	return result
}

func analyzeInlineToolProtocol(requestID string, messages []openai.ChatCompletionMessageParamUnion, result *requestProtocolAnalysis) {
	if result == nil {
		return
	}
	if len(result.LatestAssistantToolCall) == 0 {
		for i := len(messages) - 1; i >= 0; i-- {
			msg := messages[i]
			var content string
			if msg.OfAssistant != nil {
				content = extractMessageContent(msg)
			} else if msg.OfUser != nil {
				userContent := extractMessageContent(msg)
				if hasInlineToolUse(userContent) {
					content = userContent
				}
			}
			if content == "" {
				continue
			}
			inlineTools := extractInlineToolUses(content)
			if len(inlineTools) == 0 {
				continue
			}

			result.LatestAssistantIndex = i
			result.IsInlineToolProtocol = true
			for j, it := range inlineTools {
				syntheticID := generateInlineToolCallID(requestID, i, j)
				result.LatestAssistantToolCall = append(result.LatestAssistantToolCall, toolCallRef{
					ID:       syntheticID,
					FuncName: it.Name,
					RawArgs:  it.RawArgs,
				})
			}
			result.TerminalLogs = append(result.TerminalLogs, formatSecurityFlowLog(securityFlowStageToolCall, "inline tool_use found: count=%d message_index=%d", len(inlineTools), i))

			result.InlineToolResults = make(map[string]string)
			consumed := false

			if msg.OfUser != nil {
				sameResults := extractInlineToolResults(content)
				if len(sameResults) > 0 {
					result.HasToolResultMessages = true
					for k, toolResult := range sameResults {
						if k < len(result.LatestAssistantToolCall) {
							tcID := result.LatestAssistantToolCall[k].ID
							result.InlineToolResults[tcID] = toolResult
							result.ToolResultIndices = append(result.ToolResultIndices, i)
						}
					}
					result.TerminalLogs = append(result.TerminalLogs, formatSecurityFlowLog(securityFlowStageToolCallResult, "inline tool_result found in same user message: count=%d message_index=%d", len(sameResults), i))
				}
			}

			for j := i + 1; j < len(messages); j++ {
				nextMsg := messages[j]
				if nextMsg.OfUser != nil {
					nextContent := extractMessageContent(nextMsg)
					if hasInlineToolUse(nextContent) && !hasInlineToolResult(nextContent) {
						consumed = true
						result.TerminalLogs = append(result.TerminalLogs, formatSecurityFlowLog(securityFlowStageToolCallResult, "new inline tool_use cycle found; previous results consumed: message_index=%d", j))
						break
					}
					inlineResults := extractInlineToolResults(nextContent)
					if len(inlineResults) > 0 {
						result.HasToolResultMessages = true
						for k, toolResult := range inlineResults {
							if k < len(result.LatestAssistantToolCall) {
								tcID := result.LatestAssistantToolCall[k].ID
								result.InlineToolResults[tcID] = toolResult
								result.ToolResultIndices = append(result.ToolResultIndices, j)
							}
						}
						result.TerminalLogs = append(result.TerminalLogs, formatSecurityFlowLog(securityFlowStageToolCallResult, "inline tool_result found in user message: count=%d message_index=%d", len(inlineResults), j))
					}
				} else if nextMsg.OfAssistant != nil {
					nextAssistContent := extractMessageContent(nextMsg)
					if !hasInlineToolUse(nextAssistContent) {
						consumed = true
						result.TerminalLogs = append(result.TerminalLogs, formatSecurityFlowLog(securityFlowStageToolCallResult, "inline tool results consumed by assistant: message_index=%d", j))
					}
					break
				}
			}
			if consumed {
				result.LatestAssistantToolCall = nil
				result.LatestAssistantIndex = -1
				result.HasToolResultMessages = false
				result.InlineToolResults = nil
				result.IsInlineToolProtocol = false
			}
			break
		}
	}

	if result.IsInlineToolProtocol || len(result.LatestAssistantToolCall) > 0 {
		return
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.OfUser == nil {
			continue
		}
		content := extractMessageContent(msg)
		toolResults := extractInlineToolResults(content)
		if len(toolResults) == 0 {
			continue
		}
		result.IsInlineToolProtocol = true
		result.LatestAssistantIndex = i
		result.InlineToolResults = make(map[string]string)
		result.HasToolResultMessages = true
		for k, toolResult := range toolResults {
			synID := generateInlineToolCallID(requestID, i, k)
			ref := toolCallRef{
				ID:       synID,
				FuncName: "tool_result",
				RawArgs:  "",
			}
			result.LatestAssistantToolCall = append(result.LatestAssistantToolCall, ref)
			result.ToolCallsInHistory = append(result.ToolCallsInHistory, ref)
			result.InlineToolResults[synID] = toolResult
			result.ToolResultIndices = append(result.ToolResultIndices, i)
		}
		result.TerminalLogs = append(result.TerminalLogs, formatSecurityFlowLog(securityFlowStageToolCallResult, "orphan inline tool_result found without matching tool_use: count=%d message_index=%d", len(toolResults), i))
		break
	}
}

func analyzeLatestRoundToolCallIDs(messages []openai.ChatCompletionMessageParamUnion, result *requestProtocolAnalysis) {
	if result == nil {
		return
	}
	if result.LatestRoundToolCallIDs == nil {
		result.LatestRoundToolCallIDs = make(map[string]bool)
	}
	if result.IsInlineToolProtocol {
		if result.HasToolResultMessages {
			for _, tc := range result.LatestAssistantToolCall {
				result.LatestRoundToolCallIDs[tc.ID] = true
			}
		}
		return
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.OfAssistant == nil || len(msg.OfAssistant.ToolCalls) == 0 {
			continue
		}
		hasToolFollowing := false
		consumed := false
		for j := i + 1; j < len(messages); j++ {
			if messages[j].OfTool != nil {
				hasToolFollowing = true
			} else if hasToolFollowing {
				consumed = true
				break
			}
		}
		if hasToolFollowing && !consumed {
			for _, tc := range msg.OfAssistant.ToolCalls {
				result.LatestRoundToolCallIDs[strings.TrimSpace(tc.ID)] = true
			}
		}
		break
	}
}
