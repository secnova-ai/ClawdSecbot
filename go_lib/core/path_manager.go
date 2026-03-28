package core

import (
	"os"
	"path/filepath"
	"sync"

	"go_lib/core/logging"
)

// PathManager manages application paths derived from a single base directory
// provided by Flutter during startup.
type PathManager struct {
	mu sync.RWMutex

	initialized bool

	// Base paths provided or inferred at startup.
	workspaceDir string
	homeDir      string

	// Derived paths owned by core.
	logDir          string
	backupDir       string
	policyDir       string
	reactSkillDir   string
	scanSkillDir    string
	dbPath          string
	versionFilePath string
}

var (
	globalPathManager *PathManager
	pathManagerOnce   sync.Once
)

// GetPathManager returns the shared path manager instance.
func GetPathManager() *PathManager {
	pathManagerOnce.Do(func() {
		globalPathManager = &PathManager{}
	})
	return globalPathManager
}

// Initialize configures the path manager from a single app data base
// directory and the user home directory.
func (pm *PathManager) Initialize(workspaceDir, homeDir string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.initialized {
		logging.Warning("PathManager already initialized, skipping")
		return nil
	}

	pm.workspaceDir = workspaceDir
	pm.homeDir = homeDir

	// Derive all runtime-owned paths from the shared base directory.
	pm.logDir = filepath.Join(workspaceDir, "logs")
	pm.backupDir = filepath.Join(workspaceDir, "backups")
	pm.policyDir = filepath.Join(homeDir, ".botsec", "policies")
	pm.reactSkillDir = filepath.Join(workspaceDir, "skills", "shepherd_gate")
	pm.scanSkillDir = filepath.Join(workspaceDir, "skills", "skill_scanner")
	pm.dbPath = filepath.Join(workspaceDir, "bot_sec_manager.db")
	pm.versionFilePath = filepath.Join(workspaceDir, "bot_sec_manager.version")

	// Ensure required directories exist before use.
	if err := pm.ensureDirectories(); err != nil {
		logging.Error("Failed to ensure directories: %v", err)
		return err
	}

	pm.initialized = true
	logging.Info("PathManager initialized: workspaceDir=%s, homeDir=%s", workspaceDir, homeDir)
	logging.Info(
		"Derived paths: logDir=%s, backupDir=%s, policyDir=%s, dbPath=%s, versionFilePath=%s",
		pm.logDir,
		pm.backupDir,
		pm.policyDir,
		pm.dbPath,
		pm.versionFilePath,
	)
	logging.Info("Derived paths (skills): reactSkillDir=%s, scanSkillDir=%s", pm.reactSkillDir, pm.scanSkillDir)

	return nil
}

// ensureDirectories creates required runtime directories.
func (pm *PathManager) ensureDirectories() error {
	dirs := []string{pm.logDir, pm.backupDir, pm.policyDir, pm.reactSkillDir, pm.scanSkillDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// IsInitialized reports whether the manager has been configured.
func (pm *PathManager) IsInitialized() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.initialized
}

// ========== Getters ==========

// GetWorkspaceDir returns the shared app data base directory.
func (pm *PathManager) GetWorkspaceDir() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.workspaceDir
}

// GetHomeDir returns the user home directory.
func (pm *PathManager) GetHomeDir() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.homeDir
}

// GetLogDir returns the derived logs directory.
func (pm *PathManager) GetLogDir() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.logDir
}

// GetBackupDir returns the derived backups directory.
func (pm *PathManager) GetBackupDir() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.backupDir
}

// GetPolicyDir returns the derived sandbox policy directory.
func (pm *PathManager) GetPolicyDir() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.policyDir
}

// GetReActSkillDir returns the derived ReAct skill directory.
func (pm *PathManager) GetReActSkillDir() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.reactSkillDir
}

// GetScanSkillDir returns the derived scan skill directory.
func (pm *PathManager) GetScanSkillDir() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.scanSkillDir
}

// GetDBPath returns the derived database file path.
func (pm *PathManager) GetDBPath() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.dbPath
}

// GetVersionFilePath returns the derived runtime version file path.
func (pm *PathManager) GetVersionFilePath() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.versionFilePath
}

// ResetForTest resets the manager and optionally reinitializes it.
// This is intended for tests that need isolated path state.
func (pm *PathManager) ResetForTest(workspaceDir, homeDir string) error {
	pm.mu.Lock()
	pm.initialized = false
	pm.workspaceDir = ""
	pm.homeDir = ""
	pm.logDir = ""
	pm.backupDir = ""
	pm.policyDir = ""
	pm.reactSkillDir = ""
	pm.scanSkillDir = ""
	pm.dbPath = ""
	pm.versionFilePath = ""
	pm.mu.Unlock()

	if workspaceDir == "" && homeDir == "" {
		return nil
	}

	return pm.Initialize(workspaceDir, homeDir)
}

// ========== Path helpers ==========

// JoinWorkspace joins paths under the shared base directory.
func (pm *PathManager) JoinWorkspace(elem ...string) string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	parts := append([]string{pm.workspaceDir}, elem...)
	return filepath.Join(parts...)
}

// JoinHome joins paths under the user home directory.
func (pm *PathManager) JoinHome(elem ...string) string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	parts := append([]string{pm.homeDir}, elem...)
	return filepath.Join(parts...)
}

// JoinLog joins paths under the logs directory.
func (pm *PathManager) JoinLog(elem ...string) string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	parts := append([]string{pm.logDir}, elem...)
	return filepath.Join(parts...)
}

// JoinBackup joins paths under the backups directory.
func (pm *PathManager) JoinBackup(elem ...string) string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	parts := append([]string{pm.backupDir}, elem...)
	return filepath.Join(parts...)
}

// JoinPolicy joins paths under the policy directory.
func (pm *PathManager) JoinPolicy(elem ...string) string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	parts := append([]string{pm.policyDir}, elem...)
	return filepath.Join(parts...)
}

// JoinReActSkill joins paths under the ReAct skill directory.
func (pm *PathManager) JoinReActSkill(elem ...string) string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	parts := append([]string{pm.reactSkillDir}, elem...)
	return filepath.Join(parts...)
}

// JoinScanSkill joins paths under the scan skill directory.
func (pm *PathManager) JoinScanSkill(elem ...string) string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	parts := append([]string{pm.scanSkillDir}, elem...)
	return filepath.Join(parts...)
}
