package modelfactory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go_lib/chatmodel-routing/adapter"
	"go_lib/core/repository"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino/components/model"
	"google.golang.org/genai"
)

// ValidateSecurityModelConfig validates that the security model config has all required fields.
func ValidateSecurityModelConfig(config *repository.SecurityModelConfig) error {
	switch config.Provider {
	case "ark":
		if config.APIKey == "" {
			return fmt.Errorf("ARK API key is required")
		}
		if config.Endpoint == "" {
			return fmt.Errorf("ARK endpoint ID is required")
		}
	case "openai":
		if config.APIKey == "" {
			return fmt.Errorf("OpenAI API key is required")
		}
		if config.Model == "" {
			return fmt.Errorf("OpenAI model name is required")
		}
	case "ollama":
		if config.Model == "" {
			return fmt.Errorf("Ollama model name is required")
		}
	case "deepseek":
		if config.APIKey == "" {
			return fmt.Errorf("DeepSeek API key is required")
		}
		if config.Model == "" {
			return fmt.Errorf("DeepSeek model name is required")
		}
	case "claude":
		if config.APIKey == "" {
			return fmt.Errorf("Claude API key is required")
		}
		if config.Model == "" {
			return fmt.Errorf("Claude model name is required")
		}
	case "gemini", "google":
		if config.APIKey == "" {
			return fmt.Errorf("Gemini API key is required")
		}
		if config.Model == "" {
			return fmt.Errorf("Gemini model name is required")
		}
	case "qianfan":
		if config.APIKey == "" {
			return fmt.Errorf("Qianfan API key is required")
		}
		if config.Model == "" {
			return fmt.Errorf("Qianfan model name is required")
		}
	case "qwen":
		if config.APIKey == "" {
			return fmt.Errorf("Qwen API key is required")
		}
		if config.Model == "" {
			return fmt.Errorf("Qwen model name is required")
		}
	case "zhipu", "zhipu_en":
		if config.APIKey == "" {
			return fmt.Errorf("Zhipu API key is required")
		}
		if config.Model == "" {
			return fmt.Errorf("Zhipu model name is required")
		}
	case "minimax", "minimax_cn":
		if config.APIKey == "" {
			return fmt.Errorf("MiniMax API key is required")
		}
		if config.Model == "" {
			return fmt.Errorf("MiniMax model name is required")
		}
	case "moonshot", "moonshot_cn":
		if config.APIKey == "" {
			return fmt.Errorf("Moonshot API key is required")
		}
		if config.Model == "" {
			return fmt.Errorf("Moonshot model name is required")
		}
	case "openai_compatible":
		if config.APIKey == "" {
			return fmt.Errorf("OpenAI-compatible API key is required")
		}
		if config.Endpoint == "" {
			return fmt.Errorf("OpenAI-compatible base URL is required")
		}
		if config.Model == "" {
			return fmt.Errorf("OpenAI-compatible model name is required")
		}
	case "anthropic_compatible":
		if config.APIKey == "" {
			return fmt.Errorf("Anthropic-compatible API key is required")
		}
		if config.Endpoint == "" {
			return fmt.Errorf("Anthropic-compatible base URL is required")
		}
		if config.Model == "" {
			return fmt.Errorf("Anthropic-compatible model name is required")
		}
	default:
		if config.APIKey == "" {
			return fmt.Errorf("%s API key is required", config.Provider)
		}
		if config.Model == "" {
			return fmt.Errorf("%s model name is required", config.Provider)
		}
	}
	return nil
}

// CreateChatModelFromConfig creates a ChatModel instance based on the security model config.
func CreateChatModelFromConfig(ctx context.Context, config *repository.SecurityModelConfig) (model.ChatModel, error) {
	switch config.Provider {
	case "ark":
		return createARKModel(ctx, config)
	case "openai":
		return createOpenAIModel(ctx, config)
	case "ollama":
		return createOllamaModel(ctx, config)
	case "deepseek":
		return createDeepSeekModel(ctx, config)
	case "claude":
		return createClaudeModel(ctx, config)
	case "gemini", "google":
		return createGeminiModel(ctx, config)
	case "qianfan":
		return createQianfanModel(ctx, config)
	case "qwen":
		return createQwenModel(ctx, config)
	case "zhipu", "zhipu_en":
		return createZhipuModel(ctx, config)
	case "minimax", "minimax_cn":
		return createMiniMaxModel(ctx, config)
	case "moonshot", "moonshot_cn":
		return createMoonshotModel(ctx, config)
	case "openai_compatible":
		return createOpenAICompatibleModel(ctx, config)
	case "anthropic_compatible":
		return createAnthropicCompatibleModel(ctx, config)
	default:
		return createOpenAICompatibleModel(ctx, config)
	}
}

