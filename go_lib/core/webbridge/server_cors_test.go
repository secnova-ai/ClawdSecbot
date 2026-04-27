package webbridge

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsAllowedBrowserOrigin(t *testing.T) {
	tests := []struct {
		origin string
		allow  bool
	}{
		{"", true},
		{"http://127.0.0.1:3000", true},
		{"http://localhost:3000", true},
		{"https://[::1]:3000", true},
		{"https://example.com", false},
		{"file:///tmp/index.html", false},
	}

	for _, tt := range tests {
		got := isAllowedBrowserOrigin(tt.origin)
		if got != tt.allow {
			t.Fatalf("isAllowedBrowserOrigin(%q) = %v, want %v", tt.origin, got, tt.allow)
		}
	}
}

func TestWithCORS_BlocksDisallowedOrigin(t *testing.T) {
	h := withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rpc/GetProviderModels", nil)
	req.Header.Set("Origin", "https://evil.example")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disallowed origin, got %d", rr.Code)
	}
}

func TestWithCORS_AllowsLoopbackOrigin(t *testing.T) {
	h := withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rpc/GetProviderModels", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for allowed origin, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:5173" {
		t.Fatalf("unexpected allow-origin header: %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}
