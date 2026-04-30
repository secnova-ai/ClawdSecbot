package proxy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go_lib/core/logging"

	"github.com/openai/openai-go"
)

// AuditLog represents one full audit chain entry.
type AuditLog struct {
	ID                 string          `json:"id"`
	Timestamp          string          `json:"timestamp"`
	RequestID          string          `json:"request_id"`
	InstructionChainID string          `json:"instruction_chain_id,omitempty"`
	AssetName          string          `json:"asset_name,omitempty"`
	AssetID            string          `json:"asset_id,omitempty"`
	Model              string          `json:"model,omitempty"`
	RequestContent     string          `json:"request_content"`
	ToolCalls          []AuditToolCall `json:"tool_calls,omitempty"`
	OutputContent      string          `json:"output_content,omitempty"`
	HasRisk            bool            `json:"has_risk"`
	RiskLevel          string          `json:"risk_level,omitempty"`
	RiskReason         string          `json:"risk_reason,omitempty"`
	Confidence         int             `json:"confidence,omitempty"`
	Action             string          `json:"action"`
	PromptTokens       int             `json:"prompt_tokens,omitempty"`
	CompletionTokens   int             `json:"completion_tokens,omitempty"`
	TotalTokens        int             `json:"total_tokens,omitempty"`
	Duration           int64           `json:"duration_ms"`
	PersistSeq         int64           `json:"-"`
}

// AuditToolCall stores one tool invocation/result pair under a chain.
type AuditToolCall struct {
	Name        string `json:"name"`
	Arguments   string `json:"arguments"`
	Result      string `json:"result"`
	IsSensitive bool   `json:"is_sensitive,omitempty"`
}

type auditLogKeyBinding struct {
	LogID     string
	ExpiresAt time.Time
}

type auditLogState struct {
	Log        AuditLog
	CreatedAt  time.Time
	UpdatedAt  time.Time
	StartedAt  time.Time
	ToolSeq    []string
	ToolIndex  map[string]int
	SortCursor int64
}

type pendingToolResult struct {
	Result    string
	ExpiresAt time.Time
}

type pendingRequestLink struct {
	AssetID     string
	ToolCallIDs map[string]struct{}
	ExpiresAt   time.Time
}

type pendingFinalOutput struct {
	Output    string
	ExpiresAt time.Time
}

// AuditChainTracker tracks audit chains in memory and exposes pending snapshots.
type AuditChainTracker struct {
	mu sync.Mutex

	logs  map[string]*auditLogState
	order []string

	requestToLog  map[string]auditLogKeyBinding
	toolCallToLog map[string]auditLogKeyBinding
	// pendingToolResults buffers tool results that arrive before tool_call_id binding.
	pendingToolResults map[string]pendingToolResult
	// pendingRequestLinks buffers request->tool_call_id links that arrive before binding is ready.
	pendingRequestLinks map[string]pendingRequestLink
	// pendingFinalOutputs buffers assistant final output when request->log binding is not ready yet.
	pendingFinalOutputs map[string]pendingFinalOutput

	pending []AuditLog
	maxLen  int
}

var (
	auditLogIDCounter int64
	auditLogIDMu      sync.Mutex
)

const (
	auditRequestBindingTTL = 30 * time.Minute
	auditToolBindingTTL    = 2 * time.Hour
	auditDefaultMaxLen     = 1000
)

// NewAuditChainTracker creates a tracker instance.
func NewAuditChainTracker() *AuditChainTracker {
	return &AuditChainTracker{
		logs:                make(map[string]*auditLogState),
		order:               make([]string, 0, auditDefaultMaxLen),
		requestToLog:        make(map[string]auditLogKeyBinding),
		toolCallToLog:       make(map[string]auditLogKeyBinding),
		pendingToolResults:  make(map[string]pendingToolResult),
		pendingRequestLinks: make(map[string]pendingRequestLink),
		pendingFinalOutputs: make(map[string]pendingFinalOutput),
		pending:             make([]AuditLog, 0, auditDefaultMaxLen),
		maxLen:              auditDefaultMaxLen,
	}
}

func generateAuditLogID() string {
	auditLogIDMu.Lock()
	defer auditLogIDMu.Unlock()
	auditLogIDCounter++
	return fmt.Sprintf("audit_%d_%d", time.Now().UnixNano(), auditLogIDCounter)
}

func normalizeAuditToolCallIDForMatch(toolCallID string) string {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(toolCallID))
	for _, r := range toolCallID {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') {
			if r >= 'A' && r <= 'Z' {
				r = r - 'A' + 'a'
			}
			b.WriteRune(r)
		}
	}

	matchID := strings.TrimSpace(b.String())
	if matchID == "" {
		// Fallback to lowercased raw id for edge cases with non-alnum ids.
		return strings.ToLower(toolCallID)
	}
	return matchID
}

func normalizeAuditToolKey(assetID, toolCallID string) string {
	assetID = strings.TrimSpace(assetID)
	toolCallID = normalizeAuditToolCallIDForMatch(toolCallID)
	if assetID == "" || toolCallID == "" {
		return ""
	}
	return assetID + "|" + toolCallID
}

