package proxy

import (
	"strings"
	"testing"
)

func TestRedactProxyConfigForLogRedactsNestedSecrets(t *testing.T) {
	raw := `{
		"security_model":{"api_key":"sk-live-secret-value-1234567890","base_url":"https://example.test"},
		"bot_model":{"auth_token":"gateway-token-secret-value","password":"plain-password"},
		"runtime":{"daily_token_limit":1000}
	}`

	got := redactProxyConfigForLog(raw)
	for _, secret := range []string{
		"sk-live-secret-value-1234567890",
		"gateway-token-secret-value",
		"plain-password",
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("expected sensitive value to be redacted, got: %s", got)
		}
	}
	if !strings.Contains(got, "[REDACTED_SECRET]") {
		t.Fatalf("expected redacted marker, got: %s", got)
	}
	if !strings.Contains(got, "daily_token_limit") {
		t.Fatalf("expected non-secret config to remain, got: %s", got)
	}
}
