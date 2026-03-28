package nullclaw

/*
#include <stdlib.h>
*/
import "C"

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go_lib/core"
)

var applyACLForPath = applyWindowsACL

func fixPermissionByPlatform(path string, unixMode os.FileMode, isDirectory bool, unixSuccessMessage, windowsSuccessMessage string) (string, error) {
	if runtime.GOOS == "windows" {
		output, err := applyACLForPath(path, isDirectory)
		if err != nil {
			return "", err
		}
		if output == "" {
			return windowsSuccessMessage, nil
		}
		return fmt.Sprintf("%s (%s)", windowsSuccessMessage, output), nil
	}

	if err := os.Chmod(path, unixMode); err != nil {
		return "", err
	}
	return unixSuccessMessage, nil
}

// handleLoggingRedactMitigation handles the logging redaction risk mitigation
func handleLoggingRedactMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	configPath, ok := args["config_path"].(string)
	if !ok || configPath == "" {
		return C.CString(`{"success": false, "error": "config_path missing"}`)
	}

	if enable, ok := formData["redact_sensitive"].(bool); !ok || !enable {
		return C.CString(`{"success": true, "message": "skipped by user"}`)
	}

	if err := core.UpdateJSONConfig(configPath, "logging.redactSensitive", "tools"); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
	}

	return C.CString(`{"success": true}`)
}

// handleGatewayAuthMitigation handles gateway pairing mitigation.
func handleGatewayAuthMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})

	configPath, ok := args["config_path"].(string)
	if !ok || configPath == "" {
		p, err := findConfigPath()
		if err != nil {
			return C.CString(`{"success": false, "error": "config not found"}`)
		}
		configPath = p
	}

	if err := core.UpdateJSONConfig(configPath, "gateway.require_pairing", true); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "failed to enable pairing: %v"}`, err))
	}
	return C.CString(`{"success": true, "message": "gateway.require_pairing set to true"}`)
}

// handleGatewayAuthPasswordModeMitigation handles switching from password to token mode
func handleGatewayAuthPasswordModeMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	configPath, ok := args["config_path"].(string)
	if !ok || configPath == "" {
		p, err := findConfigPath()
		if err != nil {
			return C.CString(`{"success": false, "error": "config not found"}`)
		}
		configPath = p
	}

	switchToToken, _ := formData["switch_to_token"].(bool)
	if !switchToToken {
		return C.CString(`{"success": true, "message": "skipped by user"}`)
	}

	token, _ := formData["token_value"].(string)
	if token == "${GEN_RANDOM_TOKEN}" || token == "" {
		b := make([]byte, 16)
		rand.Read(b)
		token = hex.EncodeToString(b)
	}

	if err := core.UpdateJSONConfig(configPath, "gateway.auth.mode", "token"); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
	}
	if err := core.UpdateJSONConfig(configPath, "gateway.auth.token", token); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
	}
	core.UpdateJSONConfig(configPath, "gateway.auth.password", "")

	return C.CString(fmt.Sprintf(`{"success": true, "message": "Switched to token mode"}`))
}

// handleConfigPermMitigation handles config file permission fix
func handleConfigPermMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	configPath, ok := args["path"].(string)
	if !ok || configPath == "" {
		return C.CString(`{"success": false, "error": "path missing"}`)
	}

	fixPerm, _ := formData["fix_permission"].(bool)
	if !fixPerm {
		return C.CString(`{"success": true, "message": "skipped by user"}`)
	}

	message, err := fixPermissionByPlatform(configPath, 0600, false, "Permission updated to 600", "ACL updated for config file")
	if err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%s"}`, jsonEscape(fmt.Sprintf("%v", err))))
	}

	return C.CString(fmt.Sprintf(`{"success": true, "message": "%s"}`, jsonEscape(message)))
}

// handleConfigDirPermMitigation handles config directory permission fix
func handleConfigDirPermMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	dirPath, ok := args["path"].(string)
	if !ok || dirPath == "" {
		return C.CString(`{"success": false, "error": "path missing"}`)
	}

	fixPerm, _ := formData["fix_permission"].(bool)
	if !fixPerm {
		return C.CString(`{"success": true, "message": "skipped by user"}`)
	}

	message, err := fixPermissionByPlatform(dirPath, 0700, true, "Permission updated to 700", "ACL updated for config directory")
	if err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%s"}`, jsonEscape(fmt.Sprintf("%v", err))))
	}

	return C.CString(fmt.Sprintf(`{"success": true, "message": "%s"}`, jsonEscape(message)))
}

