package service

import (
	"testing"
)

// TestSaveSecurityModelConfig 验证保存安全模型配置
func TestSaveSecurityModelConfig(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	input := `{
		"type": "openai",
		"endpoint": "https://api.openai.com/v1",
		"api_key": "sk-test-key",
		"model": "gpt-4"
	}`

	result := SaveSecurityModelConfig(input)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestSaveSecurityModelConfig_InvalidJSON 验证JSON解析错误
func TestSaveSecurityModelConfig_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := SaveSecurityModelConfig("bad json")
	if result["success"] != false {
		t.Error("Expected success=false for invalid JSON")
	}
}

// TestGetSecurityModelConfig 验证获取安全模型配置
func TestGetSecurityModelConfig(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 先保存
	SaveSecurityModelConfig(`{
		"type": "openai",
		"endpoint": "https://api.openai.com/v1",
		"api_key": "sk-test",
		"model": "gpt-4"
	}`)

	result := GetSecurityModelConfig()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
	if result["data"] == nil {
		t.Error("Expected non-nil data")
	}
}

// TestSaveBotModelConfig 验证保存Bot模型配置
func TestSaveBotModelConfig(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	input := `{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1",
		"provider": "openai",
		"base_url": "https://api.siliconflow.cn/v1",
		"api_key": "sk-test",
		"model": "deepseek-chat"
	}`

	result := SaveBotModelConfig(input)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestGetBotModelConfig 验证获取Bot模型配置
func TestGetBotModelConfig(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveBotModelConfig(`{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1",
		"provider": "openai",
		"base_url": "https://api.test.com/v1",
		"api_key": "sk-test",
		"model": "test-model"
	}`)

	result := GetBotModelConfig("openclaw", "openclaw:test-1")
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
	if result["data"] == nil {
		t.Fatal("Expected non-nil data")
	}
	dataMap := result["data"].(map[string]interface{})
	if dataMap["provider"] != "openai" {
		t.Errorf("Expected provider=openai, got %v", dataMap["provider"])
	}
	if dataMap["base_url"] != "https://api.test.com/v1" {
		t.Errorf("Expected base_url, got %v", dataMap["base_url"])
	}
	if dataMap["model"] != "test-model" {
		t.Errorf("Expected model=test-model, got %v", dataMap["model"])
	}
}

// TestGetBotModelConfig_NoLegacyFallback 验证按实例ID读取时不再回退到 legacy asset_id=""
func TestGetBotModelConfig_NoLegacyFallback(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveBotModelConfig(`{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1",
		"provider": "openai",
		"base_url": "https://api.legacy.com/v1",
		"api_key": "sk-legacy",
		"model": "legacy-model"
	}`)

	result := GetBotModelConfig("openclaw", "openclaw:instance-1")
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
	if result["data"] != nil {
		t.Fatalf("Expected nil data without legacy fallback, got: %#v", result["data"])
	}
}

// TestDeleteBotModelConfig 验证删除Bot模型配置
func TestDeleteBotModelConfig(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveBotModelConfig(`{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1",
		"provider": "openai",
		"base_url": "https://test.com/v1",
		"api_key": "sk-test",
		"model": "test"
	}`)

	result := DeleteBotModelConfig("openclaw", "openclaw:test-1")
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}

	// 验证已删除
	check := GetBotModelConfig("openclaw", "openclaw:test-1")
	if check["data"] != nil {
		t.Error("Expected nil data after deletion")
	}
}

// TestSaveBotModelConfig_NewProtectionConfig 验证在没有现有ProtectionConfig时保存BotModelConfig
func TestSaveBotModelConfig_NewProtectionConfig(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 直接保存BotModelConfig（无现有ProtectionConfig）
	input := `{
		"asset_name": "Openclaw",
		"asset_id": "openclaw:test-1",
		"provider": "anthropic",
		"base_url": "https://api.anthropic.com",
		"api_key": "sk-ant-test",
		"model": "claude-3.5-sonnet"
	}`
	result := SaveBotModelConfig(input)
	if result["success"] != true {
		t.Fatalf("SaveBotModelConfig failed: %v", result)
	}

	// 验证可以读回
	getResult := GetBotModelConfig("Openclaw", "openclaw:test-1")
	if getResult["success"] != true {
		t.Fatalf("GetBotModelConfig failed: %v", getResult)
	}
	if getResult["data"] == nil {
		t.Fatal("Bot model config not persisted - data is nil!")
	}
	dataMap := getResult["data"].(map[string]interface{})
	if dataMap["provider"] != "anthropic" {
		t.Errorf("Expected provider=anthropic, got %v", dataMap["provider"])
	}
}
