package hermes

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go_lib/core"
)

func checkPermissions(configPath string, risks *[]core.Risk) {
	info, err := os.Stat(configPath)
	if err != nil {
		return
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			*risks = append(*risks, core.Risk{
				ID:          "config_perm_unsafe",
				Title:       "Config File Permission Unsafe",
				Description: fmt.Sprintf("Config file permissions are %o, expected 600", perm),
				Level:       core.RiskLevelHigh,
				Args: map[string]interface{}{
					"path": configPath,
				},
			})
		}

		dir := filepath.Dir(configPath)
		if dirInfo, err := os.Stat(dir); err == nil {
			if dirPerm := dirInfo.Mode().Perm(); dirPerm != 0700 {
				*risks = append(*risks, core.Risk{
					ID:          "config_dir_perm_unsafe",
					Title:       "Config Directory Permission Unsafe",
					Description: fmt.Sprintf("Config directory permissions are %o, expected 700", dirPerm),
					Level:       core.RiskLevelMedium,
					Args: map[string]interface{}{
						"path": dir,
					},
				})
			}
		}
	}
}

func checkTerminalBackend(config *HermesConfig, risks *[]core.Risk) {
	if config == nil {
		return
	}
	backend := strings.ToLower(strings.TrimSpace(config.Terminal.Backend))
	if backend == "" || backend == "local" {
		*risks = append(*risks, core.Risk{
			ID:          "terminal_backend_local",
			Title:       "Terminal Backend Is Local",
			Description: "terminal.backend is local; agent operations execute directly on host without remote isolation",
			Level:       core.RiskLevelMedium,
		})
	}
}

func checkApprovalsMode(config *HermesConfig, risks *[]core.Risk) {
	if config == nil {
		return
	}
	mode := strings.ToLower(strings.TrimSpace(config.Approvals.Mode))
	if mode == "off" || mode == "never" || mode == "yolo" {
		*risks = append(*risks, core.Risk{
			ID:          "approvals_mode_disabled",
			Title:       "Approvals Mode Disabled",
			Description: fmt.Sprintf("approvals.mode is '%s'; risky actions may run without interactive confirmation", mode),
			Level:       core.RiskLevelHigh,
			Args: map[string]interface{}{
				"mode": mode,
			},
		})
	}
}

func checkRedactSecrets(config *HermesConfig, raw map[string]interface{}, risks *[]core.Risk) {
	if config == nil {
		return
	}
	if config.Security.RedactSecrets != nil {
		if !*config.Security.RedactSecrets {
			*risks = append(*risks, core.Risk{
				ID:          "redact_secrets_disabled",
				Title:       "Secret Redaction Disabled",
				Description: "security.redact_secrets is false; sensitive tokens may leak into logs",
				Level:       core.RiskLevelHigh,
			})
		}
		return
	}

	if value, ok := getNestedBool(raw, "security", "redact_secrets"); ok && !value {
		*risks = append(*risks, core.Risk{
			ID:          "redact_secrets_disabled",
			Title:       "Secret Redaction Disabled",
			Description: "security.redact_secrets is false; sensitive tokens may leak into logs",
			Level:       core.RiskLevelHigh,
		})
	}
}

func checkModelBaseURL(config *HermesConfig, risks *[]core.Risk) {
	if config == nil {
		return
	}
	provider := strings.ToLower(strings.TrimSpace(config.Model.Provider))
	baseURL := strings.TrimSpace(config.Model.BaseURL)
	if provider != "custom" || baseURL == "" {
		return
	}
	if isLocalURL(baseURL) {
		return
	}

	*risks = append(*risks, core.Risk{
		ID:          "model_base_url_public",
		Title:       "Custom Model Endpoint Is Public",
		Description: fmt.Sprintf("model.provider is custom and model.base_url points to non-local endpoint: %s", baseURL),
		Level:       core.RiskLevelMedium,
		Args: map[string]interface{}{
			"base_url": baseURL,
		},
	})
}

func isLocalURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return false
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