func formatAuditToolIDSummary(ids []string, limit int) string {
	if len(ids) == 0 {
		return "[]"
	}
	cleaned := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		cleaned = append(cleaned, id)
	}
	if len(cleaned) == 0 {
		return "[]"
	}
	sort.Strings(cleaned)
	if limit <= 0 || len(cleaned) <= limit {
		return "[" + strings.Join(cleaned, ",") + "]"
	}
	return "[" + strings.Join(cleaned[:limit], ",") + fmt.Sprintf(",...(%d total)]", len(cleaned))
}

func formatAuditToolResultMapSummary(toolResults map[string]string, limit int) string {
	if len(toolResults) == 0 {
		return "[]"
	}
	ids := make([]string, 0, len(toolResults))
	for id := range toolResults {
		ids = append(ids, id)
	}
	return formatAuditToolIDSummary(ids, limit)
}

func formatAuditToolIDSetSummary(toolIDs map[string]struct{}, limit int) string {
	if len(toolIDs) == 0 {
		return "[]"
	}
	ids := make([]string, 0, len(toolIDs))
	for id := range toolIDs {
		ids = append(ids, id)
	}
	return formatAuditToolIDSummary(ids, limit)
}

func summarizeAuditToolResultProgress(toolCalls []AuditToolCall) (int, int) {
	if len(toolCalls) == 0 {
		return 0, 0
	}
	withResult := 0
	for _, tc := range toolCalls {
		if strings.TrimSpace(tc.Result) != "" {
			withResult++
		}
	}
	return withResult, len(toolCalls)
}

func extractLastUserInstruction(messages []openai.ChatCompletionMessageParamUnion) (string, bool) {
	if len(messages) == 0 {
		return "", false
	}
	last := messages[len(messages)-1]
	if !strings.EqualFold(getMessageRole(last), "user") {
		return "", false
	}
	return strings.TrimSpace(extractMessageContent(last)), true
}

func cloneAuditLog(log AuditLog) AuditLog {
	copied := log
	if len(log.ToolCalls) > 0 {
		copied.ToolCalls = make([]AuditToolCall, len(log.ToolCalls))
		copy(copied.ToolCalls, log.ToolCalls)
	}
	return copied
}

func (t *AuditChainTracker) cleanupExpiredLocked(now time.Time) {
	for requestID, binding := range t.requestToLog {
		if now.After(binding.ExpiresAt) {
			delete(t.requestToLog, requestID)
		}
	}
	for toolKey, binding := range t.toolCallToLog {
		if now.After(binding.ExpiresAt) {
			delete(t.toolCallToLog, toolKey)
		}
	}
	for toolKey, pending := range t.pendingToolResults {
		if now.After(pending.ExpiresAt) {
			delete(t.pendingToolResults, toolKey)
		}
	}
	for requestID, link := range t.pendingRequestLinks {
		if now.After(link.ExpiresAt) {
			delete(t.pendingRequestLinks, requestID)
		}
	}
	for requestID, pending := range t.pendingFinalOutputs {
		if now.After(pending.ExpiresAt) {
			delete(t.pendingFinalOutputs, requestID)
		}
	}
}

func (t *AuditChainTracker) setRequestBindingLocked(requestID, logID string, now time.Time) {
	requestID = strings.TrimSpace(requestID)
	logID = strings.TrimSpace(logID)
	if requestID == "" || logID == "" {
		logging.Info(
			"[AuditChain] bind request->log skipped: request_id=%s log_id=%s reason=empty_key",
			requestID,
			logID,
		)
		return
	}
	logging.Info("[AuditChain] bind request->log: request_id=%s log_id=%s", requestID, logID)
	t.requestToLog[requestID] = auditLogKeyBinding{
		LogID:     logID,
		ExpiresAt: now.Add(auditRequestBindingTTL),
	}
	state, ok := t.logs[logID]
	if !ok || state == nil {
		return
	}
	if t.applyPendingFinalOutputLocked(requestID, state, now) {
		t.touchStateLocked(state, now)
	}
}

func (t *AuditChainTracker) setToolBindingLocked(assetID, toolCallID, logID string, now time.Time) {
	toolKey := normalizeAuditToolKey(assetID, toolCallID)
	if toolKey == "" || strings.TrimSpace(logID) == "" {
		logging.Info(
			"[AuditChain] bind tool_call_id->log skipped: asset_id=%s tool_call_id=%s log_id=%s reason=empty_key",
			strings.TrimSpace(assetID),
			strings.TrimSpace(toolCallID),
			strings.TrimSpace(logID),
		)
		return
	}
	logging.Info(
		"[AuditChain] bind tool_call_id->log: asset_id=%s tool_call_id=%s log_id=%s",
		strings.TrimSpace(assetID),
		strings.TrimSpace(toolCallID),
		strings.TrimSpace(logID),
	)
	t.toolCallToLog[toolKey] = auditLogKeyBinding{
		LogID:     logID,
		ExpiresAt: now.Add(auditToolBindingTTL),
	}
	// Resolve delayed request->log association when this tool binding becomes available.
	t.resolvePendingRequestLinksForToolLocked(assetID, toolCallID, logID, now)
}

