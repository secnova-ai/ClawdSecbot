package service

import (
	"encoding/json"

	"go_lib/core/logging"
	"go_lib/core/repository"
)

// ========== API指标操作 ==========

// SaveApiMetrics 保存API指标
func SaveApiMetrics(jsonStr string) map[string]interface{} {
	var metrics repository.ApiMetrics
	if err := json.Unmarshal([]byte(jsonStr), &metrics); err != nil {
		logging.Error("Failed to parse api metrics JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewMetricsRepository(nil)
	if err := repo.SaveApiMetrics(&metrics); err != nil {
		logging.Error("Failed to save api metrics: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// GetApiStatistics 获取API统计数据
func GetApiStatistics(jsonStr string) map[string]interface{} {
	var input struct {
		DurationSeconds int    `json:"duration_seconds"`
		AssetName       string `json:"asset_name,omitempty"`
		AssetID         string `json:"asset_id,omitempty"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &input); err != nil {
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewMetricsRepository(nil)
	_ = input.AssetName
	stats, err := repo.GetApiStatistics(input.DurationSeconds, input.AssetID)
	if err != nil {
		logging.Error("Failed to get api statistics: %v", err)
		return errorResult(err)
	}

	return successDataResult(stats)
}

// GetRecentApiMetrics 获取最近的API指标
func GetRecentApiMetrics(jsonStr string) map[string]interface{} {
	var input struct {
		Limit int `json:"limit"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &input); err != nil {
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewMetricsRepository(nil)
	metrics, err := repo.GetRecentApiMetrics(input.Limit)
	if err != nil {
		logging.Error("Failed to get recent api metrics: %v", err)
		return errorResult(err)
	}

	return successDataResult(metrics)
}

// CleanOldApiMetrics 清理旧API指标
func CleanOldApiMetrics(jsonStr string) map[string]interface{} {
	var input struct {
		KeepDays int `json:"keep_days"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &input); err != nil {
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewMetricsRepository(nil)
	if err := repo.CleanOldApiMetrics(input.KeepDays); err != nil {
		logging.Error("Failed to clean old api metrics: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// GetDailyTokenUsage returns the daily token usage for the specified asset instance.
func GetDailyTokenUsage(assetID string) map[string]interface{} {
	repo := repository.NewMetricsRepository(nil)
	usage, err := repo.GetDailyTokenUsage(assetID)
	if err != nil {
		logging.Error("Failed to get daily token usage: %v", err)
		return errorResult(err)
	}

	return successDataResult(usage)
}
