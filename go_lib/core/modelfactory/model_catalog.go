package modelfactory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go_lib/chatmodel-routing/adapter"
	"go_lib/core/logging"
)

const (
	modelCatalogCacheTTL        = 5 * time.Minute
	modelCatalogCacheMaxEntries = 256
	modelCatalogResolveTimeout  = 2 * time.Second
)

// catalogRequest 为 GetProviderModelsJSON 的入参（JSON），不包含明文缓存 key。
type catalogRequest struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
}

type catalogResponse struct {
	Success bool            `json:"success"`
	Error   string          `json:"error,omitempty"`
	Data    *catalogPayload `json:"data,omitempty"`
}

type catalogPayload struct {
	Models  []string `json:"models"`
	Source  string   `json:"source"`
	Message string   `json:"message"`
}

type catalogCacheEntry struct {
	models  []string
	source  string
	message string
	expires time.Time
}

var (
	modelCatalogMu    sync.Mutex
	modelCatalogByKey = make(map[string]catalogCacheEntry)
)

// GetProviderModelsJSON 拉取指定 provider 的模型 id 列表。
// 优先调用官方/兼容接口，失败时回退内置推荐；结果带短 TTL 内存缓存。
// 入参为 JSON：provider / base_url / api_key；返回统一包络 JSON 字符串。
func GetProviderModelsJSON(requestJSON string) string {
	req, err := parseCatalogRequest(requestJSON)
	if err != nil {
		return mustJSON(catalogResponse{Success: false, Error: err.Error()})
	}
	p := adapter.NormalizeProviderName(req.Provider)
	if p == "" {
		return mustJSON(catalogResponse{Success: false, Error: "provider is required"})
	}
	base := strings.TrimSpace(req.BaseURL)
	validateCtx, validateCancel := context.WithTimeout(context.Background(), modelCatalogResolveTimeout)
	defer validateCancel()
	if err := validateModelCatalogBaseURL(validateCtx, p, base); err != nil {
		return mustJSON(catalogResponse{Success: false, Error: err.Error()})
	}

	key := catalogCacheKey(string(p), base, req.APIKey)
	if hit, ok := getCatalogCache(key); ok {
		return mustJSON(catalogResponse{
			Success: true,
			Data: &catalogPayload{
				Models:  hit.models,
				Source:  hit.source,
				Message: hit.message,
			},
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	models, source, msg := resolveProviderModels(ctx, p, base, strings.TrimSpace(req.APIKey))
	setCatalogCache(key, models, source, msg)

	return mustJSON(catalogResponse{
		Success: true,
		Data: &catalogPayload{
			Models:  models,
			Source:  source,
			Message: msg,
		},
	})
}

func parseCatalogRequest(raw string) (catalogRequest, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return catalogRequest{}, fmt.Errorf("empty request")
	}
	var req catalogRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		return catalogRequest{}, fmt.Errorf("invalid json: %w", err)
	}
	return req, nil
}

func catalogCacheKey(provider, baseURL, apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	short := hex.EncodeToString(sum[:4]) // 8 hex chars
	normBase := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return fmt.Sprintf("%s|%s|%s", provider, normBase, short)
}

func getCatalogCache(key string) (catalogCacheEntry, bool) {
	modelCatalogMu.Lock()
	defer modelCatalogMu.Unlock()
	e, ok := modelCatalogByKey[key]
	if !ok || time.Now().After(e.expires) {
		if ok {
			delete(modelCatalogByKey, key)
		}
		return catalogCacheEntry{}, false
	}
	return e, true
}

func setCatalogCache(key string, models []string, source, message string) {
	modelCatalogMu.Lock()
	defer modelCatalogMu.Unlock()
	now := time.Now()
	pruneExpiredCatalogCacheLocked(now)
	if _, exists := modelCatalogByKey[key]; !exists && len(modelCatalogByKey) >= modelCatalogCacheMaxEntries {
		evictOldestCatalogCacheLocked()
	}
	modelCatalogByKey[key] = catalogCacheEntry{
		models:  models,
		source:  source,
		message: message,
		expires: now.Add(modelCatalogCacheTTL),
	}
}

func pruneExpiredCatalogCacheLocked(now time.Time) {
	for k, e := range modelCatalogByKey {
		if now.After(e.expires) {
			delete(modelCatalogByKey, k)
		}
	}
}

func evictOldestCatalogCacheLocked() {
	var oldestKey string
	var oldestExpires time.Time
	for k, e := range modelCatalogByKey {
		if oldestKey == "" || e.expires.Before(oldestExpires) {
			oldestKey = k
			oldestExpires = e.expires
		}
	}
	if oldestKey != "" {
		delete(modelCatalogByKey, oldestKey)
	}
}

func validateModelCatalogBaseURL(ctx context.Context, provider adapter.ProviderName, baseURL string) error {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return nil
	}
	allowPrivateHost := provider == adapter.ProviderOpenAICompatible || provider == adapter.ProviderAnthropicCompatible

	u, err := url.Parse(trimmed)
	if err != nil || u == nil {
		return fmt.Errorf("invalid base_url")
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("base_url must use http or https")
	}
	if u.User != nil {
		return fmt.Errorf("base_url must not include userinfo")
	}

	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return fmt.Errorf("base_url host is required")
	}
	routed := adapter.EffectiveRoutingProvider(provider)

	if ip := net.ParseIP(host); ip != nil {
		if allowPrivateHost {
			if isInvalidTargetIP(ip) {
				return fmt.Errorf("base_url host is invalid")
			}
			return nil
		}
		if routed == adapter.ProviderOllama && ip.IsLoopback() {
			return nil
		}
		if isPrivateOrLocalIP(ip) {
			return fmt.Errorf("base_url host must not point to private or local network")
		}
		return nil
	}

	if host == "localhost" {
		if allowPrivateHost {
			return nil
		}
		if routed == adapter.ProviderOllama {
			return nil
		}
		return fmt.Errorf("base_url host must not use localhost for this provider")
	}

	// Reject obvious internal hostnames for non-compatible providers.
	if !allowPrivateHost && (!strings.Contains(host, ".") || strings.HasSuffix(host, ".local")) {
		return fmt.Errorf("base_url host must be a public DNS name")
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("failed to resolve base_url host")
	}
	if len(addrs) == 0 {
		return fmt.Errorf("base_url host resolves to empty address set")
	}
	for _, addr := range addrs {
		if allowPrivateHost {
			if isInvalidTargetIP(addr.IP) {
				return fmt.Errorf("base_url host resolves to invalid address")
			}
			continue
		}
		if isPrivateOrLocalIP(addr.IP) {
			return fmt.Errorf("base_url host resolves to private or local network")
		}
	}
	return nil
}

func isPrivateOrLocalIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		isInvalidTargetIP(ip)
}

func isInvalidTargetIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsMulticast() ||
		ip.IsUnspecified()
}

func resolveProviderModels(ctx context.Context, p adapter.ProviderName, baseURL, apiKey string) (models []string, source string, message string) {
	routed := adapter.EffectiveRoutingProvider(p)

	switch routed {
	case adapter.ProviderOllama:
		if baseURL == "" {
			return staticRecommendedModels(p, routed), "fallback", "ollama base_url is empty, using recommended models"
		}
		got, err := fetchOllamaModelNames(ctx, baseURL)
		if err != nil {
			logging.Warning("[ModelCatalog] Ollama list failed: %v", err)
			fb := staticRecommendedModels(p, routed)
			return fb, "fallback", err.Error()
		}
		if len(got) == 0 {
			return staticRecommendedModels(p, routed), "fallback", "ollama returned empty model list"
		}
		return got, "official", ""

	case adapter.ProviderAnthropic, adapter.ProviderMiniMax:
		if baseURL == "" || apiKey == "" {
			return staticRecommendedModels(p, routed), "fallback", "anthropic-compatible base_url or api_key is empty"
		}
		got, err := fetchAnthropicModelIDs(ctx, baseURL, apiKey)
		if err != nil {
			logging.Warning("[ModelCatalog] Anthropic-compatible models failed: %v", err)
			fb := staticRecommendedModels(p, routed)
			return fb, "fallback", err.Error()
		}
		if len(got) == 0 {
			fb := staticRecommendedModels(p, routed)
			if len(fb) > 0 {
				return fb, "fallback", "official list empty, using recommended models"
			}
		}
		return got, "official", ""

	default:
		if adapter.IsOpenAICompatible(p) && baseURL != "" {
			got, err := fetchOpenAIModelIDs(ctx, baseURL, apiKey)
			if err != nil {
				logging.Warning("[ModelCatalog] OpenAI-compatible models failed: %v", err)
				fb := staticRecommendedModels(p, routed)
				if len(fb) == 0 {
					return nil, "fallback", err.Error()
				}
				return fb, "fallback", err.Error()
			}
			if len(got) == 0 {
				fb := staticRecommendedModels(p, routed)
				if len(fb) > 0 {
					return fb, "fallback", "official list empty, using recommended models"
				}
				return nil, "official", ""
			}
			return got, "official", ""
		}
	}

	static := staticRecommendedModels(p, routed)
	if len(static) > 0 {
		return static, "static", ""
	}
	return nil, "fallback", "no remote list strategy for this provider"
}

