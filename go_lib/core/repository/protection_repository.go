package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go_lib/core/logging"
)

// ProtectionState 保护状态记录
type ProtectionState struct {
	Enabled         bool   `json:"enabled"`
	ProviderName    string `json:"provider_name,omitempty"`
	ProxyPort       int    `json:"proxy_port,omitempty"`
	OriginalBaseURL string `json:"original_base_url,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
}

// BotModelConfigData Bot模型配置数据（嵌入在ProtectionConfig中）
// 用于代理转发的目标LLM配置
type BotModelConfigData struct {
	Provider  string `json:"provider,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	Model     string `json:"model,omitempty"`
	SecretKey string `json:"secret_key,omitempty"`
}

// ProtectionConfig 保护配置记录
// 注意：CustomSecurityPrompt 已废弃，不再使用
// BotModelConfig 作为JSON字段嵌入，存储代理转发的目标LLM配置
type ProtectionConfig struct {
	AssetName               string              `json:"asset_name"`
	AssetID                 string              `json:"asset_id"`
	Enabled                 bool                `json:"enabled"`
	AuditOnly               bool                `json:"audit_only"`
	SandboxEnabled          bool                `json:"sandbox_enabled"`
	GatewayBinaryPath       string              `json:"gateway_binary_path,omitempty"`
	GatewayConfigPath       string              `json:"gateway_config_path,omitempty"`
	SingleSessionTokenLimit int                 `json:"single_session_token_limit"`
	DailyTokenLimit         int                 `json:"daily_token_limit"`
	PathPermission          string              `json:"path_permission,omitempty"`
	NetworkPermission       string              `json:"network_permission,omitempty"`
	ShellPermission         string              `json:"shell_permission,omitempty"`
	BotModelConfig          *BotModelConfigData `json:"bot_model_config,omitempty"`
	CreatedAt               string              `json:"created_at,omitempty"`
	UpdatedAt               string              `json:"updated_at,omitempty"`
}

// ProtectionStatistics 保护统计记录
type ProtectionStatistics struct {
	AssetName             string `json:"asset_name"`
	AssetID               string `json:"asset_id"`
	AnalysisCount         int    `json:"analysis_count"`
	MessageCount          int    `json:"message_count"`
	WarningCount          int    `json:"warning_count"`
	BlockedCount          int    `json:"blocked_count"`
	TotalTokens           int    `json:"total_tokens"`
	TotalPromptTokens     int    `json:"total_prompt_tokens"`
	TotalCompletionTokens int    `json:"total_completion_tokens"`
	TotalToolCalls        int    `json:"total_tool_calls"`
	RequestCount          int    `json:"request_count"`
	AuditTokens           int    `json:"audit_tokens"`
	AuditPromptTokens     int    `json:"audit_prompt_tokens"`
	AuditCompletionTokens int    `json:"audit_completion_tokens"`
	UpdatedAt             string `json:"updated_at,omitempty"`
}

// ProtectionRepository 保护相关数据仓库
// 管理 protection_state, protection_config, protection_statistics, shepherd_rules 表
type ProtectionRepository struct {
	db *sql.DB
}

const (
	DefaultProtectionPolicyAssetID   = "__default_protection_policy__"
	DefaultProtectionPolicyAssetName = "__default_protection_policy__"
)

func IsDefaultProtectionPolicyAssetID(assetID string) bool {
	return strings.TrimSpace(assetID) == DefaultProtectionPolicyAssetID
}

// NewProtectionRepository 创建保护数据仓库实例
func NewProtectionRepository(db *sql.DB) *ProtectionRepository {
	if db == nil {
		db = GetDB()
	}
	return &ProtectionRepository{db: db}
}

// --- Protection State ---

// SaveProtectionState 保存保护状态（全局唯一，id=1）
func (r *ProtectionRepository) SaveProtectionState(state *ProtectionState) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}

	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	enabled := 0
	if state.Enabled {
		enabled = 1
	}

	_, err := r.db.Exec(`
		INSERT OR REPLACE INTO protection_state (id, enabled, provider_name, proxy_port, original_base_url, updated_at)
		VALUES (1, ?, ?, ?, ?, ?)
	`, enabled, state.ProviderName, state.ProxyPort, state.OriginalBaseURL, state.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save protection state: %w", err)
	}

	logging.Info("Protection state saved: enabled=%v, provider=%s, port=%d", state.Enabled, state.ProviderName, state.ProxyPort)
	return nil
}

