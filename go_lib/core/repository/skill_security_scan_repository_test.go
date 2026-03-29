package repository

import (
	"testing"
)

// TestSkillSecurityScanRepository_SaveAndGet 验证保存和查询技能扫描结果
func TestSkillSecurityScanRepository_SaveAndGet(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSkillSecurityScanRepository(db)

	// 保存一条安全的技能扫描结果
	err := repo.SaveSkillScanResult(&SkillScanRecord{
		SkillName: "test-skill",
		SkillHash: "abc123",
		Safe:      true,
		Issues:    []string{},
	})
	if err != nil {
		t.Fatalf("SaveSkillScanResult failed: %v", err)
	}

	// 通过哈希查询
	record, err := repo.GetSkillScanByHash("abc123")
	if err != nil {
		t.Fatalf("GetSkillScanByHash failed: %v", err)
	}
	if record == nil {
		t.Fatal("Expected non-nil record")
	}

	if record.SkillName != "test-skill" {
		t.Errorf("Expected skill name 'test-skill', got '%s'", record.SkillName)
	}
	if record.SkillHash != "abc123" {
		t.Errorf("Expected hash 'abc123', got '%s'", record.SkillHash)
	}
	if !record.Safe {
		t.Error("Expected safe=true")
	}
}

// TestSkillSecurityScanRepository_SaveUnsafeSkill 验证保存有风险的技能
func TestSkillSecurityScanRepository_SaveUnsafeSkill(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSkillSecurityScanRepository(db)

	err := repo.SaveSkillScanResult(&SkillScanRecord{
		SkillName: "risky-skill",
		SkillHash: "def456",
		Safe:      false,
		Issues:    []string{"Prompt injection detected", "Arbitrary code execution"},
	})
	if err != nil {
		t.Fatalf("SaveSkillScanResult failed: %v", err)
	}

	record, err := repo.GetSkillScanByHash("def456")
	if err != nil {
		t.Fatalf("GetSkillScanByHash failed: %v", err)
	}

	if record.Safe {
		t.Error("Expected safe=false")
	}
	if len(record.Issues) != 2 {
		t.Fatalf("Expected 2 issues, got %d", len(record.Issues))
	}
	if record.Issues[0] != "Prompt injection detected" {
		t.Errorf("Expected first issue 'Prompt injection detected', got '%s'", record.Issues[0])
	}
}

// TestSkillSecurityScanRepository_GetScannedHashes 验证获取所有已扫描哈希
func TestSkillSecurityScanRepository_GetScannedHashes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSkillSecurityScanRepository(db)

	// 保存多条记录
	skills := []SkillScanRecord{
		{SkillName: "skill-1", SkillHash: "hash1", Safe: true},
		{SkillName: "skill-2", SkillHash: "hash2", Safe: false, Issues: []string{"issue"}},
		{SkillName: "skill-3", SkillHash: "hash3", Safe: true},
	}
	for _, s := range skills {
		if err := repo.SaveSkillScanResult(&s); err != nil {
			t.Fatalf("SaveSkillScanResult failed: %v", err)
		}
	}

	hashes, err := repo.GetScannedSkillHashes()
	if err != nil {
		t.Fatalf("GetScannedSkillHashes failed: %v", err)
	}

	if len(hashes) != 3 {
		t.Fatalf("Expected 3 hashes, got %d", len(hashes))
	}

	// 验证所有哈希都在
	hashSet := make(map[string]bool)
	for _, h := range hashes {
		hashSet[h] = true
	}
	for _, expected := range []string{"hash1", "hash2", "hash3"} {
		if !hashSet[expected] {
			t.Errorf("Expected hash '%s' in result", expected)
		}
	}
}

