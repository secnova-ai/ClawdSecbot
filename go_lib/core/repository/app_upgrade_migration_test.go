package repository

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestPlanDatabaseVersionMigrations_MultiStep(t *testing.T) {
	migrations := []databaseVersionMigration{
		{fromVersion: "1.0.0", toVersion: "1.0.1"},
		{fromVersion: "1.0.1", toVersion: "1.0.2"},
	}

	plan, err := planDatabaseVersionMigrations("1.0.0", "1.0.2", migrations)
	if err != nil {
		t.Fatalf("Expected migration plan to succeed, got error: %v", err)
	}

	if len(plan) != 2 {
		t.Fatalf("Expected 2 migration steps, got %d", len(plan))
	}
	if plan[0].fromVersion != "1.0.0" || plan[0].toVersion != "1.0.1" {
		t.Fatalf("Unexpected first migration step: %+v", plan[0])
	}
	if plan[1].fromVersion != "1.0.1" || plan[1].toVersion != "1.0.2" {
		t.Fatalf("Unexpected second migration step: %+v", plan[1])
	}
}

func TestPlanDatabaseVersionMigrations_GlobalRegistryTo1_0_3(t *testing.T) {
	plan, err := planDatabaseVersionMigrations("1.0.0", "1.0.3", databaseVersionMigrations)
	if err != nil {
		t.Fatalf("Expected global migration plan to succeed, got error: %v", err)
	}

	if len(plan) != 3 {
		t.Fatalf("Expected 3 migration steps, got %d", len(plan))
	}
	if plan[2].fromVersion != "1.0.2" || plan[2].toVersion != "1.0.3" {
		t.Fatalf("Expected final step 1.0.2 -> 1.0.3, got %+v", plan[2])
	}
}

func TestPlanDatabaseVersionMigrations_GlobalRegistryTo1_0_4(t *testing.T) {
	plan, err := planDatabaseVersionMigrations("1.0.0", "1.0.4", databaseVersionMigrations)
	if err != nil {
		t.Fatalf("Expected global migration plan to succeed, got error: %v", err)
	}

	if len(plan) != 4 {
		t.Fatalf("Expected 4 migration steps, got %d", len(plan))
	}
	if plan[3].fromVersion != "1.0.3" || plan[3].toVersion != "1.0.4" {
		t.Fatalf("Expected final step 1.0.3 -> 1.0.4, got %+v", plan[3])
	}
}

func TestInitDBWithVersion_FreshInstallWritesVersionState(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "fresh.db")
	versionFilePath := filepath.Join(tempDir, "bot_sec_manager.version")

	summary, err := InitDBWithVersion(dbPath, "1.0.1", versionFilePath)
	if err != nil {
		t.Fatalf("InitDBWithVersion failed: %v", err)
	}
	defer CloseDB()

	if !summary.FreshInstall {
		t.Fatalf("Expected fresh install summary, got %+v", summary)
	}
	if summary.Upgraded {
		t.Fatalf("Fresh install should not be marked upgraded: %+v", summary)
	}
	if summary.VersionSource != versionSourceFresh {
		t.Fatalf("Expected version source %q, got %q", versionSourceFresh, summary.VersionSource)
	}

	assertVersionFileContent(t, versionFilePath, "1.0.1\n")
	assertMetadataVersion(t, GetDB(), "1.0.1")

	exists, err := tableExists(GetDB(), "scans")
	if err != nil {
		t.Fatalf("Failed to check scans table existence: %v", err)
	}
	if !exists {
		t.Fatal("Expected scans table to exist after initialization")
	}
}