// GetProtectionState 获取保护状态
func (r *ProtectionRepository) GetProtectionState() (*ProtectionState, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	row := r.db.QueryRow(`SELECT enabled, provider_name, proxy_port, original_base_url, updated_at FROM protection_state WHERE id = 1`)

	var state ProtectionState
	var enabled int
	var providerName, originalBaseURL, updatedAt sql.NullString
	var proxyPort sql.NullInt64

	err := row.Scan(&enabled, &providerName, &proxyPort, &originalBaseURL, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get protection state: %w", err)
	}

	state.Enabled = enabled == 1
	state.ProviderName = providerName.String
	if proxyPort.Valid {
		state.ProxyPort = int(proxyPort.Int64)
	}
	state.OriginalBaseURL = originalBaseURL.String
	state.UpdatedAt = updatedAt.String

	return &state, nil
}

// ClearProtectionState 清空保护状态（重置为未启用）
func (r *ProtectionRepository) ClearProtectionState() error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(`
		INSERT OR REPLACE INTO protection_state (id, enabled, provider_name, proxy_port, original_base_url, updated_at)
		VALUES (1, 0, NULL, NULL, NULL, ?)
	`, now)
	if err != nil {
		return fmt.Errorf("failed to clear protection state: %w", err)
	}

	return nil
}

// --- Protection Config ---

