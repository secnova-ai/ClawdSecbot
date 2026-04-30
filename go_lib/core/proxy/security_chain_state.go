package proxy

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/openai/openai-go"
)

const (
	securityChainTTL        = 2 * time.Hour
	requestRuntimeStateTTL  = 30 * time.Minute
	securityChainSourceUser = "user_input"
	securityChainSourceTool = "tool_result"
	securityChainSourceHist = "request_history"
	securityChainSourceTemp = "temporary"
)

type SecurityChainState struct {
	ChainID       string
	AssetID       string
	Source        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ExpiresAt     time.Time
	RootRequestID string

	Degraded      bool
	DegradeReason string

	RequestIDs  map[string]struct{}
	ToolCallIDs map[string]struct{}

	LastContextMessages    []ConversationMessage
	LastUserMessageContent string

	PendingRecovery      *pendingToolCallRecovery
	PendingRecoveryArmed bool
	BlockedToolCallIDs   map[string]time.Time

	ConfirmedToolCallRiskType string
	ConfirmedToolCallUntil    time.Time
}

type RequestRuntimeState struct {
	RequestID    string
	ChainID      string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	StreamBuffer *StreamBuffer
}

type securityChainBinding struct {
	ChainID   string
	ExpiresAt time.Time
}

type securityChainMetadata struct {
	ChainID       string
	Source        string
	Degraded      bool
	DegradeReason string
}

func newSecurityChainState(chainID, assetID, requestID, source, degradeReason string, now time.Time) *SecurityChainState {
	source = strings.TrimSpace(source)
	if source == "" {
		source = securityChainSourceTemp
	}
	chain := &SecurityChainState{
		ChainID:            strings.TrimSpace(chainID),
		AssetID:            strings.TrimSpace(assetID),
		Source:             source,
		CreatedAt:          now,
		UpdatedAt:          now,
		ExpiresAt:          now.Add(securityChainTTL),
		RootRequestID:      strings.TrimSpace(requestID),
		DegradeReason:      strings.TrimSpace(degradeReason),
		RequestIDs:         make(map[string]struct{}),
		ToolCallIDs:        make(map[string]struct{}),
		BlockedToolCallIDs: make(map[string]time.Time),
	}
	chain.Degraded = chain.DegradeReason != "" || source == securityChainSourceTemp
	if requestID = strings.TrimSpace(requestID); requestID != "" {
		chain.RequestIDs[requestID] = struct{}{}
	}
	return chain
}

func securityChainID(prefix, requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	requestID = strings.ReplaceAll(requestID, " ", "_")
	return prefix + requestID
}

func (pp *ProxyProtection) ensureSecurityRuntimeLocked() {
	if pp.chains == nil {
		pp.chains = make(map[string]*SecurityChainState)
	}
	if pp.requestToChain == nil {
		pp.requestToChain = make(map[string]securityChainBinding)
	}
	if pp.toolCallToChains == nil {
		pp.toolCallToChains = make(map[string]map[string]securityChainBinding)
	}
	if pp.requestStates == nil {
		pp.requestStates = make(map[string]*RequestRuntimeState)
	}
}

func (pp *ProxyProtection) cleanupExpiredSecurityRuntimeLocked(now time.Time) {
	pp.ensureSecurityRuntimeLocked()
	for requestID, binding := range pp.requestToChain {
		if !binding.ExpiresAt.After(now) {
			delete(pp.requestToChain, requestID)
		}
	}
	for toolCallID, chains := range pp.toolCallToChains {
		for chainID, binding := range chains {
			if !binding.ExpiresAt.After(now) {
				delete(chains, chainID)
			}
		}
		if len(chains) == 0 {
			delete(pp.toolCallToChains, toolCallID)
		}
	}
	for requestID, state := range pp.requestStates {
		if state == nil || !state.ExpiresAt.After(now) {
			delete(pp.requestStates, requestID)
		}
	}
	for chainID, chain := range pp.chains {
		if chain == nil || !chain.ExpiresAt.After(now) {
			delete(pp.chains, chainID)
			logSecurityFlowInfo(securityFlowStageChain, "chain_expired: instruction_chain_id=%s", chainID)
			continue
		}
		for toolCallID, expiresAt := range chain.BlockedToolCallIDs {
			if !expiresAt.After(now) {
				delete(chain.BlockedToolCallIDs, toolCallID)
			}
		}
	}
}