func (t *AuditChainTracker) storePendingToolResultLocked(assetID, toolCallID, rawResult string, now time.Time) {
	toolKey := normalizeAuditToolKey(assetID, toolCallID)
	if toolKey == "" {
		return
	}
	logging.Info(
		"[AuditChain] store pending tool_result: asset_id=%s tool_call_id=%s result_len=%d",
		strings.TrimSpace(assetID),
		strings.TrimSpace(toolCallID),
		len(strings.TrimSpace(rawResult)),
	)
	t.pendingToolResults[toolKey] = pendingToolResult{
		Result:    truncateToBytes(strings.TrimSpace(rawResult), maxRecordMessageBytes),
		ExpiresAt: now.Add(auditToolBindingTTL),
	}
}

func (t *AuditChainTracker) popPendingToolResultLocked(assetID, toolCallID string) (string, bool) {
	toolKey := normalizeAuditToolKey(assetID, toolCallID)
	if toolKey == "" {
		return "", false
	}
	pending, ok := t.pendingToolResults[toolKey]
	if !ok {
		return "", false
	}
	delete(t.pendingToolResults, toolKey)
	return pending.Result, true
}

func (t *AuditChainTracker) storePendingRequestLinkIDsLocked(requestID, assetID string, toolCallIDs map[string]struct{}, now time.Time) {
	requestID = strings.TrimSpace(requestID)
	assetID = strings.TrimSpace(assetID)
	if requestID == "" || assetID == "" || len(toolCallIDs) == 0 {
		return
	}
	normalizedIDs := make(map[string]struct{}, len(toolCallIDs))
	for id := range toolCallIDs {
		matchID := normalizeAuditToolCallIDForMatch(id)
		if matchID == "" {
			continue
		}
		normalizedIDs[matchID] = struct{}{}
	}
	if len(normalizedIDs) == 0 {
		return
	}
	existing, ok := t.pendingRequestLinks[requestID]
	if ok && existing.AssetID == assetID {
		for id := range existing.ToolCallIDs {
			normalizedIDs[id] = struct{}{}
		}
	}
	t.pendingRequestLinks[requestID] = pendingRequestLink{
		AssetID:     assetID,
		ToolCallIDs: normalizedIDs,
		ExpiresAt:   now.Add(auditRequestBindingTTL),
	}
	logging.Info(
		"[AuditChain] store pending request link: request_id=%s asset_id=%s tool_call_ids=%s",
		requestID,
		assetID,
		formatAuditToolIDSetSummary(normalizedIDs, 12),
	)
}

func (t *AuditChainTracker) storePendingFinalOutputLocked(requestID, output string, now time.Time) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return
	}
	trimmed := strings.TrimSpace(output)
	logging.Info(
		"[AuditChain] store pending assistant output: request_id=%s output_len=%d",
		requestID,
		len(trimmed),
	)
	t.pendingFinalOutputs[requestID] = pendingFinalOutput{
		Output:    truncateToBytes(trimmed, maxRecordOutputBytes),
		ExpiresAt: now.Add(auditRequestBindingTTL),
	}
}

func (t *AuditChainTracker) popPendingFinalOutputLocked(requestID string) (string, bool) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return "", false
	}
	pending, ok := t.pendingFinalOutputs[requestID]
	if !ok {
		return "", false
	}
	delete(t.pendingFinalOutputs, requestID)
	return pending.Output, true
}

func (t *AuditChainTracker) applyPendingFinalOutputLocked(requestID string, state *auditLogState, now time.Time) bool {
	if state == nil {
		return false
	}
	pendingOutput, ok := t.popPendingFinalOutputLocked(requestID)
	if !ok {
		return false
	}
	logging.Info(
		"[AuditChain] apply pending assistant output: request_id=%s log_id=%s output_len=%d",
		strings.TrimSpace(requestID),
		state.Log.ID,
		len(pendingOutput),
	)
	changed := false
	if state.Log.OutputContent != pendingOutput {
		state.Log.OutputContent = pendingOutput
		changed = true
	}
	duration := now.Sub(state.StartedAt).Milliseconds()
	if state.Log.Duration != duration {
		state.Log.Duration = duration
		changed = true
	}
	return changed
}

func (t *AuditChainTracker) resolvePendingRequestLinksForToolLocked(assetID, toolCallID, logID string, now time.Time) {
	assetID = strings.TrimSpace(assetID)
	toolCallID = normalizeAuditToolCallIDForMatch(toolCallID)
	logID = strings.TrimSpace(logID)
	if assetID == "" || toolCallID == "" || logID == "" {
		return
	}
	for requestID, link := range t.pendingRequestLinks {
		if link.AssetID != assetID {
			continue
		}
		if _, ok := link.ToolCallIDs[toolCallID]; !ok {
			continue
		}
		logging.Info(
			"[AuditChain] resolve pending request link by tool_call_id: request_id=%s asset_id=%s tool_call_id=%s log_id=%s",
			requestID,
			assetID,
			toolCallID,
			logID,
		)
		t.setRequestBindingLocked(requestID, logID, now)
		delete(t.pendingRequestLinks, requestID)
	}
}

func (t *AuditChainTracker) getStateByRequestLocked(requestID string) *auditLogState {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil
	}
	binding, ok := t.requestToLog[requestID]
	if !ok {
		return nil
	}
	state, ok := t.logs[binding.LogID]
	if !ok {
		delete(t.requestToLog, requestID)
		return nil
	}
	return state
}

