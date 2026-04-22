package openclaw

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
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

var openClawVersionPattern = regexp.MustCompile(`(?i)\bv?(\d{4})\.(\d{1,2})\.(\d{1,2})(?:-(\d+))?\b`)

type openClawVersion struct {
	year  int
	month int
	day   int
	build int
}

type openClawConfigAdvisory struct {
	ID           string
	FixedVersion string
	Summary      string
}

var configAdvisories = []openClawConfigAdvisory{
	{
		ID:           "GHSA-qvr7-g57c-mrc7 / CVE-2026-32970",
		FixedVersion: "2026.3.11",
		Summary:      "Unavailable local auth SecretRefs can fall through to remote credentials in local mode.",
	},
	{
		ID:           "GHSA-mj59-h3q9-ghfh",
		FixedVersion: "2026.4.20",
		Summary:      "Workspace MCP stdio env can inject dangerous startup variables.",
	},
	{
		ID:           "GHSA-7jm2-g593-4qrc",
		FixedVersion: "2026.4.20",
		Summary:      "Model-driven gateway config.patch can mutate operator-trusted security settings.",
	},
	{
		ID:           "GHSA-hxvm-xjvf-93f3",
		FixedVersion: "2026.4.20",
		Summary:      "Workspace dotenv can override OPENCLAW_ runtime-control environment variables.",
	},
}

// checkDangerousGatewayFlags validates insecure or dangerous configuration toggles.
func checkDangerousGatewayFlags(config OpenclawConfig, rawConfig map[string]interface{}, risks *[]core.Risk) {
	if rawConfig == nil {
		return
	}

	var flags []string
	isCritical := false

	if v, ok := lookupNestedBool(rawConfig, "gateway", "controlUi", "allowInsecureAuth"); ok && v {
		flags = append(flags, "gateway.controlUi.allowInsecureAuth=true")
	}
	if v, ok := lookupNestedBool(rawConfig, "gateway", "controlUi", "dangerouslyDisableDeviceAuth"); ok && v {
		flags = append(flags, "gateway.controlUi.dangerouslyDisableDeviceAuth=true")
		isCritical = true
	}
	if v, ok := lookupNestedBool(rawConfig, "gateway", "controlUi", "dangerouslyAllowHostHeaderOriginFallback"); ok && v {
		flags = append(flags, "gateway.controlUi.dangerouslyAllowHostHeaderOriginFallback=true")
	}
	if v, ok := lookupNestedBool(rawConfig, "gateway", "allowRealIpFallback"); ok && v {
		flags = append(flags, "gateway.allowRealIpFallback=true")
	}

	if origins, ok := lookupNestedStringSlice(rawConfig, "gateway", "controlUi", "allowedOrigins"); ok {
		for _, origin := range origins {
			if strings.TrimSpace(origin) == "*" {
				flags = append(flags, "gateway.controlUi.allowedOrigins contains '*'")
				break
			}
		}
	}

	if strings.EqualFold(strings.TrimSpace(config.Gateway.Auth.Mode), "trusted-proxy") && len(config.Gateway.TrustedProxies) == 0 {
		flags = append(flags, "gateway.auth.mode=trusted-proxy without gateway.trustedProxies")
		isCritical = true
	}

	if len(flags) == 0 {
		return
	}

	sort.Strings(flags)
	riskLevel := core.RiskLevelHigh
	if isCritical {
		riskLevel = core.RiskLevelCritical
	}

	*risks = append(*risks, core.Risk{
		ID:    "openclaw_insecure_or_dangerous_flags",
		Title: "Insecure or Dangerous Gateway Flags Enabled",
		Description: "Detected OpenClaw gateway flags that weaken authentication or origin trust protections. " +
			"Disable these flags unless you have a strict, verified threat-model exception.",
		Level: riskLevel,
		Args: map[string]interface{}{
			"flags": flags,
		},
	})
}

// checkConfigPatchLevelByVersion reports known configuration-related advisories by version.
func checkConfigPatchLevelByVersion(version string, risks *[]core.Risk) {
	var impacted []openClawConfigAdvisory
	required := "2026.3.11"

	for _, advisory := range configAdvisories {
		if version == "" || isVersionLowerThan(version, advisory.FixedVersion) {
			impacted = append(impacted, advisory)
			if isVersionLowerThan(required, advisory.FixedVersion) {
				required = advisory.FixedVersion
			}
		}
	}

	if len(impacted) == 0 {
		return
	}

	items := make([]string, 0, len(impacted))
	for _, advisory := range impacted {
		items = append(items, fmt.Sprintf("%s (fixed in %s)", advisory.ID, advisory.FixedVersion))
	}
	sort.Strings(items)

	displayVersion := version
	if displayVersion == "" {
		displayVersion = "unknown"
	}

	*risks = append(*risks, core.Risk{
		ID:    "openclaw_config_patch_outdated",
		Title: "OpenClaw Config Security Patches Missing",
		Description: "Current OpenClaw version is missing published configuration-security fixes. " +
			"Upgrade to a patched version immediately.",
		Level: core.RiskLevelHigh,
		Args: map[string]interface{}{
			"current_version":  displayVersion,
			"required_version": required,
			"advisories":       strings.Join(items, "; "),
			"check_version":    "openclaw --version",
		},
	})
}

