package adapter

import "strings"

// ProviderName defines the standard provider identifier.
type ProviderName string

// Standard provider names.
const (
	ProviderOpenAI    ProviderName = "openai"
	ProviderAnthropic ProviderName = "anthropic"
	ProviderGoogle    ProviderName = "google"
	ProviderDeepSeek  ProviderName = "deepseek"
	ProviderOllama    ProviderName = "ollama"
	ProviderZhipu     ProviderName = "zhipu"
	ProviderQwen      ProviderName = "qwen"
	ProviderMoonshot  ProviderName = "moonshot"
	ProviderMistral   ProviderName = "mistral"
	ProviderYi        ProviderName = "yi"
	ProviderDoubao    ProviderName = "doubao"
	ProviderErnie     ProviderName = "ernie"
	ProviderHunyuan   ProviderName = "hunyuan"
	ProviderMiniMax   ProviderName = "minimax"
	ProviderXAI       ProviderName = "xai"
	ProviderGroq      ProviderName = "groq"
	ProviderLMStudio  ProviderName = "lmstudio"
	ProviderARK       ProviderName = "ark"
	ProviderQianfan   ProviderName = "qianfan"
	// Bot-only providers (OpenAI compatible)
	ProviderOpenRouter    ProviderName = "openrouter"
	ProviderCopilot       ProviderName = "copilot"
	ProviderVercelGateway ProviderName = "vercel_gateway"
	ProviderOpenCodeZen   ProviderName = "opencode_zen"
	ProviderXiaomi        ProviderName = "xiaomi"
	ProviderSynthetic     ProviderName = "synthetic"
	ProviderVeniceAI      ProviderName = "venice_ai"
	// 兼容协议与区域变体（存储 id 独立，运行时通过 RoutingCanonical 映射到同一协议实现）
	ProviderOpenAICompatible    ProviderName = "openai_compatible"
	ProviderAnthropicCompatible ProviderName = "anthropic_compatible"
	ProviderMiniMaxCN           ProviderName = "minimax_cn"
	ProviderMoonshotCN          ProviderName = "moonshot_cn"
	ProviderZhipuEN             ProviderName = "zhipu_en"
)

// ProviderScope defines the scope where a provider is available.
type ProviderScope string

const (
	ScopeSecurity ProviderScope = "security" // Security model config
	ScopeBot      ProviderScope = "bot"      // Bot model config
	ScopeAll      ProviderScope = "all"      // Both scopes
)

// ProviderInfo contains metadata about a provider for UI display.
type ProviderInfo struct {
	Name           ProviderName  `json:"name"`             // Canonical provider name (used for storage/routing)
	DisplayName    string        `json:"display_name"`     // Human-readable name for UI
	Icon           string        `json:"icon"`             // Icon identifier (lucide icon name)
	Scope          ProviderScope `json:"scope"`            // Where this provider is available
	NeedsEndpoint  bool          `json:"needs_endpoint"`   // Whether endpoint/baseURL field is needed
	NeedsAPIKey    bool          `json:"needs_api_key"`    // Whether API key is required
	NeedsSecretKey bool          `json:"needs_secret_key"` // Whether secret key is needed (e.g., qianfan)
	DefaultBaseURL string        `json:"default_base_url"` // Default base URL (empty = use provider default), also used as endpoint hint in UI
	DefaultModel   string        `json:"default_model"`    // Default model name suggestion
	APIKeyHint     string        `json:"api_key_hint"`     // Hint text for API key field
	ModelHint      string        `json:"model_hint"`       // Hint text for model field
	// AutoV1Suffix 控制当用户提供的 baseURL 中不包含版本路径时，是否自动追加 /v1。
	// 遵循标准 OpenAI 路径格式（{base}/v1/chat/completions）的 provider 设为 true，
	// 使用自定义版本路径的 provider（如智谱 /v4、ARK /v3、Ollama）设为 false。
	AutoV1Suffix bool `json:"auto_v1_suffix"`
	// RoutingCanonical 非空时，转发与安全模型工厂按该 canonical provider 选择协议实现。
	RoutingCanonical ProviderName `json:"routing_canonical,omitempty"`
	// Group 供 UI 分组（如 recommended / compatible / china / global / local）。
	Group string `json:"group,omitempty"`
	// SupportsModelList 为 true 时 UI 可尝试调用 GetProviderModels 拉取模型列表。
	SupportsModelList bool `json:"supports_model_list,omitempty"`
}