func (t *AuditChainTracker) getStateByToolCallLocked(assetID, toolCallID string) *auditLogState {
	toolKey := normalizeAuditToolKey(assetID, toolCallID)
	if toolKey == "" {
		return nil
	}
	binding, ok := t.toolCallToLog[toolKey]
	if !ok {
		return nil
	}
	state, ok := t.logs[binding.LogID]
	if !ok {
		delete(t.toolCallToLog, toolKey)
		return nil
	}
	return state
}

func (t *AuditChainTracker) appendPendingLocked(state *auditLogState) AuditLog {
	if state == nil {
		return AuditLog{}
	}
	copied := cloneAuditLog(state.Log)
	t.pending = append(t.pending, copied)
	if len(t.pending) > t.maxLen*3 {
		t.pending = t.pending[len(t.pending)-t.maxLen*3:]
	}
	return copied
}

func (t *AuditChainTracker) touchStateLocked(state *auditLogState, now time.Time) {
	if state == nil {
		return
	}
	state.UpdatedAt = now
	state.SortCursor++
	state.Log.PersistSeq = state.SortCursor
	if state.Log.TotalTokens <= 0 {
		state.Log.TotalTokens = state.Log.PromptTokens + state.Log.CompletionTokens
	}
	withResult, total := summarizeAuditToolResultProgress(state.Log.ToolCalls)
	logging.Info(
		"[AuditChain] snapshot queued: log_id=%s seq=%d tool_results=%d/%d output_len=%d tokens=%d",
		state.Log.ID,
		state.Log.PersistSeq,
		withResult,
		total,
		len(strings.TrimSpace(state.Log.OutputContent)),
		state.Log.TotalTokens,
	)
	snapshot := t.appendPendingLocked(state)
	enqueueAuditLogPersist(snapshot)
}

func (t *AuditChainTracker) evictOldestLocked() {
	for len(t.order) > t.maxLen {
		oldestID := t.order[0]
		t.order = t.order[1:]
		delete(t.logs, oldestID)
	}
}

// backfillStaleDurationsLocked 把同 asset 下仍未 finalize 的旧 log 的耗时补上。
//
// 触发场景：模型在多轮工具调用过程中尚未给出最终回复，用户已发起新一轮提问，
// 此时旧 log 永远不会到达 FinalizeRequestOutput，Duration 会一直停在 0。
// 这里仅根据 (now - StartedAt) 写入毫秒耗时，不修改 Action / RiskLevel / OutputContent，
// 保持决策语义不变；写入后通过 touchStateLocked 让持久化层覆盖更新。
func (t *AuditChainTracker) backfillStaleDurationsLocked(assetID string, now time.Time) {
	if assetID == "" {
		return
	}
	for _, state := range t.logs {
		if state == nil {
			continue
		}
		if strings.TrimSpace(state.Log.AssetID) != assetID {
			continue
		}
		if state.Log.Duration > 0 {
			continue
		}
		if state.StartedAt.IsZero() {
			continue
		}
		duration := now.Sub(state.StartedAt).Milliseconds()
		if duration <= 0 {
			continue
		}
		state.Log.Duration = duration
		logging.Info(
			"[AuditChain] backfill stale duration on new request: log_id=%s asset_id=%s duration_ms=%d",
			state.Log.ID,
			assetID,
			duration,
		)
		t.touchStateLocked(state, now)
	}
}

// StartFromRequest creates a new audit log when request messages end with role=user.
func (t *AuditChainTracker) StartFromRequest(
	requestID,
	assetName,
	assetID,
	model string,
	messages []openai.ChatCompletionMessageParamUnion,
) {
	if t == nil {
		return
	}
	instruction, ok := extractLastUserInstruction(messages)
	if !ok {
		lastRole := ""
		if len(messages) > 0 {
			lastRole = strings.TrimSpace(getMessageRole(messages[len(messages)-1]))
		}
		logging.Info(
			"[AuditChain] start skipped: request_id=%s reason=last_message_not_user last_role=%s message_count=%d",
			strings.TrimSpace(requestID),
			lastRole,
			len(messages),
		)
		return
	}

	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()

	t.cleanupExpiredLocked(now)

	// 新一轮 user 请求到达前，把同资产下仍未 finalize 的旧 log 的耗时兜底写上：
	// 模型可能在多轮工具调用中被新提问打断，导致 FinalizeRequestOutput 永远不会触发，
	// 旧 log 的 Duration 会一直停在 0ms。此处只补时间字段，不动决策/输出语义。
	t.backfillStaleDurationsLocked(strings.TrimSpace(assetID), now)

	logID := generateAuditLogID()
	state := &auditLogState{
		Log: AuditLog{
			ID:             logID,
			Timestamp:      now.Format(time.RFC3339Nano),
			RequestID:      logID,
			AssetName:      strings.TrimSpace(assetName),
			AssetID:        strings.TrimSpace(assetID),
			Model:          strings.TrimSpace(model),
			RequestContent: truncateToBytes(instruction, maxRecordMessageBytes),
			Action:         "ALLOW",
		},
		CreatedAt: now,
		UpdatedAt: now,
		StartedAt: now,
		ToolSeq:   make([]string, 0),
		ToolIndex: make(map[string]int),
	}

	t.logs[logID] = state
	t.order = append(t.order, logID)
	t.setRequestBindingLocked(requestID, logID, now)
	t.touchStateLocked(state, now)
	t.evictOldestLocked()
	logging.Info(
		"[AuditChain] start created: request_id=%s log_id=%s asset_id=%s model=%s instruction_len=%d",
		strings.TrimSpace(requestID),
		logID,
		strings.TrimSpace(assetID),
		strings.TrimSpace(model),
		len(strings.TrimSpace(instruction)),
	)
}

