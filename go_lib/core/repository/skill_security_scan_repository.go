package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"go_lib/core/logging"
)

// SkillScanRecord represents a skill scan record in the skill_scans table
type SkillScanRecord struct {
	ID           int64    `json:"id"`
	SkillName    string   `json:"skill_name"`
	SkillHash    string   `json:"skill_hash"`
	SkillPath    string   `json:"skill_path,omitempty"`
	SourcePlugin string   `json:"source_plugin,omitempty"`
	AssetID      string   `json:"asset_id,omitempty"`
	ScannedAt    string   `json:"scanned_at"`
	Safe         bool     `json:"safe"`
	RiskLevel    string   `json:"risk_level,omitempty"`
	Issues       []string `json:"issues,omitempty"`
	Trusted      bool     `json:"trusted"`
	DeletedAt    string   `json:"deleted_at,omitempty"`
}

// SkillSecurityScanRepository 技能安全扫描仓库
// 封装了技能安全扫描记录的CRUD操作
type SkillSecurityScanRepository struct {
	db *sql.DB
}

// NewSkillSecurityScanRepository 创建技能安全扫描仓库实例
// 如果db为nil，将尝试使用全局数据库连接
func NewSkillSecurityScanRepository(db *sql.DB) *SkillSecurityScanRepository {
	if db == nil {
		db = GetDB()
	}
	return &SkillSecurityScanRepository{db: db}
}

// GetScannedSkillHashes 获取所有已成功扫描且具备删除所需元数据的技能哈希
// Excludes scans that failed with risk_level='error' so they can be retried.
// Excludes records with empty skill_path/source_plugin so legacy data can be rescanned and backfilled.
func (r *SkillSecurityScanRepository) GetScannedSkillHashes() ([]string, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := r.db.Query(`
		SELECT skill_hash
		FROM skill_scans
		WHERE (risk_level IS NULL OR risk_level != 'error')
		  AND COALESCE(TRIM(skill_path), '') != ''
		  AND COALESCE(TRIM(source_plugin), '') != ''
		  AND COALESCE(TRIM(deleted_at), '') = ''
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query skill hashes: %w", err)
	}
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			logging.Warning("Failed to scan skill hash: %v", err)
			continue
		}
		hashes = append(hashes, hash)
	}

	if hashes == nil {
		hashes = []string{}
	}
	return hashes, nil
}

// SaveSkillScanResult saves a skill scan result.
// Uses INSERT OR REPLACE for idempotent writes (based on skill_hash unique constraint).
func (r *SkillSecurityScanRepository) SaveSkillScanResult(record *SkillScanRecord) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}

	if record.ScannedAt == "" {
		record.ScannedAt = time.Now().UTC().Format(time.RFC3339)
	}

	safe := 0
	if record.Safe {
		safe = 1
	}

	var issuesJSON *string
	if len(record.Issues) > 0 {
		data, err := json.Marshal(record.Issues)
		if err == nil {
			s := string(data)
			issuesJSON = &s
		}
	}

	var riskLevel *string
	if record.RiskLevel != "" {
		riskLevel = &record.RiskLevel
	}

	_, err := r.db.Exec(`
		INSERT OR REPLACE INTO skill_scans
			(skill_name, skill_hash, skill_path, source_plugin, asset_id, scanned_at, safe, issues, risk_level, trusted, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, COALESCE((SELECT trusted FROM skill_scans WHERE skill_hash = ?), 0), '')
	`, record.SkillName, record.SkillHash, record.SkillPath, record.SourcePlugin, record.AssetID, record.ScannedAt, safe, issuesJSON, riskLevel, record.SkillHash)
	if err != nil {
		return fmt.Errorf("failed to save skill scan result: %w", err)
	}

	logging.Info("Skill scan result saved: name=%s, hash=%s, safe=%v, risk_level=%s",
		record.SkillName, record.SkillHash, record.Safe, record.RiskLevel)
	return nil
}

// GetSkillScanByHash queries a scan record by skill hash.
// Returns nil if not found.
func (r *SkillSecurityScanRepository) GetSkillScanByHash(hash string) (*SkillScanRecord, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	row := r.db.QueryRow(`
		SELECT id, skill_name, skill_hash, skill_path, source_plugin, asset_id, scanned_at, safe, issues, risk_level, COALESCE(trusted, 0), COALESCE(deleted_at, '')
		FROM skill_scans WHERE skill_hash = ?
	`, hash)

	var record SkillScanRecord
	var safe int
	var issuesJSON sql.NullString
	var riskLevel sql.NullString
	var trusted int

	err := row.Scan(&record.ID, &record.SkillName, &record.SkillHash, &record.SkillPath,
		&record.SourcePlugin, &record.AssetID, &record.ScannedAt, &safe, &issuesJSON, &riskLevel, &trusted, &record.DeletedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query skill scan: %w", err)
	}

	record.Safe = safe == 1
	record.Trusted = trusted == 1
	if riskLevel.Valid && riskLevel.String != "" {
		record.RiskLevel = riskLevel.String
	}
	if issuesJSON.Valid && issuesJSON.String != "" {
		if err := json.Unmarshal([]byte(issuesJSON.String), &record.Issues); err != nil {
			logging.Warning("Failed to unmarshal skill issues: %v", err)
		}
	}
	if record.Issues == nil {
		record.Issues = []string{}
	}

	return &record, nil
}

// DeleteSkillScan 根据技能哈希删除扫描记录
func (r *SkillSecurityScanRepository) DeleteSkillScan(skillHash string) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}

	_, err := r.db.Exec(`
		UPDATE skill_scans
		SET deleted_at = ?
		WHERE skill_hash = ?
	`, time.Now().UTC().Format(time.RFC3339), skillHash)
	if err != nil {
		return fmt.Errorf("failed to delete skill scan: %w", err)
	}

	logging.Info("Skill scan soft deleted: hash=%s", skillHash)
	return nil
}

// GetRiskySkills retrieves all skills with security risks (safe=0) that are not trusted
func (r *SkillSecurityScanRepository) GetRiskySkills() ([]SkillScanRecord, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := r.db.Query(`
		SELECT id, skill_name, skill_hash, skill_path, source_plugin, asset_id, scanned_at, safe, issues, risk_level, COALESCE(trusted, 0), COALESCE(deleted_at, '')
		FROM skill_scans
		WHERE safe = 0
		  AND (trusted IS NULL OR trusted = 0)
		  AND COALESCE(TRIM(deleted_at), '') = ''
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query risky skills: %w", err)
	}
	defer rows.Close()

	var records []SkillScanRecord
	for rows.Next() {
		var record SkillScanRecord
		var safe int
		var issuesJSON sql.NullString
		var riskLevel sql.NullString
		var trusted int

		if err := rows.Scan(&record.ID, &record.SkillName, &record.SkillHash, &record.SkillPath,
			&record.SourcePlugin, &record.AssetID, &record.ScannedAt, &safe, &issuesJSON, &riskLevel, &trusted, &record.DeletedAt); err != nil {
			logging.Warning("Failed to scan risky skill row: %v", err)
			continue
		}

		record.Safe = safe == 1
		record.Trusted = trusted == 1
		if riskLevel.Valid && riskLevel.String != "" {
			record.RiskLevel = riskLevel.String
		}
		if issuesJSON.Valid && issuesJSON.String != "" {
			if err := json.Unmarshal([]byte(issuesJSON.String), &record.Issues); err != nil {
				logging.Warning("Failed to unmarshal skill issues: %v", err)
			}
		}
		if record.Issues == nil {
			record.Issues = []string{}
		}
		records = append(records, record)
	}

	if records == nil {
		records = []SkillScanRecord{}
	}
	return records, nil
}