// supportedProviders defines all supported providers with their metadata.
//
// ProviderInfo.Group 供 UI 分区，取值：recommended（OpenAI/Anthropic 兼容模板）、
// china（国内常用）、global（国际）、local（本地 Ollama）。
var supportedProviders = []ProviderInfo{
	// ---- Ollama / 本地推理 ----
	// Ollama 的 UI 地址不带 /v1，由 Ollama provider 内部自动拼接
	{
		Name: ProviderOllama, DisplayName: "Ollama", Icon: "server", Scope: ScopeAll,
		NeedsEndpoint: true, NeedsAPIKey: false, AutoV1Suffix: false,
		DefaultBaseURL: "http://localhost:11434", DefaultModel: "llama3",
		ModelHint: "llama3, qwen2, mistral, etc.",
		Group:     "local", SupportsModelList: true,
	},
	{
		Name: ProviderOpenAICompatible, DisplayName: "OpenAI-Compatible", Icon: "plug", Scope: ScopeAll,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: true,
		DefaultBaseURL: "", DefaultModel: "",
		APIKeyHint: "", ModelHint: "",
		Group: "recommended", SupportsModelList: true,
	},
	{
		Name: ProviderAnthropicCompatible, DisplayName: "Anthropic-Compatible", Icon: "plug", Scope: ScopeAll,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: false,
		RoutingCanonical: ProviderAnthropic,
		DefaultBaseURL:   "", DefaultModel: "",
		APIKeyHint: "", ModelHint: "",
		Group: "recommended", SupportsModelList: true,
	},

	// ---- 标准 OpenAI /v1 路径的 provider（AutoV1Suffix: true）----
	{
		Name: ProviderOpenAI, DisplayName: "OpenAI", Icon: "sparkles", Scope: ScopeAll,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: true,
		DefaultBaseURL: "https://api.openai.com/v1", DefaultModel: "o3",
		APIKeyHint: "sk-xxx", ModelHint: "o3, o4-mini, gpt-4.1",
		Group: "global", SupportsModelList: true,
	},
	{
		Name: ProviderDeepSeek, DisplayName: "DeepSeek", Icon: "zap", Scope: ScopeAll,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: true,
		DefaultBaseURL: "https://api.deepseek.com/v1", DefaultModel: "deepseek-reasoner",
		APIKeyHint: "Your DeepSeek API key", ModelHint: "deepseek-reasoner, deepseek-chat",
		Group: "china",
	},
	{
		Name: ProviderQwen, DisplayName: "Qwen", Icon: "bot", Scope: ScopeAll,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: true,
		DefaultBaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", DefaultModel: "qwen-turbo",
		APIKeyHint: "Your Qwen API key", ModelHint: "qwen-turbo, qwen-plus, qwen-max",
		Group: "china",
	},
	{
		Name: ProviderMoonshot, DisplayName: "Moonshot-EN", Icon: "star", Scope: ScopeAll,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: true,
		DefaultBaseURL: "https://api.moonshot.ai/v1", DefaultModel: "moonshot-v1-8k",
		APIKeyHint: "Your Moonshot API key", ModelHint: "moonshot-v1-8k",
		Group: "global",
	},
	{
		Name: ProviderMoonshotCN, DisplayName: "Moonshot-CN", Icon: "star", Scope: ScopeAll,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: true,
		RoutingCanonical: ProviderMoonshot,
		DefaultBaseURL:   "https://api.moonshot.cn/v1", DefaultModel: "moonshot-v1-8k",
		APIKeyHint: "Your Moonshot API key", ModelHint: "moonshot-v1-8k",
		Group: "china",
	},
	{
		Name: ProviderMistral, DisplayName: "Mistral", Icon: "flame", Scope: ScopeSecurity,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: true,
		DefaultBaseURL: "https://api.mistral.ai/v1", DefaultModel: "magistral-medium-latest",
		APIKeyHint: "Your Mistral API key", ModelHint: "magistral-medium-latest, mistral-large-latest",
		Group: "global",
	},
	{
		Name: ProviderYi, DisplayName: "Yi", Icon: "bot", Scope: ScopeSecurity,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: true,
		DefaultBaseURL: "https://api.lingyiwanwu.com/v1", DefaultModel: "yi-lightning",
		APIKeyHint: "Your Yi API key", ModelHint: "yi-lightning, yi-large, yi-medium-200k",
		Group: "global",
	},

	// ---- 自定义版本路径的 provider（AutoV1Suffix: false）----
	// 这些 provider 的 baseURL 已包含特殊版本路径（/v3、/v4 等），不需要自动追加 /v1
	{
		Name: ProviderAnthropic, DisplayName: "Anthropic", Icon: "message-square", Scope: ScopeAll,
		NeedsEndpoint: false, NeedsAPIKey: true, AutoV1Suffix: false,
		DefaultBaseURL: "https://api.anthropic.com/v1", DefaultModel: "claude-sonnet-4-6",
		APIKeyHint: "sk-ant-xxx", ModelHint: "claude-sonnet-4-6, claude-opus-4-6",
		Group: "global", SupportsModelList: true,
	},
	{
		Name: ProviderGoogle, DisplayName: "Google", Icon: "star", Scope: ScopeAll,
		NeedsEndpoint: false, NeedsAPIKey: true, AutoV1Suffix: false,
		DefaultBaseURL: "", DefaultModel: "gemini-3-flash-preview",
		APIKeyHint: "Your Gemini API key", ModelHint: "gemini-3-flash-preview, gemini-3-pro",
		Group: "global",
	},
	{
		Name: ProviderZhipu, DisplayName: "Z.AI-CN", Icon: "sparkles", Scope: ScopeAll,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: false,
		DefaultBaseURL: "https://open.bigmodel.cn/api/paas/v4", DefaultModel: "glm-4.7",
		APIKeyHint: "Your Zhipu API key", ModelHint: "glm-4.7, glm-4-plus, glm-4-air",
		Group: "china",
	},
	{
		Name: ProviderZhipuEN, DisplayName: "Z.AI-EN", Icon: "sparkles", Scope: ScopeAll,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: false,
		RoutingCanonical: ProviderZhipu,
		DefaultBaseURL:   "https://api.z.ai/paas/v4", DefaultModel: "glm-4.7",
		APIKeyHint: "Your Z.AI API key", ModelHint: "glm-4.7, glm-4-plus, glm-4-air",
		Group: "global",
	},
	{
		Name: ProviderARK, DisplayName: "ARK", Icon: "flame", Scope: ScopeSecurity,
		NeedsEndpoint: false, NeedsAPIKey: true, AutoV1Suffix: false,
		DefaultBaseURL: "https://ark.cn-beijing.volces.com/api/v3", DefaultModel: "ep-xxxxx-xxxxx",
		APIKeyHint: "Your ARK API key", ModelHint: "Endpoint ID (ep-xxxxxx)",
		Group: "china",
	},
	{
		Name: ProviderDoubao, DisplayName: "Doubao", Icon: "cloud", Scope: ScopeSecurity,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: false,
		DefaultBaseURL: "https://ark.cn-beijing.volces.com/api/v3", DefaultModel: "doubao-seed-1-6-250615",
		APIKeyHint: "Your Doubao API key", ModelHint: "doubao-seed-1-6-250615",
		Group: "china",
	},
	{
		// 千帆 ModelBuilder V2 OpenAI 兼容接口，使用单一 API Key（Bearer Token），无需 Secret Key
		// 调用路径：POST {baseURL}/chat/completions，Authorization: Bearer <api_key>
		Name: ProviderQianfan, DisplayName: "Qianfan", Icon: "cloud", Scope: ScopeSecurity,
		NeedsEndpoint: true, NeedsAPIKey: true, NeedsSecretKey: false, AutoV1Suffix: false,
		DefaultBaseURL: "https://qianfan.baidubce.com/v2", DefaultModel: "ernie-4.5-turbo-128k",
		APIKeyHint: "Your Qianfan API Key (bce-v3/...)",
		ModelHint:  "ernie-4.5-turbo-128k, ernie-speed-128k, deepseek-v3.1",
		Group:      "china",
	},
	{
		Name: ProviderErnie, DisplayName: "Ernie", Icon: "message-square", Scope: ScopeSecurity,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: false,
		DefaultBaseURL: "", DefaultModel: "ERNIE-Bot-turbo",
		APIKeyHint: "Your Ernie API key", ModelHint: "ERNIE-Bot-turbo",
		Group: "global",
	},
	{
		Name: ProviderHunyuan, DisplayName: "Hunyuan", Icon: "zap", Scope: ScopeSecurity,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: false,
		DefaultBaseURL: "https://api.hunyuan.cloud.tencent.com/v1", DefaultModel: "hunyuan-t1-latest",
		APIKeyHint: "Your Hunyuan API key", ModelHint: "hunyuan-t1-latest, hunyuan-turbos-latest",
		Group: "china",
	},
	{
		Name: ProviderMiniMax, DisplayName: "MiniMax-EN", Icon: "server", Scope: ScopeAll,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: false,
		DefaultBaseURL: "https://api.minimax.io/anthropic/v1", DefaultModel: "MiniMax-M2.5",
		APIKeyHint: "Your MiniMax API key", ModelHint: "MiniMax-M2.1",
		Group: "global", SupportsModelList: false,
	},
	{
		Name: ProviderMiniMaxCN, DisplayName: "MiniMax-CN", Icon: "server", Scope: ScopeAll,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: false,
		RoutingCanonical: ProviderMiniMax,
		DefaultBaseURL:   "https://api.minimaxi.com/anthropic/v1", DefaultModel: "MiniMax-M2.5",
		APIKeyHint: "Your MiniMax API key", ModelHint: "MiniMax-M2.1",
		Group: "china", SupportsModelList: false,
	},

	// ---- Bot-only providers（标准 OpenAI 兼容，AutoV1Suffix: true）----
	{
		Name: ProviderOpenRouter, DisplayName: "OpenRouter", Icon: "sparkles", Scope: ScopeBot,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: true,
		DefaultBaseURL: "https://openrouter.ai/api/v1", DefaultModel: "openai/gpt-4",
		APIKeyHint: "Your OpenRouter API key", ModelHint: "openai/gpt-4",
		Group: "global",
	},
	{
		Name: ProviderCopilot, DisplayName: "Copilot", Icon: "sparkles", Scope: ScopeBot,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: true,
		DefaultBaseURL: "", DefaultModel: "gpt-4",
		APIKeyHint: "Your Copilot API key", ModelHint: "gpt-4",
		Group: "global",
	},
	{
		Name: ProviderVercelGateway, DisplayName: "Vercel AI Gateway", Icon: "sparkles", Scope: ScopeBot,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: true,
		DefaultBaseURL: "", DefaultModel: "gpt-4",
		APIKeyHint: "Your API key", ModelHint: "gpt-4",
		Group: "global",
	},
	{
		Name: ProviderOpenCodeZen, DisplayName: "OpenCode Zen", Icon: "sparkles", Scope: ScopeBot,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: true,
		DefaultBaseURL: "", DefaultModel: "gpt-4",
		APIKeyHint: "Your API key", ModelHint: "gpt-4",
		Group: "global",
	},
	{
		Name: ProviderXiaomi, DisplayName: "Xiaomi", Icon: "sparkles", Scope: ScopeBot,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: true,
		DefaultBaseURL: "https://api.xiaomimimo.com/v1", DefaultModel: "mimo-v2-flash",
		APIKeyHint: "Your Xiaomi API key", ModelHint: "mimo-v2-flash",
		Group: "china",
	},
	{
		Name: ProviderSynthetic, DisplayName: "Synthetic", Icon: "sparkles", Scope: ScopeBot,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: true,
		DefaultBaseURL: "", DefaultModel: "gpt-4",
		APIKeyHint: "Your API key", ModelHint: "gpt-4",
		Group: "global",
	},
	{
		Name: ProviderVeniceAI, DisplayName: "Venice AI", Icon: "sparkles", Scope: ScopeBot,
		NeedsEndpoint: true, NeedsAPIKey: true, AutoV1Suffix: true,
		DefaultBaseURL: "https://api.venice.ai/api/v1", DefaultModel: "llama-3.3-70b",
		APIKeyHint: "Your Venice AI API key", ModelHint: "llama-3.3-70b",
		Group: "global",
	},
}

