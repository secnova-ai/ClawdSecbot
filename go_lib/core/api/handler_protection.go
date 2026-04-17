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

type protectionPolicyNotFoundError struct {
	botID string
}

func (e *protectionPolicyNotFoundError) Error() string {
	if e == nil || strings.TrimSpace(e.botID) == "" {
		return errProtectionPolicyNotFound.Error()
	}
	return errProtectionPolicyNotFound.Error() + ": " + strings.TrimSpace(e.botID)
}

func (e *protectionPolicyNotFoundError) Unwrap() error {
	return errProtectionPolicyNotFound
}

var (
	startProtectionProxyForPolicy = proxy.StartProtectionProxyInternal
	stopProtectionProxyForPolicy  = proxy.StopProtectionProxyByAssetInternal
	syncGatewaySandboxForPolicy   = core.SyncGatewaySandboxByAssetAndPlugin
)

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
	Key      string `json:"key"`
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
		if errors.Is(err, errProtectionPolicyNotFound) {
			missingBotID := extractProtectionPolicyMissingBotID(err, req.BotID)
			Error(w, http.StatusNotFound, CodeNotFound, "protection config not found for botId: "+missingBotID)
			return
		}
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
	if len(req.BotID) == 0 {
		if err := saveDefaultProtectionPolicy(repo, &req); err != nil {
			logging.Error("API: Failed to save default protection policy: %v", err)
			Error(w, http.StatusInternalServerError, CodeInternalError, "failed to save default protection policy: "+err.Error())
			return
		}

		Success(w, map[string]interface{}{
			"message": "default protection policy updated",
			"botId":   []string{},
			"default": true,
		})
		return
	}

	botIDs, err := resolveRequestedBotIDs(repo, req.BotID)
	if err != nil {
		if errors.Is(err, errProtectionPolicyNotFound) {
			missingBotID := extractProtectionPolicyMissingBotID(err, req.BotID)
			Error(w, http.StatusNotFound, CodeNotFound, "protection config not found for botId: "+missingBotID)
			return
		}
		logging.Error("API: Failed to resolve requested botIds: %v", err)
		Error(w, http.StatusInternalServerError, CodeInternalError, "failed to query protection configs")
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
	usesDefaultPolicy := false
	if config == nil {
		assetName, err := resolveAssetNameByBotID(repo, botID)
		if err != nil {
			return nil, err
		}
		defaultConfig, err := repo.GetDefaultProtectionConfig()
		if err != nil {
			return nil, err
		}
		if defaultConfig != nil {
			config = cloneProtectionConfig(defaultConfig)
			config.AssetName = assetName
			config.AssetID = botID
			usesDefaultPolicy = true
		} else {
			config = &repository.ProtectionConfig{
				AssetName: assetName,
				AssetID:   botID,
			}
		}
	}

	response := convertToExternalPolicy(config, botID)
	userRulesAssetID := botID
	userRulesAssetName := config.AssetName
	if usesDefaultPolicy {
		userRulesAssetID = repository.DefaultProtectionPolicyAssetID
		userRulesAssetName = repository.DefaultProtectionPolicyAssetName
	}
	userRules, found, err := repo.GetShepherdSensitiveActions(userRulesAssetName, userRulesAssetID)
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

func saveDefaultProtectionPolicy(repo *repository.ProtectionRepository, req *ProtectionPolicyRequest) error {
	config, err := repo.GetDefaultProtectionConfig()
	if err != nil {
		return err
	}
	previousConfig := cloneProtectionConfig(config)
	if config == nil {
		config = &repository.ProtectionConfig{
			AssetName: repository.DefaultProtectionPolicyAssetName,
			AssetID:   repository.DefaultProtectionPolicyAssetID,
		}
	}

	applyProtectionPolicyRequest(config, req)
	if err := repo.SaveProtectionConfig(config); err != nil {
		return err
	}

	if req.UserRules != nil {
		if err := repo.SaveShepherdSensitiveActions(
			repository.DefaultProtectionPolicyAssetName,
			repository.DefaultProtectionPolicyAssetID,
			req.UserRules,
		); err != nil {
			logging.Warning("API: Failed to save default user rules: %v", err)
		}
	}

	applyProtectionPolicyRuntime(previousConfig, config, req.UserRules)
	logging.Info("API: Default protection policy updated")
	return nil
}

func updateProtectionPolicyForBotID(repo *repository.ProtectionRepository, botID string, req *ProtectionPolicyRequest) error {
	existingConfig, assetName, err := findProtectionConfigByBotID(repo, botID)
	if err != nil {
		return err
	}
	if existingConfig == nil {
		assetName, err = resolveAssetNameByBotID(repo, botID)
		if err != nil {
			return err
		}
	}
	previousConfig := cloneProtectionConfig(existingConfig)

	config := &repository.ProtectionConfig{
		AssetName: assetName,
		AssetID:   botID,
	}
	if existingConfig != nil {
		config = existingConfig
	}

	applyProtectionPolicyRequest(config, req)

	if err := repo.SaveProtectionConfig(config); err != nil {
		return err
	}

	if req.UserRules != nil {
		if err := repo.SaveShepherdSensitiveActions(config.AssetName, botID, req.UserRules); err != nil {
			logging.Warning("API: Failed to update user rules: %v", err)
		}
	}

	applyProtectionPolicyRuntime(previousConfig, config, req.UserRules)
	logging.Info("API: Protection policy updated for botId=%s", botID)
	return nil
}

func applyProtectionPolicyRequest(config *repository.ProtectionConfig, req *ProtectionPolicyRequest) {
	if config == nil || req == nil {
		return
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
}

func resolveRequestedBotIDs(repo *repository.ProtectionRepository, requested []string) ([]string, error) {
	if len(requested) > 0 {
		botIDs := normalizeBotIDs(requested)
		for _, botID := range botIDs {
			if _, err := resolveAssetNameByBotID(repo, botID); err != nil {
				return nil, err
			}
		}
		return botIDs, nil
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

func resolveAssetNameByBotID(repo *repository.ProtectionRepository, botID string) (string, error) {
	config, assetName, err := findProtectionConfigByBotID(repo, botID)
	if err != nil {
		return "", err
	}
	if config != nil && strings.TrimSpace(assetName) != "" {
		return strings.TrimSpace(assetName), nil
	}

	plugin := core.GetPluginManager().GetPluginByAssetID(botID)
	if plugin != nil {
		assetName = strings.TrimSpace(plugin.GetAssetName())
		if assetName != "" {
			return assetName, nil
		}
	}

	return "", &protectionPolicyNotFoundError{botID: botID}
}

func extractProtectionPolicyMissingBotID(err error, fallback []string) string {
	var notFoundErr *protectionPolicyNotFoundError
	if errors.As(err, &notFoundErr) && notFoundErr != nil && strings.TrimSpace(notFoundErr.botID) != "" {
		return strings.TrimSpace(notFoundErr.botID)
	}
	for _, botID := range fallback {
		trimmed := strings.TrimSpace(botID)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
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
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return nil, "", nil
	}
	c, err := repo.GetProtectionConfig(botID)
	if err != nil {
		return nil, "", err
	}
	if c == nil {
		return nil, "", nil
	}
	return c, c.AssetName, nil
}

func cloneProtectionConfig(config *repository.ProtectionConfig) *repository.ProtectionConfig {
	if config == nil {
		return nil
	}
	cloned := *config
	if config.BotModelConfig != nil {
		botModelCopy := *config.BotModelConfig
		cloned.BotModelConfig = &botModelCopy
	}
	return &cloned
}

func buildPolicyProxyConfig(config *repository.ProtectionConfig) (*proxy.ProtectionConfig, error) {
	if config == nil {
		return nil, errors.New("protection config is nil")
	}
	if config.AssetName == "" || config.AssetID == "" {
		return nil, errors.New("asset identity is incomplete")
	}
	if config.BotModelConfig == nil {
		return nil, errors.New("bot model config is required")
	}

	securityModel, err := repository.NewSecurityModelConfigRepository(nil).Get()
	if err != nil {
		return nil, err
	}
	if securityModel == nil {
		return nil, errors.New("security model config is required")
	}

	return &proxy.ProtectionConfig{
		AssetName:     config.AssetName,
		AssetID:       config.AssetID,
		SecurityModel: securityModel,
		BotModel: &proxy.BotModelConfig{
			Provider:  config.BotModelConfig.Provider,
			BaseURL:   config.BotModelConfig.BaseURL,
			APIKey:    config.BotModelConfig.APIKey,
			Model:     config.BotModelConfig.Model,
			SecretKey: config.BotModelConfig.SecretKey,
		},
		Runtime: &proxy.ProtectionRuntimeConfig{
			AuditOnly:               config.AuditOnly,
			SingleSessionTokenLimit: config.SingleSessionTokenLimit,
			DailyTokenLimit:         config.DailyTokenLimit,
		},
	}, nil
}

func applyProtectionPolicyRuntime(previousConfig, config *repository.ProtectionConfig, userRules []string) {
	if config == nil || config.AssetName == "" || config.AssetID == "" {
		return
	}
	if repository.IsDefaultProtectionPolicyAssetID(config.AssetID) {
		return
	}

	wasEnabled := previousConfig != nil && previousConfig.Enabled
	oldSandboxEnabled := previousConfig != nil && previousConfig.SandboxEnabled

	if !wasEnabled && config.Enabled {
		protectionConfig, err := buildPolicyProxyConfig(config)
		if err != nil {
			logging.Warning("API: Failed to build proxy config for botId=%s: %v", config.AssetID, err)
			return
		}
		configJSON, err := json.Marshal(protectionConfig)
		if err != nil {
			logging.Warning("API: Failed to marshal proxy config for botId=%s: %v", config.AssetID, err)
			return
		}
		startResult := startProtectionProxyForPolicy(string(configJSON))
		if strings.Contains(startResult, `"success":false`) {
			logging.Warning("API: Failed to start protection proxy for botId=%s: %s", config.AssetID, startResult)
		}
		return
	}

	if wasEnabled && !config.Enabled {
		stopResult := stopProtectionProxyForPolicy(config.AssetID)
		if strings.Contains(stopResult, `"success":false`) {
			logging.Warning("API: Failed to stop protection proxy for botId=%s: %s", config.AssetID, stopResult)
		}
		return
	}

	runningProxy := proxy.GetProxyProtectionByAsset(config.AssetID)
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

	if config.SandboxEnabled || oldSandboxEnabled {
		if syncResult := syncGatewaySandboxForPolicy(config.AssetName, config.AssetID); strings.Contains(syncResult, `"success":false`) {
			logging.Warning("API: Failed to sync gateway sandbox for botId=%s: %s", config.AssetID, syncResult)
		}
	}
}

func applyDefaultProtectionPolicyToAssets(repo *repository.ProtectionRepository, assets []core.Asset) error {
	defaultConfig, err := repo.GetDefaultProtectionConfig()
	if err != nil {
		return err
	}
	if defaultConfig == nil {
		return nil
	}

	defaultUserRules, foundDefaultUserRules, err := repo.GetShepherdSensitiveActions(
		repository.DefaultProtectionPolicyAssetName,
		repository.DefaultProtectionPolicyAssetID,
	)
	if err != nil {
		logging.Warning("API: Failed to load default protection user rules: %v", err)
		defaultUserRules = nil
		foundDefaultUserRules = false
	}

	for _, asset := range assets {
		assetID := strings.TrimSpace(asset.ID)
		assetName := strings.TrimSpace(asset.Name)
		if assetID == "" || assetName == "" {
			continue
		}

		existingConfig, _, err := findProtectionConfigByBotID(repo, assetID)
		if err != nil {
			return err
		}
		if existingConfig != nil {
			continue
		}

		config := cloneProtectionConfig(defaultConfig)
		config.AssetName = assetName
		config.AssetID = assetID
		// When a default bot model is already configured, newly discovered assets
		// should start protection automatically after scan completion.
		if config.BotModelConfig != nil {
			config.Enabled = true
			if !defaultConfig.Enabled {
				config.AuditOnly = false
			}
		}

		if err := repo.SaveProtectionConfig(config); err != nil {
			return err
		}

		var runtimeUserRules []string
		if foundDefaultUserRules {
			runtimeUserRules = append([]string(nil), defaultUserRules...)
			if err := repo.SaveShepherdSensitiveActions(assetName, assetID, runtimeUserRules); err != nil {
				logging.Warning("API: Failed to copy default user rules for botId=%s: %v", assetID, err)
			}
		}

		applyProtectionPolicyRuntime(nil, config, runtimeUserRules)
		logging.Info("API: Applied default protection policy to newly discovered asset %s/%s", assetName, assetID)
	}

	return nil
}
