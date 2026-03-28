package api

import (
	"net/http"
)

// setupRoutes creates and configures the HTTP router with all API endpoints
// Middleware chain: logging -> auth -> handler
func (s *APIServer) setupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Register API endpoints
	// Status endpoint - returns system status
	mux.HandleFunc("GET /api/v1/status", s.handleStatus)

	// Export endpoints - control data export service
	mux.HandleFunc("POST /api/v1/export/start", s.handleExportStart)
	mux.HandleFunc("POST /api/v1/export/stop", s.handleExportStop)

	// Scan endpoint - trigger security scan
	mux.HandleFunc("POST /api/v1/scan", s.handleScan)

	// Protection policy endpoints - manage bot protection policies
	mux.HandleFunc("GET /api/v1/protection/policy", s.handleGetProtectionPolicy)
	mux.HandleFunc("POST /api/v1/protection/policy", s.handleSetProtectionPolicy)

	// Security model endpoint - manage global security model configuration
	mux.HandleFunc("POST /api/v1/security/model", s.handleSetSecurityModel)

	// Apply middleware chain: logging -> auth -> mux
	// Note: loggingMiddleware is outermost so it captures all requests including auth failures
	handler := loggingMiddleware(authMiddleware(s.token, mux))

	return handler
}