// providerAliases maps alternative names to standard provider names.
var providerAliases = map[string]ProviderName{
	// Google/Gemini aliases
	"google": ProviderGoogle,
	"gemini": ProviderGoogle,

	// Anthropic/Claude aliases
	"anthropic": ProviderAnthropic,
	"claude":    ProviderAnthropic,

	// OpenAI aliases
	"openai":      ProviderOpenAI,
	"chatgpt":     ProviderOpenAI,
	"gpt":         ProviderOpenAI,
	"azure":       ProviderOpenAI,
	"siliconflow": ProviderOpenAI,

	// Ollama aliases
	"ollama":   ProviderOllama,
	"lmstudio": ProviderLMStudio,

	// Chinese providers
	"zhipu":    ProviderZhipu,
	"zhipu_cn": ProviderZhipu,
	"zhipu_en": ProviderZhipuEN,
	"zai":      ProviderZhipu,
	"zai_cn":   ProviderZhipu,
	"zai_en":   ProviderZhipuEN,
	"glm":      ProviderZhipu,
	"qwen":     ProviderQwen,
	"tongyi":   ProviderQwen,
	"doubao":   ProviderDoubao,
	"ernie":    ProviderErnie,
	"wenxin":   ProviderErnie,
	"hunyuan":  ProviderHunyuan,
	"minimax":  ProviderMiniMax,
	"ark":      ProviderARK,
	"qianfan":  ProviderQianfan,
	"deepseek": ProviderDeepSeek,

	// Other providers
	"moonshot": ProviderMoonshot,
	"kimi":     ProviderMoonshot,
	"mistral":  ProviderMistral,
	"yi":       ProviderYi,
	"xai":      ProviderXAI,
	"grok":     ProviderXAI,
	"groq":     ProviderGroq,

	// Bot-only providers
	"openrouter":     ProviderOpenRouter,
	"copilot":        ProviderCopilot,
	"vercel_gateway": ProviderVercelGateway,
	"opencode_zen":   ProviderOpenCodeZen,
	"xiaomi":         ProviderXiaomi,
	"synthetic":      ProviderSynthetic,
	"venice_ai":      ProviderVeniceAI,
}

