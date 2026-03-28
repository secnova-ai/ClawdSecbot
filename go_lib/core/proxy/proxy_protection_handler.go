package proxy

// proxy_protection_handler.go 包含 ProxyProtection 的请求/响应处理方法。
// 从 proxy_protection.go 中拆分，保持单文件不超过 1500 行。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	chatmodelrouting "go_lib/chatmodel-routing"
	"go_lib/core/logging"
	"go_lib/core/shepherd"
	"go_lib/core/skillscan"

	"github.com/openai/openai-go"
)

// onRequest handles incoming requests
func (pp *ProxyProtection) onRequest(ctx context.Context, req *openai.ChatCompletionNewParams, rawBody []byte) (*chatmodelrouting.FilterRequestResult, bool) {
	// Extract model name from request
	modelName := string(req.Model)

	// Get config values with lock
	pp.configMu.RLock()
	sessionLimit := pp.singleSessionTokenLimit
	dailyLimit := pp.dailyTokenLimit
	initialUsage := pp.initialDailyUsage
	pp.configMu.RUnlock()

	// Check single-session token limit (total tokens since proxy start)
	// 配额是硬性预算限制，与审计模式无关，只要配置了限额且超额就必须阻断
	if sessionLimit > 0 {
		pp.metricsMu.Lock()
		sessionTotal := pp.totalTokens - pp.baselineTotalTokens
		if sessionTotal < 0 {
			sessionTotal = 0
		}
		pp.metricsMu.Unlock()

		pp.sendTerminalLog(fmt.Sprintf("📊 单会话 Token 用量: %d / %d", sessionTotal, sessionLimit))

		if sessionTotal >= sessionLimit {
			reason := fmt.Sprintf("单会话 Token 配额已用尽 (%d/%d)", sessionTotal, sessionLimit)
			pp.sendTerminalLog(fmt.Sprintf(">>> %s,已拦截请求 <<<", reason))
			pp.sendLog("proxy_session_quota_exceeded", map[string]interface{}{
				"current": sessionTotal,
				"limit":   sessionLimit,
				"model":   modelName,
			})

			// Initialize audit log for this blocked request
			pp.auditMu.Lock()
			pp.requestStartTime = time.Now()
			pp.currentRequestID = fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), pp.requestCount+1)
			pp.currentAuditLog = &AuditLog{
				ID:         generateAuditLogID(),
				Timestamp:  pp.requestStartTime.Format(time.RFC3339),
				RequestID:  pp.currentRequestID,
				AssetName:  pp.assetName,
				AssetID:    pp.assetID,
				Model:      modelName,
				HasRisk:    true,
				RiskLevel:  "QUOTA",
				RiskReason: reason,
				Action:     "BLOCK",
			}
			pp.auditMu.Unlock()

			pp.saveAuditLog(true, "QUOTA", reason, 100, "BLOCK")
			mockMsg := formatQuotaExceededMessage("session", sessionTotal, sessionLimit)
			return &chatmodelrouting.FilterRequestResult{MockContent: mockMsg}, false
		}
	}

	// Check daily token limit
	// 配额是硬性预算限制，与审计模式无关，只要配置了限额且超额就必须阻断
	if dailyLimit > 0 {
		pp.metricsMu.Lock()
		runtimeUsage := pp.totalTokens - pp.baselineTotalTokens
		if runtimeUsage < 0 {
			runtimeUsage = 0
		}
		currentTotal := initialUsage + runtimeUsage
		pp.metricsMu.Unlock()

		pp.sendTerminalLog(fmt.Sprintf("📊 每日 Token 用量: %d / %d (当日基线: %d, 本次会话新增: %d)", currentTotal, dailyLimit, initialUsage, runtimeUsage))

		if currentTotal >= dailyLimit {
			reason := fmt.Sprintf("每日 Token 配额已用尽 (%d/%d)", currentTotal, dailyLimit)
			pp.sendTerminalLog(fmt.Sprintf(">>> %s,已拦截请求 <<<", reason))
			pp.sendLog("proxy_quota_exceeded", map[string]interface{}{
				"current": currentTotal,
				"limit":   dailyLimit,
				"model":   modelName,
			})

			// Initialize audit log for this blocked request
			pp.auditMu.Lock()
			pp.requestStartTime = time.Now()
			pp.currentRequestID = fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), pp.requestCount+1)
			pp.currentAuditLog = &AuditLog{
				ID:         generateAuditLogID(),
				Timestamp:  pp.requestStartTime.Format(time.RFC3339),
				RequestID:  pp.currentRequestID,
				AssetName:  pp.assetName,
				AssetID:    pp.assetID,
				Model:      modelName,
				HasRisk:    true,
				RiskLevel:  "QUOTA",
				RiskReason: reason,
				Action:     "BLOCK",
			}
			pp.auditMu.Unlock()

			pp.saveAuditLog(true, "QUOTA", reason, 100, "BLOCK")
			mockMsg := formatQuotaExceededMessage("daily", currentTotal, dailyLimit)
			return &chatmodelrouting.FilterRequestResult{MockContent: mockMsg}, false
		}
	}

	// Increment request count
	pp.metricsMu.Lock()
	pp.requestCount++
	currentRequestNum := pp.requestCount
	pp.metricsMu.Unlock()
	// 请求进入即同步一次指标，确保顶部消息计数及时更新
	pp.sendMetricsToCallback()

	// Initialize audit log for this request
	pp.auditMu.Lock()
	pp.requestStartTime = time.Now()
	pp.currentRequestID = fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), currentRequestNum)
	pp.currentAuditLog = &AuditLog{
		ID:        generateAuditLogID(),
		Timestamp: pp.requestStartTime.Format(time.RFC3339),
		RequestID: pp.currentRequestID,
		AssetName: pp.assetName,
		AssetID:   pp.assetID,
		Model:     modelName,
		HasRisk:   false,
		Action:    "ALLOW",
	}
	pp.auditMu.Unlock()

	pp.sendLog("proxy_new_request", map[string]interface{}{
		"model": modelName,
	})
	// 打印安全模型和Bot模型转发地址信息
	securityModel := ""
	if pp.shepherdGate != nil {
		securityModel = pp.shepherdGate.GetModelName()
	}
	logging.Info("[ProxyProtection] onRequest: model=%s, messageCount=%d, requestID=%s, securityModel=%s, botTargetURL=%s", modelName, len(req.Messages), pp.currentRequestID, securityModel, pp.targetURL)
	pp.sendLog("proxy_request_info", map[string]interface{}{
		"model":        modelName,
		"messageCount": len(req.Messages),
		"stream":       "unknown",
	})
	// Debug: Print all message roles for troubleshooting
	var roles []string
	for _, msg := range req.Messages {
		roles = append(roles, getMessageRole(msg))
	}
	pp.sendTerminalLog(fmt.Sprintf("请求消息角色: %v", roles))

	// Check for tool result messages (role=tool) - only these trigger detection
	type toolCallRef struct {
		ID       string
		FuncName string
		RawArgs  string
	}
	var toolCallsInHistory []toolCallRef
	var latestAssistantToolCalls []toolCallRef
	var latestAssistantIndex int = -1
	hasToolResultMessages := false

	// Build request content summary for audit log
	var requestContentParts []string

	// Collect tool result message indices for detection
	var toolResultIndices []int

	// First pass: find the latest assistant message with tool_calls
	// AND check if tool messages immediately follow it (before any user message)
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if msg.OfAssistant != nil && len(msg.OfAssistant.ToolCalls) > 0 {
			for _, tc := range msg.OfAssistant.ToolCalls {
				latestAssistantToolCalls = append(latestAssistantToolCalls, toolCallRef{
					ID:       tc.ID,
					FuncName: tc.Function.Name,
					RawArgs:  tc.Function.Arguments,
				})
			}
			latestAssistantIndex = i
			pp.sendTerminalLog(fmt.Sprintf("找到最新的 assistant tool_calls 在索引 %d: %d 个工具调用", i, len(latestAssistantToolCalls)))
			for idx, tc := range latestAssistantToolCalls {
				pp.sendTerminalLog(fmt.Sprintf("  [%d] ID=%s", idx, tc.ID))
			}

			// Check if tool messages immediately follow this assistant
			// and no user message has been sent after the tool messages
			hasToolsFollowing := false
			for j := i + 1; j < len(req.Messages); j++ {
				if req.Messages[j].OfTool != nil {
					hasToolsFollowing = true
				} else if req.Messages[j].OfUser != nil {
					// Found a user message after tools, these tools are already processed
					pp.sendTerminalLog(fmt.Sprintf("在索引 %d 发现用户消息,说明工具结果已被处理,不触发检测", j))
					latestAssistantToolCalls = nil
					latestAssistantIndex = -1
					break
				}
			}

			if hasToolsFollowing && latestAssistantIndex >= 0 {
				pp.sendTerminalLog("工具调用后紧接着是工具结果,且之后没有用户消息,将触发检测")
			}
			break
		}
	}
	if len(latestAssistantToolCalls) == 0 {
		pp.sendTerminalLog("未找到需要检测的 assistant tool_calls")
	}

	// Initialize context messages for ShepherdGate
	pp.mu.Lock()
	pp.lastContextMessages = make([]ConversationMessage, 0, len(req.Messages))
	pp.mu.Unlock()

	// Log each message in the request
	for i, msg := range req.Messages {
		cm := extractConversationMessage(msg)
		role := cm.Role
		content := cm.Content

		// Store in context for ShepherdGate
		pp.mu.Lock()
		pp.lastContextMessages = append(pp.lastContextMessages, cm)
		pp.mu.Unlock()

		// Truncate long content for display
		displayContent := content
		if len(displayContent) > 200 {
			displayContent = displayContent[:200] + "...(truncated)"
		}
		pp.sendLog("proxy_message_info", map[string]interface{}{
			"index":   i,
			"role":    role,
			"content": displayContent,
		})

		// Track the last user message for audit (will be used after loop)
		// 更新成员变量而非局部变量，跨请求持久化防止上下文压缩丢失
		if role == "user" {
			pp.mu.Lock()
			pp.lastUserMessageContent = content
			pp.mu.Unlock()
		}

		// Collect tool calls from assistant messages (for context, not for triggering detection)
		if msg.OfAssistant != nil && len(msg.OfAssistant.ToolCalls) > 0 {
			for _, tc := range msg.OfAssistant.ToolCalls {
				toolCallsInHistory = append(toolCallsInHistory, toolCallRef{
					ID:       tc.ID,
					FuncName: tc.Function.Name,
				})
			}
		}

		// Collect tool result messages (role=tool means a tool was executed and returned)
		// ONLY trigger detection if:
		// 1. This tool result corresponds to the latest assistant tool_calls
		// 2. This tool message appears AFTER the latest assistant message (not historical)
		if msg.OfTool != nil {
			toolCallID := msg.OfTool.ToolCallID
			toolContent := content
			if len(toolContent) > 300 {
				toolContent = toolContent[:300] + "...(truncated)"
			}
			pp.sendLog("proxy_tool_result_content", map[string]interface{}{
				"index":   i,
				"tool_id": toolCallID,
				"content": toolContent,
			})
			pp.sendTerminalLog(fmt.Sprintf("发现 tool 消息在索引 %d，tool_call_id=%s", i, toolCallID))
			// Check if this tool result is AFTER the latest assistant message
			if latestAssistantIndex >= 0 && i > latestAssistantIndex {
				// Check if this tool result matches any tool call ID from the latest assistant message
				if len(latestAssistantToolCalls) > 0 {
					matched := false
					for _, tc := range latestAssistantToolCalls {
						if tc.ID == toolCallID {
							pp.sendTerminalLog(fmt.Sprintf("✓ tool_call_id 匹配成功且在最新 assistant 之后: %s", toolCallID))
							hasToolResultMessages = true
							toolResultIndices = append(toolResultIndices, i)
							matched = true
							break
						}
					}
					if !matched {
						pp.sendTerminalLog(fmt.Sprintf("✗ tool_call_id 不匹配,跳过此历史 tool 消息: %s", toolCallID))
					}
				} else {
					pp.sendTerminalLog("✗ 没有最新的 assistant tool_calls,跳过此 tool 消息")
				}
			} else {
				pp.sendTerminalLog(fmt.Sprintf("✗ tool 消息在索引 %d,但最新 assistant 在索引 %d,这是历史 tool 消息,跳过", i, latestAssistantIndex))
			}
		}
	}

	// Map tool_call_id to result content for accurate matching
	toolResultsMap := make(map[string]string)
	for _, msg := range req.Messages {
		if msg.OfTool != nil {
			toolCallID := msg.OfTool.ToolCallID
			if toolCallID != "" {
				content := extractMessageContent(msg)
				toolResultsMap[strings.TrimSpace(toolCallID)] = content
			}
		}
	}

	// Add last user message to request content for audit log
	pp.mu.Lock()
	cachedUserMsg := pp.lastUserMessageContent
	pp.mu.Unlock()
	if cachedUserMsg != "" {
		requestContentParts = append(requestContentParts, truncateString(cachedUserMsg, 500))
	}

	// Update audit log with request content
	pp.auditMu.Lock()
	if pp.currentAuditLog != nil {
		pp.currentAuditLog.RequestContent = strings.Join(requestContentParts, "\n")
		// Add tool calls to audit log
		for _, tc := range toolCallsInHistory {
			isSensitive := false
			if pp.toolValidator != nil {
				isSensitive = pp.toolValidator.IsSensitive(tc.FuncName)
			}

			// Lookup result using ID
			tcID := strings.TrimSpace(tc.ID)
			result := ""
			if val, ok := toolResultsMap[tcID]; ok {
				result = truncateString(val, 2000)
			}

			pp.currentAuditLog.ToolCalls = append(pp.currentAuditLog.ToolCalls, AuditToolCall{
				Name:        tc.FuncName,
				Arguments:   "", // Arguments not available from toolCallRef
				IsSensitive: isSensitive,
				Result:      result,
			})
		}
	}
	pp.auditMu.Unlock()

	// Debug: Log detection trigger decision
	_ = toolResultIndices
	pp.sendTerminalLog(fmt.Sprintf("检测触发判断: hasToolResultMessages=%v, 工具结果数量=%d", hasToolResultMessages, len(toolResultIndices)))

	// Store request in stream buffer
	pp.streamBuffer.ClearAll()
	pp.streamBuffer.SetRequest(req, rawBody)

	// ShepherdGate 检测：当工具执行结果返回 LLM 前进行安全分析
	if hasToolResultMessages && pp.shepherdGate != nil {
		// 检查恢复状态：用户已确认则跳过检测直接放行
		pp.ensureRecoveryMutex()
		pp.recoveryMu.Lock()
		armed := pp.pendingRecoveryArmed
		pp.recoveryMu.Unlock()

		if armed {
			pp.clearPendingToolCallRecovery()
			pp.sendTerminalLog("🔄 用户已确认恢复，跳过 ShepherdGate 检测，放行请求")
			pp.sendLog("proxy_tool_result_recovery_allowed", map[string]interface{}{
				"armed": true,
			})
		} else {
			pp.configMu.RLock()
			auditOnlyForShepherd := pp.auditOnly
			pp.configMu.RUnlock()

			// 仅审计模式：跳过 ShepherdGate 风险研判，直接放行
			if auditOnlyForShepherd {
				logging.Info("[ProxyProtection] Audit-only mode, skipping ShepherdGate analysis")
				pp.sendTerminalLog("📋 仅审计模式，跳过 ShepherdGate 检测，直接放行")
			} else {
				// 构建 ToolCallInfo 和 ToolResultInfo
				var toolCallInfos []ToolCallInfo
				for _, tcRef := range latestAssistantToolCalls {
					info := ToolCallInfo{
						Name:       tcRef.FuncName,
						RawArgs:    tcRef.RawArgs,
						ToolCallID: tcRef.ID,
					}
					if tcRef.RawArgs != "" {
						var args map[string]interface{}
						if err := json.Unmarshal([]byte(tcRef.RawArgs), &args); err == nil {
							info.Arguments = args
						}
					}
					toolCallInfos = append(toolCallInfos, info)
				}

				var toolResultInfos []ToolResultInfo
				for _, tcRef := range latestAssistantToolCalls {
					tcID := strings.TrimSpace(tcRef.ID)
					if content, ok := toolResultsMap[tcID]; ok {
						toolResultInfos = append(toolResultInfos, ToolResultInfo{
							ToolCallID: tcRef.ID,
							FuncName:   tcRef.FuncName,
							Content:    content,
						})
					}
				}

				var toolNames []string
				for _, tc := range toolCallInfos {
					toolNames = append(toolNames, tc.Name)
				}
				pp.sendTerminalLog(fmt.Sprintf("🔍 ShepherdGate 正在检查 %d 个工具结果: %s", len(toolResultInfos), strings.Join(toolNames, ", ")))

				pp.mu.Lock()
				contextMessages := pp.lastContextMessages
				cachedLastUserMsg := pp.lastUserMessageContent
				pp.mu.Unlock()

				securityModel := pp.shepherdGate.GetModelName()
				logging.Info("[ProxyProtection] ShepherdGate tool result detection triggered: toolCalls=%d, toolResults=%d, securityModel=%s", len(toolCallInfos), len(toolResultInfos), securityModel)

				// 使用代理服务的生命周期 context（pp.ctx）而非请求 context，
				// 防止客户端断连或请求超时导致 ShepherdGate 分析中途取消，影响代理正常转发。
				decision, err := pp.shepherdGate.CheckToolCall(pp.ctx, contextMessages, toolCallInfos, toolResultInfos, cachedLastUserMsg)

				// 更新分析计数
				pp.statsMu.Lock()
				pp.analysisCount++
				pp.statsMu.Unlock()
				pp.sendMetricsToCallback()

				if decision != nil && decision.Usage != nil {
					pp.metricsMu.Lock()
					pp.auditTokens += decision.Usage.TotalTokens
					pp.auditPromptTokens += decision.Usage.PromptTokens
					pp.auditCompletionTokens += decision.Usage.CompletionTokens
					pp.metricsMu.Unlock()
					pp.sendMetricsToCallback()
					pp.sendTerminalLog(fmt.Sprintf("📊 ShepherdGate Token Usage: %d (Prompt: %d, Completion: %d)",
						decision.Usage.TotalTokens, decision.Usage.PromptTokens, decision.Usage.CompletionTokens))
				}

				if err != nil {
					// fail-open: 分析失败时放行
					logging.Error("[ProxyProtection] ShepherdGate tool result check failed: %v, fail-open", err)
				} else if decision.Status != "ALLOWED" {
					logging.Info("[ProxyProtection] ShepherdGate tool result decision: status=%s, reason=%s", decision.Status, decision.Reason)

					// 拦截：存储 pending recovery，返回 mock 安全警告
					pp.sendTerminalLog(fmt.Sprintf("🛡️ ShepherdGate 拦截工具结果: %s - %s", decision.Status, decision.Reason))
					pp.sendLog("proxy_tool_result_decision", map[string]interface{}{
						"status":      decision.Status,
						"reason":      decision.Reason,
						"blocked":     true,
						"skill":       decision.Skill,
						"action_desc": decision.ActionDesc,
						"risk_type":   decision.RiskType,
					})

					// 存储恢复信息，下次用户确认后放行
					pp.storePendingToolCallRecovery(nil, "", decision.Reason, "tool_result")

					securityMsg := pp.shepherdGate.FormatSecurityMessage(decision)
					securityMsg = pp.shepherdGate.TranslateForUser(pp.ctx, securityMsg, cachedLastUserMsg)
					pp.saveAuditLog(true, "BLOCKED", decision.Reason, 100, "BLOCK")
					pp.statsMu.Lock()
					pp.blockedCount++
					pp.warningCount++
					pp.statsMu.Unlock()
					pp.sendMetricsToCallback()

					return &chatmodelrouting.FilterRequestResult{MockContent: securityMsg}, false
				} else {
					logging.Info("[ProxyProtection] ShepherdGate tool result decision: ALLOWED, tools=%s", strings.Join(toolNames, ", "))
					pp.sendTerminalLog(fmt.Sprintf("✅ ShepherdGate 工具结果检查通过 (ALLOWED): %s", strings.Join(toolNames, ", ")))
					pp.sendLog("proxy_tool_result_decision", map[string]interface{}{
						"status":      decision.Status,
						"reason":      decision.Reason,
						"blocked":     false,
						"skill":       decision.Skill,
						"action_desc": decision.ActionDesc,
						"risk_type":   decision.RiskType,
					})
				}
			}
		}
	}

	// 用户确认后，自动准备恢复上一轮被拦截的工具结果
	// 用代理生命周期 context 避免请求 context 取消导致意图识别失败
	if pp.armPendingRecoveryFromRequest(pp.ctx, req.Messages) {
		pp.sendTerminalLog("🔄 已识别用户确认，下一次请求将自动放行被拦截的工具结果")
		pp.sendLog("proxy_pending_tool_recovery_armed", map[string]interface{}{
			"armed": true,
		})
	}

	return nil, true // Allow request to pass through
}

