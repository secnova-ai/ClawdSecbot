package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"go_lib/core/logging"
	"go_lib/core/proxy"
	"go_lib/core/repository"
	"go_lib/core/shepherd"
)

// ========== 保护状态操作 ==========

// SaveProtectionState 保存保护状态
func SaveProtectionState(jsonStr string) map[string]interface{} {
	var state repository.ProtectionState
	if err := json.Unmarshal([]byte(jsonStr), &state); err != nil {
		logging.Error("Failed to parse protection state JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewProtectionRepository(nil)
	if err := repo.SaveProtectionState(&state); err != nil {
		logging.Error("Failed to save protection state: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// GetProtectionState 获取保护状态
func GetProtectionState() map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	state, err := repo.GetProtectionState()
	if err != nil {
		logging.Error("Failed to get protection state: %v", err)
		return errorResult(err)
	}

	if state == nil {
		return successDataResult(nil)
	}
	return successDataResult(state)
}

// ClearProtectionState 清空保护状态
func ClearProtectionState() map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	if err := repo.ClearProtectionState(); err != nil {
		logging.Error("Failed to clear protection state: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// ========== 保护配置操作 ==========

// SaveProtectionConfig 保存保护配置
// 采用 read-modify-write 模式：如果传入 JSON 未包含 bot_model_config，
// 则保留数据库中已有的 BotModelConfig，避免被覆盖擦除
func SaveProtectionConfig(jsonStr string) map[string]interface{} {
	var config repository.ProtectionConfig
	if err := json.Unmarshal([]byte(jsonStr), &config); err != nil {
		logging.Error("Failed to parse protection config JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewProtectionRepository(nil)

	// 如果传入的配置没有 BotModelConfig，从数据库中读取已有的值并保留
	if config.BotModelConfig == nil && strings.TrimSpace(config.AssetID) != "" {
		existing, err := repo.GetProtectionConfig(config.AssetName, config.AssetID)
		if err == nil && existing != nil && existing.BotModelConfig != nil {
			config.BotModelConfig = existing.BotModelConfig
		}
	}

	if err := repo.SaveProtectionConfig(&config); err != nil {
		logging.Error("Failed to save protection config: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// GetProtectionConfig 获取保护配置
func GetProtectionConfig(assetName string, assetID string) map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	config, err := repo.GetProtectionConfig(assetName, assetID)
	if err != nil {
		logging.Error("Failed to get protection config: %v", err)
		return errorResult(err)
	}

	if config == nil {
		return successDataResult(nil)
	}
	return successDataResult(config)
}

// GetEnabledProtectionConfigs 获取所有启用的保护配置
func GetEnabledProtectionConfigs() map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	configs, err := repo.GetEnabledProtectionConfigs()
	if err != nil {
		logging.Error("Failed to get enabled protection configs: %v", err)
		return errorResult(err)
	}

	return successDataResult(configs)
}

// GetActiveProtectionCount 获取正在防护中的资产数量
func GetActiveProtectionCount() map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	configs, err := repo.GetEnabledProtectionConfigs()
	if err != nil {
		logging.Error("Failed to get enabled protection configs: %v", err)
		return errorResult(err)
	}

	count := len(configs)
	logging.Info("Active protection count: %d", count)
	return successDataResult(map[string]interface{}{"count": count})
}

// SetProtectionEnabled 设置保护启用状态
func SetProtectionEnabled(jsonStr string) map[string]interface{} {
	var input struct {
		AssetName string `json:"asset_name"`
		AssetID   string `json:"asset_id"`
		Enabled   bool   `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &input); err != nil {
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewProtectionRepository(nil)
	if err := repo.SetProtectionEnabled(input.AssetName, input.AssetID, input.Enabled); err != nil {
		logging.Error("Failed to set protection enabled: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// DeleteProtectionConfig 删除保护配置
func DeleteProtectionConfig(assetName string, assetID string) map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	if err := repo.DeleteProtectionConfig(assetName, assetID); err != nil {
		logging.Error("Failed to delete protection config: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// ========== 保护统计操作 ==========

// SaveProtectionStatistics 保存保护统计
func SaveProtectionStatistics(jsonStr string) map[string]interface{} {
	var stats repository.ProtectionStatistics
	if err := json.Unmarshal([]byte(jsonStr), &stats); err != nil {
		logging.Error("Failed to parse protection statistics JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewProtectionRepository(nil)
	if err := repo.SaveProtectionStatistics(&stats); err != nil {
		logging.Error("Failed to save protection statistics: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// GetProtectionStatistics 获取保护统计
func GetProtectionStatistics(assetName string, assetID string) map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	stats, err := repo.GetProtectionStatistics(assetName, assetID)
	if err != nil {
		logging.Error("Failed to get protection statistics: %v", err)
		return errorResult(err)
	}

	if stats == nil {
		return successDataResult(nil)
	}
	return successDataResult(stats)
}

// ClearProtectionStatistics 清空保护统计
func ClearProtectionStatistics(assetName string, assetID string) map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	if err := repo.ClearProtectionStatistics(assetName, assetID); err != nil {
		logging.Error("Failed to clear protection statistics: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// ========== Shepherd规则操作 ==========

// GetShepherdSensitiveActions 获取Shepherd敏感操作
func GetShepherdSensitiveActions(assetName, assetID string) map[string]interface{} {
	_ = assetName
	if strings.TrimSpace(assetID) == "" {
		return errorResult(fmt.Errorf("asset_id is required"))
	}
	repo := repository.NewProtectionRepository(nil)
	actions, found, err := repo.GetShepherdSensitiveActions(assetID)
	if err != nil {
		logging.Error("Failed to get shepherd sensitive actions: %v", err)
		return errorResult(err)
	}
	if !found {
		defaultRules, defaultErr := shepherd.GetDefaultUserRules()
		if defaultErr != nil {
			logging.Error("Failed to load default shepherd rules: %v", defaultErr)
			return errorResult(defaultErr)
		}
		actions = defaultRules.SensitiveActions
	}
	return successDataResult(actions)
}

// SaveShepherdSensitiveActions 保存Shepherd敏感操作
func SaveShepherdSensitiveActions(jsonStr string) map[string]interface{} {
	var input struct {
		AssetName string   `json:"asset_name"`
		AssetID   string   `json:"asset_id"`
		Actions   []string `json:"actions"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &input); err != nil {
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	if strings.TrimSpace(input.AssetID) == "" {
		return errorResult(fmt.Errorf("asset_id is required"))
	}

	repo := repository.NewProtectionRepository(nil)
	if err := repo.SaveShepherdSensitiveActions(input.AssetName, input.AssetID, input.Actions); err != nil {
		logging.Error("Failed to save shepherd sensitive actions: %v", err)
		return errorResult(err)
	}

	rulesJSON, err := json.Marshal(map[string]interface{}{"SensitiveActions": input.Actions})
	if err != nil {
		return errorResult(err)
	}
	if result := proxy.UpdateShepherdRulesByAssetInternal(input.AssetName, input.AssetID, string(rulesJSON)); result != "ok" {
		return errorMessageResult(result)
	}

	return successResult()
}

// ========== 全局操作 ==========

// ClearAllData 清空所有运行数据
func ClearAllData() map[string]interface{} {
	repo := repository.NewProtectionRepository(nil)
	if err := repo.ClearAllData(); err != nil {
		logging.Error("Failed to clear all data: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// SaveHomeDirectoryPermission 保存Home目录授权状态
func SaveHomeDirectoryPermission(jsonStr string) map[string]interface{} {
	var input struct {
		Authorized     bool   `json:"authorized"`
		AuthorizedPath string `json:"authorized_path"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &input); err != nil {
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewProtectionRepository(nil)
	if err := repo.SaveHomeDirectoryPermission(input.Authorized, input.AuthorizedPath); err != nil {
		logging.Error("Failed to save home directory permission: %v", err)
		return errorResult(err)
	}

	return successResult()
}