// handleGatewayBindMitigation handles gateway bind address fix
func handleGatewayBindMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	configPath, ok := args["config_path"].(string)
	if !ok || configPath == "" {
		p, err := findConfigPath()
		if err != nil {
			return C.CString(`{"success": false, "error": "config not found"}`)
		}
		configPath = p
	}

	bindAddr, _ := formData["bind_address"].(string)
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}

	if err := core.UpdateJSONConfig(configPath, "gateway.host", bindAddr); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
	}
	_ = core.UpdateJSONConfig(configPath, "gateway.bind", bindAddr) // compatibility key
	if err := core.UpdateJSONConfig(configPath, "gateway.allow_public_bind", false); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
	}

	return C.CString(fmt.Sprintf(`{"success": true, "message": "Gateway host set to %s and allow_public_bind disabled"}`, bindAddr))
}

// handleGatewayWeakPasswordMitigation handles weak password fix
func handleGatewayWeakPasswordMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	configPath, ok := args["config_path"].(string)
	if !ok || configPath == "" {
		p, err := findConfigPath()
		if err != nil {
			return C.CString(`{"success": false, "error": "config not found"}`)
		}
		configPath = p
	}

	action, _ := formData["action"].(string)
	if action == "switch_to_token" {
		token, _ := formData["token_value"].(string)
		if token == "${GEN_RANDOM_TOKEN}" || token == "" {
			b := make([]byte, 16)
			rand.Read(b)
			token = hex.EncodeToString(b)
		}

		if err := core.UpdateJSONConfig(configPath, "gateway.auth.mode", "token"); err != nil {
			return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
		}
		if err := core.UpdateJSONConfig(configPath, "gateway.auth.token", token); err != nil {
			return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
		}
		core.UpdateJSONConfig(configPath, "gateway.auth.password", "")
		return C.CString(`{"success": true, "message": "Switched to token mode"}`)
	}

	return C.CString(`{"success": false, "error": "invalid action"}`)
}

// handleGatewayWeakTokenMitigation handles weak token regeneration
func handleGatewayWeakTokenMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	configPath, ok := args["config_path"].(string)
	if !ok || configPath == "" {
		p, err := findConfigPath()
		if err != nil {
			return C.CString(`{"success": false, "error": "config not found"}`)
		}
		configPath = p
	}

	regenerate, _ := formData["regenerate_token"].(bool)
	if !regenerate {
		return C.CString(`{"success": true, "message": "skipped by user"}`)
	}

	token, _ := formData["token_value"].(string)
	if token == "${GEN_RANDOM_TOKEN}" || token == "" {
		b := make([]byte, 16)
		rand.Read(b)
		token = hex.EncodeToString(b)
	}

	if err := core.UpdateJSONConfig(configPath, "gateway.auth.token", token); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
	}

	return C.CString(`{"success": true, "message": "Token regenerated"}`)
}

// handleSandboxDisabledDefaultMitigation handles default sandbox mode fix
func handleSandboxDisabledDefaultMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	configPath, ok := args["config_path"].(string)
	if !ok || configPath == "" {
		p, err := findConfigPath()
		if err != nil {
			return C.CString(`{"success": false, "error": "config not found"}`)
		}
		configPath = p
	}

	sandboxMode, _ := formData["sandbox_mode"].(string)
	if sandboxMode == "" {
		sandboxMode = "docker"
	}

	if err := core.UpdateJSONConfig(configPath, "security.sandbox.backend", sandboxMode); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
	}

	return C.CString(fmt.Sprintf(`{"success": true, "message": "security.sandbox.backend set to %s"}`, sandboxMode))
}

