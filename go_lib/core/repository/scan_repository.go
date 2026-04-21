package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go_lib/core"
	"go_lib/core/logging"
)

const retainedScanCount = 20

var (
	latestScanLogMu          sync.Mutex
	lastLatestScanSignature  string
	latestScanLogInitialized bool
)

// ScanRecord 扫描记录，对应数据库scans表的一条记录及其关联的资产和风险
type ScanRecord struct {
	ID          int64        `json:"id"`
	CreatedAt   string       `json:"created_at"`
	ConfigFound bool         `json:"config_found"`
	ConfigPath  string       `json:"config_path,omitempty"`
	ConfigJSON  string       `json:"config_json,omitempty"`
	Assets      []core.Asset `json:"assets"`
	Risks       []core.Risk  `json:"risks"`
}

// ScanRepository 扫描结果仓库
// 封装了扫描记录、资产和风险数据的CRUD操作
type ScanRepository struct {
	db *sql.DB
}

// NewScanRepository 创建扫描结果仓库实例
// 如果db为nil，将尝试使用全局数据库连接
func NewScanRepository(db *sql.DB) *ScanRepository {
	if db == nil {
		db = GetDB()
	}
	return &ScanRepository{db: db}
}

// SaveScanResult 保存完整的扫描结果（扫描记录+资产+风险）
// 使用事务保证数据一致性
func (r *ScanRepository) SaveScanResult(record *ScanRecord) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 设置创建时间
	if record.CreatedAt == "" {
		record.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	configFound := 0
	if record.ConfigFound {
		configFound = 1
	}

	// 插入扫描记录
	result, err := tx.Exec(`
		INSERT INTO scans (created_at, config_found, config_path, config_json)
		VALUES (?, ?, ?, ?)
	`, record.CreatedAt, configFound, record.ConfigPath, record.ConfigJSON)
	if err != nil {
		return fmt.Errorf("failed to insert scan record: %w", err)
	}

	scanID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get scan id: %w", err)
	}
	record.ID = scanID

	// 插入资产数据
	for _, asset := range record.Assets {
		assetJSON, err := json.Marshal(asset)
		if err != nil {
			logging.Warning("Failed to marshal asset %s: %v", asset.Name, err)
			continue
		}
		if _, err := tx.Exec(`INSERT INTO assets (scan_id, data) VALUES (?, ?)`,
			scanID, string(assetJSON)); err != nil {
			return fmt.Errorf("failed to insert asset: %w", err)
		}
	}

	// 插入风险数据
	for _, risk := range record.Risks {
		riskJSON, err := json.Marshal(risk)
		if err != nil {
			logging.Warning("Failed to marshal risk %s: %v", risk.ID, err)
			continue
		}
		if _, err := tx.Exec(`INSERT INTO risks (scan_id, data) VALUES (?, ?)`,
			scanID, string(riskJSON)); err != nil {
			return fmt.Errorf("failed to insert risk: %w", err)
		}
	}

	// 仅保留最近固定数量的扫描记录，防止数据库无限增长。
	if err := pruneOldScansTx(tx, retainedScanCount); err != nil {
		return fmt.Errorf("failed to prune old scans: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logging.Info("Scan result saved, id=%d, assets=%d, risks=%d",
		scanID, len(record.Assets), len(record.Risks))
	return nil
}

// pruneOldScansTx 在事务内仅保留最近 keepCount 条扫描记录。
// 依赖外键级联删除自动清理 assets/risks 关联数据。
func pruneOldScansTx(tx *sql.Tx, keepCount int) error {
	if keepCount <= 0 {
		return fmt.Errorf("keep_count must be greater than 0")
	}

	if _, err := tx.Exec(`
		DELETE FROM scans
		WHERE id NOT IN (
			SELECT id FROM scans ORDER BY id DESC LIMIT ?
		)
	`, keepCount); err != nil {
		return fmt.Errorf("failed to delete old scans: %w", err)
	}

	return nil
}