// TestSkillSecurityScanRepository_GetRiskySkills 验证获取有风险的技能
func TestSkillSecurityScanRepository_GetRiskySkills(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSkillSecurityScanRepository(db)

	// 保存混合安全/风险记录
	skills := []SkillScanRecord{
		{SkillName: "safe-skill", SkillHash: "hash1", Safe: true},
		{SkillName: "risky-1", SkillHash: "hash2", Safe: false, Issues: []string{"issue1"}},
		{SkillName: "risky-2", SkillHash: "hash3", Safe: false, Issues: []string{"issue2", "issue3"}},
	}
	for _, s := range skills {
		if err := repo.SaveSkillScanResult(&s); err != nil {
			t.Fatalf("SaveSkillScanResult failed: %v", err)
		}
	}

	risky, err := repo.GetRiskySkills()
	if err != nil {
		t.Fatalf("GetRiskySkills failed: %v", err)
	}

	if len(risky) != 2 {
		t.Fatalf("Expected 2 risky skills, got %d", len(risky))
	}

	// 验证只返回了unsafe的技能
	for _, r := range risky {
		if r.Safe {
			t.Errorf("Risky skill '%s' should have safe=false", r.SkillName)
		}
	}
}

// TestSkillSecurityScanRepository_DeleteSkillScan 验证删除技能扫描记录
func TestSkillSecurityScanRepository_DeleteSkillScan(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSkillSecurityScanRepository(db)

	// 保存一条记录
	err := repo.SaveSkillScanResult(&SkillScanRecord{
		SkillName: "to-delete",
		SkillHash: "hash999",
		Safe:      true,
	})
	if err != nil {
		t.Fatalf("SaveSkillScanResult failed: %v", err)
	}

	// 验证存在
	record, err := repo.GetSkillScanByHash("hash999")
	if err != nil || record == nil {
		t.Fatal("Expected record to exist before delete")
	}

	// 删除
	err = repo.DeleteSkillScan("to-delete")
	if err != nil {
		t.Fatalf("DeleteSkillScan failed: %v", err)
	}

	// 验证已删除
	record, err = repo.GetSkillScanByHash("hash999")
	if err != nil {
		t.Fatalf("GetSkillScanByHash after delete failed: %v", err)
	}
	if record != nil {
		t.Error("Expected record to be deleted")
	}
}

// TestSkillSecurityScanRepository_UpsertOnConflict 验证相同哈希的更新
func TestSkillSecurityScanRepository_UpsertOnConflict(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSkillSecurityScanRepository(db)

	// 首次保存 - 安全
	err := repo.SaveSkillScanResult(&SkillScanRecord{
		SkillName: "skill-v1",
		SkillHash: "same-hash",
		Safe:      true,
	})
	if err != nil {
		t.Fatalf("First save failed: %v", err)
	}

	// 再次保存相同哈希 - 有风险
	err = repo.SaveSkillScanResult(&SkillScanRecord{
		SkillName: "skill-v1",
		SkillHash: "same-hash",
		Safe:      false,
		Issues:    []string{"new issue found"},
	})
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// 验证已更新
	record, err := repo.GetSkillScanByHash("same-hash")
	if err != nil {
		t.Fatalf("GetSkillScanByHash failed: %v", err)
	}
	if record.Safe {
		t.Error("Expected safe=false after upsert")
	}
	if len(record.Issues) != 1 || record.Issues[0] != "new issue found" {
		t.Errorf("Expected issues to be updated, got %v", record.Issues)
	}

	// 验证没有重复记录
	hashes, _ := repo.GetScannedSkillHashes()
	if len(hashes) != 1 {
		t.Errorf("Expected 1 hash (no duplicates), got %d", len(hashes))
	}
}

// TestSkillSecurityScanRepository_GetNonExistentHash 验证查询不存在的哈希
func TestSkillSecurityScanRepository_GetNonExistentHash(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSkillSecurityScanRepository(db)

	record, err := repo.GetSkillScanByHash("nonexistent")
	if err != nil {
		t.Fatalf("GetSkillScanByHash failed: %v", err)
	}
	if record != nil {
		t.Error("Expected nil for non-existent hash")
	}
}

