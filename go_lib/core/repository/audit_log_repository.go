package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"go_lib/core/logging"
)

// AuditLog 审计日志记录
type AuditLog struct {
	ID               string `json:"id"`
	Timestamp        string `json:"timestamp"`
	RequestID        string `json:"request_id"`
	AssetName        string `json:"asset_name,omitempty"`
	AssetID          string `json:"asset_id,omitempty"`
	Model            string `json:"model,omitempty"`
	RequestContent   string `json:"request_content,omitempty"`
	ToolCalls        string `json:"tool_calls,omitempty"`
	OutputContent    string `json:"output_content,omitempty"`
	HasRisk          bool   `json:"has_risk"`
	RiskLevel        string `json:"risk_level,omitempty"`
	RiskReason       string `json:"risk_reason,omitempty"`
	Confidence       int    `json:"confidence,omitempty"`
	Action           string `json:"action"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`
	TotalTokens      int    `json:"total_tokens,omitempty"`
	DurationMs       int    `json:"duration_ms"`
}

// AuditLogFilter 审计日志查询过滤条件
type AuditLogFilter struct {
	Limit       int    `json:"limit"`
	Offset      int    `json:"offset"`
	RiskOnly    bool   `json:"risk_only"`
	AssetName   string `json:"asset_name,omitempty"`
	AssetID     string `json:"asset_id,omitempty"`
	StartTime   string `json:"start_time,omitempty"`
	EndTime     string `json:"end_time,omitempty"`
	SearchQuery string `json:"search_query,omitempty"`
}

// AuditLogStatistics 审计日志统计
type AuditLogStatistics struct {
	Total        int `json:"total"`
	RiskCount    int `json:"risk_count"`
	BlockedCount int `json:"blocked_count"`
	AllowedCount int `json:"allowed_count"`
}

type AuditLogAsset struct {
	AssetName string `json:"asset_name"`
	AssetID   string `json:"asset_id"`
}

// AuditLogRepository 审计日志仓库
type AuditLogRepository struct {
	db *sql.DB
}

// NewAuditLogRepository 创建审计日志仓库实例
func NewAuditLogRepository(db *sql.DB) *AuditLogRepository {
	if db == nil {
		db = GetDB()
	}
	return &AuditLogRepository{db: db}
}

