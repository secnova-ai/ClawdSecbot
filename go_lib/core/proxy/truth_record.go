package proxy

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	completedTruthRecordCallback   func(TruthRecord)
	completedTruthRecordCallbackMu sync.Mutex
)

// TruthRecord 阶段常量
const (
	RecordPhaseStarting  = "starting"
	RecordPhaseCompleted = "completed"
	RecordPhaseStopped   = "stopped"
)

// 审计内容字节上限（按 UTF-8 字节计）
const (
	maxRecordMessageBytes  = 256 * 1024 // 单条 RecordMessage.Content 上限 256KB
	maxRecordToolArgsBytes = 256 * 1024 // RecordToolCall.Arguments 上限 256KB
	maxRecordOutputBytes   = 512 * 1024 // TruthRecord.OutputContent 上限 512KB
)

// TruthRecord 主内容类型常量
const (
	RecordContentAssistant  = "assistant_response"
	RecordContentToolResult = "tool_result_summary"
	RecordContentSecurity   = "security_warning"
	RecordContentNoText     = "no_text_response"
	RecordContentUnknown    = "unavailable"
)

// TruthRecord 是代理防护层的唯一核心数据实体（SSOT）。
// 每个代理请求有且仅有一条 TruthRecord，从请求到达到响应完成渐进式更新。
// 它同时服务于：清晰视图卡片展示、审计日志记录、安全事件过滤。
type TruthRecord struct {
	// 身份与归属
	RequestID string `json:"request_id"`
	AssetName string `json:"asset_name,omitempty"`
	AssetID   string `json:"asset_id,omitempty"`

	// 时间线 — CompletedAt 非空即表示已完成
	StartedAt   string `json:"started_at"`
	UpdatedAt   string `json:"updated_at"`
	CompletedAt string `json:"completed_at,omitempty"`

	// 请求上下文 — MessageCount 是原始请求消息总数，Messages 仅含当前轮
	Model        string          `json:"model,omitempty"`
	MessageCount int             `json:"message_count"`
	Messages     []RecordMessage `json:"messages,omitempty"`

	// 响应 — PrimaryContent 为卡片展示截取，OutputContent 为审计全文
	Phase              string `json:"phase"`
	FinishReason       string `json:"finish_reason,omitempty"`
	PrimaryContent     string `json:"primary_content,omitempty"`
	PrimaryContentType string `json:"primary_content_type"`
	OutputContent      string `json:"output_content,omitempty"`

	// 工具链路 — 所有派生属性（count/names/argsPreview）由前端 getter 计算
	ToolCalls []RecordToolCall `json:"tool_calls,omitempty"`

	// 安全决策 — nil 表示未经安全检测
	Decision *SecurityDecision `json:"decision,omitempty"`

	// Token 指标 — TotalTokens 由前端计算
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// SecurityDecision 安全决策子结构，将原 RequestView 和 AuditLog 中 9 个散落字段收敛为一体。
type SecurityDecision struct {
	Action     string `json:"action"`              // ALLOW | WARN | BLOCK | HARD_BLOCK
	RiskLevel  string `json:"risk_level,omitempty"` // SAFE | SUSPICIOUS | DANGEROUS | CRITICAL
	Reason     string `json:"reason,omitempty"`
	Confidence int    `json:"confidence,omitempty"` // 0-100
}

// RecordToolCall 统一工具调用记录，同时服务卡片展示和审计详情。
type RecordToolCall struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Arguments   string `json:"arguments,omitempty"`
	Result      string `json:"result,omitempty"`
	IsSensitive bool   `json:"is_sensitive,omitempty"`
	Source      string `json:"source"`       // "history" (bot 请求中的工具结果) | "response" (provider 返回的工具调用)
	LatestRound bool   `json:"latest_round"` // true 表示来自最新一轮工具交互，用于前端过滤避免重复展示
}

// RecordMessage 消息条目（仅当前轮）。
type RecordMessage struct {
	Index   int    `json:"index"`
	Role    string `json:"role"`
	Content string `json:"content"`
}

// recordContentPreview 用于 applyRecordPrimaryContent 的内部中间结构。
type recordContentPreview struct {
	Content  string
	FullText string
}

// ==================== RecordStore ====================

