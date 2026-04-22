package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"go_lib/core/logging"
)

// SandboxManager manages sandboxed gateway processes
// 平台相关逻辑委托给 manager_darwin.go / manager_linux.go 中的方法实现
type SandboxManager struct {
	mu          sync.RWMutex
	assetName   string
	policyPath  string
	managedPID  int
	managedCmd  *exec.Cmd
	gatewayArgs []string
	gatewayEnv  []string
	config      SandboxConfig
	monitor     *ProcessMonitor
	isRunning   bool
	stopCh      chan struct{}
	policyDir   string
	logDir      string
	lastError   error
	retryCount  int
	maxRetries  int
	preloadLib  string // Linux: LD_PRELOAD 沙箱库路径
}

// NewSandboxManager creates a new sandbox manager
func NewSandboxManager(assetName string, policyDir string) *SandboxManager {
	return NewSandboxManagerWithLogDir(assetName, policyDir, "")
}

// NewSandboxManagerWithLogDir creates a new sandbox manager with an optional log directory.
// When logDir is empty, platform-specific defaults/fallbacks are used.
func NewSandboxManagerWithLogDir(assetName string, policyDir string, logDir string) *SandboxManager {
	return &SandboxManager{
		assetName:  assetName,
		policyDir:  policyDir,
		logDir:     logDir,
		stopCh:     make(chan struct{}),
		maxRetries: 3,
	}
}

// SetLogDir updates the sandbox log directory.
func (m *SandboxManager) SetLogDir(logDir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logDir = logDir
}

// IsSandboxSupported checks if sandbox is available on this system
func IsSandboxSupported() bool {
	return isSandboxSupportedOnPlatform()
}

// ValidateGatewayBinary validates that the gateway binary exists and is safe to use
func ValidateGatewayBinary(binaryPath string) error {
	if binaryPath == "" {
		return fmt.Errorf("gateway binary path is empty")
	}

	// Must be absolute path
	if !filepath.IsAbs(binaryPath) {
		return fmt.Errorf("gateway binary path must be absolute: %s", binaryPath)
	}

	// Check if file exists
	info, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("gateway binary not found: %w", err)
	}

	// Must be a regular file
	if !info.Mode().IsRegular() {
		return fmt.Errorf("gateway binary is not a regular file: %s", binaryPath)
	}

	// Must be executable (skip on Windows where permission bits are not meaningful)
	if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
		return fmt.Errorf("gateway binary is not executable: %s", binaryPath)
	}

	// Security: prevent path traversal
	cleanPath := filepath.Clean(binaryPath)
	if cleanPath != binaryPath {
		return fmt.Errorf("gateway binary path contains suspicious elements: %s", binaryPath)
	}

	return nil
}

// Configure sets up the sandbox configuration
func (m *SandboxManager) Configure(config SandboxConfig, gatewayArgs, gatewayEnv []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = config
	m.gatewayArgs = gatewayArgs
	m.gatewayEnv = gatewayEnv
	m.retryCount = 0
	m.lastError = nil

	return m.platformConfigure()
}

// Start starts the gateway process in a sandbox
func (m *SandboxManager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		return fmt.Errorf("sandbox already running")
	}

	if !IsSandboxSupported() {
		m.lastError = fmt.Errorf("sandbox not supported on this system")
		return m.lastError
	}

	// Build platform-specific sandbox command
	cmd, policyPath, err := m.buildSandboxCommand()
	if err != nil {
		m.lastError = err
		return fmt.Errorf("failed to build sandbox command: %w", err)
	}
	m.policyPath = policyPath

	// Start the process
	if err := cmd.Start(); err != nil {
		m.cleanupPolicyFile()
		m.lastError = err
		return fmt.Errorf("failed to start sandboxed process: %w", err)
	}

	// Platform-specific post-start hook (e.g. Windows DLL injection + resume)
	if err := m.platformPostStart(cmd); err != nil {
		m.cleanupPolicyFile()
		m.lastError = err
		return fmt.Errorf("platform post-start failed: %w", err)
	}

	m.managedCmd = cmd
	m.managedPID = cmd.Process.Pid
	m.isRunning = true
	m.lastError = nil
	m.stopCh = make(chan struct{})

	logging.Info("Started sandboxed gateway [PID: %d, Policy: %s]", m.managedPID, m.policyPath)

	// Start goroutine to wait for process exit
	go m.waitForExit()

	return nil
}

// cleanupPolicyFile removes the policy file
func (m *SandboxManager) cleanupPolicyFile() {
	if m.policyPath != "" {
		if err := os.Remove(m.policyPath); err != nil && !os.IsNotExist(err) {
			logging.Warning("Failed to cleanup policy file %s: %v", m.policyPath, err)
		}
		m.policyPath = ""
	}
}