func createARKModel(ctx context.Context, config *repository.SecurityModelConfig) (model.ChatModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("ARK API key is required")
	}
	if config.Endpoint == "" {
		return nil, fmt.Errorf("ARK endpoint ID is required")
	}
	timeout := 120 * time.Second
	return ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey:  config.APIKey,
		Model:   config.Endpoint,
		Timeout: &timeout,
	})
}

func createOpenAIModel(ctx context.Context, config *repository.SecurityModelConfig) (model.ChatModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("OpenAI model name is required")
	}
	cfg := &openai.ChatModelConfig{
		APIKey:  config.APIKey,
		Model:   config.Model,
		Timeout: 120 * time.Second,
	}
	if config.Endpoint != "" {
		cfg.BaseURL = config.Endpoint
	}
	return openai.NewChatModel(ctx, cfg)
}

func createOllamaModel(ctx context.Context, config *repository.SecurityModelConfig) (model.ChatModel, error) {
	if config.Model == "" {
		return nil, fmt.Errorf("Ollama model name is required")
	}
	baseURL := config.Endpoint
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: baseURL,
		Model:   config.Model,
		Timeout: 120 * time.Second,
	})
}

func createDeepSeekModel(ctx context.Context, config *repository.SecurityModelConfig) (model.ChatModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("DeepSeek API key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("DeepSeek model name is required")
	}
	cfg := &deepseek.ChatModelConfig{
		APIKey:  config.APIKey,
		Model:   config.Model,
		Timeout: 120 * time.Second,
	}
	if config.Endpoint != "" {
		cfg.BaseURL = config.Endpoint
	}
	return deepseek.NewChatModel(ctx, cfg)
}

func createClaudeModel(ctx context.Context, config *repository.SecurityModelConfig) (model.ChatModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("Claude API key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("Claude model name is required")
	}
	cfg := &claude.Config{
		APIKey:    config.APIKey,
		Model:     config.Model,
		MaxTokens: adapter.GetModelMaxOutputTokens(config.Model),
	}
	if ep := strings.TrimSpace(config.Endpoint); ep != "" {
		u := strings.TrimSuffix(ep, "/")
		cfg.BaseURL = &u
	}
	return claude.NewChatModel(ctx, cfg)
}

// createAnthropicCompatibleModel 使用 Claude 组件连接任意 Anthropic Messages 兼容端点。
func createAnthropicCompatibleModel(ctx context.Context, config *repository.SecurityModelConfig) (model.ChatModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("Anthropic-compatible API key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("Anthropic-compatible model name is required")
	}
	if config.Endpoint == "" {
		return nil, fmt.Errorf("Anthropic-compatible base URL is required")
	}
	baseURL := strings.TrimSuffix(strings.TrimSpace(config.Endpoint), "/")
	cfg := &claude.Config{
		APIKey:    config.APIKey,
		Model:     config.Model,
		MaxTokens: adapter.GetModelMaxOutputTokens(config.Model),
		BaseURL:   &baseURL,
	}
	return claude.NewChatModel(ctx, cfg)
}

func createGeminiModel(ctx context.Context, config *repository.SecurityModelConfig) (model.ChatModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("Gemini API key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("Gemini model name is required")
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: config.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}
	return gemini.NewChatModel(ctx, &gemini.Config{
		Client: client,
		Model:  config.Model,
	})
}

