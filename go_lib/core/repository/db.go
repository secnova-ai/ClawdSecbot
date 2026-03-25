// Package repository 提供数据库访问层
// 封装了SQLite数据库的连接管理和表结构初始化
package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"go_lib/core/logging"

	_ "modernc.org/sqlite"
)

var (
	// globalDB 全局数据库连接实例
	globalDB *sql.DB
	// dbMutex 保护数据库初始化的互斥锁
	dbMutex sync.RWMutex
)

// InitDB 初始化数据库连接
// dbPath 为SQLite数据库文件的完整路径（与Flutter端使用同一文件）
// 该方法是幂等的，重复调用会先关闭旧连接再重新打开
func InitDB(dbPath string) error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	// 如果已有连接，先关闭
	if globalDB != nil {
		globalDB.Close()
		globalDB = nil
	}

	logging.Info("Initializing database connection: %s", dbPath)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		logging.Error("Failed to open database: %v", err)
		return fmt.Errorf("failed to open database: %w", err)
	}

	// 启用WAL模式，支持与Flutter端的并发读写
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		logging.Warning("Failed to enable WAL mode: %v", err)
	}

	// Set busy timeout to 5 seconds to avoid SQLITE_BUSY errors on concurrent access
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		logging.Warning("Failed to set busy_timeout: %v", err)
	}

	// 启用外键约束
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		logging.Warning("Failed to enable foreign keys: %v", err)
	}

	// 验证连接可用
	if err := db.Ping(); err != nil {
		db.Close()
		logging.Error("Database ping failed: %v", err)
		return fmt.Errorf("database ping failed: %w", err)
	}

	globalDB = db

	// 创建资产相关的表结构（幂等）
	if err := createAssetTables(db); err != nil {
		globalDB.Close()
		globalDB = nil
		logging.Error("Failed to create asset tables: %v", err)
		return fmt.Errorf("failed to create tables: %w", err)
	}

	// 创建安全模型配置表（幂等）
	if err := CreateSecurityModelConfigTable(db); err != nil {
		globalDB.Close()
		globalDB = nil
		logging.Error("Failed to create security model config table: %v", err)
		return fmt.Errorf("failed to create security model config table: %w", err)
	}

	// 创建保护相关的表结构（幂等）
	if err := createProtectionTables(db); err != nil {
		globalDB.Close()
		globalDB = nil
		logging.Error("Failed to create protection tables: %v", err)
		return fmt.Errorf("failed to create protection tables: %w", err)
	}

	// 创建审计日志相关的表结构（幂等）
	if err := createAuditLogTables(db); err != nil {
		globalDB.Close()
		globalDB = nil
		logging.Error("Failed to create audit log tables: %v", err)
		return fmt.Errorf("failed to create audit log tables: %w", err)
	}

	// 创建API指标相关的表结构（幂等）
	if err := createMetricsTables(db); err != nil {
		globalDB.Close()
		globalDB = nil
		logging.Error("Failed to create metrics tables: %v", err)
		return fmt.Errorf("failed to create metrics tables: %w", err)
	}

	// 创建应用权限表（幂等）
	if err := createAppPermissionsTables(db); err != nil {
		globalDB.Close()
		globalDB = nil
		logging.Error("Failed to create app permissions tables: %v", err)
		return fmt.Errorf("failed to create app permissions tables: %w", err)
	}

	// 创建应用设置表（幂等）
	if err := CreateAppSettingsTable(db); err != nil {
		globalDB.Close()
		globalDB = nil
		logging.Error("Failed to create app settings table: %v", err)
		return fmt.Errorf("failed to create app settings table: %w", err)
	}

	// 创建安全事件表（幂等）
	if err := createSecurityEventTables(db); err != nil {
		globalDB.Close()
		globalDB = nil
		logging.Error("Failed to create security event tables: %v", err)
		return fmt.Errorf("failed to create security event tables: %w", err)
	}

	logging.Info("Database initialized successfully")
	return nil
}

