package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	chatmodelrouting "go_lib/chatmodel-routing"
	"go_lib/chatmodel-routing/adapter"
	"go_lib/chatmodel-routing/providers/anthropic"
	deepseekProvider "go_lib/chatmodel-routing/providers/deepseek"
	"go_lib/chatmodel-routing/providers/google"
	moonshotProvider "go_lib/chatmodel-routing/providers/moonshot"
	ollamaProvider "go_lib/chatmodel-routing/providers/ollama"
	openaiProvider "go_lib/chatmodel-routing/providers/openai"
	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/repository"
	"go_lib/core/shepherd"

	"github.com/openai/openai-go"
)

// LogMessage represents a structured log message for i18n
type LogMessage struct {
	Key    string                 `json:"key"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// ProxyProtection manages the LLM proxy for real-time protection
type ProxyProtection struct {
	proxy           *chatmodelrouting.Proxy
	server          *http.Server
	port            int
	targetURL       string
	originalBaseURL string // Original baseUrl to restore when stopping
	providerName    string
	assetName       string
	assetID         string
	// targetProviderName is the provider used to update openclaw.json after listen.
	targetProviderName string
	shepherdGate       *shepherd.ShepherdGate // ShepherdGate security module
	toolValidator      *ToolValidator         // Tool validation for pre-check

	// Config fields protected by configMu
	configMu                sync.RWMutex
	auditOnly               bool
	singleSessionTokenLimit int
	dailyTokenLimit         int
	initialDailyUsage       int

	// Stream buffer for accumulating chunks until tool call
	streamBuffer *StreamBuffer

	// Context for ShepherdGate (last request messages)
	lastContextMessages    []ConversationMessage
	lastUserMessageContent string // 跨请求持久化的最后一条用户消息，防止上下文压缩丢失

	// Tool call recovery state (when blocked tool_calls need restoration after user confirmation)
	recoveryMu           *sync.Mutex
	pendingRecovery      *pendingToolCallRecovery
	pendingRecoveryArmed bool
	sandboxBlockSeenMu   sync.Mutex
	sandboxBlockSeen     map[string]struct{}
	sandboxBlockOrder    []string

	// Server management
	listener net.Listener
	running  bool
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc

	// Log channel for streaming logs to Flutter
	logChan chan string

	// TruthRecord 存储（SSOT），替代原 RequestViewStore + AuditLogBuffer
	records *RecordStore

	// Analysis statistics
	analysisCount int
	blockedCount  int
	warningCount  int
	statsMu       sync.Mutex

	// API Metrics statistics
	// total* counters include baseline history loaded from DB for UI continuity.
	totalTokens           int
	totalPromptTokens     int
	totalCompletionTokens int
	// baselineTotalTokens is the historical token baseline loaded at proxy start.
	// Quota checks should use runtime usage (totalTokens - baselineTotalTokens),
	// not the cumulative total including historical sessions.
	baselineTotalTokens int
	// Audit (ShepherdGate) Metrics statistics
	auditTokens           int
	auditPromptTokens     int
	auditCompletionTokens int
	totalToolCalls        int
	requestCount          int
	metricsMu             sync.Mutex

	// Current request tracking
	currentRequestID string
	requestStartTime time.Time
	auditMu          sync.Mutex
}

// proxyInstance is the singleton proxy protection instance
var (
	proxyInstance   *ProxyProtection // backward-compatible active proxy pointer
	proxyInstanceMu sync.Mutex
	proxyByAssetKey = make(map[string]*ProxyProtection)
	proxyAssetMeta  = make(map[string]assetRuntimeMeta)
	activeAssetKey  = defaultProxyAssetKey
)

const defaultProxyAssetKey = "__default__"

type assetRuntimeMeta struct {
	AssetName string
	AssetID   string
	BackupDir string
}

// getProxyPort returns the port from MODEL_PROXY_PORT env or finds an available one
func getProxyPort() (int, error) {
	// Check environment variable first
	if portStr := os.Getenv("MODEL_PROXY_PORT"); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return 0, fmt.Errorf("invalid MODEL_PROXY_PORT: %v", err)
		}
		return port, nil
	}

	// Find available port starting from 13436
	return findAvailablePort(13436)
}

// findAvailablePort finds an available port starting from startPort
func findAvailablePort(startPort int) (int, error) {
	for port := startPort; port < startPort+100; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			listener.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port found in range %d-%d", startPort, startPort+100)
}

// createForwardingProvider creates a chatmodel-routing Provider for forwarding requests
// to the bot's LLM. This is independent of the security model used by ShepherdGate.
//
// baseURL is the value stored in the database (the same value shown in the UI form).
// adapter.BuildEndpointURL is called to construct the full API endpoint URL based on
// the provider type (e.g., appending "/chat/completions" for OpenAI-compatible providers).
func createForwardingProvider(providerName adapter.ProviderName, baseURL string, apiKey string) adapter.Provider {
	endpointURL := adapter.BuildEndpointURL(providerName, baseURL)
	logging.Info("[createForwardingProvider] provider=%s, baseURL=%s → endpointURL=%s", providerName, baseURL, endpointURL)

	switch providerName {
	case adapter.ProviderAnthropic, adapter.ProviderMiniMax:
		// MiniMax 使用 Anthropic 兼容协议
		p := anthropic.New(apiKey)
		if endpointURL != "" {
			p.SetBaseURL(endpointURL)
		}
		return p
	case adapter.ProviderGoogle:
		p := google.New(apiKey)
		if endpointURL != "" {
			p.SetBaseURL(endpointURL)
		}
		return p
	case adapter.ProviderOllama, adapter.ProviderLMStudio:
		p := ollamaProvider.New(apiKey)
		if endpointURL != "" {
			p.SetBaseURL(endpointURL)
		}
		return p
	case adapter.ProviderMoonshot:
		p := moonshotProvider.New(apiKey)
		if endpointURL != "" {
			p.SetBaseURL(endpointURL)
		}
		return p
	case adapter.ProviderDeepSeek:
		p := deepseekProvider.New(apiKey)
		if endpointURL != "" {
			p.SetBaseURL(endpointURL)
		}
		return p
	default:
		// OpenAI compatible (covers openai, zhipu, siliconflow, xai, etc.)
		p := openaiProvider.New(apiKey)
		if endpointURL != "" {
			p.SetBaseURL(endpointURL)
		}
		return p
	}
}

func buildReActSkillRuntimeConfig(runtime *ProtectionRuntimeConfig) *shepherd.ReActSkillRuntimeConfig {
	cfg := shepherd.DefaultReActSkillRuntimeConfig()
	if runtime == nil {
		return &cfg
	}
	if runtime.ReActEnableBuiltinSkills != nil {
		cfg.EnableBuiltinSkills = *runtime.ReActEnableBuiltinSkills
	}
	return &cfg
}

func resolveAssetPluginForProxy(assetName string) core.BotPlugin {
	name := strings.TrimSpace(assetName)
	if name == "" {
		return nil
	}
	return core.GetPluginManager().GetPluginByAssetName(name)
}

func resolveForwardingTargetFromPlugin(plugin core.BotPlugin, assetID string) (*core.ProxyForwardingTarget, error) {
	resolver, ok := plugin.(core.ProxyForwardingTargetResolver)
	if !ok {
		return nil, fmt.Errorf("plugin %s does not support forwarding target resolution", plugin.GetAssetName())
	}
	target, err := resolver.ResolveProxyForwardingTarget(strings.TrimSpace(assetID))
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, fmt.Errorf("plugin %s returned nil forwarding target", plugin.GetAssetName())
	}
	return target, nil
}

func isLoopbackHost(host string) bool {
	h := strings.TrimSpace(strings.ToLower(host))
	h = strings.TrimPrefix(h, "[")
	h = strings.TrimSuffix(h, "]")
	return h == "127.0.0.1" || h == "localhost" || h == "::1" || h == "loopback"
}

func isSelfProxyEndpoint(baseURL string, proxyPort int) bool {
	raw := strings.TrimSpace(baseURL)
	if raw == "" || proxyPort <= 0 {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return false
	}
	if !isLoopbackHost(parsed.Hostname()) {
		return false
	}

	port := parsed.Port()
	if port == "" {
		switch strings.ToLower(parsed.Scheme) {
		case "http":
			return proxyPort == 80
		case "https":
			return proxyPort == 443
		default:
			return false
		}
	}

	parsedPort, err := strconv.Atoi(port)
	if err != nil {
		return false
	}
	return parsedPort == proxyPort
}

// NewProxyProtectionFromConfig creates a proxy protection instance from protection config.
//
// The config contains THREE separate configurations:
//   - SecurityModel: used by ShepherdGate for risk analysis
//   - BotModel: optional forwarding target config (required only by plugins that
//     depend on explicit bot model configuration)
//   - Runtime: proxy runtime settings like audit mode and token limits
func NewProxyProtectionFromConfig(protectionConfig *ProtectionConfig, logChan chan string) (*ProxyProtection, error) {
	// 从配置中提取各部分
	securityModel := protectionConfig.SecurityModel
	botModel := protectionConfig.BotModel
	runtime := protectionConfig.Runtime
	assetName := strings.TrimSpace(protectionConfig.AssetName)
	assetID := strings.TrimSpace(protectionConfig.AssetID)

	// ==================== Bot 模型配置（代理转发目标） ====================
	// 代理将 openclaw 发来的请求转发到 Bot 模型的 LLM 服务。
	// Bot 模型与安全模型完全独立，不使用安全模型作为回退。

	plugin := resolveAssetPluginForProxy(assetName)
	requiresBotModelConfig := true
	if plugin != nil {
		requiresBotModelConfig = plugin.RequiresBotModelConfig()
		logging.Info("[ProxyProtection] Asset plugin=%s, requires_bot_model_config=%v", plugin.GetAssetName(), requiresBotModelConfig)
	} else if assetName != "" {
		logging.Warning("[ProxyProtection] Plugin not found for asset=%s, fallback to requires_bot_model_config=true", assetName)
	}

	// 1. Resolve forwarding target from bot model config and/or plugin resolver.
	botCfgProvider := ""
	botCfgBaseURL := ""
	botCfgAPIKey := ""
	if botModel != nil {
		botCfgProvider = botModel.Provider
		botCfgBaseURL = botModel.BaseURL
		botCfgAPIKey = botModel.APIKey
	}

	// Plugins that do not require explicit bot model config must resolve
	// forwarding target from plugin runtime config instead of DB bot_model.
	// This avoids stale bot_model values causing proxy self-looping.
	if !requiresBotModelConfig {
		if plugin == nil {
			return nil, fmt.Errorf("forwarding target is required: plugin not found for asset %s", assetName)
		}
		target, err := resolveForwardingTargetFromPlugin(plugin, assetID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve forwarding target for asset %s: %w", assetName, err)
		}
		if botModel != nil && (strings.TrimSpace(botModel.Provider) != "" || strings.TrimSpace(botModel.BaseURL) != "") {
			logging.Warning("[ProxyProtection] Plugin %s does not require bot_model, ignoring DB bot_model forwarding target", plugin.GetAssetName())
		}
		botCfgProvider = target.Provider
		botCfgBaseURL = target.BaseURL
		if strings.TrimSpace(target.APIKey) != "" {
			botCfgAPIKey = target.APIKey
		}
		logging.Info("[ProxyProtection] Forwarding target resolved from plugin config: provider=%s, base_url=%s",
			botCfgProvider, botCfgBaseURL)
	} else if botModel == nil {
		return nil, fmt.Errorf("bot model config is required: proxy cannot forward without bot model configuration")
	}

	logging.Info("[ProxyProtection] 初始化 - Bot模型: Provider=%s, BaseURL=%s", botCfgProvider, botCfgBaseURL)

	// 2. 确定 Bot 模型的 provider 类型
	botProviderName := adapter.NormalizeProviderName(botCfgProvider)
	if botProviderName == "" {
		if requiresBotModelConfig {
			return nil, fmt.Errorf("bot model provider is required: please configure bot model provider before starting proxy")
		}
		return nil, fmt.Errorf("forwarding provider is required: plugin must provide valid provider or configure bot model provider")
	}

	// 3. 确定 Bot 模型的 API 基础地址
	botBaseURL := strings.TrimSpace(botCfgBaseURL)
	// 环境变量覆盖
	if backendURL := os.Getenv("MODEL_PROXY_BACKEND"); backendURL != "" {
		botBaseURL = backendURL
		logging.Info("[ProxyProtection] Bot BaseURL 被环境变量 MODEL_PROXY_BACKEND 覆盖: %s", botBaseURL)
	}
	if botBaseURL == "" {
		if requiresBotModelConfig {
			return nil, fmt.Errorf("bot model base URL is required: please configure bot model endpoint before starting proxy")
		}
		return nil, fmt.Errorf("forwarding base URL is required: plugin must provide valid endpoint or configure bot model endpoint")
	}

	// 4. 确定 Bot 模型的 API Key
	botAPIKey := strings.TrimSpace(botCfgAPIKey)
	if botAPIKey == "" {
		botAPIKey = os.Getenv("MODEL_PROXY_API_KEY")
	}
	if botAPIKey == "" {
		if info := adapter.GetProviderInfo(botProviderName); info == nil || info.NeedsAPIKey {
			logging.Warning("[ProxyProtection] Bot 模型 provider %s 未配置 API Key，转发请求可能返回 401", botProviderName)
		}
	}

	// 5. Get Proxy Port
	port := 0
	if runtime != nil {
		port = runtime.ProxyPort
	}
	if port == 0 {
		var err error
		port, err = getProxyPort()
		if err != nil {
			return nil, fmt.Errorf("failed to get proxy port: %w", err)
		}
	} else if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid proxy port: %d", port)
	}

	// Prevent self-referential forwarding loops:
	// when upstream base_url points back to this proxy listen address,
	// requests recurse indefinitely and flood logs.
	if isSelfProxyEndpoint(botBaseURL, port) {
		return nil, fmt.Errorf(
			"forwarding base URL points to local proxy itself (%s): restore original model endpoint or reconfigure bot model",
			botBaseURL,
		)
	}

	// 6. Create forwarding Provider using Bot model config
	prov := createForwardingProvider(botProviderName, botBaseURL, botAPIKey)

	// Get actual base URL for logging
	var actualBaseURL string
	if provWithURL, ok := prov.(adapter.ProviderWithBaseURL); ok {
		actualBaseURL = provWithURL.GetBaseURL()
	}
	logging.Info("[ProxyProtection] Bot forwarding target: Provider=%s, BaseURL=%s", botProviderName, actualBaseURL)

	// ==================== Security Model Config (for ShepherdGate) ====================
	// ShepherdGate uses the security model's config to create its own ChatModel for risk analysis.
	if securityModel == nil {
		return nil, fmt.Errorf("security model config is required: ShepherdGate needs security model for risk analysis")
	}
	reactSkillCfg := buildReActSkillRuntimeConfig(runtime)
	shepherdGate, err := shepherd.NewShepherdGateWithRuntime(securityModel, reactSkillCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create shepherd gate: %w", err)
	}
	shepherdGate.SetAssetContext(protectionConfig.AssetName, protectionConfig.AssetID)
	if persistedActions, found, repoErr := repository.NewProtectionRepository(nil).GetShepherdSensitiveActions(protectionConfig.AssetName, protectionConfig.AssetID); repoErr != nil {
		logging.Warning("[ProxyProtection] Failed to load persisted shepherd rules for asset_id=%s: %v", protectionConfig.AssetID, repoErr)
	} else if found {
		shepherdGate.UpdateUserRules(persistedActions)
	}
	logging.Info("[ProxyProtection] Security model: Provider=%s, Model=%s", securityModel.Provider, securityModel.Model)
	if protectionConfig.AssetName != "" {
		repo := repository.NewProtectionRepository(nil)
		userRules, found, err := repo.GetShepherdSensitiveActions(protectionConfig.AssetName, protectionConfig.AssetID)
		if err != nil {
			logging.Warning("[ProxyProtection] Failed to load instance user rules: asset=%s id=%s err=%v",
				protectionConfig.AssetName, protectionConfig.AssetID, err)
		} else if found {
			shepherdGate.UpdateUserRules(userRules)
		}
	}

	// ==================== Runtime Config ====================
	auditOnly := false
	singleSessionTokenLimit := 0
	dailyTokenLimit := 0
	initialDailyUsage := 0
	if runtime != nil {
		auditOnly = runtime.AuditOnly
		singleSessionTokenLimit = runtime.SingleSessionTokenLimit
		dailyTokenLimit = runtime.DailyTokenLimit
		initialDailyUsage = runtime.InitialDailyTokenUsage
	}

	pp := &ProxyProtection{
		port:                    port,
		targetURL:               actualBaseURL,
		originalBaseURL:         botBaseURL,
		providerName:            string(botProviderName),
		assetName:               strings.TrimSpace(protectionConfig.AssetName),
		assetID:                 strings.TrimSpace(protectionConfig.AssetID),
		targetProviderName:      string(botProviderName),
		shepherdGate:            shepherdGate,
		toolValidator:           NewToolValidator(logChan),
		auditOnly:               auditOnly,
		singleSessionTokenLimit: singleSessionTokenLimit,
		dailyTokenLimit:         dailyTokenLimit,
		initialDailyUsage:       initialDailyUsage,
		streamBuffer:            NewStreamBuffer(),
		logChan:                 logChan,
		records:                 NewRecordStore(),
		recoveryMu:              &sync.Mutex{},
	}

	// Initialize statistics with baselines from config
	pp.analysisCount = protectionConfig.BaselineAnalysisCount
	pp.blockedCount = protectionConfig.BaselineBlockedCount
	pp.warningCount = protectionConfig.BaselineWarningCount
	pp.totalTokens = protectionConfig.BaselineTotalTokens
	pp.totalPromptTokens = protectionConfig.BaselineTotalPromptTokens
	pp.totalCompletionTokens = protectionConfig.BaselineTotalCompletionTokens
	pp.baselineTotalTokens = protectionConfig.BaselineTotalTokens
	pp.totalToolCalls = protectionConfig.BaselineTotalToolCalls
	pp.requestCount = protectionConfig.BaselineRequestCount
	pp.auditTokens = protectionConfig.BaselineAuditTokens
	pp.auditPromptTokens = protectionConfig.BaselineAuditPromptTokens
	pp.auditCompletionTokens = protectionConfig.BaselineAuditCompletionTokens

	logging.Info("[ProxyProtection] Token limits: session=%d, daily=%d, initialDailyUsage=%d, auditOnly=%v",
		pp.singleSessionTokenLimit, pp.dailyTokenLimit, pp.initialDailyUsage, pp.auditOnly)

	// Create filter with callbacks
	filter := chatmodelrouting.NewCallbackFilter(
		pp.onRequest,
		pp.onResponse,
		pp.onStreamChunk,
	)

	// Create proxy
	pp.proxy, err = chatmodelrouting.NewProxy(prov, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy: %w", err)
	}

	return pp, nil
}

// UpdateProtectionConfig 更新运行时防护配置（审计模式、token 限额等），不涉及安全模型。
// 安全模型的更新请使用 UpdateSecurityModelConfig。
func (pp *ProxyProtection) UpdateProtectionConfig(runtime *ProtectionRuntimeConfig) {
	pp.configMu.Lock()
	if runtime != nil {
		pp.auditOnly = runtime.AuditOnly
		pp.singleSessionTokenLimit = runtime.SingleSessionTokenLimit
		pp.dailyTokenLimit = runtime.DailyTokenLimit
		pp.initialDailyUsage = runtime.InitialDailyTokenUsage
	}
	pp.configMu.Unlock()

	// 同步更新 ReAct 风险技能配置（内置技能开关）
	if runtime != nil && pp.shepherdGate != nil {
		reactCfg := buildReActSkillRuntimeConfig(runtime)
		if err := pp.shepherdGate.UpdateReActSkillConfig(reactCfg); err != nil {
			logging.Warning("[ProxyProtection] Failed to update ReAct skill runtime config: %v", err)
		} else {
			logging.Info("[ProxyProtection] ReAct skill runtime config applied: enableBuiltin=%v",
				reactCfg.EnableBuiltinSkills)
		}
	}

	pp.sendTerminalLog(fmt.Sprintf("防护配置已更新 - 审计模式: %v, 单会话限额: %d, 每日限额: %d, 已用: %d",
		pp.auditOnly, pp.singleSessionTokenLimit, pp.dailyTokenLimit, pp.initialDailyUsage))
}

// UpdateSecurityModelConfig 热更新安全模型（ShepherdGate）的 chat model 配置。
// 当用户在 UI 中修改安全模型配置后调用，与 UpdateProtectionConfig 完全独立。
func (pp *ProxyProtection) UpdateSecurityModelConfig(config *repository.SecurityModelConfig) error {
	if pp.shepherdGate == nil {
		return fmt.Errorf("ShepherdGate not initialized")
	}
	if err := pp.shepherdGate.UpdateModelConfig(config); err != nil {
		pp.sendTerminalLog(fmt.Sprintf("ShepherdGate 模型更新失败: %v", err))
		logging.Error("Failed to update ShepherdGate model config: %v", err)
		return err
	}
	pp.sendTerminalLog("ShepherdGate 模型配置已更新")
	logging.Info("ShepherdGate model config updated successfully")
	return nil
}

// UpdateShepherdRules updates sensitive action rules for this proxy's ShepherdGate.
func (pp *ProxyProtection) UpdateShepherdRules(sensitiveActions []string) error {
	if pp.shepherdGate == nil {
		return fmt.Errorf("ShepherdGate not initialized")
	}
	pp.shepherdGate.UpdateUserRules(sensitiveActions)
	return nil
}

// GetShepherdRules returns current ShepherdGate user rules for this proxy.
func (pp *ProxyProtection) GetShepherdRules() *shepherd.UserRules {
	if pp.shepherdGate == nil {
		return &shepherd.UserRules{SensitiveActions: []string{}}
	}
	return pp.shepherdGate.GetUserRules()
}

// UpdateUserRules hot-updates instance-level Shepherd rules for the active proxy.
func (pp *ProxyProtection) UpdateUserRules(sensitiveActions []string) {
	if pp == nil || pp.shepherdGate == nil {
		return
	}
	pp.shepherdGate.UpdateUserRules(sensitiveActions)
	pp.sendTerminalLog(fmt.Sprintf("Shepherd user rules updated: %d rule(s)", len(sensitiveActions)))
	logging.Info("[ProxyProtection] Shepherd user rules updated: asset=%s id=%s count=%d",
		pp.assetName, pp.assetID, len(sensitiveActions))
}

// updateBotForwardingProvider hot-swaps the proxy's forwarding provider
// when the bot model config changes (e.g., user switches from OpenAI to Zhipu).
func (pp *ProxyProtection) updateBotForwardingProvider(botConfig *BotModelConfig) {
	botProvider := strings.TrimSpace(botConfig.Provider)
	botBaseURL := strings.TrimSpace(botConfig.BaseURL)
	botAPIKey := strings.TrimSpace(botConfig.APIKey)

	if botProvider == "" && botBaseURL == "" && botAPIKey == "" {
		// No bot config provided, skip
		return
	}

	// Determine provider name
	botProviderName := adapter.NormalizeProviderName(botProvider)
	if botProviderName == "" {
		botProviderName = adapter.ProviderName(pp.providerName)
	}
	if botProviderName == "" {
		botProviderName = adapter.ProviderOpenAI
	}

	// Use existing values as fallback if not provided
	if botBaseURL == "" {
		botBaseURL = pp.originalBaseURL
	}

	// Create new forwarding provider
	newProv := createForwardingProvider(botProviderName, botBaseURL, botAPIKey)

	// Get actual URL for logging
	var newBaseURL string
	if provWithURL, ok := newProv.(adapter.ProviderWithBaseURL); ok {
		newBaseURL = provWithURL.GetBaseURL()
	}

	// Hot-swap the provider
	pp.proxy.UpdateProvider(newProv)
	pp.providerName = string(botProviderName)
	pp.originalBaseURL = botBaseURL

	logging.Info("[ProxyProtection] Bot forwarding provider hot-updated: Provider=%s, BaseURL=%s", botProviderName, newBaseURL)
	pp.sendTerminalLog(fmt.Sprintf("Bot 转发目标已更新: Provider=%s, BaseURL=%s", botProviderName, newBaseURL))
}

// SetAuditOnly updates the audit-only mode at runtime
func (pp *ProxyProtection) SetAuditOnly(auditOnly bool) {
	pp.configMu.Lock()
	oldValue := pp.auditOnly
	pp.auditOnly = auditOnly
	pp.configMu.Unlock()
	if auditOnly {
		pp.sendTerminalLog(fmt.Sprintf("⚙️ SetAuditOnly: 仅审计模式已开启 (old=%v → new=%v)", oldValue, auditOnly))
	} else {
		pp.sendTerminalLog(fmt.Sprintf("⚙️ SetAuditOnly: 仅审计模式已关闭 (old=%v → new=%v)", oldValue, auditOnly))
	}
	logging.Info("[ProxyProtection] SetAuditOnly: %v → %v", oldValue, auditOnly)
}

func (pp *ProxyProtection) sendLog(key string, params map[string]interface{}) {
	pp.sendLogForRequest("", key, params)
}

func (pp *ProxyProtection) sendLogForRequest(requestID, key string, params map[string]interface{}) {
	if params == nil {
		params = map[string]interface{}{}
	}

	reqID := strings.TrimSpace(requestID)
	if reqID == "" {
		pp.auditMu.Lock()
		reqID = pp.currentRequestID
		pp.auditMu.Unlock()
	}

	if reqID != "" {
		if _, exists := params["request_id"]; !exists {
			params["request_id"] = reqID
		}
	}
	if _, exists := params["asset_id"]; !exists && strings.TrimSpace(pp.assetID) != "" {
		params["asset_id"] = strings.TrimSpace(pp.assetID)
	}
	if _, exists := params["asset_name"]; !exists && strings.TrimSpace(pp.assetName) != "" {
		params["asset_name"] = strings.TrimSpace(pp.assetName)
	}

	logMsg := LogMessage{
		Key:    key,
		Params: params,
	}
	jsonBytes, _ := json.Marshal(logMsg)
	msg := string(jsonBytes)

	// Send to log channel for UI display (legacy FFI polling)
	if pp.logChan != nil {
		select {
		case pp.logChan <- msg:
		case <-time.After(10 * time.Millisecond):
			// Channel full, skip after short timeout to avoid blocking too long
		}
	}

	// 发送到 Callback Bridge 用于推送式通信
	sendToCallback(msg)
}

func (pp *ProxyProtection) providerProtocol() string {
	switch adapter.NormalizeProviderName(pp.providerName) {
	case adapter.ProviderAnthropic, adapter.ProviderMiniMax:
		return "anthropic_native"
	case adapter.ProviderGoogle:
		return "gemini_native"
	default:
		return "openai_compatible"
	}
}

func truncateJSONPreview(raw []byte, maxLen int) string {
	if len(raw) == 0 {
		return ""
	}
	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[:maxLen] + "..."
}

func summarizeForwardedRequest(req *openai.ChatCompletionNewParams) string {
	roles := make([]string, 0, len(req.Messages))
	for _, msg := range req.Messages {
		roles = append(roles, getMessageRole(msg))
	}
	return fmt.Sprintf(
		"model=%s messages=%d roles=%s tools=%d",
		string(req.Model),
		len(req.Messages),
		strings.Join(roles, ","),
		len(req.Tools),
	)
}

func (pp *ProxyProtection) emitMonitorRequestCreated(req *openai.ChatCompletionNewParams, rawBody []byte, stream bool) {
	pp.sendLog("monitor_request_created", map[string]interface{}{
		"asset_id":          pp.assetID,
		"start_time":        time.Now().Format(time.RFC3339Nano),
		"provider_name":     pp.providerName,
		"provider_protocol": pp.providerProtocol(),
		"client_model":      string(req.Model),
		"stream":            stream,
		"message_count":     len(req.Messages),
		"messages_raw":      truncateJSONPreview(rawBody, 1200),
	})
}

func (pp *ProxyProtection) emitMonitorClientMessage(index int, role, content string) {
	pp.sendLog("monitor_client_message_received", map[string]interface{}{
		"index":   index,
		"role":    role,
		"content": truncateString(content, 500),
	})
}

func (pp *ProxyProtection) emitMonitorUpstreamRequestBuilt(req *openai.ChatCompletionNewParams, rawBody []byte) {
	pp.sendLog("monitor_upstream_request_built", map[string]interface{}{
		"provider_name":        pp.providerName,
		"provider_protocol":    pp.providerProtocol(),
		"forward_start_time":   time.Now().Format(time.RFC3339Nano),
		"forwarded_raw":        truncateJSONPreview(rawBody, 1200),
		"forwarded_normalized": summarizeForwardedRequest(req),
	})
}

func (pp *ProxyProtection) emitMonitorUpstreamRequestSent() {
	pp.sendLog("monitor_upstream_request_sent", map[string]interface{}{
		"provider_name":     pp.providerName,
		"provider_protocol": pp.providerProtocol(),
	})
}

func (pp *ProxyProtection) emitMonitorSecurityDecision(status, reason string, blocked bool, securityMessage string) {
	pp.sendLog("monitor_security_decision", map[string]interface{}{
		"status":           status,
		"reason":           reason,
		"blocked":          blocked,
		"security_message": truncateString(securityMessage, 1000),
	})
}

func (pp *ProxyProtection) emitMonitorResponseReturned(status, returnedText, returnedRaw string) {
	pp.sendLog("monitor_response_returned", map[string]interface{}{
		"status":                status,
		"returned_to_user_text": truncateString(returnedText, 2000),
		"returned_to_user_raw":  truncateString(returnedRaw, 2000),
	})
}

func (pp *ProxyProtection) emitMonitorRequestFailed(status, errMsg string) {
	pp.sendLog("monitor_request_failed", map[string]interface{}{
		"status": status,
		"error":  truncateString(errMsg, 1000),
	})
}

func (pp *ProxyProtection) previewToolResult(content string) string {
	flat := strings.Join(strings.Fields(content), " ")
	if len(flat) <= 160 {
		return flat
	}
	return flat[:160] + "..."
}

// sendMetricsToCallback 将当前指标发送到 Callback Bridge
func (pp *ProxyProtection) sendMetricsToCallback() {
	pp.statsMu.Lock()
	analysisCount := pp.analysisCount
	blockedCount := pp.blockedCount
	warningCount := pp.warningCount
	pp.statsMu.Unlock()

	pp.metricsMu.Lock()
	metrics := map[string]interface{}{
		"analysis_count":          analysisCount,
		"blocked_count":           blockedCount,
		"warning_count":           warningCount,
		"total_tokens":            pp.totalTokens,
		"total_prompt_tokens":     pp.totalPromptTokens,
		"total_completion_tokens": pp.totalCompletionTokens,
		"audit_tokens":            pp.auditTokens,
		"audit_prompt_tokens":     pp.auditPromptTokens,
		"audit_completion_tokens": pp.auditCompletionTokens,
		"total_tool_calls":        pp.totalToolCalls,
		"request_count":           pp.requestCount,
		"asset_name":              pp.assetName,
		"asset_id":                pp.assetID,
	}
	pp.metricsMu.Unlock()

	sendMetricsToCallback(metrics)
}

// updateTruthRecord 是 handler 中更新请求记录的唯一入口。
// 每次调用都会：1) 更新 RecordStore 2) 推送快照给 CallbackBridge 3) 写入视图日志 4) 写入原始日志流。
func (pp *ProxyProtection) updateTruthRecord(requestID string, update func(r *TruthRecord)) {
	if pp == nil || pp.records == nil {
		return
	}
	snapshot := pp.records.Upsert(requestID, func(r *TruthRecord) {
		if r.AssetName == "" {
			r.AssetName = pp.assetName
		}
		if r.AssetID == "" {
			r.AssetID = pp.assetID
		}
		update(r)
	})
	if snapshot != nil {
		sendTruthRecordToCallback(truthRecordToMap(snapshot))
		pp.sendLog("protection_record_snapshot", truthRecordToMap(snapshot))
	}
}

// GetPendingTruthRecords 返回待推送的 TruthRecord 快照（轮询回退模式使用）。
func (pp *ProxyProtection) GetPendingTruthRecords() []TruthRecord {
	if pp == nil || pp.records == nil {
		return nil
	}
	return pp.records.Pending()
}

func (pp *ProxyProtection) activeRequestID() string {
	if pp == nil {
		return ""
	}
	if pp.streamBuffer != nil {
		if reqID := strings.TrimSpace(pp.streamBuffer.RequestID()); reqID != "" {
			return reqID
		}
	}
	pp.auditMu.Lock()
	defer pp.auditMu.Unlock()
	return strings.TrimSpace(pp.currentRequestID)
}

func (pp *ProxyProtection) sendTerminalLog(message string) {
	pp.auditMu.Lock()
	reqID := pp.currentRequestID
	pp.auditMu.Unlock()

	if reqID != "" {
		logging.Info("[Proxy][%s] %s", reqID, message)
		return
	}

	logging.Info("[Proxy] %s", message)
}

const sandboxBlockedToolResultLimit = 20

func isClawdSecbotSandboxBlockedToolResult(content string) bool {
	if content == "" {
		return false
	}
	return strings.Contains(content, "[ClawdSecbot]") &&
		strings.Contains(content, "ACTION=BLOCK")
}

func (pp *ProxyProtection) markSandboxBlockedToolResultIfFirst(
	toolCallID string,
) bool {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return false
	}

	pp.sandboxBlockSeenMu.Lock()
	defer pp.sandboxBlockSeenMu.Unlock()

	if pp.sandboxBlockSeen == nil {
		pp.sandboxBlockSeen = make(map[string]struct{})
	}
	if _, exists := pp.sandboxBlockSeen[toolCallID]; exists {
		return false
	}

	pp.sandboxBlockSeen[toolCallID] = struct{}{}
	pp.sandboxBlockOrder = append(pp.sandboxBlockOrder, toolCallID)
	for len(pp.sandboxBlockOrder) > sandboxBlockedToolResultLimit {
		evicted := pp.sandboxBlockOrder[0]
		pp.sandboxBlockOrder = pp.sandboxBlockOrder[1:]
		delete(pp.sandboxBlockSeen, evicted)
	}

	return true
}

// finalizeTruthRecord 完成请求记录：设置输出内容、token、生成的工具调用，并标记为 completed。
// 工具调用按 ID 去重，避免流式路径中 onStreamChunk 与 finalize 双重追加。
func (pp *ProxyProtection) finalizeTruthRecord(requestID string, outputContent string, generatedToolCalls []openai.ChatCompletionMessageToolCall, promptTokens, completionTokens int) {
	pp.updateTruthRecord(requestID, func(r *TruthRecord) {
		r.OutputContent = truncateToBytes(outputContent, maxRecordOutputBytes)
		r.PromptTokens = promptTokens
		r.CompletionTokens = completionTokens
		r.CompletedAt = time.Now().Format(time.RFC3339Nano)
		r.Phase = RecordPhaseCompleted

		existingIDs := make(map[string]bool, len(r.ToolCalls))
		for _, existing := range r.ToolCalls {
			if existing.ID != "" {
				existingIDs[existing.ID] = true
			}
		}

		for _, tc := range generatedToolCalls {
			if tc.ID != "" && existingIDs[tc.ID] {
				continue
			}
			isSensitive := false
			if pp.toolValidator != nil {
				isSensitive = pp.toolValidator.IsSensitive(tc.Function.Name)
			}
			r.ToolCalls = append(r.ToolCalls, RecordToolCall{
				ID:          tc.ID,
				Name:        tc.Function.Name,
				Arguments:   truncateToBytes(tc.Function.Arguments, maxRecordToolArgsBytes),
				IsSensitive: isSensitive,
				Source:      "response",
			})
		}
	})
}

// ResetStatistics resets all statistical counters
func (pp *ProxyProtection) ResetStatistics() {
	// Reset analysis statistics
	pp.statsMu.Lock()
	pp.analysisCount = 0
	pp.blockedCount = 0
	pp.warningCount = 0
	pp.statsMu.Unlock()

	// Reset metrics statistics
	pp.metricsMu.Lock()
	pp.totalTokens = 0
	pp.totalPromptTokens = 0
	pp.totalCompletionTokens = 0
	pp.baselineTotalTokens = 0
	pp.auditTokens = 0
	pp.auditPromptTokens = 0
	pp.auditCompletionTokens = 0
	pp.totalToolCalls = 0
	pp.requestCount = 0
	pp.metricsMu.Unlock()

	// Reset stream buffer if exists
	if pp.streamBuffer != nil {
		pp.streamBuffer.ClearAll()
	}

	pp.sendTerminalLog("统计数据已重置")
}

// Start starts the proxy server and configures openclaw to use it
func (pp *ProxyProtection) Start() error {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	if pp.running {
		return fmt.Errorf("proxy already running")
	}

	// Start proxy server first
	var err error
	addr := fmt.Sprintf("127.0.0.1:%d", pp.port)
	pp.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", pp.port, err)
	}

	pp.ctx, pp.cancel = context.WithCancel(context.Background())

	// Create HTTP server with the proxy as handler
	pp.server = &http.Server{
		Handler: pp.proxy,
	}

	go func() {
		if err := pp.server.Serve(pp.listener); err != nil && err != http.ErrServerClosed {
			pp.sendLog("proxy_server_error", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}()

	pp.running = true
	// 中文终端日志: 代理启动
	pp.sendTerminalLog(fmt.Sprintf("防护代理已启动 - 端口: %d, 目标: %s, 目标提供商: %s", pp.port, pp.targetURL, pp.targetProviderName))
	pp.sendLog("proxy_started", map[string]interface{}{
		"port":     pp.port,
		"target":   pp.targetURL,
		"provider": pp.targetProviderName,
	})

	return nil
}

// Stop stops the proxy server and restores original config
func (pp *ProxyProtection) Stop() error {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	if !pp.running {
		return nil
	}

	pp.sendLog("proxy_stopping", nil)

	if pp.cancel != nil {
		pp.cancel()
	}

	if pp.server != nil {
		// Graceful shutdown with timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pp.server.Shutdown(shutdownCtx)
	}

	pp.running = false

	pp.sendLog("proxy_stopped", nil)
	return nil
}

// GetPort returns the proxy port
func (pp *ProxyProtection) GetPort() int {
	return pp.port
}

// GetProxyURL returns the proxy URL for configuring clients
func (pp *ProxyProtection) GetProxyURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", pp.port)
}

// IsRunning returns whether the proxy is running
func (pp *ProxyProtection) IsRunning() bool {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	return pp.running
}

// GetOriginalBaseURL returns the original base URL for state persistence
func (pp *ProxyProtection) GetOriginalBaseURL() string {
	return pp.originalBaseURL
}

// GetBackupDir returns the backup directory path for plugin lifecycle hooks
func (pp *ProxyProtection) GetBackupDir() string {
	pm := core.GetPathManager()
	if pm.IsInitialized() {
		return pm.GetBackupDir()
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".botsec", "backups")
}

// GetProxyProtection 获取全局代理防护实例
func GetProxyProtection() *ProxyProtection {
	proxyInstanceMu.Lock()
	defer proxyInstanceMu.Unlock()
	return proxyInstance
}

// GetProxyProtectionByAsset returns the proxy instance bound to the specified asset instance.
func GetProxyProtectionByAsset(assetID string) *ProxyProtection {
	proxyInstanceMu.Lock()
	defer proxyInstanceMu.Unlock()
	return proxyByAssetKey[buildAssetKey(assetID)]
}

func buildAssetKey(assetID string) string {
	id := strings.TrimSpace(assetID)
	if id == "" {
		return defaultProxyAssetKey
	}
	return id
}

// UpdateLanguage 更新全局语言设置
// 如果 ProxyProtection 实例存在，会更新其 ShepherdGate 的语言
func UpdateLanguage(lang string) {
	proxyInstanceMu.Lock()
	proxies := make([]*ProxyProtection, 0, len(proxyByAssetKey))
	if proxyInstance != nil {
		proxies = append(proxies, proxyInstance)
	}
	for _, p := range proxyByAssetKey {
		if p != nil && p != proxyInstance {
			proxies = append(proxies, p)
		}
	}
	proxyInstanceMu.Unlock()

	for _, proxy := range proxies {
		if proxy != nil && proxy.shepherdGate != nil {
			proxy.shepherdGate.SetLanguage(lang)
		}
	}
}