// GetLatestScanResult 获取最新的扫描结果
// 返回nil表示没有扫描记录
func (r *ScanRepository) GetLatestScanResult() (*ScanRecord, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// 查询最新的扫描记录
	row := r.db.QueryRow(`SELECT id, created_at, config_found, config_path, config_json
		FROM scans ORDER BY id DESC LIMIT 1`)

	var record ScanRecord
	var configFound int
	var configPath, configJSON sql.NullString

	err := row.Scan(&record.ID, &record.CreatedAt, &configFound, &configPath, &configJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query latest scan: %w", err)
	}

	record.ConfigFound = configFound == 1
	if configPath.Valid {
		record.ConfigPath = configPath.String
	}
	if configJSON.Valid {
		record.ConfigJSON = configJSON.String
	}

	// 查询关联的资产
	assetRows, err := r.db.Query(`SELECT data FROM assets WHERE scan_id = ?`, record.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to query assets: %w", err)
	}
	defer assetRows.Close()

	for assetRows.Next() {
		var dataStr string
		if err := assetRows.Scan(&dataStr); err != nil {
			logging.Warning("Failed to scan asset row: %v", err)
			continue
		}
		var asset core.Asset
		if err := json.Unmarshal([]byte(dataStr), &asset); err != nil {
			logging.Warning("Failed to unmarshal asset data: %v", err)
			continue
		}
		record.Assets = append(record.Assets, asset)
	}

	// 查询关联的风险
	riskRows, err := r.db.Query(`SELECT data FROM risks WHERE scan_id = ?`, record.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to query risks: %w", err)
	}
	defer riskRows.Close()

	for riskRows.Next() {
		var dataStr string
		if err := riskRows.Scan(&dataStr); err != nil {
			logging.Warning("Failed to scan risk row: %v", err)
			continue
		}
		var risk core.Risk
		if err := json.Unmarshal([]byte(dataStr), &risk); err != nil {
			logging.Warning("Failed to unmarshal risk data: %v", err)
			continue
		}
		record.Risks = append(record.Risks, risk)
	}

	if record.Assets == nil {
		record.Assets = []core.Asset{}
	}
	if record.Risks == nil {
		record.Risks = []core.Risk{}
	}

	if shouldLogLatestScanResult(&record) {
		logging.Info("Loaded latest scan result, id=%d, assets=%d, risks=%d",
			record.ID, len(record.Assets), len(record.Risks))
	}
	return &record, nil
}

// shouldLogLatestScanResult 仅在最新扫描关键信息变化时返回 true。
func shouldLogLatestScanResult(record *ScanRecord) bool {
	signature := fmt.Sprintf("%d|%d|%d", record.ID, len(record.Assets), len(record.Risks))

	latestScanLogMu.Lock()
	defer latestScanLogMu.Unlock()

	if latestScanLogInitialized && lastLatestScanSignature == signature {
		return false
	}
	lastLatestScanSignature = signature
	latestScanLogInitialized = true
	return true
}

// DeleteScanResultByID 删除指定扫描记录及其关联资产、风险数据。
// 用于上层在“保存成功但后续同步失败”场景下执行补偿回滚，保证原子语义。
func (r *ScanRepository) DeleteScanResultByID(scanID int64) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if scanID <= 0 {
		return fmt.Errorf("scan_id must be greater than 0")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin delete scan transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM risks WHERE scan_id = ?`, scanID); err != nil {
		return fmt.Errorf("failed to delete risks for scan_id=%d: %w", scanID, err)
	}
	if _, err := tx.Exec(`DELETE FROM assets WHERE scan_id = ?`, scanID); err != nil {
		return fmt.Errorf("failed to delete assets for scan_id=%d: %w", scanID, err)
	}
	if _, err := tx.Exec(`DELETE FROM scans WHERE id = ?`, scanID); err != nil {
		return fmt.Errorf("failed to delete scan record for scan_id=%d: %w", scanID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit delete scan transaction: %w", err)
	}

	logging.Info("Scan result rollback completed, scan_id=%d", scanID)
	return nil
}