func (pp *ProxyProtection) createSecurityChain(requestID, source, degradeReason string) *SecurityChainState {
	if pp == nil {
		return nil
	}
	prefix := "sg_chain_"
	if source == securityChainSourceHist {
		prefix = "sg_recovered_"
	} else if strings.TrimSpace(degradeReason) != "" || source == securityChainSourceTemp {
		prefix = "sg_degraded_"
	}
	now := time.Now()
	chainID := securityChainID(prefix, requestID)

	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	if existing := pp.chains[chainID]; existing != nil {
		existing.UpdatedAt = now
		existing.ExpiresAt = now.Add(securityChainTTL)
		pp.bindRequestToSecurityChainLocked(requestID, chainID, now)
		return existing
	}

	chain := newSecurityChainState(chainID, pp.assetID, requestID, source, degradeReason, now)
	pp.chains[chainID] = chain
	pp.bindRequestToSecurityChainLocked(requestID, chainID, now)
	logSecurityFlowInfo(
		securityFlowStageChain,
		"chain_created: instruction_chain_id=%s request_id=%s source=%s degraded=%v reason=%s",
		chainID,
		strings.TrimSpace(requestID),
		chain.Source,
		chain.Degraded,
		chain.DegradeReason,
	)
	return chain
}

func (pp *ProxyProtection) bindRequestToSecurityChainLocked(requestID, chainID string, now time.Time) {
	requestID = strings.TrimSpace(requestID)
	chainID = strings.TrimSpace(chainID)
	if requestID == "" || chainID == "" {
		return
	}
	pp.requestToChain[requestID] = securityChainBinding{ChainID: chainID, ExpiresAt: now.Add(securityChainTTL)}
	if chain := pp.chains[chainID]; chain != nil {
		chain.RequestIDs[requestID] = struct{}{}
		chain.UpdatedAt = now
		chain.ExpiresAt = now.Add(securityChainTTL)
	}
	logSecurityFlowInfo(securityFlowStageChain, "chain_bound_request: instruction_chain_id=%s request_id=%s", chainID, requestID)
}

func (pp *ProxyProtection) bindRequestToSecurityChain(requestID, chainID string) {
	if pp == nil {
		return
	}
	now := time.Now()
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	pp.bindRequestToSecurityChainLocked(requestID, chainID, now)
}

func (pp *ProxyProtection) bindToolCallIDsToSecurityChain(requestID string, toolCallIDs []string) {
	if pp == nil || len(toolCallIDs) == 0 {
		return
	}
	now := time.Now()
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	chainID := pp.chainIDForRequestLocked(requestID, now)
	if chainID == "" {
		logSecurityFlowWarning(securityFlowStageChain, "chain_bind_tool_call_skipped: request_id=%s reason=chain_not_found", strings.TrimSpace(requestID))
		return
	}
	chain := pp.chains[chainID]
	if chain == nil {
		return
	}
	for _, rawID := range toolCallIDs {
		toolCallID := normalizeBlockedToolCallID(rawID)
		if toolCallID == "" {
			continue
		}
		if pp.toolCallToChains[toolCallID] == nil {
			pp.toolCallToChains[toolCallID] = make(map[string]securityChainBinding)
		}
		for existingChainID := range pp.toolCallToChains[toolCallID] {
			if existingChainID != chainID {
				logSecurityFlowWarning(
					securityFlowStageChain,
					"chain_conflict: request_id=%s tool_call_id=%s existing_chain=%s new_chain=%s",
					strings.TrimSpace(requestID),
					toolCallID,
					existingChainID,
					chainID,
				)
			}
		}
		pp.toolCallToChains[toolCallID][chainID] = securityChainBinding{ChainID: chainID, ExpiresAt: now.Add(securityChainTTL)}
		chain.ToolCallIDs[toolCallID] = struct{}{}
		chain.UpdatedAt = now
		chain.ExpiresAt = now.Add(securityChainTTL)
		logSecurityFlowInfo(securityFlowStageChain, "chain_bound_tool_call: instruction_chain_id=%s request_id=%s tool_call_id=%s", chainID, strings.TrimSpace(requestID), toolCallID)
	}
}

