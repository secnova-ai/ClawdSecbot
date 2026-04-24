package nullclaw

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"go_lib/core"
	"go_lib/core/cmdutil"
)

// checkPermissions validates file and directory permissions.
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

func resolveGatewayHost(config NullclawConfig) string {
	host := strings.TrimSpace(config.Gateway.Host)
	if host == "" {
		host = strings.TrimSpace(config.Gateway.Bind)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return host
}

func isLocalHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	return h == "127.0.0.1" || h == "localhost" || h == "::1" || h == "loopback"
}

// checkNetworkExposure validates gateway network binding and pairing settings.
func checkNetworkExposure(config NullclawConfig, risks *[]core.Risk) {
	host := resolveGatewayHost(config)

	if !isLocalHost(host) || config.Gateway.AllowPublicBind {
		*risks = append(*risks, core.Risk{
			ID:          "gateway_bind_unsafe",
			Title:       "Gateway Bound to Public Interface",
			Description: fmt.Sprintf("Gateway host=%s allow_public_bind=%v may expose runtime externally", host, config.Gateway.AllowPublicBind),
			Level:       core.RiskLevelHigh,
			Args: map[string]interface{}{
				"host":              host,
				"allow_public_bind": config.Gateway.AllowPublicBind,
			},
		})
	}

	if !config.Gateway.RequirePairing {
		*risks = append(*risks, core.Risk{
			ID:          "gateway_auth_disabled",
			Title:       "Gateway Pairing Disabled",
			Description: "gateway.require_pairing is false; unauthenticated external callers may access the runtime",
			Level:       core.RiskLevelCritical,
		})
	}
}

// checkSandbox validates sandbox/autonomy guardrails.
func checkSandbox(config NullclawConfig, rawConfig map[string]interface{}, risks *[]core.Risk) {
	backend := strings.ToLower(strings.TrimSpace(config.Security.Sandbox.Backend))
	if backend == "none" || backend == "off" || backend == "disabled" {
		*risks = append(*risks, core.Risk{
			ID:          "sandbox_disabled_default",
			Title:       "Sandbox Disabled",
			Description: fmt.Sprintf("security.sandbox.backend is '%s'", backend),
			Level:       core.RiskLevelCritical,
		})
	}

	if !config.Autonomy.WorkspaceOnly {
		*risks = append(*risks, core.Risk{
			ID:          "autonomy_workspace_unrestricted",
			Title:       "Workspace Restriction Disabled",
			Description: "autonomy.workspace_only is false; file access is no longer constrained to workspace",
			Level:       core.RiskLevelHigh,
		})
	}

	// Compatibility check: detect legacy per-agent sandbox mode in raw config.
	if agents, ok := rawConfig["agents"].(map[string]interface{}); ok {
		for name, agentVal := range agents {
			if name == "defaults" {
				continue
			}
			agentMap, ok := agentVal.(map[string]interface{})
			if !ok {
				continue
			}
			sandbox, ok := agentMap["sandbox"].(map[string]interface{})
			if !ok {
				continue
			}
			mode, _ := sandbox["mode"].(string)
			mode = strings.ToLower(strings.TrimSpace(mode))
			if mode == "none" || mode == "off" || mode == "disabled" {
				*risks = append(*risks, core.Risk{
					ID:          "sandbox_disabled_agent",
					Title:       fmt.Sprintf("Sandbox Disabled for Agent: %s", name),
					Description: fmt.Sprintf("Agent '%s' has sandbox mode '%s'", name, mode),
					Level:       core.RiskLevelCritical,
					Args:        map[string]interface{}{"agent": name},
				})
			}
		}
	}
}

// checkLogging validates audit settings and log directory permissions.
func checkLogging(config NullclawConfig, configPath string, risks *[]core.Risk) {
	if !config.Security.Audit.Enabled {
		*risks = append(*risks, core.Risk{
			ID:          "audit_disabled",
			Title:       "Audit Log Disabled",
			Description: "security.audit.enabled is false",
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

// checkCredentialsInConfig detects potential plaintext secrets.
func checkCredentialsInConfig(configPath string, risks *[]core.Risk) {
	content, err := os.ReadFile(configPath)
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

// getNullclawVersion tries to get nullclaw version.
func getNullclawVersion() string {
	cmd := cmdutil.Command("nullclaw", "--version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func compareNullclawVersion(current, target string) (int, bool) {
	parse := func(version string) (string, int, bool, bool) {
		version = strings.TrimSpace(version)
		if version == "" {
			return "", 0, false, false
		}
		parts := strings.SplitN(version, "-", 2)
		datePart := parts[0]
		build := 0
		hasBuild := false
		if len(parts) == 2 {
			value, err := strconv.Atoi(parts[1])
			if err != nil {
				return "", 0, false, false
			}
			build = value
			hasBuild = true
		}
		dateSegs := strings.Split(datePart, ".")
		if len(dateSegs) != 3 {
			return "", 0, false, false
		}
		return datePart, build, hasBuild, true
	}

	currentDate, currentBuild, currentHasBuild, ok := parse(current)
	if !ok {
		return 0, false
	}
	targetDate, targetBuild, targetHasBuild, ok := parse(target)
	if !ok {
		return 0, false
	}
	switch {
	case currentDate < targetDate:
		return -1, true
	case currentDate > targetDate:
		return 1, true
	case currentHasBuild != targetHasBuild:
		if !currentHasBuild && targetHasBuild {
			return -1, true
		}
		return 1, true
	case currentBuild < targetBuild:
		return -1, true
	case currentBuild > targetBuild:
		return 1, true
	default:
		return 0, true
	}
}
