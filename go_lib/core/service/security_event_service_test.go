package service

import (
	"encoding/json"
	"testing"
)

// TestSaveSecurityEventsBatch 验证批量保存安全事件
func TestSaveSecurityEventsBatch(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	input := `[
		{
			"id": "sevt_1_1",
			"timestamp": "2026-03-15T10:00:00Z",
			"event_type": "tool_execution",
			"action_desc": "执行ls命令列出文件",
			"risk_type": "",
			"detail": "",
			"source": "react_agent"
		},
		{
			"id": "sevt_2_2",
			"timestamp": "2026-03-15T10:01:00Z",
			"event_type": "blocked",
			"action_desc": "尝试rm -rf /删除系统文件",
			"risk_type": "权限提升",
			"detail": "致命命令检测",
			"source": "heuristic"
		}
	]`

	result := SaveSecurityEventsBatch(input)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestSaveSecurityEventsBatch_InvalidJSON 验证JSON解析错误
func TestSaveSecurityEventsBatch_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := SaveSecurityEventsBatch("bad json")
	if result["success"] != false {
		t.Error("Expected success=false for invalid JSON")
	}
}

// TestGetSecurityEvents 验证获取安全事件
func TestGetSecurityEvents(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 先保存一条
	SaveSecurityEventsBatch(`[{
		"id": "sevt_test_1",
		"timestamp": "2026-03-15T10:00:00Z",
		"event_type": "tool_execution",
		"action_desc": "读取配置文件",
		"source": "react_agent",
		"asset_name": "openclaw",
		"asset_id": "asset_open_1"
	}]`)

	result := GetSecurityEvents(`{"limit": 10, "offset": 0}`)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}

	data := result["data"]
	events, ok := data.([]interface{})
	if !ok {
		// 可能是 JSON 格式，尝试 marshal/unmarshal
		b, _ := json.Marshal(data)
		var arr []interface{}
		json.Unmarshal(b, &arr)
		events = arr
	}
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
}

func TestGetSecurityEvents_FilterByAssetID(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveSecurityEventsBatch(`[
		{
			"id": "sevt_open_1",
			"timestamp": "2026-03-15T10:00:00Z",
			"event_type": "blocked",
			"action_desc": "openclaw blocked action",
			"source": "react_agent",
			"asset_name": "openclaw",
			"asset_id": "asset_open_1"
		},
		{
			"id": "sevt_null_1",
			"timestamp": "2026-03-15T10:01:00Z",
			"event_type": "blocked",
			"action_desc": "nullclaw blocked action",
			"source": "react_agent",
			"asset_name": "nullclaw",
			"asset_id": "asset_null_1"
		}
	]`)

	result := GetSecurityEvents(`{"limit": 10, "offset": 0, "asset_name": "wrong_name_should_be_ignored", "asset_id": "asset_null_1"}`)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}

	data := result["data"]
	events, ok := data.([]interface{})
	if !ok {
		b, _ := json.Marshal(data)
		var arr []interface{}
		json.Unmarshal(b, &arr)
		events = arr
	}
	if len(events) != 1 {
		t.Fatalf("Expected 1 filtered event, got %d", len(events))
	}
	eventMap, ok := events[0].(map[string]interface{})
	if !ok {
		b, _ := json.Marshal(events[0])
		_ = json.Unmarshal(b, &eventMap)
	}
	if got := eventMap["asset_id"]; got != "asset_null_1" {
		t.Fatalf("Expected asset_id=asset_null_1, got %v", got)
	}
}

// TestGetSecurityEvents_InvalidJSON 验证JSON解析错误
func TestGetSecurityEvents_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := GetSecurityEvents("bad")
	if result["success"] != false {
		t.Error("Expected success=false for invalid JSON")
	}
}

// TestGetSecurityEventCount 验证获取安全事件数量
func TestGetSecurityEventCount(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := GetSecurityEventCount()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestClearAllSecurityEvents 验证清空安全事件
func TestClearAllSecurityEvents(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := ClearAllSecurityEvents()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

func TestClearSecurityEvents_FilterByAssetID(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveSecurityEventsBatch(`[
		{
			"id": "sevt_open_clear_1",
			"timestamp": "2026-03-15T10:00:00Z",
			"event_type": "blocked",
			"action_desc": "open event",
			"source": "react_agent",
			"asset_name": "openclaw",
			"asset_id": "asset_open_1"
		},
		{
			"id": "sevt_null_clear_1",
			"timestamp": "2026-03-15T10:01:00Z",
			"event_type": "blocked",
			"action_desc": "null event",
			"source": "react_agent",
			"asset_name": "nullclaw",
			"asset_id": "asset_null_1"
		}
	]`)

	clearResult := ClearSecurityEvents(`{"asset_name":"wrong_name_should_be_ignored","asset_id":"asset_null_1"}`)
	if clearResult["success"] != true {
		t.Fatalf("Expected clear success=true, got: %v", clearResult)
	}

	result := GetSecurityEvents(`{"limit": 10, "offset": 0}`)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
	data := result["data"]
	events, ok := data.([]interface{})
	if !ok {
		b, _ := json.Marshal(data)
		var arr []interface{}
		_ = json.Unmarshal(b, &arr)
		events = arr
	}
	if len(events) != 1 {
		t.Fatalf("Expected 1 remaining event after filtered clear, got %d", len(events))
	}
}