// Stop stops the managed sandboxed process
func (m *SandboxManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		// Still cleanup policy file if exists
		m.cleanupPolicyFile()
		return nil
	}

	// Signal stop (only if channel is open)
	select {
	case <-m.stopCh:
		// Already closed
	default:
		close(m.stopCh)
	}

	if m.managedCmd != nil && m.managedCmd.Process != nil {
		logging.Info("Stopping sandboxed gateway [PID: %d]", m.managedPID)
		if err := gracefulTerminate(m.managedCmd.Process); err != nil {
			logging.Warning("Failed to send termination signal: %v", err)
		}

		// Wait for graceful shutdown
		done := make(chan error, 1)
		go func() {
			done <- m.managedCmd.Wait()
		}()

		select {
		case <-done:
			logging.Info("Gateway stopped gracefully")
		case <-time.After(5 * time.Second):
			// Force kill
			logging.Warning("Gateway did not stop gracefully, forcing kill")
			if err := m.managedCmd.Process.Kill(); err != nil {
				logging.Error("Failed to kill process: %v", err)
			}
		}
	}

	// Cleanup policy file
	m.cleanupPolicyFile()

	m.isRunning = false
	m.managedPID = 0
	m.managedCmd = nil
	m.stopCh = make(chan struct{})

	return nil
}

// waitForExit waits for the managed process to exit
func (m *SandboxManager) waitForExit() {
	if m.managedCmd == nil {
		return
	}

	err := m.managedCmd.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	if err != nil {
		logging.Warning("Sandboxed gateway exited with error: %v", err)
	} else {
		logging.Info("Sandboxed gateway exited normally")
	}

	m.isRunning = false
	m.managedPID = 0
	m.managedCmd = nil
}

// GetStatus returns the current sandbox status
func (m *SandboxManager) GetStatus() SandboxStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return SandboxStatus{
		Running:       m.isRunning,
		ManagedPID:    m.managedPID,
		PolicyPath:    m.policyPath,
		AssetName:     m.assetName,
		GatewayBinary: "openclaw",
	}
}

// IsRunning returns true if the sandbox is running
func (m *SandboxManager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isRunning
}

// GetManagedPID returns the PID of the managed process
func (m *SandboxManager) GetManagedPID() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.managedPID
}

// SetMonitor sets the process monitor
func (m *SandboxManager) SetMonitor(monitor *ProcessMonitor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.monitor = monitor
}

// Global sandbox manager registry
var (
	sandboxManagers = make(map[string]*SandboxManager)
	sandboxMu       sync.RWMutex
)

// GetSandboxManager gets or creates a sandbox manager for an asset
func GetSandboxManager(assetName string, policyDir string) *SandboxManager {
	return GetSandboxManagerByKey(assetName, policyDir)
}

// GetSandboxManagerByKey gets or creates a sandbox manager for an explicit instance key.
func GetSandboxManagerByKey(assetKey string, policyDir string) *SandboxManager {
	assetKey = strings.TrimSpace(assetKey)
	if assetKey == "" {
		return nil
	}
	sandboxMu.Lock()
	defer sandboxMu.Unlock()

	if manager, exists := sandboxManagers[assetKey]; exists {
		return manager
	}

	manager := NewSandboxManager(assetKey, policyDir)
	sandboxManagers[assetKey] = manager
	return manager
}

// GetExistingSandboxManager returns existing sandbox manager keyed by assetName.
func GetExistingSandboxManager(assetName string) *SandboxManager {
	return GetExistingSandboxManagerByKey(assetName)
}

// GetExistingSandboxManagerByKey returns an existing sandbox manager by explicit instance key.
func GetExistingSandboxManagerByKey(assetKey string) *SandboxManager {
	assetKey = strings.TrimSpace(assetKey)
	if assetKey == "" {
		return nil
	}
	sandboxMu.RLock()
	defer sandboxMu.RUnlock()
	return sandboxManagers[assetKey]
}

// RemoveSandboxManager removes a sandbox manager
func RemoveSandboxManager(assetName string) {
	RemoveSandboxManagerByKey(assetName)
}

// RemoveSandboxManagerByKey removes a sandbox manager by explicit instance key.
func RemoveSandboxManagerByKey(assetKey string) {
	assetKey = strings.TrimSpace(assetKey)
	if assetKey == "" {
		return
	}
	sandboxMu.Lock()
	defer sandboxMu.Unlock()

	if manager, exists := sandboxManagers[assetKey]; exists {
		manager.Stop()
		delete(sandboxManagers, assetKey)
	}
}

// GetAllSandboxStatus returns status of all sandbox managers
func GetAllSandboxStatus() map[string]SandboxStatus {
	sandboxMu.RLock()
	defer sandboxMu.RUnlock()

	result := make(map[string]SandboxStatus)
	for name, manager := range sandboxManagers {
		result[name] = manager.GetStatus()
	}
	return result
}

// ParseSandboxConfigJSON parses sandbox config from JSON
func ParseSandboxConfigJSON(jsonStr string) (*SandboxConfig, error) {
	var config SandboxConfig
	if err := json.Unmarshal([]byte(jsonStr), &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// GetDefaultPolicyDir returns the default policy directory
// This is a fallback - prefer passing explicit policyDir from Dart layer
func GetDefaultPolicyDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		wd, wdErr := os.Getwd()
		if wdErr != nil {
			return filepath.Join(".", ".botsec", "policies")
		}
		return filepath.Join(wd, ".botsec", "policies")
	}

	// Use ~/.botsec/policies (matches Dart layer SandboxService)
	return filepath.Join(homeDir, ".botsec", "policies")
}
