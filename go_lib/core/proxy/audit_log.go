package proxy

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// AuditLog represents a single audit log entry for protection proxy
type AuditLog struct {
	ID               string          `json:"id"`
	Timestamp        string          `json:"timestamp"`
	RequestID        string          `json:"request_id"`
	AssetName        string          `json:"asset_name,omitempty"`
	AssetID          string          `json:"asset_id,omitempty"`
	Model            string          `json:"model,omitempty"`
	RequestContent   string          `json:"request_content"` // Summary of request messages
	ToolCalls        []AuditToolCall `json:"tool_calls,omitempty"`
	OutputContent    string          `json:"output_content,omitempty"` // Summary of response
	HasRisk          bool            `json:"has_risk"`
	RiskLevel        string          `json:"risk_level,omitempty"`  // SAFE, SUSPICIOUS, DANGEROUS, CRITICAL
	RiskReason       string          `json:"risk_reason,omitempty"` // Reason for risk detection
	Confidence       int             `json:"confidence,omitempty"`  // Confidence level 0-100
	Action           string          `json:"action"`                // ALLOW, WARN, BLOCK, HARD_BLOCK
	PromptTokens     int             `json:"prompt_tokens,omitempty"`
	CompletionTokens int             `json:"completion_tokens,omitempty"`
	TotalTokens      int             `json:"total_tokens,omitempty"`
	Duration         int64           `json:"duration_ms"` // Request duration in milliseconds
}

// AuditToolCall represents a tool call in the audit log
type AuditToolCall struct {
	Name        string `json:"name"`
	Arguments   string `json:"arguments"`
	Result      string `json:"result"` // Remove omitempty to ensure empty string is sent
	IsSensitive bool   `json:"is_sensitive,omitempty"`
}

// AuditLogBuffer stores audit logs before they are persisted
type AuditLogBuffer struct {
	mu     sync.Mutex
	logs   []AuditLog
	maxLen int
}

var (
	auditLogBuffer = &AuditLogBuffer{
		logs:   make([]AuditLog, 0),
		maxLen: 1000, // Keep last 1000 logs in memory
	}
	auditLogIDCounter int64
	auditLogMu        sync.Mutex
)

// generateAuditLogID generates a unique audit log ID
func generateAuditLogID() string {
	auditLogMu.Lock()
	defer auditLogMu.Unlock()
	auditLogIDCounter++
	return fmt.Sprintf("audit_%d_%d", time.Now().UnixNano(), auditLogIDCounter)
}

// AddAuditLog adds an audit log entry to the buffer
func (b *AuditLogBuffer) AddAuditLog(log AuditLog) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if log.ID == "" {
		log.ID = generateAuditLogID()
	}
	if log.Timestamp == "" {
		log.Timestamp = time.Now().Format(time.RFC3339)
	}

	b.logs = append(b.logs, log)

	// Trim if exceeds max length
	if len(b.logs) > b.maxLen {
		b.logs = b.logs[len(b.logs)-b.maxLen:]
	}
}

// UpdateAuditLogTokens updates token usage for an existing audit log by RequestID
func (b *AuditLogBuffer) UpdateAuditLogTokens(requestID string, promptTokens, completionTokens, totalTokens int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Search from newest to oldest as we likely want to update the most recent one
	for i := len(b.logs) - 1; i >= 0; i-- {
		if b.logs[i].RequestID == requestID {
			b.logs[i].PromptTokens = promptTokens
			b.logs[i].CompletionTokens = completionTokens
			b.logs[i].TotalTokens = totalTokens
			return true
		}
	}
	return false
}

// GetAuditLogs returns audit logs from the buffer with optional filtering
func (b *AuditLogBuffer) GetAuditLogs(limit int, offset int, riskOnly bool) []AuditLog {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Apply risk filter
	var filtered []AuditLog
	if riskOnly {
		for _, log := range b.logs {
			if log.HasRisk {
				filtered = append(filtered, log)
			}
		}
	} else {
		filtered = b.logs
	}

	// Reverse order (newest first)
	result := make([]AuditLog, len(filtered))
	for i, log := range filtered {
		result[len(filtered)-1-i] = log
	}

	// Apply offset and limit
	if offset >= len(result) {
		return []AuditLog{}
	}
	result = result[offset:]
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}

	return result
}

// GetAuditLogCount returns the count of audit logs
func (b *AuditLogBuffer) GetAuditLogCount(riskOnly bool) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !riskOnly {
		return len(b.logs)
	}

	count := 0
	for _, log := range b.logs {
		if log.HasRisk {
			count++
		}
	}
	return count
}

// ClearAuditLogs clears all audit logs from the buffer
func (b *AuditLogBuffer) ClearAuditLogs() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.logs = make([]AuditLog, 0)
}

func (b *AuditLogBuffer) ClearAuditLogsWithFilter(assetName, assetID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if assetID == "" && assetName == "" {
		b.logs = make([]AuditLog, 0)
		return
	}

	filtered := make([]AuditLog, 0, len(b.logs))
	for _, log := range b.logs {
		matches := false
		if assetID != "" {
			matches = log.AssetID == assetID
		} else {
			matches = log.AssetName == assetName
		}
		if !matches {
			filtered = append(filtered, log)
		}
	}
	b.logs = filtered
}

// GetAndClearAuditLogs returns all logs and clears the buffer
// This is used for batch persistence to SQLite from Dart side
func (b *AuditLogBuffer) GetAndClearAuditLogs() []AuditLog {
	b.mu.Lock()
	defer b.mu.Unlock()

	logs := b.logs
	b.logs = make([]AuditLog, 0)
	return logs
}

// ==================== FFI wrapper functions ====================

// GetAuditLogsInternal retrieves audit logs from the buffer
func GetAuditLogsInternal(limit, offset int, riskOnly bool) string {
	logs := auditLogBuffer.GetAuditLogs(limit, offset, riskOnly)

	result := map[string]interface{}{
		"logs":  logs,
		"total": auditLogBuffer.GetAuditLogCount(riskOnly),
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return `{"logs":[],"total":0,"error":"` + err.Error() + `"}`
	}
	return string(jsonBytes)
}

// GetPendingAuditLogsInternal retrieves and clears pending audit logs for persistence
func GetPendingAuditLogsInternal() string {
	logs := auditLogBuffer.GetAndClearAuditLogs()

	jsonBytes, err := json.Marshal(logs)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

// ClearAuditLogsInternal clears all audit logs from the buffer
func ClearAuditLogsInternal() string {
	auditLogBuffer.ClearAuditLogs()
	return `{"success":true}`
}

func ClearAuditLogsWithFilterInternal(filterJSON string) string {
	var input struct {
		AssetName string `json:"asset_name,omitempty"`
		AssetID   string `json:"asset_id,omitempty"`
	}
	if filterJSON != "" {
		if err := json.Unmarshal([]byte(filterJSON), &input); err != nil {
			return `{"success":false,"error":"invalid JSON"}`
		}
	}

	auditLogBuffer.ClearAuditLogsWithFilter(input.AssetName, input.AssetID)
	return `{"success":true}`
}