// TestSkillSecurityScanRepository_EmptyHashes 验证空数据库返回空列表
func TestSkillSecurityScanRepository_EmptyHashes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSkillSecurityScanRepository(db)

	hashes, err := repo.GetScannedSkillHashes()
	if err != nil {
		t.Fatalf("GetScannedSkillHashes failed: %v", err)
	}
	if len(hashes) != 0 {
		t.Errorf("Expected 0 hashes for empty DB, got %d", len(hashes))
	}
}

// TestSkillSecurityScanRepository_GetAllSkillScans 验证获取所有技能扫描记录
func TestSkillSecurityScanRepository_GetAllSkillScans(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSkillSecurityScanRepository(db)

	// 保存混合记录：安全、有风险、已信任
	skills := []SkillScanRecord{
		{SkillName: "safe-skill", SkillHash: "hash1", Safe: true, ScannedAt: "2026-01-01T00:00:00Z"},
		{SkillName: "risky-skill", SkillHash: "hash2", Safe: false, Issues: []string{"issue1"}, ScannedAt: "2026-01-02T00:00:00Z"},
		{SkillName: "trusted-skill", SkillHash: "hash3", Safe: false, Issues: []string{"issue2"}, ScannedAt: "2026-01-03T00:00:00Z"},
	}
	for _, s := range skills {
		if err := repo.SaveSkillScanResult(&s); err != nil {
			t.Fatalf("SaveSkillScanResult failed: %v", err)
		}
	}
	// Mark one as trusted
	if err := repo.TrustSkill("trusted-skill"); err != nil {
		t.Fatalf("TrustSkill failed: %v", err)
	}

	records, err := repo.GetAllSkillScans()
	if err != nil {
		t.Fatalf("GetAllSkillScans failed: %v", err)
	}

	// Should return ALL 3 records (safe + risky + trusted)
	if len(records) != 3 {
		t.Fatalf("Expected 3 records, got %d", len(records))
	}

	// Should be ordered by scanned_at DESC
	if records[0].SkillName != "trusted-skill" {
		t.Errorf("Expected first record to be 'trusted-skill' (latest), got '%s'", records[0].SkillName)
	}
	if records[2].SkillName != "safe-skill" {
		t.Errorf("Expected last record to be 'safe-skill' (earliest), got '%s'", records[2].SkillName)
	}

	// Verify trusted flag is returned correctly
	if !records[0].Trusted {
		t.Error("Expected trusted-skill to have trusted=true")
	}
	if records[1].Trusted {
		t.Error("Expected risky-skill to have trusted=false")
	}
}

// TestSkillSecurityScanRepository_GetAllSkillScansEmpty 验证空数据库返回空列表
func TestSkillSecurityScanRepository_GetAllSkillScansEmpty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSkillSecurityScanRepository(db)

	records, err := repo.GetAllSkillScans()
	if err != nil {
		t.Fatalf("GetAllSkillScans failed: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("Expected 0 records for empty DB, got %d", len(records))
	}
}

