package repository

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// setupProtectionTestDB 创建包含所有表的测试数据库
func setupProtectionTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	if err := createAssetTables(db); err != nil {
		t.Fatalf("Failed to create asset tables: %v", err)
	}
	if err := createProtectionTables(db); err != nil {
		t.Fatalf("Failed to create protection tables: %v", err)
	}
	if err := createAuditLogTables(db); err != nil {
		t.Fatalf("Failed to create audit log tables: %v", err)
	}
	if err := createMetricsTables(db); err != nil {
		t.Fatalf("Failed to create metrics tables: %v", err)
	}
	if err := createAppPermissionsTables(db); err != nil {
		t.Fatalf("Failed to create app permissions tables: %v", err)
	}
	return db
}

func TestProtectionState_SaveAndGet(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewProtectionRepository(db)

	// 初始状态为空
	state, err := repo.GetProtectionState()
	if err != nil {
		t.Fatalf("GetProtectionState failed: %v", err)
	}
	if state != nil {
		t.Fatal("Expected nil state initially")
	}

	// 保存状态
	err = repo.SaveProtectionState(&ProtectionState{
		Enabled:      true,
		ProviderName: "openai",
		ProxyPort:    8080,
	})
	if err != nil {
		t.Fatalf("SaveProtectionState failed: %v", err)
	}

	// 获取状态
	state, err = repo.GetProtectionState()
	if err != nil {
		t.Fatalf("GetProtectionState failed: %v", err)
	}
	if state == nil {
		t.Fatal("Expected non-nil state")
	}
	if !state.Enabled {
		t.Error("Expected enabled=true")
	}
	if state.ProviderName != "openai" {
		t.Errorf("Expected provider_name=openai, got %s", state.ProviderName)
	}
	if state.ProxyPort != 8080 {
		t.Errorf("Expected proxy_port=8080, got %d", state.ProxyPort)
	}

	// 清空状态
	err = repo.ClearProtectionState()
	if err != nil {
		t.Fatalf("ClearProtectionState failed: %v", err)
	}
	state, err = repo.GetProtectionState()
	if err != nil {
		t.Fatalf("GetProtectionState failed: %v", err)
	}
	if state == nil {
		t.Fatal("Expected non-nil state after clear")
	}
	if state.Enabled {
		t.Error("Expected enabled=false after clear")
	}
}

func TestProtectionConfig_CRUD(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewProtectionRepository(db)

	// 保存配置
	config := &ProtectionConfig{
		AssetName:               "openclaw",
		AssetID:                 "openclaw:test-1",
		Enabled:                 true,
		AuditOnly:               false,
		SandboxEnabled:          true,
		SingleSessionTokenLimit: 1000,
		DailyTokenLimit:         5000,
		PathPermission:          `{"allowed":["/"]}`,
		NetworkPermission:       `{"allowed":["*"]}`,
		ShellPermission:         `{"allowed":["ls"]}`,
	}
	err := repo.SaveProtectionConfig(config)
	if err != nil {
		t.Fatalf("SaveProtectionConfig failed: %v", err)
	}

	// 获取配置
	got, err := repo.GetProtectionConfig("openclaw", "openclaw:test-1")
	if err != nil {
		t.Fatalf("GetProtectionConfig failed: %v", err)
	}
	if got == nil {
		t.Fatal("Expected non-nil config")
	}
	if !got.Enabled || got.AssetName != "openclaw" || !got.SandboxEnabled {
		t.Errorf("Config mismatch: %+v", got)
	}
	if got.SingleSessionTokenLimit != 1000 || got.DailyTokenLimit != 5000 {
		t.Errorf("Token limits mismatch: session=%d, daily=%d", got.SingleSessionTokenLimit, got.DailyTokenLimit)
	}

	// 获取已启用的配置
	enabled, err := repo.GetEnabledProtectionConfigs()
	if err != nil {
		t.Fatalf("GetEnabledProtectionConfigs failed: %v", err)
	}
	if len(enabled) != 1 {
		t.Fatalf("Expected 1 enabled config, got %d", len(enabled))
	}

	// 禁用
	err = repo.SetProtectionEnabled("openclaw", "openclaw:test-1", false)
	if err != nil {
		t.Fatalf("SetProtectionEnabled failed: %v", err)
	}
	enabled, err = repo.GetEnabledProtectionConfigs()
	if err != nil {
		t.Fatalf("GetEnabledProtectionConfigs failed: %v", err)
	}
	if len(enabled) != 0 {
		t.Fatalf("Expected 0 enabled configs, got %d", len(enabled))
	}

	// 删除
	err = repo.DeleteProtectionConfig("openclaw", "openclaw:test-1")
	if err != nil {
		t.Fatalf("DeleteProtectionConfig failed: %v", err)
	}
	got, err = repo.GetProtectionConfig("openclaw", "openclaw:test-1")
	if err != nil {
		t.Fatalf("GetProtectionConfig failed: %v", err)
	}
	if got != nil {
		t.Error("Expected nil config after delete")
	}
}