// handleSandboxDisabledAgentMitigation handles agent-specific sandbox mode fix
func handleSandboxDisabledAgentMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	configPath, ok := args["config_path"].(string)
	if !ok || configPath == "" {
		p, err := findConfigPath()
		if err != nil {
			return C.CString(`{"success": false, "error": "config not found"}`)
		}
		configPath = p
	}

	agentName, ok := args["agent"].(string)
	if !ok || agentName == "" {
		return C.CString(`{"success": false, "error": "agent name missing"}`)
	}

	sandboxMode, _ := formData["sandbox_mode"].(string)
	if sandboxMode == "" {
		sandboxMode = "docker"
	}

	path := fmt.Sprintf("agents.%s.sandbox.mode", agentName)
	if err := core.UpdateJSONConfig(configPath, path, sandboxMode); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
	}

	return C.CString(fmt.Sprintf(`{"success": true, "message": "Sandbox mode for %s set to %s"}`, agentName, sandboxMode))
}

// handleLogDirPermMitigation handles log directory permission fix
func handleLogDirPermMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	configPath, ok := args["config_path"].(string)
	if !ok || configPath == "" {
		return C.CString(`{"success": false, "error": "config_path missing"}`)
	}

	fixPerm, _ := formData["fix_permission"].(bool)
	if !fixPerm {
		return C.CString(`{"success": true, "message": "skipped by user"}`)
	}

	logDir := filepath.Join(filepath.Dir(configPath), "logs")
	message, err := fixPermissionByPlatform(logDir, 0700, true, "Log directory permission updated to 700", "ACL updated for log directory")
	if err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%s"}`, jsonEscape(fmt.Sprintf("%v", err))))
	}

	return C.CString(fmt.Sprintf(`{"success": true, "message": "%s"}`, jsonEscape(message)))
}

func jsonEscape(input string) string {
	input = strings.ReplaceAll(input, `\`, `\\`)
	input = strings.ReplaceAll(input, `"`, `\"`)
	input = strings.ReplaceAll(input, "\r", " ")
	input = strings.ReplaceAll(input, "\n", " ")
	return strings.TrimSpace(input)
}

// handleAuditDisabledMitigation enables security.audit.enabled.
func handleAuditDisabledMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	configPath, ok := args["config_path"].(string)
	if !ok || configPath == "" {
		p, err := findConfigPath()
		if err != nil {
			return C.CString(`{"success": false, "error": "config not found"}`)
		}
		configPath = p
	}
	if err := core.UpdateJSONConfig(configPath, "security.audit.enabled", true); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
	}
	return C.CString(`{"success": true, "message": "security.audit.enabled set to true"}`)
}

// handleWorkspaceOnlyMitigation enables autonomy.workspace_only.
func handleWorkspaceOnlyMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	configPath, ok := args["config_path"].(string)
	if !ok || configPath == "" {
		p, err := findConfigPath()
		if err != nil {
			return C.CString(`{"success": false, "error": "config not found"}`)
		}
		configPath = p
	}
	if err := core.UpdateJSONConfig(configPath, "autonomy.workspace_only", true); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%v"}`, err))
	}
	return C.CString(`{"success": true, "message": "autonomy.workspace_only set to true"}`)
}

// handlePlaintextSecretsMitigation handles plaintext secrets fix
func handlePlaintextSecretsMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	action, _ := formData["action"].(string)
	envVarName, _ := formData["env_var_name"].(string)

	if action == "use_env_var" {
		if envVarName == "" {
			envVarName = "MOLTBOT_SECRET"
		}

		pattern, _ := args["pattern"].(string)
		msg := fmt.Sprintf("Please move the secret (pattern: %s) to environment variable: %s", pattern, envVarName)
		return C.CString(fmt.Sprintf(`{"success": true, "message": "%s"}`, msg))
	}

	if action == "manual_review" {
		return C.CString(`{"success": true, "message": "Please manually review and remove plaintext secrets"}`)
	}

	return C.CString(`{"success": false, "error": "invalid action"}`)
}