// RecordStore 管理所有 TruthRecord 的生命周期，替代原 RequestViewStore + AuditLogBuffer。
type RecordStore struct {
	mu        sync.Mutex
	records   map[string]*TruthRecord
	pending   []TruthRecord
	completed []TruthRecord
	maxLen    int
}

// NewRecordStore 创建 RecordStore 实例。
func NewRecordStore() *RecordStore {
	return &RecordStore{
		records: make(map[string]*TruthRecord),
		maxLen:  1000,
	}
}

// Upsert 创建或更新一条 TruthRecord，返回更新后的快照。
// 每次调用都会将快照追加到 pending 队列供 CallbackBridge 推送。
func (s *RecordStore) Upsert(requestID string, update func(r *TruthRecord)) *TruthRecord {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil
	}

	s.mu.Lock()
	r := s.records[requestID]
	wasComplete := false
	if r == nil {
		now := time.Now().Format(time.RFC3339Nano)
		r = &TruthRecord{
			RequestID:          requestID,
			StartedAt:          now,
			UpdatedAt:          now,
			PrimaryContentType: RecordContentUnknown,
			Phase:              RecordPhaseStarting,
		}
		s.records[requestID] = r
	} else {
		wasComplete = isRecordComplete(r)
	}

	update(r)
	r.UpdatedAt = time.Now().Format(time.RFC3339Nano)
	normalizeTruthRecord(r)
	snapshot := cloneTruthRecord(r)
	s.pending = append(s.pending, snapshot)
	becameComplete := !wasComplete && isRecordComplete(r)

	if isRecordComplete(r) {
		s.completed = append(s.completed, snapshot)
		if len(s.completed) > s.maxLen {
			s.completed = s.completed[len(s.completed)-s.maxLen:]
		}
	}
	s.mu.Unlock()

	if becameComplete {
		completedTruthRecordCallbackMu.Lock()
		cb := completedTruthRecordCallback
		completedTruthRecordCallbackMu.Unlock()
		if cb != nil {
			cb(snapshot)
		}
	}
	return &snapshot
}

// SetCompletedTruthRecordCallback sets a callback that is triggered once when a
// TruthRecord first transitions into a completed/stopped state.
func SetCompletedTruthRecordCallback(cb func(TruthRecord)) {
	completedTruthRecordCallbackMu.Lock()
	defer completedTruthRecordCallbackMu.Unlock()
	completedTruthRecordCallback = cb
}

// Pending 返回并清空待推送的快照队列（用于 CallbackBridge 实时推送 + 轮询回退）。
func (s *RecordStore) Pending() []TruthRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) == 0 {
		return nil
	}
	out := make([]TruthRecord, len(s.pending))
	copy(out, s.pending)
	s.pending = nil
	return out
}

// CompletedPending 返回并清空已完成记录队列（替代 GetAndClearAuditLogs，用于 Flutter 持久化）。
func (s *RecordStore) CompletedPending() []TruthRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.completed) == 0 {
		return nil
	}
	out := make([]TruthRecord, len(s.completed))
	copy(out, s.completed)
	s.completed = nil
	return out
}

// GetCompletedRecords 返回已完成记录缓冲（不清空），用于审计日志查询兼容。
func (s *RecordStore) GetCompletedRecords(limit, offset int, riskOnly bool) []TruthRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	var filtered []TruthRecord
	for _, r := range s.records {
		if !isRecordComplete(r) {
			continue
		}
		if riskOnly && !isRecordRisky(r) {
			continue
		}
		filtered = append(filtered, cloneTruthRecord(r))
	}

	// 按 UpdatedAt 倒序（newest first）
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}

	if offset >= len(filtered) {
		return nil
	}
	filtered = filtered[offset:]
	if limit > 0 && limit < len(filtered) {
		filtered = filtered[:limit]
	}
	return filtered
}

// GetCompletedCount 返回已完成记录数量。
func (s *RecordStore) GetCompletedCount(riskOnly bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, r := range s.records {
		if !isRecordComplete(r) {
			continue
		}
		if riskOnly && !isRecordRisky(r) {
			continue
		}
		count++
	}
	return count
}

