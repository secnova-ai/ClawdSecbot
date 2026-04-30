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

func TestShepherdRules_SaveRawStructuredRules(t *testing.T) {
	db := setupProtectionTestDB(t)
	defer db.Close()

	repo := NewProtectionRepository(db)
	raw := `{"semantic_rules":[{"id":"no_delete_files","enabled":true,"description":"不允许删除文件","applies_to":["tool_call"],"action":"needs_confirmation","risk_type":"HIGH_RISK_OPERATION"}]}`
	if err := repo.SaveShepherdRulesRaw("openclaw", "bot-structured", raw); err != nil {
		t.Fatalf("SaveShepherdRulesRaw failed: %v", err)
	}

	gotRaw, found, err := repo.GetShepherdRulesRaw("bot-structured")
	if err != nil {
		t.Fatalf("GetShepherdRulesRaw failed: %v", err)
	}
	if !found || gotRaw != raw {
		t.Fatalf("expected structured raw rules, found=%v raw=%s", found, gotRaw)
	}
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
		AssetName:                 "openclaw",
		AssetID:                   "openclaw:test-1",
		Enabled:                   true,
		AuditOnly:                 false,
		SandboxEnabled:            true,
		UserInputDetectionEnabled: true,
		SingleSessionTokenLimit:   1000,
		DailyTokenLimit:           5000,
		PathPermission:            `{"allowed":["/"]}`,
		NetworkPermission:         `{"allowed":["*"]}`,
		ShellPermission:           `{"allowed":["ls"]}`,
	}
	err := repo.SaveProtectionConfig(config)
	if err != nil {
		t.Fatalf("SaveProtectionConfig failed: %v", err)
	}

	// 获取配置
	got, err := repo.GetProtectionConfig("openclaw:test-1")
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

	allConfigs, err := repo.GetAllProtectionConfigs()
	if err != nil {
		t.Fatalf("GetAllProtectionConfigs failed: %v", err)
	}
	if len(allConfigs) != 1 {
		t.Fatalf("Expected 1 total config, got %d", len(allConfigs))
	}

	// 禁用
	err = repo.SetProtectionEnabled("openclaw:test-1", false)
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

	allConfigs, err = repo.GetAllProtectionConfigs()
	if err != nil {
		t.Fatalf("GetAllProtectionConfigs failed: %v", err)
	}
	if len(allConfigs) != 1 {
		t.Fatalf("Expected disabled config to remain queryable, got %d", len(allConfigs))
	}

	// 删除
	err = repo.DeleteProtectionConfig("openclaw:test-1")
	if err != nil {
		t.Fatalf("DeleteProtectionConfig failed: %v", err)
	}
	got, err = repo.GetProtectionConfig("openclaw:test-1")
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

	got, err := repo.GetProtectionStatistics("openclaw:test-1")
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
	err = repo.ClearProtectionStatistics("openclaw:test-1")
	if err != nil {
		t.Fatalf("ClearProtectionStatistics failed: %v", err)
	}
	got, err = repo.GetProtectionStatistics("openclaw:test-1")
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
	raw, found, err := repo.GetShepherdRulesRaw("bot-1")
	if err != nil {
		t.Fatalf("GetShepherdRulesRaw failed: %v", err)
	}
	if found {
		t.Fatalf("Expected found=false initially")
	}
	if raw != "" {
		t.Fatalf("Expected empty raw rules, got %s", raw)
	}

	// 保存
	raw1 := `{"semantic_rules":[{"id":"rule1","enabled":true},{"id":"rule2","enabled":true}]}`
	err = repo.SaveShepherdRulesRaw("openclaw", "bot-1", raw1)
	if err != nil {
		t.Fatalf("SaveShepherdRulesRaw failed: %v", err)
	}

	// 获取
	raw, found, err = repo.GetShepherdRulesRaw("bot-1")
	if err != nil {
		t.Fatalf("GetShepherdRulesRaw failed: %v", err)
	}
	if !found {
		t.Fatalf("Expected found=true after save")
	}
	if raw != raw1 {
		t.Errorf("Rules mismatch: %s", raw)
	}

	// 保存另一个资产的规则，不影响第一个
	err = repo.SaveShepherdRulesRaw("other_bot", "bot-9", `{"semantic_rules":[{"id":"rule3","enabled":true}]}`)
	if err != nil {
		t.Fatalf("SaveShepherdRulesRaw failed: %v", err)
	}
	raw, found, err = repo.GetShepherdRulesRaw("bot-1")
	if err != nil {
		t.Fatalf("GetShepherdRulesRaw failed: %v", err)
	}
	if !found {
		t.Fatalf("Expected found=true for openclaw after other bot save")
	}
	if raw != raw1 {
		t.Fatalf("Expected openclaw rules unchanged, got %s", raw)
	}

	raw2 := `{"semantic_rules":[{"id":"rule-x","enabled":true}]}`
	err = repo.SaveShepherdRulesRaw("openclaw", "bot-2", raw2)
	if err != nil {
		t.Fatalf("SaveShepherdRulesRaw failed: %v", err)
	}
	raw, found, err = repo.GetShepherdRulesRaw("bot-2")
	if err != nil {
		t.Fatalf("GetShepherdRulesRaw failed: %v", err)
	}
	if !found {
		t.Fatalf("Expected found=true for bot-2")
	}
	if raw != raw2 {
		t.Fatalf("Expected isolated rules for bot-2, got %s", raw)
	}
	raw, found, err = repo.GetShepherdRulesRaw("bot-1")
	if err != nil {
		t.Fatalf("GetShepherdRulesRaw failed: %v", err)
	}
	if !found {
		t.Fatalf("Expected found=true for bot-1")
	}
	if raw != raw1 {
		t.Fatalf("Expected bot-1 rules unchanged, got %s", raw)
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

func TestShouldLogEnabledProtectionConfigs_ContentChangeWithSameCount(t *testing.T) {
	enabledConfigLogMu.Lock()
	lastEnabledConfigSignature = ""
	enabledConfigLogInitialized = false
	enabledConfigLogMu.Unlock()

	base := []*ProtectionConfig{
		{
			AssetName:               "openclaw",
			AssetID:                 "asset-1",
			Enabled:                 true,
			SandboxEnabled:          true,
			SingleSessionTokenLimit: 1000,
			DailyTokenLimit:         5000,
			PathPermission:          `{"allowed":["/tmp"]}`,
		},
	}

	if !shouldLogEnabledProtectionConfigs(base) {
		t.Fatal("Expected first evaluation to require logging")
	}

	if shouldLogEnabledProtectionConfigs(base) {
		t.Fatal("Expected same content to skip logging")
	}

	// 数量不变但配置内容变化，仍应触发日志。
	changed := []*ProtectionConfig{
		{
			AssetName:               "openclaw",
			AssetID:                 "asset-1",
			Enabled:                 true,
			SandboxEnabled:          true,
			SingleSessionTokenLimit: 2000,
			DailyTokenLimit:         5000,
			PathPermission:          `{"allowed":["/tmp"]}`,
		},
	}

	if !shouldLogEnabledProtectionConfigs(changed) {
		t.Fatal("Expected content change with same count to require logging")
	}
}

func TestBuildEnabledProtectionConfigsSignature_OrderInsensitive(t *testing.T) {
	configA := &ProtectionConfig{
		AssetName:      "openclaw",
		AssetID:        "asset-a",
		Enabled:        true,
		PathPermission: `{"allowed":["/a"]}`,
	}
	configB := &ProtectionConfig{
		AssetName:      "hermes",
		AssetID:        "asset-b",
		Enabled:        true,
		PathPermission: `{"allowed":["/b"]}`,
	}

	signature1 := buildEnabledProtectionConfigsSignature([]*ProtectionConfig{
		configA,
		configB,
	})
	signature2 := buildEnabledProtectionConfigsSignature([]*ProtectionConfig{
		configB,
		configA,
	})

	if signature1 != signature2 {
		t.Fatalf("Expected signature to be order-insensitive, got %q and %q", signature1, signature2)
	}
}
