package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/repository"
)

// ProxyStartResponse represents the response from starting the proxy
type ProxyStartResponse struct {
	Success         bool   `json:"success"`
	Port            int    `json:"port,omitempty"`
	ProxyURL        string `json:"proxy_url,omitempty"`
	ProviderName    string `json:"provider_name,omitempty"`
	TargetURL       string `json:"target_url,omitempty"`
	OriginalBaseURL string `json:"original_base_url,omitempty"`
	Error           string `json:"error,omitempty"`
}

// ProxyStatusResponse represents the proxy status
type ProxyStatusResponse struct {
	Running         bool   `json:"running"`
	Port            int    `json:"port,omitempty"`
	ProxyURL        string `json:"proxy_url,omitempty"`
	ProviderName    string `json:"provider_name,omitempty"`
	OriginalBaseURL string `json:"original_base_url,omitempty"`
}

// ProxySession manages the proxy with log streaming
type ProxySession struct {
	SessionID string
	Proxy     *ProxyProtection
	LogChan   chan string
	Logs      []string
	LogMu     sync.Mutex
	Done      chan struct{}
	doneOnce  sync.Once
	LogCond   *sync.Cond

	AnalysisCount int
	BlockedCount  int
	WarningCount  int
	StatsMu       sync.Mutex
}

var (
	proxySessionsMu sync.RWMutex
	proxySessions   = make(map[string]*ProxySession)
	proxyCounter    int64
)

func generateProxySessionID() string {
	proxyCounter++
	return fmt.Sprintf("proxy_%d", proxyCounter)
}

func registerProxySession(session *ProxySession) {
	proxySessionsMu.Lock()
	defer proxySessionsMu.Unlock()
	proxySessions[session.SessionID] = session
}

func getProxySession(sessionID string) (*ProxySession, bool) {
	proxySessionsMu.RLock()
	defer proxySessionsMu.RUnlock()
	session, ok := proxySessions[sessionID]
	return session, ok
}

func removeProxySession(sessionID string) {
	proxySessionsMu.Lock()
	defer proxySessionsMu.Unlock()
	delete(proxySessions, sessionID)
}

func closeSessionsForProxy(pp *ProxyProtection) {
	if pp == nil {
		return
	}
	proxySessionsMu.Lock()
	defer proxySessionsMu.Unlock()
	for id, s := range proxySessions {
		if s.Proxy != pp {
			continue
		}
		s.closeDone()
		delete(proxySessions, id)
	}
}

func (s *ProxySession) closeDone() {
	if s == nil {
		return
	}
	s.doneOnce.Do(func() {
		close(s.Done)
	})
}

