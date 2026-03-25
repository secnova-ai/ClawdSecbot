package api

import "net/http"

// handleStatus handles GET /api/v1/status
// Returns API service export status, with focus on enabled state and export path.
func (s *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	Success(w, s.getExportStatus())
}

// getExportStatus returns the current export service status.
func (s *APIServer) getExportStatus() ExportStatusInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.exportService == nil {
		return ExportStatusInfo{
			Enabled:    false,
			ExportDir:  "",
			StatusFile: statusFileName,
			AuditFile:  auditFileName,
			EventsFile: eventsFileName,
		}
	}

	if impl, ok := s.exportService.(*ExportServiceImpl); ok {
		return impl.ExportStatus()
	}

	// Fallback for interface-only implementations.
	return ExportStatusInfo{
		Enabled:    true,
		StatusFile: statusFileName,
		AuditFile:  auditFileName,
		EventsFile: eventsFileName,
	}
}