// LinkRequestByToolResults binds the current request to an existing log by tool_call_id mappings.
func (t *AuditChainTracker) LinkRequestByToolResults(requestID, assetID string, toolResults map[string]string) {
	if t == nil || len(toolResults) == 0 {
		return
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return
	}
	assetID = strings.TrimSpace(assetID)

	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.requestToLog[requestID]; exists {
		logging.Info(
			"[AuditChain] link by tool_result skipped: request_id=%s reason=already_bound tool_call_ids=%s",
			requestID,
			formatAuditToolResultMapSummary(toolResults, 12),
		)
		return
	}
	t.cleanupExpiredLocked(now)
	logging.Info(
		"[AuditChain] link by tool_result begin: request_id=%s asset_id=%s tool_call_ids=%s",
		requestID,
		assetID,
		formatAuditToolResultMapSummary(toolResults, 12),
	)

	var latest *auditLogState
	unresolvedIDs := make(map[string]struct{})
	for toolCallID := range toolResults {
		toolCallID = normalizeAuditToolCallIDForMatch(toolCallID)
		if toolCallID == "" {
			continue
		}
		state := t.getStateByToolCallLocked(assetID, toolCallID)
		if state == nil {
			unresolvedIDs[toolCallID] = struct{}{}
			continue
		}
		if latest == nil || state.UpdatedAt.After(latest.UpdatedAt) {
			latest = state
		}
	}
	if latest != nil {
		t.setRequestBindingLocked(requestID, latest.Log.ID, now)
		logging.Info(
			"[AuditChain] link by tool_result matched: request_id=%s log_id=%s",
			requestID,
			latest.Log.ID,
		)
	} else {
		logging.Info(
			"[AuditChain] link by tool_result no direct match: request_id=%s unresolved_ids=%s",
			requestID,
			formatAuditToolIDSetSummary(unresolvedIDs, 12),
		)
	}
	if len(unresolvedIDs) > 0 {
		t.storePendingRequestLinkIDsLocked(requestID, assetID, unresolvedIDs, now)
	} else {
		delete(t.pendingRequestLinks, requestID)
	}
}

func (t *AuditChainTracker) ensureToolCallLocked(state *auditLogState, toolCallID string) (int, bool) {
	matchID := normalizeAuditToolCallIDForMatch(toolCallID)
	if state == nil || matchID == "" {
		return -1, false
	}
	if idx, ok := state.ToolIndex[matchID]; ok {
		return idx, false
	}
	state.ToolSeq = append(state.ToolSeq, matchID)
	idx := len(state.ToolSeq) - 1
	state.ToolIndex[matchID] = idx
	state.Log.ToolCalls = append(state.Log.ToolCalls, AuditToolCall{})
	return idx, true
}

func (t *AuditChainTracker) updateToolCallLocked(
	state *auditLogState,
	assetID,
	toolCallID,
	toolName,
	args string,
	isSensitive bool,
	now time.Time,
) bool {
	if state == nil {
		return false
	}
	idx, created := t.ensureToolCallLocked(state, toolCallID)
	if idx < 0 || idx >= len(state.Log.ToolCalls) {
		return false
	}
	changed := created

	if toolName != "" {
		if state.Log.ToolCalls[idx].Name != toolName {
			state.Log.ToolCalls[idx].Name = toolName
			changed = true
		}
	}
	if args != "" {
		trimmedArgs := truncateToBytes(args, maxRecordToolArgsBytes)
		if state.Log.ToolCalls[idx].Arguments != trimmedArgs {
			state.Log.ToolCalls[idx].Arguments = trimmedArgs
			changed = true
		}
	}
	if isSensitive {
		if !state.Log.ToolCalls[idx].IsSensitive {
			state.Log.ToolCalls[idx].IsSensitive = true
			changed = true
		}
	}
	if pendingResult, ok := t.popPendingToolResultLocked(assetID, toolCallID); ok {
		if state.Log.ToolCalls[idx].Result != pendingResult {
			state.Log.ToolCalls[idx].Result = pendingResult
			changed = true
		}
	}
	t.setToolBindingLocked(assetID, toolCallID, state.Log.ID, now)
	return changed
}

