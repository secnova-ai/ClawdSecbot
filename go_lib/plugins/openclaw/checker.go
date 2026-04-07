package openclaw

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"go_lib/core"
	"go_lib/core/cmdutil"
)

// checkPermissions validates file and directory permissions
func checkPermissions(configPath string, risks *[]core.Risk) {
	info, err := os.Stat(configPath)
	if err != nil {
		return
	}

	if runtime.GOOS == "windows" {
		configACL, aclErr := checkWindowsACL(configPath)
		if aclErr != nil {
			*risks = append(*risks, core.Risk{
				ID:          "config_perm_unsafe",
				Title:       "Config File Permission Unsafe",
				Description: fmt.Sprintf("Failed to verify config file ACL: %v", aclErr),
				Level:       core.RiskLevelCritical,
				Args:        map[string]interface{}{"path": configPath, "acl_summary": "acl check failed"},
			})
		} else if !configACL.Safe {
			*risks = append(*risks, core.Risk{
				ID:          "config_perm_unsafe",
				Title:       "Config File Permission Unsafe",
				Description: fmt.Sprintf("Config file ACL is unsafe: %s", configACL.Summary),
				Level:       core.RiskLevelCritical,
				Args: map[string]interface{}{
					"path":           configPath,
					"acl_summary":    configACL.Summary,
					"acl_violations": configACL.Violations,
				},
			})
		}

		dir := filepath.Dir(configPath)
		if _, err := os.Stat(dir); err == nil {
			dirACL, aclErr := checkWindowsACL(dir)
			if aclErr != nil {
				*risks = append(*risks, core.Risk{
					ID:          "config_dir_perm_unsafe",
					Title:       "Config Directory Permission Unsafe",
					Description: fmt.Sprintf("Failed to verify config directory ACL: %v", aclErr),
					Level:       core.RiskLevelCritical,
					Args:        map[string]interface{}{"path": dir, "acl_summary": "acl check failed"},
				})
			} else if !dirACL.Safe {
				*risks = append(*risks, core.Risk{
					ID:          "config_dir_perm_unsafe",
					Title:       "Config Directory Permission Unsafe",
					Description: fmt.Sprintf("Config directory ACL is unsafe: %s", dirACL.Summary),
					Level:       core.RiskLevelCritical,
					Args: map[string]interface{}{
						"path":           dir,
						"acl_summary":    dirACL.Summary,
						"acl_violations": dirACL.Violations,
					},
				})
			}
		}
		return
	}

	// Check config file permissions (should be 600)
	perm := info.Mode().Perm()
	if perm != 0600 {
		*risks = append(*risks, core.Risk{
			ID:          "config_perm_unsafe",
			Title:       "Config File Permission Unsafe",
			Description: fmt.Sprintf("Config file permissions are %o, expected 600", perm),
			Level:       core.RiskLevelCritical,
			Args:        map[string]interface{}{"path": configPath, "current": fmt.Sprintf("%o", perm)},
		})
	}

	// Check directory permissions (should be 700)
	dir := filepath.Dir(configPath)
	dirInfo, err := os.Stat(dir)
	if err == nil {
		dirPerm := dirInfo.Mode().Perm()
		if dirPerm != 0700 {
			*risks = append(*risks, core.Risk{
				ID:          "config_dir_perm_unsafe",
				Title:       "Config Directory Permission Unsafe",
				Description: fmt.Sprintf("Config directory permissions are %o, expected 700", dirPerm),
				Level:       core.RiskLevelCritical,
				Args:        map[string]interface{}{"path": dir, "current": fmt.Sprintf("%o", dirPerm)},
			})
		}
	}
}

