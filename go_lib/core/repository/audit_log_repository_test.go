package repository

import (
	"testing"
	"time"
)

func TestAuditLog_SaveAndGet(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewAuditLogRepository(db)

	log := &AuditLog{
		ID:             "log-001",
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		RequestID:      "req-001",
		Model:          "gpt-4",
		RequestContent: "test request",
		HasRisk:        true,
		RiskLevel:      "high",
		RiskReason:     "dangerous command",
		Action:         "WARN",
		PromptTokens:   100,
		TotalTokens:    200,
		DurationMs:     50,
	}

	err := repo.SaveAuditLog(log)
	if err != nil {
		t.Fatalf("SaveAuditLog failed: %v", err)
	}

	// 查询
	logs, err := repo.GetAuditLogs(&AuditLogFilter{Limit: 10})
	if err != nil {
		t.Fatalf("GetAuditLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("Expected 1 log, got %d", len(logs))
	}
	if logs[0].ID != "log-001" || logs[0].Action != "WARN" || !logs[0].HasRisk {
		t.Errorf("Log mismatch: %+v", logs[0])
	}
}

func TestAuditLog_BatchSave(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewAuditLogRepository(db)

	now := time.Now().UTC().Format(time.RFC3339)
	logs := []*AuditLog{
		{ID: "log-001", Timestamp: now, RequestID: "req-1", Action: "ALLOW"},
		{ID: "log-002", Timestamp: now, RequestID: "req-2", Action: "WARN", HasRisk: true},
		{ID: "log-003", Timestamp: now, RequestID: "req-3", Action: "BLOCK", HasRisk: true},
	}

	err := repo.SaveAuditLogsBatch(logs)
	if err != nil {
		t.Fatalf("SaveAuditLogsBatch failed: %v", err)
	}

	count, err := repo.GetAuditLogCount(false, "", "")
	if err != nil {
		t.Fatalf("GetAuditLogCount failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 logs, got %d", count)
	}

	riskCount, err := repo.GetAuditLogCount(true, "", "")
	if err != nil {
		t.Fatalf("GetAuditLogCount(riskOnly) failed: %v", err)
	}
	if riskCount != 2 {
		t.Errorf("Expected 2 risk logs, got %d", riskCount)
	}
}

func TestAuditLog_Statistics(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewAuditLogRepository(db)

	now := time.Now().UTC().Format(time.RFC3339)
	logs := []*AuditLog{
		{ID: "1", Timestamp: now, RequestID: "r1", Action: "ALLOW"},
		{ID: "2", Timestamp: now, RequestID: "r2", Action: "WARN", HasRisk: true},
		{ID: "3", Timestamp: now, RequestID: "r3", Action: "BLOCK", HasRisk: true},
		{ID: "4", Timestamp: now, RequestID: "r4", Action: "ALLOW"},
	}
	_ = repo.SaveAuditLogsBatch(logs)

	stats, err := repo.GetAuditLogStatistics("", "")
	if err != nil {
		t.Fatalf("GetAuditLogStatistics failed: %v", err)
	}
	if stats.Total != 4 {
		t.Errorf("Expected total=4, got %d", stats.Total)
	}
	if stats.RiskCount != 1 {
		t.Errorf("Expected risk_count=1 (WARN only), got %d", stats.RiskCount)
	}
	if stats.BlockedCount != 1 {
		t.Errorf("Expected blocked_count=1, got %d", stats.BlockedCount)
	}
	if stats.AllowedCount != 2 {
		t.Errorf("Expected allowed_count=2, got %d", stats.AllowedCount)
	}
}

func TestAuditLog_GetAuditLogAssets(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewAuditLogRepository(db)

	logs := []*AuditLog{
		{
			ID:        "1",
			Timestamp: time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
			RequestID: "r1",
			AssetName: "Bot A",
			AssetID:   "asset-a",
			Action:    "ALLOW",
		},
		{
			ID:        "2",
			Timestamp: time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339),
			RequestID: "r2",
			AssetName: "Bot B",
			AssetID:   "asset-b",
			Action:    "WARN",
			HasRisk:   true,
		},
		{
			ID:        "3",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			RequestID: "r3",
			AssetName: "Bot A",
			AssetID:   "asset-a",
			Action:    "ALLOW",
		},
		{
			ID:        "4",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			RequestID: "r4",
			Action:    "ALLOW",
		},
	}
	if err := repo.SaveAuditLogsBatch(logs); err != nil {
		t.Fatalf("SaveAuditLogsBatch failed: %v", err)
	}

	assets, err := repo.GetAuditLogAssets()
	if err != nil {
		t.Fatalf("GetAuditLogAssets failed: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("Expected 2 assets, got %d", len(assets))
	}
	if assets[0].AssetName != "Bot A" || assets[0].AssetID != "asset-a" {
		t.Errorf("Expected latest asset first to be Bot A/asset-a, got %+v", assets[0])
	}
	if assets[1].AssetName != "Bot B" || assets[1].AssetID != "asset-b" {
		t.Errorf("Expected second asset to be Bot B/asset-b, got %+v", assets[1])
	}
}

func TestAuditLog_Filter(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewAuditLogRepository(db)

	now := time.Now().UTC().Format(time.RFC3339)
	logs := []*AuditLog{
		{ID: "1", Timestamp: now, RequestID: "r1", Action: "ALLOW", RequestContent: "hello world"},
		{ID: "2", Timestamp: now, RequestID: "r2", Action: "WARN", HasRisk: true, RequestContent: "rm -rf /"},
	}
	_ = repo.SaveAuditLogsBatch(logs)

	// riskOnly过滤
	result, err := repo.GetAuditLogs(&AuditLogFilter{Limit: 10, RiskOnly: true})
	if err != nil {
		t.Fatalf("GetAuditLogs riskOnly failed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("Expected 1 risk log, got %d", len(result))
	}

	// 搜索过滤
	result, err = repo.GetAuditLogs(&AuditLogFilter{Limit: 10, SearchQuery: "hello"})
	if err != nil {
		t.Fatalf("GetAuditLogs search failed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("Expected 1 search result, got %d", len(result))
	}
}

func TestAuditLog_ClearAll(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewAuditLogRepository(db)

	now := time.Now().UTC().Format(time.RFC3339)
	_ = repo.SaveAuditLog(&AuditLog{ID: "1", Timestamp: now, RequestID: "r1", Action: "ALLOW"})

	err := repo.ClearAllAuditLogs()
	if err != nil {
		t.Fatalf("ClearAllAuditLogs failed: %v", err)
	}

	count, _ := repo.GetAuditLogCount(false, "", "")
	if count != 0 {
		t.Errorf("Expected 0 logs after clear, got %d", count)
	}
}
