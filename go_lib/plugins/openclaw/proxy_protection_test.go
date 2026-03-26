package openclaw

import (
	"testing"

	"go_lib/chatmodel-routing/adapter"
	"go_lib/core/repository"
)

func TestOpenclawPlugin_RequiresBotModelConfig(t *testing.T) {
	if !GetOpenclawPlugin().RequiresBotModelConfig() {
		t.Fatal("expected Openclaw plugin to require bot model config")
	}
}

// TestNewProxyProtectionFromConfig_BotConfigRequired 验证 Bot 模型配置为 nil 时返回错误
func TestNewProxyProtectionFromConfig_BotConfigRequired(t *testing.T) {
	config := &ProtectionConfig{
		SecurityModel: &repository.SecurityModelConfig{
			Provider: "openai",
			Endpoint: "https://api.openai.com/v1",
			APIKey:   "sk-test",
			Model:    "gpt-4",
		},
		BotModel: nil, // Bot 配置缺失
		Runtime: &ProtectionRuntimeConfig{
			ProxyPort: 13436,
		},
	}
	logChan := make(chan string, 100)

	_, err := NewProxyProtectionFromConfig(config, logChan)
	if err == nil {
		t.Fatal("Expected error when Bot config is nil, got nil")
	}
	if got := err.Error(); got != "bot model config is required: proxy cannot forward without bot model configuration" {
		t.Errorf("Unexpected error message: %s", got)
	}
}

// TestNewProxyProtectionFromConfig_BotProviderRequired 验证 Bot 模型 provider 为空时返回错误
func TestNewProxyProtectionFromConfig_BotProviderRequired(t *testing.T) {
	config := &ProtectionConfig{
		SecurityModel: &repository.SecurityModelConfig{
			Provider: "openai",
			Endpoint: "https://api.openai.com/v1",
			APIKey:   "sk-test",
			Model:    "gpt-4",
		},
		BotModel: &BotModelConfig{
			Provider: "",
			BaseURL:  "https://api.siliconflow.cn/v1",
			APIKey:   "sk-bot-test",
		},
		Runtime: &ProtectionRuntimeConfig{
			ProxyPort: 13436,
		},
	}
	logChan := make(chan string, 100)

	_, err := NewProxyProtectionFromConfig(config, logChan)
	if err == nil {
		t.Fatal("Expected error when Bot provider is empty, got nil")
	}
	if got := err.Error(); got != "bot model provider is required: please configure bot model provider before starting proxy" {
		t.Errorf("Unexpected error message: %s", got)
	}
}

// TestNewProxyProtectionFromConfig_BotBaseURLRequired 验证 Bot 模型 baseURL 为空时返回错误
func TestNewProxyProtectionFromConfig_BotBaseURLRequired(t *testing.T) {
	config := &ProtectionConfig{
		SecurityModel: &repository.SecurityModelConfig{
			Provider: "openai",
			Endpoint: "https://api.openai.com/v1",
			APIKey:   "sk-test",
			Model:    "gpt-4",
		},
		BotModel: &BotModelConfig{
			Provider: "openai",
			BaseURL:  "",
			APIKey:   "sk-bot-test",
		},
		Runtime: &ProtectionRuntimeConfig{
			ProxyPort: 13436,
		},
	}
	logChan := make(chan string, 100)

	_, err := NewProxyProtectionFromConfig(config, logChan)
	if err == nil {
		t.Fatal("Expected error when Bot baseURL is empty, got nil")
	}
	if got := err.Error(); got != "bot model base URL is required: please configure bot model endpoint before starting proxy" {
		t.Errorf("Unexpected error message: %s", got)
	}
}

// TestNewProxyProtectionFromConfig_NoFallbackToSecurityModel 验证 Bot 模型不会回退到安全模型的配置
func TestNewProxyProtectionFromConfig_NoFallbackToSecurityModel(t *testing.T) {
	// 安全模型有完整配置，Bot 模型配置缺失
	// 新逻辑应该直接报错
	config := &ProtectionConfig{
		SecurityModel: &repository.SecurityModelConfig{
			Provider: "openai",
			Endpoint: "https://api.openai.com/v1",
			APIKey:   "sk-security-key",
			Model:    "gpt-4",
		},
		BotModel: nil,
		Runtime: &ProtectionRuntimeConfig{
			ProxyPort: 13436,
		},
	}
	logChan := make(chan string, 100)

	_, err := NewProxyProtectionFromConfig(config, logChan)
	if err == nil {
		t.Fatal("Should NOT fall back to security model config when Bot is nil")
	}
}

// TestBotModelConfig_ModelField 验证 BotModelConfig 包含 Model 字段
func TestBotModelConfig_ModelField(t *testing.T) {
	bot := &BotModelConfig{
		Provider: "openai",
		BaseURL:  "https://api.openai.com/v1",
		APIKey:   "sk-test",
		Model:    "gpt-4o",
	}

	if bot.Model != "gpt-4o" {
		t.Errorf("Expected Model='gpt-4o', got '%s'", bot.Model)
	}
}

// TestBotModelConfig_IndependentFromSecurityModel 验证 Bot 和安全模型配置完全独立
func TestBotModelConfig_IndependentFromSecurityModel(t *testing.T) {
	config := &ProtectionConfig{
		// 安全模型配置
		SecurityModel: &repository.SecurityModelConfig{
			Provider: "claude",
			Endpoint: "https://api.anthropic.com",
			APIKey:   "sk-security-claude",
			Model:    "claude-3.5-sonnet",
		},
		// Bot 模型配置（完全不同）
		BotModel: &BotModelConfig{
			Provider: "openai",
			BaseURL:  "https://api.openai.com/v1",
			APIKey:   "sk-bot-openai",
			Model:    "gpt-4o",
		},
		Runtime: &ProtectionRuntimeConfig{
			ProxyPort: 13436,
		},
	}

	// 验证安全模型字段
	if config.SecurityModel.Provider != "claude" {
		t.Errorf("Security model type should be claude, got %s", config.SecurityModel.Provider)
	}
	if config.SecurityModel.APIKey != "sk-security-claude" {
		t.Errorf("Security model API key mismatch")
	}

	// 验证 Bot 模型字段
	if config.BotModel.Provider != "openai" {
		t.Errorf("Bot provider should be openai, got %s", config.BotModel.Provider)
	}
	if config.BotModel.APIKey != "sk-bot-openai" {
		t.Errorf("Bot API key should be sk-bot-openai, got %s", config.BotModel.APIKey)
	}
	if config.BotModel.Model != "gpt-4o" {
		t.Errorf("Bot model should be gpt-4o, got %s", config.BotModel.Model)
	}

	// 验证两者之间无交叉引用
	if config.BotModel.Provider == config.SecurityModel.Provider {
		// 这里 openai != claude, 所以不应相等
		t.Error("Bot provider should NOT equal security model type in this test case")
	}
	if config.BotModel.APIKey == config.SecurityModel.APIKey {
		t.Error("Bot API key should NOT equal security model API key")
	}
}

// TestNormalizeProviderName_Empty 验证空 provider 名称归一化后为空
func TestNormalizeProviderName_Empty(t *testing.T) {
	result := adapter.NormalizeProviderName("")
	if result != "" {
		t.Errorf("Expected empty string for empty provider, got '%s'", result)
	}
}
