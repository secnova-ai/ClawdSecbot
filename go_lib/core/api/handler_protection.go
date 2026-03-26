package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/proxy"
	"go_lib/core/repository"
	"go_lib/core/shepherd"
)

var errProtectionPolicyNotFound = errors.New("protection policy not found")

// ========== External API Types (matching _rules/setting_task.md) ==========

// ProtectionPolicyRequest represents the external API format for protection policy.
type ProtectionPolicyRequest struct {
	BotID      []string          `json:"botId"`
	Protection string            `json:"protection"` // "enabled", "bypass", "disabled"
	UserRules  []string          `json:"userRules,omitempty"`
	TokenLimit *TokenLimitConfig `json:"tokenLimit,omitempty"`
	Permission *PermissionConfig `json:"permission,omitempty"`
	BotModel   *BotModelConfig   `json:"botModel,omitempty"`
}

// TokenLimitConfig represents token limit settings.
type TokenLimitConfig struct {
	Session int `json:"session"`
	Daily   int `json:"daily"`
}

// PermissionConfig represents permission settings.
type PermissionConfig struct {
	Open    *bool              `json:"open,omitempty"`
	Path    *PathPermission    `json:"path,omitempty"`
	Network *NetworkPermission `json:"network,omitempty"`
	Shell   *ShellPermission   `json:"shell,omitempty"`
}

// PathPermission represents path permission settings.
type PathPermission struct {
	Mode  string   `json:"mode"` // "blacklist" or "whitelist"
	Paths []string `json:"paths"`
}

// NetworkPermission represents network permission settings.
type NetworkPermission struct {
	Inbound  *AddressPermission `json:"inbound,omitempty"`
	Outbound *AddressPermission `json:"outbound,omitempty"`
}

// AddressPermission represents network address permission.
type AddressPermission struct {
	Mode      string   `json:"mode"` // "blacklist" or "whitelist"
	Addresses []string `json:"addresses"`
}

// ShellPermission represents shell command permission settings.
type ShellPermission struct {
	Mode     string   `json:"mode"` // "blacklist" or "whitelist"
	Commands []string `json:"commands"`
}

// BotModelConfig represents bot model configuration in external API.
type BotModelConfig struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	URL      string `json:"url"`
	Key      string `json:"key,omitempty"`
}

// ProtectionPolicyResponse represents the response format for protection policy.
type ProtectionPolicyResponse struct {
	BotID      string            `json:"botId"`
	Protection string            `json:"protection"`
	UserRules  []string          `json:"userRules"`
	TokenLimit *TokenLimitConfig `json:"tokenLimit"`
	Permission *PermissionConfig `json:"permission"`
	BotModel   *BotModelConfig   `json:"botModel"`
}

// handleGetProtectionPolicy handles GET /api/v1/protection/policy
// Returns protection policies for the requested bots, or all bots when botId is empty.
func (s *APIServer) handleGetProtectionPolicy(w http.ResponseWriter, r *http.Request) {
	req, err := parseProtectionPolicyRequest(r)
	if err != nil {
		Error(w, http.StatusBadRequest, CodeInvalidParam, err.Error())
		return
	}

	repo := repository.NewProtectionRepository(nil)
	botIDs, err := resolveRequestedBotIDs(repo, req.BotID)
	if err != nil {
		logging.Error("API: Failed to resolve requested botIds: %v", err)
		Error(w, http.StatusInternalServerError, CodeInternalError, "failed to query protection configs")
		return
	}

	if len(botIDs) == 0 {
		Success(w, []*ProtectionPolicyResponse{})
		return
	}

	logging.Info("API: Getting protection policy for %d bot(s)", len(botIDs))

	responses := make([]*ProtectionPolicyResponse, 0, len(botIDs))
	for _, botID := range botIDs {
		response, err := buildProtectionPolicyResponse(repo, botID)
		if err != nil {
			if errors.Is(err, errProtectionPolicyNotFound) {
				Error(w, http.StatusNotFound, CodeNotFound, "protection config not found for botId: "+botID)
				return
			}
			logging.Error("API: Failed to build protection policy for botId=%s: %v", botID, err)
			Error(w, http.StatusInternalServerError, CodeInternalError, "failed to query protection configs")
			return
		}
		responses = append(responses, response)
	}

	Success(w, responses)
}

