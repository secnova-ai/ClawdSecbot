package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go_lib/core/logging"
)

const (
	legacyDatabaseVersion = "1.0.0"
	appMetadataVersionKey = "runtime_version"
	versionSourceFresh    = "fresh_install"
	versionSourceFile     = "version_file"
	versionSourceMetadata = "app_metadata"
	versionSourceLegacyDB = "legacy_database"
)

type DBInitSummary struct {
	DBPath          string
	CurrentVersion  string
	PreviousVersion string
	VersionFilePath string
	VersionSource   string
	FreshInstall    bool
	Upgraded        bool
}

type databaseVersionMigration struct {
	fromVersion string
	toVersion   string
	run         func(*sql.DB) error
}

type storedVersionState struct {
	version      string
	source       string
	freshInstall bool
}

var databaseVersionMigrations = []databaseVersionMigration{
	{
		fromVersion: legacyDatabaseVersion,
		toVersion:   "1.0.1",
		run:         migrateDatabaseFrom1_0_0To1_0_1,
	},
	{
		fromVersion: "1.0.1",
		toVersion:   "1.0.2",
		run:         migrateDatabaseFrom1_0_1To1_0_2,
	},
	{
		fromVersion: "1.0.2",
		toVersion:   "1.0.3",
		run:         migrateDatabaseFrom1_0_2To1_0_3,
	},
}

func initializeDatabaseState(db *sql.DB, currentVersion, versionFilePath string) (*DBInitSummary, error) {
	normalizedCurrent := ""
	var err error
	if strings.TrimSpace(currentVersion) != "" {
		normalizedCurrent, err = normalizeVersion(currentVersion)
		if err != nil {
			return nil, fmt.Errorf("invalid current version %q: %w", currentVersion, err)
		}
	}

	state, err := resolveStoredVersionState(db, versionFilePath)
	if err != nil {
		return nil, err
	}

	summary := &DBInitSummary{
		CurrentVersion:  normalizedCurrent,
		PreviousVersion: state.version,
		VersionFilePath: versionFilePath,
		VersionSource:   state.source,
		FreshInstall:    state.freshInstall,
	}

	if normalizedCurrent != "" && state.version != "" {
		switch compareVersions(state.version, normalizedCurrent) {
		case 1:
			return nil, fmt.Errorf(
				"database version %s is newer than current application version %s; downgrade is not supported",
				state.version,
				normalizedCurrent,
			)
		case -1:
			logging.Info(
				"Running startup database migrations: %s -> %s (source=%s)",
				state.version,
				normalizedCurrent,
				state.source,
			)
			if err := runDatabaseVersionMigrations(db, state.version, normalizedCurrent); err != nil {
				return nil, err
			}
			summary.Upgraded = true
		}
	}

	if err := ensureAllTables(db); err != nil {
		return nil, err
	}

	if normalizedCurrent != "" {
		if err := persistDatabaseVersionState(db, versionFilePath, normalizedCurrent); err != nil {
			return nil, err
		}
	}

	return summary, nil
}

func resolveStoredVersionState(db *sql.DB, versionFilePath string) (storedVersionState, error) {
	fileVersion, hasFileVersion, err := readVersionFile(versionFilePath)
	if err != nil {
		return storedVersionState{}, err
	}

	metadataVersion, hasMetadataVersion, err := readVersionFromMetadata(db)
	if err != nil {
		return storedVersionState{}, err
	}

	if hasFileVersion && hasMetadataVersion {
		if compareVersions(fileVersion, metadataVersion) == 0 {
			return storedVersionState{
				version: fileVersion,
				source:  versionSourceFile,
			}, nil
		}

		logging.Warning(
			"Detected mismatched persisted versions: file=%s metadata=%s; using the newer version",
			fileVersion,
			metadataVersion,
		)

		if compareVersions(fileVersion, metadataVersion) > 0 {
			return storedVersionState{
				version: fileVersion,
				source:  versionSourceFile,
			}, nil
		}

		return storedVersionState{
			version: metadataVersion,
			source:  versionSourceMetadata,
		}, nil
	}

	if hasFileVersion {
		return storedVersionState{
			version: fileVersion,
			source:  versionSourceFile,
		}, nil
	}

	if hasMetadataVersion {
		return storedVersionState{
			version: metadataVersion,
			source:  versionSourceMetadata,
		}, nil
	}

	hasUserTables, err := hasApplicationTables(db)
	if err != nil {
		return storedVersionState{}, err
	}
	if hasUserTables {
		logging.Info(
			"No persisted version state found, treating existing database as legacy version %s",
			legacyDatabaseVersion,
		)
		return storedVersionState{
			version: legacyDatabaseVersion,
			source:  versionSourceLegacyDB,
		}, nil
	}

	return storedVersionState{
		source:       versionSourceFresh,
		freshInstall: true,
	}, nil
}