// TestSkillSecurityScanRepository_DeleteSkillScansNotIn_RemovesOrphans verifies orphaned records are deleted
func TestSkillSecurityScanRepository_DeleteSkillScansNotIn_RemovesOrphans(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSkillSecurityScanRepository(db)

	skills := []SkillScanRecord{
		{SkillName: "skill-a", SkillHash: "hash-a", Safe: true},
		{SkillName: "skill-b", SkillHash: "hash-b", Safe: false, Issues: []string{"issue"}},
		{SkillName: "skill-c", SkillHash: "hash-c", Safe: true},
	}
	for _, s := range skills {
		if err := repo.SaveSkillScanResult(&s); err != nil {
			t.Fatalf("SaveSkillScanResult failed: %v", err)
		}
	}

	// Only skill-a and skill-c exist on disk; skill-b was deleted
	deleted, err := repo.DeleteSkillScansNotIn([]string{"skill-a", "skill-c"})
	if err != nil {
		t.Fatalf("DeleteSkillScansNotIn failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Verify skill-b is gone
	record, err := repo.GetSkillScanByHash("hash-b")
	if err != nil {
		t.Fatalf("GetSkillScanByHash failed: %v", err)
	}
	if record != nil {
		t.Error("Expected skill-b record to be deleted")
	}

	// Verify skill-a and skill-c remain
	remaining, err := repo.GetAllSkillScans()
	if err != nil {
		t.Fatalf("GetAllSkillScans failed: %v", err)
	}
	if len(remaining) != 2 {
		t.Errorf("Expected 2 remaining records, got %d", len(remaining))
	}
}

// TestSkillSecurityScanRepository_DeleteSkillScansNotIn_EmptySlice verifies no deletion on empty input
func TestSkillSecurityScanRepository_DeleteSkillScansNotIn_EmptySlice(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSkillSecurityScanRepository(db)

	if err := repo.SaveSkillScanResult(&SkillScanRecord{
		SkillName: "skill-x", SkillHash: "hash-x", Safe: true,
	}); err != nil {
		t.Fatalf("SaveSkillScanResult failed: %v", err)
	}

	// Empty slice should not delete anything (safety guard)
	deleted, err := repo.DeleteSkillScansNotIn([]string{})
	if err != nil {
		t.Fatalf("DeleteSkillScansNotIn failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("Expected 0 deleted for empty slice, got %d", deleted)
	}

	records, err := repo.GetAllSkillScans()
	if err != nil {
		t.Fatalf("GetAllSkillScans failed: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("Expected 1 record to remain, got %d", len(records))
	}
}

// TestSkillSecurityScanRepository_DeleteSkillScansNotIn_AllExist verifies no deletion when all skills exist
func TestSkillSecurityScanRepository_DeleteSkillScansNotIn_AllExist(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSkillSecurityScanRepository(db)

	skills := []SkillScanRecord{
		{SkillName: "s1", SkillHash: "h1", Safe: true},
		{SkillName: "s2", SkillHash: "h2", Safe: true},
	}
	for _, s := range skills {
		if err := repo.SaveSkillScanResult(&s); err != nil {
			t.Fatalf("SaveSkillScanResult failed: %v", err)
		}
	}

	deleted, err := repo.DeleteSkillScansNotIn([]string{"s1", "s2"})
	if err != nil {
		t.Fatalf("DeleteSkillScansNotIn failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("Expected 0 deleted when all exist, got %d", deleted)
	}
}

// TestSkillSecurityScanRepository_DeleteSkillScansNotIn_NoneExist verifies all orphans are deleted
func TestSkillSecurityScanRepository_DeleteSkillScansNotIn_NoneExist(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewSkillSecurityScanRepository(db)

	skills := []SkillScanRecord{
		{SkillName: "old-1", SkillHash: "h1", Safe: true},
		{SkillName: "old-2", SkillHash: "h2", Safe: false, Issues: []string{"issue"}},
	}
	for _, s := range skills {
		if err := repo.SaveSkillScanResult(&s); err != nil {
			t.Fatalf("SaveSkillScanResult failed: %v", err)
		}
	}

	// All disk skills are different from DB records
	deleted, err := repo.DeleteSkillScansNotIn([]string{"new-1", "new-2"})
	if err != nil {
		t.Fatalf("DeleteSkillScansNotIn failed: %v", err)
	}
	if deleted != 2 {
		t.Errorf("Expected 2 deleted, got %d", deleted)
	}

	records, err := repo.GetAllSkillScans()
	if err != nil {
		t.Fatalf("GetAllSkillScans failed: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("Expected 0 records after cleanup, got %d", len(records))
	}
}
