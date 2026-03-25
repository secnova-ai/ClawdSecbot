package service

import (
	"encoding/json"

	"go_lib/core/logging"
	"go_lib/core/repository"
)

// ========== 审计日志操作 ==========

// SaveAuditLog 保存单条审计日志
func SaveAuditLog(jsonStr string) map[string]interface{} {
	var log repository.AuditLog
	if err := json.Unmarshal([]byte(jsonStr), &log); err != nil {
		logging.Error("Failed to parse audit log JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewAuditLogRepository(nil)
	if err := repo.SaveAuditLog(&log); err != nil {
		logging.Error("Failed to save audit log: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// SaveAuditLogsBatch 批量保存审计日志
func SaveAuditLogsBatch(jsonStr string) map[string]interface{} {
	var logs []*repository.AuditLog
	if err := json.Unmarshal([]byte(jsonStr), &logs); err != nil {
		logging.Error("Failed to parse audit logs batch JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewAuditLogRepository(nil)
	if err := repo.SaveAuditLogsBatch(logs); err != nil {
		logging.Error("Failed to save audit logs batch: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// GetAuditLogs 获取审计日志（支持过滤）
func GetAuditLogs(filterJSON string) map[string]interface{} {
	var filter repository.AuditLogFilter
	if err := json.Unmarshal([]byte(filterJSON), &filter); err != nil {
		logging.Error("Failed to parse audit log filter JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewAuditLogRepository(nil)
	logs, err := repo.GetAuditLogs(&filter)
	if err != nil {
		logging.Error("Failed to get audit logs: %v", err)
		return errorResult(err)
	}

	return successDataResult(logs)
}

// GetAuditLogCount 获取审计日志数量
func GetAuditLogCount(jsonStr string) map[string]interface{} {
	var input struct {
		RiskOnly  bool   `json:"risk_only"`
		AssetName string `json:"asset_name"`
		AssetID   string `json:"asset_id"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &input); err != nil {
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewAuditLogRepository(nil)
	count, err := repo.GetAuditLogCount(input.RiskOnly, input.AssetName, input.AssetID)
	if err != nil {
		logging.Error("Failed to get audit log count: %v", err)
		return errorResult(err)
	}

	return successDataResult(count)
}

// GetAuditLogStatistics 获取审计日志统计
func GetAuditLogStatistics(jsonStr string) map[string]interface{} {
	var input struct {
		AssetName string `json:"asset_name"`
		AssetID   string `json:"asset_id"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &input); err != nil {
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewAuditLogRepository(nil)
	stats, err := repo.GetAuditLogStatistics(input.AssetName, input.AssetID)
	if err != nil {
		logging.Error("Failed to get audit log statistics: %v", err)
		return errorResult(err)
	}

	return successDataResult(stats)
}

// CleanOldAuditLogs 清理旧审计日志
func CleanOldAuditLogs(jsonStr string) map[string]interface{} {
	var input struct {
		KeepDays int `json:"keep_days"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &input); err != nil {
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewAuditLogRepository(nil)
	if err := repo.CleanOldAuditLogs(input.KeepDays); err != nil {
		logging.Error("Failed to clean old audit logs: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// ClearAllAuditLogs 清空所有审计日志
func ClearAllAuditLogs(jsonStr string) map[string]interface{} {
	var input struct {
		AssetName string `json:"asset_name"`
		AssetID   string `json:"asset_id"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &input); err != nil {
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewAuditLogRepository(nil)
	if err := repo.ClearAllAuditLogs(input.AssetName, input.AssetID); err != nil {
		logging.Error("Failed to clear all audit logs: %v", err)
		return errorResult(err)
	}

	return successResult()
}
