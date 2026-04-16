package service

import (
	"encoding/json"
	"strings"
	"testing"

	"go_lib/core/repository"
)

// TestSaveScanResult 验证保存扫描结果
func TestSaveScanResult(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	input := `{
		"config_found": true,
		"config_path": "/home/user/.openclaw/openclaw.json",
		"assets": [{"name": "Openclaw", "type": "Service", "version": "1.0.0"}],
		"risks": [{"id": "test_risk", "title": "Test Risk", "level": "high"}]
	}`

	result := SaveScanResult(input)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
	if result["scan_id"] == nil {
		t.Error("Expected scan_id to be set")
	}
}

// TestSaveScanResult_InvalidJSON 验证JSON解析错误
func TestSaveScanResult_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := SaveScanResult("invalid json")
	if result["success"] != false {
		t.Error("Expected success=false for invalid JSON")
	}
	if result["error"] == nil {
		t.Error("Expected error message")
	}
}

// TestSaveScanResult_EmptyAssets 验证空资产列表
func TestSaveScanResult_EmptyAssets(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	input := `{"config_found": false, "assets": [], "risks": []}`
	result := SaveScanResult(input)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

// TestGetLatestScanResult 验证获取最新扫描结果
func TestGetLatestScanResult(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 先保存一条记录
	SaveScanResult(`{
		"config_found": true,
		"config_path": "/test/path",
		"assets": [{"name": "TestBot", "type": "Service"}],
		"risks": []
	}`)

	result := GetLatestScanResult()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
	if result["data"] == nil {
		t.Fatal("Expected non-nil data")
	}
}

// TestGetLatestScanResult_EmptyDB 验证空数据库返回nil data
func TestGetLatestScanResult_EmptyDB(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := GetLatestScanResult()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
	if result["data"] != nil {
		t.Errorf("Expected nil data for empty DB, got: %v", result["data"])
	}
}

// TestGetScannedSkillHashes 验证获取已扫描技能哈希
func TestGetScannedSkillHashes(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 先保存一条技能扫描
	SaveSkillScanResult(`{
		"skill_name": "test-skill",
		"skill_hash": "hash123",
		"safe": true,
		"issues": []
	}`)

	result := GetScannedSkillHashes()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}

	data, ok := result["data"].([]string)
	if !ok {
		t.Fatalf("Expected data to be []string, got: %T", result["data"])
	}
	if len(data) != 1 || data[0] != "hash123" {
		t.Errorf("Expected [hash123], got: %v", data)
	}
}

// TestSaveSkillScanResult 验证保存技能扫描结果
func TestSaveSkillScanResult(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	input := `{
		"skill_name": "dangerous-skill",
		"skill_hash": "xyz789",
		"safe": false,
		"issues": ["file access", "network access"]
	}`

	result := SaveSkillScanResult(input)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
}

func TestSaveSkillScanResult_PreservesStructuredIssueJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	input := `{
		"skill_name": "dangerous-skill",
		"skill_hash": "evidence-hash",
		"safe": false,
		"issues": [
			"{\"type\":\"prompt_injection\",\"severity\":\"high\",\"file\":\"SKILL.md\",\"description\":\"Injected template\",\"evidence\":\"prompt = f'Execute {user_input}'\"}"
		]
	}`

	result := SaveSkillScanResult(input)
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}

	saved := GetSkillScanByHash("evidence-hash")
	if saved["success"] != true {
		t.Fatalf("Expected saved lookup success=true, got: %v", saved)
	}

	data, ok := saved["data"].(*repository.SkillScanRecord)
	if !ok || data == nil {
		t.Fatalf("Expected SkillScanRecord, got %T", saved["data"])
	}
	if len(data.Issues) != 1 || data.Issues[0] == "" {
		t.Fatalf("Expected one persisted issue, got %+v", data.Issues)
	}
	if !strings.Contains(data.Issues[0], `"evidence":"prompt = f'Execute {user_input}'"`) {
		t.Fatalf("Expected evidence to be preserved, got %s", data.Issues[0])
	}
}

// TestSaveSkillScanResult_InvalidJSON 验证JSON解析错误
func TestSaveSkillScanResult_InvalidJSON(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := SaveSkillScanResult("not json")
	if result["success"] != false {
		t.Error("Expected success=false for invalid JSON")
	}
}

// TestGetSkillScanByHash 验证按哈希查询技能扫描
func TestGetSkillScanByHash(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveSkillScanResult(`{
		"skill_name": "test-skill",
		"skill_hash": "lookup_hash",
		"safe": true,
		"issues": []
	}`)

	result := GetSkillScanByHash("lookup_hash")
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}
	if result["data"] == nil {
		t.Error("Expected non-nil data")
	}
}

// TestDeleteSkillScan 验证删除技能扫描记录（软删除）
func TestDeleteSkillScan(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	SaveSkillScanResult(`{
		"skill_name": "to-delete",
		"skill_hash": "del_hash",
		"safe": true,
		"issues": []
	}`)

	result := DeleteSkillScan("del_hash")
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}

	// 验证记录仍可查询，且带删除标记
	check := GetSkillScanByHash("del_hash")
	data, ok := check["data"].(*repository.SkillScanRecord)
	if !ok || data == nil {
		t.Fatalf("Expected SkillScanRecord after soft delete, got %T", check["data"])
	}
	if data.DeletedAt == "" {
		t.Error("Expected deleted_at to be populated after soft delete")
	}
}

// TestGetRiskySkills 验证获取风险技能列表
func TestGetRiskySkills(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 保存一个安全的和一个有风险的
	SaveSkillScanResult(`{"skill_name": "safe-skill", "skill_hash": "safe1", "safe": true, "issues": []}`)
	SaveSkillScanResult(`{"skill_name": "risky-skill", "skill_hash": "risky1", "safe": false, "issues": ["danger"]}`)
	DeleteSkillScan("risky1")

	result := GetRiskySkills()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}

	// 序列化再解析验证数量
	dataJSON, _ := json.Marshal(result["data"])
	var records []map[string]interface{}
	json.Unmarshal(dataJSON, &records)
	if len(records) != 0 {
		t.Errorf("Expected deleted risky skill to be excluded, got %d", len(records))
	}
}

// TestGetAllSkillScans 验证获取所有技能扫描记录
func TestGetAllSkillScans(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 保存安全、有风险、已信任的技能
	SaveSkillScanResult(`{"skill_name": "safe-skill", "skill_hash": "hash_s", "safe": true, "issues": []}`)
	SaveSkillScanResult(`{"skill_name": "risky-skill", "skill_hash": "hash_r", "safe": false, "issues": ["danger"]}`)
	SaveSkillScanResult(`{"skill_name": "trusted-skill", "skill_hash": "hash_t", "safe": false, "issues": ["known-issue"]}`)
	TrustSkill("hash_t")

	result := GetAllSkillScans()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}

	dataJSON, _ := json.Marshal(result["data"])
	var records []map[string]interface{}
	json.Unmarshal(dataJSON, &records)

	// Should return ALL 3 records
	if len(records) != 3 {
		t.Errorf("Expected 3 skill scans, got %d", len(records))
	}
}

// TestGetAllSkillScans_Empty 验证空数据库返回空列表
func TestGetAllSkillScans_Empty(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	result := GetAllSkillScans()
	if result["success"] != true {
		t.Fatalf("Expected success=true, got: %v", result)
	}

	dataJSON, _ := json.Marshal(result["data"])
	var records []map[string]interface{}
	json.Unmarshal(dataJSON, &records)

	if len(records) != 0 {
		t.Errorf("Expected 0 skill scans, got %d", len(records))
	}
}