func readVersionFile(versionFilePath string) (string, bool, error) {
	if strings.TrimSpace(versionFilePath) == "" {
		return "", false, nil
	}

	content, err := os.ReadFile(versionFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read version file failed: %w", err)
	}

	version, err := normalizeVersion(string(content))
	if err != nil {
		return "", false, fmt.Errorf("invalid version file content in %s: %w", versionFilePath, err)
	}

	return version, true, nil
}

func readVersionFromMetadata(db *sql.DB) (string, bool, error) {
	exists, err := tableExists(db, "app_metadata")
	if err != nil {
		return "", false, err
	}
	if !exists {
		return "", false, nil
	}

	var version string
	err = db.QueryRow(
		`SELECT value FROM app_metadata WHERE key = ?`,
		appMetadataVersionKey,
	).Scan(&version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read app metadata version failed: %w", err)
	}

	normalizedVersion, err := normalizeVersion(version)
	if err != nil {
		return "", false, fmt.Errorf("invalid app metadata version %q: %w", version, err)
	}

	return normalizedVersion, true, nil
}

func persistDatabaseVersionState(db *sql.DB, versionFilePath, version string) error {
	if err := upsertRuntimeVersionMetadata(db, version); err != nil {
		return err
	}

	if strings.TrimSpace(versionFilePath) == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(versionFilePath), 0755); err != nil {
		return fmt.Errorf("create version file directory failed: %w", err)
	}

	if err := os.WriteFile(versionFilePath, []byte(version+"\n"), 0644); err != nil {
		return fmt.Errorf("write version file failed: %w", err)
	}

	return nil
}

func upsertRuntimeVersionMetadata(db *sql.DB, version string) error {
	updatedAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO app_metadata (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at
	`, appMetadataVersionKey, version, updatedAt); err != nil {
		return fmt.Errorf("upsert app metadata version failed: %w", err)
	}

	return nil
}

func runDatabaseVersionMigrations(db *sql.DB, fromVersion, toVersion string) error {
	plan, err := planDatabaseVersionMigrations(fromVersion, toVersion, databaseVersionMigrations)
	if err != nil {
		return err
	}

	for _, migration := range plan {
		logging.Info(
			"Applying database migration: %s -> %s",
			migration.fromVersion,
			migration.toVersion,
		)
		if err := migration.run(db); err != nil {
			return fmt.Errorf(
				"database migration %s -> %s failed: %w",
				migration.fromVersion,
				migration.toVersion,
				err,
			)
		}
	}

	return nil
}

func planDatabaseVersionMigrations(
	fromVersion,
	toVersion string,
	migrations []databaseVersionMigration,
) ([]databaseVersionMigration, error) {
	normalizedFrom, err := normalizeVersion(fromVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid source version %q: %w", fromVersion, err)
	}

	normalizedTo, err := normalizeVersion(toVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid target version %q: %w", toVersion, err)
	}

	if compareVersions(normalizedFrom, normalizedTo) >= 0 {
		return nil, nil
	}

	byFromVersion := make(map[string]databaseVersionMigration, len(migrations))
	for _, migration := range migrations {
		normalizedMigrationFrom, err := normalizeVersion(migration.fromVersion)
		if err != nil {
			return nil, fmt.Errorf("invalid migration source version %q: %w", migration.fromVersion, err)
		}
		normalizedMigrationTo, err := normalizeVersion(migration.toVersion)
		if err != nil {
			return nil, fmt.Errorf("invalid migration target version %q: %w", migration.toVersion, err)
		}
		migration.fromVersion = normalizedMigrationFrom
		migration.toVersion = normalizedMigrationTo
		byFromVersion[migration.fromVersion] = migration
	}

	current := normalizedFrom
	plan := make([]databaseVersionMigration, 0, 4)
	for compareVersions(current, normalizedTo) < 0 {
		migration, ok := byFromVersion[current]
		if !ok {
			return nil, fmt.Errorf(
				"missing database migration from %s to %s",
				current,
				normalizedTo,
			)
		}
		if compareVersions(migration.toVersion, current) <= 0 {
			return nil, fmt.Errorf(
				"invalid migration step %s -> %s",
				migration.fromVersion,
				migration.toVersion,
			)
		}
		if compareVersions(migration.toVersion, normalizedTo) > 0 {
			return nil, fmt.Errorf(
				"migration step %s -> %s overshoots target version %s",
				migration.fromVersion,
				migration.toVersion,
				normalizedTo,
			)
		}
		plan = append(plan, migration)
		current = migration.toVersion
	}

	if compareVersions(current, normalizedTo) != 0 {
		return nil, fmt.Errorf(
			"incomplete database migration chain: ended at %s, target is %s",
			current,
			normalizedTo,
		)
	}

	return plan, nil
}

func migrateDatabaseFrom1_0_0To1_0_1(db *sql.DB) error {
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		return fmt.Errorf("disable foreign keys failed: %w", err)
	}
	defer func() {
		if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
			logging.Warning("Failed to re-enable foreign keys after migration: %v", err)
		}
	}()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration transaction failed: %w", err)
	}

	if err := dropAllApplicationTables(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration transaction failed: %w", err)
	}

	return nil
}

func dropAllApplicationTables(tx *sql.Tx) error {
	rows, err := tx.Query(`
		SELECT name
		FROM sqlite_master
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
	`)
	if err != nil {
		return fmt.Errorf("query existing tables failed: %w", err)
	}
	defer rows.Close()

	tableNames := make([]string, 0, 16)
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return fmt.Errorf("scan existing table name failed: %w", err)
		}
		tableNames = append(tableNames, tableName)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate existing tables failed: %w", err)
	}

	for _, tableName := range tableNames {
		if _, err := tx.Exec(fmt.Sprintf(
			"DROP TABLE IF EXISTS %s",
			quoteSQLiteIdentifier(tableName),
		)); err != nil {
			return fmt.Errorf("drop table %s failed: %w", tableName, err)
		}
	}

	return nil
}

func hasApplicationTables(db *sql.DB) (bool, error) {
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(1)
		FROM sqlite_master
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
	`).Scan(&count); err != nil {
		return false, fmt.Errorf("count existing tables failed: %w", err)
	}

	return count > 0, nil
}

