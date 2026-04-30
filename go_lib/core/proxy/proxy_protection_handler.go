package proxy

// proxy_protection_handler.go contains request/response handlers for ProxyProtection.
// Request lifecycle updates are written through updateTruthRecord() as SSOT snapshots.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	chatmodelrouting "go_lib/chatmodel-routing"
	"go_lib/core/logging"

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

	requestPolicyState, requestPolicyDecision := pp.runRequestPolicyHooks(req, modelName)
	if requestPolicyDecision != nil {
		return pp.applyRequestPolicyDecision(ctx, req, rawBody, stream, modelName, requestPolicyState, requestPolicyDecision)
	}
	conversationUsage := requestPolicyState.ConversationUsage

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
	pp.bindRequestContext(ctx, requestID)
	pp.auditLogSafe("start_from_request_normal", func(tracker *AuditChainTracker) {
		tracker.StartFromRequest(requestID, pp.assetName, pp.assetID, modelName, req.Messages)
	})

	pp.updateTruthRecord(requestID, func(r *TruthRecord) {
		r.Model = modelName
		r.MessageCount = len(req.Messages)
		r.Messages = extractCurrentRoundRecordMessages(req.Messages)
		r.Phase = RecordPhaseStarting
		r.Decision = &SecurityDecision{Action: "ALLOW"}
		r.ConversationTokens = conversationUsage
		r.DailyTokens = pp.currentDailyTokenUsage()
	})

	pp.sendLog("proxy_new_request", map[string]interface{}{
		"model": modelName,
	})
	pp.emitMonitorRequestCreated(req, rawBody, stream)

	securityModel := ""
	if pp.shepherdGate != nil {
		securityModel = pp.shepherdGate.GetModelName()
	}
	logSecurityFlowInfo(securityFlowStageRequest, "received request: model=%s message_count=%d request_id=%s security_model=%s bot_target_url=%s", modelName, len(req.Messages), requestID, securityModel, pp.targetURL)
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
	pp.sendSecurityFlowLog(securityFlowStageRequest, "message_roles=%v", roles)

	// ==================== 工具结果检测逻辑 ====================
	protocol := analyzeRequestProtocol(requestID, req.Messages)
	toolCallsInHistory := protocol.ToolCallsInHistory
	latestAssistantToolCalls := protocol.LatestAssistantToolCall
	isInlineToolProtocol := protocol.IsInlineToolProtocol
	inlineToolResultsMap := protocol.InlineToolResults
	latestRoundTCIDs := protocol.LatestRoundToolCallIDs

	contextMessages := make([]ConversationMessage, 0, len(req.Messages))
	lastUserMessageContent := ""

	for i, msg := range req.Messages {
		cm := extractConversationMessage(msg)
		role := cm.Role
		content := cm.Content

		contextMessages = append(contextMessages, cm)

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

		if role == "user" {
			lastUserMessageContent = content
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
					synID := generateInlineToolCallID(requestID, i, j)
					toolCallsInHistory = append(toolCallsInHistory, toolCallRef{
						ID:       synID,
						FuncName: it.Name,
						RawArgs:  it.RawArgs,
					})
				}
			}
		}

	}

	toolResultsMap := collectTailToolResults(req.Messages)
	// DinTalClaw 内嵌协议：将 <tool_result> 结果合并到 toolResultsMap
	if isInlineToolProtocol && inlineToolResultsMap != nil {
		for id, result := range inlineToolResultsMap {
			toolResultsMap[id] = result
		}
	}
	hasToolResultMessages := len(toolResultsMap) > 0
	chain := pp.prepareSecurityChainForRequest(requestID, req.Messages, toolResultsMap)
	chainID := ""
	if chain != nil {
		chainID = chain.ChainID
	}
	pp.updateSecurityChainContext(requestID, contextMessages, lastUserMessageContent)
	pp.createRequestRuntimeState(requestID, chainID, req, rawBody)
	pp.updateTruthRecord(requestID, func(r *TruthRecord) {
		r.InstructionChainID = chainID
	})

	tailToolCount, tailToolWithID, tailToolMissingID, tailToolIDs := summarizeTailToolMessages(req.Messages)
	lastRole := ""
	if len(req.Messages) > 0 {
		lastRole = strings.TrimSpace(getMessageRole(req.Messages[len(req.Messages)-1]))
	}
	logging.Info(
		"[AuditChain] request observed: request_id=%s asset_id=%s model=%s message_count=%d last_role=%s tail_tools=%d tail_tools_with_id=%d tail_tools_missing_id=%d tail_tool_results=%d tail_tool_ids=%s tool_call_ids=%s",
		requestID,
		strings.TrimSpace(pp.assetID),
		modelName,
		len(req.Messages),
		lastRole,
		tailToolCount,
		tailToolWithID,
		tailToolMissingID,
		len(toolResultsMap),
		formatAuditToolIDSummary(tailToolIDs, 12),
		formatAuditToolResultMapSummary(toolResultsMap, 12),
	)
	if tailToolCount > 0 && tailToolMissingID > 0 {
		logging.Warning(
			"[AuditChain] request tail tool messages missing tool_call_id: request_id=%s missing=%d tail_tools=%d",
			requestID,
			tailToolMissingID,
			tailToolCount,
		)
	}
	pp.auditLogSafe("link_request_by_tool_results", func(tracker *AuditChainTracker) {
		tracker.LinkRequestByToolResults(requestID, pp.assetID, toolResultsMap)
		tracker.RecordToolResults(pp.assetID, toolResultsMap)
		tracker.SetRequestInstructionChainID(requestID, chainID)
	})

	// 将历史工具调用记录写入 TruthRecord，标记最新一轮
	for _, tc := range toolCallsInHistory {
		isSensitive := false
		if pp.toolValidator != nil {
			isSensitive = pp.toolValidator.IsSensitive(tc.FuncName)
		}
		tcID := strings.TrimSpace(tc.ID)
		result := ""
		if val, _, ok := toolResultContentByToolCallID(toolResultsMap, tcID); ok {
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

	pp.sendSecurityFlowLog(securityFlowStageToolCallResult, "trigger_check: has_tool_results=%v tool_result_count=%d latest_tool_calls=%d", hasToolResultMessages, len(toolResultsMap), len(latestAssistantToolCalls))

	hadPendingRecoveryBeforeArm := pp.hasPendingToolCallRecoveryForRequest(requestID)
	recoveryArmedForRequest := pp.armPendingRecoveryFromRequest(pp.ctx, requestID, req.Messages)
	recoveryRejectedForRequest := hadPendingRecoveryBeforeArm && !recoveryArmedForRequest && !pp.hasPendingToolCallRecoveryForRequest(requestID)
	recoveryRequiresToolResult := recoveryArmedForRequest && pp.pendingRecoveryRequiresToolResultForRequest(requestID)
	if recoveryArmedForRequest {
		pp.sendSecurityFlowLog(securityFlowStageRecovery, "user confirmation recognized; skipping user_input policy for this recovery request")
		pp.sendLog("proxy_pending_tool_recovery_armed", map[string]interface{}{
			"armed": true,
		})
	} else if recoveryRejectedForRequest {
		pp.sendSecurityFlowLog(securityFlowStageRecovery, "user rejection recognized; skipping user_input policy and keeping quarantined tool results isolated")
		pp.sendLog("proxy_pending_tool_recovery_rejected", map[string]interface{}{
			"rejected": true,
		})
	} else {
		userInputPolicyResult := pp.runUserInputPolicyHooks(ctx, userInputPolicyContext{
			RequestID: requestID,
			Messages:  req.Messages,
		})
		if userInputPolicyResult.Handled {
			return userInputPolicyResult.Result, userInputPolicyResult.Pass
		}
	}

	// ==================== ShepherdGate 安全检测 ====================
	toolPolicyResult := pp.runToolResultPolicyHooks(ctx, toolResultPolicyContext{
		RequestID:                requestID,
		HasToolResultMessages:    hasToolResultMessages,
		LatestAssistantToolCalls: latestAssistantToolCalls,
		ToolResultsMap:           toolResultsMap,
	})
	if toolPolicyResult.Handled {
		return toolPolicyResult.Result, toolPolicyResult.Pass
	}
	if recoveryArmedForRequest && !recoveryRequiresToolResult {
		pp.clearPendingToolCallRecoveryForRequest(requestID)
	}
	if recoveryArmedForRequest && recoveryRequiresToolResult && !hasToolResultMessages {
		toolCallIDs := pp.pendingRecoveryToolCallIDsForRequest(requestID)
		cleared := pp.clearBlockedToolCallIDsForRequest(requestID, toolCallIDs)
		pp.clearPendingToolCallRecoveryForRequest(requestID)
		pp.sendSecurityFlowLog(securityFlowStageRecovery, "confirmed historical recovery without tail tool results; allowing original history cleared_blocked_tool_call_ids=%d", cleared)
	}

	rewriteResult := pp.runRequestRewriteHooks(ctx, requestRewriteContext{
		RequestID: requestID,
		RawBody:   rawBody,
		Messages:  req.Messages,
	})

	if rewriteResult.Rewrote {
		return rewriteResult.Result, true
	}
	return nil, true
}

// onResponse handles non-streaming responses
func (pp *ProxyProtection) onResponse(ctx context.Context, resp *openai.ChatCompletion) bool {
	requestID := pp.requestIDFromContext(ctx)
	defer pp.clearRequestContext(ctx)
	defer pp.clearRequestRuntimeState(requestID)
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
	streamBuffer := pp.streamBufferForRequest(requestID)
	rawToolPayload := extractRawResponseToolPayload(resp.RawJSON())
	if len(rawToolPayload.Calls) > 0 {
		pp.sendSecurityFlowLog(securityFlowStageToolCall, "raw_response tool_call_count=%d", len(rawToolPayload.Calls))
		toolCallPolicyResult := pp.runToolCallPolicyHooks(ctx, toolCallPolicyContext{
			RequestID:     requestID,
			ToolCallInfos: rawToolPayload.Calls,
		})
		if toolCallPolicyResult.Handled && toolCallPolicyResult.Decision != nil {
			return pp.applyResponseSecurityPolicyDecision(ctx, requestID, *toolCallPolicyResult.Decision, false)
		}
		pp.metricsMu.Lock()
		pp.totalToolCalls += len(rawToolPayload.Calls)
		pp.metricsMu.Unlock()
		pp.sendMetricsToCallback()
	}
	if len(rawToolPayload.Results) > 0 {
		toolResultPolicyResult := pp.runToolResultPolicyHooks(ctx, toolResultPolicyContext{
			RequestID:                requestID,
			HasToolResultMessages:    true,
			LatestAssistantToolCalls: toolCallRefsFromInfos(rawToolPayload.Calls),
			ToolResultsMap:           toolResultMapFromInfos(rawToolPayload.Results),
		})
		if toolResultPolicyResult.Handled {
			return toolResultPolicyResult.Pass
		}
	}

	if len(resp.Choices) > 0 {
		msg := &resp.Choices[0].Message
		outputContent = msg.Content
		generatedToolCalls = msg.ToolCalls
		if ensureResponseToolCallIDs(generatedToolCalls) {
			msg.ToolCalls = generatedToolCalls
		}
		pp.bindToolCallsToSecurityChain(requestID, generatedToolCalls)
		toolCallPolicyResult := pp.runToolCallPolicyHooks(ctx, toolCallPolicyContext{
			RequestID: requestID,
			ToolCalls: generatedToolCalls,
		})
		if toolCallPolicyResult.Handled && toolCallPolicyResult.Decision != nil {
			return pp.applyResponseSecurityPolicyDecision(ctx, requestID, *toolCallPolicyResult.Decision, false)
		}
		if len(generatedToolCalls) == 0 && strings.TrimSpace(outputContent) != "" {
			finalPolicyResult := pp.runFinalResultPolicyHooks(ctx, finalResultPolicyContext{
				RequestID: requestID,
				Content:   outputContent,
			})
			if finalPolicyResult.Handled && finalPolicyResult.Decision != nil {
				if !finalPolicyResult.Pass {
					return pp.applyResponseSecurityPolicyDecision(ctx, requestID, *finalPolicyResult.Decision, false)
				}
				if finalPolicyResult.Mutated {
					msg.Content = finalPolicyResult.Content
					outputContent = finalPolicyResult.Content
					pp.recordFinalResultPolicyEvent(requestID, *finalPolicyResult.Decision)
				}
			}
		}

		pp.sendSecurityFlowLog(securityFlowStageToolCall, "non_stream_response tool_call_count=%d", len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			pp.sendSecurityFlowLog(securityFlowStageToolCall, "non_stream_response tool_call name=%s id=%s", tc.Function.Name, tc.ID)
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
		streamBuffer.mu.Lock()
		reqMsgs := make([]ConversationMessage, len(streamBuffer.requestMessages))
		copy(reqMsgs, streamBuffer.requestMessages)
		toolsRaw := make([]byte, len(streamBuffer.toolsRaw))
		copy(toolsRaw, streamBuffer.toolsRaw)
		streamBuffer.mu.Unlock()

		promptTokens = calculateRequestTokensFromRaw(reqMsgs, toolsRaw)
		completionTokens = estimateTokenCount(outputContent)
		if len(generatedToolCalls) > 0 {
			if toolCallsBytes, err := json.Marshal(generatedToolCalls); err == nil {
				completionTokens += estimateTokenCount(string(toolCallsBytes))
			}
		}
		logging.Info(
			"[AuditChain] response observed: request_id=%s mode=non_stream tool_calls=%d output_len=%d",
			requestID,
			len(generatedToolCalls),
			len(strings.TrimSpace(outputContent)),
		)
		totalTokens = promptTokens + completionTokens
	}

	if totalTokens > 0 {
		pp.metricsMu.Lock()
		pp.totalPromptTokens += promptTokens
		pp.totalCompletionTokens += completionTokens
		pp.totalTokens += totalTokens
		pp.metricsMu.Unlock()
		conversationTotal := pp.addConversationTokens(totalTokens)
		dailyTotal := pp.currentDailyTokenUsage()
		pp.updateRecordTokenTotals(requestID, promptTokens, completionTokens, totalTokens, conversationTotal, dailyTotal)

		if hasUsageFromUpstream {
			pp.sendLogForRequest(requestID, "proxy_token_usage", map[string]interface{}{
				"promptTokens":       promptTokens,
				"completionTokens":   completionTokens,
				"totalTokens":        totalTokens,
				"conversationTokens": conversationTotal,
			})
		} else {
			pp.sendLogForRequest(requestID, "proxy_token_usage_estimated", map[string]interface{}{
				"promptTokens":       promptTokens,
				"completionTokens":   completionTokens,
				"totalTokens":        totalTokens,
				"conversationTokens": conversationTotal,
			})
		}
		pp.sendMetricsToCallback()
	}
	pp.auditLogSafe("update_tokens_on_response", func(tracker *AuditChainTracker) {
		tracker.UpdateRequestTokens(requestID, promptTokens, completionTokens, totalTokens)
	})

	if len(generatedToolCalls) > 0 {
		logging.Info(
			"[AuditChain] finalize assistant output skipped: request_id=%s mode=non_stream reason=tool_calls_present tool_calls=%d",
			requestID,
			len(generatedToolCalls),
		)
		pp.auditLogSafe("record_toolcalls_on_response", func(tracker *AuditChainTracker) {
			tracker.RecordToolCallsForRequest(requestID, pp.assetID, generatedToolCalls, pp.toolValidator)
		})
	} else {
		logging.Info(
			"[AuditChain] finalize assistant output begin: request_id=%s mode=non_stream output_len=%d",
			requestID,
			len(strings.TrimSpace(outputContent)),
		)
		pp.auditLogSafe("finalize_output_on_response", func(tracker *AuditChainTracker) {
			tracker.FinalizeRequestOutput(requestID, outputContent)
		})
	}

	// Finalize via TruthRecord
	pp.finalizeTruthRecord(requestID, outputContent, generatedToolCalls, promptTokens, completionTokens, totalTokens)
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
	requestID := pp.requestIDFromContext(ctx)
	streamBuffer := pp.streamBufferForRequest(requestID)
	var currentRequestPromptTokens, currentRequestCompletionTokens, currentRequestTotalTokens int

	if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 || chunk.Usage.TotalTokens > 0 {
		currentRequestPromptTokens = int(chunk.Usage.PromptTokens)
		currentRequestCompletionTokens = int(chunk.Usage.CompletionTokens)
		currentRequestTotalTokens = int(chunk.Usage.TotalTokens)
		if currentRequestTotalTokens == 0 && (currentRequestPromptTokens > 0 || currentRequestCompletionTokens > 0) {
			currentRequestTotalTokens = currentRequestPromptTokens + currentRequestCompletionTokens
		}

		streamBuffer.mu.Lock()
		prevPromptTokens := streamBuffer.promptTokens
		prevCompletionTokens := streamBuffer.completionTokens
		prevTotalTokens := streamBuffer.totalTokens
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
		streamBuffer.promptTokens = currentRequestPromptTokens
		streamBuffer.completionTokens = currentRequestCompletionTokens
		streamBuffer.totalTokens = currentRequestTotalTokens
		streamBuffer.mu.Unlock()

		pp.metricsMu.Lock()
		pp.totalPromptTokens += deltaPromptTokens
		pp.totalCompletionTokens += deltaCompletionTokens
		pp.totalTokens += deltaTotalTokens
		pp.metricsMu.Unlock()
		conversationTotal := pp.addConversationTokens(deltaTotalTokens)
		dailyTotal := pp.currentDailyTokenUsage()
		pp.updateRecordTokenTotals(
			requestID,
			currentRequestPromptTokens,
			currentRequestCompletionTokens,
			currentRequestTotalTokens,
			conversationTotal,
			dailyTotal,
		)

		pp.sendLogForRequest(requestID, "proxy_token_usage", map[string]interface{}{
			"promptTokens":       currentRequestPromptTokens,
			"completionTokens":   currentRequestCompletionTokens,
			"totalTokens":        currentRequestTotalTokens,
			"conversationTokens": conversationTotal,
		})
		pp.sendMetricsToCallback()
	}

	if len(chunk.Choices) == 0 {
		return true
	}

	choice := &chunk.Choices[0]

	if choice.Delta.Content == "" && choice.Delta.Role == "" && len(choice.Delta.ToolCalls) == 0 && choice.FinishReason == "" {
		return true
	}

	if choice.Delta.Content != "" {
		finalPolicyResult := pp.runFinalResultPolicyHooks(ctx, finalResultPolicyContext{
			RequestID: requestID,
			Content:   choice.Delta.Content,
			Stream:    true,
		})
		if finalPolicyResult.Handled && finalPolicyResult.Decision != nil {
			if !finalPolicyResult.Pass {
				securityMsg := pp.formatPolicySecurityMessage(*finalPolicyResult.Decision)
				choice.Delta.Content = securityMsg
				choice.FinishReason = "stop"
				return pp.applyResponseSecurityPolicyDecision(ctx, requestID, *finalPolicyResult.Decision, true)
			}
			if finalPolicyResult.Mutated {
				choice.Delta.Content = finalPolicyResult.Content
				pp.recordFinalResultPolicyEvent(requestID, *finalPolicyResult.Decision)
			}
		}

		if !streamBuffer.started {
			streamBuffer.started = true
			pp.sendLog("monitor_upstream_stream_started", map[string]interface{}{
				"response_mode": "stream",
			})
		}

		streamBuffer.mu.Lock()
		prevLen := 0
		for _, c := range streamBuffer.contentChunks {
			prevLen += len(c)
		}
		streamBuffer.mu.Unlock()

		streamBuffer.AppendContent(choice.Delta.Content)
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
		readyToolCalls := make([]openai.ChatCompletionMessageToolCall, 0)
		for i := range choice.Delta.ToolCalls {
			originalID := strings.TrimSpace(choice.Delta.ToolCalls[i].ID)
			resolvedID := streamBuffer.MergeStreamToolCall(choice.Delta.ToolCalls[i])
			if originalID == "" && resolvedID != "" {
				choice.Delta.ToolCalls[i].ID = resolvedID
			}
		}
		for _, update := range streamBuffer.ConsumeNewlyReadyToolCalls() {
			tc := update.ToolCall
			readyToolCalls = append(readyToolCalls, tc)
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
		if len(readyToolCalls) > 0 {
			pp.bindToolCallsToSecurityChain(requestID, readyToolCalls)
			pp.sendSecurityFlowLog(securityFlowStageToolCall, "stream_delta tool_call metadata observed; defer security analysis until stream finish")
		}
		if len(readyToolCalls) > 0 {
			pp.auditLogSafe("record_toolcalls_on_stream_delta", func(tracker *AuditChainTracker) {
				tracker.RecordToolCallsForRequest(requestID, pp.assetID, readyToolCalls, pp.toolValidator)
			})
		}
	}

	if choice.FinishReason != "" {
		var finishToolCallCount int
		pp.sendLogForRequest(requestID, "proxy_stream_finished", map[string]interface{}{
			"reason": choice.FinishReason,
		})
		pp.updateTruthRecord(requestID, func(r *TruthRecord) {
			r.FinishReason = string(choice.FinishReason)
		})

		if streamBuffer.HasToolCalls() {
			streamBuffer.mu.Lock()
			toolCallCount := len(streamBuffer.toolCalls)
			finishToolCallCount = toolCallCount
			bufferToolCalls := make([]openai.ChatCompletionMessageToolCall, len(streamBuffer.toolCalls))
			copy(bufferToolCalls, streamBuffer.toolCalls)
			var contentWithTools string
			for _, c := range streamBuffer.contentChunks {
				contentWithTools += c
			}
			streamBuffer.mu.Unlock()
			pp.bindToolCallsToSecurityChain(requestID, bufferToolCalls)

			pp.sendSecurityFlowLog(securityFlowStageToolCall, "stream_response tool_call_count=%d", toolCallCount)
			toolCallPolicyResult := pp.runToolCallPolicyHooks(ctx, toolCallPolicyContext{
				RequestID: requestID,
				ToolCalls: bufferToolCalls,
			})
			if toolCallPolicyResult.Handled && toolCallPolicyResult.Decision != nil {
				choice.Delta.ToolCalls = nil
				choice.Delta.Content = pp.formatPolicySecurityMessage(*toolCallPolicyResult.Decision)
				choice.FinishReason = "stop"
				return pp.applyResponseSecurityPolicyDecision(ctx, requestID, *toolCallPolicyResult.Decision, true)
			}

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
						ID:          tc.ID,
						Name:        tc.Function.Name,
						Arguments:   args,
						Source:      "response",
						LatestRound: true,
					})
				}
				if contentWithTools == "" {
					applyRecordPrimaryContent(r, RecordContentNoText, "Assistant generated tool calls only.", false)
				}
			})
			pp.sendLogForRequest(requestID, "proxy_tool_calls_pending", nil)
			pp.auditLogSafe("record_toolcalls_on_stream_finish", func(tracker *AuditChainTracker) {
				tracker.RecordToolCallsForRequest(requestID, pp.assetID, bufferToolCalls, pp.toolValidator)
			})
		} else {
			streamBuffer.mu.Lock()
			var accumulatedContent string
			for _, c := range streamBuffer.contentChunks {
				accumulatedContent += c
			}
			streamBuffer.mu.Unlock()

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
							synID := generateInlineToolCallID(requestID, r.MessageCount, j)
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
								LatestRound: true,
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
					pp.sendSecurityFlowLog(securityFlowStageToolCall, "stream_response inline_tool_use_count=%d", len(inlineResponseTools))
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
		streamBuffer.mu.Lock()
		var outputContent string
		for _, c := range streamBuffer.contentChunks {
			outputContent += c
		}
		promptTokens := streamBuffer.promptTokens
		completionTokens := streamBuffer.completionTokens
		totalTokens := streamBuffer.totalTokens
		generatedToolCalls := streamBuffer.toolCalls

		if totalTokens == 0 {
			promptTokens = calculateRequestTokensFromRaw(streamBuffer.requestMessages, streamBuffer.toolsRaw)
			completionTokens = estimateTokenCount(outputContent)
			if len(streamBuffer.toolCalls) > 0 {
				if toolCallsBytes, err := json.Marshal(streamBuffer.toolCalls); err == nil {
					completionTokens += estimateTokenCount(string(toolCallsBytes))
				}
			}
			totalTokens = promptTokens + completionTokens

			pp.metricsMu.Lock()
			pp.totalPromptTokens += promptTokens
			pp.totalCompletionTokens += completionTokens
			pp.totalTokens += totalTokens
			pp.metricsMu.Unlock()
			conversationTotal := pp.addConversationTokens(totalTokens)
			dailyTotal := pp.currentDailyTokenUsage()
			pp.updateRecordTokenTotals(
				requestID,
				promptTokens,
				completionTokens,
				totalTokens,
				conversationTotal,
				dailyTotal,
			)
			pp.sendMetricsToCallback()

			streamBuffer.promptTokens = promptTokens
			streamBuffer.completionTokens = completionTokens
			streamBuffer.totalTokens = totalTokens

			pp.sendLogForRequest(requestID, "proxy_token_usage_estimated", map[string]interface{}{
				"promptTokens":       promptTokens,
				"completionTokens":   completionTokens,
				"totalTokens":        totalTokens,
				"conversationTokens": conversationTotal,
			})
		}
		streamBuffer.mu.Unlock()

		if len(generatedToolCalls) == 0 && strings.TrimSpace(outputContent) != "" {
			finalPolicyResult := pp.runFinalResultPolicyHooks(ctx, finalResultPolicyContext{
				RequestID: requestID,
				Content:   outputContent,
			})
			if finalPolicyResult.Handled && finalPolicyResult.Decision != nil {
				if !finalPolicyResult.Pass {
					choice.Delta.ToolCalls = nil
					choice.Delta.Content = pp.formatPolicySecurityMessage(*finalPolicyResult.Decision)
					choice.FinishReason = "stop"
					return pp.applyResponseSecurityPolicyDecision(ctx, requestID, *finalPolicyResult.Decision, true)
				}
				if finalPolicyResult.Mutated {
					outputContent = finalPolicyResult.Content
					pp.recordFinalResultPolicyEvent(requestID, *finalPolicyResult.Decision)
				}
			}
		}
		logging.Info(
			"[AuditChain] response observed: request_id=%s mode=stream finish_reason=%s tool_calls=%d output_len=%d",
			requestID,
			string(choice.FinishReason),
			finishToolCallCount,
			len(strings.TrimSpace(outputContent)),
		)
		pp.auditLogSafe("update_tokens_on_stream_finish", func(tracker *AuditChainTracker) {
			tracker.UpdateRequestTokens(requestID, promptTokens, completionTokens, totalTokens)
		})
		if len(generatedToolCalls) == 0 {
			logging.Info(
				"[AuditChain] finalize assistant output begin: request_id=%s mode=stream output_len=%d",
				requestID,
				len(strings.TrimSpace(outputContent)),
			)
			pp.auditLogSafe("finalize_output_on_stream_finish", func(tracker *AuditChainTracker) {
				tracker.FinalizeRequestOutput(requestID, outputContent)
			})
		} else {
			logging.Info(
				"[AuditChain] finalize assistant output skipped: request_id=%s mode=stream reason=tool_calls_present tool_calls=%d",
				requestID,
				len(generatedToolCalls),
			)
		}

		pp.finalizeTruthRecord(requestID, outputContent, generatedToolCalls, promptTokens, completionTokens, totalTokens)
		pp.sendLog("monitor_upstream_completed", map[string]interface{}{
			"response_mode":        "stream",
			"final_text":           truncateString(outputContent, 2000),
			"finish_reason":        choice.FinishReason,
			"raw_response_preview": truncateString(outputContent, 2000),
		})
		pp.emitMonitorResponseReturned("COMPLETED", outputContent, outputContent)

		pp.clearRequestContext(ctx)
		pp.clearRequestRuntimeState(requestID)
		streamBuffer.Clear()
	}

	return true
}