// checkOneClickRCEVulnerabilityByVersion detects the one-click token exfiltration RCE chain.
func checkOneClickRCEVulnerabilityByVersion(version string, risks *[]core.Risk) {
	if version != "" && !isVersionLowerThan(version, "2026.1.29") {
		return
	}

	displayVersion := version
	if displayVersion == "" {
		displayVersion = "unknown"
	}

	*risks = append(*risks, core.Risk{
		ID:    "openclaw_1click_rce_vulnerability",
		Title: "1-click RCE via gatewayUrl Token Exfiltration",
		Description: "OpenClaw Control UI may trust gatewayUrl from query parameters and auto-connect with a stored auth token, " +
			"which can enable attacker-controlled token theft and gateway takeover (GHSA-g8p2-7wf7-98mq / CVE-2026-25253).",
		Level: core.RiskLevelCritical,
		Args: map[string]interface{}{
			"cvss_score":       "8.8",
			"attack_vector":    "Network",
			"check_version":    "openclaw --version",
			"vulnerable_below": "2026.1.29",
			"current_version":  displayVersion,
			"advisory":         "GHSA-g8p2-7wf7-98mq / CVE-2026-25253",
		},
	})
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

	return extractOpenClawVersion(string(output))
}

func extractOpenClawVersion(raw string) string {
	match := openClawVersionPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(match) < 4 {
		return ""
	}
	version := fmt.Sprintf("%s.%s.%s", match[1], match[2], match[3])
	if len(match) >= 5 && match[4] != "" {
		version = version + "-" + match[4]
	}
	return version
}

func parseOpenClawVersion(value string) (openClawVersion, bool) {
	match := openClawVersionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) < 4 {
		return openClawVersion{}, false
	}

	parsed := openClawVersion{}
	if _, err := fmt.Sscanf(match[1], "%d", &parsed.year); err != nil {
		return openClawVersion{}, false
	}
	if _, err := fmt.Sscanf(match[2], "%d", &parsed.month); err != nil {
		return openClawVersion{}, false
	}
	if _, err := fmt.Sscanf(match[3], "%d", &parsed.day); err != nil {
		return openClawVersion{}, false
	}
	parsed.build = 0
	if len(match) >= 5 && match[4] != "" {
		if _, err := fmt.Sscanf(match[4], "%d", &parsed.build); err != nil {
			return openClawVersion{}, false
		}
	}
	return parsed, true
}

func compareOpenClawVersion(left, right string) (int, bool) {
	lv, lok := parseOpenClawVersion(left)
	rv, rok := parseOpenClawVersion(right)
	if !lok || !rok {
		return 0, false
	}

	switch {
	case lv.year != rv.year:
		if lv.year < rv.year {
			return -1, true
		}
		return 1, true
	case lv.month != rv.month:
		if lv.month < rv.month {
			return -1, true
		}
		return 1, true
	case lv.day != rv.day:
		if lv.day < rv.day {
			return -1, true
		}
		return 1, true
	case lv.build != rv.build:
		if lv.build < rv.build {
			return -1, true
		}
		return 1, true
	default:
		return 0, true
	}
}

func isVersionLowerThan(current, fixed string) bool {
	diff, ok := compareOpenClawVersion(current, fixed)
	if !ok {
		// Fail closed when version parsing fails.
		return true
	}
	return diff < 0
}

func lookupNestedValue(rawConfig map[string]interface{}, path ...string) (interface{}, bool) {
	var current interface{} = rawConfig
	for _, key := range path {
		nextMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		val, ok := nextMap[key]
		if !ok {
			return nil, false
		}
		current = val
	}
	return current, true
}

func lookupNestedBool(rawConfig map[string]interface{}, path ...string) (bool, bool) {
	value, ok := lookupNestedValue(rawConfig, path...)
	if !ok {
		return false, false
	}
	v, ok := value.(bool)
	return v, ok
}

func lookupNestedStringSlice(rawConfig map[string]interface{}, path ...string) ([]string, bool) {
	value, ok := lookupNestedValue(rawConfig, path...)
	if !ok {
		return nil, false
	}

	switch list := value.(type) {
	case []string:
		return list, true
	case []interface{}:
		result := make([]string, 0, len(list))
		for _, item := range list {
			text, ok := item.(string)
			if !ok {
				continue
			}
			result = append(result, text)
		}
		return result, true
	default:
		return nil, false
	}
}
