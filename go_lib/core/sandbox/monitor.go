package sandbox

import (
	"os"
	"strings"
	"sync"
	"time"

	"go_lib/core/logging"
)

// ProcessMonitor monitors for unmanaged gateway processes and enforces sandbox
type ProcessMonitor struct {
	mu                sync.RWMutex
	assetName         string
	gatewayPattern    string
	sandboxManager    *SandboxManager
	config            SandboxConfig
	isMonitoring      bool
	stopCh            chan struct{}
	checkInterval     time.Duration
	onProcessKilled   func(pid int)
	onProcessTakeover func(pid int)
}

// NewProcessMonitor creates a new process monitor
func NewProcessMonitor(assetName string, gatewayPattern string) *ProcessMonitor {
	return &ProcessMonitor{
		assetName:      assetName,
		gatewayPattern: gatewayPattern,
		checkInterval:  5 * time.Second,
		stopCh:         make(chan struct{}),
	}
}

// SetSandboxManager sets the sandbox manager to use for restarting
func (m *ProcessMonitor) SetSandboxManager(manager *SandboxManager) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sandboxManager = manager
}

// SetConfig sets the sandbox config
func (m *ProcessMonitor) SetConfig(config SandboxConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}

// SetCheckInterval sets the monitoring interval
func (m *ProcessMonitor) SetCheckInterval(interval time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkInterval = interval
}

// SetCallbacks sets callback functions for events
func (m *ProcessMonitor) SetCallbacks(onKilled, onTakeover func(pid int)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onProcessKilled = onKilled
	m.onProcessTakeover = onTakeover
}

// Start starts the process monitoring
func (m *ProcessMonitor) Start() error {
	m.mu.Lock()
	if m.isMonitoring {
		m.mu.Unlock()
		return nil
	}
	m.isMonitoring = true
	m.stopCh = make(chan struct{})
	m.mu.Unlock()

	logging.Info("Starting process monitor for: %s (pattern: %s)", m.assetName, m.gatewayPattern)

	go m.monitorLoop()
	return nil
}

// Stop stops the process monitoring
func (m *ProcessMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isMonitoring {
		return
	}

	logging.Info("Stopping process monitor for: %s", m.assetName)
	close(m.stopCh)
	m.isMonitoring = false
}

// IsMonitoring returns true if monitoring is active
func (m *ProcessMonitor) IsMonitoring() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isMonitoring
}

// monitorLoop is the main monitoring loop
func (m *ProcessMonitor) monitorLoop() {
	m.mu.RLock()
	interval := m.checkInterval
	m.mu.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkAndEnforce()
		case <-m.stopCh:
			logging.Info("Process monitor stopped for: %s", m.assetName)
			return
		}
	}
}

// checkAndEnforce checks for unmanaged processes and enforces sandbox
func (m *ProcessMonitor) checkAndEnforce() {
	m.mu.RLock()
	manager := m.sandboxManager
	pattern := m.gatewayPattern
	onKilled := m.onProcessKilled
	onTakeover := m.onProcessTakeover
	m.mu.RUnlock()

	if manager == nil {
		return
	}

	// Validate pattern is specific enough to avoid killing wrong processes
	if len(pattern) < 5 || pattern == "/" || pattern == "." {
		logging.Error("Gateway pattern too generic, refusing to scan: %s", pattern)
		return
	}

	// Find all processes matching the gateway pattern
	pids, err := FindProcessesByName(pattern)
	if err != nil {
		logging.Warning("Failed to find processes: %v", err)
		return
	}

	if len(pids) == 0 {
		return
	}

	// Get the managed PID
	managedPID := manager.GetManagedPID()
	myPID := os.Getpid()

	// Find unmanaged processes with additional verification
	var unmanagedPIDs []int
	for _, pid := range pids {
		// Skip our managed process
		if pid == managedPID {
			continue
		}
		// Skip our own process
		if pid == myPID {
			continue
		}
		// Skip PID 0 and 1 (kernel/init)
		if pid <= 1 {
			continue
		}
		// Verify process still exists and matches pattern
		if verified, err := verifyProcessMatch(pid, pattern); err != nil || !verified {
			continue
		}
		unmanagedPIDs = append(unmanagedPIDs, pid)
	}

	if len(unmanagedPIDs) == 0 {
		return
	}

	// Limit number of processes to kill in one cycle (safety measure)
	const maxKillsPerCycle = 3
	if len(unmanagedPIDs) > maxKillsPerCycle {
		logging.Warning("Too many unmanaged processes (%d), only killing first %d", len(unmanagedPIDs), maxKillsPerCycle)
		unmanagedPIDs = unmanagedPIDs[:maxKillsPerCycle]
	}

	logging.Warning("Detected %d unmanaged gateway process(es): %v", len(unmanagedPIDs), unmanagedPIDs)

	// Kill unmanaged processes
	killedCount := 0
	for _, pid := range unmanagedPIDs {
		logging.Info("Killing unmanaged process [PID: %d]", pid)
		if err := KillProcess(pid); err != nil {
			logging.Error("Failed to kill process %d: %v", pid, err)
			continue
		}
		logging.Info("Successfully killed unmanaged process [PID: %d]", pid)
		killedCount++

		if onKilled != nil {
			onKilled(pid)
		}
	}

	if killedCount == 0 {
		return
	}

	// Wait for processes to die
	time.Sleep(2 * time.Second)

	// Check if our managed process is still running
	if !manager.IsRunning() {
		// Restart with sandbox
		logging.Info("Restarting gateway with sandbox protection")
		if err := manager.Start(); err != nil {
			logging.Error("Failed to restart sandboxed gateway: %v", err)
			return
		}

		if onTakeover != nil {
			onTakeover(manager.GetManagedPID())
		}
	}
}

