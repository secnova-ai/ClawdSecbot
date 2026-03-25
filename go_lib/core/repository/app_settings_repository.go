// Package repository 提供应用设置的数据库访问层
// 包括语言设置等全局配置
package repository

import (
	"database/sql"
	"fmt"
	"time"

	"go_lib/core/logging"
)

// AppSettingsRepository 应用设置仓库
// 封装了app_settings表的CRUD操作
type AppSettingsRepository struct {
	db *sql.DB
}

// NewAppSettingsRepository 创建应用设置仓库实例
// 如果db为nil，将尝试使用全局数据库连接
func NewAppSettingsRepository(db *sql.DB) *AppSettingsRepository {
	if db == nil {
		db = GetDB()
	}
	return &AppSettingsRepository{db: db}
}

// CreateAppSettingsTable 创建应用设置表结构
// 使用 IF NOT EXISTS 保证幂等性
func CreateAppSettingsTable(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("failed to create app_settings table: %w", err)
	}

	logging.Info("App settings table created/verified successfully")
	return nil
}

// SaveSetting 保存设置项（INSERT OR REPLACE）
func (r *AppSettingsRepository) SaveSetting(key, value string) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}

	updatedAt := time.Now().UTC().Format(time.RFC3339)

	_, err := r.db.Exec(`
		INSERT OR REPLACE INTO app_settings (key, value, updated_at)
		VALUES (?, ?, ?)
	`, key, value, updatedAt)
	if err != nil {
		return fmt.Errorf("failed to save setting %s: %w", key, err)
	}

	logging.Info("App setting saved: key=%s", key)
	return nil
}

// GetSetting 获取设置项
// 如果不存在返回空字符串
func (r *AppSettingsRepository) GetSetting(key string) (string, error) {
	if r.db == nil {
		return "", fmt.Errorf("database not initialized")
	}

	row := r.db.QueryRow(`
		SELECT value FROM app_settings WHERE key = ?
	`, key)

	var value string
	err := row.Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to query setting %s: %w", key, err)
	}

	return value, nil
}

// DeleteSetting 删除设置项
func (r *AppSettingsRepository) DeleteSetting(key string) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}

	_, err := r.db.Exec(`DELETE FROM app_settings WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("failed to delete setting %s: %w", key, err)
	}

	logging.Info("App setting deleted: key=%s", key)
	return nil
}

// 常用设置键常量
const (
	SettingKeyLanguage      = "language"
	SettingKeyIsFirstLaunch = "is_first_launch"
	SettingKeyAPIServer     = "api_server_enabled"
)