func TestInitDBWithVersion_LegacyDatabaseUpgradesFrom1_0_0(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "legacy.db")
	versionFilePath := filepath.Join(tempDir, "bot_sec_manager.version")

	prepareSQLiteFile(t, dbPath, func(db *sql.DB) {
		if _, err := db.Exec(`CREATE TABLE legacy_only (id INTEGER PRIMARY KEY, value TEXT)`); err != nil {
			t.Fatalf("Failed to create legacy table: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO legacy_only (value) VALUES ('legacy-data')`); err != nil {
			t.Fatalf("Failed to seed legacy table: %v", err)
		}
	})

	summary, err := InitDBWithVersion(dbPath, "1.0.1", versionFilePath)
	if err != nil {
		t.Fatalf("InitDBWithVersion failed: %v", err)
	}
	defer CloseDB()

	if !summary.Upgraded {
		t.Fatalf("Expected legacy database to be upgraded, got %+v", summary)
	}
	if summary.PreviousVersion != legacyDatabaseVersion {
		t.Fatalf("Expected previous version %s, got %s", legacyDatabaseVersion, summary.PreviousVersion)
	}
	if summary.VersionSource != versionSourceLegacyDB {
		t.Fatalf("Expected source %q, got %q", versionSourceLegacyDB, summary.VersionSource)
	}

	legacyTableExists, err := tableExists(GetDB(), "legacy_only")
	if err != nil {
		t.Fatalf("Failed to check legacy table existence: %v", err)
	}
	if legacyTableExists {
		t.Fatal("Expected legacy table to be dropped during migration")
	}

	scansTableExists, err := tableExists(GetDB(), "scans")
	if err != nil {
		t.Fatalf("Failed to check scans table existence: %v", err)
	}
	if !scansTableExists {
		t.Fatal("Expected scans table to exist after migration")
	}

	assertVersionFileContent(t, versionFilePath, "1.0.1\n")
	assertMetadataVersion(t, GetDB(), "1.0.1")
}

func TestInitDBWithVersion_UsesMetadataWhenVersionFileMissing(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "metadata.db")
	versionFilePath := filepath.Join(tempDir, "bot_sec_manager.version")

	prepareSQLiteFile(t, dbPath, func(db *sql.DB) {
		if _, err := db.Exec(`
			CREATE TABLE app_metadata (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)
		`); err != nil {
			t.Fatalf("Failed to create app_metadata table: %v", err)
		}
		if _, err := db.Exec(`
			INSERT INTO app_metadata (key, value, updated_at)
			VALUES (?, ?, ?)
		`, appMetadataVersionKey, "1.0.1", "2026-03-28T00:00:00Z"); err != nil {
			t.Fatalf("Failed to seed app metadata version: %v", err)
		}
	})

	summary, err := InitDBWithVersion(dbPath, "1.0.1", versionFilePath)
	if err != nil {
		t.Fatalf("InitDBWithVersion failed: %v", err)
	}
	defer CloseDB()

	if summary.VersionSource != versionSourceMetadata {
		t.Fatalf("Expected metadata source, got %+v", summary)
	}
	if summary.Upgraded {
		t.Fatalf("Expected no upgrade when metadata version matches current version: %+v", summary)
	}
	if summary.PreviousVersion != "1.0.1" {
		t.Fatalf("Expected previous version 1.0.1, got %s", summary.PreviousVersion)
	}

	assertVersionFileContent(t, versionFilePath, "1.0.1\n")
	assertMetadataVersion(t, GetDB(), "1.0.1")
}

func TestInitDBWithVersion_RejectsDowngrade(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "downgrade.db")
	versionFilePath := filepath.Join(tempDir, "bot_sec_manager.version")

	if err := os.WriteFile(versionFilePath, []byte("1.0.2\n"), 0644); err != nil {
		t.Fatalf("Failed to seed version file: %v", err)
	}

	_, err := InitDBWithVersion(dbPath, "1.0.1", versionFilePath)
	defer CloseDB()
	if err == nil {
		t.Fatal("Expected downgrade initialization to fail")
	}
}

