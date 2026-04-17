package hermes

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func mitigationSuccess(message string) string {
	payload := map[string]interface{}{"success": true, "message": message}
	b, _ := json.Marshal(payload)
	return string(b)
}

func mitigationError(message string) string {
	payload := map[string]interface{}{"success": false, "error": message}
	b, _ := json.Marshal(payload)
	return string(b)
}

func mitigationSkipped() string {
	return mitigationSuccess("skipped by user")
}

func updateHermesConfig(configPath string, updater func(raw map[string]interface{}) error) error {
	_, raw, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	if err := updater(raw); err != nil {
		return err
	}
	return saveConfig(configPath, raw)
}

func handleConfigPermMitigation(req map[string]interface{}) string {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})
	path, _ := args["path"].(string)
	if strings.TrimSpace(path) == "" {
		return mitigationError("path missing")
	}
	if enabled, _ := formData["fix_permission"].(bool); !enabled {
		return mitigationSkipped()
	}
	if err := os.Chmod(path, 0600); err != nil {
		return mitigationError(fmt.Sprintf("chmod failed: %v", err))
	}
	return mitigationSuccess("config file permission updated")
}

func handleConfigDirPermMitigation(req map[string]interface{}) string {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})
	path, _ := args["path"].(string)
	if strings.TrimSpace(path) == "" {
		return mitigationError("path missing")
	}
	if enabled, _ := formData["fix_permission"].(bool); !enabled {
		return mitigationSkipped()
	}
	if err := os.Chmod(path, 0700); err != nil {
		return mitigationError(fmt.Sprintf("chmod failed: %v", err))
	}
	return mitigationSuccess("config directory permission updated")
}

func handleRedactSecretsMitigation(req map[string]interface{}) string {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})
	configPath, _ := args["config_path"].(string)
	if strings.TrimSpace(configPath) == "" {
		var err error
		configPath, err = findConfigPath()
		if err != nil {
			return mitigationError("config not found")
		}
	}
	if enabled, _ := formData["enable_redaction"].(bool); !enabled {
		return mitigationSkipped()
	}
	if err := updateHermesConfig(configPath, func(raw map[string]interface{}) error {
		security := ensureMap(raw, "security")
		security["redact_secrets"] = true
		return nil
	}); err != nil {
		return mitigationError(fmt.Sprintf("update failed: %v", err))
	}
	return mitigationSuccess("security.redact_secrets enabled")
}

func handleApprovalsModeMitigation(req map[string]interface{}) string {
	args, _ := req["args"].(map[string]interface{})
	formData, _ := req["form_data"].(map[string]interface{})
	configPath, _ := args["config_path"].(string)
	if strings.TrimSpace(configPath) == "" {
		var err error
		configPath, err = findConfigPath()
		if err != nil {
			return mitigationError("config not found")
		}
	}
	mode, _ := formData["mode"].(string)
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "manual"
	}
	if mode != "manual" && mode != "smart" {
		return mitigationError("invalid approvals mode")
	}
	if err := updateHermesConfig(configPath, func(raw map[string]interface{}) error {
		approvals := ensureMap(raw, "approvals")
		approvals["mode"] = mode
		return nil
	}); err != nil {
		return mitigationError(fmt.Sprintf("update failed: %v", err))
	}
	return mitigationSuccess("approvals.mode updated")
}

// MitigateRiskDispatch routes mitigation actions by risk ID.
func MitigateRiskDispatch(riskInfo string) string {
	var req map[string]interface{}
	if err := json.Unmarshal([]byte(riskInfo), &req); err != nil {
		return mitigationError(fmt.Sprintf("invalid json: %v", err))
	}
	riskID, _ := req["id"].(string)
	switch riskID {
	case "config_perm_unsafe":
		return handleConfigPermMitigation(req)
	case "config_dir_perm_unsafe":
		return handleConfigDirPermMitigation(req)
	case "redact_secrets_disabled":
		return handleRedactSecretsMitigation(req)
	case "approvals_mode_disabled":
		return handleApprovalsModeMitigation(req)
	default:
		return mitigationError("not implemented")
	}
}
