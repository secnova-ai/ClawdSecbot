package repository

import (
	"crypto/sha256"
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
	{
		fromVersion: "1.0.3",
		toVersion:   "1.0.4",
		run:         migrateDatabaseFrom1_0_3To1_0_4,
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
	logging.Info("Migrating database 1.0.2 -> 1.0.3: rebuilding audit/risk tables and remapping asset_id")

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration transaction failed: %w", err)
	}
	defer tx.Rollback()

	if err := migrateAssetIDFingerprintFrom1_0_2To1_0_3(tx); err != nil {
		return fmt.Errorf("migrate asset_id fingerprint failed: %w", err)
	}

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

func migrateDatabaseFrom1_0_3To1_0_4(db *sql.DB) error {
	logging.Info("Migrating database 1.0.3 -> 1.0.4: adding ShepherdGate instruction chain fields and user input detection switch")
	ensureAuditLogTruthRecordColumns(db)
	addColumnSafe(db, "security_events", "instruction_chain_id", "TEXT NOT NULL DEFAULT ''")
	addColumnSafe(db, "protection_config", "user_input_detection_enabled", "INTEGER NOT NULL DEFAULT 1")
	return nil
}

type protectionConfigAssetIDMigration struct {
	rowID      int64
	oldAssetID string
	newAssetID string
}

type assetIDMapping struct {
	oldAssetID string
	newAssetID string
}

func migrateAssetIDFingerprintFrom1_0_2To1_0_3(tx *sql.Tx) error {
	migrations, err := collectProtectionConfigAssetIDMigrations(tx)
	if err != nil {
		return err
	}
	if len(migrations) == 0 {
		return nil
	}

	if err := rewriteProtectionConfigAssetIDs(tx, migrations); err != nil {
		return err
	}

	mappings := dedupeAssetIDMappings(migrations)
	if err := rewriteProtectionStatisticsAssetIDs(tx, mappings); err != nil {
		return err
	}
	if err := rewriteShepherdRulesAssetIDs(tx, mappings); err != nil {
		return err
	}
	if err := rewriteTableAssetIDs(tx, "api_metrics", mappings); err != nil {
		return err
	}
	if err := rewriteTableAssetIDs(tx, "security_events", mappings); err != nil {
		return err
	}

	logging.Info("Migrated %d asset_id rows from runtime fingerprint to stable config_path fingerprint", len(migrations))
	return nil
}

func collectProtectionConfigAssetIDMigrations(tx *sql.Tx) ([]protectionConfigAssetIDMigration, error) {
	exists, err := tableExistsTx(tx, "protection_config")
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	rows, err := tx.Query(`
		SELECT id, asset_id, asset_name, COALESCE(gateway_config_path, '')
		FROM protection_config
		WHERE asset_id <> ''
		  AND asset_id <> ?
	`, DefaultProtectionPolicyAssetID)
	if err != nil {
		return nil, fmt.Errorf("query protection_config for asset_id migration failed: %w", err)
	}
	defer rows.Close()

	migrations := make([]protectionConfigAssetIDMigration, 0, 8)
	for rows.Next() {
		var (
			rowID       int64
			oldAssetID  string
			assetName   string
			gatewayPath string
		)
		if err := rows.Scan(&rowID, &oldAssetID, &assetName, &gatewayPath); err != nil {
			return nil, fmt.Errorf("scan protection_config migration row failed: %w", err)
		}

		newAssetID := computeStableAssetIDForMigration(assetName, gatewayPath)
		if newAssetID == "" || newAssetID == oldAssetID {
			continue
		}

		migrations = append(migrations, protectionConfigAssetIDMigration{
			rowID:      rowID,
			oldAssetID: oldAssetID,
			newAssetID: newAssetID,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate protection_config migration rows failed: %w", err)
	}

	return migrations, nil
}

func computeStableAssetIDForMigration(name, configPath string) string {
	nameLower := strings.ToLower(strings.TrimSpace(name))
	if nameLower == "" {
		return ""
	}

	parts := []string{"name=" + nameLower}
	configPath = strings.TrimSpace(configPath)
	if configPath != "" {
		parts = append(parts, "config="+configPath)
	}

	canonical := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("%s:%x", nameLower, hash[:6])
}

func rewriteProtectionConfigAssetIDs(tx *sql.Tx, migrations []protectionConfigAssetIDMigration) error {
	for _, migration := range migrations {
		_, err := tx.Exec(`
			UPDATE protection_config
			SET asset_id = ?
			WHERE id = ? AND asset_id = ?
		`, migration.newAssetID, migration.rowID, migration.oldAssetID)
		if err == nil {
			continue
		}
		if isUniqueConstraintError(err) {
			logging.Warning(
				"asset_id migration conflict in protection_config, dropping duplicate row id=%d (%s -> %s)",
				migration.rowID,
				migration.oldAssetID,
				migration.newAssetID,
			)
			if _, deleteErr := tx.Exec(`DELETE FROM protection_config WHERE id = ?`, migration.rowID); deleteErr != nil {
				return fmt.Errorf(
					"delete duplicate protection_config row id=%d after conflict (%s -> %s) failed: %w",
					migration.rowID,
					migration.oldAssetID,
					migration.newAssetID,
					deleteErr,
				)
			}
			continue
		}
		return fmt.Errorf(
			"update protection_config asset_id id=%d (%s -> %s) failed: %w",
			migration.rowID,
			migration.oldAssetID,
			migration.newAssetID,
			err,
		)
	}
	return nil
}

func dedupeAssetIDMappings(migrations []protectionConfigAssetIDMigration) []assetIDMapping {
	mappingByOld := make(map[string]string, len(migrations))
	for _, migration := range migrations {
		mappingByOld[migration.oldAssetID] = migration.newAssetID
	}

	mappings := make([]assetIDMapping, 0, len(mappingByOld))
	for oldAssetID, newAssetID := range mappingByOld {
		mappings = append(mappings, assetIDMapping{
			oldAssetID: oldAssetID,
			newAssetID: newAssetID,
		})
	}
	return mappings
}

func rewriteProtectionStatisticsAssetIDs(tx *sql.Tx, mappings []assetIDMapping) error {
	exists, err := tableExistsTx(tx, "protection_statistics")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	for _, mapping := range mappings {
		if mapping.oldAssetID == mapping.newAssetID {
			continue
		}
		_, err := tx.Exec(
			`UPDATE protection_statistics SET asset_id = ? WHERE asset_id = ?`,
			mapping.newAssetID,
			mapping.oldAssetID,
		)
		if err == nil {
			continue
		}
		if !isUniqueConstraintError(err) {
			return fmt.Errorf(
				"update protection_statistics asset_id (%s -> %s) failed: %w",
				mapping.oldAssetID,
				mapping.newAssetID,
				err,
			)
		}

		// Merge duplicate statistics rows into the stable asset_id row.
		if _, err := tx.Exec(`
			INSERT INTO protection_statistics (
				asset_name, asset_id, analysis_count, message_count, warning_count, blocked_count,
				total_tokens, total_prompt_tokens, total_completion_tokens, total_tool_calls,
				request_count, audit_tokens, audit_prompt_tokens, audit_completion_tokens, updated_at
			)
			SELECT
				asset_name, ?, analysis_count, message_count, warning_count, blocked_count,
				total_tokens, total_prompt_tokens, total_completion_tokens, total_tool_calls,
				request_count, audit_tokens, audit_prompt_tokens, audit_completion_tokens, updated_at
			FROM protection_statistics
			WHERE asset_id = ?
			ON CONFLICT(asset_id) DO UPDATE SET
				analysis_count = protection_statistics.analysis_count + excluded.analysis_count,
				message_count = protection_statistics.message_count + excluded.message_count,
				warning_count = protection_statistics.warning_count + excluded.warning_count,
				blocked_count = protection_statistics.blocked_count + excluded.blocked_count,
				total_tokens = protection_statistics.total_tokens + excluded.total_tokens,
				total_prompt_tokens = protection_statistics.total_prompt_tokens + excluded.total_prompt_tokens,
				total_completion_tokens = protection_statistics.total_completion_tokens + excluded.total_completion_tokens,
				total_tool_calls = protection_statistics.total_tool_calls + excluded.total_tool_calls,
				request_count = protection_statistics.request_count + excluded.request_count,
				audit_tokens = protection_statistics.audit_tokens + excluded.audit_tokens,
				audit_prompt_tokens = protection_statistics.audit_prompt_tokens + excluded.audit_prompt_tokens,
				audit_completion_tokens = protection_statistics.audit_completion_tokens + excluded.audit_completion_tokens,
				updated_at = CASE
					WHEN excluded.updated_at > protection_statistics.updated_at THEN excluded.updated_at
					ELSE protection_statistics.updated_at
				END
		`, mapping.newAssetID, mapping.oldAssetID); err != nil {
			return fmt.Errorf(
				"merge protection_statistics asset_id (%s -> %s) failed: %w",
				mapping.oldAssetID,
				mapping.newAssetID,
				err,
			)
		}
		if _, err := tx.Exec(`DELETE FROM protection_statistics WHERE asset_id = ?`, mapping.oldAssetID); err != nil {
			return fmt.Errorf(
				"cleanup old protection_statistics asset_id %s failed: %w",
				mapping.oldAssetID,
				err,
			)
		}
	}
	return nil
}

func rewriteShepherdRulesAssetIDs(tx *sql.Tx, mappings []assetIDMapping) error {
	exists, err := tableExistsTx(tx, "shepherd_rules")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	for _, mapping := range mappings {
		if mapping.oldAssetID == mapping.newAssetID {
			continue
		}
		_, err := tx.Exec(
			`UPDATE shepherd_rules SET asset_id = ? WHERE asset_id = ?`,
			mapping.newAssetID,
			mapping.oldAssetID,
		)
		if err == nil {
			continue
		}
		if !isUniqueConstraintError(err) {
			return fmt.Errorf(
				"update shepherd_rules asset_id (%s -> %s) failed: %w",
				mapping.oldAssetID,
				mapping.newAssetID,
				err,
			)
		}

		// Keep the latest rules payload when both IDs exist.
		if _, err := tx.Exec(`
			INSERT INTO shepherd_rules (asset_id, asset_name, sensitive_actions, updated_at)
			SELECT ?, asset_name, sensitive_actions, updated_at
			FROM shepherd_rules
			WHERE asset_id = ?
			ON CONFLICT(asset_id) DO UPDATE SET
				asset_name = CASE
					WHEN excluded.updated_at >= shepherd_rules.updated_at THEN excluded.asset_name
					ELSE shepherd_rules.asset_name
				END,
				sensitive_actions = CASE
					WHEN excluded.updated_at >= shepherd_rules.updated_at THEN excluded.sensitive_actions
					ELSE shepherd_rules.sensitive_actions
				END,
				updated_at = CASE
					WHEN excluded.updated_at >= shepherd_rules.updated_at THEN excluded.updated_at
					ELSE shepherd_rules.updated_at
				END
		`, mapping.newAssetID, mapping.oldAssetID); err != nil {
			return fmt.Errorf(
				"merge shepherd_rules asset_id (%s -> %s) failed: %w",
				mapping.oldAssetID,
				mapping.newAssetID,
				err,
			)
		}
		if _, err := tx.Exec(`DELETE FROM shepherd_rules WHERE asset_id = ?`, mapping.oldAssetID); err != nil {
			return fmt.Errorf(
				"cleanup old shepherd_rules asset_id %s failed: %w",
				mapping.oldAssetID,
				err,
			)
		}
	}
	return nil
}

func rewriteTableAssetIDs(tx *sql.Tx, tableName string, mappings []assetIDMapping) error {
	exists, err := tableExistsTx(tx, tableName)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	for _, mapping := range mappings {
		if mapping.oldAssetID == mapping.newAssetID {
			continue
		}
		if _, err := tx.Exec(
			fmt.Sprintf(`UPDATE %s SET asset_id = ? WHERE asset_id = ?`, quoteSQLiteIdentifier(tableName)),
			mapping.newAssetID,
			mapping.oldAssetID,
		); err != nil {
			return fmt.Errorf(
				"update %s asset_id (%s -> %s) failed: %w",
				tableName,
				mapping.oldAssetID,
				mapping.newAssetID,
				err,
			)
		}
	}
	return nil
}

func tableExistsTx(tx *sql.Tx, tableName string) (bool, error) {
	var count int
	if err := tx.QueryRow(`
		SELECT COUNT(1)
		FROM sqlite_master
		WHERE type = 'table'
		  AND name = ?
	`, tableName).Scan(&count); err != nil {
		return false, fmt.Errorf("check table %s existence failed: %w", tableName, err)
	}
	return count > 0, nil
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") ||
		strings.Contains(msg, "primary key constraint failed")
}
