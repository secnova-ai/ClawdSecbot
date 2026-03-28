package service

import (
	"encoding/json"

	"go_lib/core/logging"
	"go_lib/core/repository"
)

// ========== 安全事件操作 ==========

// SaveSecurityEventsBatch 批量保存安全事件
func SaveSecurityEventsBatch(jsonStr string) map[string]interface{} {
	var events []*repository.SecurityEventRecord
	if err := json.Unmarshal([]byte(jsonStr), &events); err != nil {
		logging.Error("Failed to parse security events batch JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewSecurityEventRepository(nil)
	if err := repo.SaveSecurityEventsBatch(events); err != nil {
		logging.Error("Failed to save security events batch: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// GetSecurityEvents 获取安全事件列表
func GetSecurityEvents(filterJSON string) map[string]interface{} {
	var filter struct {
		Limit   int    `json:"limit"`
		Offset  int    `json:"offset"`
		AssetID string `json:"asset_id"`
	}
	if err := json.Unmarshal([]byte(filterJSON), &filter); err != nil {
		logging.Error("Failed to parse security event filter JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewSecurityEventRepository(nil)
	events, err := repo.GetSecurityEvents(filter.Limit, filter.Offset, filter.AssetID)
	if err != nil {
		logging.Error("Failed to get security events: %v", err)
		return errorResult(err)
	}

	return successDataResult(events)
}

// GetSecurityEventCount 获取安全事件数量
func GetSecurityEventCount() map[string]interface{} {
	repo := repository.NewSecurityEventRepository(nil)
	count, err := repo.GetSecurityEventCount("")
	if err != nil {
		logging.Error("Failed to get security event count: %v", err)
		return errorResult(err)
	}

	return successDataResult(count)
}

// ClearSecurityEvents 清空安全事件（仅按 asset_id 过滤）
func ClearSecurityEvents(filterJSON string) map[string]interface{} {
	var filter struct {
		AssetID string `json:"asset_id"`
	}
	if err := json.Unmarshal([]byte(filterJSON), &filter); err != nil {
		logging.Error("Failed to parse security event clear filter JSON: %v", err)
		return errorMessageResult("invalid JSON: " + err.Error())
	}

	repo := repository.NewSecurityEventRepository(nil)
	if err := repo.ClearAllSecurityEvents(filter.AssetID); err != nil {
		logging.Error("Failed to clear security events: %v", err)
		return errorResult(err)
	}

	return successResult()
}

// ClearAllSecurityEvents 清空所有安全事件
func ClearAllSecurityEvents() map[string]interface{} {
	return ClearSecurityEvents(`{}`)
}
