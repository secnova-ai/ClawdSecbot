package repository

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	"go_lib/core"

	_ "modernc.org/sqlite"
)

// setupTestDB 创建一个临时的内存数据库用于测试
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	// SQLite 的 PRAGMA 是连接级设置，限制连接池为 1 以确保每次复用同一连接。
	db.SetMaxOpenConns(1)
	// 开启外键约束，与生产 InitDB 行为一致，保证 ON DELETE CASCADE 生效。
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}
	if err := createAssetTables(db); err != nil {
		t.Fatalf("Failed to create tables: %v", err)
	}
	return db
}

// TestScanRepository_SaveAndGetLatest 验证保存和获取最新扫描结果
func TestScanRepository_SaveAndGetLatest(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewScanRepository(db)

	// 保存一条扫描记录
	record := &ScanRecord{
		ConfigFound: true,
		ConfigPath:  "/home/user/.openclaw/openclaw.json",
		Assets: []core.Asset{
			{
				Name:         "Openclaw",
				Type:         "Service",
				Version:      "1.0.0",
				Ports:        []int{18789},
				ServiceName:  "openclaw-gateway",
				ProcessPaths: []string{"/usr/local/bin/openclaw"},
				Metadata:     map[string]string{"config_path": "/home/user/.openclaw"},
			},
		},
		Risks: []core.Risk{
			{
				ID:          "config_perm_unsafe",
				Title:       "Config File Permission Unsafe",
				Description: "Config file permissions are 644, expected 600",
				Level:       core.RiskLevelCritical,
			},
		},
	}

	err := repo.SaveScanResult(record)
	if err != nil {
		t.Fatalf("SaveScanResult failed: %v", err)
	}

	if record.ID == 0 {
		t.Error("Expected scan ID to be set after save")
	}

	// 获取最新记录
	latest, err := repo.GetLatestScanResult()
	if err != nil {
		t.Fatalf("GetLatestScanResult failed: %v", err)
	}
	if latest == nil {
		t.Fatal("Expected non-nil result")
	}

	// 验证字段
	if !latest.ConfigFound {
		t.Error("Expected config_found to be true")
	}
	if latest.ConfigPath != "/home/user/.openclaw/openclaw.json" {
		t.Errorf("Expected config_path, got '%s'", latest.ConfigPath)
	}

	// 验证资产
	if len(latest.Assets) != 1 {
		t.Fatalf("Expected 1 asset, got %d", len(latest.Assets))
	}
	if latest.Assets[0].Name != "Openclaw" {
		t.Errorf("Expected asset name 'Openclaw', got '%s'", latest.Assets[0].Name)
	}
	if len(latest.Assets[0].Ports) != 1 || latest.Assets[0].Ports[0] != 18789 {
		t.Errorf("Expected port [18789], got %v", latest.Assets[0].Ports)
	}

	// 验证风险
	if len(latest.Risks) != 1 {
		t.Fatalf("Expected 1 risk, got %d", len(latest.Risks))
	}
	if latest.Risks[0].ID != "config_perm_unsafe" {
		t.Errorf("Expected risk ID 'config_perm_unsafe', got '%s'", latest.Risks[0].ID)
	}
	if latest.Risks[0].Level != core.RiskLevelCritical {
		t.Errorf("Expected risk level 'critical', got '%s'", latest.Risks[0].Level)
	}
}

// TestScanRepository_MultipleScans 验证多次扫描时获取最新的
func TestScanRepository_MultipleScans(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewScanRepository(db)

	// 保存第一次扫描
	record1 := &ScanRecord{
		ConfigFound: false,
		Assets:      []core.Asset{},
		Risks:       []core.Risk{},
	}
	if err := repo.SaveScanResult(record1); err != nil {
		t.Fatalf("SaveScanResult #1 failed: %v", err)
	}

	// 保存第二次扫描
	record2 := &ScanRecord{
		ConfigFound: true,
		ConfigPath:  "/home/.openclaw/openclaw.json",
		Assets: []core.Asset{
			{Name: "Openclaw", Type: "Service", Metadata: map[string]string{}},
		},
		Risks: []core.Risk{},
	}
	if err := repo.SaveScanResult(record2); err != nil {
		t.Fatalf("SaveScanResult #2 failed: %v", err)
	}

	// 获取最新应该是第二次
	latest, err := repo.GetLatestScanResult()
	if err != nil {
		t.Fatalf("GetLatestScanResult failed: %v", err)
	}
	if !latest.ConfigFound {
		t.Error("Expected latest scan to have config_found=true")
	}
	if len(latest.Assets) != 1 {
		t.Errorf("Expected 1 asset in latest scan, got %d", len(latest.Assets))
	}
}

// TestScanRepository_EmptyDB 验证空数据库返回nil
func TestScanRepository_EmptyDB(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewScanRepository(db)

	latest, err := repo.GetLatestScanResult()
	if err != nil {
		t.Fatalf("GetLatestScanResult failed: %v", err)
	}
	if latest != nil {
		t.Error("Expected nil for empty database")
	}
}

// TestScanRepository_NilDB 验证未初始化时返回错误
func TestScanRepository_NilDB(t *testing.T) {
	repo := NewScanRepository(nil)
	// GetDB() returns nil when not initialized, so db will be nil
	// We explicitly create with nil
	repo.db = nil

	_, err := repo.GetLatestScanResult()
	if err == nil {
		t.Error("Expected error for nil database")
	}

	err = repo.SaveScanResult(&ScanRecord{})
	if err == nil {
		t.Error("Expected error for nil database")
	}
}

