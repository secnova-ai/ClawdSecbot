# Version Upgrade Migration Guide

This document merges the migration principles and the day-to-day execution guide into one English reference for version upgrade work.

## Background

As the software evolves, the database schema also changes. If a new version starts directly against an older database without a clear upgrade path, it can easily cause:

- Missing columns
- Incompatible tables
- Runtime SQL failures
- Old data semantics conflicting with new logic

To avoid this, the project uses an explicit startup migration mechanism driven by the application version.

## Goals

The migration mechanism is designed to ensure:

1. All version migration logic is managed in one fixed Go file.
2. Each version only implements the previous-version to current-version step.
3. Startup automatically detects whether an upgrade happened.
4. Cross-version upgrades run each migration step in order.
5. The current version is persisted after migration completes.
6. Fresh installs are not misclassified as upgrades.
7. A missing version file does not incorrectly downgrade state detection to `1.0.0`.
8. Downgrade startup is explicitly blocked.

## Core Files

The migration system is centered around these files:

- [app_upgrade_migration.go](/Users/kidbei/projects/bot_sec_manager/go_lib/core/repository/app_upgrade_migration.go)
- [db.go](/Users/kidbei/projects/bot_sec_manager/go_lib/core/repository/db.go)
- [db_service.go](/Users/kidbei/projects/bot_sec_manager/go_lib/core/service/db_service.go)
- [database_service.dart](/Users/kidbei/projects/bot_sec_manager/lib/services/database_service.dart)
- [native_library_service.dart](/Users/kidbei/projects/bot_sec_manager/lib/services/native_library_service.dart)

## Persisted Version State

The current implementation stores version state in two places:

1. A disk version file
   `Application Support/bot_sec_manager.version`
2. A database metadata table
   `app_metadata`

The version file is the primary source. `app_metadata.runtime_version` is the fallback source.

This keeps the design aligned with the original requirement of checking a disk file, while also protecting against cases where the file is deleted or not written successfully.

## Startup Flow

At startup, Flutter passes these values to Go through FFI:

- the shared app data base directory through `InitPathsFFI`
- `current_version`

The Go side then performs the following sequence:

1. Open SQLite
2. Derive the database path and version file path from `PathManager`
3. Read the disk version file
4. Read `app_metadata.runtime_version`
5. Parse the current running version
6. Resolve the effective stored database version
7. If stored version is lower than current version, run the migration chain
8. Rebuild or verify all current-version tables
9. Persist the current version back to:
   - the disk version file
   - `app_metadata`

Flutter does not own the database path or version file path contract anymore. It only provides the shared base directory, and core derives all runtime-owned paths from that single source of truth.

## Version Resolution Rules

Priority order:

1. Disk version file
2. `app_metadata.runtime_version`
3. Compatibility inference

If neither the version file nor metadata exists:

- If the database has no application tables, treat it as a fresh install
- If the database already has application tables, treat it as a legacy `1.0.0` database

That distinction is important:

- A fresh install of `1.0.1` must not trigger `1.0.0 -> 1.0.1`
- A real upgrade from released `1.0.0` to `1.0.1` must trigger migration

## Migration Rules

All future migration work must follow these rules:

1. Put every version migration only in [app_upgrade_migration.go](/Users/kidbei/projects/bot_sec_manager/go_lib/core/repository/app_upgrade_migration.go)
2. Implement one step per version
3. Do not skip versions
4. Do not put upgrade logic into table-creation helpers
5. Table creation functions only describe the current schema
6. Migration failures must stop startup
7. Every new migration must include repository/service tests

## Multi-Step Upgrades

Migration steps are chained. For example:

- `1.0.0 -> 1.0.1`
- `1.0.1 -> 1.0.2`
- `1.0.2 -> 1.0.3`

If a user upgrades directly from `1.0.0` to `1.0.3`, the application must run:

1. `1.0.0 -> 1.0.1`
2. `1.0.1 -> 1.0.2`
3. `1.0.2 -> 1.0.3`

If any intermediate step is missing, startup must fail rather than guessing a path.

## Current Special Case: `1.0.0 -> 1.0.1`

`1.0.1` is not schema-compatible with `1.0.0`.

So the migration strategy for `1.0.0 -> 1.0.1` is destructive:

- Drop all application tables
- Recreate the full schema using the current version structure