// onResponse handles non-streaming responses
// Note: Detection is NOT triggered here when tool_calls are present.
// Detection only happens in onRequest when tool results (role=tool) are received.
func (pp *ProxyProtection) onResponse(ctx context.Context, resp *openai.ChatCompletion) bool {
	pp.sendLog("proxy_response_non_stream", nil)
	pp.sendLog("proxy_response_info", map[string]interface{}{
		"model":       resp.Model,
		"choiceCount": len(resp.Choices),
	})
	var outputContent string
	var generatedToolCalls []openai.ChatCompletionMessageToolCall

	if len(resp.Choices) > 0 {
		msg := &resp.Choices[0].Message
		outputContent = msg.Content
		generatedToolCalls = msg.ToolCalls

		pp.sendTerminalLog(fmt.Sprintf("onResponse: toolCalls=%d", len(msg.ToolCalls)))
		for _, tc := range msg.ToolCalls {
			pp.sendTerminalLog(fmt.Sprintf("onResponse tool_call: %s", tc.Function.Name))
		}

		// Log response content
		if msg.Content != "" {
			displayContent := msg.Content
			if len(displayContent) > 2000 {
				displayContent = displayContent[:2000] + "...(truncated)"
			}
			pp.sendLog("proxy_response_content", map[string]interface{}{
				"content": displayContent,
			})
		}

		// Log tool calls (for visibility only, detection happens in onRequest when tool results arrive)
		if len(msg.ToolCalls) > 0 {
			pp.metricsMu.Lock()
			pp.totalToolCalls += len(msg.ToolCalls)
			pp.metricsMu.Unlock()
			pp.sendMetricsToCallback()

			pp.sendLog("proxy_tool_calls_detected", nil)
			pp.sendLog("proxy_tool_call_count", map[string]interface{}{
				"count": len(msg.ToolCalls),
			})
			for i, tc := range msg.ToolCalls {
				args := tc.Function.Arguments
				if len(args) > 200 {
					args = args[:200] + "..."
				}
				pp.sendLog("proxy_tool_call_name", map[string]interface{}{
					"index": i,
					"name":  tc.Function.Name,
				})
				pp.sendLog("proxy_tool_call_args", map[string]interface{}{
					"index": i,
					"args":  args,
				})
			}
			pp.sendLog("proxy_tool_calls_pending", nil)
		}
	}

	// Collect token usage statistics.
	// For providers that omit usage in non-streaming mode, estimate to avoid undercounting.
	promptTokens := int(resp.Usage.PromptTokens)
	completionTokens := int(resp.Usage.CompletionTokens)
	totalTokens := int(resp.Usage.TotalTokens)
	hasUsageFromUpstream := promptTokens > 0 || completionTokens > 0 || totalTokens > 0

	if totalTokens == 0 && (promptTokens > 0 || completionTokens > 0) {
		totalTokens = promptTokens + completionTokens
	}

	if totalTokens == 0 {
		pp.streamBuffer.mu.Lock()
		reqMsgs := make([]ConversationMessage, len(pp.streamBuffer.requestMessages))
		copy(reqMsgs, pp.streamBuffer.requestMessages)
		toolsRaw := make([]byte, len(pp.streamBuffer.toolsRaw))
		copy(toolsRaw, pp.streamBuffer.toolsRaw)
		pp.streamBuffer.mu.Unlock()

		promptTokens = calculateRequestTokensFromRaw(reqMsgs, toolsRaw)
		completionTokens = estimateTokenCount(outputContent)
		if len(generatedToolCalls) > 0 {
			if toolCallsBytes, err := json.Marshal(generatedToolCalls); err == nil {
				completionTokens += estimateTokenCount(string(toolCallsBytes))
			}
		}
		totalTokens = promptTokens + completionTokens
	}

	if totalTokens > 0 {
		pp.metricsMu.Lock()
		pp.totalPromptTokens += promptTokens
		pp.totalCompletionTokens += completionTokens
		pp.totalTokens += totalTokens
		pp.metricsMu.Unlock()

		if hasUsageFromUpstream {
			pp.sendLog("proxy_token_usage", map[string]interface{}{
				"promptTokens":     promptTokens,
				"completionTokens": completionTokens,
				"totalTokens":      totalTokens,
			})
		} else {
			pp.sendLog("proxy_token_usage_estimated", map[string]interface{}{
				"promptTokens":     promptTokens,
				"completionTokens": completionTokens,
				"totalTokens":      totalTokens,
			})
		}

		// 发送指标到 Callback Bridge
		pp.sendMetricsToCallback()
	}

	// Finalize audit log for non-streaming response
	pp.finalizeAuditLog(outputContent, generatedToolCalls, promptTokens, completionTokens, totalTokens)

	return true
}

