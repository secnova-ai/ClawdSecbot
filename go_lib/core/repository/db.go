// Package repository provides the database access layer.
// It manages SQLite connections and schema initialization.
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
	// globalDB is the shared database connection.
	globalDB *sql.DB
	// globalDBPath is current database file path.
	globalDBPath string
	// dbMutex guards database initialization and shutdown.
	dbMutex sync.RWMutex
)

// InitDB initializes the database connection.
// dbPath must be the full SQLite database path shared with Flutter.
// The method is idempotent and reopens the connection on repeated calls.
func InitDB(dbPath string) error {
	_, err := InitDBWithVersion(dbPath, "", "")
	return err
}

// InitDBWithVersion initializes the database and optionally runs versioned
// startup migrations when currentVersion is provided.
func InitDBWithVersion(dbPath, currentVersion, versionFilePath string) (*DBInitSummary, error) {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	// Close the previous shared connection before reopening.
	if globalDB != nil {
		globalDB.Close()
		globalDB = nil
	}
	globalDBPath = dbPath

	logging.Info("Initializing database connection: %s", dbPath)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		logging.Error("Failed to open database: %v", err)
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for concurrent access with the Flutter side.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		logging.Warning("Failed to enable WAL mode: %v", err)
	}

	// Set busy timeout to reduce SQLITE_BUSY on concurrent access.
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		logging.Warning("Failed to set busy_timeout: %v", err)
	}

	// Enable foreign key constraints.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		logging.Warning("Failed to enable foreign keys: %v", err)
	}

	// Verify the connection is usable before exposing it globally.
	if err := db.Ping(); err != nil {
		db.Close()
		logging.Error("Database ping failed: %v", err)
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	globalDB = db

	summary, err := initializeDatabaseState(db, currentVersion, versionFilePath)
	if err != nil {
		globalDB.Close()
		globalDB = nil
		logging.Error("Failed to initialize database state: %v", err)
		return nil, err
	}
	summary.DBPath = dbPath

	logging.Info("Database initialized successfully")
	return summary, nil
}

// createProtectionTables creates protection-related tables.
func createProtectionTables(db *sql.DB) error {
	// protection_state is globally unique with id=1.
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

	// protection_config is unique by asset instance.
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

	// protection_statistics is unique by asset instance.
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

	// shepherd_rules are isolated by asset_id.
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

// createAuditLogTables creates audit log tables.
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
			duration_ms INTEGER NOT NULL DEFAULT 0,
			phase TEXT DEFAULT 'completed',
			primary_content TEXT,
			primary_content_type TEXT DEFAULT 'unavailable',
			finish_reason TEXT,
			message_count INTEGER DEFAULT 0,
			messages TEXT,
			completed_at TEXT
		)
	`); err != nil {
		return fmt.Errorf("failed to create audit_logs table: %w", err)
	}

	// 为已有数据库安全添加 TruthRecord 对齐所需的新列
	ensureAuditLogTruthRecordColumns(db)

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

// ensureAuditLogTruthRecordColumns 安全地为 audit_logs 表添加 TruthRecord 对齐所需的新列。
// 对已有数据库执行 ALTER TABLE ADD COLUMN, 忽略列已存在的错误以实现幂等。
func ensureAuditLogTruthRecordColumns(db *sql.DB) {
	columns := []struct {
		name string
		def  string
	}{
		{"phase", "TEXT DEFAULT 'completed'"},
		{"primary_content", "TEXT"},
		{"primary_content_type", "TEXT DEFAULT 'unavailable'"},
		{"finish_reason", "TEXT"},
		{"message_count", "INTEGER DEFAULT 0"},
		{"messages", "TEXT"},
		{"completed_at", "TEXT"},
	}
	for _, col := range columns {
		addColumnSafe(db, "audit_logs", col.name, col.def)
	}
}

// addColumnSafe 安全地向表中添加列, 忽略 "duplicate column name" 错误。
func addColumnSafe(db *sql.DB, table, column, colDef string) {
	stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colDef)
	_, err := db.Exec(stmt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return
		}
		logging.Warning("Failed to add column %s to %s: %v", column, table, err)
	}
}

// createMetricsTables creates API metrics tables.
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

// createAppPermissionsTables creates application permission tables.
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

// createSecurityEventTables creates security event tables.
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
			asset_id TEXT NOT NULL DEFAULT '',
			request_id TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		return fmt.Errorf("failed to create security_events table: %w", err)
	}

	// Migrate: add request_id column for existing databases
	if _, err := db.Exec(`ALTER TABLE security_events ADD COLUMN request_id TEXT NOT NULL DEFAULT ''`); err != nil {
		// Column already exists — safe to ignore
	}

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

	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_security_events_request_id ON security_events(request_id)
	`); err != nil {
		return fmt.Errorf("failed to create security_events request_id index: %w", err)
	}

	logging.Info("Security event tables created/verified successfully")
	return nil
}