// handleSkillsNotScannedMitigation handles the skill scanning mitigation
func handleSkillsNotScannedMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	scanSkills, _ := formData["scan_skills"].(bool)
	if !scanSkills {
		return C.CString(`{"success": true, "message": "skipped by user"}`)
	}

	skillNames, ok := args["skills"].([]interface{})
	if !ok || len(skillNames) == 0 {
		return C.CString(`{"success": false, "error": "no skills to scan"}`)
	}

	// 从 skill_paths 获取每个技能的完整路径
	skillPathsRaw, _ := args["skill_paths"].(map[string]interface{})

	var scannedCount int
	var issues []string

	for _, nameRaw := range skillNames {
		name, ok := nameRaw.(string)
		if !ok {
			continue
		}

		// 从 skill_paths map 获取完整路径
		skillPath, _ := skillPathsRaw[name].(string)
		if skillPath == "" {
			issues = append(issues, fmt.Sprintf("Failed to scan %s: path not found", name))
			continue
		}

		result, err := ScanSkillForPromptInjection(skillPath)
		if err != nil {
			issues = append(issues, fmt.Sprintf("Failed to scan %s: %v", name, err))
			continue
		}

		scannedCount++
		if !result.Safe {
			issues = append(issues, fmt.Sprintf("%s: %d potential issues found", name, len(result.Issues)))
		}
	}

	if len(issues) > 0 {
		return C.CString(fmt.Sprintf(`{"success": true, "message": "Scanned %d skills. Issues: %s"}`, scannedCount, strings.Join(issues, "; ")))
	}

	return C.CString(fmt.Sprintf(`{"success": true, "message": "Scanned %d skills successfully. No issues found."}`, scannedCount))
}

// handleSkillSecurityAnalyzerRiskMitigation 处理 AI 安全分析发现的风险
func handleSkillSecurityAnalyzerRiskMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	skillPath, ok := args["skill_path"].(string)
	if !ok || skillPath == "" {
		return C.CString(`{"success": false, "error": "skill_path missing"}`)
	}

	deleteSkill, _ := formData["delete_skill"].(bool)
	if !deleteSkill {
		return C.CString(`{"success": true, "message": "skipped by user"}`)
	}

	// Verify the path is within skills directory
	if !isWithinSkillsDirs(skillPath) {
		return C.CString(`{"success": false, "error": "skill path is not within skills directory"}`)
	}

	// Delete the skill directory
	if err := removeSkillDirectory(skillPath); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "failed to delete skill: %v"}`, err))
	}

	skillName := filepath.Base(skillPath)
	return C.CString(fmt.Sprintf(`{"success": true, "message": "Skill '%s' has been deleted"}`, skillName))
}

// MitigateRiskDispatch 处理风险缓解请求，根据风险ID分发到对应的处理函数
func MitigateRiskDispatch(riskInfo string) string {
	var req map[string]interface{}
	if err := json.Unmarshal([]byte(riskInfo), &req); err != nil {
		return fmt.Sprintf(`{"success": false, "error": "invalid json: %v"}`, err)
	}

	riskID, _ := req["id"].(string)

	switch riskID {
	case "logging_redact_off":
		return C.GoString(handleLoggingRedactMitigation(req))
	case "gateway_auth_disabled":
		return C.GoString(handleGatewayAuthMitigation(req))
	case "gateway_auth_password_mode":
		return C.GoString(handleGatewayAuthPasswordModeMitigation(req))
	case "config_perm_unsafe":
		return C.GoString(handleConfigPermMitigation(req))
	case "config_dir_perm_unsafe":
		return C.GoString(handleConfigDirPermMitigation(req))
	case "gateway_bind_unsafe":
		return C.GoString(handleGatewayBindMitigation(req))
	case "gateway_weak_password":
		return C.GoString(handleGatewayWeakPasswordMitigation(req))
	case "gateway_weak_token":
		return C.GoString(handleGatewayWeakTokenMitigation(req))
	case "sandbox_disabled_default":
		return C.GoString(handleSandboxDisabledDefaultMitigation(req))
	case "sandbox_disabled_agent":
		return C.GoString(handleSandboxDisabledAgentMitigation(req))
	case "log_dir_perm_unsafe":
		return C.GoString(handleLogDirPermMitigation(req))
	case "audit_disabled":
		return C.GoString(handleAuditDisabledMitigation(req))
	case "autonomy_workspace_unrestricted":
		return C.GoString(handleWorkspaceOnlyMitigation(req))
	case "plaintext_secrets":
		return C.GoString(handlePlaintextSecretsMitigation(req))
	case "skills_not_scanned":
		return C.GoString(handleSkillsNotScannedMitigation(req))
	case "skill_agent_risk":
		return C.GoString(handleSkillSecurityAnalyzerRiskMitigation(req))
	default:
		return `{"success": false, "error": "not implemented"}`
	}
}