// onStreamChunk handles streaming responses
// Note: Tool calls are accumulated for audit purposes but NOT stripped from chunks.
// Detection happens in onRequest when tool results (role=tool) are received.
func (pp *ProxyProtection) onStreamChunk(ctx context.Context, chunk *openai.ChatCompletionChunk) bool {
	// Track current request's token usage (for audit log)
	// Note: In streaming, usage is typically only sent in the final chunk with finish_reason
	var currentRequestPromptTokens, currentRequestCompletionTokens, currentRequestTotalTokens int
	// Only process usage when it contains meaningful data (at least one non-zero token count).
	if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 || chunk.Usage.TotalTokens > 0 {
		// Store current request's token for audit log
		currentRequestPromptTokens = int(chunk.Usage.PromptTokens)
		currentRequestCompletionTokens = int(chunk.Usage.CompletionTokens)
		currentRequestTotalTokens = int(chunk.Usage.TotalTokens)
		if currentRequestTotalTokens == 0 && (currentRequestPromptTokens > 0 || currentRequestCompletionTokens > 0) {
			currentRequestTotalTokens = currentRequestPromptTokens + currentRequestCompletionTokens
		}

		// Usage may be emitted as cumulative values across multiple chunks.
		// Convert to deltas before accumulating global counters.
		pp.streamBuffer.mu.Lock()
		prevPromptTokens := pp.streamBuffer.promptTokens
		prevCompletionTokens := pp.streamBuffer.completionTokens
		prevTotalTokens := pp.streamBuffer.totalTokens
		deltaPromptTokens := currentRequestPromptTokens - prevPromptTokens
		deltaCompletionTokens := currentRequestCompletionTokens - prevCompletionTokens
		deltaTotalTokens := currentRequestTotalTokens - prevTotalTokens
		if deltaPromptTokens < 0 {
			deltaPromptTokens = currentRequestPromptTokens
		}
		if deltaCompletionTokens < 0 {
			deltaCompletionTokens = currentRequestCompletionTokens
		}
		if deltaTotalTokens < 0 {
			deltaTotalTokens = currentRequestTotalTokens
		}

		// Store latest usage snapshot for this request.
		pp.streamBuffer.promptTokens = currentRequestPromptTokens
		pp.streamBuffer.completionTokens = currentRequestCompletionTokens
		pp.streamBuffer.totalTokens = currentRequestTotalTokens
		pp.streamBuffer.mu.Unlock()

		// Also update global cumulative metrics
		pp.metricsMu.Lock()
		pp.totalPromptTokens += deltaPromptTokens
		pp.totalCompletionTokens += deltaCompletionTokens
		pp.totalTokens += deltaTotalTokens
		pp.metricsMu.Unlock()

		// Update audit log (whether pending or finalized)
		pp.auditMu.Lock()
		if pp.currentAuditLog != nil {
			pp.currentAuditLog.PromptTokens = currentRequestPromptTokens
			pp.currentAuditLog.CompletionTokens = currentRequestCompletionTokens
			pp.currentAuditLog.TotalTokens = currentRequestTotalTokens
			pp.auditMu.Unlock()
		} else {
			reqID := pp.currentRequestID
			pp.auditMu.Unlock()
			auditLogBuffer.UpdateAuditLogTokens(reqID, currentRequestPromptTokens, currentRequestCompletionTokens, currentRequestTotalTokens)
		}

		pp.sendLog("proxy_token_usage", map[string]interface{}{
			"promptTokens":     currentRequestPromptTokens,
			"completionTokens": currentRequestCompletionTokens,
			"totalTokens":      currentRequestTotalTokens,
		})

		// 发送指标到 Callback Bridge
		pp.sendMetricsToCallback()
	}

	if len(chunk.Choices) == 0 {
		return true
	}

	choice := &chunk.Choices[0]

	// Check if there's meaningful delta content or finish reason
	if choice.Delta.Content == "" && choice.Delta.Role == "" && len(choice.Delta.ToolCalls) == 0 && choice.FinishReason == "" {
		return true
	}

	// Accumulate content (for logging purposes)
	if choice.Delta.Content != "" {
		// 获取追加前的长度
		pp.streamBuffer.mu.Lock()
		prevLen := 0
		for _, c := range pp.streamBuffer.contentChunks {
			prevLen += len(c)
		}
		pp.streamBuffer.mu.Unlock()

		pp.streamBuffer.AppendContent(choice.Delta.Content)

		// 计算追加后的长度
		newLen := prevLen + len(choice.Delta.Content)

		// 节流：首次收到内容或每 200 字符发送一次进度日志
		if prevLen == 0 || (prevLen/200 != newLen/200) {
			pp.sendLog("proxy_stream_content", map[string]interface{}{
				"chars": newLen,
			})
		}
	}

	// Accumulate tool calls for audit logging (detection happens in onRequest, not here)
	if len(choice.Delta.ToolCalls) > 0 {
		for _, stc := range choice.Delta.ToolCalls {
			pp.streamBuffer.MergeStreamToolCall(stc)
		}
	}

	// Check finish reason - process tool calls ONLY when the stream (or tool call) finishes
	if choice.FinishReason != "" {
		pp.sendLog("proxy_stream_finished", map[string]interface{}{
			"reason": choice.FinishReason,
		})
		// Check if we have buffered tool calls - log for visibility (detection happens in onRequest)
		if pp.streamBuffer.HasToolCalls() {
			pp.streamBuffer.mu.Lock()
			toolCallCount := len(pp.streamBuffer.toolCalls)
			bufferToolCalls := make([]openai.ChatCompletionMessageToolCall, len(pp.streamBuffer.toolCalls))
			copy(bufferToolCalls, pp.streamBuffer.toolCalls)
			var contentWithTools string
			for _, c := range pp.streamBuffer.contentChunks {
				contentWithTools += c
			}
			pp.streamBuffer.mu.Unlock()

			pp.sendTerminalLog(fmt.Sprintf("onStreamChunk: %d tool calls in stream", toolCallCount))

			// Collect tool call statistics
			pp.metricsMu.Lock()
			pp.totalToolCalls += toolCallCount
			pp.metricsMu.Unlock()
			pp.sendMetricsToCallback()

			// Log accumulated text content even when tool_calls are present,
			// so the grouped card can display the assistant's text response.
			if len(contentWithTools) > 0 {
				displayContent := contentWithTools
				if len(displayContent) > 300 {
					displayContent = displayContent[:300] + "...(truncated)"
				}
				pp.sendLog("proxy_stream_content_with_tools", map[string]interface{}{
					"content": displayContent,
				})
			}

			// Log tool calls for Flutter UI display
			pp.sendLog("proxy_tool_call_count", map[string]interface{}{
				"count": toolCallCount,
			})
			for i, tc := range bufferToolCalls {
				pp.sendLog("proxy_tool_call_name", map[string]interface{}{
					"index": i,
					"name":  tc.Function.Name,
				})
			}
			// Detection will be triggered in the next request when tool results arrive
			pp.sendLog("proxy_tool_calls_pending", nil)
		} else {
			// No tool calls - log accumulated content
			pp.streamBuffer.mu.Lock()
			var accumulatedContent string
			for _, c := range pp.streamBuffer.contentChunks {
				accumulatedContent += c
			}
			pp.streamBuffer.mu.Unlock()

			if len(accumulatedContent) > 0 {
				displayContent := accumulatedContent
				if len(displayContent) > 300 {
					displayContent = displayContent[:300] + "...(truncated)"
				}
				pp.sendLog("proxy_stream_content_no_tools", map[string]interface{}{
					"content": displayContent,
				})
			}
		}

		// Finalize audit log for streaming response
		pp.streamBuffer.mu.Lock()
		var outputContent string
		for _, c := range pp.streamBuffer.contentChunks {
			outputContent += c
		}
		// Get token usage for current request (stored when usage was received in stream)
		promptTokens := pp.streamBuffer.promptTokens
		completionTokens := pp.streamBuffer.completionTokens
		totalTokens := pp.streamBuffer.totalTokens
		generatedToolCalls := pp.streamBuffer.toolCalls

		// If no usage was returned in stream (common in some providers), estimate it
		if totalTokens == 0 {
			promptTokens = calculateRequestTokensFromRaw(pp.streamBuffer.requestMessages, pp.streamBuffer.toolsRaw)
			completionTokens = estimateTokenCount(outputContent)

			// Add tokens for tool calls if present
			if len(pp.streamBuffer.toolCalls) > 0 {
				if toolCallsBytes, err := json.Marshal(pp.streamBuffer.toolCalls); err == nil {
					completionTokens += estimateTokenCount(string(toolCallsBytes))
				}
			}

			totalTokens = promptTokens + completionTokens

			// Update global metrics since they weren't updated during stream
			pp.metricsMu.Lock()
			pp.totalPromptTokens += promptTokens
			pp.totalCompletionTokens += completionTokens
			pp.totalTokens += totalTokens
			pp.metricsMu.Unlock()
			// 流式估算 token 也要同步指标，保证统计与日志结束时一致
			pp.sendMetricsToCallback()

			// Update streamBuffer for consistency
			pp.streamBuffer.promptTokens = promptTokens
			pp.streamBuffer.completionTokens = completionTokens
			pp.streamBuffer.totalTokens = totalTokens

			pp.sendLog("proxy_token_usage_estimated", map[string]interface{}{
				"promptTokens":     promptTokens,
				"completionTokens": completionTokens,
				"totalTokens":      totalTokens,
			})
		}
		pp.streamBuffer.mu.Unlock()

		pp.finalizeAuditLog(outputContent, generatedToolCalls, promptTokens, completionTokens, totalTokens)

		pp.streamBuffer.Clear()
	}

	return true
}

// formatQuotaExceededMessage 根据语言生成配额超限的模拟返回消息
// quotaType: "session" 或 "daily"
func formatQuotaExceededMessage(quotaType string, current, limit int) string {
	lang := shepherd.NormalizeShepherdLanguage(skillscan.GetLanguageFromAppSettings())

	if lang == "zh" {
		var quotaName string
		if quotaType == "session" {
			quotaName = "单会话 Token 配额已用尽"
		} else {
			quotaName = "每日 Token 配额已用尽"
		}
		return fmt.Sprintf("[ClawSecbot] 状态: QUOTA_EXCEEDED | 原因: %s (%d/%d)\n\n当前请求已被拦截，请调整配额设置或等待配额重置。",
			quotaName, current, limit)
	}

	// English (default)
	var quotaName string
	if quotaType == "session" {
		quotaName = "Session token quota exceeded"
	} else {
		quotaName = "Daily token quota exceeded"
	}
	return fmt.Sprintf("[ClawSecbot] Status: QUOTA_EXCEEDED | Reason: %s (%d/%d)\n\nThis request has been blocked. Please adjust quota settings or wait for quota reset.",
		quotaName, current, limit)
}