// NormalizeProviderName converts any provider name or alias to the standard ProviderName.
// Returns empty string if the provider is not recognized.
func NormalizeProviderName(name string) ProviderName {
	if name == "" {
		return ""
	}

	// Check aliases
	if alias, ok := providerAliases[name]; ok {
		return alias
	}

	// Return as-is for unknown providers (will be handled by caller)
	return ProviderName(name)
}

// GetProviders returns all supported providers for a given scope.
func GetProviders(scope ProviderScope) []ProviderInfo {
	var result []ProviderInfo
	for _, p := range supportedProviders {
		if p.Scope == scope || p.Scope == ScopeAll {
			result = append(result, p)
		}
	}
	return result
}

// GetAllProviders returns all supported providers.
func GetAllProviders() []ProviderInfo {
	return supportedProviders
}

// GetProviderInfo returns the ProviderInfo for a given provider name.
func GetProviderInfo(name ProviderName) *ProviderInfo {
	for i := range supportedProviders {
		if supportedProviders[i].Name == name {
			return &supportedProviders[i]
		}
	}
	return nil
}

// EffectiveRoutingProvider 返回用于协议选择与端点拼接的 canonical provider。
// 区域变体（如 minimax_cn）或 Anthropic 兼容模板映射到同一运行时实现。
func EffectiveRoutingProvider(name ProviderName) ProviderName {
	if name == "" {
		return ""
	}
	if info := GetProviderInfo(name); info != nil && info.RoutingCanonical != "" {
		return info.RoutingCanonical
	}
	return name
}