// GetDB returns the shared database connection.
// It returns nil when the database has not been initialized.
func GetDB() *sql.DB {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	return globalDB
}

// CloseDB closes the shared database connection.
func CloseDB() error {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if globalDB != nil {
		err := globalDB.Close()
		globalDB = nil
		globalDBPath = ""
		return err
	}
	globalDBPath = ""
	return nil
}

// createAssetTables creates asset scanning tables.
// IF NOT EXISTS keeps it idempotent and aligned with the Flutter-side schema.
// GetDBPath 获取当前数据库文件路径。
func GetDBPath() string {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	return globalDBPath
}

// createAssetTables creates asset scanning tables.
// IF NOT EXISTS keeps it idempotent and aligned with the Flutter-side schema.
func createAssetTables(db *sql.DB) error {
	// scans stores each scan session.
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

	// assets stores scanned asset payloads.
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

	// risks stores scanned risk payloads.
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

	// skill_scans stores skill scan results.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS skill_scans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			skill_name TEXT NOT NULL,
			skill_hash TEXT NOT NULL UNIQUE,
			skill_path TEXT NOT NULL DEFAULT '',
			source_plugin TEXT NOT NULL DEFAULT '',
			asset_id TEXT NOT NULL DEFAULT '',
			scanned_at TEXT NOT NULL,
			safe INTEGER NOT NULL,
			issues TEXT,
			trusted INTEGER NOT NULL DEFAULT 0,
			risk_level TEXT,
			deleted_at TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		return fmt.Errorf("failed to create skill_scans table: %w", err)
	}

	// Add trusted column if it doesn't exist (migration for existing databases)
	addColumnSafe(db, "skill_scans", "trusted", "INTEGER DEFAULT 0")
	addColumnSafe(db, "skill_scans", "risk_level", "TEXT")
	addColumnSafe(db, "skill_scans", "skill_path", "TEXT NOT NULL DEFAULT ''")
	addColumnSafe(db, "skill_scans", "source_plugin", "TEXT NOT NULL DEFAULT ''")
	addColumnSafe(db, "skill_scans", "asset_id", "TEXT NOT NULL DEFAULT ''")
	addColumnSafe(db, "skill_scans", "deleted_at", "TEXT NOT NULL DEFAULT ''")

	logging.Info("Asset tables created/verified successfully")
	return nil
}

func ensureAllTables(db *sql.DB) error {
	if err := createAssetTables(db); err != nil {
		return fmt.Errorf("failed to create asset tables: %w", err)
	}

	if err := CreateSecurityModelConfigTable(db); err != nil {
		return fmt.Errorf("failed to create security model config table: %w", err)
	}

	if err := createProtectionTables(db); err != nil {
		return fmt.Errorf("failed to create protection tables: %w", err)
	}

	if err := createAuditLogTables(db); err != nil {
		return fmt.Errorf("failed to create audit log tables: %w", err)
	}

	if err := createMetricsTables(db); err != nil {
		return fmt.Errorf("failed to create metrics tables: %w", err)
	}

	if err := createAppPermissionsTables(db); err != nil {
		return fmt.Errorf("failed to create app permissions tables: %w", err)
	}

	if err := CreateAppSettingsTable(db); err != nil {
		return fmt.Errorf("failed to create app settings table: %w", err)
	}

	if err := createSecurityEventTables(db); err != nil {
		return fmt.Errorf("failed to create security event tables: %w", err)
	}

	if err := createAppMetadataTable(db); err != nil {
		return fmt.Errorf("failed to create app metadata table: %w", err)
	}

	return nil
}

func createAppMetadataTable(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS app_metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		return err
	}

	logging.Info("App metadata table created/verified successfully")
	return nil
}
