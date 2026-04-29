package proxy

import "go_lib/core/repository"

// BotModelConfig is the Bot model configuration for proxy forwarding target.
type BotModelConfig struct {
	Provider  string `json:"provider,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	Model     string `json:"model,omitempty"`
	SecretKey string `json:"secret_key,omitempty"`
}

// ProtectionRuntimeConfig is the proxy runtime configuration.
type ProtectionRuntimeConfig struct {
	ProxyPort                     int    `json:"proxy_port,omitempty"`
	AuditOnly                     bool   `json:"audit_only,omitempty"`
	SingleSessionTokenLimit       int    `json:"single_session_token_limit,omitempty"`
	DailyTokenLimit               int    `json:"daily_token_limit,omitempty"`
	InitialDailyTokenUsage        int    `json:"initial_daily_token_usage,omitempty"`
	CustomSecurityPrompt          string `json:"custom_security_prompt,omitempty"`
	ReActEnableBuiltinSkills      *bool  `json:"react_enable_builtin_skills,omitempty"`
	DetectionBackend              string `json:"detection_backend,omitempty"`
	RemoteDetectionEndpoint       string `json:"remote_detection_endpoint,omitempty"`
	RemoteDetectionAPIKey         string `json:"remote_detection_api_key,omitempty"`
	RemoteDetectionTimeoutSeconds int    `json:"remote_detection_timeout_seconds,omitempty"`
}

// ProtectionConfig is the complete configuration for starting proxy protection.
type ProtectionConfig struct {
	// AssetName 资产名称（如 "openclaw"），用于定位插件并调用其生命周期钩子
	AssetName string `json:"asset_name,omitempty"`
	// AssetID 资产实例ID
	AssetID string `json:"asset_id,omitempty"`

	SecurityModel *repository.SecurityModelConfig `json:"security_model,omitempty"`
	BotModel      *BotModelConfig                 `json:"bot_model,omitempty"`
	Runtime       *ProtectionRuntimeConfig        `json:"runtime,omitempty"`

	BaselineAnalysisCount         int `json:"baseline_analysis_count,omitempty"`
	BaselineBlockedCount          int `json:"baseline_blocked_count,omitempty"`
	BaselineWarningCount          int `json:"baseline_warning_count,omitempty"`
	BaselineTotalTokens           int `json:"baseline_total_tokens,omitempty"`
	BaselineTotalPromptTokens     int `json:"baseline_total_prompt_tokens,omitempty"`
	BaselineTotalCompletionTokens int `json:"baseline_total_completion_tokens,omitempty"`
	BaselineTotalToolCalls        int `json:"baseline_total_tool_calls,omitempty"`
	BaselineRequestCount          int `json:"baseline_request_count,omitempty"`
	BaselineAuditTokens           int `json:"baseline_audit_tokens,omitempty"`
	BaselineAuditPromptTokens     int `json:"baseline_audit_prompt_tokens,omitempty"`
	BaselineAuditCompletionTokens int `json:"baseline_audit_completion_tokens,omitempty"`
}