This is implemented in `migrateDatabaseFrom1_0_0To1_0_1`.

This approach is intentional because:

- `1.0.0` had no persisted migration version state
- The old schema is not compatible with the new schema
- Continuing to rely on scattered column-level compatibility patches would make future upgrades much harder to maintain

## How To Add a New Migration

Assume the current released version is `1.0.1`, and the next version is `1.0.2`.

### Step 1: Decide whether migration is needed

Ask:

- Can the new code use the old database directly?
- Is the old data still semantically correct under the new logic?

If either answer is no, add a migration.

### Step 2: Register the migration step

Add a new entry in `databaseVersionMigrations`:

```go
var databaseVersionMigrations = []databaseVersionMigration{
	{
		fromVersion: "1.0.0",
		toVersion:   "1.0.1",
		run:         migrateDatabaseFrom1_0_0To1_0_1,
	},
	{
		fromVersion: "1.0.1",
		toVersion:   "1.0.2",
		run:         migrateDatabaseFrom1_0_1To1_0_2,
	},
}
```

### Step 3: Implement the migration function

Example:

```go
func migrateDatabaseFrom1_0_1To1_0_2(db *sql.DB) error {
	// 1. Transform schema
	// 2. Migrate data
	// 3. Clean up deprecated structures
	return nil
}
```

Recommended patterns:

- If schemas are fundamentally incompatible, rebuild
- If data must be preserved, create a new structure and migrate data explicitly
- If only defaults or semantics changed, run targeted updates

Do not:

- mix multiple version jumps into one function
- hide migration logic in repository initialization
- add speculative compatibility code unrelated to the release

## Rebuild vs. Data Migration

Use rebuild when:

- historical data is low-value
- schema differences are too large
- preserving data is more complex than the value it provides
- old versions do not have stable state to reason about safely

Use data migration when:

- historical user data must be retained
- old and new structures have a clear mapping
- migration correctness can be validated

When preserving data, prefer this order:

1. Create the new structure
2. Convert and copy data
3. Remove the old structure

## Keep Current Schema in Sync

Migration functions handle the transition path. Table creation helpers still need to reflect the final schema for the current version.

Always update:

- [db.go](/Users/kidbei/projects/bot_sec_manager/go_lib/core/repository/db.go)
- related repository SQL
- service-layer assumptions

The rule is:

- migrations describe how to move forward
- table creation describes the final state

## Do Not Reintroduce Scattered Implicit Migrations

Do not go back to patterns like:

- `ALTER TABLE ADD COLUMN` hidden inside create-table code
- field rename compatibility logic buried in repositories
- small ad-hoc migration fragments spread across modules

Those patterns make upgrade behavior harder to trace, validate, and maintain.

## Downgrade Policy

If the persisted version is higher than the current running version, startup must fail with a downgrade-not-supported error.

This prevents:

- older code from misreading newer schemas
- incompatible writes back into a newer database
- silent data corruption

## Error Handling Rules

If migration fails:

- return an error
- stop startup
- do not swallow the failure

Startup migration is high-risk. Continuing after a failed migration can leave the database in a half-old, half-new state.

## Test Requirements

Every new migration should at least cover:

1. Fresh install
2. Single-step upgrade
3. Multi-step upgrade chain
4. Missing version file with metadata fallback
5. Downgrade rejection

Tests should primarily live in:

- [app_upgrade_migration_test.go](/Users/kidbei/projects/bot_sec_manager/go_lib/core/repository/app_upgrade_migration_test.go)
- [db_service_test.go](/Users/kidbei/projects/bot_sec_manager/go_lib/core/service/db_service_test.go)

## Release Checklist

Before releasing a new version, confirm:

1. `pubspec.yaml` version is updated
2. the new version step is registered in `databaseVersionMigrations`
3. the migration function only handles a single version jump
4. current schema helpers reflect the new final structure
5. repository/service tests are updated
6. both fresh-install and upgrade paths were verified
7. this document remains accurate

## Recommended Commit Message

For migration-related changes, include the target version and scope clearly, for example:

```text
feat: add startup db migration for 1.0.1 -> 1.0.2
```

## Summary

All future database upgrades should follow one rule:

Detect the version first, run the single-step migration chain second, and rebuild/verify the current schema last.

As long as that rule is kept, version upgrades will stay predictable and maintainable.