// TestScanRepository_AssetMetadata 验证资产metadata的JSON序列化
func TestScanRepository_AssetMetadata(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewScanRepository(db)

	record := &ScanRecord{
		ConfigFound: true,
		Assets: []core.Asset{
			{
				Name: "Openclaw",
				Type: "Service",
				Metadata: map[string]string{
					"config_path":   "/home/.openclaw/openclaw.json",
					"gateway_bind":  "127.0.0.1",
					"gateway_port":  "18789",
					"auth_mode":     "token",
					"sandbox_mode":  "strict",
					"logging_redact": "on",
				},
			},
		},
		Risks: []core.Risk{},
	}

	if err := repo.SaveScanResult(record); err != nil {
		t.Fatalf("SaveScanResult failed: %v", err)
	}

	latest, err := repo.GetLatestScanResult()
	if err != nil {
		t.Fatalf("GetLatestScanResult failed: %v", err)
	}

	// 验证所有metadata字段都正确保存和恢复
	asset := latest.Assets[0]
	expectedMetadata := map[string]string{
		"config_path":   "/home/.openclaw/openclaw.json",
		"gateway_bind":  "127.0.0.1",
		"gateway_port":  "18789",
		"auth_mode":     "token",
		"sandbox_mode":  "strict",
		"logging_redact": "on",
	}

	for key, expected := range expectedMetadata {
		if got := asset.Metadata[key]; got != expected {
			t.Errorf("Metadata[%s]: expected '%s', got '%s'", key, expected, got)
		}
	}
}

// TestScanRepository_RiskWithMitigation 验证包含mitigation的风险保存和恢复
func TestScanRepository_RiskWithMitigation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewScanRepository(db)

	record := &ScanRecord{
		ConfigFound: true,
		Assets:      []core.Asset{},
		Risks: []core.Risk{
			{
				ID:          "sandbox_disabled_default",
				Title:       "Sandbox Disabled by Default",
				Description: "Default sandbox mode is set to 'none'",
				Level:       core.RiskLevelCritical,
				Mitigation: &core.Mitigation{
					Type: "suggestion",
					Suggestions: []core.SuggestionGroup{
						{
							Priority: "P0",
							Category: "Security",
							Items: []core.SuggestionItem{
								{Action: "Enable sandbox", Detail: "Set sandbox mode to 'strict'"},
							},
						},
					},
				},
			},
		},
	}

	if err := repo.SaveScanResult(record); err != nil {
		t.Fatalf("SaveScanResult failed: %v", err)
	}

	latest, err := repo.GetLatestScanResult()
	if err != nil {
		t.Fatalf("GetLatestScanResult failed: %v", err)
	}

	risk := latest.Risks[0]
	if risk.Mitigation == nil {
		t.Fatal("Expected mitigation to be saved")
	}
	if risk.Mitigation.Type != "suggestion" {
		t.Errorf("Expected mitigation type 'suggestion', got '%s'", risk.Mitigation.Type)
	}
	if len(risk.Mitigation.Suggestions) != 1 {
		t.Fatalf("Expected 1 suggestion group, got %d", len(risk.Mitigation.Suggestions))
	}
}

// TestScanRepository_RetainLatestScans 验证仅保留最近固定数量的扫描记录及其关联数据
func TestScanRepository_RetainLatestScans(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewScanRepository(db)

	const totalScans = 25
	for i := 1; i <= totalScans; i++ {
		record := &ScanRecord{
			ConfigFound: i%2 == 0,
			ConfigPath:  fmt.Sprintf("/tmp/scan-%d.json", i),
			Assets: []core.Asset{
				{
					Name: fmt.Sprintf("asset-%d", i),
					Type: "Service",
				},
			},
			Risks: []core.Risk{},
		}
		if err := repo.SaveScanResult(record); err != nil {
			t.Fatalf("SaveScanResult #%d failed: %v", i, err)
		}
	}

	var scanCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM scans`).Scan(&scanCount); err != nil {
		t.Fatalf("Failed to count scans: %v", err)
	}
	if scanCount != retainedScanCount {
		t.Fatalf("Expected %d scans retained, got %d", retainedScanCount, scanCount)
	}

	var assetCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM assets`).Scan(&assetCount); err != nil {
		t.Fatalf("Failed to count assets: %v", err)
	}
	if assetCount != retainedScanCount {
		t.Fatalf("Expected %d assets retained, got %d", retainedScanCount, assetCount)
	}

	var oldestRetainedConfigPath string
	if err := db.QueryRow(`SELECT config_path FROM scans ORDER BY id ASC LIMIT 1`).Scan(&oldestRetainedConfigPath); err != nil {
		t.Fatalf("Failed to query oldest retained scan: %v", err)
	}
	expectedOldestRetained := "/tmp/scan-6.json"
	if oldestRetainedConfigPath != expectedOldestRetained {
		t.Fatalf("Expected oldest retained config_path %s, got %s", expectedOldestRetained, oldestRetainedConfigPath)
	}
}

// TestInitDB_FileDB 验证文件数据库初始化
func TestInitDB_FileDB(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	defer os.Remove(tmpFile)

	err := InitDB(tmpFile)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer CloseDB()

	// 验证可以正常使用
	db := GetDB()
	if db == nil {
		t.Fatal("GetDB returned nil after InitDB")
	}

	repo := NewScanRepository(db)
	record := &ScanRecord{
		ConfigFound: false,
		Assets:      []core.Asset{},
		Risks:       []core.Risk{},
	}
	if err := repo.SaveScanResult(record); err != nil {
		t.Fatalf("SaveScanResult after InitDB failed: %v", err)
	}
}