// SaveProtectionConfig 保存保护配置（按资产实例ID）
func (r *ProtectionRepository) SaveProtectionConfig(config *ProtectionConfig) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}
	config.AssetID = strings.TrimSpace(config.AssetID)
	if config.AssetID == "" {
		return fmt.Errorf("asset_id is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if config.CreatedAt == "" {
		config.CreatedAt = now
	}
	config.UpdatedAt = now

	enabled := 0
	if config.Enabled {
		enabled = 1
	}
	auditOnly := 0
	if config.AuditOnly {
		auditOnly = 1
	}
	sandboxEnabled := 0
	if config.SandboxEnabled {
		sandboxEnabled = 1
	}

	// 序列化 BotModelConfig 为 JSON
	var botModelConfigJSON string
	if config.BotModelConfig != nil {
		jsonBytes, err := json.Marshal(config.BotModelConfig)
		if err != nil {
			return fmt.Errorf("failed to marshal bot model config: %w", err)
		}
		botModelConfigJSON = string(jsonBytes)
	}

	_, err := r.db.Exec(`
		INSERT OR REPLACE INTO protection_config 
		(asset_name, asset_id, enabled, audit_only, sandbox_enabled, gateway_binary_path, gateway_config_path,
		 custom_security_prompt, single_session_token_limit, daily_token_limit,
		 path_permission, network_permission, shell_permission, bot_model_config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, config.AssetName, config.AssetID, enabled, auditOnly, sandboxEnabled,
		config.GatewayBinaryPath, config.GatewayConfigPath,
		"", config.SingleSessionTokenLimit, config.DailyTokenLimit,
		config.PathPermission, config.NetworkPermission, config.ShellPermission,
		botModelConfigJSON, config.CreatedAt, config.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save protection config: %w", err)
	}

	logging.Info("Protection config saved: asset=%s (id=%s), enabled=%v", config.AssetName, config.AssetID, config.Enabled)
	return nil
}

// GetProtectionConfig returns the protection config for the specified asset instance.
func (r *ProtectionRepository) GetProtectionConfig(assetID string) (*ProtectionConfig, error) {
// GetDefaultProtectionConfig 获取默认防护策略配置。
func (r *ProtectionRepository) GetDefaultProtectionConfig() (*ProtectionConfig, error) {
	return r.GetProtectionConfig(DefaultProtectionPolicyAssetID)
}

	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return nil, fmt.Errorf("asset_id is required")
	}

	row := r.db.QueryRow(`SELECT asset_name, asset_id, enabled, audit_only, sandbox_enabled, 
		gateway_binary_path, gateway_config_path, custom_security_prompt, 
		single_session_token_limit, daily_token_limit, 
		path_permission, network_permission, shell_permission, bot_model_config, created_at, updated_at
		FROM protection_config WHERE asset_id = ?`, assetID)

	config, err := scanProtectionConfig(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get protection config: %w", err)
	}

	return config, nil
}

// GetEnabledProtectionConfigs 获取所有启用的保护配置
func (r *ProtectionRepository) GetEnabledProtectionConfigs() ([]*ProtectionConfig, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := r.db.Query(`SELECT asset_name, asset_id, enabled, audit_only, sandbox_enabled, 
		gateway_binary_path, gateway_config_path, custom_security_prompt, 
		single_session_token_limit, daily_token_limit, 
		path_permission, network_permission, shell_permission, bot_model_config, created_at, updated_at
		FROM protection_config WHERE enabled = 1 AND asset_id <> ?`, DefaultProtectionPolicyAssetID)
	if err != nil {
		return nil, fmt.Errorf("failed to query enabled protection configs: %w", err)
	}
	defer rows.Close()

	var configs []*ProtectionConfig
	for rows.Next() {
		config, err := scanProtectionConfigFromRows(rows)
		if err != nil {
			logging.Warning("Failed to scan protection config row: %v", err)
			continue
		}
		configs = append(configs, config)
	}

	if configs == nil {
		configs = []*ProtectionConfig{}
	}

	logging.Info("Enabled protection configs count: %d", len(configs))
	return configs, nil
}

// SetProtectionEnabled updates the enabled state for the specified asset instance.
func (r *ProtectionRepository) SetProtectionEnabled(assetID string, enabled bool) error {
// GetAllProtectionConfigs 获取所有保护配置
func (r *ProtectionRepository) GetAllProtectionConfigs() ([]*ProtectionConfig, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := r.db.Query(`SELECT asset_name, asset_id, enabled, audit_only, sandbox_enabled,
		gateway_binary_path, gateway_config_path, custom_security_prompt,
		single_session_token_limit, daily_token_limit,
		path_permission, network_permission, shell_permission, bot_model_config, created_at, updated_at
		FROM protection_config WHERE asset_id <> ?`, DefaultProtectionPolicyAssetID)
	if err != nil {
		return nil, fmt.Errorf("failed to query protection configs: %w", err)
	}
	defer rows.Close()

	var configs []*ProtectionConfig
	for rows.Next() {
		config, err := scanProtectionConfigFromRows(rows)
		if err != nil {
			logging.Warning("Failed to scan protection config row: %v", err)
			continue
		}
		configs = append(configs, config)
	}

	if configs == nil {
		configs = []*ProtectionConfig{}
	}

	logging.Info("Protection configs count: %d", len(configs))
	return configs, nil
}

	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return fmt.Errorf("asset_id is required")
	}

	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := r.db.Exec(`UPDATE protection_config SET enabled = ?, updated_at = ? WHERE asset_id = ?`,
		enabledInt, now, assetID)
	if err != nil {
		return fmt.Errorf("failed to set protection enabled: %w", err)
	}

	return nil
}

// DeleteProtectionConfig removes the protection config for the specified asset instance.
func (r *ProtectionRepository) DeleteProtectionConfig(assetID string) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return fmt.Errorf("asset_id is required")
	}

	_, err := r.db.Exec(`DELETE FROM protection_config WHERE asset_id = ?`, assetID)
	if err != nil {
		return fmt.Errorf("failed to delete protection config: %w", err)
	}

	return nil
}

// --- Protection Statistics ---

// SaveProtectionStatistics 保存保护统计
func (r *ProtectionRepository) SaveProtectionStatistics(stats *ProtectionStatistics) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}
	stats.AssetID = strings.TrimSpace(stats.AssetID)
	if stats.AssetID == "" {
		return fmt.Errorf("asset_id is required")
	}

	stats.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	_, err := r.db.Exec(`
		INSERT OR REPLACE INTO protection_statistics 
		(asset_name, asset_id, analysis_count, message_count, warning_count, blocked_count, 
		 total_tokens, total_prompt_tokens, total_completion_tokens, total_tool_calls, 
		 request_count, audit_tokens, audit_prompt_tokens, audit_completion_tokens, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, stats.AssetName, stats.AssetID, stats.AnalysisCount, stats.MessageCount, stats.WarningCount, stats.BlockedCount,
		stats.TotalTokens, stats.TotalPromptTokens, stats.TotalCompletionTokens, stats.TotalToolCalls,
		stats.RequestCount, stats.AuditTokens, stats.AuditPromptTokens, stats.AuditCompletionTokens,
		stats.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save protection statistics: %w", err)
	}

	return nil
}

// GetProtectionStatistics returns the protection statistics for the specified asset instance.
func (r *ProtectionRepository) GetProtectionStatistics(assetID string) (*ProtectionStatistics, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return nil, fmt.Errorf("asset_id is required")
	}

	row := r.db.QueryRow(`SELECT asset_name, asset_id, analysis_count, message_count, warning_count, blocked_count,
		total_tokens, total_prompt_tokens, total_completion_tokens, total_tool_calls,
		request_count, audit_tokens, audit_prompt_tokens, audit_completion_tokens, updated_at
		FROM protection_statistics WHERE asset_id = ?`, assetID)

	var stats ProtectionStatistics
	var auditTokens, auditPromptTokens, auditCompletionTokens sql.NullInt64

	err := row.Scan(&stats.AssetName, &stats.AssetID, &stats.AnalysisCount, &stats.MessageCount,
		&stats.WarningCount, &stats.BlockedCount,
		&stats.TotalTokens, &stats.TotalPromptTokens, &stats.TotalCompletionTokens,
		&stats.TotalToolCalls, &stats.RequestCount,
		&auditTokens, &auditPromptTokens, &auditCompletionTokens, &stats.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get protection statistics: %w", err)
	}

	if auditTokens.Valid {
		stats.AuditTokens = int(auditTokens.Int64)
	}
	if auditPromptTokens.Valid {
		stats.AuditPromptTokens = int(auditPromptTokens.Int64)
	}
	if auditCompletionTokens.Valid {
		stats.AuditCompletionTokens = int(auditCompletionTokens.Int64)
	}

	return &stats, nil
}