// RecordToolCallsForRequest links response tool calls to the request's audit log.
func (t *AuditChainTracker) RecordToolCallsForRequest(
	requestID,
	assetID string,
	toolCalls []openai.ChatCompletionMessageToolCall,
	validator *ToolValidator,
) {
	if t == nil || len(toolCalls) == 0 {
		return
	}
	requestID = strings.TrimSpace(requestID)
	assetID = strings.TrimSpace(assetID)

	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cleanupExpiredLocked(now)
	toolIDs := make([]string, 0, len(toolCalls))
	for _, tc := range toolCalls {
		toolIDs = append(toolIDs, strings.TrimSpace(tc.ID))
	}
	logging.Info(
		"[AuditChain] record tool_calls begin: request_id=%s asset_id=%s tool_call_ids=%s",
		requestID,
		assetID,
		formatAuditToolIDSummary(toolIDs, 12),
	)

	state := t.getStateByRequestLocked(requestID)
	if state == nil {
		logging.Warning(
			"[AuditChain] record tool_calls skipped: request_id=%s reason=request_not_bound tool_call_ids=%s",
			requestID,
			formatAuditToolIDSummary(toolIDs, 12),
		)
		return
	}

	changed := false
	for _, tc := range toolCalls {
		toolCallID := strings.TrimSpace(tc.ID)
		if toolCallID == "" {
			continue
		}
		isSensitive := false
		if validator != nil {
			isSensitive = validator.IsSensitive(tc.Function.Name)
		}
		if t.updateToolCallLocked(
			state,
			assetID,
			toolCallID,
			strings.TrimSpace(tc.Function.Name),
			strings.TrimSpace(tc.Function.Arguments),
			isSensitive,
			now,
		) {
			changed = true
		}
		logging.Info(
			"[AuditChain] record tool_call item: request_id=%s log_id=%s tool_call_id=%s tool_name=%s",
			requestID,
			state.Log.ID,
			toolCallID,
			strings.TrimSpace(tc.Function.Name),
		)
	}
	if changed {
		t.touchStateLocked(state, now)
	}
}

// RecordToolResults links tool results from request messages by tool_call_id.
func (t *AuditChainTracker) RecordToolResults(assetID string, toolResults map[string]string) {
	if t == nil || len(toolResults) == 0 {
		return
	}
	assetID = strings.TrimSpace(assetID)

	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cleanupExpiredLocked(now)
	logging.Info(
		"[AuditChain] record tool_results begin: asset_id=%s tool_call_ids=%s",
		assetID,
		formatAuditToolResultMapSummary(toolResults, 12),
	)

	updated := make(map[string]*auditLogState)
	for toolCallID, rawResult := range toolResults {
		toolCallID = normalizeAuditToolCallIDForMatch(toolCallID)
		if toolCallID == "" {
			logging.Info(
				"[AuditChain] record tool_result skipped: asset_id=%s reason=empty_tool_call_id result_len=%d",
				assetID,
				len(strings.TrimSpace(rawResult)),
			)
			continue
		}
		state := t.getStateByToolCallLocked(assetID, toolCallID)
		if state == nil {
			t.storePendingToolResultLocked(assetID, toolCallID, rawResult, now)
			logging.Info(
				"[AuditChain] record tool_result pending: asset_id=%s tool_call_id=%s result_len=%d",
				assetID,
				toolCallID,
				len(strings.TrimSpace(rawResult)),
			)
			continue
		}
		idx, _ := t.ensureToolCallLocked(state, toolCallID)
		if idx < 0 || idx >= len(state.Log.ToolCalls) {
			logging.Info(
				"[AuditChain] record tool_result skipped: asset_id=%s tool_call_id=%s reason=index_out_of_range idx=%d tool_calls=%d",
				assetID,
				toolCallID,
				idx,
				len(state.Log.ToolCalls),
			)
			continue
		}
		newResult := truncateToBytes(strings.TrimSpace(rawResult), maxRecordMessageBytes)
		if state.Log.ToolCalls[idx].Result != newResult {
			state.Log.ToolCalls[idx].Result = newResult
			updated[state.Log.ID] = state
		}
		logging.Info(
			"[AuditChain] record tool_result matched: asset_id=%s tool_call_id=%s log_id=%s result_len=%d",
			assetID,
			toolCallID,
			state.Log.ID,
			len(newResult),
		)
	}

	for _, state := range updated {
		t.touchStateLocked(state, now)
	}
}

// UpdateRequestTokens updates token counters for the log bound to requestID.
func (t *AuditChainTracker) UpdateRequestTokens(requestID string, promptTokens, completionTokens, totalTokens int) {
	if t == nil {
		return
	}
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cleanupExpiredLocked(now)

	state := t.getStateByRequestLocked(requestID)
	if state == nil {
		return
	}
	state.Log.PromptTokens = promptTokens
	state.Log.CompletionTokens = completionTokens
	state.Log.TotalTokens = totalTokens
	t.touchStateLocked(state, now)
}

// SetRequestDecision updates risk/action fields for the request-bound log.
func (t *AuditChainTracker) SetRequestDecision(requestID, action, riskLevel, riskReason string, confidence int) {
	if t == nil {
		return
	}
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cleanupExpiredLocked(now)

	state := t.getStateByRequestLocked(requestID)
	if state == nil {
		return
	}
	action = strings.TrimSpace(action)
	if action == "" {
		action = state.Log.Action
	}
	state.Log.Action = action
	state.Log.RiskLevel = strings.TrimSpace(riskLevel)
	state.Log.RiskReason = strings.TrimSpace(riskReason)
	if confidence > 0 {
		state.Log.Confidence = confidence
	}
	state.Log.HasRisk = strings.TrimSpace(state.Log.RiskLevel) != "" && !strings.EqualFold(state.Log.RiskLevel, "SAFE")
	if strings.EqualFold(state.Log.Action, "WARN") || strings.EqualFold(state.Log.Action, "BLOCK") || strings.EqualFold(state.Log.Action, "HARD_BLOCK") {
		state.Log.HasRisk = true
	}
	t.touchStateLocked(state, now)
}

