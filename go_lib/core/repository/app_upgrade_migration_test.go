package repository

import (
	"database/sql"
	"os"
	"path/filepath"
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
