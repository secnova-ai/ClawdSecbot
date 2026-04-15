package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"go_lib/core/logging"
)

// responseCapture wraps http.ResponseWriter to capture the status code
type responseCapture struct {
	http.ResponseWriter
	statusCode int
}

func newResponseCapture(w http.ResponseWriter) *responseCapture {
	return &responseCapture{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rc *responseCapture) WriteHeader(code int) {
	rc.statusCode = code
	rc.ResponseWriter.WriteHeader(code)
}

// loggingMiddleware logs request method, path, status code, and duration
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rc := newResponseCapture(w)

		next.ServeHTTP(rc, r)

		duration := time.Since(start)
		logging.Info("API: %s %s -> %d (%v)", r.Method, r.URL.Path, rc.statusCode, duration)
	})
}

// authMiddleware verifies the Bearer token in Authorization header
// It uses constant-time comparison to prevent timing attacks
func authMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")

		// Extract Bearer token
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			Error(w, http.StatusUnauthorized, CodeAuthFailed, "missing or invalid authorization header")
			return
		}

		providedToken := strings.TrimPrefix(authHeader, "Bearer ")

		// Constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(providedToken), []byte(token)) != 1 {
			Error(w, http.StatusUnauthorized, CodeAuthFailed, "invalid token")
			return
		}

		next.ServeHTTP(w, r)
	})
}
