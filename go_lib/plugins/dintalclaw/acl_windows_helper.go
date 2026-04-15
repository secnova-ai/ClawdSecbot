package dintalclaw

import (
	"fmt"
	"os"
	"os/user"
	"strings"

	"go_lib/core/cmdutil"
)

type aclCheckResult struct {
	Safe       bool
	Summary    string
	Violations []string
}

var runCommandCombinedOutput = func(name string, args ...string) ([]byte, error) {
	return cmdutil.Command(name, args...).CombinedOutput()
}

var getCurrentUserName = func() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

func checkWindowsACL(path string) (aclCheckResult, error) {
	out, err := runCommandCombinedOutput("icacls", path)
	if err != nil {
		return aclCheckResult{}, fmt.Errorf("icacls check failed: %w; output: %s", err, strings.TrimSpace(string(out)))
	}

	currentUser, err := getCurrentUserName()
	if err != nil {
		return aclCheckResult{}, fmt.Errorf("failed to resolve current user: %w", err)
	}

	violations := parseACLViolations(path, string(out), buildAllowedPrincipals(currentUser))
	summary := "acl safe"
	if len(violations) > 0 {
		summary = "acl has non-whitelisted principal access"
	}

	return aclCheckResult{
		Safe:       len(violations) == 0,
		Summary:    summary,
		Violations: violations,
	}, nil
}

func applyWindowsACL(path string, isDirectory bool) (string, error) {
	currentUser, err := getCurrentUserName()
	if err != nil {
		return "", fmt.Errorf("failed to resolve current user: %w", err)
	}

	principalACL := []string{
		fmt.Sprintf("%s:(F)", currentUser),
		"SYSTEM:(F)",
		"Administrators:(F)",
	}

	if isDirectory {
		principalACL = []string{
			fmt.Sprintf("%s:(OI)(CI)F", currentUser),
			"SYSTEM:(OI)(CI)F",
			"Administrators:(OI)(CI)F",
		}
	}

	args := append([]string{path, "/inheritance:r", "/grant:r"}, principalACL...)
	out, err := runCommandCombinedOutput("icacls", args...)
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		return trimmed, fmt.Errorf("icacls grant failed: %w; output: %s", err, trimmed)
	}

	for _, principal := range []string{"Everyone", "Users", "Authenticated Users", "*S-1-1-0", "*S-1-5-32-545", "*S-1-5-11"} {
		_, _ = runCommandCombinedOutput("icacls", path, "/remove:g", principal)
	}

	checkRes, checkErr := checkWindowsACL(path)
	if checkErr != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("acl verify failed: %w", checkErr)
	}
	if !checkRes.Safe {
		return strings.TrimSpace(string(out)), fmt.Errorf("acl verify failed: %s (%s)", checkRes.Summary, strings.Join(checkRes.Violations, "; "))
	}

	return strings.TrimSpace(string(out)), nil
}

func parseACLViolations(path, rawOutput string, allowSet map[string]struct{}) []string {
	lines := strings.Split(rawOutput, "\n")
	violations := make([]string, 0)

	pathPrefix := strings.ToUpper(strings.TrimSpace(path)) + " "

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		upperLine := strings.ToUpper(line)
		if strings.HasPrefix(upperLine, "SUCCESSFULLY PROCESSED") || strings.HasPrefix(upperLine, "FAILED PROCESSING") {
			continue
		}

		if strings.HasPrefix(upperLine, pathPrefix) {
			line = strings.TrimSpace(line[len(pathPrefix):])
		}

		colon := strings.Index(line, ":")
		if colon <= 0 {
			continue
		}

		principal := normalizePrincipal(line[:colon])
		perms := strings.TrimSpace(line[colon+1:])
		if principal == "" || perms == "" {
			continue
		}
		if strings.Contains(strings.ToUpper(perms), "DENY") {
			continue
		}
		if _, ok := allowSet[principal]; ok {
			continue
		}
		if !isPrincipalAccessDangerous(principal, perms) {
			continue
		}

		violations = append(violations, fmt.Sprintf("%s => %s", principal, perms))
	}

	return violations
}

func hasReadAccess(perms string) bool {
	upper := strings.ToUpper(perms)
	for _, marker := range []string{
		"(F)", "(M)", "(R)", "(RX)", "(GR)", "(GE)", "(GA)",
	} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

func hasWriteAccess(perms string) bool {
	upper := strings.ToUpper(perms)
	for _, marker := range []string{
		"(F)", "(M)", "(W)", "(WD)", "(AD)", "(D)", "(DC)", "(GW)", "(GA)",
	} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

func isBroadPrincipal(principal string) bool {
	switch normalizePrincipal(principal) {
	case normalizePrincipal("Everyone"),
		normalizePrincipal("BUILTIN\\Users"),
		normalizePrincipal("Users"),
		normalizePrincipal("Authenticated Users"),
		normalizePrincipal("NT AUTHORITY\\Authenticated Users"),
		normalizePrincipal("S-1-1-0"),
		normalizePrincipal("S-1-5-32-545"),
		normalizePrincipal("S-1-5-11"):
		return true
	default:
		return false
	}
}

func isPrincipalAccessDangerous(principal, perms string) bool {
	if isBroadPrincipal(principal) {
		return hasReadAccess(perms)
	}
	return hasWriteAccess(perms)
}

func buildAllowedPrincipals(currentUser string) map[string]struct{} {
	allowed := map[string]struct{}{
		normalizePrincipal("SYSTEM"):                  {},
		normalizePrincipal("NT AUTHORITY\\SYSTEM"):    {},
		normalizePrincipal("S-1-5-18"):                {},
		normalizePrincipal("Administrators"):          {},
		normalizePrincipal("BUILTIN\\Administrators"): {},
		normalizePrincipal("S-1-5-32-544"):            {},
	}

	addPrincipalAliases(allowed, currentUser)
	addPrincipalAliases(allowed, os.Getenv("USERNAME"))

	return allowed
}

func addPrincipalAliases(allowSet map[string]struct{}, principal string) {
	p := normalizePrincipal(principal)
	if p == "" {
		return
	}
	allowSet[p] = struct{}{}

	if idx := strings.LastIndex(p, `\`); idx >= 0 && idx+1 < len(p) {
		allowSet[p[idx+1:]] = struct{}{}
	}
	if idx := strings.LastIndex(p, `/`); idx >= 0 && idx+1 < len(p) {
		allowSet[p[idx+1:]] = struct{}{}
	}
}

func normalizePrincipal(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "*")
	return strings.ToUpper(p)
}