// createProtectionTables 创建保护相关的表结构
func createProtectionTables(db *sql.DB) error {
	// 保护状态表（全局唯一，id=1）
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS protection_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			enabled INTEGER NOT NULL DEFAULT 0,
			provider_name TEXT,
			proxy_port INTEGER,
			original_base_url TEXT,
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("failed to create protection_state table: %w", err)
	}

	// 保护配置表（按资产实例ID唯一）
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS protection_config (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			asset_name TEXT NOT NULL,
			asset_id TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			audit_only INTEGER NOT NULL DEFAULT 0,
			sandbox_enabled INTEGER NOT NULL DEFAULT 0,
			gateway_binary_path TEXT,
			gateway_config_path TEXT,
			custom_security_prompt TEXT DEFAULT '',
			single_session_token_limit INTEGER DEFAULT 0,
			daily_token_limit INTEGER DEFAULT 0,
			path_permission TEXT DEFAULT '{}',
			network_permission TEXT DEFAULT '{}',
			shell_permission TEXT DEFAULT '{}',
			bot_model_config TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(asset_id)
		)
	`); err != nil {
		return fmt.Errorf("failed to create protection_config table: %w", err)
	}

	// 确保 bot_model_config 列存在
	migrateAddColumn(db, "protection_config", "bot_model_config", "TEXT")

	// 保护统计表（按资产实例ID唯一）
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS protection_statistics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			asset_name TEXT NOT NULL,
			asset_id TEXT NOT NULL,
			analysis_count INTEGER NOT NULL DEFAULT 0,
			message_count INTEGER NOT NULL DEFAULT 0,
			warning_count INTEGER NOT NULL DEFAULT 0,
			blocked_count INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			total_prompt_tokens INTEGER NOT NULL DEFAULT 0,
			total_completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tool_calls INTEGER NOT NULL DEFAULT 0,
			request_count INTEGER NOT NULL DEFAULT 0,
			audit_tokens INTEGER NOT NULL DEFAULT 0,
			audit_prompt_tokens INTEGER NOT NULL DEFAULT 0,
			audit_completion_tokens INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			UNIQUE(asset_id)
		)
	`); err != nil {
		return fmt.Errorf("failed to create protection_statistics table: %w", err)
	}

	// Shepherd规则表（按资产实例ID隔离）
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS shepherd_rules (
			asset_id TEXT PRIMARY KEY,
			asset_name TEXT NOT NULL,
			sensitive_actions TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("failed to create shepherd_rules table: %w", err)
	}

	logging.Info("Protection tables created/verified successfully")
	return nil
}

// createAuditLogTables 创建审计日志相关的表结构
func createAuditLogTables(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_logs (
			id TEXT PRIMARY KEY,
			timestamp TEXT NOT NULL,
			request_id TEXT NOT NULL,
			asset_name TEXT NOT NULL DEFAULT '',
			asset_id TEXT NOT NULL DEFAULT '',
			model TEXT,
			request_content TEXT,
			tool_calls TEXT,
			output_content TEXT,
			has_risk INTEGER NOT NULL DEFAULT 0,
			risk_level TEXT,
			risk_reason TEXT,
			confidence INTEGER,
			action TEXT NOT NULL DEFAULT 'ALLOW',
			prompt_tokens INTEGER,
			completion_tokens INTEGER,
			total_tokens INTEGER,
			duration_ms INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		return fmt.Errorf("failed to create audit_logs table: %w", err)
	}

	migrateAddColumn(db, "audit_logs", "asset_name", "TEXT NOT NULL DEFAULT ''")
	migrateAddColumn(db, "audit_logs", "asset_id", "TEXT NOT NULL DEFAULT ''")

	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp)
	`); err != nil {
		return fmt.Errorf("failed to create audit_logs timestamp index: %w", err)
	}

	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_audit_logs_has_risk ON audit_logs(has_risk)
	`); err != nil {
		return fmt.Errorf("failed to create audit_logs has_risk index: %w", err)
	}

	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_audit_logs_asset ON audit_logs(asset_id, timestamp)
	`); err != nil {
		return fmt.Errorf("failed to create audit_logs asset index: %w", err)
	}

	logging.Info("Audit log tables created/verified successfully")
	return nil
}

// createMetricsTables 创建API指标相关的表结构
func createMetricsTables(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS api_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			tool_call_count INTEGER NOT NULL DEFAULT 0,
			model TEXT,
			is_blocked INTEGER NOT NULL DEFAULT 0,
			risk_level TEXT,
			asset_name TEXT,
			asset_id TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("failed to create api_metrics table: %w", err)
	}

	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_api_metrics_timestamp ON api_metrics(timestamp)
	`); err != nil {
		return fmt.Errorf("failed to create api_metrics timestamp index: %w", err)
	}

	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_api_metrics_asset ON api_metrics(asset_name)
	`); err != nil {
		return fmt.Errorf("failed to create api_metrics asset index: %w", err)
	}

	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_api_metrics_asset_id ON api_metrics(asset_id)
	`); err != nil {
		return fmt.Errorf("failed to create api_metrics asset_id index: %w", err)
	}

	logging.Info("Metrics tables created/verified successfully")
	return nil
}

