package service

import (
	"encoding/json"
	"testing"
)

// TestSaveProtectionState 验证保存保护状态
func TestSaveProtectionState(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	input := `{
		"enabled": true,
		"provider_name": "openai",
		"proxy_port": 8080,
		"original_base_url": "https://api.openai.com/v1"
	}`

	result := SaveProtectionState(input)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestGetProtectionState 验证获取保护状态
func TestGetProtectionState(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveProtectionState(`{
		"enabled": true,
		"provider_name": "openai",
		"proxy_port": 9090
	}`)

	result := GetProtectionState()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestClearProtectionState 验证清空保护状态
func TestClearProtectionState(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveProtectionState(`{"enabled": true, "provider_name": "test"}`)
	result := ClearProtectionState()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestSaveProtectionConfig 验证保存保护配置
func TestSaveProtectionConfig(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	input := `{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1",
		"enabled": true,
		"audit_only": false,
		"sandbox_enabled": true,
		"custom_security_prompt": "test prompt"
	}`

	result := SaveProtectionConfig(input)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestGetProtectionConfig 验证获取保护配置
func TestGetProtectionConfig(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveProtectionConfig(`{"asset_name": "openclaw", "asset_id":"openclaw:test-1", "enabled": true}`)

	result := GetProtectionConfig("openclaw:test-1")
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestGetEnabledProtectionConfigs 验证获取启用的保护配置
func TestGetEnabledProtectionConfigs(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveProtectionConfig(`{"asset_name": "openclaw", "asset_id":"openclaw:test-1", "enabled": true}`)

	result := GetEnabledProtectionConfigs()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestGetActiveProtectionCount 验证获取正在防护中的资产数量
func TestGetActiveProtectionCount(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 测试无防护资产
	result := GetActiveProtectionCount()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}

	data := result["data"].(map[string]interface{})
	var count int
	switch v := data["count"].(type) {
	case int:
		count = v
	case float64:
		count = int(v)
	default:
		t.Fatalf("Unexpected count type: %T", v)
	}

	if count != 0 {
		t.Errorf("Expected count=0, got: %d", count)
	}

	// 添加启用的防护配置
	SaveProtectionConfig(`{"asset_name": "openclaw", "asset_id":"openclaw:test-1", "enabled": true}`)
	SaveProtectionConfig(`{"asset_name": "test", "asset_id":"test:test-2", "enabled": true}`)

	// 再次检查
	result = GetActiveProtectionCount()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}

	data = result["data"].(map[string]interface{})
	switch v := data["count"].(type) {
	case int:
		count = v
	case float64:
		count = int(v)
	default:
		t.Fatalf("Unexpected count type: %T", v)
	}

	if count != 2 {
		t.Errorf("Expected count=2, got: %d", count)
	}

	// 禁用一个
	SetProtectionEnabled(`{"asset_name": "test", "asset_id":"test:test-2", "enabled": false}`)

	// 检查数量是否减少
	result = GetActiveProtectionCount()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}

	data = result["data"].(map[string]interface{})
	switch v := data["count"].(type) {
	case int:
		count = v
	case float64:
		count = int(v)
	default:
		t.Fatalf("Unexpected count type: %T", v)
	}

	if count != 1 {
		t.Errorf("Expected count=1, got: %d", count)
	}
}

// TestSetProtectionEnabled 验证设置保护启用状态
func TestSetProtectionEnabled(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveProtectionConfig(`{"asset_name": "openclaw", "asset_id":"openclaw:test-1", "enabled": false}`)

	result := SetProtectionEnabled(`{"asset_name": "openclaw", "asset_id":"openclaw:test-1", "enabled": true}`)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestSetProtectionEnabled_InvalidJSON 验证JSON解析错误
func TestSetProtectionEnabled_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := SetProtectionEnabled("bad")
	if result["success"] != false {
		t.Error("Expected success=false for invalid JSON")
	}
}

// TestDeleteProtectionConfig 验证删除保护配置
func TestDeleteProtectionConfig(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveProtectionConfig(`{"asset_name": "openclaw", "asset_id":"openclaw:test-1", "enabled": true}`)
	result := DeleteProtectionConfig("openclaw:test-1")
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestSaveProtectionStatistics 验证保存保护统计
func TestSaveProtectionStatistics(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	input := `{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1",
		"analysis_count": 10,
		"warning_count": 2,
		"blocked_count": 1,
		"total_tokens": 5000,
		"request_count": 50
	}`

	result := SaveProtectionStatistics(input)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestGetProtectionStatistics 验证获取保护统计
func TestGetProtectionStatistics(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveProtectionStatistics(`{"asset_name": "openclaw", "asset_id":"openclaw:test-1", "analysis_count": 5}`)

	result := GetProtectionStatistics("openclaw:test-1")
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestClearProtectionStatistics 验证清空保护统计
func TestClearProtectionStatistics(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveProtectionStatistics(`{"asset_name": "openclaw", "asset_id":"openclaw:test-1", "analysis_count": 5}`)
	result := ClearProtectionStatistics("openclaw:test-1")
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestSaveShepherdSensitiveActions 验证保存Shepherd敏感操作
func TestSaveShepherdSensitiveActions(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	input := `{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1",
		"actions": ["file_write", "shell_exec", "network_connect"]
	}`

	result := SaveShepherdSensitiveActions(input)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestGetShepherdSensitiveActions 验证获取Shepherd敏感操作
func TestGetShepherdSensitiveActions(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveShepherdSensitiveActions(`{"asset_name": "openclaw", "asset_id":"openclaw:test-1", "actions": ["file_write"]}`)

	result := GetShepherdSensitiveActions("openclaw:test-1")
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestGetShepherdSensitiveActions_DefaultFallback 验证未保存实例规则时返回默认内置规则
func TestGetShepherdSensitiveActions_DefaultFallback(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := GetShepherdSensitiveActions("openclaw:test-default")
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
	data, ok := result["data"].([]string)
	if !ok {
		raw, _ := json.Marshal(result["data"])
		var parsed []string
		_ = json.Unmarshal(raw, &parsed)
		data = parsed
	}
	if len(data) == 0 {
		t.Fatal("Expected default shepherd rules to be non-empty")
	}
}

// TestGetShepherdSensitiveActions_SavedEmptyRules 验证保存空规则后返回空列表
func TestGetShepherdSensitiveActions_SavedEmptyRules(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	saveResult := SaveShepherdSensitiveActions(`{"asset_name":"openclaw","asset_id":"openclaw:test-empty","actions":[]}`)
	if saveResult["success"] != true {
		t.Fatalf("SaveShepherdSensitiveActions failed: %v", saveResult)
	}

	result := GetShepherdSensitiveActions("openclaw:test-empty")
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
	data, ok := result["data"].([]string)
	if !ok {
		raw, _ := json.Marshal(result["data"])
		var parsed []string
		_ = json.Unmarshal(raw, &parsed)
		data = parsed
	}
	if len(data) != 0 {
		t.Fatalf("Expected empty saved shepherd rules, got: %v", data)
	}
}

func TestGetShepherdSensitiveActions_IsolatedByAssetID(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveShepherdSensitiveActions(`{"asset_name": "openclaw", "asset_id": "bot-1", "actions": ["file_write"]}`)
	SaveShepherdSensitiveActions(`{"asset_name": "openclaw", "asset_id": "bot-2", "actions": ["shell_exec"]}`)

	result1 := GetShepherdSensitiveActions("bot-1")
	if result1["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result1)
	}
	actions1 := result1["data"].([]string)
	if len(actions1) != 1 || actions1[0] != "file_write" {
		t.Fatalf("unexpected bot-1 rules: %v", actions1)
	}

	result2 := GetShepherdSensitiveActions("bot-2")
	if result2["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result2)
	}
	actions2 := result2["data"].([]string)
	if len(actions2) != 1 || actions2[0] != "shell_exec" {
		t.Fatalf("unexpected bot-2 rules: %v", actions2)
	}
}

// TestClearAllData 验证清空所有运行数据
func TestClearAllData(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveProtectionStatistics(`{"asset_name": "openclaw", "asset_id":"openclaw:test-1", "analysis_count": 5}`)
	result := ClearAllData()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestSaveHomeDirectoryPermission 验证保存Home目录授权
func TestSaveHomeDirectoryPermission(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	input := `{"authorized": true, "authorized_path": "/Users/test"}`
	result := SaveHomeDirectoryPermission(input)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestSaveHomeDirectoryPermission_InvalidJSON 验证JSON解析错误
func TestSaveHomeDirectoryPermission_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := SaveHomeDirectoryPermission("bad")
	if result["success"] != false {
		t.Error("Expected success=false for invalid JSON")
	}
}

// TestSaveShepherdSensitiveActions_InvalidJSON 验证JSON解析错误
func TestSaveShepherdSensitiveActions_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := SaveShepherdSensitiveActions("bad json")
	if result["success"] != false {
		t.Error("Expected success=false for invalid JSON")
	}
}

// TestSaveProtectionState_InvalidJSON 验证JSON解析错误
func TestSaveProtectionState_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := SaveProtectionState("not json")
	if result["success"] != false {
		t.Error("Expected success=false for invalid JSON")
	}
}

// TestSaveProtectionConfig_InvalidJSON 验证JSON解析错误
func TestSaveProtectionConfig_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := SaveProtectionConfig("not json")
	if result["success"] != false {
		t.Error("Expected success=false for invalid JSON")
	}
}

// TestSaveProtectionStatistics_InvalidJSON 验证JSON解析错误
func TestSaveProtectionStatistics_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := SaveProtectionStatistics("not json")
	if result["success"] != false {
		t.Error("Expected success=false for invalid JSON")
	}
}

// TestProtectionConfig_RoundTrip 验证保护配置的完整存取
func TestProtectionConfig_RoundTrip(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	input := `{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1",
		"enabled": true,
		"audit_only": true,
		"sandbox_enabled": false,
		"single_session_token_limit": 1000,
		"daily_token_limit": 50000
	}`

	SaveProtectionConfig(input)

	result := GetProtectionConfig("openclaw:test-1")
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}

	// 序列化验证数据
	dataJSON, _ := json.Marshal(result["data"])
	var config map[string]interface{}
	json.Unmarshal(dataJSON, &config)

	if config["asset_name"] != "openclaw" {
		t.Errorf("Expected asset_name=openclaw, got: %v", config["asset_name"])
	}
	if config["enabled"] != true {
		t.Errorf("Expected enabled=true, got: %v", config["enabled"])
	}
	if config["audit_only"] != true {
		t.Errorf("Expected audit_only=true, got: %v", config["audit_only"])
	}
}

// TestSaveProtectionConfig_PreservesBotModelConfig 验证 SaveProtectionConfig 不会擦除已有的 BotModelConfig
func TestSaveProtectionConfig_PreservesBotModelConfig(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 1. 先通过 SaveBotModelConfig 保存 bot 模型配置
	botInput := `{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1",
		"provider": "openai",
		"base_url": "https://api.openai.com/v1",
		"api_key": "sk-test-key",
		"model": "gpt-4"
	}`
	result := SaveBotModelConfig(botInput)
	if result["success"] != true {
		t.Fatalf("SaveBotModelConfig failed: %v", result)
	}

	// 2. 验证 bot 模型配置已保存
	getResult := GetBotModelConfig("openclaw:test-1")
	if getResult["success"] != true {
		t.Fatalf("GetBotModelConfig failed: %v", getResult)
	}
	data := getResult["data"].(map[string]interface{})
	if data["provider"] != "openai" {
		t.Fatalf("Expected provider=openai, got: %v", data["provider"])
	}

	// 3. 通过 SaveProtectionConfig 保存其他配置（不含 bot_model_config）
	// 模拟 Flutter ProtectionDatabaseService.saveProtectionConfig 的行为
	protInput := `{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1",
		"enabled": true,
		"audit_only": false,
		"sandbox_enabled": true,
		"single_session_token_limit": 2000,
		"daily_token_limit": 100000
	}`
	result = SaveProtectionConfig(protInput)
	if result["success"] != true {
		t.Fatalf("SaveProtectionConfig failed: %v", result)
	}

	// 4. 验证 bot 模型配置没有被擦除
	getResult = GetBotModelConfig("openclaw:test-1")
	if getResult["success"] != true {
		t.Fatalf("GetBotModelConfig after SaveProtectionConfig failed: %v", getResult)
	}
	data2 := getResult["data"]
	if data2 == nil {
		t.Fatal("BotModelConfig was erased by SaveProtectionConfig - expected it to be preserved")
	}
	preserved := data2.(map[string]interface{})
	if preserved["provider"] != "openai" {
		t.Errorf("Expected provider=openai after SaveProtectionConfig, got: %v", preserved["provider"])
	}
	if preserved["base_url"] != "https://api.openai.com/v1" {
		t.Errorf("Expected base_url preserved, got: %v", preserved["base_url"])
	}
	if preserved["api_key"] != "sk-test-key" {
		t.Errorf("Expected api_key preserved, got: %v", preserved["api_key"])
	}
	if preserved["model"] != "gpt-4" {
		t.Errorf("Expected model preserved, got: %v", preserved["model"])
	}
}

func TestSaveProtectionConfig_PreservesInheritedDefaultPolicyWhenFieldOmitted(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	initial := `{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1",
		"inherits_default_policy": true,
		"enabled": true,
		"audit_only": false,
		"sandbox_enabled": true
	}`
	result := SaveProtectionConfig(initial)
	if result["success"] != true {
		t.Fatalf("initial SaveProtectionConfig failed: %v", result)
	}

	// 模拟 UI 打开后未修改配置直接保存：旧 payload 不携带 inherits_default_policy。
	updateWithoutInheritanceFlag := `{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1",
		"enabled": true,
		"audit_only": false,
		"sandbox_enabled": true
	}`
	result = SaveProtectionConfig(updateWithoutInheritanceFlag)
	if result["success"] != true {
		t.Fatalf("SaveProtectionConfig without inheritance flag failed: %v", result)
	}

	getResult := GetProtectionConfig("openclaw:test-1")
	if getResult["success"] != true {
		t.Fatalf("GetProtectionConfig failed: %v", getResult)
	}
	dataJSON, _ := json.Marshal(getResult["data"])
	var config map[string]interface{}
	if err := json.Unmarshal(dataJSON, &config); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	if config["inherits_default_policy"] != true {
		t.Fatalf("expected inherits_default_policy to be preserved, got: %v", config["inherits_default_policy"])
	}
}

func TestSaveProtectionConfig_ClearsInheritedDefaultPolicyWhenContentChanges(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	initial := `{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1",
		"inherits_default_policy": true,
		"enabled": true,
		"audit_only": false,
		"sandbox_enabled": true,
		"single_session_token_limit": 1000
	}`
	result := SaveProtectionConfig(initial)
	if result["success"] != true {
		t.Fatalf("initial SaveProtectionConfig failed: %v", result)
	}

	changedWithoutInheritanceFlag := `{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-1",
		"enabled": true,
		"audit_only": true,
		"sandbox_enabled": true,
		"single_session_token_limit": 1000
	}`
	result = SaveProtectionConfig(changedWithoutInheritanceFlag)
	if result["success"] != true {
		t.Fatalf("SaveProtectionConfig changed payload failed: %v", result)
	}

	getResult := GetProtectionConfig("openclaw:test-1")
	if getResult["success"] != true {
		t.Fatalf("GetProtectionConfig failed: %v", getResult)
	}
	dataJSON, _ := json.Marshal(getResult["data"])
	var config map[string]interface{}
	if err := json.Unmarshal(dataJSON, &config); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	if config["inherits_default_policy"] != false {
		t.Fatalf("expected inherits_default_policy to be cleared, got: %v", config["inherits_default_policy"])
	}
}

// TestSaveProtectionConfig_PreservesInheritanceWhenPermissionsAreEmpty 验证：
// 默认策略以空字符串保存“无规则”，UI 重新打开未改动会回填
// {"mode":"blacklist","paths":[]} 这类等价空规则；保存时不应被误判为内容变更
// 而清掉 inherits_default_policy。
func TestSaveProtectionConfig_PreservesInheritanceWhenPermissionsAreEmpty(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	initial := `{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-empty-perms",
		"inherits_default_policy": true,
		"enabled": true,
		"audit_only": false,
		"sandbox_enabled": false,
		"path_permission": "",
		"network_permission": "",
		"shell_permission": ""
	}`
	if result := SaveProtectionConfig(initial); result["success"] != true {
		t.Fatalf("initial SaveProtectionConfig failed: %v", result)
	}

	uiPayload := `{
		"asset_name": "openclaw",
		"asset_id": "openclaw:test-empty-perms",
		"enabled": true,
		"audit_only": false,
		"sandbox_enabled": false,
		"path_permission": "{\"mode\":\"blacklist\",\"paths\":[]}",
		"network_permission": "{\"inbound\":{\"mode\":\"blacklist\",\"addresses\":[]},\"outbound\":{\"mode\":\"blacklist\",\"addresses\":[]}}",
		"shell_permission": "{\"mode\":\"blacklist\",\"commands\":[]}"
	}`
	if result := SaveProtectionConfig(uiPayload); result["success"] != true {
		t.Fatalf("SaveProtectionConfig UI payload failed: %v", result)
	}

	getResult := GetProtectionConfig("openclaw:test-empty-perms")
	if getResult["success"] != true {
		t.Fatalf("GetProtectionConfig failed: %v", getResult)
	}
	dataJSON, _ := json.Marshal(getResult["data"])
	var config map[string]interface{}
	if err := json.Unmarshal(dataJSON, &config); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	if config["inherits_default_policy"] != true {
		t.Fatalf("expected inherits_default_policy to remain true when permissions are semantically empty, got: %v", config["inherits_default_policy"])
	}
}