// checkNetworkExposure validates gateway network binding and authentication
func checkNetworkExposure(config OpenclawConfig, risks *[]core.Risk) {
	bind := config.Gateway.Bind
	if bind == "" {
		bind = config.Gateway.Host
	}

	if bind != "" && bind != "127.0.0.1" && bind != "loopback" && bind != "::1" {
		*risks = append(*risks, core.Risk{
			ID:          "gateway_bind_unsafe",
			Title:       "Gateway Bound to Non-Local Interface",
			Description: fmt.Sprintf("Gateway is bound to %s, potentially exposing it to network", bind),
			Level:       core.RiskLevelHigh,
			Args:        map[string]interface{}{"bind": bind},
		})
	}

	// Check Auth Mode
	auth := config.Gateway.Auth
	if !auth.Enabled && auth.Mode == "" {
		*risks = append(*risks, core.Risk{
			ID:          "gateway_auth_disabled",
			Title:       "Gateway Authentication Disabled",
			Description: "Gateway authentication is disabled (Fail-Closed violation)",
			Level:       core.RiskLevelCritical,
		})
	} else if auth.Mode == "password" {
		*risks = append(*risks, core.Risk{
			ID:          "gateway_auth_password_mode",
			Title:       "Weak Authentication Mode (Password)",
			Description: "Password mode is discouraged due to brute-force risks. Use Token mode instead.",
			Level:       core.RiskLevelMedium,
		})
		if len(auth.Password) < 12 {
			*risks = append(*risks, core.Risk{
				ID:          "gateway_weak_password",
				Title:       "Gateway Password Weak",
				Description: "Gateway password is too short (less than 12 characters)",
				Level:       core.RiskLevelHigh,
			})
		}
	} else if auth.Mode == "token" {
		if len(auth.Token) < 32 && !strings.HasPrefix(auth.Token, "${") {
			*risks = append(*risks, core.Risk{
				ID:          "gateway_weak_token",
				Title:       "Gateway Token Too Short",
				Description: "Gateway token should be at least 32 characters (128+ bits entropy)",
				Level:       core.RiskLevelHigh,
			})
		}
	} else if auth.Mode == "" && auth.Enabled {
		if len(auth.Password) < 12 {
			*risks = append(*risks, core.Risk{
				ID:          "gateway_weak_password",
				Title:       "Gateway Password Weak",
				Description: "Gateway password is too short (less than 12 characters)",
				Level:       core.RiskLevelHigh,
			})
		}
	}
}

// checkSandbox validates sandbox configuration for agents
func checkSandbox(config OpenclawConfig, rawConfig map[string]interface{}, risks *[]core.Risk) {
	mode := config.Agents.Defaults.Sandbox.Mode
	if mode == "none" {
		*risks = append(*risks, core.Risk{
			ID:          "sandbox_disabled_default",
			Title:       "Sandbox Disabled by Default",
			Description: "Default sandbox mode is set to 'none'",
			Level:       core.RiskLevelCritical,
		})
	}

	if agents, ok := rawConfig["agents"].(map[string]interface{}); ok {
		for name, agentVal := range agents {
			if name == "defaults" {
				continue
			}
			if agentMap, ok := agentVal.(map[string]interface{}); ok {
				if sandbox, ok := agentMap["sandbox"].(map[string]interface{}); ok {
					if m, ok := sandbox["mode"].(string); ok && m == "none" {
						*risks = append(*risks, core.Risk{
							ID:          "sandbox_disabled_agent",
							Title:       fmt.Sprintf("Sandbox Disabled for Agent: %s", name),
							Description: fmt.Sprintf("Agent '%s' has sandbox mode set to 'none'", name),
							Level:       core.RiskLevelCritical,
							Args:        map[string]interface{}{"agent": name},
						})
					}
				}
			}
		}
	}
}

// checkLogging validates logging configuration and permissions
func checkLogging(config OpenclawConfig, configPath string, risks *[]core.Risk) {
	if config.Logging.RedactSensitive == "off" || config.Logging.RedactSensitive == "" {
		*risks = append(*risks, core.Risk{
			ID:          "logging_redact_off",
			Title:       "Sensitive Data Redaction Disabled",
			Description: "Logging redaction is set to 'off' or not configured",
			Level:       core.RiskLevelHigh,
			Args:        map[string]interface{}{"config_path": configPath},
		})
	}

	logDir := filepath.Join(filepath.Dir(configPath), "logs")
	if info, err := os.Stat(logDir); err == nil {
		if runtime.GOOS == "windows" {
			logACL, aclErr := checkWindowsACL(logDir)
			if aclErr != nil {
				*risks = append(*risks, core.Risk{
					ID:          "log_dir_perm_unsafe",
					Title:       "Log Directory Permission Unsafe",
					Description: fmt.Sprintf("Failed to verify log directory ACL: %v", aclErr),
					Level:       core.RiskLevelCritical,
					Args: map[string]interface{}{
						"path":        logDir,
						"config_path": configPath,
						"acl_summary": "acl check failed",
					},
				})
				return
			}
			if !logACL.Safe {
				*risks = append(*risks, core.Risk{
					ID:          "log_dir_perm_unsafe",
					Title:       "Log Directory Permission Unsafe",
					Description: fmt.Sprintf("Log directory ACL is unsafe: %s", logACL.Summary),
					Level:       core.RiskLevelCritical,
					Args: map[string]interface{}{
						"path":           logDir,
						"config_path":    configPath,
						"acl_summary":    logACL.Summary,
						"acl_violations": logACL.Violations,
					},
				})
			}
			return
		}

		if info.Mode().Perm() != 0700 {
			*risks = append(*risks, core.Risk{
				ID:          "log_dir_perm_unsafe",
				Title:       "Log Directory Permission Unsafe",
				Description: fmt.Sprintf("Log directory permissions are %o, expected 700", info.Mode().Perm()),
				Level:       core.RiskLevelMedium,
				Args:        map[string]interface{}{"config_path": configPath},
			})
		}
	}
}