// SetRequestInstructionChainID records the ShepherdGate runtime chain for the request-bound log.
func (t *AuditChainTracker) SetRequestInstructionChainID(requestID, instructionChainID string) {
	if t == nil {
		return
	}
	instructionChainID = strings.TrimSpace(instructionChainID)
	if instructionChainID == "" {
		return
	}
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cleanupExpiredLocked(now)

	state := t.getStateByRequestLocked(requestID)
	if state == nil {
		return
	}
	state.Log.InstructionChainID = instructionChainID
	t.touchStateLocked(state, now)
}

// FinalizeRequestOutput updates final response for the request-bound log.
func (t *AuditChainTracker) FinalizeRequestOutput(requestID, output string) {
	if t == nil {
		return
	}
	requestID = strings.TrimSpace(requestID)
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cleanupExpiredLocked(now)

	state := t.getStateByRequestLocked(requestID)
	if state == nil {
		if _, pending := t.pendingRequestLinks[requestID]; !pending {
			logging.Info(
				"[AuditChain] finalize assistant output skipped: request_id=%s reason=request_not_bound_and_no_pending_link",
				requestID,
			)
			return
		}
		t.storePendingFinalOutputLocked(requestID, output, now)
		logging.Info(
			"[AuditChain] finalize assistant output pending: request_id=%s output_len=%d",
			requestID,
			len(strings.TrimSpace(output)),
		)
		return
	}
	state.Log.OutputContent = truncateToBytes(strings.TrimSpace(output), maxRecordOutputBytes)
	state.Log.Duration = now.Sub(state.StartedAt).Milliseconds()
	logging.Info(
		"[AuditChain] finalize assistant output matched: request_id=%s log_id=%s output_len=%d",
		requestID,
		state.Log.ID,
		len(strings.TrimSpace(output)),
	)
	t.touchStateLocked(state, now)
	t.releaseCompletedChainResourcesLocked(state, requestID)
}

// GetAuditLogs returns newest-first snapshots with optional risk filter.
func (t *AuditChainTracker) GetAuditLogs(limit, offset int, riskOnly bool) []AuditLog {
	if t == nil {
		return nil
	}
	if limit <= 0 {
		limit = 100
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.cleanupExpiredLocked(now)

	items := make([]*auditLogState, 0, len(t.logs))
	for _, state := range t.logs {
		if riskOnly && !state.Log.HasRisk {
			continue
		}
		items = append(items, state)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].Log.ID > items[j].Log.ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	if offset >= len(items) {
		return []AuditLog{}
	}
	items = items[offset:]
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}

	out := make([]AuditLog, 0, len(items))
	for _, state := range items {
		entry := state.Log
		if len(state.Log.ToolCalls) > 0 {
			entry.ToolCalls = make([]AuditToolCall, len(state.Log.ToolCalls))
			copy(entry.ToolCalls, state.Log.ToolCalls)
		}
		out = append(out, entry)
	}
	return out
}

// getAuditLogsSnapshot 在单次加锁内返回完整审计快照与总数，确保分页读取使用同一视图。
func (t *AuditChainTracker) getAuditLogsSnapshot(riskOnly bool) ([]AuditLog, int) {
	if t == nil {
		return nil, 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.cleanupExpiredLocked(now)

	items := make([]*auditLogState, 0, len(t.logs))
	for _, state := range t.logs {
		if riskOnly && !state.Log.HasRisk {
			continue
		}
		items = append(items, state)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].Log.ID > items[j].Log.ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	out := make([]AuditLog, 0, len(items))
	for _, state := range items {
		entry := state.Log
		if len(state.Log.ToolCalls) > 0 {
			entry.ToolCalls = make([]AuditToolCall, len(state.Log.ToolCalls))
			copy(entry.ToolCalls, state.Log.ToolCalls)
		}
		out = append(out, entry)
	}
	return out, len(items)
}

// GetAuditLogCount returns count with optional risk filter.
func (t *AuditChainTracker) GetAuditLogCount(riskOnly bool) int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.cleanupExpiredLocked(now)
	if !riskOnly {
		return len(t.logs)
	}
	count := 0
	for _, state := range t.logs {
		if state.Log.HasRisk {
			count++
		}
	}
	return count
}

// GetAndClearPending returns incremental snapshots and clears pending queue.
func (t *AuditChainTracker) GetAndClearPending() []AuditLog {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.pending) == 0 {
		return nil
	}
	out := make([]AuditLog, len(t.pending))
	copy(out, t.pending)
	t.pending = t.pending[:0]
	return out
}

// ClearAll clears all tracker state.
func (t *AuditChainTracker) ClearAll() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.logs = make(map[string]*auditLogState)
	t.order = t.order[:0]
	t.requestToLog = make(map[string]auditLogKeyBinding)
	t.toolCallToLog = make(map[string]auditLogKeyBinding)
	t.pendingToolResults = make(map[string]pendingToolResult)
	t.pendingRequestLinks = make(map[string]pendingRequestLink)
	t.pendingFinalOutputs = make(map[string]pendingFinalOutput)
	t.pending = t.pending[:0]
}

