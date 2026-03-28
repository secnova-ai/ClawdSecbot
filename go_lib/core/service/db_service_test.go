package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go_lib/core"
	"go_lib/core/repository"

	_ "modernc.org/sqlite"
)

// TestInitializeDatabase verifies database initialization.
func TestInitializeDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "bot_sec_manager.db")
	versionFile := filepath.Join(tmpDir, "bot_sec_manager.version")
	mustInitPathManager(t, tmpDir)

	result := InitializeDatabase(mustInitDatabaseRequestJSON(t, "1.0.1"))
	defer repository.CloseDB()

	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data payload, got: %v", result)
	}
	if data["path"] != tmpFile {
		t.Errorf("Expected path=%s, got: %v", tmpFile, data["path"])
	}

	// Verify the shared database is available.
	db := repository.GetDB()
	if db == nil {
		t.Fatal("GetDB returned nil after InitializeDatabase")
	}

	content, err := os.ReadFile(versionFile)
	if err != nil {
		t.Fatalf("Expected version file to be written: %v", err)
	}
	if string(content) != "1.0.1\n" {
		t.Fatalf("Expected version file content 1.0.1, got %q", string(content))
	}
}

// TestInitializeDatabase_PathManagerNotInitialized verifies init fails when
// core path state is unavailable.
func TestInitializeDatabase_PathManagerNotInitialized(t *testing.T) {
	if err := core.GetPathManager().ResetForTest("", ""); err != nil {
		t.Fatalf("Failed to reset path manager: %v", err)
	}
	result := InitializeDatabase(mustInitDatabaseRequestJSON(t, "1.0.1"))
	defer repository.CloseDB()

	if result["success"] != false {
		t.Errorf("Expected success=false for uninitialized path manager, got: %v", result)
	}
	if result["error"] == nil {
		t.Error("Expected error message for uninitialized path manager")
	}
}

func TestInitializeDatabase_InvalidRequest(t *testing.T) {
	result := InitializeDatabase(`{}`)
	if result["success"] != false {
		t.Fatalf("Expected success=false, got: %v", result)
	}
	if result["error"] == nil {
		t.Fatal("Expected validation error for invalid request")
	}
}

// TestCloseDatabase verifies database shutdown.
func TestCloseDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "bot_sec_manager.db")
	versionFile := filepath.Join(tmpDir, "bot_sec_manager.version")
	mustInitPathManager(t, tmpDir)
	InitializeDatabase(mustInitDatabaseRequestJSON(t, "1.0.1"))

	result := CloseDatabase()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}

	if _, err := os.Stat(tmpFile); err != nil {
		t.Fatalf("Expected database file to exist: %v", err)
	}
	if _, err := os.Stat(versionFile); err != nil {
		t.Fatalf("Expected version file to exist: %v", err)
	}
}

// TestCloseDatabase_NotInitialized verifies closing before initialization is safe.
func TestCloseDatabase_NotInitialized(t *testing.T) {
	result := CloseDatabase()
	if result["success"] != true {
		t.Errorf("Expected success=true for closing uninitialized DB, got: %v", result)
	}
}

func mustInitDatabaseRequestJSON(t *testing.T, currentVersion string) string {
	t.Helper()

	request := InitializeDatabaseRequest{
		CurrentVersion: currentVersion,
	}

	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal test request: %v", err)
	}

	return string(data)
}

func mustInitPathManager(t *testing.T, workspaceDir string) {
	t.Helper()

	if err := core.GetPathManager().ResetForTest(workspaceDir, t.TempDir()); err != nil {
		t.Fatalf("Failed to initialize path manager: %v", err)
	}
}