// checkCredentialsInConfig detects potential plaintext secrets
func checkCredentialsInConfig(configPath string, risks *[]core.Risk) {
	content, err := ioutil.ReadFile(configPath)
	if err != nil {
		return
	}

	patterns := []string{"sk-", "ghp_", "ghu_", "Bearer ", "AWS_ACCESS_KEY_ID"}
	for _, p := range patterns {
		if strings.Contains(string(content), p) {
			*risks = append(*risks, core.Risk{
				ID:          "plaintext_secrets",
				Title:       "Plaintext Secrets Detected in Config",
				Description: fmt.Sprintf("Found potential secret matching pattern '%s' in config file", p),
				Level:       core.RiskLevelCritical,
				Args:        map[string]interface{}{"pattern": p},
			})
			break
		}
	}
}

// checkOneClickRCEVulnerability detects 1-click RCE vulnerability (CVSS 10.0)
// This vulnerability exists in openclaw versions before 2026.1.24-1
func checkOneClickRCEVulnerability(risks *[]core.Risk) {
	// Try to get OpenClaw version
	version := getOpenClawVersion()

	// If version check fails or version is vulnerable, report the risk
	if version == "" || isVulnerableVersion(version) {
		*risks = append(*risks, core.Risk{
			ID:    "openclaw_1click_rce_vulnerability",
			Title: "1-click RCE Vulnerability (CVSS 10.0)",
			Description: "OpenClaw存在严重的1-click RCE漏洞,攻击者可通过诱导用户访问恶意网站,利用Gateway URL参数覆盖、Token泄露和WebSocket Origin绕过三个漏洞链完成远程代码执行." +
				"漏洞链：(1) URL参数gatewayUrl可被覆盖无白名单验证 → (2) Token明文存储在localStorage被泄露 → (3) WebSocket未验证Origin导致攻击者可直连本地Gateway → (4) 完成认证并执行任意命令." +
				"受影响版本：< 2026.1.24-1,建议升级至最新版本并检查是否包含修复补丁.",
			Level: core.RiskLevelCritical,
			Args: map[string]interface{}{
				"cvss_score":       "10.0",
				"attack_vector":    "Network",
				"attack_chain":     "URL覆盖 + Token泄露 + Origin绕过 + RCE",
				"affected_files":   "ui/src/ui/app-settings.ts, ui/src/ui/app-gateway.ts, ui/src/ui/storage.ts, src/gateway/auth.ts, src/gateway/server/ws-connection.ts",
				"check_version":    "openclaw --version",
				"vulnerable_below": "2026.1.24-1",
				"current_version":  version, // 添加当前检测到的版本
			},
		})
	}
}

// getOpenClawVersion tries to get the OpenClaw version
func getOpenClawVersion() string {
	// Try to execute: openclaw --version
	cmd := cmdutil.Command("openclaw", "--version")
	output, err := cmd.Output()
	if err != nil {
		// Command failed, return empty string
		return ""
	}

	// Parse version from output
	// Expected format: "2026.2.2-3" or similar
	version := strings.TrimSpace(string(output))
	return version
}

// isVulnerableVersion checks if the version is vulnerable
func isVulnerableVersion(version string) bool {
	if version == "" {
		// Unknown version, assume vulnerable for safety
		return true
	}

	// Parse version string (format: YYYY.M.D-BUILD)
	// Example: "2026.1.24-1"
	parts := strings.Split(version, "-")
	if len(parts) < 2 {
		return true // Cannot parse, assume vulnerable
	}

	datePart := parts[0]
	buildPart := parts[1]

	// Compare with fix version: 2026.1.24-1
	// Versions before 2026.1.24-1 are vulnerable
	if datePart < "2026.1.24" {
		return true
	} else if datePart == "2026.1.24" {
		// Same date, compare build number
		buildNum, err := strconv.Atoi(buildPart)
		if err != nil || buildNum < 1 {
			return true
		}
	}

	// Version is 2026.1.24-1 or later, not vulnerable
	return false
}