func getBackupDir() string {
	pm := core.GetPathManager()
	if pm.IsInitialized() {
		dir := pm.GetBackupDir()
		_ = os.MkdirAll(dir, 0755)
		return dir
	}
	homeDir, _ := os.UserHomeDir()
	dir := core.ResolveBackupDir(homeDir)
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func buildProtectionContext(meta assetRuntimeMeta, pp *ProxyProtection, runtime *ProtectionRuntimeConfig) *core.ProtectionContext {
	auditOnly := false
	if runtime != nil {
		auditOnly = runtime.AuditOnly
	}
	return &core.ProtectionContext{
		AssetID:   meta.AssetID,
		ProxyPort: pp.GetPort(),
		BackupDir: meta.BackupDir,
		Config: core.ProtectionConfig{
			ProxyEnabled: true,
			ProxyPort:    pp.GetPort(),
			AuditOnly:    auditOnly,
		},
	}
}

func callPluginStartHook(meta assetRuntimeMeta, pp *ProxyProtection, runtime *ProtectionRuntimeConfig) {
	if meta.AssetName == "" {
		logging.Info("[StartProtectionProxy] No assetName in config, skipping plugin lifecycle hook")
		return
	}

	plugin := core.GetPluginManager().GetPluginByAssetName(meta.AssetName)
	if plugin == nil {
		logging.Info("[StartProtectionProxy] Plugin not found: %s, skipping lifecycle hook", meta.AssetName)
		return
	}

	hooks, ok := plugin.(core.ProtectionLifecycleHooks)
	if !ok {
		logging.Info("[StartProtectionProxy] Plugin %s does not implement ProtectionLifecycleHooks, skipping", meta.AssetName)
		return
	}

	ctx := buildProtectionContext(meta, pp, runtime)
	logging.Info("[StartProtectionProxy] Calling plugin %s.OnProtectionStart hook...", meta.AssetName)
	result, err := hooks.OnProtectionStart(ctx)
	if err != nil {
		logging.Warning("[StartProtectionProxy] Plugin lifecycle hook failed: %v", err)
		return
	}
	if result != nil {
		logging.Info("[StartProtectionProxy] Plugin lifecycle hook result: %v", result)
	}
}

func callPluginBeforeStopHook(meta assetRuntimeMeta, pp *ProxyProtection) {
	if meta.AssetName == "" {
		return
	}

	plugin := core.GetPluginManager().GetPluginByAssetName(meta.AssetName)
	if plugin == nil {
		return
	}

	hooks, ok := plugin.(core.ProtectionLifecycleHooks)
	if !ok {
		return
	}

	ctx := &core.ProtectionContext{
		AssetID:   meta.AssetID,
		ProxyPort: pp.GetPort(),
		BackupDir: meta.BackupDir,
		Config: core.ProtectionConfig{
			ProxyEnabled: false,
			ProxyPort:    pp.GetPort(),
		},
	}

	logging.Info("[StopProtectionProxy] Calling plugin %s.OnBeforeProxyStop hook...", meta.AssetName)
	hooks.OnBeforeProxyStop(ctx)
}

func selectActiveProxyLocked() *ProxyProtection {
	if proxyInstance != nil {
		return proxyInstance
	}

	if p, ok := proxyByAssetKey[activeAssetKey]; ok {
		proxyInstance = p
		return p
	}

	for key, p := range proxyByAssetKey {
		activeAssetKey = key
		proxyInstance = p
		return p
	}
	return nil
}

func stopProxyByAssetKey(assetKey string) error {
	proxyInstanceMu.Lock()
	key := assetKey
	if key == "" || key == defaultProxyAssetKey {
		if activeAssetKey != "" {
			key = activeAssetKey
		}
	}

	pp := proxyByAssetKey[key]
	meta := proxyAssetMeta[key]
	proxyInstanceMu.Unlock()

	if pp == nil {
		return nil
	}

	callPluginBeforeStopHook(meta, pp)
	closeSessionsForProxy(pp)
	if err := pp.Stop(); err != nil {
		return err
	}

	proxyInstanceMu.Lock()
	delete(proxyByAssetKey, key)
	delete(proxyAssetMeta, key)
	if len(proxyByAssetKey) == 0 {
		activeAssetKey = defaultProxyAssetKey
		proxyInstance = nil
	} else if activeAssetKey == key || proxyInstance == pp {
		proxyInstance = nil
		_ = selectActiveProxyLocked()
	}
	proxyInstanceMu.Unlock()

	return nil
}

func stopAllProxies() error {
	proxyInstanceMu.Lock()
	keys := make([]string, 0, len(proxyByAssetKey))
	for key := range proxyByAssetKey {
		keys = append(keys, key)
	}
	proxyInstanceMu.Unlock()

	var firstErr error
	for _, key := range keys {
		if err := stopProxyByAssetKey(key); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func getProxyForOperationLocked(assetID string) *ProxyProtection {
	key := buildAssetKey(assetID)
	if key == defaultProxyAssetKey {
		return selectActiveProxyLocked()
	}

	pp := proxyByAssetKey[key]
	if pp != nil {
		activeAssetKey = key
		proxyInstance = pp
	}
	return pp
}

func (s *ProxySession) collectLogs() {
	for {
		select {
		case log, ok := <-s.LogChan:
			if !ok {
				return
			}
			s.LogMu.Lock()
			s.Logs = append(s.Logs, log)
			s.LogMu.Unlock()
			if s.LogCond != nil {
				s.LogCond.Broadcast()
			}
		case <-s.Done:
			for {
				select {
				case log, ok := <-s.LogChan:
					if !ok {
						return
					}
					s.LogMu.Lock()
					s.Logs = append(s.Logs, log)
					s.LogMu.Unlock()
					if s.LogCond != nil {
						s.LogCond.Broadcast()
					}
				default:
					return
				}
			}
		}
	}
}

func (s *ProxySession) getAndClearLogs() []string {
	s.LogMu.Lock()
	defer s.LogMu.Unlock()
	logs := s.Logs
	s.Logs = nil
	return logs
}

func collectProxyStatistics(pp *ProxyProtection) map[string]interface{} {
	stats := map[string]interface{}{
		"analysis_count":          0,
		"blocked_count":           0,
		"warning_count":           0,
		"total_tokens":            0,
		"total_prompt_tokens":     0,
		"total_completion_tokens": 0,
		"audit_tokens":            0,
		"audit_prompt_tokens":     0,
		"audit_completion_tokens": 0,
		"total_tool_calls":        0,
		"request_count":           0,
	}
	if pp == nil {
		return stats
	}

	pp.statsMu.Lock()
	stats["analysis_count"] = pp.analysisCount
	stats["blocked_count"] = pp.blockedCount
	stats["warning_count"] = pp.warningCount
	pp.statsMu.Unlock()

	pp.metricsMu.Lock()
	stats["total_tokens"] = pp.totalTokens
	stats["total_prompt_tokens"] = pp.totalPromptTokens
	stats["total_completion_tokens"] = pp.totalCompletionTokens
	stats["audit_tokens"] = pp.auditTokens
	stats["audit_prompt_tokens"] = pp.auditPromptTokens
	stats["audit_completion_tokens"] = pp.auditCompletionTokens
	stats["total_tool_calls"] = pp.totalToolCalls
	stats["request_count"] = pp.requestCount
	pp.metricsMu.Unlock()

	return stats
}

// ToJSONString marshals a value to JSON string (exported for use by plugins).
func ToJSONString(v interface{}) string {
	return toJSONString(v)
}

func toJSONString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"success":false,"error":"marshal error"}`
	}
	return string(b)
}

// ==================== 内部包装函数（供 main.go FFI 调用）====================

// StartProtectionProxyInternal 启动代理防护
func StartProtectionProxyInternal(protectionConfigJSON string) string {
	// 对敏感数据脱敏后记录日志
	maskedJSON := protectionConfigJSON
	if len(maskedJSON) > 200 {
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(protectionConfigJSON), &raw); err == nil {
			if key, ok := raw["api_key"].(string); ok && len(key) > 8 {
				raw["api_key"] = key[:4] + "****" + key[len(key)-4:]
			}
			if secret, ok := raw["secret_key"].(string); ok && len(secret) > 8 {
				raw["secret_key"] = secret[:4] + "****" + secret[len(secret)-4:]
			}
			if b, err := json.Marshal(raw); err == nil {
				maskedJSON = string(b)
			}
		}
	}
	logging.Info("[StartProtectionProxy] Received config: %s", maskedJSON)

	var protectionConfig ProtectionConfig
	if err := json.Unmarshal([]byte(protectionConfigJSON), &protectionConfig); err != nil {
		return toJSONString(ProxyStartResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid protection config: %v", err),
		})
	}

	assetKey := buildAssetKey(protectionConfig.AssetID)
	meta := assetRuntimeMeta{
		AssetName: protectionConfig.AssetName,
		AssetID:   protectionConfig.AssetID,
		BackupDir: getBackupDir(),
	}

	proxyInstanceMu.Lock()
	if existing := proxyByAssetKey[assetKey]; existing != nil && existing.IsRunning() {
		activeAssetKey = assetKey
		proxyInstance = existing
		proxyAssetMeta[assetKey] = meta
		closeSessionsForProxy(existing)

		existing.UpdateProtectionConfig(protectionConfig.Runtime)
		logging.Info("[StartProtectionProxy] Updated existing proxy protection config for assetKey=%s", assetKey)

		session := &ProxySession{
			SessionID: generateProxySessionID(),
			Proxy:     existing,
			LogChan:   existing.logChan,
			Done:      make(chan struct{}),
		}
		session.LogCond = sync.NewCond(&session.LogMu)
		go session.collectLogs()
		registerProxySession(session)

		port := existing.GetPort()
		proxyURL := existing.GetProxyURL()
		providerName := existing.providerName
		targetURL := existing.targetURL
		originalBaseURL := existing.originalBaseURL
		proxyInstanceMu.Unlock()

		return toJSONString(map[string]interface{}{
			"success":           true,
			"port":              port,
			"proxy_url":         proxyURL,
			"provider_name":     providerName,
			"target_url":        targetURL,
			"original_base_url": originalBaseURL,
			"session_id":        session.SessionID,
		})
	}
	proxyInstanceMu.Unlock()

	logChan := make(chan string, 10000)

	pp, err := NewProxyProtectionFromConfig(&protectionConfig, logChan)
	if err != nil {
		return toJSONString(ProxyStartResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to create proxy: %v", err),
		})
	}

	session := &ProxySession{
		SessionID: generateProxySessionID(),
		Proxy:     pp,
		LogChan:   logChan,
		Done:      make(chan struct{}),
	}
	session.LogCond = sync.NewCond(&session.LogMu)

	go session.collectLogs()
	registerProxySession(session)

	if err := pp.Start(); err != nil {
		session.closeDone()
		removeProxySession(session.SessionID)
		return toJSONString(ProxyStartResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to start proxy: %v", err),
		})
	}

	callPluginStartHook(meta, pp, protectionConfig.Runtime)

	proxyInstanceMu.Lock()
	activeAssetKey = assetKey
	proxyInstance = pp
	proxyByAssetKey[assetKey] = pp
	proxyAssetMeta[assetKey] = meta
	proxyInstanceMu.Unlock()

	return toJSONString(map[string]interface{}{
		"success":           true,
		"port":              pp.GetPort(),
		"proxy_url":         pp.GetProxyURL(),
		"provider_name":     pp.targetProviderName,
		"target_url":        pp.targetURL,
		"original_base_url": pp.originalBaseURL,
		"session_id":        session.SessionID,
	})
}

// StopProtectionProxyInternal 停止代理防护
func StopProtectionProxyInternal() string {
	if err := stopAllProxies(); err != nil {
		return toJSONString(map[string]interface{}{"success": false, "error": err.Error()})
	}
	return toJSONString(map[string]interface{}{"success": true})
}

// StopProtectionProxyByAssetInternal stops the proxy for the specified asset instance.
func StopProtectionProxyByAssetInternal(assetID string) string {
	if err := stopProxyByAssetKey(buildAssetKey(assetID)); err != nil {
		return toJSONString(map[string]interface{}{"success": false, "error": err.Error()})
	}
	return toJSONString(map[string]interface{}{"success": true})
}

// ResetProtectionStatisticsInternal 重置防护统计
func ResetProtectionStatisticsInternal() string {
	proxyInstanceMu.Lock()
	defer proxyInstanceMu.Unlock()

	pp := selectActiveProxyLocked()
	if pp == nil {
		return toJSONString(map[string]interface{}{"success": false, "error": "proxy not running"})
	}

	pp.ResetStatistics()
	return toJSONString(map[string]interface{}{"success": true})
}

// GetProtectionProxyStatusInternal 获取代理防护状态
func GetProtectionProxyStatusInternal() string {
	proxyInstanceMu.Lock()
	defer proxyInstanceMu.Unlock()

	pp := selectActiveProxyLocked()
	if pp == nil {
		return toJSONString(ProxyStatusResponse{Running: false})
	}

	return toJSONString(ProxyStatusResponse{
		Running:         pp.IsRunning(),
		Port:            pp.GetPort(),
		ProxyURL:        pp.GetProxyURL(),
		ProviderName:    pp.providerName,
		OriginalBaseURL: pp.originalBaseURL,
	})
}

// GetProtectionProxyStatusByAssetInternal returns the proxy status for the specified asset instance.
func GetProtectionProxyStatusByAssetInternal(assetID string) string {
	proxyInstanceMu.Lock()
	defer proxyInstanceMu.Unlock()

	pp := proxyByAssetKey[buildAssetKey(assetID)]
	if pp == nil {
		return toJSONString(ProxyStatusResponse{Running: false})
	}

	return toJSONString(ProxyStatusResponse{
		Running:         pp.IsRunning(),
		Port:            pp.GetPort(),
		ProxyURL:        pp.GetProxyURL(),
		ProviderName:    pp.providerName,
		OriginalBaseURL: pp.originalBaseURL,
	})
}

// UpdateProtectionConfigInternal updates runtime config.
func UpdateProtectionConfigInternal(configJSON string) string {
	return UpdateProtectionConfigByAssetInternal("", configJSON)
}

// UpdateProtectionConfigByAssetInternal updates runtime config for a specific asset instance.
func UpdateProtectionConfigByAssetInternal(assetID, configJSON string) string {
	var cfg ProtectionRuntimeConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return toJSONString(map[string]interface{}{"success": false, "error": fmt.Sprintf("invalid runtime config: %v", err)})
	}

	proxyInstanceMu.Lock()
	defer proxyInstanceMu.Unlock()

	pp := getProxyForOperationLocked(assetID)
	if pp == nil {
		return toJSONString(map[string]interface{}{"success": false, "error": "proxy not running"})
	}

	pp.UpdateProtectionConfig(&cfg)
	return toJSONString(map[string]interface{}{"success": true})
}

// UpdateSecurityModelConfigInternal updates security model config.
func UpdateSecurityModelConfigInternal(configJSON string) string {
	return UpdateSecurityModelConfigByAssetInternal("", configJSON)
}

// UpdateSecurityModelConfigByAssetInternal updates security model config for a specific asset instance.
func UpdateSecurityModelConfigByAssetInternal(assetID, configJSON string) string {
	var cfg repository.SecurityModelConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return toJSONString(map[string]interface{}{"success": false, "error": fmt.Sprintf("invalid security model config: %v", err)})
	}

	proxyInstanceMu.Lock()
	defer proxyInstanceMu.Unlock()

	pp := getProxyForOperationLocked(assetID)
	if pp == nil {
		return toJSONString(map[string]interface{}{"success": false, "error": "proxy not running"})
	}

	if err := pp.UpdateSecurityModelConfig(&cfg); err != nil {
		return toJSONString(map[string]interface{}{"success": false, "error": err.Error()})
	}
	return toJSONString(map[string]interface{}{"success": true})
}

// UpdateBotForwardingProviderInternal 更新 Bot 转发 Provider
func UpdateBotForwardingProviderInternal(botConfigJSON string) string {
	var botCfg BotModelConfig
	if err := json.Unmarshal([]byte(botConfigJSON), &botCfg); err != nil {
		return toJSONString(map[string]interface{}{"success": false, "error": fmt.Sprintf("invalid bot config: %v", err)})
	}

	proxyInstanceMu.Lock()
	defer proxyInstanceMu.Unlock()

	pp := selectActiveProxyLocked()
	if pp == nil {
		return toJSONString(map[string]interface{}{"success": false, "error": "proxy not running"})
	}

	pp.updateBotForwardingProvider(&botCfg)
	return toJSONString(map[string]interface{}{"success": true})
}

// SetProtectionProxyAuditOnlyInternal sets audit-only mode.
func SetProtectionProxyAuditOnlyInternal(auditOnly bool) string {
	return SetProtectionProxyAuditOnlyByAssetInternal("", auditOnly)
}

// SetProtectionProxyAuditOnlyByAssetInternal sets audit-only mode for a specific asset instance.
func SetProtectionProxyAuditOnlyByAssetInternal(assetID string, auditOnly bool) string {
	proxyInstanceMu.Lock()
	defer proxyInstanceMu.Unlock()

	pp := getProxyForOperationLocked(assetID)
	if pp == nil {
		return toJSONString(map[string]interface{}{"success": false, "error": "proxy not running"})
	}

	pp.SetAuditOnly(auditOnly)
	return toJSONString(map[string]interface{}{"success": true})
}

// GetProtectionProxyLogsInternal 获取代理防护日志
func GetProtectionProxyLogsInternal(sessionID string) string {
	session, ok := getProxySession(sessionID)
	if !ok {
		return toJSONString(map[string]interface{}{
			"logs":  []string{},
			"error": "session not found",
		})
	}

	logs := session.getAndClearLogs()
	result := collectProxyStatistics(session.Proxy)
	result["logs"] = logs
	result["request_views"] = session.Proxy.GetPendingTruthRecords()
	return toJSONString(result)
}

// WaitForProtectionLogsInternal 等待防护日志
func WaitForProtectionLogsInternal(sessionID string, timeoutMs int) string {
	session, ok := getProxySession(sessionID)
	if !ok {
		return toJSONString(map[string]interface{}{
			"logs":    []string{},
			"error":   "session not found",
			"timeout": false,
		})
	}

	session.LogMu.Lock()
	if len(session.Logs) > 0 {
		logs := session.Logs
		session.Logs = nil
		session.LogMu.Unlock()

		result := collectProxyStatistics(session.Proxy)
		result["logs"] = logs
		result["request_views"] = session.Proxy.GetPendingTruthRecords()
		result["timeout"] = false
		return toJSONString(result)
	}

	done := make(chan struct{})
	var timedOut bool

	go func() {
		select {
		case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
			timedOut = true
			session.LogCond.Broadcast()
		case <-done:
			return
		case <-session.Done:
			session.LogCond.Broadcast()
			return
		}
	}()

	session.LogCond.Wait()
	close(done)

	logs := session.Logs
	session.Logs = nil
	session.LogMu.Unlock()

	result := collectProxyStatistics(session.Proxy)
	result["logs"] = logs
	result["request_views"] = session.Proxy.GetPendingTruthRecords()
	result["timeout"] = timedOut
	return toJSONString(result)
}

// UpdateShepherdRulesByAssetInternal updates Shepherd rules for a specific asset instance.
func UpdateShepherdRulesByAssetInternal(assetID, rulesJSON string) string {
	var rules UserRules
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		logging.Error("[ShepherdGate] Failed to parse rules JSON: %v", err)
		return "error: parse failed"
	}

	proxyInstanceMu.Lock()
	pp := proxyByAssetKey[buildAssetKey(assetID)]
	proxyInstanceMu.Unlock()
	if pp == nil {
		return "ok"
	}

	if err := pp.UpdateShepherdRules(rules.SensitiveActions); err != nil {
		logging.Error("[ShepherdGate] Failed to update rules by asset (id=%s): %v", assetID, err)
		return "error: update failed"
	}
	return "ok"
}

// GetShepherdRulesByAssetInternal returns current Shepherd user rules JSON for a specific asset instance.
func GetShepherdRulesByAssetInternal(assetID string) string {
	proxyInstanceMu.Lock()
	pp := proxyByAssetKey[buildAssetKey(assetID)]
	proxyInstanceMu.Unlock()
	if pp == nil {
		return `{"SensitiveActions":[]}`
	}
	rules := pp.GetShepherdRules()
	data, err := json.Marshal(rules)
	if err != nil {
		logging.Error("[ShepherdGate] Failed to marshal rules: %v", err)
		return `{"SensitiveActions":[]}`
	}
	return string(data)
}