// SaveAuditLog 保存单条审计日志
func (r *AuditLogRepository) SaveAuditLog(log *AuditLog) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}

	hasRisk := 0
	if log.HasRisk {
		hasRisk = 1
	}

	_, err := r.db.Exec(`
		INSERT OR REPLACE INTO audit_logs 
		(id, timestamp, request_id, asset_name, asset_id, model, request_content, tool_calls, output_content,
		 has_risk, risk_level, risk_reason, confidence, action,
		 prompt_tokens, completion_tokens, total_tokens, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, log.ID, log.Timestamp, log.RequestID, log.AssetName, log.AssetID, log.Model,
		log.RequestContent, log.ToolCalls, log.OutputContent,
		hasRisk, log.RiskLevel, log.RiskReason, log.Confidence, log.Action,
		log.PromptTokens, log.CompletionTokens, log.TotalTokens, log.DurationMs)
	if err != nil {
		return fmt.Errorf("failed to save audit log: %w", err)
	}

	return nil
}

// SaveAuditLogsBatch 批量保存审计日志
func (r *AuditLogRepository) SaveAuditLogsBatch(logs []*AuditLog) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if len(logs) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO audit_logs 
		(id, timestamp, request_id, asset_name, asset_id, model, request_content, tool_calls, output_content,
		 has_risk, risk_level, risk_reason, confidence, action,
		 prompt_tokens, completion_tokens, total_tokens, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, log := range logs {
		hasRisk := 0
		if log.HasRisk {
			hasRisk = 1
		}
		_, err := stmt.Exec(log.ID, log.Timestamp, log.RequestID, log.AssetName, log.AssetID, log.Model,
			log.RequestContent, log.ToolCalls, log.OutputContent,
			hasRisk, log.RiskLevel, log.RiskReason, log.Confidence, log.Action,
			log.PromptTokens, log.CompletionTokens, log.TotalTokens, log.DurationMs)
		if err != nil {
			logging.Warning("Failed to save audit log %s: %v", log.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit batch: %w", err)
	}

	return nil
}

// GetAuditLogs 获取审计日志（支持过滤）
func (r *AuditLogRepository) GetAuditLogs(filter *AuditLogFilter) ([]*AuditLog, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if filter.Limit <= 0 {
		filter.Limit = 100
	}

	conditions := []string{}
	params := []interface{}{}

	if filter.RiskOnly {
		conditions = append(conditions, "has_risk = 1")
	}
	// asset_id is the unique instance key; when present it is sufficient and preferred.
	if filter.AssetID != "" {
		conditions = append(conditions, "asset_id = ?")
		params = append(params, filter.AssetID)
	} else if filter.AssetName != "" {
		conditions = append(conditions, "asset_name = ?")
		params = append(params, filter.AssetName)
	}
	if filter.StartTime != "" {
		conditions = append(conditions, "timestamp >= ?")
		params = append(params, filter.StartTime)
	}
	if filter.EndTime != "" {
		conditions = append(conditions, "timestamp <= ?")
		params = append(params, filter.EndTime)
	}
	if filter.SearchQuery != "" {
		conditions = append(conditions, "(request_content LIKE ? OR output_content LIKE ? OR risk_reason LIKE ?)")
		pattern := "%" + filter.SearchQuery + "%"
		params = append(params, pattern, pattern, pattern)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	params = append(params, filter.Limit, filter.Offset)

	query := fmt.Sprintf(`
		SELECT id, timestamp, request_id, asset_name, asset_id, model, request_content, tool_calls, output_content,
			has_risk, risk_level, risk_reason, confidence, action,
			prompt_tokens, completion_tokens, total_tokens, duration_ms
		FROM audit_logs %s ORDER BY timestamp DESC LIMIT ? OFFSET ?
	`, whereClause)

	rows, err := r.db.Query(query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}
	defer rows.Close()

	var logs []*AuditLog
	for rows.Next() {
		log, err := scanAuditLog(rows)
		if err != nil {
			logging.Warning("Failed to scan audit log row: %v", err)
			continue
		}
		logs = append(logs, log)
	}

	if logs == nil {
		logs = []*AuditLog{}
	}
	return logs, nil
}

// GetAuditLogCount 获取审计日志数量
func (r *AuditLogRepository) GetAuditLogCount(riskOnly bool, assetName, assetID string) (int, error) {
	if r.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	conditions := make([]string, 0, 3)
	params := make([]interface{}, 0, 3)
	if riskOnly {
		conditions = append(conditions, "has_risk = 1")
	}
	// Prefer unique asset_id; only fallback to asset_name when asset_id is absent.
	if assetID != "" {
		conditions = append(conditions, "asset_id = ?")
		params = append(params, assetID)
	} else if assetName != "" {
		conditions = append(conditions, "asset_name = ?")
		params = append(params, assetName)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var count int
	err := r.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM audit_logs %s", whereClause), params...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count audit logs: %w", err)
	}

	return count, nil
}

// GetAuditLogStatistics 获取审计日志统计
func (r *AuditLogRepository) GetAuditLogStatistics(assetName, assetID string) (*AuditLogStatistics, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	conditions := make([]string, 0, 2)
	params := make([]interface{}, 0, 2)
	// Prefer unique asset_id; only fallback to asset_name when asset_id is absent.
	if assetID != "" {
		conditions = append(conditions, "asset_id = ?")
		params = append(params, assetID)
	} else if assetName != "" {
		conditions = append(conditions, "asset_name = ?")
		params = append(params, assetName)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT 
			COUNT(*) as total,
			COALESCE(SUM(CASE WHEN action = 'WARN' THEN 1 ELSE 0 END), 0) as risk_count,
			COALESCE(SUM(CASE WHEN action = 'BLOCK' OR action = 'HARD_BLOCK' THEN 1 ELSE 0 END), 0) as blocked_count,
			COALESCE(SUM(CASE WHEN action = 'ALLOW' THEN 1 ELSE 0 END), 0) as allowed_count
		FROM audit_logs %s
	`, whereClause)

	row := r.db.QueryRow(query, params...)

	var stats AuditLogStatistics
	err := row.Scan(&stats.Total, &stats.RiskCount, &stats.BlockedCount, &stats.AllowedCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get audit log statistics: %w", err)
	}

	return &stats, nil
}