// createQianfanModel creates a Qianfan ChatModel instance.
// Qianfan ModelBuilder V2 exposes an OpenAI-compatible protocol:
//   - Default base URL: https://qianfan.baidubce.com/v2
//   - Auth: Authorization: Bearer <api_key> (single API key, no secret key)
//   - Reference: https://cloud.baidu.com/doc/qianfan-api/s/3m7of64lb
func createQianfanModel(ctx context.Context, config *repository.SecurityModelConfig) (model.ChatModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("Qianfan API key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("Qianfan model name is required")
	}
	baseURL := config.Endpoint
	if baseURL == "" {
		baseURL = "https://qianfan.baidubce.com/v2"
	}
	return openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  config.APIKey,
		BaseURL: baseURL,
		Model:   config.Model,
		Timeout: 120 * time.Second,
	})
}

func createQwenModel(ctx context.Context, config *repository.SecurityModelConfig) (model.ChatModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("Qwen API key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("Qwen model name is required")
	}
	baseURL := config.Endpoint
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	return qwen.NewChatModel(ctx, &qwen.ChatModelConfig{
		APIKey:  config.APIKey,
		BaseURL: baseURL,
		Model:   config.Model,
		Timeout: 120 * time.Second,
	})
}

func createZhipuModel(ctx context.Context, config *repository.SecurityModelConfig) (model.ChatModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("Zhipu API key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("Zhipu model name is required")
	}
	baseURL := config.Endpoint
	if baseURL == "" {
		baseURL = adapter.GetDefaultBaseURL(adapter.ProviderName(config.Provider))
		if baseURL == "" {
			baseURL = "https://open.bigmodel.cn/api/paas/v4"
		}
	}
	return openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  config.APIKey,
		BaseURL: baseURL,
		Model:   config.Model,
		Timeout: 120 * time.Second,
	})
}

// createMiniMaxModel creates a MiniMax ChatModel instance.
// MiniMax provides Anthropic-compatible protocol, default endpoint: https://api.minimax.io/anthropic
// Note: the underlying claude component automatically appends /v1/messages, so baseURL should not contain /v1 suffix
func createMiniMaxModel(ctx context.Context, config *repository.SecurityModelConfig) (model.ChatModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("MiniMax API key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("MiniMax model name is required")
	}
	baseURL := strings.TrimSpace(config.Endpoint)
	if baseURL == "" {
		baseURL = adapter.GetDefaultBaseURL(adapter.ProviderName(config.Provider))
		if baseURL == "" {
			baseURL = "https://api.minimax.io/anthropic"
		}
	}
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	baseURL = strings.TrimSuffix(baseURL, "/")
	cfg := &claude.Config{
		APIKey:    config.APIKey,
		Model:     config.Model,
		MaxTokens: adapter.GetModelMaxOutputTokens(config.Model),
		BaseURL:   &baseURL,
	}
	return claude.NewChatModel(ctx, cfg)
}

// createMoonshotModel creates a Moonshot ChatModel instance using deepseek ChatModel.
// Moonshot's API protocol is compatible with DeepSeek, both support reasoning_content field.
func createMoonshotModel(ctx context.Context, config *repository.SecurityModelConfig) (model.ChatModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("Moonshot API key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("Moonshot model name is required")
	}
	baseURL := strings.TrimSpace(config.Endpoint)
	if baseURL == "" {
		baseURL = adapter.GetDefaultBaseURL(adapter.ProviderName(config.Provider))
		if baseURL == "" {
			baseURL = "https://api.moonshot.ai/v1"
		}
	}
	return deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:  config.APIKey,
		BaseURL: baseURL,
		Model:   config.Model,
		Timeout: 120 * time.Second,
	})
}

func createOpenAICompatibleModel(ctx context.Context, config *repository.SecurityModelConfig) (model.ChatModel, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("%s API key is required", config.Provider)
	}
	if config.Model == "" {
		return nil, fmt.Errorf("%s model name is required", config.Provider)
	}
	if strings.TrimSpace(config.Endpoint) == "" {
		return nil, fmt.Errorf("%s base URL is required", config.Provider)
	}
	cfg := &openai.ChatModelConfig{
		APIKey:  config.APIKey,
		Model:   config.Model,
		Timeout: 120 * time.Second,
	}
	if config.Endpoint != "" {
		cfg.BaseURL = config.Endpoint
	}
	return openai.NewChatModel(ctx, cfg)
}