// IsOpenAICompatible returns true if the provider uses OpenAI-compatible API.
func IsOpenAICompatible(name ProviderName) bool {
	if name == "" {
		return true
	}
	routed := EffectiveRoutingProvider(name)
	switch routed {
	case ProviderAnthropic, ProviderMiniMax, ProviderGoogle:
		return false
	default:
		return true
	}
}

// ProviderConfig holds common configuration for providers.
type ProviderConfig struct {
	Name    ProviderName
	APIKey  string
	BaseURL string // Custom base URL, if empty uses default
}

// GetDefaultBaseURL returns the default base URL for a provider.
func GetDefaultBaseURL(name ProviderName) string {
	info := GetProviderInfo(name)
	if info != nil {
		return info.DefaultBaseURL
	}
	return ""
}

// NeedsV1Suffix 返回该 provider 的 baseURL 是否需要自动追加 /v1 后缀。
// 遵循标准 OpenAI 路径格式（{base}/v1/chat/completions）的返回 true，
// 使用自定义版本路径的（如智谱 /v4、ARK /v3、Ollama）返回 false。
// 对于未注册的 provider，默认返回 true（按 OpenAI 兼容处理）。
func NeedsV1Suffix(name ProviderName) bool {
	info := GetProviderInfo(name)
	if info != nil {
		return info.AutoV1Suffix
	}
	// 未知 provider 默认按 OpenAI 标准路径处理
	return true
}

