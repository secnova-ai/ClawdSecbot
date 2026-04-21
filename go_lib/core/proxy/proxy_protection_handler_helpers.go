package proxy

import (
	"fmt"
	"strings"

	"go_lib/core/shepherd"
	"go_lib/core/skillscan"

	"github.com/openai/openai-go"
)

const recentConversationMessageWindow = 3

func appendRequestMessagesToTruthRecord(r *TruthRecord, req *openai.ChatCompletionNewParams) {
	if r == nil || req == nil || len(r.Messages) > 0 {
		return
	}
	r.Messages = extractCurrentRoundRecordMessages(req.Messages)
}

func appendAssistantMessageToTruthRecord(r *TruthRecord, content string) {
	if r == nil {
		return
	}
	assistantIndex := len(r.Messages)
	if assistantIndex < r.MessageCount {
		assistantIndex = r.MessageCount
	}
	r.Messages = append(r.Messages, RecordMessage{
		Index:   assistantIndex,
		Role:    "assistant",
		Content: truncateToBytes(content, maxRecordMessageBytes),
	})
}

// extractCurrentRoundRecordMessages keeps only the current round:
//   - if tail role is tool, keep the contiguous tail tool block
//   - otherwise keep messages from the last user to tail
//
// This avoids replaying full conversation history into each TruthRecord card.
func extractCurrentRoundRecordMessages(messages []openai.ChatCompletionMessageParamUnion) []RecordMessage {
	if len(messages) == 0 {
		return nil
	}

	end := len(messages) - 1
	start := 0

	if messages[end].OfTool != nil {
		start = end
		for start >= 0 && messages[start].OfTool != nil {
			start--
		}
		start++
	} else {
		lastUser := -1
		for i := end; i >= 0; i-- {
			if strings.EqualFold(getMessageRole(messages[i]), "user") {
				lastUser = i
				break
			}
		}
		if lastUser >= 0 {
			start = lastUser
		} else {
			start = end
		}
	}

	if start < 0 {
		start = 0
	}
	out := make([]RecordMessage, 0, end-start+1)
	for i := start; i <= end; i++ {
		msg := messages[i]
		out = append(out, RecordMessage{
			Index:   i,
			Role:    getMessageRole(msg),
			Content: truncateToBytes(extractMessageContent(msg), maxRecordMessageBytes),
		})
	}
	return out
}

func detectConversationContinuation(currentAll []NormalizedMessage, prevRecent []NormalizedMessage, currentCount, prevCount int) bool {
	if len(currentAll) == 0 || len(prevRecent) == 0 {
		return false
	}
	if currentCount <= prevCount {
		return false
	}
	if prevCount > len(currentAll) {
		return false
	}
	start := prevCount - len(prevRecent)
	if start < 0 {
		start = 0
	}
	end := start + len(prevRecent)
	if end > len(currentAll) {
		return false
	}
	for i := range prevRecent {
		if currentAll[start+i] != prevRecent[i] {
			return false
		}
	}
	return true
}

func (pp *ProxyProtection) evaluateConversationWindow(req *openai.ChatCompletionNewParams) (int, int, bool) {
	currentAll := extractComparableMessages(req.Messages)
	currentRecent := extractRecentComparableMessages(req.Messages, recentConversationMessageWindow)
	currentCount := len(req.Messages)

	pp.metricsMu.Lock()
	prevRecent := cloneNormalizedMessages(pp.lastRecentMessages)
	prevCount := pp.lastRecentMessageCount
	isContinuation := detectConversationContinuation(currentAll, prevRecent, currentCount, prevCount)
	if !isContinuation {
		pp.currentConversationTokenUsage = 0
	}
	pp.lastRecentMessages = cloneNormalizedMessages(currentRecent)
	pp.lastRecentMessageCount = currentCount
	currentUsage := pp.currentConversationTokenUsage
	pp.metricsMu.Unlock()

	return currentUsage, len(currentRecent), isContinuation
}

func (pp *ProxyProtection) addConversationTokens(totalTokens int) int {
	pp.metricsMu.Lock()
	defer pp.metricsMu.Unlock()
	if totalTokens > 0 {
		pp.currentConversationTokenUsage += totalTokens
	}
	return pp.currentConversationTokenUsage
}