func TestProtectionStatistics_SaveAndGet(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewProtectionRepository(db)

	stats := &ProtectionStatistics{
		AssetName:     "openclaw",
		AssetID:       "openclaw:test-1",
		AnalysisCount: 100,
		MessageCount:  200,
		WarningCount:  5,
		BlockedCount:  2,
		TotalTokens:   50000,
		RequestCount:  150,
	}
	err := repo.SaveProtectionStatistics(stats)
	if err != nil {
		t.Fatalf("SaveProtectionStatistics failed: %v", err)
	}

	got, err := repo.GetProtectionStatistics("openclaw", "openclaw:test-1")
	if err != nil {
		t.Fatalf("GetProtectionStatistics failed: %v", err)
	}
	if got == nil {
		t.Fatal("Expected non-nil stats")
	}
	if got.AnalysisCount != 100 || got.WarningCount != 5 || got.TotalTokens != 50000 {
		t.Errorf("Stats mismatch: %+v", got)
	}

	// 清空
	err = repo.ClearProtectionStatistics("openclaw", "openclaw:test-1")
	if err != nil {
		t.Fatalf("ClearProtectionStatistics failed: %v", err)
	}
	got, err = repo.GetProtectionStatistics("openclaw", "openclaw:test-1")
	if err != nil {
		t.Fatalf("GetProtectionStatistics failed: %v", err)
	}
	if got != nil {
		t.Error("Expected nil stats after clear")
	}
}

func TestShepherdRules_SaveAndGet(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewProtectionRepository(db)

	// 初始为空
	actions, found, err := repo.GetShepherdSensitiveActions("openclaw:test-1")
	if err != nil {
		t.Fatalf("GetShepherdSensitiveActions failed: %v", err)
	}
	if found {
		t.Fatalf("Expected found=false initially")
	}
	if len(actions) != 0 {
		t.Fatalf("Expected 0 actions, got %d", len(actions))
	}

	// 保存
	err = repo.SaveShepherdSensitiveActions("openclaw", "openclaw:test-1", []string{"action1", "action2"})
	if err != nil {
		t.Fatalf("SaveShepherdSensitiveActions failed: %v", err)
	}

	// 获取
	actions, found, err = repo.GetShepherdSensitiveActions("openclaw:test-1")
	if err != nil {
		t.Fatalf("GetShepherdSensitiveActions failed: %v", err)
	}
	if !found {
		t.Fatalf("Expected found=true after save")
	}
	if len(actions) != 2 {
		t.Fatalf("Expected 2 actions, got %d", len(actions))
	}
	if actions[0] != "action1" || actions[1] != "action2" {
		t.Errorf("Actions mismatch: %v", actions)
	}

	// 保存另一个资产的规则，不影响第一个
	err = repo.SaveShepherdSensitiveActions("other_bot", "other:test-2", []string{"action3"})
	if err != nil {
		t.Fatalf("SaveShepherdSensitiveActions failed: %v", err)
	}
	actions, found, err = repo.GetShepherdSensitiveActions("openclaw:test-1")
	if err != nil {
		t.Fatalf("GetShepherdSensitiveActions failed: %v", err)
	}
	if !found {
		t.Fatalf("Expected found=true for openclaw after other bot save")
	}
	if len(actions) != 2 {
		t.Fatalf("Expected 2 actions for openclaw, got %d", len(actions))
	}
}

func TestClearAllData(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewProtectionRepository(db)

	// 清空不应出错
	err := repo.ClearAllData()
	if err != nil {
		t.Fatalf("ClearAllData failed: %v", err)
	}
}

func TestSaveHomeDirectoryPermission(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewProtectionRepository(db)

	err := repo.SaveHomeDirectoryPermission(true, "/Users/test")
	if err != nil {
		t.Fatalf("SaveHomeDirectoryPermission failed: %v", err)
	}

	// 验证写入
	var authorized int
	var path string
	err = db.QueryRow("SELECT home_dir_authorized, authorized_path FROM app_permissions WHERE id = 1").
		Scan(&authorized, &path)
	if err != nil {
		t.Fatalf("Failed to query app_permissions: %v", err)
	}
	if authorized != 1 || path != "/Users/test" {
		t.Errorf("Permission mismatch: authorized=%d, path=%s", authorized, path)
	}
}
