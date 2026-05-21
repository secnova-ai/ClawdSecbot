package webbridge

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/repository"
)

func TestServerHealthEndpointReturnsSuccessEnvelope(t *testing.T) {
	server := NewServer("")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode health payload: %v", err)
	}
	if payload["success"] != true {
		t.Fatalf("expected success=true, got %#v", payload)
	}
}

func TestServerStaticServesIndexFallbackForSPARoutes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("index page"), 0600); err != nil {
		t.Fatalf("failed to write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "asset.txt"), []byte("asset"), 0600); err != nil {
		t.Fatalf("failed to write asset: %v", err)
	}

	server := NewServer(root)
	handler := server.Handler()

	assetReq := httptest.NewRequest(http.MethodGet, "/asset.txt", nil)
	assetRR := httptest.NewRecorder()
	handler.ServeHTTP(assetRR, assetReq)
	if assetRR.Code != http.StatusOK || strings.TrimSpace(assetRR.Body.String()) != "asset" {
		t.Fatalf("expected asset response, got status=%d body=%q", assetRR.Code, assetRR.Body.String())
	}

	routeReq := httptest.NewRequest(http.MethodGet, "/nested/route", nil)
	routeRR := httptest.NewRecorder()
	handler.ServeHTTP(routeRR, routeReq)
	if routeRR.Code != http.StatusOK || !strings.Contains(routeRR.Body.String(), "index page") {
		t.Fatalf("expected SPA fallback, got status=%d body=%q", routeRR.Code, routeRR.Body.String())
	}
}

func TestBootstrapInitUsesSandboxDir(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	homeDir := filepath.Join(t.TempDir(), "home")
	sandboxDir := filepath.Join(t.TempDir(), ".botsec")
	t.Cleanup(func() {
		_ = repository.CloseDB()
		logging.Close()
		logging.CloseHistory()
		logging.CloseShepherdGate()
		_ = core.GetPathManager().ResetForTest("", "")
	})
	_ = core.GetPathManager().ResetForTest("", "")

	body := []byte(`{"workspace_dir_prefix":` + strconvQuote(workspaceDir) + `,"home_dir":` + strconvQuote(homeDir) + `,"sandbox_dir":` + strconvQuote(sandboxDir) + `,"current_version":"1.0.3"}`)
	server := NewServer("")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bootstrap/init", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := core.GetPathManager().GetSandboxDir(); got != sandboxDir {
		t.Fatalf("sandbox dir = %q, want %q", got, sandboxDir)
	}
}

func strconvQuote(value string) string {
	b, _ := json.Marshal(value)
	return string(b)
}
