package webbridge

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticFileUsesGzipWhenAccepted(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("console.log('botsec web');\n", 128)
	if err := os.WriteFile(filepath.Join(root, "main.dart.js"), []byte(content), 0644); err != nil {
		t.Fatalf("write static file: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/main.dart.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	NewServer(root).Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected gzip content encoding, got %q", rr.Header().Get("Content-Encoding"))
	}
	vary := strings.Join(rr.Header().Values("Vary"), ",")
	if !strings.Contains(vary, "Origin") || !strings.Contains(vary, "Accept-Encoding") {
		t.Fatalf("expected Vary to include Origin, got %v", rr.Header().Values("Vary"))
	}

	reader, err := gzip.NewReader(rr.Body)
	if err != nil {
		t.Fatalf("create gzip reader: %v", err)
	}
	defer reader.Close()
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read gzip body: %v", err)
	}
	if string(raw) != content {
		t.Fatalf("unexpected gzip body")
	}
}

func TestStaticFileSkipsGzipWhenNotAccepted(t *testing.T) {
	root := t.TempDir()
	content := "plain"
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte(content), 0644); err != nil {
		t.Fatalf("write static file: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	NewServer(root).Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Encoding") != "" {
		t.Fatalf("expected no content encoding, got %q", rr.Header().Get("Content-Encoding"))
	}
	if rr.Body.String() != content {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}
