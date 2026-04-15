package api

import (
	"net/http"

	"go_lib/core/logging"
)

// handleExportStart handles POST /api/v1/export/start
// Starts the export service for data export.
func (s *APIServer) handleExportStart(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if export service already exists and is running
	if s.exportService != nil {
		if impl, ok := s.exportService.(*ExportServiceImpl); ok && impl.IsRunning() {
			Success(w, "export service already started")
			return
		}
	}

	// Create and start new export service
	exportSvc := NewExportService()
	if err := exportSvc.Start(); err != nil {
		logging.Error("API: Failed to start export service: %v", err)
		Error(w, http.StatusInternalServerError, CodeInternalError, "failed to start export service: "+err.Error())
		return
	}

	s.exportService = exportSvc
	logging.Info("API: export service starting")

	Success(w, "export service starting")
}

// handleExportStop handles POST /api/v1/export/stop
// Stops the export service.
func (s *APIServer) handleExportStop(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.exportService == nil {
		Success(w, "export service already stopped")
		return
	}

	if err := s.exportService.Stop(); err != nil {
		logging.Warning("API: Failed to stop export service: %v", err)
		Error(w, http.StatusInternalServerError, CodeInternalError, "failed to stop export service: "+err.Error())
		return
	}

	s.exportService = nil
	logging.Info("API: Export service stopped")

	Success(w, "export service stopping")
}