func staticRecommendedModels(p, routed adapter.ProviderName) []string {
	switch routed {
	case adapter.ProviderOpenAI:
		return []string{"o3", "o4-mini", "gpt-4.1"}
	case adapter.ProviderAnthropic:
		return []string{"claude-sonnet-4-6", "claude-opus-4-6"}
	case adapter.ProviderMiniMax:
		return []string{
			"MiniMax-M2.7",
			"MiniMax-M2.7-highspeed",
			"MiniMax-M2.5",
			"MiniMax-M2.5-highspeed",
			"MiniMax-M2.1",
			"MiniMax-M2.1-highspeed",
			"MiniMax-M2",
		}
	case adapter.ProviderMoonshot:
		return []string{"moonshot-v1-8k", "moonshot-v1-32k", "moonshot-v1-128k"}
	case adapter.ProviderDeepSeek:
		return []string{"deepseek-chat", "deepseek-reasoner"}
	case adapter.ProviderQwen:
		return []string{"qwen-turbo", "qwen-plus", "qwen-max"}
	case adapter.ProviderZhipu:
		return []string{"glm-4.7", "glm-4-plus", "glm-4-air"}
	case adapter.ProviderGoogle:
		return []string{"gemini-3-flash-preview", "gemini-3-pro-preview"}
	default:
		return modelHintFallback(p)
	}
}

func modelHintFallback(p adapter.ProviderName) []string {
	info := adapter.GetProviderInfo(p)
	if info == nil || strings.TrimSpace(info.ModelHint) == "" {
		return nil
	}
	parts := strings.Split(info.ModelHint, ",")
	models := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := strings.TrimSpace(part)
		if clean != "" && !strings.EqualFold(clean, "etc.") {
			models = append(models, clean)
		}
	}
	return models
}

// normalizeOpenAIBaseForModels 将用户输入的 chat/completions 等完整路径裁剪到适合 GET /v1/models 的根路径。
func normalizeOpenAIBaseForModels(base string) string {
	u := strings.TrimSpace(base)
	u = strings.TrimRight(u, "/")
	if u == "" {
		return ""
	}
	lower := strings.ToLower(u)
	cutSuffixes := []string{
		"/v1/chat/completions",
		"/chat/completions",
		"/v1/completions",
		"/completions",
	}
	for _, suf := range cutSuffixes {
		if strings.HasSuffix(lower, suf) {
			u = u[:len(u)-len(suf)]
			u = strings.TrimRight(u, "/")
			lower = strings.ToLower(u)
			break
		}
	}
	if strings.HasSuffix(lower, "/v1") {
		return u
	}
	if !strings.Contains(u, "/v") && !strings.Contains(u, "/api/paas") && !strings.Contains(u, "/compatible-mode") {
		return u + "/v1"
	}
	return u
}

func fetchOpenAIModelIDs(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	root := normalizeOpenAIBaseForModels(baseURL)
	if root == "" {
		return nil, fmt.Errorf("empty base url after normalization")
	}
	listURL := strings.TrimRight(root, "/") + "/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("models http %d: %s", resp.StatusCode, truncateForLog(body, 200))
	}

	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse models json: %w", err)
	}
	out := make([]string, 0, len(parsed.Data))
	for _, row := range parsed.Data {
		id := strings.TrimSpace(row.ID)
		if id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}

func fetchAnthropicModelIDs(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	root := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	lower := strings.ToLower(root)
	if strings.HasSuffix(lower, "/messages") {
		root = root[:len(root)-len("/messages")]
		root = strings.TrimRight(root, "/")
	}
	if !strings.HasSuffix(strings.ToLower(root), "/v1") {
		root = root + "/v1"
	}
	listURL := root + "/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("models http %d: %s", resp.StatusCode, truncateForLog(body, 200))
	}

	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse anthropic models json: %w", err)
	}
	out := make([]string, 0, len(parsed.Data))
	for _, row := range parsed.Data {
		id := strings.TrimSpace(row.ID)
		if id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}

func fetchOllamaModelNames(ctx context.Context, baseURL string) ([]string, error) {
	root := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	listURL := root + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 再尝试 OpenAI 兼容 /v1/models
		got, err2 := fetchOpenAIModelIDs(ctx, baseURL, "")
		if err2 == nil && len(got) > 0 {
			return got, nil
		}
		return nil, fmt.Errorf("ollama tags http %d: %s", resp.StatusCode, truncateForLog(body, 200))
	}
	var parsed struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse ollama tags: %w", err)
	}
	out := make([]string, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		n := strings.TrimSpace(m.Name)
		if n != "" {
			out = append(out, n)
		}
	}
	return out, nil
}

func truncateForLog(b []byte, max int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func mustJSON(v catalogResponse) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"success":false,"error":"marshal failed"}`
	}
	return string(b)
}