// GetAllSkillScans retrieves all skill scan records ordered by scan time descending
func (r *SkillSecurityScanRepository) GetAllSkillScans() ([]SkillScanRecord, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := r.db.Query(`
		SELECT id, skill_name, skill_hash, skill_path, source_plugin, asset_id, scanned_at, safe, issues, risk_level, COALESCE(trusted, 0), COALESCE(deleted_at, '')
		FROM skill_scans ORDER BY scanned_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query all skill scans: %w", err)
	}
	defer rows.Close()

	var records []SkillScanRecord
	for rows.Next() {
		var record SkillScanRecord
		var safe int
		var issuesJSON sql.NullString
		var riskLevel sql.NullString
		var trusted int

		if err := rows.Scan(&record.ID, &record.SkillName, &record.SkillHash, &record.SkillPath,
			&record.SourcePlugin, &record.AssetID, &record.ScannedAt, &safe, &issuesJSON, &riskLevel, &trusted, &record.DeletedAt); err != nil {
			logging.Warning("Failed to scan skill scan row: %v", err)
			continue
		}

		record.Safe = safe == 1
		record.Trusted = trusted == 1
		if riskLevel.Valid && riskLevel.String != "" {
			record.RiskLevel = riskLevel.String
		}
		if issuesJSON.Valid && issuesJSON.String != "" {
			if err := json.Unmarshal([]byte(issuesJSON.String), &record.Issues); err != nil {
				logging.Warning("Failed to unmarshal skill issues: %v", err)
			}
		}
		if record.Issues == nil {
			record.Issues = []string{}
		}
		records = append(records, record)
	}

	if records == nil {
		records = []SkillScanRecord{}
	}
	return records, nil
}

// TrustSkill marks a skill scan record as trusted by skill hash
func (r *SkillSecurityScanRepository) TrustSkill(skillHash string) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := r.db.Exec(`UPDATE skill_scans SET trusted = 1 WHERE skill_hash = ?`, skillHash)
	if err != nil {
		return fmt.Errorf("failed to trust skill: %w", err)
	}
	logging.Info("Skill trusted: hash=%s", skillHash)
	return nil
}