// GetAll 返回所有当前记录的快照（非破坏性读取，不清空任何队列）。
// 用于监视器窗口的 catch-up 补漏：当回调桥偶发丢失快照时，通过此方法同步最新状态。
func (s *RecordStore) GetAll() []TruthRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TruthRecord, 0, len(s.records))
	for _, r := range s.records {
		out = append(out, cloneTruthRecord(r))
	}
	return out
}

// ClearAll 清空所有记录。
func (s *RecordStore) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = make(map[string]*TruthRecord)
	s.pending = nil
	s.completed = nil
}

// ClearWithFilter 按 asset 过滤清除已完成记录。
func (s *RecordStore) ClearWithFilter(assetName, assetID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if assetID == "" && assetName == "" {
		s.records = make(map[string]*TruthRecord)
		s.completed = nil
		return
	}

	for id, r := range s.records {
		matches := false
		if assetID != "" {
			matches = r.AssetID == assetID
		} else {
			matches = r.AssetName == assetName
		}
		if matches {
			delete(s.records, id)
		}
	}
}

// ==================== 辅助函数 ====================

func isRecordComplete(r *TruthRecord) bool {
	return r.Phase == RecordPhaseCompleted || r.Phase == RecordPhaseStopped
}

func isRecordRisky(r *TruthRecord) bool {
	if r.Decision == nil {
		return false
	}
	rl := strings.TrimSpace(r.Decision.RiskLevel)
	return rl != "" && rl != "SAFE"
}

// normalizeTruthRecord 清理和规范化字段值。
func normalizeTruthRecord(r *TruthRecord) {
	r.AssetName = strings.TrimSpace(r.AssetName)
	r.AssetID = strings.TrimSpace(r.AssetID)
	r.Model = strings.TrimSpace(r.Model)
	r.PrimaryContent = strings.TrimSpace(r.PrimaryContent)
	r.PrimaryContentType = normalizeRecordContentType(r.PrimaryContentType)
	r.FinishReason = strings.TrimSpace(r.FinishReason)
	r.Phase = advanceRecordPhase(r.Phase, r.Phase)
}

// cloneTruthRecord 创建 TruthRecord 的深拷贝。
func cloneTruthRecord(r *TruthRecord) TruthRecord {
	if r == nil {
		return TruthRecord{}
	}
	snapshot := *r
	if len(r.Messages) > 0 {
		snapshot.Messages = make([]RecordMessage, len(r.Messages))
		copy(snapshot.Messages, r.Messages)
	}
	if len(r.ToolCalls) > 0 {
		snapshot.ToolCalls = make([]RecordToolCall, len(r.ToolCalls))
		copy(snapshot.ToolCalls, r.ToolCalls)
	}
	if r.Decision != nil {
		d := *r.Decision
		snapshot.Decision = &d
	}
	return snapshot
}

// truthRecordToMap 将 TruthRecord 序列化为 map，用于 CallbackBridge 和 logChan。
func truthRecordToMap(r *TruthRecord) map[string]interface{} {
	if r == nil {
		return map[string]interface{}{}
	}
	b, err := json.Marshal(r)
	if err != nil {
		return map[string]interface{}{}
	}
	payload := make(map[string]interface{})
	if err := json.Unmarshal(b, &payload); err != nil {
		return map[string]interface{}{}
	}
	return payload
}

// previewRecordContent 截取内容预览。
func previewRecordContent(raw string, limit int) recordContentPreview {
	content := strings.TrimSpace(raw)
	if content == "" {
		return recordContentPreview{}
	}
	fullText := content
	if limit > 0 && len(content) > limit {
		content = content[:limit] + "...(truncated)"
	}
	return recordContentPreview{
		Content:  content,
		FullText: fullText,
	}
}

// applyRecordPrimaryContent 设置主内容，遵循优先级规则。
// force=true 时强制覆盖（用于安全阻断等场景）。
func applyRecordPrimaryContent(r *TruthRecord, contentType string, content string, force bool) {
	if r == nil {
		return
	}
	contentType = normalizeRecordContentType(contentType)
	if !force && recordContentPriority(contentType) < recordContentPriority(r.PrimaryContentType) {
		return
	}
	r.PrimaryContentType = contentType
	r.PrimaryContent = strings.TrimSpace(content)
}