// handleSetProtectionPolicy handles POST /api/v1/protection/policy
// Updates protection policies for the requested bots, or all bots when botId is empty.
func (s *APIServer) handleSetProtectionPolicy(w http.ResponseWriter, r *http.Request) {
	req, err := parseProtectionPolicyRequest(r)
	if err != nil {
		Error(w, http.StatusBadRequest, CodeInvalidParam, err.Error())
		return
	}

	// Validate protection mode
	req.Protection = strings.ToLower(req.Protection)
	if req.Protection != "" && req.Protection != "enabled" && req.Protection != "bypass" && req.Protection != "disabled" {
		Error(w, http.StatusBadRequest, CodeInvalidParam, "invalid protection mode, must be: enabled, bypass, or disabled")
		return
	}

	repo := repository.NewProtectionRepository(nil)
	botIDs, err := resolveRequestedBotIDs(repo, req.BotID)
	if err != nil {
		logging.Error("API: Failed to resolve requested botIds: %v", err)
		Error(w, http.StatusInternalServerError, CodeInternalError, "failed to query protection configs")
		return
	}

	if len(botIDs) == 0 {
		Success(w, map[string]interface{}{
			"message": "protection policy updated",
			"botId":   []string{},
		})
		return
	}

	logging.Info("API: Setting protection policy for %d bot(s)", len(botIDs))

	for _, botID := range botIDs {
		if err := updateProtectionPolicyForBotID(repo, botID, &req); err != nil {
			if errors.Is(err, errProtectionPolicyNotFound) {
				Error(w, http.StatusNotFound, CodeNotFound, "protection config not found for botId: "+botID)
				return
			}
			logging.Error("API: Failed to update protection policy for botId=%s: %v", botID, err)
			Error(w, http.StatusInternalServerError, CodeInternalError, "failed to save protection config: "+err.Error())
			return
		}
	}

	Success(w, map[string]interface{}{
		"message": "protection policy updated",
		"botId":   botIDs,
	})
}

func parseProtectionPolicyRequest(r *http.Request) (ProtectionPolicyRequest, error) {
	var req ProtectionPolicyRequest

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return req, errors.New("failed to read request body")
	}
	defer r.Body.Close()

	if len(strings.TrimSpace(string(body))) == 0 {
		return req, nil
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return req, errors.New("invalid JSON: " + err.Error())
	}

	return req, nil
}

func buildProtectionPolicyResponse(repo *repository.ProtectionRepository, botID string) (*ProtectionPolicyResponse, error) {
	config, _, err := findProtectionConfigByBotID(repo, botID)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, errProtectionPolicyNotFound
	}

	response := convertToExternalPolicy(config, botID)
	userRules, found, err := repo.GetShepherdSensitiveActions(config.AssetName, botID)
	if err != nil {
		logging.Warning("API: Failed to query instance user rules for botId=%s: %v", botID, err)
		defaultRules, defaultErr := shepherd.GetDefaultUserRules()
		if defaultErr != nil {
			logging.Warning("API: Failed to query default shepherd rules for botId=%s: %v", botID, defaultErr)
			userRules = []string{}
		} else {
			userRules = defaultRules.SensitiveActions
		}
	} else if !found {
		defaultRules, defaultErr := shepherd.GetDefaultUserRules()
		if defaultErr != nil {
			logging.Warning("API: Failed to query default shepherd rules for botId=%s: %v", botID, defaultErr)
			userRules = []string{}
		} else {
			userRules = defaultRules.SensitiveActions
		}
	}
	if len(userRules) > 0 {
		response.UserRules = userRules
	}

	return response, nil
}

func updateProtectionPolicyForBotID(repo *repository.ProtectionRepository, botID string, req *ProtectionPolicyRequest) error {
	existingConfig, assetName, err := findProtectionConfigByBotID(repo, botID)
	if err != nil {
		return err
	}

	config := &repository.ProtectionConfig{
		AssetName: assetName,
		AssetID:   botID,
	}
	if existingConfig != nil {
		config = existingConfig
	}
	if assetName == "" {
		assetName = "openclaw"
		config.AssetName = assetName
	}

	switch req.Protection {
	case "enabled":
		config.Enabled = true
		config.AuditOnly = false
	case "bypass":
		config.Enabled = true
		config.AuditOnly = true
	case "disabled":
		config.Enabled = false
	}

	if req.TokenLimit != nil {
		config.SingleSessionTokenLimit = req.TokenLimit.Session
		config.DailyTokenLimit = req.TokenLimit.Daily
	}

	if req.Permission != nil {
		if req.Permission.Open != nil {
			config.SandboxEnabled = *req.Permission.Open
		}
		if req.Permission.Path != nil {
			pathJSON, _ := json.Marshal(req.Permission.Path)
			config.PathPermission = string(pathJSON)
		}
		if req.Permission.Network != nil {
			networkJSON, _ := json.Marshal(req.Permission.Network)
			config.NetworkPermission = string(networkJSON)
		}
		if req.Permission.Shell != nil {
			shellJSON, _ := json.Marshal(req.Permission.Shell)
			config.ShellPermission = string(shellJSON)
		}
	}

	if req.BotModel != nil {
		config.BotModelConfig = &repository.BotModelConfigData{
			Provider: req.BotModel.Provider,
			Model:    req.BotModel.ID,
			BaseURL:  req.BotModel.URL,
			APIKey:   req.BotModel.Key,
		}
	}

	if err := repo.SaveProtectionConfig(config); err != nil {
		return err
	}

	if req.UserRules != nil {
		if err := repo.SaveShepherdSensitiveActions(config.AssetName, botID, req.UserRules); err != nil {
			logging.Warning("API: Failed to update user rules: %v", err)
		}
	}

	applyProtectionPolicyRuntime(config, req.UserRules)
	logging.Info("API: Protection policy updated for botId=%s", botID)
	return nil
}

