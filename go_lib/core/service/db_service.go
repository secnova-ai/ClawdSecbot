// Package service provides the database FFI service layer.
// This layer sits between core and repository, handling JSON parsing,
// repository calls, and response formatting.
package service

import (
	"encoding/json"
	"fmt"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/repository"
)

// ========== Database lifecycle management ==========

type InitializeDatabaseRequest struct {
	CurrentVersion string `json:"current_version"`
}

// InitializeDatabase initializes the database connection.
// Request must use JSON input and include current_version.
func InitializeDatabase(requestJSON string) map[string]interface{} {
	var request InitializeDatabaseRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		logging.Error("Failed to parse InitializeDatabase request: %v", err)
		return errorResult(fmt.Errorf("invalid InitializeDatabase request: %w", err))
	}

	if request.CurrentVersion == "" {
		return errorResult(fmt.Errorf("current_version is required"))
	}

	pm := core.GetPathManager()
	if !pm.IsInitialized() {
		return errorResult(fmt.Errorf("path manager is not initialized"))
	}

	logging.Info(
		"Initializing database: db_path=%s current_version=%s version_file=%s",
		pm.GetDBPath(),
		request.CurrentVersion,
		pm.GetVersionFilePath(),
	)

	summary, err := repository.InitDBWithVersion(
		pm.GetDBPath(),
		request.CurrentVersion,
		pm.GetVersionFilePath(),
	)
	if err != nil {
		logging.Error("Failed to initialize database: %v", err)
		return errorResult(err)
	}

	return successDataResult(map[string]interface{}{
		"path":              pm.GetDBPath(),
		"current_version":   summary.CurrentVersion,
		"previous_version":  summary.PreviousVersion,
		"version_source":    summary.VersionSource,
		"fresh_install":     summary.FreshInstall,
		"upgraded":          summary.Upgraded,
		"version_file_path": pm.GetVersionFilePath(),
	})
}

// CloseDatabase closes the database connection.
func CloseDatabase() map[string]interface{} {
	logging.Info("Closing database")

	if err := repository.CloseDB(); err != nil {
		logging.Error("Failed to close database: %v", err)
		return errorResult(err)
	}

	logging.Info("Database closed successfully")
	return map[string]interface{}{
		"success": true,
	}
}