func (pp *ProxyProtection) bindToolCallsToSecurityChain(requestID string, toolCalls []openai.ChatCompletionMessageToolCall) {
	ids := make([]string, 0, len(toolCalls))
	for _, tc := range toolCalls {
		if id := normalizeBlockedToolCallID(tc.ID); id != "" {
			ids = append(ids, id)
		}
	}
	pp.bindToolCallIDsToSecurityChain(requestID, ids)
}

func (pp *ProxyProtection) chainIDForRequestLocked(requestID string, now time.Time) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ""
	}
	if binding, ok := pp.requestToChain[requestID]; ok && binding.ExpiresAt.After(now) {
		return strings.TrimSpace(binding.ChainID)
	}
	return ""
}

func (pp *ProxyProtection) chainIDForRequest(requestID string) string {
	if pp == nil {
		return ""
	}
	now := time.Now()
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	return pp.chainIDForRequestLocked(requestID, now)
}

func (pp *ProxyProtection) chainByRequestID(requestID string) *SecurityChainState {
	if pp == nil {
		return nil
	}
	now := time.Now()
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	chainID := pp.chainIDForRequestLocked(requestID, now)
	if chainID == "" {
		return nil
	}
	return cloneSecurityChainState(pp.chains[chainID])
}

func cloneSecurityChainState(chain *SecurityChainState) *SecurityChainState {
	if chain == nil {
		return nil
	}
	out := *chain
	out.RequestIDs = cloneStringSet(chain.RequestIDs)
	out.ToolCallIDs = cloneStringSet(chain.ToolCallIDs)
	out.LastContextMessages = cloneConversationMessages(chain.LastContextMessages)
	out.BlockedToolCallIDs = make(map[string]time.Time, len(chain.BlockedToolCallIDs))
	for k, v := range chain.BlockedToolCallIDs {
		out.BlockedToolCallIDs[k] = v
	}
	if chain.PendingRecovery != nil {
		snapshot := *chain.PendingRecovery
		snapshot.ToolCalls = cloneToolCalls(chain.PendingRecovery.ToolCalls)
		snapshot.ToolCallIDs = cloneStringSlice(chain.PendingRecovery.ToolCallIDs)
		out.PendingRecovery = &snapshot
	}
	return &out
}

func cloneStringSet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}

func cloneConversationMessages(in []ConversationMessage) []ConversationMessage {
	out := make([]ConversationMessage, len(in))
	copy(out, in)
	return out
}

func (pp *ProxyProtection) resolveUniqueSecurityChainByToolCallIDsLocked(toolCallIDs []string, now time.Time) (string, bool) {
	candidates := make(map[string]struct{})
	seenToolCallIDs := 0
	hasUnresolved := false
	for _, rawID := range toolCallIDs {
		toolCallID := normalizeBlockedToolCallID(rawID)
		if toolCallID == "" {
			continue
		}
		seenToolCallIDs++
		activeBindings := 0
		for chainID, binding := range pp.toolCallToChains[toolCallID] {
			if binding.ExpiresAt.After(now) {
				candidates[chainID] = struct{}{}
				activeBindings++
			}
		}
		if activeBindings == 0 {
			hasUnresolved = true
		}
	}
	if seenToolCallIDs == 0 || hasUnresolved {
		return "", false
	}
	if len(candidates) != 1 {
		return "", len(candidates) > 1
	}
	for chainID := range candidates {
		return chainID, false
	}
	return "", false
}

func (pp *ProxyProtection) resolveUniqueSecurityChainByToolCallIDs(toolCallIDs []string) (string, bool) {
	if pp == nil {
		return "", false
	}
	now := time.Now()
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	return pp.resolveUniqueSecurityChainByToolCallIDsLocked(toolCallIDs, now)
}