// ClearWithFilter clears by asset_id only (instance isolation).
func (t *AuditChainTracker) ClearWithFilter(assetID string) {
	if t == nil {
		return
	}
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for logID, state := range t.logs {
		if state.Log.AssetID != assetID {
			continue
		}
		delete(t.logs, logID)
	}

	filteredOrder := make([]string, 0, len(t.order))
	for _, logID := range t.order {
		if state, ok := t.logs[logID]; ok && state != nil {
			filteredOrder = append(filteredOrder, logID)
		}
	}
	t.order = filteredOrder

	for requestID, binding := range t.requestToLog {
		if _, ok := t.logs[binding.LogID]; !ok {
			delete(t.requestToLog, requestID)
		}
	}
	for toolKey, binding := range t.toolCallToLog {
		if _, ok := t.logs[binding.LogID]; !ok {
			delete(t.toolCallToLog, toolKey)
		}
	}
	for toolKey := range t.pendingToolResults {
		parts := strings.SplitN(toolKey, "|", 2)
		if len(parts) > 0 && parts[0] == assetID {
			delete(t.pendingToolResults, toolKey)
		}
	}
	for requestID, link := range t.pendingRequestLinks {
		if link.AssetID == assetID {
			delete(t.pendingRequestLinks, requestID)
		}
	}

	filteredPending := make([]AuditLog, 0, len(t.pending))
	for _, log := range t.pending {
		if log.AssetID == assetID {
			continue
		}
		filteredPending = append(filteredPending, log)
	}
	t.pending = filteredPending
}

func getAllActiveAuditTrackers() []*AuditChainTracker {
	trackers := make([]*AuditChainTracker, 0, len(proxyByAssetKey)+1)
	seen := make(map[*AuditChainTracker]bool)
	for _, pp := range proxyByAssetKey {
		if pp == nil || pp.auditTracker == nil || seen[pp.auditTracker] {
			continue
		}
		trackers = append(trackers, pp.auditTracker)
		seen[pp.auditTracker] = true
	}
	if proxyInstance != nil && proxyInstance.auditTracker != nil && !seen[proxyInstance.auditTracker] {
		trackers = append(trackers, proxyInstance.auditTracker)
	}
	return trackers
}

// GetAuditLogsInternal retrieves audit logs from all active proxy trackers.
func GetAuditLogsInternal(limit, offset int, riskOnly bool) string {
	proxyInstanceMu.Lock()
	trackers := getAllActiveAuditTrackers()
	proxyInstanceMu.Unlock()

	all := make([]AuditLog, 0)
	total := 0
	for _, tracker := range trackers {
		trackerLogs, trackerTotal := tracker.getAuditLogsSnapshot(riskOnly)
		total += trackerTotal
		if trackerTotal <= 0 {
			continue
		}
		all = append(all, trackerLogs...)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp > all[j].Timestamp
	})

	if offset >= len(all) {
		all = nil
	} else if offset > 0 {
		all = all[offset:]
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}

	result := map[string]interface{}{
		"logs":  all,
		"total": total,
	}
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return `{"logs":[],"total":0,"error":"` + err.Error() + `"}`
	}
	return string(jsonBytes)
}

// GetPendingAuditLogsInternal returns and clears pending snapshots.
func GetPendingAuditLogsInternal() string {
	proxyInstanceMu.Lock()
	trackers := getAllActiveAuditTrackers()
	proxyInstanceMu.Unlock()

	pending := make([]AuditLog, 0)
	for _, tracker := range trackers {
		pending = append(pending, tracker.GetAndClearPending()...)
	}

	jsonBytes, err := json.Marshal(pending)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

// ClearAuditLogsInternal clears all in-memory audit trackers.
func ClearAuditLogsInternal() string {
	proxyInstanceMu.Lock()
	trackers := getAllActiveAuditTrackers()
	proxyInstanceMu.Unlock()
	for _, tracker := range trackers {
		tracker.ClearAll()
	}
	return `{"success":true}`
}

// ClearAuditLogsWithFilterInternal clears by asset_id only.
func ClearAuditLogsWithFilterInternal(filterJSON string) string {
	var input struct {
		AssetName string `json:"asset_name,omitempty"`
		AssetID   string `json:"asset_id,omitempty"`
	}
	if strings.TrimSpace(filterJSON) != "" {
		if err := json.Unmarshal([]byte(filterJSON), &input); err != nil {
			return `{"success":false,"error":"invalid JSON"}`
		}
	}
	if strings.TrimSpace(input.AssetID) == "" {
		return `{"success":true}`
	}

	proxyInstanceMu.Lock()
	trackers := getAllActiveAuditTrackers()
	proxyInstanceMu.Unlock()
	for _, tracker := range trackers {
		tracker.ClearWithFilter(input.AssetID)
	}
	return `{"success":true}`
}

func (pp *ProxyProtection) auditLogSafe(operation string, fn func(tracker *AuditChainTracker)) {
	if pp == nil || pp.auditTracker == nil || fn == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			logging.Warning("[AuditLog] operation=%s recovered panic: %v", operation, r)
		}
	}()
	fn(pp.auditTracker)
}