// createAppPermissionsTables 创建应用权限表
func createAppPermissionsTables(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS app_permissions (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			home_dir_authorized INTEGER NOT NULL DEFAULT 0,
			authorized_path TEXT,
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("failed to create app_permissions table: %w", err)
	}

	logging.Info("App permissions tables created/verified successfully")
	return nil
}

// createSecurityEventTables 创建安全事件相关的表结构
func createSecurityEventTables(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS security_events (
			id TEXT PRIMARY KEY,
			timestamp TEXT NOT NULL,
			event_type TEXT NOT NULL,
			action_desc TEXT NOT NULL,
			risk_type TEXT,
			detail TEXT,
			source TEXT NOT NULL,
			asset_name TEXT NOT NULL DEFAULT '',
			asset_id TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		return fmt.Errorf("failed to create security_events table: %w", err)
	}

	migrateAddColumn(db, "security_events", "asset_name", "TEXT NOT NULL DEFAULT ''")
	migrateAddColumn(db, "security_events", "asset_id", "TEXT NOT NULL DEFAULT ''")

	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_security_events_timestamp ON security_events(timestamp)
	`); err != nil {
		return fmt.Errorf("failed to create security_events timestamp index: %w", err)
	}

	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_security_events_event_type ON security_events(event_type)
	`); err != nil {
		return fmt.Errorf("failed to create security_events event_type index: %w", err)
	}

	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_security_events_asset_id ON security_events(asset_id, timestamp)
	`); err != nil {
		return fmt.Errorf("failed to create security_events asset_id index: %w", err)
	}

	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_security_events_asset_name ON security_events(asset_name, timestamp)
	`); err != nil {
		return fmt.Errorf("failed to create security_events asset_name index: %w", err)
	}

	logging.Info("Security event tables created/verified successfully")
	return nil
}

// GetDB 获取全局数据库连接
// 如果数据库未初始化，返回nil
func GetDB() *sql.DB {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	return globalDB
}

// CloseDB 关闭数据库连接
func CloseDB() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if globalDB != nil {
		err := globalDB.Close()
		globalDB = nil
		return err
	}
	return nil
}

// createAssetTables 创建资产扫描相关的表结构
// 使用 IF NOT EXISTS 保证幂等性，与Flutter端创建的表结构完全一致
func createAssetTables(db *sql.DB) error {
	// 扫描记录表
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS scans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TEXT NOT NULL,
			config_found INTEGER NOT NULL,
			config_path TEXT,
			config_json TEXT
		)
	`); err != nil {
		return fmt.Errorf("failed to create scans table: %w", err)
	}

	// 资产表
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS assets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			scan_id INTEGER NOT NULL,
			data TEXT NOT NULL,
			FOREIGN KEY (scan_id) REFERENCES scans (id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("failed to create assets table: %w", err)
	}

	// 风险表
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS risks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			scan_id INTEGER NOT NULL,
			data TEXT NOT NULL,
			FOREIGN KEY (scan_id) REFERENCES scans (id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("failed to create risks table: %w", err)
	}

	// skill_scans table
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS skill_scans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			skill_name TEXT NOT NULL,
			skill_hash TEXT NOT NULL UNIQUE,
			scanned_at TEXT NOT NULL,
			safe INTEGER NOT NULL,
			issues TEXT,
			risk_level TEXT
		)
	`); err != nil {
		return fmt.Errorf("failed to create skill_scans table: %w", err)
	}

	// Add trusted column if it doesn't exist (migration for existing databases)
	if _, err := db.Exec(`ALTER TABLE skill_scans ADD COLUMN trusted INTEGER DEFAULT 0`); err != nil {
		// Column might already exist, ignore error
		if !strings.Contains(err.Error(), "duplicate column") {
			logging.Warning("Failed to add trusted column: %v", err)
		}
	}

	// Add risk_level column if it doesn't exist (migration for existing databases)
	if _, err := db.Exec(`ALTER TABLE skill_scans ADD COLUMN risk_level TEXT`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column") {
			logging.Warning("Failed to add risk_level column: %v", err)
		}
	}

	logging.Info("Asset tables created/verified successfully")
	return nil
}