func (pp *ProxyProtection) currentDailyTokenUsage() int {
	pp.configMu.RLock()
	initialUsage := pp.initialDailyUsage
	pp.configMu.RUnlock()

	pp.metricsMu.Lock()
	runtimeUsage := pp.totalTokens - pp.baselineTotalTokens
	pp.metricsMu.Unlock()
	if runtimeUsage < 0 {
		runtimeUsage = 0
	}
	return initialUsage + runtimeUsage
}

func (pp *ProxyProtection) updateRecordTokenTotals(requestID string, promptTokens, completionTokens, conversationTokens, dailyTokens int) {
	pp.updateTruthRecord(requestID, func(r *TruthRecord) {
		r.PromptTokens = promptTokens
		r.CompletionTokens = completionTokens
		r.ConversationTokens = conversationTokens
		r.DailyTokens = dailyTokens
	})
}

// collectTailToolResults returns only the latest contiguous tail block of
// role=tool messages from a request message list.
func collectTailToolResults(messages []openai.ChatCompletionMessageParamUnion) map[string]string {
	results := make(map[string]string)
	if len(messages) == 0 {
		return results
	}
	end := len(messages) - 1
	// Only treat it as "tool results round" when the tail itself is tool.
	// If the last message is user/assistant, this request is not a tool-result
	// continuation and must not back-scan historical tool messages.
	if messages[end].OfTool == nil {
		return results
	}

	start := end
	for start >= 0 && messages[start].OfTool != nil {
		start--
	}
	for i := start + 1; i <= end; i++ {
		msg := messages[i]
		if msg.OfTool == nil {
			continue
		}
		toolCallID := strings.TrimSpace(msg.OfTool.ToolCallID)
		if toolCallID == "" {
			continue
		}
		results[toolCallID] = extractMessageContent(msg)
	}
	return results
}

func summarizeTailToolMessages(messages []openai.ChatCompletionMessageParamUnion) (int, int, int, []string) {
	if len(messages) == 0 {
		return 0, 0, 0, nil
	}
	end := len(messages) - 1
	if messages[end].OfTool == nil {
		return 0, 0, 0, nil
	}

	start := end
	for start >= 0 && messages[start].OfTool != nil {
		start--
	}

	tailToolCount := 0
	withIDCount := 0
	missingIDCount := 0
	ids := make([]string, 0, end-start)
	for i := start + 1; i <= end; i++ {
		msg := messages[i]
		if msg.OfTool == nil {
			continue
		}
		tailToolCount++
		toolCallID := strings.TrimSpace(msg.OfTool.ToolCallID)
		if toolCallID == "" {
			missingIDCount++
			continue
		}
		withIDCount++
		ids = append(ids, toolCallID)
	}
	return tailToolCount, withIDCount, missingIDCount, ids
}

// formatQuotaExceededMessage builds localized quota exceeded text.
func formatQuotaExceededMessage(quotaType string, current, limit int) string {
	lang := shepherd.NormalizeShepherdLanguage(skillscan.GetLanguageFromAppSettings())

	if lang == "zh" {
		var quotaName string
		if quotaType == "session" || quotaType == "conversation" {
			quotaName = "单会话 Token 配额已用尽"
		} else {
			quotaName = "每日 Token 配额已用尽"
		}
		return fmt.Sprintf("[ClawSecbot] 状态: QUOTA_EXCEEDED | 原因: %s (%d/%d)\n\n当前请求已被拦截，请调整配额设置或等待配额重置。",
			quotaName, current, limit)
	}

	var quotaName string
	if quotaType == "session" || quotaType == "conversation" {
		quotaName = "Conversation token quota exceeded"
	} else {
		quotaName = "Daily token quota exceeded"
	}
	return fmt.Sprintf("[ClawSecbot] Status: QUOTA_EXCEEDED | Reason: %s (%d/%d)\n\nThis request has been blocked. Please adjust quota settings or wait for quota reset.",
		quotaName, current, limit)
}