// recordContentPriority 返回内容类型的优先级，数值越大优先级越高。
func recordContentPriority(contentType string) int {
	switch normalizeRecordContentType(contentType) {
	case RecordContentSecurity:
		return 4
	case RecordContentAssistant:
		return 3
	case RecordContentToolResult:
		return 2
	case RecordContentNoText:
		return 1
	default:
		return 0
	}
}

func normalizeRecordContentType(value string) string {
	switch strings.TrimSpace(value) {
	case RecordContentAssistant,
		RecordContentToolResult,
		RecordContentSecurity,
		RecordContentNoText:
		return strings.TrimSpace(value)
	default:
		return RecordContentUnknown
	}
}

// truncateToBytes truncates s to at most maxBytes UTF-8 bytes, cutting at a rune boundary.
func truncateToBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	if maxBytes <= 0 {
		return "...(truncated)"
	}
	i := maxBytes
	for i > 0 && (s[i]>>6) == 2 {
		i--
	}
	if i == 0 {
		return "...(truncated)"
	}
	return s[:i] + "...(truncated)"
}

func advanceRecordPhase(current, next string) string {
	order := map[string]int{
		RecordPhaseStarting:  0,
		RecordPhaseCompleted: 1,
		RecordPhaseStopped:   2,
	}
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if next == "" {
		next = RecordPhaseStarting
	}
	if current == "" {
		return next
	}
	if order[next] < order[current] {
		return current
	}
	return next
}

// ==================== FFI 兼容层（替代原 audit_log.go 的 FFI 函数） ====================

// getAllActiveRecordStores 返回所有活跃 proxy handler 的 RecordStore（含去重的 legacy 实例）。
// 调用方必须已持有 proxyInstanceMu 或在安全上下文中调用。
func getAllActiveRecordStores() []*RecordStore {
	var stores []*RecordStore
	seen := make(map[*RecordStore]bool)

	for _, pp := range proxyByAssetKey {
		if pp != nil && pp.records != nil && !seen[pp.records] {
			stores = append(stores, pp.records)
			seen[pp.records] = true
		}
	}
	if proxyInstance != nil && proxyInstance.records != nil && !seen[proxyInstance.records] {
		stores = append(stores, proxyInstance.records)
	}
	return stores
}