// ClearProtectionStatistics clears the protection statistics for the specified asset instance.
func (r *ProtectionRepository) ClearProtectionStatistics(assetID string) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return fmt.Errorf("asset_id is required")
	}

	_, err := r.db.Exec(`DELETE FROM protection_statistics WHERE asset_id = ?`, assetID)
	if err != nil {
		return fmt.Errorf("failed to clear protection statistics: %w", err)
	}

	return nil
}

// --- Shepherd Rules ---

// GetShepherdSensitiveActions 获取指定资产实例的 Shepherd 敏感操作列表。
func (r *ProtectionRepository) GetShepherdSensitiveActions(assetName string, assetID string) ([]string, bool, error) {
	if r.db == nil {
		return nil, false, fmt.Errorf("database not initialized")
	}
	_ = assetName
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return nil, false, fmt.Errorf("asset_id is required")
	}

	row := r.db.QueryRow(`SELECT sensitive_actions FROM shepherd_rules WHERE asset_id = ?`, assetID)

	var raw sql.NullString
	err := row.Scan(&raw)
	if err == sql.ErrNoRows || !raw.Valid || raw.String == "" {
		return []string{}, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to get shepherd rules: %w", err)
	}

	var actions []string
	if err := json.Unmarshal([]byte(raw.String), &actions); err != nil {
		return []string{}, true, nil
	}
	return normalizeShepherdActions(actions), true, nil
}

