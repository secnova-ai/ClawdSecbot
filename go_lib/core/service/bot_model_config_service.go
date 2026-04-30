// Package service provides business services for bot model configuration.
// BotModelConfig is stored as part of ProtectionConfig.
package service

import (
	"encoding/json"
	"strings"

	"go_lib/core/logging"
	"go_lib/core/repository"
)

// SaveBotModelConfig stores bot model config by asset ID.
// AssetName is optional display metadata; instance isolation is based on asset_id.
// Internally it updates ProtectionConfig.BotModelConfig.
func SaveBotModelConfig(jsonStr string) map[string]interface{} {
	var input struct {
		AssetName string `json:"asset_name"`
		AssetID   string `json:"asset_id"`
		Provider  string `json:"provider"`
		BaseURL   string `json:"base_url"`
		APIKey    string `json:"api_key"`
		Model     string `json:"model"`
		SecretKey string `json:"secret_key,omitempty"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &input); err != nil {
		logging.Error("Failed to parse bot model config JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	if strings.TrimSpace(input.AssetID) == "" {
		return errorMessageResult("asset_id is required")
	}
	input.AssetID = strings.TrimSpace(input.AssetID)
	input.AssetName = strings.TrimSpace(input.AssetName)

	repo := repository.NewProtectionRepository(nil)

	// Load existing protection config.
	config, err := repo.GetProtectionConfig(input.AssetID)
	if err != nil {
		logging.Error("Failed to get protection config: %v", err)
		return errorResult(err)
	}

	// Create config row when absent.
	if config == nil {
		assetName := input.AssetName
		if assetName == "" {
			assetName = inferAssetNameFromAssetID(input.AssetID)
		}
		config = &repository.ProtectionConfig{
			AssetName:                 assetName,
			AssetID:                   input.AssetID,
			UserInputDetectionEnabled: true,
		}
	} else {
		if input.AssetName != "" {
			config.AssetName = input.AssetName
		} else if strings.TrimSpace(config.AssetName) == "" {
			config.AssetName = inferAssetNameFromAssetID(input.AssetID)
		}
	}

	// Update bot model config.
	config.BotModelConfig = &repository.BotModelConfigData{
		Provider:  input.Provider,
		BaseURL:   input.BaseURL,
		APIKey:    input.APIKey,
		Model:     input.Model,
		SecretKey: input.SecretKey,
	}

	// Persist.
	if err := repo.SaveProtectionConfig(config); err != nil {
		logging.Error("Failed to save protection config with bot model: %v", err)
		return errorResult(err)
	}

	logging.Info("Bot model config saved via protection config: asset=%s (id=%s)", config.AssetName, input.AssetID)
	return successResult()
}

func inferAssetNameFromAssetID(assetID string) string {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return ""
	}
	parts := strings.SplitN(assetID, ":", 2)
	return strings.TrimSpace(parts[0])
}

// GetBotModelConfig loads bot model config for a specific asset instance.
// Internally it reads from ProtectionConfig.BotModelConfig.
func GetBotModelConfig(assetID string) map[string]interface{} {
	if strings.TrimSpace(assetID) == "" {
		return errorMessageResult("asset_id is required")
	}
	repo := repository.NewProtectionRepository(nil)
	config, err := repo.GetProtectionConfig(assetID)
	if err != nil {
		logging.Error("Failed to get protection config: %v", err)
		return errorResult(err)
	}

	if config == nil || config.BotModelConfig == nil {
		return successDataResult(nil)
	}

	// Build response payload.
	result := map[string]interface{}{
		"asset_name": config.AssetName,
		"asset_id":   assetID,
		"provider":   config.BotModelConfig.Provider,
		"base_url":   config.BotModelConfig.BaseURL,
		"api_key":    config.BotModelConfig.APIKey,
		"model":      config.BotModelConfig.Model,
		"secret_key": config.BotModelConfig.SecretKey,
	}

	return successDataResult(result)
}

// DeleteBotModelConfig deletes bot model config for a specific asset instance.
// Internally it clears ProtectionConfig.BotModelConfig.
func DeleteBotModelConfig(assetID string) map[string]interface{} {
	if strings.TrimSpace(assetID) == "" {
		return errorMessageResult("asset_id is required")
	}
	repo := repository.NewProtectionRepository(nil)

	// Load existing protection config.
	config, err := repo.GetProtectionConfig(assetID)
	if err != nil {
		logging.Error("Failed to get protection config: %v", err)
		return errorResult(err)
	}

	if config == nil {
		return successResult()
	}

	// Clear bot model config.
	config.BotModelConfig = nil

	// Persist.
	if err := repo.SaveProtectionConfig(config); err != nil {
		logging.Error("Failed to save protection config: %v", err)
		return errorResult(err)
	}

	logging.Info("Bot model config deleted via protection config: asset=%s (id=%s)", config.AssetName, assetID)
	return successResult()
}
