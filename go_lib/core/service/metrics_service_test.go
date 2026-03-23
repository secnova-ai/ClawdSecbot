package service

import (
	"testing"
)

// TestSaveApiMetrics 验证保存API指标
func TestSaveApiMetrics(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	input := `{
		"prompt_tokens": 100,
		"completion_tokens": 200,
		"total_tokens": 300,
		"tool_call_count": 5,
		"model": "gpt-4",
		"is_blocked": false,
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1"
	}`

	result := SaveApiMetrics(input)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestSaveApiMetrics_InvalidJSON 验证JSON解析错误
func TestSaveApiMetrics_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := SaveApiMetrics("bad json")
	if result["success"] != false {
		t.Error("Expected success=false for invalid JSON")
	}
}

// TestGetApiStatistics 验证获取API统计
func TestGetApiStatistics(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveApiMetrics(`{
		"prompt_tokens": 100,
		"completion_tokens": 200,
		"total_tokens": 300,
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1"
	}`)

	result := GetApiStatistics(`{"duration_seconds": 3600, "asset_name": "openclaw", "asset_id":"openclaw:test-1"}`)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestGetApiStatistics_InvalidJSON 验证JSON解析错误
func TestGetApiStatistics_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := GetApiStatistics("bad")
	if result["success"] != false {
		t.Error("Expected success=false for invalid JSON")
	}
}

// TestGetRecentApiMetrics 验证获取最近的API指标
func TestGetRecentApiMetrics(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveApiMetrics(`{
		"prompt_tokens": 50,
		"total_tokens": 100,
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1"
	}`)

	result := GetRecentApiMetrics(`{"limit": 10}`)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestGetRecentApiMetrics_InvalidJSON 验证JSON解析错误
func TestGetRecentApiMetrics_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := GetRecentApiMetrics("bad")
	if result["success"] != false {
		t.Error("Expected success=false for invalid JSON")
	}
}

// TestCleanOldApiMetrics 验证清理旧指标
func TestCleanOldApiMetrics(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := CleanOldApiMetrics(`{"keep_days": 30}`)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestCleanOldApiMetrics_InvalidJSON 验证JSON解析错误
func TestCleanOldApiMetrics_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := CleanOldApiMetrics("bad")
	if result["success"] != false {
		t.Error("Expected success=false for invalid JSON")
	}
}

// TestGetDailyTokenUsage 验证获取每日token用量
func TestGetDailyTokenUsage(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := GetDailyTokenUsage("openclaw", "openclaw:test-1")
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}