// modelPrefixToProvider maps model name prefixes to provider names.
var modelPrefixToProvider = map[string]ProviderName{
	"claude":   ProviderAnthropic,
	"gpt":      ProviderOpenAI,
	"gemini":   ProviderGoogle,
	"glm":      ProviderZhipu,
	"deepseek": ProviderDeepSeek,
	"qwen":     ProviderQwen,
	"llama":    ProviderOllama,
	"moonshot": ProviderMoonshot,
	"mistral":  ProviderMistral,
	"yi":       ProviderYi,
	"doubao":   ProviderDoubao,
	"ernie":    ProviderErnie,
	"hunyuan":  ProviderHunyuan,
	"minimax":  ProviderMiniMax,
}

// InferProviderFromModel infers provider from model name prefix.
func InferProviderFromModel(model string) ProviderName {
	if model == "" {
		return ""
	}
	for prefix, provider := range modelPrefixToProvider {
		if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
			return provider
		}
	}
	return ""
}

// ProviderToVendor maps provider name to proxy vendor string for routing.
func ProviderToVendor(name ProviderName) string {
	routed := EffectiveRoutingProvider(name)
	switch routed {
	case ProviderAnthropic, ProviderMiniMax:
		return "anthropic"
	case ProviderGoogle:
		return "gemini"
	case ProviderOllama, ProviderLMStudio:
		return "ollama"
	default:
		return "openai"
	}
}

// ToModelType converts ProviderName to eino ModelType string.
// Special case: anthropic -> claude for eino framework compatibility.
func (p ProviderName) ToModelType() string {
	if p == ProviderAnthropic {
		return "claude"
	}
	if p == ProviderGoogle {
		return "gemini"
	}
	return string(p)
}

// BuildEndpointURL constructs the full API endpoint URL from a user-configured base URL.
//
// The baseURL is the value stored in the database (as shown in the UI form).
// This method appends the appropriate path suffix based on the provider's API protocol:
//
//   - OpenAI-compatible providers: append "/chat/completions"
//   - Anthropic: append "/messages"
//   - Ollama/LMStudio: append "/v1/chat/completions" (or "/chat/completions" if /v1 already present)
//   - Google: return as-is (Google provider constructs URLs internally)
//
// If the URL already ends with the correct suffix, it is returned unchanged.
//
// Examples (OpenAI-compatible):
//
//	"https://api.openai.com/v1"                      → "https://api.openai.com/v1/chat/completions"
//	"https://open.bigmodel.cn/api/paas/v4"           → "https://open.bigmodel.cn/api/paas/v4/chat/completions"
//	"https://api.openai.com/v1/chat/completions"     → unchanged
//
// Examples (Ollama):
//
//	"http://localhost:11434"                          → "http://localhost:11434/v1/chat/completions"
//	"http://localhost:11434/v1"                       → "http://localhost:11434/v1/chat/completions"
//
// Examples (Anthropic):
//
//	"https://api.anthropic.com/v1"                   → "https://api.anthropic.com/v1/messages"
func BuildEndpointURL(providerName ProviderName, baseURL string) string {
	if baseURL == "" {
		return ""
	}
	baseURL = strings.TrimRight(baseURL, "/")
	routed := EffectiveRoutingProvider(providerName)

	switch routed {
	case ProviderAnthropic, ProviderMiniMax:
		// MiniMax 使用 Anthropic 兼容协议，追加 /messages
		if strings.HasSuffix(baseURL, "/messages") {
			return baseURL
		}
		// MiniMax 的 Anthropic 兼容端点需要 /v1 路径
		if routed == ProviderMiniMax && !strings.Contains(baseURL, "/v1") {
			baseURL += "/v1"
		}
		return baseURL + "/messages"

	case ProviderGoogle:
		// Google provider constructs URLs internally
		// (e.g., {baseURL}/models/{model}:generateContent?key={apiKey})
		return baseURL

	case ProviderOllama, ProviderLMStudio:
		if strings.HasSuffix(baseURL, "/chat/completions") {
			return baseURL
		}
		// Ollama default baseURL (e.g., "http://localhost:11434") has no /v1,
		// so prepend /v1 if not already present.
		if !strings.Contains(baseURL, "/v1") {
			baseURL += "/v1"
		}
		return baseURL + "/chat/completions"

	default:
		// OpenAI-compatible providers (openai, zhipu, deepseek, qwen, etc.)
		if strings.HasSuffix(baseURL, "/chat/completions") {
			return baseURL
		}
		return baseURL + "/chat/completions"
	}
}
