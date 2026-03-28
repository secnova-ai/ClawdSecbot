package repository

import (
	"testing"
	"time"
)

func TestMetrics_SaveAndGetRecent(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewMetricsRepository(db)

	metrics := &ApiMetrics{
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		ToolCallCount:    2,
		Model:            "gpt-4",
		IsBlocked:        false,
		AssetName:        "openclaw",
		AssetID:          "openclaw:test-1",
	}

	err := repo.SaveApiMetrics(metrics)
	if err != nil {
		t.Fatalf("SaveApiMetrics failed: %v", err)
	}

	recent, err := repo.GetRecentApiMetrics(10)
	if err != nil {
		t.Fatalf("GetRecentApiMetrics failed: %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("Expected 1 metric, got %d", len(recent))
	}
	if recent[0].TotalTokens != 150 || recent[0].Model != "gpt-4" {
		t.Errorf("Metrics mismatch: %+v", recent[0])
	}
}

func TestMetrics_Statistics(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewMetricsRepository(db)

	now := time.Now().UTC().Format(time.RFC3339)
	for i := 0; i < 3; i++ {
		_ = repo.SaveApiMetrics(&ApiMetrics{
			Timestamp:        now,
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
			ToolCallCount:    1,
			AssetName:        "openclaw",
			AssetID:          "openclaw:test-1",
		})
	}
	_ = repo.SaveApiMetrics(&ApiMetrics{
		Timestamp:   now,
		TotalTokens: 200,
		IsBlocked:   true,
		AssetName:   "openclaw",
		AssetID:     "openclaw:test-1",
	})

	stats, err := repo.GetApiStatistics(86400, "openclaw:test-1")
	if err != nil {
		t.Fatalf("GetApiStatistics failed: %v", err)
	}
	if stats.RequestCount != 4 {
		t.Errorf("Expected 4 requests, got %d", stats.RequestCount)
	}
	if stats.TotalTokens != 650 {
		t.Errorf("Expected 650 total tokens, got %d", stats.TotalTokens)
	}
	if stats.BlockedCount != 1 {
		t.Errorf("Expected 1 blocked, got %d", stats.BlockedCount)
	}
}

func TestMetrics_DailyTokenUsage(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewMetricsRepository(db)

	now := time.Now().UTC().Format(time.RFC3339)
	_ = repo.SaveApiMetrics(&ApiMetrics{
		Timestamp:   now,
		TotalTokens: 500,
		AssetName:   "openclaw",
		AssetID:     "openclaw:test-1",
	})
	_ = repo.SaveApiMetrics(&ApiMetrics{
		Timestamp:   now,
		TotalTokens: 300,
		AssetName:   "openclaw",
		AssetID:     "openclaw:test-1",
	})

	usage, err := repo.GetDailyTokenUsage("openclaw:test-1")
	if err != nil {
		t.Fatalf("GetDailyTokenUsage failed: %v", err)
	}
	if usage != 800 {
		t.Errorf("Expected 800 daily tokens, got %d", usage)
	}
}

func TestMetrics_CleanOld(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewMetricsRepository(db)

	// 插入一条过期数据
	oldTime := time.Now().AddDate(0, 0, -10).UTC().Format(time.RFC3339)
	_ = repo.SaveApiMetrics(&ApiMetrics{
		Timestamp:   oldTime,
		TotalTokens: 100,
		AssetName:   "openclaw",
		AssetID:     "openclaw:test-1",
	})

	// 插入一条新数据
	now := time.Now().UTC().Format(time.RFC3339)
	_ = repo.SaveApiMetrics(&ApiMetrics{
		Timestamp:   now,
		TotalTokens: 200,
		AssetName:   "openclaw",
		AssetID:     "openclaw:test-1",
	})

	err := repo.CleanOldApiMetrics(7)
	if err != nil {
		t.Fatalf("CleanOldApiMetrics failed: %v", err)
	}

	recent, _ := repo.GetRecentApiMetrics(10)
	if len(recent) != 1 {
		t.Errorf("Expected 1 metric after clean, got %d", len(recent))
	}
}