func sortedToolResultIDs(toolResults map[string]string) []string {
	ids := make([]string, 0, len(toolResults))
	for rawID := range toolResults {
		if id := normalizeBlockedToolCallID(rawID); id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (pp *ProxyProtection) prepareSecurityChainForRequest(requestID string, messages []openai.ChatCompletionMessageParamUnion, toolResults map[string]string) *SecurityChainState {
	if pp == nil {
		return nil
	}
	requestID = strings.TrimSpace(requestID)
	toolCallIDs := sortedToolResultIDs(toolResults)

	if promptIndex, promptContent := latestRecoveryPromptBeforeLatestUser(messages); promptIndex >= 0 {
		recoveredIDs := toolCallIDsImmediatelyBefore(messages, promptIndex)
		chainID, conflicted := pp.resolveUniqueSecurityChainByToolCallIDs(recoveredIDs)
		if chainID != "" && !conflicted {
			pp.bindRequestToSecurityChain(requestID, chainID)
			pp.ensureRecoveredPendingRecovery(requestID, recoveredIDs, promptContent, recoveryReasonFromPrompt(promptContent))
			pp.markBlockedToolCallIDsForRequest(requestID, recoveredIDs)
			logSecurityFlowInfo(securityFlowStageChain, "chain_recovered_from_history: instruction_chain_id=%s request_id=%s tool_call_ids=%v", chainID, requestID, recoveredIDs)
			return pp.chainByRequestID(requestID)
		}
		reason := "request_history"
		if conflicted {
			reason = "ambiguous_recovery_tool_call_id"
		}
		chain := pp.createSecurityChain(requestID, securityChainSourceHist, reason)
		pp.ensureRecoveredPendingRecovery(requestID, recoveredIDs, promptContent, recoveryReasonFromPrompt(promptContent))
		pp.markBlockedToolCallIDsForRequest(requestID, recoveredIDs)
		logSecurityFlowInfo(securityFlowStageChain, "chain_recovered_from_history: instruction_chain_id=%s request_id=%s tool_call_ids=%v", chain.ChainID, requestID, recoveredIDs)
		return chain
	}

	if len(toolResults) > 0 {
		if len(toolCallIDs) == 0 {
			return pp.createSecurityChain(requestID, securityChainSourceTool, "missing_tool_call_id")
		}
		chainID, conflicted := pp.resolveUniqueSecurityChainByToolCallIDs(toolCallIDs)
		if chainID != "" && !conflicted {
			pp.bindRequestToSecurityChain(requestID, chainID)
			logSecurityFlowInfo(securityFlowStageChain, "chain_resolved_from_tool_result: instruction_chain_id=%s request_id=%s tool_call_ids=%v", chainID, requestID, toolCallIDs)
			return pp.chainByRequestID(requestID)
		}
		reason := "unresolved_tool_call_id"
		if conflicted {
			reason = "ambiguous_tool_call_id"
		}
		logSecurityFlowWarning(securityFlowStageChain, "chain_degraded: reason=%s request_id=%s tool_call_ids=%v", reason, requestID, toolCallIDs)
		return pp.createSecurityChain(requestID, securityChainSourceTool, reason)
	}

	lastRole := ""
	if len(messages) > 0 {
		lastRole = strings.TrimSpace(getMessageRole(messages[len(messages)-1]))
	}
	if strings.EqualFold(lastRole, "user") {
		return pp.createSecurityChain(requestID, securityChainSourceUser, "")
	}
	logSecurityFlowWarning(securityFlowStageChain, "chain_degraded: reason=non_user_request_without_tool_result request_id=%s", requestID)
	return pp.createSecurityChain(requestID, securityChainSourceTemp, "non_user_request_without_tool_result")
}

func (pp *ProxyProtection) updateSecurityChainContext(requestID string, contextMessages []ConversationMessage, lastUserMessage string) {
	if pp == nil {
		return
	}
	now := time.Now()
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	chainID := pp.chainIDForRequestLocked(requestID, now)
	if chainID == "" {
		return
	}
	chain := pp.chains[chainID]
	if chain == nil {
		return
	}
	chain.LastContextMessages = cloneConversationMessages(contextMessages)
	if strings.TrimSpace(lastUserMessage) != "" {
		chain.LastUserMessageContent = lastUserMessage
	}
	chain.UpdatedAt = now
	chain.ExpiresAt = now.Add(securityChainTTL)
}

func (pp *ProxyProtection) securityChainContext(requestID string) ([]ConversationMessage, string, securityChainMetadata, bool) {
	chain := pp.chainByRequestID(requestID)
	if chain == nil {
		logSecurityFlowWarning(securityFlowStageChain, "chain_not_found; using current request context only request_id=%s", strings.TrimSpace(requestID))
		return nil, "", securityChainMetadata{}, false
	}
	return cloneConversationMessages(chain.LastContextMessages), chain.LastUserMessageContent, securityChainMetadata{
		ChainID:       chain.ChainID,
		Source:        chain.Source,
		Degraded:      chain.Degraded,
		DegradeReason: chain.DegradeReason,
	}, true
}

func (pp *ProxyProtection) consumeConfirmedToolCallGrantForRequest(requestID string, decision securityPolicyDecision) bool {
	if pp == nil || decision.normalizedAction() != decisionActionNeedsConfirm {
		return false
	}
	decisionRiskType := normalizeRecoveryRiskType(decision.RiskType)
	if decisionRiskType == "" {
		return false
	}
	now := time.Now()
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	chainID := pp.chainIDForRequestLocked(requestID, now)
	if chainID == "" {
		return false
	}
	chain := pp.chains[chainID]
	if chain == nil || !chain.ConfirmedToolCallUntil.After(now) {
		return false
	}
	grantRiskType := normalizeRecoveryRiskType(chain.ConfirmedToolCallRiskType)
	if grantRiskType == "" || grantRiskType != decisionRiskType {
		return false
	}
	chain.ConfirmedToolCallRiskType = ""
	chain.ConfirmedToolCallUntil = time.Time{}
	chain.UpdatedAt = now
	chain.ExpiresAt = now.Add(securityChainTTL)
	logSecurityFlowInfo(
		securityFlowStageRecovery,
		"confirmed matching tool_call allowed once: instruction_chain_id=%s request_id=%s risk_type=%s tool_call_id=%s",
		chainID,
		strings.TrimSpace(requestID),
		decisionRiskType,
		strings.TrimSpace(decision.ToolCallID),
	)
	return true
}

func (pp *ProxyProtection) createRequestRuntimeState(requestID, chainID string, req *openai.ChatCompletionNewParams, rawBody []byte) *RequestRuntimeState {
	if pp == nil {
		return nil
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil
	}
	now := time.Now()
	state := &RequestRuntimeState{
		RequestID:    requestID,
		ChainID:      strings.TrimSpace(chainID),
		CreatedAt:    now,
		ExpiresAt:    now.Add(requestRuntimeStateTTL),
		StreamBuffer: NewStreamBuffer(),
	}
	if req != nil {
		state.StreamBuffer.SetRequest(requestID, req, rawBody)
	}
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	pp.requestStates[requestID] = state
	return state
}

func (pp *ProxyProtection) requestRuntimeState(requestID string) *RequestRuntimeState {
	if pp == nil {
		return nil
	}
	now := time.Now()
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	state := pp.requestStates[strings.TrimSpace(requestID)]
	if state == nil {
		return nil
	}
	out := *state
	return &out
}

func (pp *ProxyProtection) clearRequestRuntimeState(requestID string) {
	if pp == nil {
		return
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return
	}
	now := time.Now()
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	delete(pp.requestStates, requestID)
	logSecurityFlowInfo(securityFlowStageChain, "request_state_cleared: request_id=%s", requestID)
}

func (pp *ProxyProtection) streamBufferForRequest(requestID string) *StreamBuffer {
	if pp == nil {
		return nil
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil
	}
	now := time.Now()
	pp.chainMu.Lock()
	defer pp.chainMu.Unlock()
	pp.cleanupExpiredSecurityRuntimeLocked(now)
	if state := pp.requestStates[requestID]; state != nil && state.StreamBuffer != nil {
		state.ExpiresAt = now.Add(requestRuntimeStateTTL)
		return state.StreamBuffer
	}
	chainID := pp.chainIDForRequestLocked(requestID, now)
	state := &RequestRuntimeState{
		RequestID:    requestID,
		ChainID:      chainID,
		CreatedAt:    now,
		ExpiresAt:    now.Add(requestRuntimeStateTTL),
		StreamBuffer: NewStreamBuffer(),
	}
	state.StreamBuffer.requestID = requestID
	pp.requestStates[requestID] = state
	logSecurityFlowWarning(securityFlowStageChain, "request_state_recovered: request_id=%s instruction_chain_id=%s", requestID, chainID)
	return state.StreamBuffer
}

func (pp *ProxyProtection) chainMetadataForRequest(requestID string) securityChainMetadata {
	chain := pp.chainByRequestID(requestID)
	if chain == nil {
		return securityChainMetadata{}
	}
	return securityChainMetadata{
		ChainID:       chain.ChainID,
		Source:        chain.Source,
		Degraded:      chain.Degraded,
		DegradeReason: chain.DegradeReason,
	}
}