// SaveShepherdSensitiveActions 保存指定资产实例的Shepherd敏感操作列表
func (r *ProtectionRepository) SaveShepherdSensitiveActions(assetName, assetID string, actions []string) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return fmt.Errorf("asset_id is required")
	}

	jsonStr, err := json.Marshal(normalizeShepherdActions(actions))
	if err != nil {
		return fmt.Errorf("failed to marshal shepherd rules: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = r.db.Exec(`
		INSERT OR REPLACE INTO shepherd_rules (asset_id, asset_name, sensitive_actions, updated_at)
		VALUES (?, ?, ?, ?)
	`, assetID, assetName, string(jsonStr), now)
	if err != nil {
		return fmt.Errorf("failed to save shepherd rules: %w", err)
	}

	return nil
}

func normalizeShepherdActions(actions []string) []string {
	seen := make(map[string]struct{}, len(actions))
	normalized := make([]string, 0, len(actions))
	for _, action := range actions {
		trimmed := strings.TrimSpace(action)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

// --- ClearAllData ---

// ClearAllData 清空所有运行数据（保留配置表）
func (r *ProtectionRepository) ClearAllData() error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	tables := []string{
		"audit_logs", "api_metrics", "protection_statistics",
		"risks", "assets", "scans", "skill_scans",
	}
	for _, table := range tables {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			return fmt.Errorf("failed to clear table %s: %w", table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit clear all data: %w", err)
	}

	logging.Info("All data cleared successfully")
	return nil
}

// --- SaveHomeDirectoryPermission ---

// SaveHomeDirectoryPermission 保存Home目录授权状态
func (r *ProtectionRepository) SaveHomeDirectoryPermission(authorized bool, authorizedPath string) error {
	if r.db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	auth := 0
	if authorized {
		auth = 1
	}

	_, err := r.db.Exec(`
		INSERT OR REPLACE INTO app_permissions (id, home_dir_authorized, authorized_path, updated_at)
		VALUES (1, ?, ?, ?)
	`, auth, authorizedPath, now)
	if err != nil {
		return fmt.Errorf("failed to save home directory permission: %w", err)
	}

	return nil
}

// --- 内部辅助函数 ---

// scanProtectionConfig 从单行查询结果扫描ProtectionConfig
func scanProtectionConfig(row *sql.Row) (*ProtectionConfig, error) {
	var config ProtectionConfig
	var enabled, auditOnly, sandboxEnabled int
	var gatewayBinaryPath, gatewayConfigPath, customSecurityPrompt sql.NullString
	var pathPermission, networkPermission, shellPermission sql.NullString
	var botModelConfigJSON sql.NullString
	var createdAt, updatedAt sql.NullString

	err := row.Scan(&config.AssetName, &config.AssetID, &enabled, &auditOnly, &sandboxEnabled,
		&gatewayBinaryPath, &gatewayConfigPath, &customSecurityPrompt,
		&config.SingleSessionTokenLimit, &config.DailyTokenLimit,
		&pathPermission, &networkPermission, &shellPermission,
		&botModelConfigJSON, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	config.Enabled = enabled == 1
	config.AuditOnly = auditOnly == 1
	config.SandboxEnabled = sandboxEnabled == 1
	config.GatewayBinaryPath = gatewayBinaryPath.String
	config.GatewayConfigPath = gatewayConfigPath.String
	// customSecurityPrompt is deprecated and ignored
	config.PathPermission = pathPermission.String
	config.NetworkPermission = networkPermission.String
	config.ShellPermission = shellPermission.String
	config.CreatedAt = createdAt.String
	config.UpdatedAt = updatedAt.String

	// 反序列化 BotModelConfig JSON
	if botModelConfigJSON.Valid && botModelConfigJSON.String != "" {
		var botConfig BotModelConfigData
		if err := json.Unmarshal([]byte(botModelConfigJSON.String), &botConfig); err == nil {
			config.BotModelConfig = &botConfig
		}
	}

	return &config, nil
}

// scanProtectionConfigFromRows 从多行查询结果扫描ProtectionConfig
func scanProtectionConfigFromRows(rows *sql.Rows) (*ProtectionConfig, error) {
	var config ProtectionConfig
	var enabled, auditOnly, sandboxEnabled int
	var gatewayBinaryPath, gatewayConfigPath, customSecurityPrompt sql.NullString
	var pathPermission, networkPermission, shellPermission sql.NullString
	var botModelConfigJSON sql.NullString
	var createdAt, updatedAt sql.NullString

	err := rows.Scan(&config.AssetName, &config.AssetID, &enabled, &auditOnly, &sandboxEnabled,
		&gatewayBinaryPath, &gatewayConfigPath, &customSecurityPrompt,
		&config.SingleSessionTokenLimit, &config.DailyTokenLimit,
		&pathPermission, &networkPermission, &shellPermission,
		&botModelConfigJSON, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	config.Enabled = enabled == 1
	config.AuditOnly = auditOnly == 1
	config.SandboxEnabled = sandboxEnabled == 1
	config.GatewayBinaryPath = gatewayBinaryPath.String
	config.GatewayConfigPath = gatewayConfigPath.String
	// customSecurityPrompt is deprecated and ignored
	config.PathPermission = pathPermission.String
	config.NetworkPermission = networkPermission.String
	config.ShellPermission = shellPermission.String
	config.CreatedAt = createdAt.String
	config.UpdatedAt = updatedAt.String

	// 反序列化 BotModelConfig JSON
	if botModelConfigJSON.Valid && botModelConfigJSON.String != "" {
		var botConfig BotModelConfigData
		if err := json.Unmarshal([]byte(botModelConfigJSON.String), &botConfig); err == nil {
			config.BotModelConfig = &botConfig
		}
	}

	return &config, nil
}
