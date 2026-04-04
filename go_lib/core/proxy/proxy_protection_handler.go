package proxy

// proxy_protection_handler.go 包含 ProxyProtection 的请求/响应处理方法。
// 所有请求生命周期信息统一通过 updateTruthRecord() 写入单一 TruthRecord（SSOT）。

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
	modelName := string(req.Model)
	stream := false
	var requestProbe struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(rawBody, &requestProbe); err == nil {
		stream = requestProbe.Stream
	}

	pp.configMu.RLock()
	sessionLimit := pp.singleSessionTokenLimit
	dailyLimit := pp.dailyTokenLimit
	initialUsage := pp.initialDailyUsage
	pp.configMu.RUnlock()

	// ==================== 单会话 Token 配额检查 ====================
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

			pp.auditMu.Lock()
			pp.requestStartTime = time.Now()
			pp.currentRequestID = fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), pp.requestCount+1)
			requestID := pp.currentRequestID
			pp.auditMu.Unlock()

			pp.sendLog("proxy_session_quota_exceeded", map[string]interface{}{
				"current": sessionTotal,
				"limit":   sessionLimit,
				"model":   modelName,
			})

			mockMsg := formatQuotaExceededMessage("session", sessionTotal, sessionLimit)
			pp.updateTruthRecord(requestID, func(r *TruthRecord) {
				r.Model = modelName
				r.MessageCount = len(req.Messages)
				r.Phase = RecordPhaseStopped
				r.CompletedAt = time.Now().Format(time.RFC3339Nano)
				r.Decision = &SecurityDecision{
					Action:     "BLOCK",
					RiskLevel:  "QUOTA",
					Reason:     reason,
					Confidence: 100,
				}
				applyRecordPrimaryContent(r, RecordContentSecurity, mockMsg, true)
			})

			pp.emitMonitorRequestCreated(req, rawBody, stream)
			pp.emitMonitorSecurityDecision("QUOTA_EXCEEDED", reason, true, mockMsg)
			pp.emitMonitorResponseReturned("QUOTA_EXCEEDED", mockMsg, mockMsg)
			return &chatmodelrouting.FilterRequestResult{MockContent: mockMsg}, false
		}
	}

	// ==================== 每日 Token 配额检查 ====================
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

			pp.auditMu.Lock()
			pp.requestStartTime = time.Now()
			pp.currentRequestID = fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), pp.requestCount+1)
			requestID := pp.currentRequestID
			pp.auditMu.Unlock()

			pp.sendLog("proxy_quota_exceeded", map[string]interface{}{
				"current": currentTotal,
				"limit":   dailyLimit,
				"model":   modelName,
			})

			mockMsg := formatQuotaExceededMessage("daily", currentTotal, dailyLimit)
			pp.updateTruthRecord(requestID, func(r *TruthRecord) {
				r.Model = modelName
				r.MessageCount = len(req.Messages)
				r.Phase = RecordPhaseStopped
				r.CompletedAt = time.Now().Format(time.RFC3339Nano)
				r.Decision = &SecurityDecision{
					Action:     "BLOCK",
					RiskLevel:  "QUOTA",
					Reason:     reason,
					Confidence: 100,
				}
				applyRecordPrimaryContent(r, RecordContentSecurity, mockMsg, true)
			})

			pp.emitMonitorRequestCreated(req, rawBody, stream)
			pp.emitMonitorSecurityDecision("QUOTA_EXCEEDED", reason, true, mockMsg)
			pp.emitMonitorResponseReturned("QUOTA_EXCEEDED", mockMsg, mockMsg)
			return &chatmodelrouting.FilterRequestResult{MockContent: mockMsg}, false
		}
	}

	// ==================== 正常请求处理 ====================
	pp.metricsMu.Lock()
	pp.requestCount++
	currentRequestNum := pp.requestCount
	pp.metricsMu.Unlock()
	pp.sendMetricsToCallback()

	pp.auditMu.Lock()
	pp.requestStartTime = time.Now()
	pp.currentRequestID = fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), currentRequestNum)
	requestID := pp.currentRequestID
	pp.auditMu.Unlock()

	pp.updateTruthRecord(requestID, func(r *TruthRecord) {
		r.Model = modelName
		r.MessageCount = len(req.Messages)
		r.Phase = RecordPhaseStarting
		r.Decision = &SecurityDecision{Action: "ALLOW"}
	})

	pp.sendLog("proxy_new_request", map[string]interface{}{
		"model": modelName,
	})
	pp.emitMonitorRequestCreated(req, rawBody, stream)

	securityModel := ""
	if pp.shepherdGate != nil {
		securityModel = pp.shepherdGate.GetModelName()
	}
	logging.Info("[ProxyProtection] onRequest: model=%s, messageCount=%d, requestID=%s, securityModel=%s, botTargetURL=%s", modelName, len(req.Messages), requestID, securityModel, pp.targetURL)
	pp.sendLog("proxy_request_info", map[string]interface{}{
		"model":        modelName,
		"messageCount": len(req.Messages),
		"stream":       fmt.Sprintf("%v", stream),
	})
	pp.emitMonitorUpstreamRequestBuilt(req, rawBody)
	pp.emitMonitorUpstreamRequestSent()

	var roles []string
	for _, msg := range req.Messages {
		roles = append(roles, getMessageRole(msg))
	}
	pp.sendTerminalLog(fmt.Sprintf("请求消息角色: %v", roles))

	// ==================== 工具结果检测逻辑 ====================
	type toolCallRef struct {
		ID       string
		FuncName string
		RawArgs  string
	}
	var toolCallsInHistory []toolCallRef
	var latestAssistantToolCalls []toolCallRef
	var latestAssistantIndex int = -1
	hasToolResultMessages := false

	var toolResultIndices []int

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

			hasToolsFollowing := false
			for j := i + 1; j < len(req.Messages); j++ {
				if req.Messages[j].OfTool != nil {
					hasToolsFollowing = true
				} else if req.Messages[j].OfUser != nil {
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

	// ==================== DinTalClaw 内嵌工具协议回退检测 ====================
	// 当标准 OpenAI tool_calls 未找到时，检查消息 content 中的 <tool_use> / <tool_result> 标签。
	// DinTalClaw 可能将 assistant 内容（含 <tool_use>）嵌入 user 角色消息中，因此也需扫描 user 消息。
	isInlineToolProtocol := false
	var inlineToolResultsMap map[string]string
	if len(latestAssistantToolCalls) == 0 {
		for i := len(req.Messages) - 1; i >= 0; i-- {
			msg := req.Messages[i]
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

			latestAssistantIndex = i
			isInlineToolProtocol = true
			for j, it := range inlineTools {
				syntheticID := generateInlineToolCallID(i, j)
				latestAssistantToolCalls = append(latestAssistantToolCalls, toolCallRef{
					ID:       syntheticID,
					FuncName: it.Name,
					RawArgs:  it.RawArgs,
				})
			}
			pp.sendTerminalLog(fmt.Sprintf("[InlineTool] Found %d inline <tool_use> in message at index %d", len(inlineTools), i))

			inlineToolResultsMap = make(map[string]string)
			consumed := false

			// DinTalClaw 单消息模式：同一条 user 消息可能同时包含 <tool_use> 和 <tool_result>
			if msg.OfUser != nil {
				sameResults := extractInlineToolResults(content)
				if len(sameResults) > 0 {
					hasToolResultMessages = true
					for k, result := range sameResults {
						if k < len(latestAssistantToolCalls) {
							tcID := latestAssistantToolCalls[k].ID
							inlineToolResultsMap[tcID] = result
							toolResultIndices = append(toolResultIndices, i)
						}
					}
					pp.sendTerminalLog(fmt.Sprintf("[InlineTool] Found %d inline <tool_result> in same user message at index %d", len(sameResults), i))
				}
			}

			for j := i + 1; j < len(req.Messages); j++ {
				nextMsg := req.Messages[j]
				if nextMsg.OfUser != nil {
					nextContent := extractMessageContent(nextMsg)
					// 后续 user 消息中出现新 <tool_use> 且无 <tool_result>，视为新工具轮次
					if hasInlineToolUse(nextContent) && !hasInlineToolResult(nextContent) {
						consumed = true
						pp.sendTerminalLog(fmt.Sprintf("[InlineTool] New <tool_use> cycle in user message at index %d, previous results consumed", j))
						break
					}
					inlineResults := extractInlineToolResults(nextContent)
					if len(inlineResults) > 0 {
						hasToolResultMessages = true
						for k, result := range inlineResults {
							if k < len(latestAssistantToolCalls) {
								tcID := latestAssistantToolCalls[k].ID
								inlineToolResultsMap[tcID] = result
								toolResultIndices = append(toolResultIndices, j)
							}
						}
						pp.sendTerminalLog(fmt.Sprintf("[InlineTool] Found %d inline <tool_result> in user at index %d", len(inlineResults), j))
					}
				} else if nextMsg.OfAssistant != nil {
					nextAssistContent := extractMessageContent(nextMsg)
					if !hasInlineToolUse(nextAssistContent) {
						consumed = true
						pp.sendTerminalLog(fmt.Sprintf("[InlineTool] Inline tool results consumed by assistant at index %d", j))
					}
					break
				}
			}
			if consumed {
				latestAssistantToolCalls = nil
				latestAssistantIndex = -1
				hasToolResultMessages = false
				inlineToolResultsMap = nil
				isInlineToolProtocol = false
			}
			break
		}
	}

	// DinTalClaw 孤儿 <tool_result> 检测：
	// 当请求中没有任何 <tool_use>（标准 / 内嵌均未找到），但 user 消息中包含 <tool_result> 时，
	// 仍将其提取为工具结果记录，确保分组视图能展示。
	if !isInlineToolProtocol && len(latestAssistantToolCalls) == 0 {
		for i := len(req.Messages) - 1; i >= 0; i-- {
			msg := req.Messages[i]
			if msg.OfUser == nil {
				continue
			}
			content := extractMessageContent(msg)
			results := extractInlineToolResults(content)
			if len(results) == 0 {
				continue
			}
			isInlineToolProtocol = true
			latestAssistantIndex = i
			inlineToolResultsMap = make(map[string]string)
			hasToolResultMessages = true
			for k, result := range results {
				synID := generateInlineToolCallID(i, k)
				latestAssistantToolCalls = append(latestAssistantToolCalls, toolCallRef{
					ID:       synID,
					FuncName: "tool_result",
					RawArgs:  "",
				})
				inlineToolResultsMap[synID] = result
				toolResultIndices = append(toolResultIndices, i)
				toolCallsInHistory = append(toolCallsInHistory, toolCallRef{
					ID:       synID,
					FuncName: "tool_result",
					RawArgs:  "",
				})
			}
			pp.sendTerminalLog(fmt.Sprintf("[InlineTool] Found %d orphan <tool_result> in user message at index %d (no matching <tool_use>)", len(results), i))
			break
		}
	}

	// 独立于 ShepherdGate 清除逻辑，收集最新一轮 assistant 工具调用 ID，用于 TruthRecord 标记。
	// 仅当工具结果尚未被后续 assistant 回复消费时才标记 — 即 tool 消息后面不能紧跟非 tool 消息。
	latestRoundTCIDs := make(map[string]bool)
	if isInlineToolProtocol {
		// DinTalClaw 内嵌协议：使用合成 ID 标记最新一轮
		if hasToolResultMessages {
			for _, tc := range latestAssistantToolCalls {
				latestRoundTCIDs[tc.ID] = true
			}
		}
	} else {
		for i := len(req.Messages) - 1; i >= 0; i-- {
			msg := req.Messages[i]
			if msg.OfAssistant != nil && len(msg.OfAssistant.ToolCalls) > 0 {
				hasToolFollowing := false
				consumed := false
				for j := i + 1; j < len(req.Messages); j++ {
					if req.Messages[j].OfTool != nil {
						hasToolFollowing = true
					} else if hasToolFollowing {
						consumed = true
						break
					}
				}
				if hasToolFollowing && !consumed {
					for _, tc := range msg.OfAssistant.ToolCalls {
						latestRoundTCIDs[tc.ID] = true
					}
				}
				break
			}
		}
	}

	pp.mu.Lock()
	pp.lastContextMessages = make([]ConversationMessage, 0, len(req.Messages))
	pp.mu.Unlock()

	for i, msg := range req.Messages {
		cm := extractConversationMessage(msg)
		role := cm.Role
		content := cm.Content

		pp.mu.Lock()
		pp.lastContextMessages = append(pp.lastContextMessages, cm)
		pp.mu.Unlock()

		displayContent := content
		if len(displayContent) > 200 {
			displayContent = truncateString(displayContent, 200)
		}
		// 审计用完整内容，单条上限 256KB
		recordContent := truncateToBytes(content, maxRecordMessageBytes)
		if len(recordContent) < len(content) {
			logging.Debug("[TruthRecord] message content truncated to %d bytes (original %d) at index %d role=%s", len(recordContent), len(content), i, role)
		}
		pp.sendLog("proxy_message_info", map[string]interface{}{
			"index":   i,
			"role":    role,
			"content": displayContent,
		})
		pp.emitMonitorClientMessage(i, role, content)
		pp.updateTruthRecord(requestID, func(r *TruthRecord) {
			r.Messages = append(r.Messages, RecordMessage{
				Index:   i,
				Role:    role,
				Content: recordContent,
			})
		})

		if role == "user" {
			pp.mu.Lock()
			pp.lastUserMessageContent = content
			pp.mu.Unlock()
		}

		if msg.OfAssistant != nil && len(msg.OfAssistant.ToolCalls) > 0 {
			for _, tc := range msg.OfAssistant.ToolCalls {
				toolCallsInHistory = append(toolCallsInHistory, toolCallRef{
					ID:       tc.ID,
					FuncName: tc.Function.Name,
					RawArgs:  tc.Function.Arguments,
				})
			}
		}

		// DinTalClaw 内嵌协议：收集消息内容中的 <tool_use> 到历史工具调用（assistant 或 user 消息）
		if isInlineToolProtocol {
			shouldCollect := false
			if msg.OfAssistant != nil && len(msg.OfAssistant.ToolCalls) == 0 {
				shouldCollect = true
			} else if msg.OfUser != nil && hasInlineToolUse(content) {
				shouldCollect = true
			}
			if shouldCollect {
				inlineTools := extractInlineToolUses(content)
				for j, it := range inlineTools {
					synID := generateInlineToolCallID(i, j)
					toolCallsInHistory = append(toolCallsInHistory, toolCallRef{
						ID:       synID,
						FuncName: it.Name,
						RawArgs:  it.RawArgs,
					})
				}
			}
		}

		if msg.OfTool != nil {
			toolCallID := msg.OfTool.ToolCallID
			toolContent := content
			if len(toolContent) > 300 {
				toolContent = truncateString(toolContent, 300)
			}
			pp.updateTruthRecord(requestID, func(r *TruthRecord) {
				applyRecordPrimaryContent(r, RecordContentToolResult, toolContent, false)
			})
			pp.sendTerminalLog(fmt.Sprintf("发现 tool 消息在索引 %d，tool_call_id=%s", i, toolCallID))

			if latestAssistantIndex >= 0 && i > latestAssistantIndex {
				if len(latestAssistantToolCalls) > 0 {
					matched := false
					for _, tc := range latestAssistantToolCalls {
						if tc.ID == toolCallID {
							pp.sendTerminalLog(fmt.Sprintf("✓ tool_call_id 匹配成功且在最新 assistant 之后: %s", toolCallID))
							pp.sendLog("proxy_tool_result_content", map[string]interface{}{
								"index":   i,
								"tool_id": toolCallID,
								"content": toolContent,
							})
							pp.sendLog("monitor_upstream_tool_result", map[string]interface{}{
								"tool_id": toolCallID,
								"summary": pp.previewToolResult(toolContent),
							})
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
	// DinTalClaw 内嵌协议：将 <tool_result> 结果合并到 toolResultsMap
	if isInlineToolProtocol && inlineToolResultsMap != nil {
		for id, result := range inlineToolResultsMap {
			toolResultsMap[id] = result
		}
	}

	// 将历史工具调用记录写入 TruthRecord，标记最新一轮
	for _, tc := range toolCallsInHistory {
		isSensitive := false
		if pp.toolValidator != nil {
			isSensitive = pp.toolValidator.IsSensitive(tc.FuncName)
		}
		tcID := strings.TrimSpace(tc.ID)
		result := ""
		if val, ok := toolResultsMap[tcID]; ok {
			result = truncateToBytes(val, maxRecordMessageBytes)
		}
		args := truncateToBytes(tc.RawArgs, maxRecordToolArgsBytes)
		isLatest := latestRoundTCIDs[tcID]
		pp.updateTruthRecord(requestID, func(r *TruthRecord) {
			r.ToolCalls = append(r.ToolCalls, RecordToolCall{
				ID:          tc.ID,
				Name:        tc.FuncName,
				Arguments:   args,
				Result:      result,
				IsSensitive: isSensitive,
				Source:      "history",
				LatestRound: isLatest,
			})
		})
	}

	_ = toolResultIndices
	pp.sendTerminalLog(fmt.Sprintf("检测触发判断: hasToolResultMessages=%v, 工具结果数量=%d", hasToolResultMessages, len(toolResultIndices)))

	pp.streamBuffer.ClearAll()
	pp.streamBuffer.SetRequest(requestID, req, rawBody)

	// ==================== ShepherdGate 安全检测 ====================
	if hasToolResultMessages && pp.shepherdGate != nil {
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
			pp.emitMonitorSecurityDecision("RECOVERY_ALLOWED", "user confirmed recovery", false, "")
		} else {
			pp.configMu.RLock()
			auditOnlyForShepherd := pp.auditOnly
			pp.configMu.RUnlock()

			if auditOnlyForShepherd {
				logging.Info("[ProxyProtection] Audit-only mode, skipping ShepherdGate analysis")
				pp.sendTerminalLog("📋 仅审计模式，跳过 ShepherdGate 检测，直接放行")
			} else {
				var toolCallInfos []ToolCallInfo
				for _, tcRef := range latestAssistantToolCalls {
					info := ToolCallInfo{
						Name:       tcRef.FuncName,
						RawArgs:    tcRef.RawArgs,
						ToolCallID: tcRef.ID,
					}
					if pp.toolValidator != nil {
						info.IsSensitive = pp.toolValidator.IsSensitive(tcRef.FuncName)
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

				skipShepherdForSandboxBlock := false
				for _, tr := range toolResultInfos {
					if !isClawdSecbotSandboxBlockedToolResult(tr.Content) {
						continue
					}
					if !pp.markSandboxBlockedToolResultIfFirst(tr.ToolCallID) {
						continue
					}

					skipShepherdForSandboxBlock = true
					pp.sendTerminalLog(fmt.Sprintf(
						"检测到 ClawdSecbot 沙箱已阻止工具结果，跳过 ShepherdGate 二次确认: tool=%s, tool_call_id=%s",
						tr.FuncName,
						tr.ToolCallID,
					))
					pp.sendLog("proxy_tool_result_sandbox_blocked", map[string]interface{}{
						"tool_id":  tr.ToolCallID,
						"tool":     tr.FuncName,
						"detected": true,
					})
					break
				}

				if skipShepherdForSandboxBlock {
					pp.emitMonitorSecurityDecision(
						"SANDBOX_BLOCKED",
						"tool result already blocked by ClawdSecbot sandbox",
						false,
						"",
					)
					return nil, true
				}

				var toolNames []string
				for _, tc := range toolCallInfos {
					toolNames = append(toolNames, tc.Name)
				}
				pp.updateTruthRecord(requestID, func(r *TruthRecord) {
					// Tool names/count will be computed from ToolCalls by frontend getters
				})
				pp.sendTerminalLog(fmt.Sprintf("🔍 ShepherdGate 正在检查 %d 个工具结果: %s", len(toolResultInfos), strings.Join(toolNames, ", ")))

				pp.mu.Lock()
				contextMessages := pp.lastContextMessages
				cachedLastUserMsg := pp.lastUserMessageContent
				pp.mu.Unlock()

				securityModel := pp.shepherdGate.GetModelName()
				logging.Info("[ProxyProtection] ShepherdGate tool result detection triggered: toolCalls=%d, toolResults=%d, securityModel=%s", len(toolCallInfos), len(toolResultInfos), securityModel)

				decision, err := pp.shepherdGate.CheckToolCall(pp.ctx, contextMessages, toolCallInfos, toolResultInfos, cachedLastUserMsg, requestID)

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
					logging.Error("[ProxyProtection] ShepherdGate tool result check failed: %v, fail-open", err)
				} else if decision.Status != "ALLOWED" {
					logging.Info("[ProxyProtection] ShepherdGate tool result decision: status=%s, reason=%s", decision.Status, decision.Reason)
					pp.sendTerminalLog(fmt.Sprintf("🛡️ ShepherdGate 拦截工具结果: %s - %s", decision.Status, decision.Reason))
					pp.sendLog("proxy_tool_result_decision", map[string]interface{}{
						"status":      decision.Status,
						"reason":      decision.Reason,
						"blocked":     true,
						"skill":       decision.Skill,
						"action_desc": decision.ActionDesc,
						"risk_type":   decision.RiskType,
					})

					pp.storePendingToolCallRecovery(nil, "", decision.Reason, "tool_result")

					securityMsg := pp.shepherdGate.FormatSecurityMessage(decision)
					securityMsg = pp.shepherdGate.TranslateForUser(pp.ctx, securityMsg, cachedLastUserMsg)
					pp.emitMonitorSecurityDecision(decision.Status, decision.Reason, true, securityMsg)
					recordAction := "BLOCK"
					recordRiskLevel := "BLOCKED"
					if decision.Status == "NEEDS_CONFIRMATION" {
						recordAction = "NEEDS_CONFIRMATION"
						recordRiskLevel = "NEEDS_CONFIRMATION"
					}
					pp.updateTruthRecord(requestID, func(r *TruthRecord) {
						r.Phase = RecordPhaseStopped
						r.CompletedAt = time.Now().Format(time.RFC3339Nano)
						r.Decision = &SecurityDecision{
							Action:     recordAction,
							RiskLevel:  recordRiskLevel,
							Reason:     decision.Reason,
							Confidence: 100,
						}
						applyRecordPrimaryContent(r, RecordContentSecurity, securityMsg, true)
					})
					pp.statsMu.Lock()
					pp.blockedCount++
					pp.warningCount++
					pp.statsMu.Unlock()
					pp.sendMetricsToCallback()
					pp.emitMonitorResponseReturned(decision.Status, securityMsg, securityMsg)

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
					pp.emitMonitorSecurityDecision(decision.Status, decision.Reason, false, "")
					pp.updateTruthRecord(requestID, func(r *TruthRecord) {
						r.Decision = &SecurityDecision{
							Action: "ALLOW",
							Reason: decision.Reason,
						}
					})
				}
			}
		}
	}

	if pp.armPendingRecoveryFromRequest(pp.ctx, req.Messages) {
		pp.sendTerminalLog("🔄 已识别用户确认，下一次请求将自动放行被拦截的工具结果")
		pp.sendLog("proxy_pending_tool_recovery_armed", map[string]interface{}{
			"armed": true,
		})
	}

	return nil, true
}

// onResponse handles non-streaming responses
func (pp *ProxyProtection) onResponse(ctx context.Context, resp *openai.ChatCompletion) bool {
	requestID := pp.activeRequestID()
	pp.sendLogForRequest(requestID, "proxy_response_non_stream", nil)
	pp.sendLogForRequest(requestID, "proxy_response_info", map[string]interface{}{
		"model":       resp.Model,
		"choiceCount": len(resp.Choices),
	})
	pp.updateTruthRecord(requestID, func(r *TruthRecord) {
		r.Phase = RecordPhaseCompleted
		if r.Model == "" {
			r.Model = resp.Model
		}
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

		if msg.Content != "" {
			logContent := msg.Content
			if len(logContent) > 2000 {
				logContent = truncateString(logContent, 2000)
			}
			recordContent := truncateToBytes(msg.Content, maxRecordMessageBytes)
			pp.sendLogForRequest(requestID, "proxy_response_content", map[string]interface{}{
				"content": logContent,
			})
			pp.updateTruthRecord(requestID, func(r *TruthRecord) {
				assistantIndex := len(r.Messages)
				if assistantIndex < r.MessageCount {
					assistantIndex = r.MessageCount
				}
				if len(r.Messages) == 0 ||
					r.Messages[len(r.Messages)-1].Role != "assistant" ||
					r.Messages[len(r.Messages)-1].Content != recordContent {
					r.Messages = append(r.Messages, RecordMessage{
						Index:   assistantIndex,
						Role:    "assistant",
						Content: recordContent,
					})
				}
				applyRecordPrimaryContent(r, RecordContentAssistant, previewRecordContent(msg.Content, 300).Content, true)
			})
		}

		if len(msg.ToolCalls) > 0 {
			pp.metricsMu.Lock()
			pp.totalToolCalls += len(msg.ToolCalls)
			pp.metricsMu.Unlock()
			pp.sendMetricsToCallback()

			pp.sendLogForRequest(requestID, "proxy_tool_calls_detected", nil)
			pp.sendLogForRequest(requestID, "proxy_tool_call_count", map[string]interface{}{
				"count": len(msg.ToolCalls),
			})
			for i, tc := range msg.ToolCalls {
				args := tc.Function.Arguments
				if len(args) > 200 {
					args = truncateString(args, 200)
				}
				pp.sendLogForRequest(requestID, "proxy_tool_call_name", map[string]interface{}{
					"index": i,
					"name":  tc.Function.Name,
				})
				pp.sendLogForRequest(requestID, "proxy_tool_call_args", map[string]interface{}{
					"index": i,
					"args":  args,
				})
				pp.sendLog("monitor_upstream_tool_call", map[string]interface{}{
					"index": i,
					"name":  tc.Function.Name,
				})
			}
			pp.sendLogForRequest(requestID, "proxy_tool_calls_pending", nil)
		}
	}

	// Token usage
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
			pp.sendLogForRequest(requestID, "proxy_token_usage", map[string]interface{}{
				"promptTokens":     promptTokens,
				"completionTokens": completionTokens,
				"totalTokens":      totalTokens,
			})
		} else {
			pp.sendLogForRequest(requestID, "proxy_token_usage_estimated", map[string]interface{}{
				"promptTokens":     promptTokens,
				"completionTokens": completionTokens,
				"totalTokens":      totalTokens,
			})
		}
		pp.sendMetricsToCallback()
	}

	// Finalize via TruthRecord
	pp.finalizeTruthRecord(requestID, outputContent, generatedToolCalls, promptTokens, completionTokens)
	pp.sendLog("monitor_upstream_completed", map[string]interface{}{
		"response_mode":        "non_stream",
		"final_text":           truncateString(outputContent, 2000),
		"finish_reason":        "stop",
		"raw_response_preview": truncateString(resp.RawJSON(), 2000),
	})
	pp.emitMonitorResponseReturned("COMPLETED", outputContent, resp.RawJSON())
	pp.updateTruthRecord(requestID, func(r *TruthRecord) {
		if len(resp.Choices) > 0 {
			r.FinishReason = string(resp.Choices[0].FinishReason)
		}
	})
	return true
}

// onStreamChunk handles streaming responses
func (pp *ProxyProtection) onStreamChunk(ctx context.Context, chunk *openai.ChatCompletionChunk) bool {
	requestID := pp.activeRequestID()
	var currentRequestPromptTokens, currentRequestCompletionTokens, currentRequestTotalTokens int

	if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 || chunk.Usage.TotalTokens > 0 {
		currentRequestPromptTokens = int(chunk.Usage.PromptTokens)
		currentRequestCompletionTokens = int(chunk.Usage.CompletionTokens)
		currentRequestTotalTokens = int(chunk.Usage.TotalTokens)
		if currentRequestTotalTokens == 0 && (currentRequestPromptTokens > 0 || currentRequestCompletionTokens > 0) {
			currentRequestTotalTokens = currentRequestPromptTokens + currentRequestCompletionTokens
		}

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
		pp.streamBuffer.promptTokens = currentRequestPromptTokens
		pp.streamBuffer.completionTokens = currentRequestCompletionTokens
		pp.streamBuffer.totalTokens = currentRequestTotalTokens
		pp.streamBuffer.mu.Unlock()

		pp.metricsMu.Lock()
		pp.totalPromptTokens += deltaPromptTokens
		pp.totalCompletionTokens += deltaCompletionTokens
		pp.totalTokens += deltaTotalTokens
		pp.metricsMu.Unlock()

		pp.sendLogForRequest(requestID, "proxy_token_usage", map[string]interface{}{
			"promptTokens":     currentRequestPromptTokens,
			"completionTokens": currentRequestCompletionTokens,
			"totalTokens":      currentRequestTotalTokens,
		})
		pp.sendMetricsToCallback()
		pp.updateTruthRecord(requestID, func(r *TruthRecord) {
			r.PromptTokens = currentRequestPromptTokens
			r.CompletionTokens = currentRequestCompletionTokens
		})
	}

	if len(chunk.Choices) == 0 {
		return true
	}

	choice := &chunk.Choices[0]

	if choice.Delta.Content == "" && choice.Delta.Role == "" && len(choice.Delta.ToolCalls) == 0 && choice.FinishReason == "" {
		return true
	}

	if choice.Delta.Content != "" {
		if !pp.streamBuffer.started {
			pp.streamBuffer.started = true
			pp.sendLog("monitor_upstream_stream_started", map[string]interface{}{
				"response_mode": "stream",
			})
		}

		pp.streamBuffer.mu.Lock()
		prevLen := 0
		for _, c := range pp.streamBuffer.contentChunks {
			prevLen += len(c)
		}
		pp.streamBuffer.mu.Unlock()

		pp.streamBuffer.AppendContent(choice.Delta.Content)
		newLen := prevLen + len(choice.Delta.Content)

		if prevLen == 0 || (prevLen/200 != newLen/200) {
			pp.sendLogForRequest(requestID, "proxy_stream_content", map[string]interface{}{
				"chars": newLen,
			})
		}
		pp.sendLog("proxy_stream_delta", map[string]interface{}{
			"content": choice.Delta.Content,
		})
		pp.sendLog("monitor_upstream_stream_delta", map[string]interface{}{
			"content": choice.Delta.Content,
		})
	}

	if len(choice.Delta.ToolCalls) > 0 {
		for _, stc := range choice.Delta.ToolCalls {
			pp.streamBuffer.MergeStreamToolCall(stc)
		}
		for _, update := range pp.streamBuffer.ConsumeNewlyReadyToolCalls() {
			tc := update.ToolCall
			pp.sendLog("proxy_tool_call_name", map[string]interface{}{
				"index": update.Index,
				"name":  tc.Function.Name,
			})
			pp.sendLog("monitor_upstream_tool_call", map[string]interface{}{
				"index": update.Index,
				"name":  tc.Function.Name,
			})
			if tc.Function.Arguments != "" {
				args := tc.Function.Arguments
				if len(args) > 200 {
					args = truncateString(args, 200)
				}
				pp.sendLog("proxy_tool_call_args", map[string]interface{}{
					"index": update.Index,
					"args":  args,
				})
			}
		}
	}

	if choice.FinishReason != "" {
		pp.sendLogForRequest(requestID, "proxy_stream_finished", map[string]interface{}{
			"reason": choice.FinishReason,
		})
		pp.updateTruthRecord(requestID, func(r *TruthRecord) {
			r.FinishReason = string(choice.FinishReason)
		})

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

			pp.metricsMu.Lock()
			pp.totalToolCalls += toolCallCount
			pp.metricsMu.Unlock()
			pp.sendMetricsToCallback()

			if len(contentWithTools) > 0 {
				displayContent := contentWithTools
				if len(displayContent) > 300 {
					displayContent = truncateString(displayContent, 300)
				}
				pp.sendLog("proxy_stream_content_with_tools", map[string]interface{}{
					"content": displayContent,
				})
			}

			pp.sendLogForRequest(requestID, "proxy_tool_call_count", map[string]interface{}{
				"count": toolCallCount,
			})
			for i, tc := range bufferToolCalls {
				pp.sendLogForRequest(requestID, "proxy_tool_call_name", map[string]interface{}{
					"index": i,
					"name":  tc.Function.Name,
				})
			}
			pp.updateTruthRecord(requestID, func(r *TruthRecord) {
				for _, tc := range bufferToolCalls {
					args := truncateToBytes(tc.Function.Arguments, maxRecordToolArgsBytes)
					r.ToolCalls = append(r.ToolCalls, RecordToolCall{
						ID:        tc.ID,
						Name:      tc.Function.Name,
						Arguments: args,
						Source:    "response",
					})
				}
				if contentWithTools == "" {
					applyRecordPrimaryContent(r, RecordContentNoText, "Assistant generated tool calls only.", false)
				}
			})
			pp.sendLogForRequest(requestID, "proxy_tool_calls_pending", nil)
		} else {
			pp.streamBuffer.mu.Lock()
			var accumulatedContent string
			for _, c := range pp.streamBuffer.contentChunks {
				accumulatedContent += c
			}
			pp.streamBuffer.mu.Unlock()

			if len(accumulatedContent) > 0 {
				logContent := accumulatedContent
				if len(logContent) > 300 {
					logContent = truncateString(logContent, 300)
				}
				recordContent := truncateToBytes(accumulatedContent, maxRecordMessageBytes)

				// DinTalClaw 内嵌协议：检测流式响应中的 <tool_use> 标签
				inlineResponseTools := extractInlineToolUses(accumulatedContent)
				if len(inlineResponseTools) > 0 {
					pp.sendLogForRequest(requestID, "proxy_stream_content_with_tools", map[string]interface{}{
						"content": logContent,
					})
					pp.updateTruthRecord(requestID, func(r *TruthRecord) {
						for j, it := range inlineResponseTools {
							synID := generateInlineToolCallID(r.MessageCount, j)
							isSensitive := false
							if pp.toolValidator != nil {
								isSensitive = pp.toolValidator.IsSensitive(it.Name)
							}
							r.ToolCalls = append(r.ToolCalls, RecordToolCall{
								ID:          synID,
								Name:        it.Name,
								Arguments:   truncateToBytes(it.RawArgs, maxRecordToolArgsBytes),
								IsSensitive: isSensitive,
								Source:      "response",
							})
						}
						assistantIndex := len(r.Messages)
						if assistantIndex < r.MessageCount {
							assistantIndex = r.MessageCount
						}
						r.Messages = append(r.Messages, RecordMessage{
							Index:   assistantIndex,
							Role:    "assistant",
							Content: recordContent,
						})
						applyRecordPrimaryContent(r, RecordContentNoText, "Assistant generated inline tool calls.", false)
					})
					pp.sendTerminalLog(fmt.Sprintf("[InlineTool] Detected %d <tool_use> in stream response", len(inlineResponseTools)))
					for i, it := range inlineResponseTools {
						pp.sendLogForRequest(requestID, "proxy_tool_call_name", map[string]interface{}{
							"index": i,
							"name":  it.Name,
						})
					}
				} else {
					pp.sendLogForRequest(requestID, "proxy_stream_content_no_tools", map[string]interface{}{
						"content": logContent,
					})
					pp.updateTruthRecord(requestID, func(r *TruthRecord) {
						assistantIndex := len(r.Messages)
						if assistantIndex < r.MessageCount {
							assistantIndex = r.MessageCount
						}
						if len(r.Messages) == 0 ||
							r.Messages[len(r.Messages)-1].Role != "assistant" ||
							r.Messages[len(r.Messages)-1].Content != recordContent {
							r.Messages = append(r.Messages, RecordMessage{
								Index:   assistantIndex,
								Role:    "assistant",
								Content: recordContent,
							})
						}
						applyRecordPrimaryContent(r, RecordContentAssistant, previewRecordContent(accumulatedContent, 300).Content, true)
					})
				}
			}
		}

		// Finalize
		pp.streamBuffer.mu.Lock()
		var outputContent string
		for _, c := range pp.streamBuffer.contentChunks {
			outputContent += c
		}
		promptTokens := pp.streamBuffer.promptTokens
		completionTokens := pp.streamBuffer.completionTokens
		totalTokens := pp.streamBuffer.totalTokens
		generatedToolCalls := pp.streamBuffer.toolCalls

		if totalTokens == 0 {
			promptTokens = calculateRequestTokensFromRaw(pp.streamBuffer.requestMessages, pp.streamBuffer.toolsRaw)
			completionTokens = estimateTokenCount(outputContent)
			if len(pp.streamBuffer.toolCalls) > 0 {
				if toolCallsBytes, err := json.Marshal(pp.streamBuffer.toolCalls); err == nil {
					completionTokens += estimateTokenCount(string(toolCallsBytes))
				}
			}
			totalTokens = promptTokens + completionTokens

			pp.metricsMu.Lock()
			pp.totalPromptTokens += promptTokens
			pp.totalCompletionTokens += completionTokens
			pp.totalTokens += totalTokens
			pp.metricsMu.Unlock()
			pp.sendMetricsToCallback()

			pp.streamBuffer.promptTokens = promptTokens
			pp.streamBuffer.completionTokens = completionTokens
			pp.streamBuffer.totalTokens = totalTokens

			pp.sendLogForRequest(requestID, "proxy_token_usage_estimated", map[string]interface{}{
				"promptTokens":     promptTokens,
				"completionTokens": completionTokens,
				"totalTokens":      totalTokens,
			})
		}
		pp.streamBuffer.mu.Unlock()

		pp.finalizeTruthRecord(requestID, outputContent, generatedToolCalls, promptTokens, completionTokens)
		pp.sendLog("monitor_upstream_completed", map[string]interface{}{
			"response_mode":        "stream",
			"final_text":           truncateString(outputContent, 2000),
			"finish_reason":        choice.FinishReason,
			"raw_response_preview": truncateString(outputContent, 2000),
		})
		pp.emitMonitorResponseReturned("COMPLETED", outputContent, outputContent)

		pp.streamBuffer.Clear()
	}

	return true
}

// formatQuotaExceededMessage 根据语言生成配额超限的模拟返回消息
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

	var quotaName string
	if quotaType == "session" {
		quotaName = "Session token quota exceeded"
	} else {
		quotaName = "Daily token quota exceeded"
	}
	return fmt.Sprintf("[ClawSecbot] Status: QUOTA_EXCEEDED | Reason: %s (%d/%d)\n\nThis request has been blocked. Please adjust quota settings or wait for quota reset.",
		quotaName, current, limit)
}
