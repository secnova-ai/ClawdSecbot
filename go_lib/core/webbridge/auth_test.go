package webbridge

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"go_lib/core/repository"
)

func TestWebAuthInitializesAndAuthenticates(t *testing.T) {
	initTestDB(t)

	auth := NewWebAuthManager()
	bootstrap, err := auth.EnsureInitialized()
	if err != nil {
		t.Fatalf("EnsureInitialized failed: %v", err)
	}
	if !bootstrap.GeneratedInitialPassword {
		t.Fatalf("expected generated initial password")
	}
	if bootstrap.Username != webAuthUsername {
		t.Fatalf("unexpected username: %q", bootstrap.Username)
	}
	if len([]rune(bootstrap.InitialPassword)) != 6 {
		t.Fatalf("initial password should be 6 runes, got %q", bootstrap.InitialPassword)
	}
	for _, r := range bootstrap.InitialPassword {
		if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
			t.Fatalf("initial password should contain only letters and digits, got %q", bootstrap.InitialPassword)
		}
	}
	if bootstrap.Token == "" {
		t.Fatalf("expected bootstrap token")
	}

	token, err := auth.Login(webAuthUsername, bootstrap.InitialPassword)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rpc/GetPluginsFFI", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if !auth.AuthenticateRequest(req) {
		t.Fatalf("expected authenticated request")
	}
}

func TestWebAuthChangePasswordInvalidatesSessions(t *testing.T) {
	initTestDB(t)

	auth := NewWebAuthManager()
	bootstrap, err := auth.EnsureInitialized()
	if err != nil {
		t.Fatalf("EnsureInitialized failed: %v", err)
	}
	token, err := auth.Login(webAuthUsername, bootstrap.InitialPassword)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if err := auth.ChangePassword(bootstrap.InitialPassword, "NewPwd9"); err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rpc/GetPluginsFFI", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if auth.AuthenticateRequest(req) {
		t.Fatalf("old token should be invalid after password change")
	}
	if _, err := auth.Login(webAuthUsername, bootstrap.InitialPassword); err == nil {
		t.Fatalf("old password should not authenticate")
	}
	if _, err := auth.Login(webAuthUsername, "NewPwd9"); err != nil {
		t.Fatalf("new password should authenticate: %v", err)
	}
}

func initTestDB(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	if _, err := repository.InitDBWithVersion(
		filepath.Join(tmp, "botsec.db"),
		"1.0.3",
		filepath.Join(tmp, "version.json"),
	); err != nil {
		t.Fatalf("InitDBWithVersion failed: %v", err)
	}
	t.Cleanup(func() {
		_ = repository.CloseDB()
	})
}
