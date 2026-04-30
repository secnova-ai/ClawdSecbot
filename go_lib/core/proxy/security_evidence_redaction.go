package proxy

import (
	"regexp"
	"strings"
)

var securityEvidenceSecretPatterns = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`), "[REDACTED_OPENAI_KEY]"},
	{regexp.MustCompile(`ghp_[A-Za-z0-9_]{20,}`), "[REDACTED_GITHUB_TOKEN]"},
	{regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{20,}`), "[REDACTED_SLACK_TOKEN]"},
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "[REDACTED_AWS_ACCESS_KEY]"},
	{regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password|appSecret)\s*[:=]\s*['"]?[A-Za-z0-9_./+=-]{8,}`), `${1}=[REDACTED_SECRET]`},
}

func redactSecurityEvidence(value string) string {
	redacted := strings.TrimSpace(value)
	if redacted == "" {
		return ""
	}
	for _, item := range securityEvidenceSecretPatterns {
		redacted = item.pattern.ReplaceAllString(redacted, item.replacement)
	}
	return redacted
}
