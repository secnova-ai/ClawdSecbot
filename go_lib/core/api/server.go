package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/repository"
)

const (
	discoveryFileName = "api.json"
	tokenBytes        = 32 // 32 bytes = 64 hex characters
	shutdownTimeout   = 5 * time.Second
)

// ExportService is a placeholder interface for the export service.
// The actual implementation will be provided in Task 2.
type ExportService interface {
	// Stop gracefully stops the export service
	Stop() error
}

// DiscoveryInfo represents the API discovery file content
type DiscoveryInfo struct {
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	Token     string `json:"token"`
	URL       string `json:"url"`
	StartedAt string `json:"startedAt"`
}

// APIServer is the HTTP API server for external integrations
type APIServer struct {
	httpServer    *http.Server
	token         string
	port          int
	exportService ExportService
	mu            sync.Mutex
	running       bool
}

// NewAPIServer creates a new APIServer instance
func NewAPIServer() *APIServer {
	return &APIServer{}
}

// Start starts the API server on the specified port
// It generates a random token, creates a discovery file, and starts the HTTP server
func (s *APIServer) Start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("API server is already running")
	}

	// Generate random token
	tokenBytes := make([]byte, tokenBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("failed to generate token: %w", err)
	}
	s.token = hex.EncodeToString(tokenBytes)
	s.port = port

	// Clean up any residual discovery file before starting
	if err := s.cleanupDiscoveryFile(); err != nil {
		logging.Warning("API: failed to cleanup residual discovery file: %v", err)
	}

	// Setup HTTP server
	handler := s.setupRoutes()
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: handler,
	}

	// Create listener to get the actual port if 0 was specified
	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.httpServer.Addr, err)
	}

	// Get actual port from listener
	actualPort := listener.Addr().(*net.TCPAddr).Port
	s.port = actualPort

	// Write discovery file
	if err := s.writeDiscoveryFile(); err != nil {
		listener.Close()
		return fmt.Errorf("failed to write discovery file: %w", err)
	}

	// Start server in goroutine
	s.running = true
	go func() {
		logging.Info("API: Server starting on 127.0.0.1:%d", actualPort)
		if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			logging.Error("API: Server error: %v", err)
		}
	}()

	logging.Info("API: Server started successfully, port=%d", actualPort)
	return nil
}

// Stop gracefully stops the API server
func (s *APIServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	logging.Info("API: Stopping server...")

	// Cleanup export service if exists
	if s.exportService != nil {
		if err := s.exportService.Stop(); err != nil {
			logging.Warning("API: Failed to stop export service: %v", err)
		}
		s.exportService = nil
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		logging.Error("API: Server shutdown error: %v", err)
	}

	// Remove discovery file
	if err := s.removeDiscoveryFile(); err != nil {
		logging.Warning("API: Failed to remove discovery file: %v", err)
	}

	s.running = false
	s.token = ""
	s.port = 0
	s.httpServer = nil

	logging.Info("API: Server stopped")
	return nil
}

// Token returns the current authentication token
func (s *APIServer) Token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// Port returns the current server port
func (s *APIServer) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

// IsRunning returns whether the server is currently running
func (s *APIServer) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// SetExportService sets the export service instance
func (s *APIServer) SetExportService(svc ExportService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exportService = svc
}

// AppendAuditLog appends an audit entry to export/audit.jsonl when export service is active.
func (s *APIServer) AppendAuditLog(entry *AuditLogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running || s.exportService == nil || entry == nil {
		return nil
	}
	impl, ok := s.exportService.(*ExportServiceImpl)
	if !ok || !impl.IsRunning() {
		return nil
	}
	return impl.WriteAuditLog(entry)
}

// AppendSecurityEvent appends a security event to export/events.jsonl when export service is active.
func (s *APIServer) AppendSecurityEvent(entry *SecurityEventEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running || s.exportService == nil || entry == nil {
		return nil
	}
	impl, ok := s.exportService.(*ExportServiceImpl)
	if !ok || !impl.IsRunning() {
		return nil
	}
	return impl.WriteSecurityEvent(entry)
}

// getDiscoveryFilePath returns the path for the discovery file
// The discovery file is placed in the same directory as the database file
func (s *APIServer) getDiscoveryFilePath() string {
	dbPath := repository.GetDBPath()
	if dbPath != "" {
		return filepath.Join(filepath.Dir(dbPath), discoveryFileName)
	}

	pm := core.GetPathManager()
	return filepath.Join(filepath.Dir(pm.GetDBPath()), discoveryFileName)
}

// writeDiscoveryFile writes the API discovery information to a file
func (s *APIServer) writeDiscoveryFile() error {
	info := DiscoveryInfo{
		PID:       os.Getpid(),
		Port:      s.port,
		Token:     s.token,
		URL:       fmt.Sprintf("http://127.0.0.1:%d", s.port),
		StartedAt: time.Now().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal discovery info: %w", err)
	}

	filePath := s.getDiscoveryFilePath()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create discovery file directory: %w", err)
	}

	// Write with restricted permissions (0600 - owner read/write only)
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write discovery file: %w", err)
	}

	logging.Info("API: Discovery file written to %s", filePath)
	return nil
}

// removeDiscoveryFile removes the discovery file
func (s *APIServer) removeDiscoveryFile() error {
	filePath := s.getDiscoveryFilePath()
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	logging.Info("API: Discovery file removed")
	return nil
}

// cleanupDiscoveryFile removes any residual discovery file
func (s *APIServer) cleanupDiscoveryFile() error {
	return s.removeDiscoveryFile()
}
