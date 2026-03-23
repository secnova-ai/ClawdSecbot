package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
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

	// Server management
	listener net.Listener
	running  bool
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc

	// Log channel for streaming logs to Flutter
	logChan chan string

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

	// Current request audit log tracking
	currentAuditLog  *AuditLog
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

// NewProxyProtectionFromConfig creates a proxy protection instance from protection config.
//
// The config contains THREE separate configurations:
//   - SecurityModel: used by ShepherdGate for risk analysis
//   - BotModel: the LLM that openclaw uses, proxy forwards requests to this target
//   - Runtime: proxy runtime settings like audit mode and token limits
func NewProxyProtectionFromConfig(protectionConfig *ProtectionConfig, logChan chan string) (*ProxyProtection, error) {
	// 从配置中提取各部分
	securityModel := protectionConfig.SecurityModel
	botModel := protectionConfig.BotModel
	runtime := protectionConfig.Runtime

	// ==================== Bot 模型配置（代理转发目标） ====================
	// 代理将 openclaw 发来的请求转发到 Bot 模型的 LLM 服务。
	// Bot 模型与安全模型完全独立，不使用安全模型作为回退。

	// 1. 验证 Bot 模型配置完整性
	if botModel == nil {
		return nil, fmt.Errorf("bot model config is required: proxy cannot forward without bot model configuration")
	}

	botCfgProvider := botModel.Provider
	botCfgBaseURL := botModel.BaseURL
	botCfgAPIKey := botModel.APIKey

	logging.Info("[ProxyProtection] 初始化 - Bot模型: Provider=%s, BaseURL=%s", botCfgProvider, botCfgBaseURL)

	// 2. 确定 Bot 模型的 provider 类型
	botProviderName := adapter.NormalizeProviderName(botCfgProvider)
	if botProviderName == "" {
		return nil, fmt.Errorf("bot model provider is required: please configure bot model provider before starting proxy")
	}

	// 3. 确定 Bot 模型的 API 基础地址
	botBaseURL := strings.TrimSpace(botCfgBaseURL)
	// 环境变量覆盖
	if backendURL := os.Getenv("MODEL_PROXY_BACKEND"); backendURL != "" {
		botBaseURL = backendURL
		logging.Info("[ProxyProtection] Bot BaseURL 被环境变量 MODEL_PROXY_BACKEND 覆盖: %s", botBaseURL)
	}
	if botBaseURL == "" {
		return nil, fmt.Errorf("bot model base URL is required: please configure bot model endpoint before starting proxy")
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
	if persistedActions, found, repoErr := repository.NewProtectionRepository(nil).GetShepherdSensitiveActions(protectionConfig.AssetID); repoErr != nil {
		logging.Warning("[ProxyProtection] Failed to load persisted shepherd rules for asset_id=%s: %v", protectionConfig.AssetID, repoErr)
	} else if found {
		shepherdGate.UpdateUserRules(persistedActions)
	}
	logging.Info("[ProxyProtection] Security model: Provider=%s, Model=%s", securityModel.Provider, securityModel.Model)

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
		targetProviderName:      string(botProviderName),
		shepherdGate:            shepherdGate,
		toolValidator:           NewToolValidator(logChan),
		auditOnly:               auditOnly,
		singleSessionTokenLimit: singleSessionTokenLimit,
		dailyTokenLimit:         dailyTokenLimit,
		initialDailyUsage:       initialDailyUsage,
		streamBuffer:            &StreamBuffer{},
		logChan:                 logChan,
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
	if params == nil {
		params = map[string]interface{}{}
	}

	pp.auditMu.Lock()
	reqID := pp.currentRequestID
	pp.auditMu.Unlock()

	if reqID != "" {
		if _, exists := params["request_id"]; !exists {
			params["request_id"] = reqID
		}
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
	}
	pp.metricsMu.Unlock()

	sendMetricsToCallback(metrics)
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

// saveAuditLog saves the current audit log to the buffer
func (pp *ProxyProtection) saveAuditLog(hasRisk bool, riskLevel, riskReason string, confidence int, action string) {
	pp.auditMu.Lock()
	defer pp.auditMu.Unlock()

	if pp.currentAuditLog == nil {
		return
	}

	// Calculate duration
	duration := time.Since(pp.requestStartTime).Milliseconds()

	pp.currentAuditLog.HasRisk = hasRisk
	pp.currentAuditLog.RiskLevel = riskLevel
	pp.currentAuditLog.RiskReason = riskReason
	pp.currentAuditLog.Confidence = confidence
	pp.currentAuditLog.Action = action
	pp.currentAuditLog.Duration = duration

	// Add to buffer
	auditLogBuffer.AddAuditLog(*pp.currentAuditLog)

	// Clear current audit log
	pp.currentAuditLog = nil
}

// finalizeAuditLog finalizes and saves the audit log for a completed request
func (pp *ProxyProtection) finalizeAuditLog(outputContent string, generatedToolCalls []openai.ChatCompletionMessageToolCall, promptTokens, completionTokens, totalTokens int) {
	pp.auditMu.Lock()
	defer pp.auditMu.Unlock()

	if pp.currentAuditLog == nil {
		return
	}

	// Calculate duration
	duration := time.Since(pp.requestStartTime).Milliseconds()

	pp.currentAuditLog.OutputContent = truncateString(outputContent, 2000)
	pp.currentAuditLog.PromptTokens = promptTokens
	pp.currentAuditLog.CompletionTokens = completionTokens
	pp.currentAuditLog.TotalTokens = totalTokens
	pp.currentAuditLog.Duration = duration

	// Add generated tool calls to audit log
	for _, tc := range generatedToolCalls {
		isSensitive := false
		if pp.toolValidator != nil {
			isSensitive = pp.toolValidator.IsSensitive(tc.Function.Name)
		}
		pp.currentAuditLog.ToolCalls = append(pp.currentAuditLog.ToolCalls, AuditToolCall{
			Name:        tc.Function.Name,
			Arguments:   truncateString(tc.Function.Arguments, 1000),
			IsSensitive: isSensitive,
			Result:      "", // No result yet for generated tool calls
		})
	}

	// Add to buffer
	auditLogBuffer.AddAuditLog(*pp.currentAuditLog)

	// Clear current audit log
	pp.currentAuditLog = nil
}

// updateAuditLogRisk updates the risk information in the current audit log
func (pp *ProxyProtection) updateAuditLogRisk(hasRisk bool, riskLevel, riskReason string, confidence int, action string) {
	pp.auditMu.Lock()
	defer pp.auditMu.Unlock()

	if pp.currentAuditLog == nil {
		return
	}

	pp.currentAuditLog.HasRisk = hasRisk
	pp.currentAuditLog.RiskLevel = riskLevel
	pp.currentAuditLog.RiskReason = riskReason
	pp.currentAuditLog.Confidence = confidence
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
func GetProxyProtectionByAsset(assetName, assetID string) *ProxyProtection {
	proxyInstanceMu.Lock()
	defer proxyInstanceMu.Unlock()
	return proxyByAssetKey[buildAssetKey(assetName, assetID)]
}

func buildAssetKey(assetName, assetID string) string {
	name := strings.ToLower(strings.TrimSpace(assetName))
	id := strings.TrimSpace(assetID)

	if name == "" && id == "" {
		return defaultProxyAssetKey
	}
	return name + "::" + id
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