// verifyProcessMatch double-checks that a PID matches the expected pattern
func verifyProcessMatch(pid int, pattern string) (bool, error) {
	info, err := GetProcessInfo(pid)
	if err != nil {
		return false, err
	}
	return strings.Contains(info, pattern), nil
}

// ForceCheck triggers an immediate check
func (m *ProcessMonitor) ForceCheck() {
	m.checkAndEnforce()
}

// MonitorStatus represents the current monitor status
type MonitorStatus struct {
	AssetName      string `json:"asset_name"`
	IsMonitoring   bool   `json:"is_monitoring"`
	GatewayPattern string `json:"gateway_pattern"`
	CheckInterval  string `json:"check_interval"`
}

// GetStatus returns the current monitor status
func (m *ProcessMonitor) GetStatus() MonitorStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return MonitorStatus{
		AssetName:      m.assetName,
		IsMonitoring:   m.isMonitoring,
		GatewayPattern: m.gatewayPattern,
		CheckInterval:  m.checkInterval.String(),
	}
}

// Global process monitor registry
var (
	processMonitors = make(map[string]*ProcessMonitor)
	monitorMu       sync.RWMutex
)

// GetProcessMonitor gets or creates a process monitor keyed by assetName.
func GetProcessMonitor(assetName string, gatewayPattern string) *ProcessMonitor {
	return GetProcessMonitorByKey(assetName, assetName, gatewayPattern)
}

// GetProcessMonitorByKey gets or creates a process monitor with an explicit instance key.
func GetProcessMonitorByKey(assetKey, displayName, gatewayPattern string) *ProcessMonitor {
	assetKey = strings.TrimSpace(assetKey)
	displayName = strings.TrimSpace(displayName)
	if assetKey == "" {
		assetKey = displayName
	}
	if displayName == "" {
		displayName = assetKey
	}
	monitorMu.Lock()
	defer monitorMu.Unlock()

	if monitor, exists := processMonitors[assetKey]; exists {
		return monitor
	}

	monitor := NewProcessMonitor(displayName, gatewayPattern)
	processMonitors[assetKey] = monitor
	return monitor
}

// RemoveProcessMonitor removes a process monitor keyed by assetName.
func RemoveProcessMonitor(assetName string) {
	RemoveProcessMonitorByKey(assetName)
}

// RemoveProcessMonitorByKey removes a process monitor by explicit instance key.
func RemoveProcessMonitorByKey(assetKey string) {
	assetKey = strings.TrimSpace(assetKey)
	if assetKey == "" {
		return
	}
	monitorMu.Lock()
	defer monitorMu.Unlock()

	if monitor, exists := processMonitors[assetKey]; exists {
		monitor.Stop()
		delete(processMonitors, assetKey)
	}
}

// GetAllMonitorStatus returns status of all process monitors
func GetAllMonitorStatus() map[string]MonitorStatus {
	monitorMu.RLock()
	defer monitorMu.RUnlock()

	result := make(map[string]MonitorStatus)
	for name, monitor := range processMonitors {
		result[name] = monitor.GetStatus()
	}
	return result
}

// StopAllMonitors stops all process monitors
func StopAllMonitors() {
	monitorMu.Lock()
	defer monitorMu.Unlock()

	for _, monitor := range processMonitors {
		monitor.Stop()
	}
	processMonitors = make(map[string]*ProcessMonitor)
}