func (r *AuditLogRepository) GetAuditLogAssets() ([]*AuditLogAsset, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := r.db.Query(`
		SELECT asset_name, asset_id
		FROM audit_logs
		WHERE asset_name != ''
		GROUP BY asset_name, asset_id
		ORDER BY MAX(timestamp) DESC, asset_name ASC, asset_id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit log assets: %w", err)
	}
	defer rows.Close()

	assets := make([]*AuditLogAsset, 0)
	for rows.Next() {
		var asset AuditLogAsset
		if err := rows.Scan(&asset.AssetName, &asset.AssetID); err != nil {
			logging.Warning("Failed to scan audit log asset row: %v", err)
			continue
		}
		assets = append(assets, &asset)
	}

	return assets, nil
}

// CleanOldAuditLogs 清理旧审计日志（保留最近N天）
func (r *AuditLogRepository) CleanOldAuditLogs(keepDays int) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}

	if keepDays <= 0 {
		keepDays = 30
	}

	cutoffTime := time.Now().AddDate(0, 0, -keepDays).UTC().Format(time.RFC3339)
	_, err := r.db.Exec("DELETE FROM audit_logs WHERE timestamp < ?", cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to clean old audit logs: %w", err)
	}

	return nil
}

// ClearAllAuditLogs 清空所有审计日志
func (r *AuditLogRepository) ClearAllAuditLogs() error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}

	_, err := r.db.Exec("DELETE FROM audit_logs")
	if err != nil {
		return fmt.Errorf("failed to clear all audit logs: %w", err)
	}

	return nil
}

func (r *AuditLogRepository) ClearAuditLogs(assetName, assetID string) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}

	if assetID == "" && assetName == "" {
		return r.ClearAllAuditLogs()
	}

	var (
		query string
		args  []interface{}
	)
	if assetID != "" {
		query = "DELETE FROM audit_logs WHERE asset_id = ?"
		args = append(args, assetID)
	} else {
		query = "DELETE FROM audit_logs WHERE asset_name = ?"
		args = append(args, assetName)
	}

	if _, err := r.db.Exec(query, args...); err != nil {
		return fmt.Errorf("failed to clear filtered audit logs: %w", err)
	}

	return nil
}

// scanAuditLog 从查询结果行扫描AuditLog
func scanAuditLog(rows *sql.Rows) (*AuditLog, error) {
	var log AuditLog
	var hasRisk int
	var assetName, assetID sql.NullString
	var model, requestContent, toolCalls, outputContent sql.NullString
	var riskLevel, riskReason, action sql.NullString
	var confidence, promptTokens, completionTokens, totalTokens sql.NullInt64

	err := rows.Scan(&log.ID, &log.Timestamp, &log.RequestID, &assetName, &assetID,
		&model, &requestContent, &toolCalls, &outputContent,
		&hasRisk, &riskLevel, &riskReason, &confidence, &action,
		&promptTokens, &completionTokens, &totalTokens, &log.DurationMs)
	if err != nil {
		return nil, err
	}

	log.AssetName = assetName.String
	log.AssetID = assetID.String
	log.HasRisk = hasRisk == 1
	log.Model = model.String
	log.RequestContent = requestContent.String
	log.ToolCalls = toolCalls.String
	log.OutputContent = outputContent.String
	log.RiskLevel = riskLevel.String
	log.RiskReason = riskReason.String
	log.Action = action.String
	if action.String == "" {
		log.Action = "ALLOW"
	}
	if confidence.Valid {
		log.Confidence = int(confidence.Int64)
	}
	if promptTokens.Valid {
		log.PromptTokens = int(promptTokens.Int64)
	}
	if completionTokens.Valid {
		log.CompletionTokens = int(completionTokens.Int64)
	}
	if totalTokens.Valid {
		log.TotalTokens = int(totalTokens.Int64)
	}

	return &log, nil
}
