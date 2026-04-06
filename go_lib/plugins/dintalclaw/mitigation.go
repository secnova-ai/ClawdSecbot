package dintalclaw

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var applyACLForPath = applyWindowsACL

// fixPermissionByPlatform 跨平台权限修复
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

// handleConfigPermMitigation 修复配置文件权限
func handleConfigPermMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	configPath, ok := args["path"].(string)
	if !ok || configPath == "" {
		return C.CString(`{"success": false, "error": "path missing"}`)
	}

	fixPerm, _ := formData["fix_permission"].(bool)
	if !fixPerm {
		return C.CString(`{"success": true, "message": "用户选择跳过修复"}`)
	}

	message, err := fixPermissionByPlatform(configPath, 0600, false, "权限已更新为 600", "配置文件 ACL 已更新")
	if err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%s"}`, jsonEscape(fmt.Sprintf("%v", err))))
	}

	return C.CString(fmt.Sprintf(`{"success": true, "message": "%s"}`, jsonEscape(message)))
}

// handleMemoryDirPermMitigation 修复 memory 目录权限
func handleMemoryDirPermMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	dirPath, ok := args["path"].(string)
	if !ok || dirPath == "" {
		return C.CString(`{"success": false, "error": "path missing"}`)
	}

	fixPerm, _ := formData["fix_permission"].(bool)
	if !fixPerm {
		return C.CString(`{"success": true, "message": "用户选择跳过修复"}`)
	}

	message, err := fixPermissionByPlatform(dirPath, 0700, true, "权限已更新为 700", "memory 目录 ACL 已更新")
	if err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%s"}`, jsonEscape(fmt.Sprintf("%v", err))))
	}

	return C.CString(fmt.Sprintf(`{"success": true, "message": "%s"}`, jsonEscape(message)))
}

// handleLogDirPermMitigation 修复日志目录权限
func handleLogDirPermMitigation(req map[string]interface{}) *C.char {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})

	logDirPath, ok := args["path"].(string)
	if !ok || logDirPath == "" {
		return C.CString(`{"success": false, "error": "path missing"}`)
	}

	fixPerm, _ := formData["fix_permission"].(bool)
	if !fixPerm {
		return C.CString(`{"success": true, "message": "用户选择跳过修复"}`)
	}

	// args.path 已是日志目录绝对路径，不能再次拼接 "temp"，否则会变成 .../temp/temp 导致 chmod 失败。
	message, err := fixPermissionByPlatform(logDirPath, 0700, true, "日志目录权限已更新为 700", "日志目录 ACL 已更新")
	if err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "%s"}`, jsonEscape(fmt.Sprintf("%v", err))))
	}

	return C.CString(fmt.Sprintf(`{"success": true, "message": "%s"}`, jsonEscape(message)))
}

// handlePlaintextSecretsMitigation 处理明文密钥风险
func handlePlaintextSecretsMitigation(req map[string]interface{}) *C.char {
	formData, _ := req["form_data"].(map[string]interface{})
	args, _ := req["args"].(map[string]interface{})

	action, _ := formData["action"].(string)
	envVarName, _ := formData["env_var_name"].(string)

	if action == "use_env_var" {
		if envVarName == "" {
			envVarName = "DINTALCLAW_SECRET"
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

// handleSkillsNotScannedMitigation 处理技能未扫描风险
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

	skillPathsRaw, _ := args["skill_paths"].(map[string]interface{})
	var scannedCount int
	var issues []string

	for _, nameRaw := range skillNames {
		name, ok := nameRaw.(string)
		if !ok {
			continue
		}
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

// handleSkillSecurityAnalyzerRiskMitigation 处理 AI 安全分析风险
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

	if !isWithinSkillsDirs(skillPath) {
		return C.CString(`{"success": false, "error": "skill path is not within skills directory"}`)
	}

	if err := removeSkillDirectory(skillPath); err != nil {
		return C.CString(fmt.Sprintf(`{"success": false, "error": "failed to delete skill: %v"}`, err))
	}

	skillName := filepath.Base(skillPath)
	return C.CString(fmt.Sprintf(`{"success": true, "message": "Skill '%s' has been deleted"}`, skillName))
}

func jsonEscape(input string) string {
	input = strings.ReplaceAll(input, `\`, `\\`)
	input = strings.ReplaceAll(input, `"`, `\"`)
	input = strings.ReplaceAll(input, "\r", " ")
	input = strings.ReplaceAll(input, "\n", " ")
	return strings.TrimSpace(input)
}

// MitigateRiskDispatch 风险缓解分发器
func MitigateRiskDispatch(riskInfo string) string {
	var req map[string]interface{}
	if err := json.Unmarshal([]byte(riskInfo), &req); err != nil {
		return fmt.Sprintf(`{"success": false, "error": "invalid json: %v"}`, err)
	}

	riskID, _ := req["id"].(string)

	switch riskID {
	case "config_perm_unsafe":
		return C.GoString(handleConfigPermMitigation(req))
	case "memory_dir_perm_unsafe":
		return C.GoString(handleMemoryDirPermMitigation(req))
	case "log_dir_perm_unsafe":
		return C.GoString(handleLogDirPermMitigation(req))
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