func tableExists(db *sql.DB, tableName string) (bool, error) {
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(1)
		FROM sqlite_master
		WHERE type = 'table'
		  AND name = ?
	`, tableName).Scan(&count); err != nil {
		return false, fmt.Errorf("check table %s existence failed: %w", tableName, err)
	}

	return count > 0, nil
}

func quoteSQLiteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func normalizeVersion(version string) (string, error) {
	trimmed := strings.TrimSpace(version)
	trimmed = strings.TrimPrefix(trimmed, "v")
	if trimmed == "" {
		return "", fmt.Errorf("version is empty")
	}

	if idx := strings.IndexAny(trimmed, "+-"); idx >= 0 {
		trimmed = trimmed[:idx]
	}

	parts := strings.Split(trimmed, ".")
	normalizedParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return "", fmt.Errorf("version contains an empty segment")
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return "", fmt.Errorf("invalid version segment %q", part)
		}
		normalizedParts = append(normalizedParts, strconv.Itoa(value))
	}

	return strings.Join(normalizedParts, "."), nil
}

func compareVersions(leftVersion, rightVersion string) int {
	leftParts := parseVersionParts(leftVersion)
	rightParts := parseVersionParts(rightVersion)

	maxLen := len(leftParts)
	if len(rightParts) > maxLen {
		maxLen = len(rightParts)
	}

	for idx := 0; idx < maxLen; idx++ {
		leftValue := 0
		rightValue := 0
		if idx < len(leftParts) {
			leftValue = leftParts[idx]
		}
		if idx < len(rightParts) {
			rightValue = rightParts[idx]
		}

		if leftValue > rightValue {
			return 1
		}
		if leftValue < rightValue {
			return -1
		}
	}

	return 0
}

func parseVersionParts(version string) []int {
	parts := strings.Split(version, ".")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			value = 0
		}
		values = append(values, value)
	}
	return values
}

// migrateDatabaseFrom1_0_1To1_0_2 为 audit_logs 表添加 TruthRecord 对齐所需的新列。
func migrateDatabaseFrom1_0_1To1_0_2(db *sql.DB) error {
	logging.Info("Migrating audit_logs table: adding TruthRecord columns")
	ensureAuditLogTruthRecordColumns(db)
	return nil
}

// migrateDatabaseFrom1_0_2To1_0_3 rebuilds audit and risk-detection tables.
// Decision:
//  1. audit_logs: schema and semantics changed, keep no legacy rows.
//  2. risk detection (scans/assets/risks/skill_scans): no stable 1:1 mapping
//     for the new runtime semantics, so destructive rebuild is safer.
func migrateDatabaseFrom1_0_2To1_0_3(db *sql.DB) error {
	logging.Info("Migrating database 1.0.2 -> 1.0.3: rebuilding audit and risk tables")

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration transaction failed: %w", err)
	}
	defer tx.Rollback()

	// Drop child tables before parent tables to avoid FK-order issues.
	tablesToDrop := []string{
		"audit_logs",
		"assets",
		"risks",
		"scans",
		"skill_scans",
	}
	for _, tableName := range tablesToDrop {
		if _, err := tx.Exec(fmt.Sprintf(
			"DROP TABLE IF EXISTS %s",
			quoteSQLiteIdentifier(tableName),
		)); err != nil {
			return fmt.Errorf("drop table %s failed: %w", tableName, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration transaction failed: %w", err)
	}

	return nil
}