func TestInitDBWithVersion_UpgradesFrom1_0_2To1_0_3_RebuildsAuditAndRiskTables(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "upgrade_102_to_103.db")
	versionFilePath := filepath.Join(tempDir, "bot_sec_manager.version")

	if err := os.WriteFile(versionFilePath, []byte("1.0.2\n"), 0644); err != nil {
		t.Fatalf("Failed to seed version file: %v", err)
	}

	prepareSQLiteFile(t, dbPath, func(db *sql.DB) {
		if err := createProtectionTables(db); err != nil {
			t.Fatalf("Failed to create protection tables: %v", err)
		}
		if err := createMetricsTables(db); err != nil {
			t.Fatalf("Failed to create metrics tables: %v", err)
		}
		if err := createSecurityEventTables(db); err != nil {
			t.Fatalf("Failed to create security event tables: %v", err)
		}

		if _, err := db.Exec(`CREATE TABLE audit_logs (id TEXT PRIMARY KEY, legacy_col TEXT)`); err != nil {
			t.Fatalf("Failed to create legacy audit_logs: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO audit_logs (id, legacy_col) VALUES ('audit_1', 'legacy')`); err != nil {
			t.Fatalf("Failed to seed legacy audit_logs: %v", err)
		}

		if _, err := db.Exec(`CREATE TABLE scans (id INTEGER PRIMARY KEY AUTOINCREMENT, legacy_col TEXT)`); err != nil {
			t.Fatalf("Failed to create legacy scans: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO scans (legacy_col) VALUES ('legacy')`); err != nil {
			t.Fatalf("Failed to seed legacy scans: %v", err)
		}

		if _, err := db.Exec(`CREATE TABLE assets (id INTEGER PRIMARY KEY AUTOINCREMENT, scan_id INTEGER, data TEXT)`); err != nil {
			t.Fatalf("Failed to create legacy assets: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO assets (scan_id, data) VALUES (1, '{"legacy":true}')`); err != nil {
			t.Fatalf("Failed to seed legacy assets: %v", err)
		}

		if _, err := db.Exec(`CREATE TABLE risks (id INTEGER PRIMARY KEY AUTOINCREMENT, scan_id INTEGER, data TEXT)`); err != nil {
			t.Fatalf("Failed to create legacy risks: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO risks (scan_id, data) VALUES (1, '{"legacy":true}')`); err != nil {
			t.Fatalf("Failed to seed legacy risks: %v", err)
		}

		if _, err := db.Exec(`CREATE TABLE skill_scans (id INTEGER PRIMARY KEY AUTOINCREMENT, skill_name TEXT)`); err != nil {
			t.Fatalf("Failed to create legacy skill_scans: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO skill_scans (skill_name) VALUES ('legacy_skill')`); err != nil {
			t.Fatalf("Failed to seed legacy skill_scans: %v", err)
		}

		assetName := "Openclaw"
		configPath := "/Users/test/.openclaw/config.json"
		oldAssetID := legacyComputeAssetID(assetName, configPath, []int{3000, 13436}, []string{
			"/usr/local/bin/openclaw",
			"/Applications/Openclaw.app",
		})

		if _, err := db.Exec(`
			INSERT INTO protection_config (
				asset_name, asset_id, inherits_default_policy, enabled, audit_only, sandbox_enabled,
				gateway_binary_path, gateway_config_path, custom_security_prompt,
				single_session_token_limit, daily_token_limit,
				path_permission, network_permission, shell_permission, bot_model_config,
				created_at, updated_at
			) VALUES (?, ?, 0, 1, 0, 1, ?, ?, '', 100000, 5000000, '{}', '{}', '{}', '{}', ?, ?)
		`,
			assetName,
			oldAssetID,
			"/usr/local/bin/openclaw",
			configPath,
			"2026-04-01T10:00:00Z",
			"2026-04-01T10:05:00Z",
		); err != nil {
			t.Fatalf("Failed to seed protection_config: %v", err)
		}

		if _, err := db.Exec(`
			INSERT INTO protection_statistics (
				asset_name, asset_id, analysis_count, message_count, warning_count, blocked_count,
				total_tokens, total_prompt_tokens, total_completion_tokens, total_tool_calls,
				request_count, audit_tokens, audit_prompt_tokens, audit_completion_tokens, updated_at
			) VALUES (?, ?, 11, 22, 3, 4, 100, 60, 40, 5, 6, 7, 4, 3, ?)
		`, assetName, oldAssetID, "2026-04-01T10:06:00Z"); err != nil {
			t.Fatalf("Failed to seed protection_statistics: %v", err)
		}

		if _, err := db.Exec(`
			INSERT INTO shepherd_rules (asset_id, asset_name, sensitive_actions, updated_at)
			VALUES (?, ?, '["delete","rm"]', ?)
		`, oldAssetID, assetName, "2026-04-01T10:07:00Z"); err != nil {
			t.Fatalf("Failed to seed shepherd_rules: %v", err)
		}

		if _, err := db.Exec(`
			INSERT INTO api_metrics (
				timestamp, prompt_tokens, completion_tokens, total_tokens, tool_call_count,
				model, is_blocked, risk_level, asset_name, asset_id
			) VALUES (?, 1, 2, 3, 4, 'gpt-4.1', 0, '', ?, ?)
		`, "2026-04-01T10:08:00Z", assetName, oldAssetID); err != nil {
			t.Fatalf("Failed to seed api_metrics: %v", err)
		}

		if _, err := db.Exec(`
			INSERT INTO security_events (
				id, timestamp, event_type, action_desc, risk_type, detail, source, asset_name, asset_id, request_id
			) VALUES ('evt_1', ?, 'blocked', 'blocked command', 'QUOTA', 'legacy', 'react_agent', ?, ?, 'req_1')
		`, "2026-04-01T10:09:00Z", assetName, oldAssetID); err != nil {
			t.Fatalf("Failed to seed security_events: %v", err)
		}
	})

	summary, err := InitDBWithVersion(dbPath, "1.0.3", versionFilePath)
	if err != nil {
		t.Fatalf("InitDBWithVersion failed: %v", err)
	}
	defer CloseDB()

	if !summary.Upgraded {
		t.Fatalf("Expected database upgraded=true, got %+v", summary)
	}
	if summary.PreviousVersion != "1.0.2" {
		t.Fatalf("Expected previous version 1.0.2, got %s", summary.PreviousVersion)
	}

	assertVersionFileContent(t, versionFilePath, "1.0.3\n")
	assertMetadataVersion(t, GetDB(), "1.0.3")

	assertTableExists(t, GetDB(), "audit_logs")
	assertTableExists(t, GetDB(), "scans")
	assertTableExists(t, GetDB(), "assets")
	assertTableExists(t, GetDB(), "risks")
	assertTableExists(t, GetDB(), "skill_scans")

	assertTableRowCount(t, GetDB(), "audit_logs", 0)
	assertTableRowCount(t, GetDB(), "scans", 0)
	assertTableRowCount(t, GetDB(), "assets", 0)
	assertTableRowCount(t, GetDB(), "risks", 0)
	assertTableRowCount(t, GetDB(), "skill_scans", 0)

	expectedAssetID := computeStableAssetIDForMigration("Openclaw", "/Users/test/.openclaw/config.json")
	legacyAssetID := legacyComputeAssetID("Openclaw", "/Users/test/.openclaw/config.json", []int{3000, 13436}, []string{
		"/usr/local/bin/openclaw",
		"/Applications/Openclaw.app",
	})
	assertTableRowCountWithPredicate(t, GetDB(), "protection_config", "asset_id = '"+expectedAssetID+"'", 1)
	assertTableRowCountWithPredicate(t, GetDB(), "protection_statistics", "asset_id = '"+expectedAssetID+"'", 1)
	assertTableRowCountWithPredicate(t, GetDB(), "shepherd_rules", "asset_id = '"+expectedAssetID+"'", 1)
	assertTableRowCountWithPredicate(t, GetDB(), "api_metrics", "asset_id = '"+expectedAssetID+"'", 1)
	assertTableRowCountWithPredicate(t, GetDB(), "security_events", "asset_id = '"+expectedAssetID+"'", 1)
	assertTableRowCountWithPredicate(t, GetDB(), "protection_config", "asset_id = '"+legacyAssetID+"'", 0)
}

func TestInitDBWithVersion_UpgradesFrom1_0_3To1_0_4_AddsInstructionChainColumns(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "chain_columns.db")
	versionFilePath := filepath.Join(tempDir, "bot_sec_manager.version")

	prepareSQLiteFile(t, dbPath, func(db *sql.DB) {
		if _, err := db.Exec(`
			CREATE TABLE app_metadata (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)
		`); err != nil {
			t.Fatalf("Failed to create app_metadata: %v", err)
		}
		if _, err := db.Exec(`
			INSERT INTO app_metadata (key, value, updated_at)
			VALUES (?, '1.0.3', '2026-04-25T00:00:00Z')
		`, appMetadataVersionKey); err != nil {
			t.Fatalf("Failed to seed metadata version: %v", err)
		}
		if _, err := db.Exec(`
			CREATE TABLE audit_logs (
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
			t.Fatalf("Failed to create audit_logs: %v", err)
		}
		if _, err := db.Exec(`
			INSERT INTO audit_logs (id, timestamp, request_id, action, duration_ms)
			VALUES ('audit_1', '2026-04-25T00:00:00Z', 'req_1', 'ALLOW', 1)
		`); err != nil {
			t.Fatalf("Failed to seed audit_logs: %v", err)
		}
		if _, err := db.Exec(`
			CREATE TABLE security_events (
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
			t.Fatalf("Failed to create security_events: %v", err)
		}
		if _, err := db.Exec(`
			INSERT INTO security_events (id, timestamp, event_type, action_desc, source, request_id)
			VALUES ('evt_1', '2026-04-25T00:00:00Z', 'blocked', 'blocked', 'react_agent', 'req_1')
		`); err != nil {
			t.Fatalf("Failed to seed security_events: %v", err)
		}
		if _, err := db.Exec(`
			CREATE TABLE protection_config (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				asset_name TEXT NOT NULL,
				asset_id TEXT NOT NULL,
				inherits_default_policy INTEGER NOT NULL DEFAULT 0,
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
			t.Fatalf("Failed to create protection_config: %v", err)
		}
		if _, err := db.Exec(`
			INSERT INTO protection_config (asset_name, asset_id, created_at, updated_at)
			VALUES ('openclaw', 'openclaw:a1', '2026-04-25T00:00:00Z', '2026-04-25T00:00:00Z')
		`); err != nil {
			t.Fatalf("Failed to seed protection_config: %v", err)
		}
	})

	summary, err := InitDBWithVersion(dbPath, "1.0.4", versionFilePath)
	if err != nil {
		t.Fatalf("InitDBWithVersion failed: %v", err)
	}
	defer CloseDB()

	if !summary.Upgraded {
		t.Fatalf("Expected 1.0.3 database to upgrade, got %+v", summary)
	}
	assertVersionFileContent(t, versionFilePath, "1.0.4\n")
	assertMetadataVersion(t, GetDB(), "1.0.4")
	assertColumnExists(t, GetDB(), "audit_logs", "instruction_chain_id")
	assertColumnExists(t, GetDB(), "security_events", "instruction_chain_id")
	assertColumnExists(t, GetDB(), "protection_config", "user_input_detection_enabled")
	assertTableRowCount(t, GetDB(), "audit_logs", 1)
	assertTableRowCount(t, GetDB(), "security_events", 1)
	var enabled int
	if err := GetDB().QueryRow(`SELECT user_input_detection_enabled FROM protection_config WHERE asset_id = 'openclaw:a1'`).Scan(&enabled); err != nil {
		t.Fatalf("Failed to query user_input_detection_enabled: %v", err)
	}
	if enabled != 1 {
		t.Fatalf("Expected existing configs to default user_input_detection_enabled=1, got %d", enabled)
	}
}

func prepareSQLiteFile(t *testing.T, dbPath string, setup func(db *sql.DB)) {
	t.Helper()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open sqlite file %s: %v", dbPath, err)
	}
	defer db.Close()

	setup(db)
}

func assertVersionFileContent(t *testing.T, path, expected string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read version file %s: %v", path, err)
	}
	if string(content) != expected {
		t.Fatalf("Expected version file content %q, got %q", expected, string(content))
	}
}

func assertMetadataVersion(t *testing.T, db *sql.DB, expected string) {
	t.Helper()

	version, exists, err := readVersionFromMetadata(db)
	if err != nil {
		t.Fatalf("Failed to read metadata version: %v", err)
	}
	if !exists {
		t.Fatal("Expected metadata version to exist")
	}
	if version != expected {
		t.Fatalf("Expected metadata version %q, got %q", expected, version)
	}
}

func assertTableExists(t *testing.T, db *sql.DB, tableName string) {
	t.Helper()

	exists, err := tableExists(db, tableName)
	if err != nil {
		t.Fatalf("Failed to check table %s existence: %v", tableName, err)
	}
	if !exists {
		t.Fatalf("Expected table %s to exist", tableName)
	}
}

func assertColumnExists(t *testing.T, db *sql.DB, tableName, columnName string) {
	t.Helper()

	rows, err := db.Query("PRAGMA table_info(" + quoteSQLiteIdentifier(tableName) + ")")
	if err != nil {
		t.Fatalf("Failed to inspect table %s: %v", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("Failed to scan table info for %s: %v", tableName, err)
		}
		if name == columnName {
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("Failed to iterate table info for %s: %v", tableName, err)
	}
	t.Fatalf("Expected column %s.%s to exist", tableName, columnName)
}

func assertTableRowCount(t *testing.T, db *sql.DB, tableName string, expected int) {
	t.Helper()

	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM ` + quoteSQLiteIdentifier(tableName)).Scan(&count); err != nil {
		t.Fatalf("Failed to count rows in %s: %v", tableName, err)
	}
	if count != expected {
		t.Fatalf("Expected %d rows in %s, got %d", expected, tableName, count)
	}
}

func assertTableRowCountWithPredicate(t *testing.T, db *sql.DB, tableName, predicate string, expected int) {
	t.Helper()

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(1) FROM ` + quoteSQLiteIdentifier(tableName) + ` WHERE ` + predicate,
	).Scan(&count); err != nil {
		t.Fatalf("Failed to count rows in %s with predicate %q: %v", tableName, predicate, err)
	}
	if count != expected {
		t.Fatalf("Expected %d rows in %s with predicate %q, got %d", expected, tableName, predicate, count)
	}
}

func legacyComputeAssetID(name, configPath string, ports []int, processPaths []string) string {
	nameLower := strings.ToLower(strings.TrimSpace(name))
	parts := []string{"name=" + nameLower}

	configPath = strings.TrimSpace(configPath)
	if configPath != "" {
		parts = append(parts, "config="+configPath)
	}

	if len(ports) > 0 {
		sortedPorts := append([]int(nil), ports...)
		sort.Ints(sortedPorts)
		portStrings := make([]string, 0, len(sortedPorts))
		for _, port := range sortedPorts {
			portStrings = append(portStrings, fmt.Sprintf("%d", port))
		}
		parts = append(parts, "ports="+strings.Join(portStrings, ","))
	}

	if len(processPaths) > 0 {
		sortedPaths := append([]string(nil), processPaths...)
		sort.Strings(sortedPaths)
		parts = append(parts, "paths="+strings.Join(sortedPaths, ","))
	}

	canonical := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("%s:%x", nameLower, hash[:6])
}