// GetAllTruthRecordSnapshotsInternal 非破坏性地从所有活跃 proxy 实例获取 TruthRecord 最新快照。
// 返回原生 TruthRecord JSON 数组（非审计兼容格式），供监视器窗口 catch-up 使用。
func GetAllTruthRecordSnapshotsInternal() string {
	proxyInstanceMu.Lock()
	stores := getAllActiveRecordStores()
	proxyInstanceMu.Unlock()

	var all []TruthRecord
	for _, store := range stores {
		all = append(all, store.GetAll()...)
	}
	jsonBytes, err := json.Marshal(all)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

// GetTruthRecordsInternal 从所有活跃 proxy 实例获取已完成记录（兼容原 GetAuditLogsInternal）。
func GetTruthRecordsInternal(limit, offset int, riskOnly bool) string {
	proxyInstanceMu.Lock()
	stores := getAllActiveRecordStores()
	proxyInstanceMu.Unlock()

	var allRecords []TruthRecord
	totalCount := 0
	for _, store := range stores {
		allRecords = append(allRecords, store.GetCompletedRecords(0, 0, riskOnly)...)
		totalCount += store.GetCompletedCount(riskOnly)
	}

	// Sort by started_at descending, then apply limit/offset
	sort.Slice(allRecords, func(i, j int) bool {
		return allRecords[i].StartedAt > allRecords[j].StartedAt
	})
	if offset > 0 && offset < len(allRecords) {
		allRecords = allRecords[offset:]
	} else if offset >= len(allRecords) {
		allRecords = nil
	}
	if limit > 0 && limit < len(allRecords) {
		allRecords = allRecords[:limit]
	}

	result := map[string]interface{}{
		"logs":  truthRecordsToAuditCompat(allRecords),
		"total": totalCount,
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return `{"logs":[],"total":0,"error":"` + err.Error() + `"}`
	}
	return string(jsonBytes)
}

// GetPendingTruthRecordsInternal 获取并清空所有活跃 proxy 实例的已完成记录
// （兼容原 GetPendingAuditLogsInternal）。
func GetPendingTruthRecordsInternal() string {
	proxyInstanceMu.Lock()
	stores := getAllActiveRecordStores()
	proxyInstanceMu.Unlock()

	var allCompleted []TruthRecord
	for _, store := range stores {
		allCompleted = append(allCompleted, store.CompletedPending()...)
	}
	compat := truthRecordsToAuditCompat(allCompleted)

	jsonBytes, err := json.Marshal(compat)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

// ClearTruthRecordsInternal 清空所有活跃 proxy 实例的记录（兼容原 ClearAuditLogsInternal）。
func ClearTruthRecordsInternal() string {
	proxyInstanceMu.Lock()
	stores := getAllActiveRecordStores()
	proxyInstanceMu.Unlock()

	for _, store := range stores {
		store.ClearAll()
	}
	return `{"success":true}`
}

// ClearTruthRecordsWithFilterInternal 按 asset 过滤清除记录。
func ClearTruthRecordsWithFilterInternal(filterJSON string) string {
	var input struct {
		AssetName string `json:"asset_name,omitempty"`
		AssetID   string `json:"asset_id,omitempty"`
	}
	if filterJSON != "" {
		if err := json.Unmarshal([]byte(filterJSON), &input); err != nil {
			return `{"success":false,"error":"invalid JSON"}`
		}
	}

	proxyInstanceMu.Lock()
	stores := getAllActiveRecordStores()
	proxyInstanceMu.Unlock()

	for _, store := range stores {
		store.ClearWithFilter(input.AssetName, input.AssetID)
	}
	return `{"success":true}`
}

// truthRecordsToAuditCompat 将 TruthRecord 列表转换为兼容旧 AuditLog JSON 格式，
// 确保 Flutter 审计日志页在过渡期间无需立即改造。
func truthRecordsToAuditCompat(records []TruthRecord) []map[string]interface{} {
	if len(records) == 0 {
		return []map[string]interface{}{}
	}
	out := make([]map[string]interface{}, 0, len(records))
	for _, r := range records {
		durationMs := int64(0)
		if r.CompletedAt != "" && r.StartedAt != "" {
			if start, err := time.Parse(time.RFC3339Nano, r.StartedAt); err == nil {
				if end, err := time.Parse(time.RFC3339Nano, r.CompletedAt); err == nil {
					durationMs = end.Sub(start).Milliseconds()
				}
			}
		}

		action := "ALLOW"
		riskLevel := ""
		riskReason := ""
		confidence := 0
		hasRisk := false
		if r.Decision != nil {
			action = r.Decision.Action
			riskLevel = r.Decision.RiskLevel
			riskReason = r.Decision.Reason
			confidence = r.Decision.Confidence
			hasRisk = isRecordRisky(&r)
		}

		toolCalls := make([]map[string]interface{}, 0, len(r.ToolCalls))
		for _, tc := range r.ToolCalls {
			toolCalls = append(toolCalls, map[string]interface{}{
				"name":         tc.Name,
				"arguments":    tc.Arguments,
				"result":       tc.Result,
				"is_sensitive": tc.IsSensitive,
			})
		}

		requestContent := ""
		for _, msg := range r.Messages {
			if strings.EqualFold(msg.Role, "user") {
				requestContent = msg.Content
				break
			}
		}

		// 序列化完整 messages 供审计持久化
		messagesJSON := "[]"
		if len(r.Messages) > 0 {
			if b, err := json.Marshal(r.Messages); err == nil {
				messagesJSON = string(b)
			}
		}

		entry := map[string]interface{}{
			"id":                r.RequestID,
			"timestamp":         r.StartedAt,
			"request_id":        r.RequestID,
			"asset_name":        r.AssetName,
			"asset_id":          r.AssetID,
			"model":             r.Model,
			"request_content":   requestContent,
			"tool_calls":        toolCalls,
			"output_content":    r.OutputContent,
			"has_risk":          hasRisk,
			"risk_level":        riskLevel,
			"risk_reason":       riskReason,
			"confidence":        confidence,
			"action":            action,
			"prompt_tokens":     r.PromptTokens,
			"completion_tokens": r.CompletionTokens,
			"total_tokens":      r.PromptTokens + r.CompletionTokens,
			"duration_ms":       durationMs,
			"messages":          messagesJSON,
			"message_count":     r.MessageCount,
		}
		out = append(out, entry)
	}
	return out
}
