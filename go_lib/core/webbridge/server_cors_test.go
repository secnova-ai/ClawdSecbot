package webbridge

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithCORS_AllowsRemoteOrigin(t *testing.T) {
	h := withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rpc/GetProviderModels", nil)
	req.Header.Set("Origin", "http://7.6.13.146:18080")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for remote origin, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("unexpected allow-origin header: %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestWithCORS_AllowsPreflight(t *testing.T) {
	h := withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/rpc/GetProviderModels", nil)
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for preflight, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("unexpected allow-origin header: %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}
