//go:build windows

package nullclaw

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckWindowsACL_DetectsNonWhitelistedPrincipal(t *testing.T) {
	origRunner := runCommandCombinedOutput
	origUser := getCurrentUserName
	defer func() {
		runCommandCombinedOutput = origRunner
		getCurrentUserName = origUser
	}()

	runCommandCombinedOutput = func(name string, args ...string) ([]byte, error) {
		return []byte(`C:\cfg\nullclaw.json NT AUTHORITY\SYSTEM:(I)(F)
BUILTIN\Administrators:(I)(F)
DESKTOP\alice:(I)(F)
BUILTIN\Users:(I)(RX)
Successfully processed 1 files; Failed processing 0 files`), nil
	}
	getCurrentUserName = func() (string, error) { return `DESKTOP\alice`, nil }

	res, err := checkWindowsACL(`C:\cfg\nullclaw.json`)
	if err != nil {
		t.Fatalf("checkWindowsACL returned error: %v", err)
	}
	if res.Safe {
		t.Fatalf("expected unsafe ACL, got safe result")
	}
	if len(res.Violations) == 0 {
		t.Fatalf("expected violations, got none")
	}
	if !strings.Contains(res.Violations[0], `BUILTIN\USERS`) {
		t.Fatalf("expected BUILTIN\\USERS in violations, got %v", res.Violations)
	}
}

func TestCheckWindowsACL_WhitelistedOnlyIsSafe(t *testing.T) {
	origRunner := runCommandCombinedOutput
	origUser := getCurrentUserName
	defer func() {
		runCommandCombinedOutput = origRunner
		getCurrentUserName = origUser
	}()

	runCommandCombinedOutput = func(name string, args ...string) ([]byte, error) {
		return []byte(`C:\cfg\nullclaw.json NT AUTHORITY\SYSTEM:(I)(F)
BUILTIN\Administrators:(I)(F)
DESKTOP\alice:(I)(F)
Successfully processed 1 files; Failed processing 0 files`), nil
	}
	getCurrentUserName = func() (string, error) { return `DESKTOP\alice`, nil }

	res, err := checkWindowsACL(`C:\cfg\nullclaw.json`)
	if err != nil {
		t.Fatalf("checkWindowsACL returned error: %v", err)
	}
	if !res.Safe {
		t.Fatalf("expected safe ACL, got violations: %v", res.Violations)
	}
}

func TestCheckWindowsACL_CodexSandboxUsersAllowed(t *testing.T) {
	origRunner := runCommandCombinedOutput
	origUser := getCurrentUserName
	defer func() {
		runCommandCombinedOutput = origRunner
		getCurrentUserName = origUser
	}()

	runCommandCombinedOutput = func(name string, args ...string) ([]byte, error) {
		return []byte(`C:\cfg NT AUTHORITY\SYSTEM:(OI)(CI)(F)
BUILTIN\Administrators:(OI)(CI)(F)
DESKTOP\alice:(OI)(CI)(F)
HUDYA89A\CODEXSANDBOXUSERS:(OI)(CI)(RX)
Successfully processed 1 files; Failed processing 0 files`), nil
	}
	getCurrentUserName = func() (string, error) { return `DESKTOP\alice`, nil }

	res, err := checkWindowsACL(`C:\cfg`)
	if err != nil {
		t.Fatalf("checkWindowsACL returned error: %v", err)
	}
	if !res.Safe {
		t.Fatalf("expected safe ACL, got violations: %v", res.Violations)
	}
}

func TestApplyWindowsACL_IncludesCommandOutputOnFailure(t *testing.T) {
	origRunner := runCommandCombinedOutput
	origUser := getCurrentUserName
	defer func() {
		runCommandCombinedOutput = origRunner
		getCurrentUserName = origUser
	}()

	getCurrentUserName = func() (string, error) { return `DESKTOP\alice`, nil }
	runCommandCombinedOutput = func(name string, args ...string) ([]byte, error) {
		if len(args) > 1 && args[1] == "/inheritance:r" {
			return []byte("Access is denied."), errors.New("exit status 5")
		}
		return []byte("ok"), nil
	}

	_, err := applyWindowsACL(`C:\cfg\nullclaw.json`, false)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Access is denied.") {
		t.Fatalf("expected command output in error, got: %v", err)
	}
}

func TestFixPermissionByPlatform_UsesACLForThreeCases(t *testing.T) {
	origApply := applyACLForPath
	defer func() { applyACLForPath = origApply }()

	var calls []string
	applyACLForPath = func(path string, isDirectory bool) (string, error) {
		if isDirectory {
			calls = append(calls, "dir:"+path)
		} else {
			calls = append(calls, "file:"+path)
		}
		return "applied", nil
	}

	_, err := fixPermissionByPlatform(`C:\cfg\nullclaw.json`, 0600, false, "u1", "w1")
	if err != nil {
		t.Fatalf("unexpected error for config file: %v", err)
	}
	_, err = fixPermissionByPlatform(`C:\cfg`, 0700, true, "u2", "w2")
	if err != nil {
		t.Fatalf("unexpected error for config dir: %v", err)
	}
	_, err = fixPermissionByPlatform(`C:\cfg\logs`, 0700, true, "u3", "w3")
	if err != nil {
		t.Fatalf("unexpected error for log dir: %v", err)
	}

	expected := []string{
		`file:C:\cfg\nullclaw.json`,
		`dir:C:\cfg`,
		`dir:C:\cfg\logs`,
	}
	if len(calls) != len(expected) {
		t.Fatalf("expected %d ACL calls, got %d (%v)", len(expected), len(calls), calls)
	}
	for i := range expected {
		if calls[i] != expected[i] {
			t.Fatalf("unexpected call[%d]: want %s, got %s", i, expected[i], calls[i])
		}
	}
}