func resolveRequestedBotIDs(repo *repository.ProtectionRepository, requested []string) ([]string, error) {
	if len(requested) > 0 {
		return normalizeBotIDs(requested), nil
	}

	configs, err := repo.GetAllProtectionConfigs()
	if err != nil {
		return nil, err
	}

	botIDs := make([]string, 0, len(configs))
	for _, config := range configs {
		if config == nil || strings.TrimSpace(config.AssetID) == "" {
			continue
		}
		botIDs = append(botIDs, config.AssetID)
	}

	return normalizeBotIDs(botIDs), nil
}

func normalizeBotIDs(botIDs []string) []string {
	seen := make(map[string]struct{}, len(botIDs))
	result := make([]string, 0, len(botIDs))
	for _, botID := range botIDs {
		trimmed := strings.TrimSpace(botID)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

// convertToExternalPolicy converts internal ProtectionConfig to external API format.
func convertToExternalPolicy(config *repository.ProtectionConfig, botID string) *ProtectionPolicyResponse {
	response := &ProtectionPolicyResponse{
		BotID:      botID,
		UserRules:  []string{},
		TokenLimit: &TokenLimitConfig{},
		Permission: defaultPermissionConfig(),
		BotModel:   &BotModelConfig{},
	}

	// Determine protection mode
	if !config.Enabled {
		response.Protection = "disabled"
	} else if config.AuditOnly {
		response.Protection = "bypass"
	} else {
		response.Protection = "enabled"
	}

	// Token limits
	response.TokenLimit = &TokenLimitConfig{
		Session: config.SingleSessionTokenLimit,
		Daily:   config.DailyTokenLimit,
	}

	// Permissions
	sandboxEnabled := config.SandboxEnabled
	response.Permission.Open = &sandboxEnabled

	if config.PathPermission != "" {
		var pathPerm PathPermission
		if err := json.Unmarshal([]byte(config.PathPermission), &pathPerm); err == nil {
			response.Permission.Path = &pathPerm
		}
	}

	if config.NetworkPermission != "" {
		var networkPerm NetworkPermission
		if err := json.Unmarshal([]byte(config.NetworkPermission), &networkPerm); err == nil {
			response.Permission.Network = &networkPerm
		}
	}

	if config.ShellPermission != "" {
		var shellPerm ShellPermission
		if err := json.Unmarshal([]byte(config.ShellPermission), &shellPerm); err == nil {
			response.Permission.Shell = &shellPerm
		}
	}

	// Bot model config
	if config.BotModelConfig != nil {
		response.BotModel.Provider = config.BotModelConfig.Provider
		response.BotModel.ID = config.BotModelConfig.Model
		response.BotModel.URL = config.BotModelConfig.BaseURL
		response.BotModel.Key = config.BotModelConfig.APIKey
	}

	return response
}

func defaultPermissionConfig() *PermissionConfig {
	open := false
	return &PermissionConfig{
		Open: &open,
		Path: &PathPermission{
			Mode:  "blacklist",
			Paths: []string{},
		},
		Network: &NetworkPermission{
			Inbound: &AddressPermission{
				Mode:      "blacklist",
				Addresses: []string{},
			},
			Outbound: &AddressPermission{
				Mode:      "blacklist",
				Addresses: []string{},
			},
		},
		Shell: &ShellPermission{
			Mode:     "blacklist",
			Commands: []string{},
		},
	}
}

func findProtectionConfigByBotID(repo *repository.ProtectionRepository, botID string) (*repository.ProtectionConfig, string, error) {
	configs, err := repo.GetEnabledProtectionConfigs()
	if err != nil {
		return nil, "", err
	}
	for _, c := range configs {
		if c.AssetID == botID {
			return c, c.AssetName, nil
		}
	}

	assetNames := []string{"openclaw", "Openclaw", "moltbot", "Moltbot"}
	for _, assetName := range assetNames {
		c, err := repo.GetProtectionConfig(assetName, botID)
		if err != nil {
			return nil, "", err
		}
		if c != nil {
			return c, assetName, nil
		}
	}

	return nil, "", nil
}

func applyProtectionPolicyRuntime(config *repository.ProtectionConfig, userRules []string) {
	if config == nil || config.AssetName == "" || config.AssetID == "" {
		return
	}

	runningProxy := proxy.GetProxyProtectionByAsset(config.AssetName, config.AssetID)
	if runningProxy != nil && runningProxy.IsRunning() {
		runtimeCfg := &proxy.ProtectionRuntimeConfig{
			AuditOnly:               config.AuditOnly,
			SingleSessionTokenLimit: config.SingleSessionTokenLimit,
			DailyTokenLimit:         config.DailyTokenLimit,
		}
		runningProxy.UpdateProtectionConfig(runtimeCfg)
		if userRules != nil {
			runningProxy.UpdateUserRules(userRules)
		}
	}

	if syncResult := core.SyncGatewaySandboxByAssetAndPlugin(config.AssetName, config.AssetID); strings.Contains(syncResult, `"success":false`) {
		logging.Warning("API: Failed to sync gateway sandbox for botId=%s: %s", config.AssetID, syncResult)
	}
}
