package skillagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
)

// collectAgentOutput iterates ADK events and collects final output plus model usage.
func collectAgentOutput(ctx context.Context, iter *adk.AsyncIterator[*adk.AgentEvent]) (string, TokenUsage, error) {
	var lastContent string
	var lastErr error
	totalUsage := TokenUsage{}

	for {
		event, ok := iter.Next()
		if !ok {
			break
		}

		if event.Err != nil {
			lastErr = event.Err
			continue
		}

		// 使用 ADK 的 GetMessage 提取消息内容
		msg, _, err := adk.GetMessage(event)
		if err != nil {
			continue
		}
		totalUsage = addTokenUsage(totalUsage, tokenUsageFromMessage(msg))
		if msg != nil && msg.Content != "" {
			lastContent = msg.Content
		}
	}

	if lastErr != nil && lastContent == "" {
		return "", totalUsage, lastErr
	}

	return lastContent, totalUsage, nil
}

// streamAgentEvents 遍历 ADK AsyncIterator，将 AgentEvent 转换为 StreamEvent 发送到 emitter。
// 用于流式 ExecuteStream 调用。
func streamAgentEvents(ctx context.Context, iter *adk.AsyncIterator[*adk.AgentEvent], emitter *StreamEventEmitter) {
	toolCallCount := 0

	for {
		event, ok := iter.Next()
		if !ok {
			break
		}

		if event.Err != nil {
			emitter.EmitError(event.Err)
			continue
		}

		// 处理 Action 事件
		if event.Action != nil {
			if event.Action.Exit {
				break
			}
		}

		// 处理 Output 事件
		if event.Output != nil && event.Output.MessageOutput != nil {
			msg, _, err := adk.GetMessage(event)
			if err == nil && msg != nil && msg.Content != "" {
				emitter.EmitPartialOutput(msg.Content)
			}

			// 检查工具调用
			if msg != nil && len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					toolCallCount++
					args := ""
					if tc.Function.Arguments != "" {
						args = tc.Function.Arguments
					}
					emitter.EmitToolCalling(tc.Function.Name, args)
				}
			}
		}
	}

	_ = toolCallCount
}

// buildADKMessages 将用户输入和额外上下文构建为 ADK AgentInput。
func buildADKMessages(userInput string, additionalContext string) *adk.AgentInput {
	message := userInput
	if additionalContext != "" {
		message = userInput + "\n\nAdditional Context:\n" + additionalContext
	}

	return &adk.AgentInput{
		Messages: []adk.Message{
			{
				Role:    "user",
				Content: message,
			},
		},
	}
}

// buildSkillInstruction 构建 ADK Agent 的 Instruction（系统提示词）。
func buildSkillInstruction(prefix, suffix, skillName, description, instructions string, resources []string) string {
	var parts []string

	if prefix != "" {
		parts = append(parts, prefix)
	}

	header := fmt.Sprintf("You are executing the skill: %s\n\nSkill Description: %s", skillName, description)
	parts = append(parts, header)

	if instructions != "" {
		parts = append(parts, "## Skill Instructions\n\n"+instructions)
	}

	if len(resources) > 0 {
		parts = append(parts, "## Available Resources\n\n"+strings.Join(resources, "\n"))
	}

	if suffix != "" {
		parts = append(parts, suffix)
	}

	return strings.Join(parts, "\n\n")
}
