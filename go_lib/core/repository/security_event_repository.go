package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"go_lib/core/logging"
)

// SecurityEventRecord 安全事件数据库记录
type SecurityEventRecord struct {
	ID         string `json:"id"`
	Timestamp  string `json:"timestamp"`
	EventType  string `json:"event_type"`
	ActionDesc string `json:"action_desc"`
	RiskType   string `json:"risk_type"`
	Detail     string `json:"detail"`
	Source     string `json:"source"`
	AssetName  string `json:"asset_name,omitempty"`
	AssetID    string `json:"asset_id,omitempty"`
}

// SecurityEventRepository 安全事件仓库
type SecurityEventRepository struct {
	db *sql.DB
}

// NewSecurityEventRepository 创建安全事件仓库实例
func NewSecurityEventRepository(db *sql.DB) *SecurityEventRepository {
	if db == nil {
		db = GetDB()
	}
	return &SecurityEventRepository{db: db}
}

// SaveSecurityEventsBatch 批量保存安全事件
func (r *SecurityEventRepository) SaveSecurityEventsBatch(events []*SecurityEventRecord) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if len(events) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO security_events
		(id, timestamp, event_type, action_desc, risk_type, detail, source, asset_name, asset_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, evt := range events {
		_, err := stmt.Exec(evt.ID, evt.Timestamp, evt.EventType,
			evt.ActionDesc, evt.RiskType, evt.Detail, evt.Source,
			strings.TrimSpace(evt.AssetName), strings.TrimSpace(evt.AssetID))
		if err != nil {
			logging.Warning("Failed to save security event %s: %v", evt.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit batch: %w", err)
	}

	return nil
}

// GetSecurityEvents 获取安全事件列表（按时间倒序，仅按 asset_id 过滤）
func (r *SecurityEventRepository) GetSecurityEvents(limit, offset int, assetID string) ([]*SecurityEventRecord, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if limit <= 0 {
		limit = 100
	}

	assetID = strings.TrimSpace(assetID)

	query := `
		SELECT id, timestamp, event_type, action_desc, risk_type, detail, source, asset_name, asset_id
		FROM security_events
	`
	args := make([]interface{}, 0, 4)
	where := make([]string, 0, 1)
	if assetID != "" {
		where = append(where, "asset_id = ?")
		args = append(args, assetID)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY timestamp DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query security events: %w", err)
	}
	defer rows.Close()

	var events []*SecurityEventRecord
	for rows.Next() {
		evt, err := scanSecurityEvent(rows)
		if err != nil {
			logging.Warning("Failed to scan security event row: %v", err)
			continue
		}
		events = append(events, evt)
	}

	if events == nil {
		events = []*SecurityEventRecord{}
	}
	return events, nil
}

// GetSecurityEventCount 获取安全事件数量（仅按 asset_id 过滤）
func (r *SecurityEventRepository) GetSecurityEventCount(assetID string) (int, error) {
	if r.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	assetID = strings.TrimSpace(assetID)

	var count int
	query := "SELECT COUNT(*) FROM security_events"
	args := make([]interface{}, 0, 2)
	where := make([]string, 0, 1)
	if assetID != "" {
		where = append(where, "asset_id = ?")
		args = append(args, assetID)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	err := r.db.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count security events: %w", err)
	}
	return count, nil
}

// CleanOldSecurityEvents 清理旧安全事件（保留最近N天）
func (r *SecurityEventRepository) CleanOldSecurityEvents(keepDays int) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if keepDays <= 0 {
		keepDays = 30
	}

	cutoffTime := time.Now().AddDate(0, 0, -keepDays).UTC().Format(time.RFC3339)
	_, err := r.db.Exec("DELETE FROM security_events WHERE timestamp < ?", cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to clean old security events: %w", err)
	}
	return nil
}

// ClearAllSecurityEvents 清空安全事件（仅按 asset_id 过滤）
func (r *SecurityEventRepository) ClearAllSecurityEvents(assetID string) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}
	assetID = strings.TrimSpace(assetID)

	query := "DELETE FROM security_events"
	args := make([]interface{}, 0, 2)
	where := make([]string, 0, 1)
	if assetID != "" {
		where = append(where, "asset_id = ?")
		args = append(args, assetID)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	_, err := r.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to clear all security events: %w", err)
	}
	return nil
}

func scanSecurityEvent(rows *sql.Rows) (*SecurityEventRecord, error) {
	var evt SecurityEventRecord
	var riskType, detail sql.NullString
	var assetName, assetID sql.NullString

	err := rows.Scan(&evt.ID, &evt.Timestamp, &evt.EventType,
		&evt.ActionDesc, &riskType, &detail, &evt.Source, &assetName, &assetID)
	if err != nil {
		return nil, err
	}

	evt.RiskType = riskType.String
	evt.Detail = detail.String
	evt.AssetName = assetName.String
	evt.AssetID = assetID.String
	return &evt, nil
}
