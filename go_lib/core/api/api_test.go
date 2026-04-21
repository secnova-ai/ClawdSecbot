package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go_lib/core"
	"go_lib/core/proxy"
	"go_lib/core/repository"
	"go_lib/plugin_sdk"
)

type apiTestPlugin struct {
	assetName string
	pluginID  string
	assets    []core.Asset
}

func (p *apiTestPlugin) GetAssetName() string {
	return p.assetName
}

func (p *apiTestPlugin) GetID() string {
	return p.pluginID
}

func (p *apiTestPlugin) GetManifest() plugin_sdk.PluginManifest {
	return plugin_sdk.PluginManifest{
		PluginID:           p.pluginID,
		BotType:            strings.ToLower(p.assetName),
		DisplayName:        p.assetName,
		APIVersion:         "v1",
		Capabilities:       []string{"scan", "protection_proxy"},
		SupportedPlatforms: []string{"windows"},
	}
}

func (p *apiTestPlugin) GetAssetUISchema() *plugin_sdk.AssetUISchema {
	return &plugin_sdk.AssetUISchema{
		ID:      p.pluginID + ".asset.v1",
		Version: "1",
	}
}

// RequiresBotModelConfig 测试中假定需要 Bot 模型配置，与 core 测试桩行为一致。
func (p *apiTestPlugin) RequiresBotModelConfig() bool {
	return true
}

func (p *apiTestPlugin) ScanAssets() ([]core.Asset, error) {
	return p.assets, nil
}

func (p *apiTestPlugin) AssessRisks(scannedHashes map[string]bool) ([]core.Risk, error) {
	return nil, nil
}

func (p *apiTestPlugin) MitigateRisk(riskInfo string) string {
	return `{"success":true}`
}

func (p *apiTestPlugin) StartProtection(assetID string, config core.ProtectionConfig) error {
	return nil
}

func (p *apiTestPlugin) StopProtection(assetID string) error {
	return nil
}

func (p *apiTestPlugin) GetProtectionStatus(assetID string) core.ProtectionStatus {
	return core.ProtectionStatus{}
}

func registerScannedAPITestAsset(t *testing.T, assetName, botID string) {
	t.Helper()

	plugin := &apiTestPlugin{
		assetName: assetName,
		pluginID:  strings.ToLower(assetName) + "-api-test",
		assets: []core.Asset{
			{
				ID:           botID,
				Name:         assetName,
				SourcePlugin: assetName,
			},
		},
	}

	pm := core.GetPluginManager()
	pm.Register(plugin)
	if _, err := pm.ScanAllAssets(); err != nil {
		t.Fatalf("ScanAllAssets failed: %v", err)
	}
}

// ========== Test Helper Functions ==========

// newAuthRequest creates an HTTP request with the given auth token.
func newAuthRequest(method, path string, token string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, path, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// setupTestServer creates an APIServer with a known token for testing.
// Returns the server, httptest server, and the test token.
func setupTestServer(t *testing.T) (*APIServer, *httptest.Server, string) {
	t.Helper()

	server := NewAPIServer()
	// Set a known token for testing
	testToken := "test-token-12345"
	server.token = testToken

	handler := server.setupRoutes()
	ts := httptest.NewServer(handler)

	return server, ts, testToken
}

// parseAPIResponse parses the response body into an APIResponse struct.
func parseAPIResponse(t *testing.T, body io.Reader) *APIResponse {
	t.Helper()
	var resp APIResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		t.Fatalf("Failed to parse API response: %v", err)
	}
	return &resp
}

// assertStatusCode verifies the HTTP status code matches expected.
func assertStatusCode(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("Status code = %d, want %d", got, want)
	}
}

// assertErrorCode verifies the API error code in the response.
func assertErrorCode(t *testing.T, resp *APIResponse, wantCode int) {
	t.Helper()
	if resp.Code != wantCode {
		t.Errorf("API code = %d, want %d", resp.Code, wantCode)
	}
}

func setupPolicyRuntimeTestDB(t *testing.T) {
	t.Helper()

	// 清理全局 PluginManager 单例残留的注册信息，避免跨用例状态泄漏。
	core.GetPluginManager().ResetForTest()

	dbPath := filepath.Join(t.TempDir(), "policy_runtime_test.db")
	if err := repository.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = repository.CloseDB()
		core.GetPluginManager().ResetForTest()
	})
}

// ========== Authentication Middleware Tests ==========

func TestAuth_NoToken(t *testing.T) {
	_, ts, _ := setupTestServer(t)
	defer ts.Close()

	// Request without Authorization header
	resp, err := http.Get(ts.URL + "/api/v1/status")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	assertStatusCode(t, resp.StatusCode, http.StatusUnauthorized)

	apiResp := parseAPIResponse(t, resp.Body)
	assertErrorCode(t, apiResp, CodeAuthFailed)

	if !strings.Contains(apiResp.Message, "authorization") {
		t.Errorf("Expected message about authorization, got: %s", apiResp.Message)
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	_, ts, _ := setupTestServer(t)
	defer ts.Close()

	req, err := http.NewRequest("GET", ts.URL+"/api/v1/status", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer wrong-token")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	assertStatusCode(t, resp.StatusCode, http.StatusUnauthorized)

	apiResp := parseAPIResponse(t, resp.Body)
	assertErrorCode(t, apiResp, CodeAuthFailed)

	if !strings.Contains(apiResp.Message, "invalid token") {
		t.Errorf("Expected 'invalid token' message, got: %s", apiResp.Message)
	}
}

func TestAuth_ValidToken(t *testing.T) {
	_, ts, token := setupTestServer(t)
	defer ts.Close()

	req, err := http.NewRequest("GET", ts.URL+"/api/v1/status", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// With valid token, should not get 401
	if resp.StatusCode == http.StatusUnauthorized {
		t.Error("Valid token should not return 401")
	}

	// Should get 200 (handler may fail for other reasons but auth should pass)
	assertStatusCode(t, resp.StatusCode, http.StatusOK)
}

func TestAuth_MalformedHeader(t *testing.T) {
	_, ts, token := setupTestServer(t)
	defer ts.Close()

	testCases := []struct {
		name   string
		header string
	}{
		{"No Bearer prefix", token},
		{"Basic instead of Bearer", "Basic " + token},
		{"Empty header", ""},
		{"Bearer without token", "Bearer "},
		{"Bearer with spaces", "Bearer   "},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", ts.URL+"/api/v1/status", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			assertStatusCode(t, resp.StatusCode, http.StatusUnauthorized)

			apiResp := parseAPIResponse(t, resp.Body)
			assertErrorCode(t, apiResp, CodeAuthFailed)
		})
	}
}

// ========== Status Endpoint Tests ==========

func TestGetStatus_Success(t *testing.T) {
	_, ts, token := setupTestServer(t)
	defer ts.Close()

	req, err := http.NewRequest("GET", ts.URL+"/api/v1/status", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	assertStatusCode(t, resp.StatusCode, http.StatusOK)

	// Verify JSON structure
	apiResp := parseAPIResponse(t, resp.Body)
	assertErrorCode(t, apiResp, CodeSuccess)

	// Verify response data structure contains expected fields
	data, ok := apiResp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be a map, got %T", apiResp.Data)
	}

	// Check for required fields in export status
	expectedFields := []string{"enabled", "exportDir", "statusFile", "auditFile", "eventsFile"}
	for _, field := range expectedFields {
		if _, exists := data[field]; !exists {
			t.Errorf("Expected field %q in status response", field)
		}
	}
	legacyFields := []string{"botInfo", "riskInfo", "skillResult", "timestamp"}
	for _, field := range legacyFields {
		if _, exists := data[field]; exists {
			t.Errorf("Did not expect legacy status field %q in status response", field)
		}
	}

	// When export service is not running, enabled should be false
	if enabled, exists := data["enabled"]; !exists {
		t.Error("Expected 'enabled' field in status response")
	} else if enabled != false {
		t.Errorf("Expected enabled=false when service not running, got %v", enabled)
	}
}

func TestGetStatus_WrongMethod(t *testing.T) {
	_, ts, token := setupTestServer(t)
	defer ts.Close()

	// POST to status endpoint should fail (only GET is allowed)
	req, err := http.NewRequest("POST", ts.URL+"/api/v1/status", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should return 405 Method Not Allowed or 404 (depending on mux implementation)
	if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 405 or 404, got %d", resp.StatusCode)
	}
}

func TestAppShutdown_MissingRestoreConfig(t *testing.T) {
	server := NewAPIServer()
	server.token = "test-token"

	req := newAuthRequest("POST", "/api/v1/app/shutdown", "test-token", nil)
	rec := httptest.NewRecorder()
	server.setupRoutes().ServeHTTP(rec, req)

	assertStatusCode(t, rec.Code, http.StatusBadRequest)
	apiResp := parseAPIResponse(t, rec.Body)
	assertErrorCode(t, apiResp, CodeInvalidParam)
}

func TestAppShutdown_ExplicitRestoreConfigTrue(t *testing.T) {
	server := NewAPIServer()
	server.token = "test-token"
	shutdownCh := make(chan AppShutdownOptions, 1)
	server.SetShutdownHandler(func(options AppShutdownOptions) error {
		shutdownCh <- options
		return nil
	})

	req := newAuthRequest(
		"POST",
		"/api/v1/app/shutdown",
		"test-token",
		bytes.NewBufferString(`{"restoreConfig":true}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.setupRoutes().ServeHTTP(rec, req)

	assertStatusCode(t, rec.Code, http.StatusOK)

	select {
	case options := <-shutdownCh:
		if !options.RestoreConfig {
			t.Fatal("restoreConfig should be true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected shutdown handler to be called")
	}
}

func TestAppShutdown_ExplicitRestoreConfigFalse(t *testing.T) {
	server := NewAPIServer()
	server.token = "test-token"
	shutdownCh := make(chan AppShutdownOptions, 1)
	server.SetShutdownHandler(func(options AppShutdownOptions) error {
		shutdownCh <- options
		return nil
	})

	req := newAuthRequest(
		"POST",
		"/api/v1/app/shutdown",
		"test-token",
		bytes.NewBufferString(`{"restoreConfig":false}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.setupRoutes().ServeHTTP(rec, req)

	assertStatusCode(t, rec.Code, http.StatusOK)

	select {
	case options := <-shutdownCh:
		if options.RestoreConfig {
			t.Fatal("restoreConfig should be false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected shutdown handler to be called")
	}
}

func TestAppShutdown_DisablesAllAssetProtectionInStatusFile(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	repo := repository.NewProtectionRepository(nil)
	if err := repo.SaveProtectionConfig(&repository.ProtectionConfig{
		AssetName: "openclaw",
		AssetID:   "shutdown-bot-1",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("SaveProtectionConfig bot1 failed: %v", err)
	}
	if err := repo.SaveProtectionConfig(&repository.ProtectionConfig{
		AssetName: "nullclaw",
		AssetID:   "shutdown-bot-2",
		Enabled:   true,
		AuditOnly: true,
	}); err != nil {
		t.Fatalf("SaveProtectionConfig bot2 failed: %v", err)
	}

	scanRepo := repository.NewScanRepository(nil)
	if err := scanRepo.SaveScanResult(&repository.ScanRecord{
		ConfigFound: true,
		Assets: []core.Asset{
			{ID: "shutdown-bot-1", Name: "OpenClaw", SourcePlugin: "openclaw"},
			{ID: "shutdown-bot-2", Name: "NullClaw", SourcePlugin: "nullclaw"},
		},
		Risks: []core.Risk{},
	}); err != nil {
		t.Fatalf("SaveScanResult failed: %v", err)
	}

	originalStop := stopProtectionProxyForPolicy
	stopProtectionProxyForPolicy = func(assetID string) string {
		return `{"success":true}`
	}
	t.Cleanup(func() {
		stopProtectionProxyForPolicy = originalStop
	})

	statusFile := filepath.Join(t.TempDir(), "status.json")
	server := NewAPIServer()
	server.token = "test-token"
	server.exportService = &ExportServiceImpl{
		running:    true,
		statusFile: statusFile,
	}
	shutdownCh := make(chan AppShutdownOptions, 1)
	server.SetShutdownHandler(func(options AppShutdownOptions) error {
		shutdownCh <- options
		return nil
	})

	req := newAuthRequest(
		"POST",
		"/api/v1/app/shutdown",
		"test-token",
		bytes.NewBufferString(`{"restoreConfig":false}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.setupRoutes().ServeHTTP(rec, req)

	assertStatusCode(t, rec.Code, http.StatusOK)

	updatedConfigs, err := repo.GetAllProtectionConfigs()
	if err != nil {
		t.Fatalf("GetAllProtectionConfigs failed: %v", err)
	}
	if len(updatedConfigs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(updatedConfigs))
	}
	for _, config := range updatedConfigs {
		if config == nil {
			t.Fatal("config should not be nil")
		}
		if config.Enabled {
			t.Fatalf("expected config %s to be disabled after shutdown", config.AssetID)
		}
	}

	raw, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatalf("expected status file to be written: %v", err)
	}

	var status struct {
		BotInfo []struct {
			ID         string `json:"id"`
			Protection string `json:"protection"`
		} `json:"botInfo"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		t.Fatalf("failed to parse status file: %v", err)
	}
	if len(status.BotInfo) != 2 {
		t.Fatalf("expected 2 botInfo entries, got %d", len(status.BotInfo))
	}
	for _, bot := range status.BotInfo {
		if bot.Protection != "disabled" {
			t.Fatalf("expected bot %s protection=disabled, got %q", bot.ID, bot.Protection)
		}
	}

	select {
	case options := <-shutdownCh:
		if options.RestoreConfig {
			t.Fatal("restoreConfig should be false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected shutdown handler to be called")
	}
}

func TestAppShutdown_RewritesExistingStatusFileWhenExportStopped(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	repo := repository.NewProtectionRepository(nil)
	if err := repo.SaveProtectionConfig(&repository.ProtectionConfig{
		AssetName: "openclaw",
		AssetID:   "shutdown-existing-status-bot",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("SaveProtectionConfig failed: %v", err)
	}

	originalStop := stopProtectionProxyForPolicy
	stopProtectionProxyForPolicy = func(assetID string) string {
		return `{"success":true}`
	}
	t.Cleanup(func() {
		stopProtectionProxyForPolicy = originalStop
	})

	originalAssetRefresher := statusRewriteAssetRefresher
	statusRewriteAssetRefresher = func() []core.Asset {
		return []core.Asset{
			{
				ID:           "shutdown-existing-status-bot",
				Name:         "OpenClaw",
				SourcePlugin: "openclaw",
				Metadata: map[string]string{
					"pid": "24680",
				},
			},
			{
				ID:           "other-bot",
				Name:         "Other",
				SourcePlugin: "nullclaw",
				Metadata: map[string]string{
					"pid": "13579",
				},
			},
		}
	}
	t.Cleanup(func() {
		statusRewriteAssetRefresher = originalAssetRefresher
	})

	workspace := t.TempDir()
	homeDir := t.TempDir()
	pm := core.GetPathManager()
	if err := pm.ResetForTest(workspace, homeDir); err != nil {
		t.Fatalf("ResetForTest failed: %v", err)
	}
	t.Cleanup(func() {
		_ = pm.ResetForTest("", "")
	})

	exportDir := filepath.Join(workspace, "export")
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		t.Fatalf("MkdirAll export dir failed: %v", err)
	}
	statusFile := filepath.Join(exportDir, "status.json")
	originalStatus := map[string]interface{}{
		"timestamp": float64(123),
		"botInfo": []map[string]interface{}{
			{
				"id":         "shutdown-existing-status-bot",
				"pid":        "12345",
				"protection": "enabled",
			},
			{
				"id":         "other-bot",
				"pid":        "67890",
				"protection": "bypass",
			},
		},
	}
	statusRaw, err := json.MarshalIndent(originalStatus, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent status failed: %v", err)
	}
	if err := os.WriteFile(statusFile, statusRaw, 0644); err != nil {
		t.Fatalf("WriteFile status failed: %v", err)
	}

	server := NewAPIServer()
	server.token = "test-token"
	shutdownCh := make(chan AppShutdownOptions, 1)
	server.SetShutdownHandler(func(options AppShutdownOptions) error {
		shutdownCh <- options
		return nil
	})

	req := newAuthRequest(
		"POST",
		"/api/v1/app/shutdown",
		"test-token",
		bytes.NewBufferString(`{"restoreConfig":false}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.setupRoutes().ServeHTTP(rec, req)

	assertStatusCode(t, rec.Code, http.StatusOK)

	updatedRaw, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatalf("ReadFile status failed: %v", err)
	}

	var updatedStatus struct {
		BotInfo []struct {
			ID         string `json:"id"`
			PID        string `json:"pid"`
			Protection string `json:"protection"`
		} `json:"botInfo"`
	}
	if err := json.Unmarshal(updatedRaw, &updatedStatus); err != nil {
		t.Fatalf("Unmarshal status failed: %v", err)
	}
	if len(updatedStatus.BotInfo) != 2 {
		t.Fatalf("expected 2 botInfo entries, got %d", len(updatedStatus.BotInfo))
	}
	for _, bot := range updatedStatus.BotInfo {
		if bot.Protection != "disabled" {
			t.Fatalf("expected bot %s protection=disabled, got %q", bot.ID, bot.Protection)
		}
		switch bot.ID {
		case "shutdown-existing-status-bot":
			if bot.PID != "24680" {
				t.Fatalf("expected bot %s pid=24680, got %q", bot.ID, bot.PID)
			}
		case "other-bot":
			if bot.PID != "13579" {
				t.Fatalf("expected bot %s pid=13579, got %q", bot.ID, bot.PID)
			}
		}
	}

	select {
	case <-shutdownCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected shutdown handler to be called")
	}
}

func TestAppShutdown_InvalidJSON(t *testing.T) {
	server := NewAPIServer()
	server.token = "test-token"

	req := newAuthRequest(
		"POST",
		"/api/v1/app/shutdown",
		"test-token",
		bytes.NewBufferString(`{"restoreConfig":`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.setupRoutes().ServeHTTP(rec, req)

	assertStatusCode(t, rec.Code, http.StatusBadRequest)
	apiResp := parseAPIResponse(t, rec.Body)
	assertErrorCode(t, apiResp, CodeInvalidParam)
}

// ========== Export Control Tests ==========

func TestExportStop_NotRunning(t *testing.T) {
	_, ts, token := setupTestServer(t)
	defer ts.Close()

	req, err := http.NewRequest("POST", ts.URL+"/api/v1/export/stop", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	assertStatusCode(t, resp.StatusCode, http.StatusOK)

	apiResp := parseAPIResponse(t, resp.Body)
	assertErrorCode(t, apiResp, CodeSuccess)

	data, ok := apiResp.Data.(string)
	if !ok {
		t.Fatalf("Expected data to be a string, got %T", apiResp.Data)
	}
	if data != "export service already stopped" {
		t.Errorf("Expected data='export service already stopped', got: %v", data)
	}
}

func TestExportStart_AlreadyRunning(t *testing.T) {
	server := NewAPIServer()
	server.token = "test-token"
	server.exportService = &ExportServiceImpl{running: true}
	handler := server.setupRoutes()

	req := newAuthRequest("POST", "/api/v1/export/start", "test-token", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertStatusCode(t, rec.Code, http.StatusOK)

	apiResp := parseAPIResponse(t, rec.Body)
	assertErrorCode(t, apiResp, CodeSuccess)

	data, ok := apiResp.Data.(string)
	if !ok {
		t.Fatalf("Expected data to be a string, got %T", apiResp.Data)
	}
	if data != "export service already started" {
		t.Errorf("Expected data='export service already started', got: %v", data)
	}
}

func TestExportEndpoints_WrongMethod(t *testing.T) {
	_, ts, token := setupTestServer(t)
	defer ts.Close()

	testCases := []struct {
		name   string
		method string
		path   string
	}{
		{"GET export/start", "GET", "/api/v1/export/start"},
		{"GET export/stop", "GET", "/api/v1/export/stop"},
	}

	client := &http.Client{}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			// Should return 405 or 404
			if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotFound {
				t.Errorf("Expected 405 or 404, got %d", resp.StatusCode)
			}
		})
	}
}

// ========== Scan Endpoint Tests ==========

func TestScan_WrongMethod(t *testing.T) {
	_, ts, token := setupTestServer(t)
	defer ts.Close()

	req, err := http.NewRequest("GET", ts.URL+"/api/v1/scan", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should return 405 or 404
	if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 405 or 404, got %d", resp.StatusCode)
	}
}

func TestScan_AuthRequired(t *testing.T) {
	_, ts, _ := setupTestServer(t)
	defer ts.Close()

	req, err := http.NewRequest("POST", ts.URL+"/api/v1/scan", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	// No auth header

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	assertStatusCode(t, resp.StatusCode, http.StatusUnauthorized)
}

// ========== Protection Policy Tests ==========

func TestGetProtectionPolicy_EmptyBodyUsesAllBots(t *testing.T) {
	server := NewAPIServer()
	server.token = "test-token"
	handler := server.setupRoutes()

	req := newAuthRequest("GET", "/api/v1/protection/policy", "test-token", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusBadRequest {
		t.Errorf("Expected empty body to be accepted as all bots, got %d", rec.Code)
	}
}

func TestSetProtectionPolicy_InvalidJSON(t *testing.T) {
	_, ts, token := setupTestServer(t)
	defer ts.Close()

	req, err := http.NewRequest("POST", ts.URL+"/api/v1/protection/policy", strings.NewReader("invalid json"))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	assertStatusCode(t, resp.StatusCode, http.StatusBadRequest)

	apiResp := parseAPIResponse(t, resp.Body)
	assertErrorCode(t, apiResp, CodeInvalidParam)

	if !strings.Contains(apiResp.Message, "invalid JSON") {
		t.Errorf("Expected 'invalid JSON' message, got: %s", apiResp.Message)
	}
}

func TestSetProtectionPolicy_InvalidProtectionMode(t *testing.T) {
	_, ts, token := setupTestServer(t)
	defer ts.Close()

	body := `{"protection": "invalid_mode"}`
	req, err := http.NewRequest("POST", ts.URL+"/api/v1/protection/policy", strings.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	assertStatusCode(t, resp.StatusCode, http.StatusBadRequest)

	apiResp := parseAPIResponse(t, resp.Body)
	assertErrorCode(t, apiResp, CodeInvalidParam)

	if !strings.Contains(apiResp.Message, "invalid protection mode") {
		t.Errorf("Expected 'invalid protection mode' message, got: %s", apiResp.Message)
	}
}

func TestSetProtectionPolicy_ValidModes(t *testing.T) {
	validModes := []string{"enabled", "bypass", "disabled", "ENABLED", "BYPASS", "DISABLED"}

	for _, mode := range validModes {
		t.Run("mode_"+mode, func(t *testing.T) {
			_, ts, token := setupTestServer(t)
			defer ts.Close()

			body := `{"botId":["test-bot"],"protection":"` + mode + `"}`
			req, err := http.NewRequest("POST", ts.URL+"/api/v1/protection/policy", strings.NewReader(body))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			// Should not return 400 for invalid mode
			if resp.StatusCode == http.StatusBadRequest {
				body, _ := io.ReadAll(resp.Body)
				if strings.Contains(string(body), "invalid protection mode") {
					t.Errorf("Mode %q should be valid", mode)
				}
			}
		})
	}
}

func TestSetProtectionPolicy_InvalidBotIDDoesNotApplyRuntime(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	originalStart := startProtectionProxyForPolicy
	originalStop := stopProtectionProxyForPolicy
	originalSync := syncGatewaySandboxForPolicy
	startProtectionProxyForPolicy = func(configJSON string) string {
		t.Fatalf("start should not be called for invalid botId")
		return ""
	}
	stopProtectionProxyForPolicy = func(assetID string) string {
		t.Fatalf("stop should not be called for invalid botId: %s", assetID)
		return ""
	}
	syncGatewaySandboxForPolicy = func(assetName, assetID string) string {
		t.Fatalf("sandbox sync should not be called for invalid botId: %s/%s", assetName, assetID)
		return ""
	}
	t.Cleanup(func() {
		startProtectionProxyForPolicy = originalStart
		stopProtectionProxyForPolicy = originalStop
		syncGatewaySandboxForPolicy = originalSync
	})

	_, ts, token := setupTestServer(t)
	defer ts.Close()

	body := bytes.NewBufferString(`{"botId":["wrong-bot"],"protection":"enabled"}`)
	req, err := http.NewRequest("POST", ts.URL+"/api/v1/protection/policy", body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	assertStatusCode(t, resp.StatusCode, http.StatusNotFound)

	apiResp := parseAPIResponse(t, resp.Body)
	assertErrorCode(t, apiResp, CodeNotFound)
	if !strings.Contains(apiResp.Message, "wrong-bot") {
		t.Fatalf("expected not found message to include botId, got %q", apiResp.Message)
	}

	configs, err := repository.NewProtectionRepository(nil).GetAllProtectionConfigs()
	if err != nil {
		t.Fatalf("GetAllProtectionConfigs failed: %v", err)
	}
	if len(configs) != 0 {
		t.Fatalf("expected no protection configs to be saved, got %d", len(configs))
	}
}

func TestGetProtectionPolicy_ScannedBotWithoutConfigReturnsDefaultDisabled(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	registerScannedAPITestAsset(t, "PolicyScanOnlyGet", "policy-scan-only-get")

	_, ts, token := setupTestServer(t)
	defer ts.Close()

	req := newAuthRequest(
		"GET",
		"/api/v1/protection/policy",
		token,
		bytes.NewBufferString(`{"botId":["policy-scan-only-get"]}`),
	)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)

	assertStatusCode(t, rec.Code, http.StatusOK)

	apiResp := parseAPIResponse(t, rec.Body)
	assertErrorCode(t, apiResp, CodeSuccess)

	items, ok := apiResp.Data.([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("expected one policy item, got %#v", apiResp.Data)
	}

	item, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected policy item map, got %#v", items[0])
	}

	if item["botId"] != "policy-scan-only-get" {
		t.Fatalf("expected botId=policy-scan-only-get, got %#v", item["botId"])
	}
	if item["protection"] != "disabled" {
		t.Fatalf("expected default protection=disabled, got %#v", item["protection"])
	}
}

func TestSetProtectionPolicy_ScannedBotWithoutConfigCreatesConfig(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	registerScannedAPITestAsset(t, "PolicyScanOnlySet", "policy-scan-only-set")

	_, ts, token := setupTestServer(t)
	defer ts.Close()

	body := bytes.NewBufferString(`{"botId":["policy-scan-only-set"],"protection":"enabled","botModel":{"provider":"openai","id":"gpt-4.1","url":"https://bot.example.com/v1","key":"bot-key"}}`)
	req, err := http.NewRequest("POST", ts.URL+"/api/v1/protection/policy", body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	assertStatusCode(t, resp.StatusCode, http.StatusOK)

	apiResp := parseAPIResponse(t, resp.Body)
	assertErrorCode(t, apiResp, CodeSuccess)

	config, err := repository.NewProtectionRepository(nil).GetProtectionConfig("policy-scan-only-set")
	if err != nil {
		t.Fatalf("GetProtectionConfig failed: %v", err)
	}
	if config == nil {
		t.Fatal("expected protection config to be created")
	}
	if !config.Enabled || config.AuditOnly {
		t.Fatalf("expected enabled non-bypass config, got %+v", config)
	}
	if config.BotModelConfig == nil || config.BotModelConfig.Model != "gpt-4.1" {
		t.Fatalf("expected bot model to be saved, got %+v", config.BotModelConfig)
	}
}

func TestSetProtectionPolicy_WithoutBotIDCreatesDefaultPolicy(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	_, ts, token := setupTestServer(t)
	defer ts.Close()

	body := bytes.NewBufferString(`{"protection":"enabled","userRules":["confirm-delete"],"botModel":{"provider":"openai","id":"gpt-4.1","url":"https://bot.example.com/v1","key":"bot-key"}}`)
	req, err := http.NewRequest("POST", ts.URL+"/api/v1/protection/policy", body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	assertStatusCode(t, resp.StatusCode, http.StatusOK)

	config, err := repository.NewProtectionRepository(nil).GetDefaultProtectionConfig()
	if err != nil {
		t.Fatalf("GetDefaultProtectionConfig failed: %v", err)
	}
	if config == nil {
		t.Fatal("expected default protection policy to be created")
	}
	if !config.Enabled || config.AuditOnly {
		t.Fatalf("expected enabled default policy, got %+v", config)
	}
	if config.BotModelConfig == nil || config.BotModelConfig.Model != "gpt-4.1" {
		t.Fatalf("expected default bot model to be saved, got %+v", config.BotModelConfig)
	}

	rules, found, err := repository.NewProtectionRepository(nil).GetShepherdSensitiveActions(
		repository.DefaultProtectionPolicyAssetName,
		repository.DefaultProtectionPolicyAssetID,
	)
	if err != nil {
		t.Fatalf("GetShepherdSensitiveActions failed: %v", err)
	}
	if !found || len(rules) != 1 || rules[0] != "confirm-delete" {
		t.Fatalf("expected default user rules to be saved, got found=%v rules=%v", found, rules)
	}
}

func TestHandleScan_AppliesDefaultPolicyToNewAssets(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	repo := repository.NewProtectionRepository(nil)
	if err := repo.SaveProtectionConfig(&repository.ProtectionConfig{
		AssetName:      repository.DefaultProtectionPolicyAssetName,
		AssetID:        repository.DefaultProtectionPolicyAssetID,
		Enabled:        true,
		AuditOnly:      true,
		SandboxEnabled: true,
		BotModelConfig: &repository.BotModelConfigData{
			Provider: "openai",
			BaseURL:  "https://bot.example.com/v1",
			APIKey:   "bot-key",
			Model:    "gpt-4.1",
		},
	}); err != nil {
		t.Fatalf("SaveProtectionConfig failed: %v", err)
	}
	if err := repository.NewSecurityModelConfigRepository(nil).Save(&repository.SecurityModelConfig{
		Provider: "openai",
		Endpoint: "https://security.example.com/v1",
		APIKey:   "sec-key",
		Model:    "gpt-4.1-mini",
	}); err != nil {
		t.Fatalf("Save security model failed: %v", err)
	}
	if err := repo.SaveShepherdSensitiveActions(
		repository.DefaultProtectionPolicyAssetName,
		repository.DefaultProtectionPolicyAssetID,
		[]string{"confirm-delete"},
	); err != nil {
		t.Fatalf("SaveShepherdSensitiveActions failed: %v", err)
	}

	registerScannedAPITestAsset(t, "PolicyScanAutoApply", "policy-scan-auto-apply")

	_, ts, token := setupTestServer(t)
	defer ts.Close()

	req := newAuthRequest("POST", "/api/v1/scan", token, nil)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)

	assertStatusCode(t, rec.Code, http.StatusOK)

	config, err := repo.GetProtectionConfig("policy-scan-auto-apply")
	if err != nil {
		t.Fatalf("GetProtectionConfig failed: %v", err)
	}
	if config == nil {
		t.Fatal("expected scan to create asset-specific protection config")
	}
	if !config.Enabled || !config.AuditOnly || !config.SandboxEnabled {
		t.Fatalf("expected config to inherit default policy, got %+v", config)
	}
	// 新扫描资产不继承默认策略的 BotModelConfig，待用户后续单独配置。
	if config.BotModelConfig != nil {
		t.Fatalf("expected new scanned asset to have no bot model config, got %+v", config.BotModelConfig)
	}
	if !config.InheritsDefaultPolicy {
		t.Fatalf("expected new asset to be marked as inheriting default policy, got %+v", config)
	}

	rules, found, err := repo.GetShepherdSensitiveActions("PolicyScanAutoApply", "policy-scan-auto-apply")
	if err != nil {
		t.Fatalf("GetShepherdSensitiveActions failed: %v", err)
	}
	if !found || len(rules) != 1 || rules[0] != "confirm-delete" {
		t.Fatalf("expected inherited user rules, got found=%v rules=%v", found, rules)
	}
}

func TestHandleScan_AutoEnablesProtectionForNewAssetsWhenDefaultBotModelExists(t *testing.T) {
	// 业务规则调整后：新扫描资产不继承默认策略的 BotModelConfig，
	// 也不做"有模型即强制启用"的覆盖。默认策略为 disabled 时，新资产也继承 disabled。
	setupPolicyRuntimeTestDB(t)

	repo := repository.NewProtectionRepository(nil)
	if err := repo.SaveProtectionConfig(&repository.ProtectionConfig{
		AssetName:      repository.DefaultProtectionPolicyAssetName,
		AssetID:        repository.DefaultProtectionPolicyAssetID,
		Enabled:        false,
		AuditOnly:      false,
		SandboxEnabled: true,
		BotModelConfig: &repository.BotModelConfigData{
			Provider: "openai",
			BaseURL:  "https://bot.example.com/v1",
			APIKey:   "bot-key",
			Model:    "gpt-4.1",
		},
	}); err != nil {
		t.Fatalf("SaveProtectionConfig failed: %v", err)
	}
	if err := repository.NewSecurityModelConfigRepository(nil).Save(&repository.SecurityModelConfig{
		Provider: "openai",
		Endpoint: "https://security.example.com/v1",
		APIKey:   "sec-key",
		Model:    "gpt-4.1-mini",
	}); err != nil {
		t.Fatalf("Save security model failed: %v", err)
	}

	registerScannedAPITestAsset(t, "PolicyScanAutoEnable", "policy-scan-auto-enable")

	_, ts, token := setupTestServer(t)
	defer ts.Close()

	req := newAuthRequest("POST", "/api/v1/scan", token, nil)
	rec := httptest.NewRecorder()
	ts.Config.Handler.ServeHTTP(rec, req)

	assertStatusCode(t, rec.Code, http.StatusOK)

	config, err := repo.GetProtectionConfig("policy-scan-auto-enable")
	if err != nil {
		t.Fatalf("GetProtectionConfig failed: %v", err)
	}
	if config == nil {
		t.Fatal("expected scan to create asset-specific protection config")
	}
	if config.Enabled {
		t.Fatalf("expected new asset protection to inherit disabled default, got %+v", config)
	}
	// 新扫描资产不携带 BotModelConfig。
	if config.BotModelConfig != nil {
		t.Fatalf("expected new scanned asset to have no bot model config, got %+v", config.BotModelConfig)
	}
	if !config.InheritsDefaultPolicy {
		t.Fatalf("expected new asset to be marked as inheriting default policy, got %+v", config)
	}
}

func TestSetSecurityModel_SavesConfig(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	_, ts, token := setupTestServer(t)
	defer ts.Close()

	body := bytes.NewBufferString(`{"provider":"openai","id":"gpt-4.1","url":"https://api.openai.com/v1","key":"sec-key"}`)
	req, err := http.NewRequest("POST", ts.URL+"/api/v1/security/model", body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	assertStatusCode(t, resp.StatusCode, http.StatusOK)
	apiResp := parseAPIResponse(t, resp.Body)
	assertErrorCode(t, apiResp, CodeSuccess)

	saved, err := repository.NewSecurityModelConfigRepository(nil).Get()
	if err != nil {
		t.Fatalf("Get security model config failed: %v", err)
	}
	if saved == nil {
		t.Fatal("expected security model config to be saved")
	}
	if saved.Provider != "openai" || saved.Model != "gpt-4.1" || saved.Endpoint != "https://api.openai.com/v1" || saved.APIKey != "sec-key" {
		t.Fatalf("unexpected saved config: %+v", saved)
	}
}

func TestSetSecurityModel_InvalidConfigReturnsBadRequest(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	_, ts, token := setupTestServer(t)
	defer ts.Close()

	body := bytes.NewBufferString(`{"provider":"openai","id":"","url":"https://api.openai.com/v1","key":"sec-key"}`)
	req, err := http.NewRequest("POST", ts.URL+"/api/v1/security/model", body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	assertStatusCode(t, resp.StatusCode, http.StatusBadRequest)

	apiResp := parseAPIResponse(t, resp.Body)
	assertErrorCode(t, apiResp, CodeInvalidParam)
	if !strings.Contains(apiResp.Message, "invalid security model config") {
		t.Fatalf("unexpected error message: %s", apiResp.Message)
	}
}

func TestSetSecurityModel_RefreshesStatusFileWhenExportRunning(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	statusFile := filepath.Join(t.TempDir(), "status.json")
	server := NewAPIServer()
	server.token = "test-token"
	server.exportService = &ExportServiceImpl{
		running:    true,
		statusFile: statusFile,
	}

	handler := server.setupRoutes()
	body := bytes.NewBufferString(`{"provider":"openai","id":"gpt-4.1","url":"https://api.openai.com/v1","key":"sec-key"}`)
	req := newAuthRequest("POST", "/api/v1/security/model", "test-token", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assertStatusCode(t, rec.Code, http.StatusOK)
	apiResp := parseAPIResponse(t, rec.Body)
	assertErrorCode(t, apiResp, CodeSuccess)

	raw, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatalf("expected status file to be written: %v", err)
	}

	var status map[string]interface{}
	if err := json.Unmarshal(raw, &status); err != nil {
		t.Fatalf("failed to parse status file: %v", err)
	}

	securityModel, ok := status["securityModel"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected securityModel object in status file, got %#v", status["securityModel"])
	}
	if securityModel["provider"] != "openai" || securityModel["id"] != "gpt-4.1" || securityModel["url"] != "https://api.openai.com/v1" || securityModel["key"] != "sec-key" {
		t.Fatalf("unexpected securityModel in status file: %#v", securityModel)
	}
}

func TestApplyProtectionPolicyRuntime_StartsProxyWhenProtectionBecomesEnabled(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	if err := repository.NewSecurityModelConfigRepository(nil).Save(&repository.SecurityModelConfig{
		Provider: "openai",
		Endpoint: "https://example.com/v1",
		APIKey:   "sec-key",
		Model:    "gpt-4.1",
	}); err != nil {
		t.Fatalf("Save security model failed: %v", err)
	}

	var startPayload string
	originalStart := startProtectionProxyForPolicy
	originalStop := stopProtectionProxyForPolicy
	originalSync := syncGatewaySandboxForPolicy
	startProtectionProxyForPolicy = func(configJSON string) string {
		startPayload = configJSON
		return `{"success":true}`
	}
	stopProtectionProxyForPolicy = func(assetID string) string {
		t.Fatalf("stop should not be called, id=%s", assetID)
		return ""
	}
	syncGatewaySandboxForPolicy = func(assetName, assetID string) string {
		t.Fatalf("sync should not be called, asset=%s id=%s", assetName, assetID)
		return ""
	}
	t.Cleanup(func() {
		startProtectionProxyForPolicy = originalStart
		stopProtectionProxyForPolicy = originalStop
		syncGatewaySandboxForPolicy = originalSync
	})

	newConfig := &repository.ProtectionConfig{
		AssetName:      "openclaw",
		AssetID:        "bot-1",
		Enabled:        true,
		AuditOnly:      true,
		BotModelConfig: &repository.BotModelConfigData{Provider: "openai", BaseURL: "https://bot.example.com/v1", APIKey: "bot-key", Model: "gpt-4o"},
	}

	applyProtectionPolicyRuntime(nil, newConfig, nil)

	if startPayload == "" {
		t.Fatal("expected start proxy to be called")
	}

	var startConfig proxy.ProtectionConfig
	if err := json.Unmarshal([]byte(startPayload), &startConfig); err != nil {
		t.Fatalf("Failed to parse start payload: %v", err)
	}
	if startConfig.AssetName != "openclaw" || startConfig.AssetID != "bot-1" {
		t.Fatalf("unexpected asset identity: %+v", startConfig)
	}
	if startConfig.SecurityModel == nil || startConfig.SecurityModel.Provider != "openai" {
		t.Fatalf("expected security model to be included, got %+v", startConfig.SecurityModel)
	}
	if startConfig.BotModel == nil || startConfig.BotModel.BaseURL != "https://bot.example.com/v1" {
		t.Fatalf("expected bot model to be included, got %+v", startConfig.BotModel)
	}
	if startConfig.Runtime == nil || !startConfig.Runtime.AuditOnly {
		t.Fatalf("expected runtime config with auditOnly=true, got %+v", startConfig.Runtime)
	}
}

func TestApplyProtectionPolicyRuntime_StopsProxyWhenProtectionBecomesDisabled(t *testing.T) {
	var stoppedAssetID string
	originalStart := startProtectionProxyForPolicy
	originalStop := stopProtectionProxyForPolicy
	originalSync := syncGatewaySandboxForPolicy
	startProtectionProxyForPolicy = func(configJSON string) string {
		t.Fatalf("start should not be called")
		return ""
	}
	stopProtectionProxyForPolicy = func(assetID string) string {
		stoppedAssetID = assetID
		return `{"success":true}`
	}
	syncGatewaySandboxForPolicy = func(assetName, assetID string) string {
		t.Fatalf("sync should not be called")
		return ""
	}
	t.Cleanup(func() {
		startProtectionProxyForPolicy = originalStart
		stopProtectionProxyForPolicy = originalStop
		syncGatewaySandboxForPolicy = originalSync
	})

	oldConfig := &repository.ProtectionConfig{AssetName: "openclaw", AssetID: "bot-2", Enabled: true}
	newConfig := &repository.ProtectionConfig{AssetName: "openclaw", AssetID: "bot-2", Enabled: false}

	applyProtectionPolicyRuntime(oldConfig, newConfig, nil)

	if stoppedAssetID != "bot-2" {
		t.Fatalf("unexpected stop target id: %s", stoppedAssetID)
	}
}

func TestApplyProtectionPolicyRuntime_SyncsSandboxLikeUIFlow(t *testing.T) {
	var syncCalls int
	originalStart := startProtectionProxyForPolicy
	originalStop := stopProtectionProxyForPolicy
	originalSync := syncGatewaySandboxForPolicy
	startProtectionProxyForPolicy = func(configJSON string) string {
		t.Fatalf("start should not be called")
		return ""
	}
	stopProtectionProxyForPolicy = func(assetID string) string {
		t.Fatalf("stop should not be called")
		return ""
	}
	syncGatewaySandboxForPolicy = func(assetName, assetID string) string {
		syncCalls++
		if assetName != "openclaw" || assetID != "bot-3" {
			t.Fatalf("unexpected sync target: %s/%s", assetName, assetID)
		}
		return `{"success":true}`
	}
	t.Cleanup(func() {
		startProtectionProxyForPolicy = originalStart
		stopProtectionProxyForPolicy = originalStop
		syncGatewaySandboxForPolicy = originalSync
	})

	oldConfig := &repository.ProtectionConfig{AssetName: "openclaw", AssetID: "bot-3", Enabled: true, SandboxEnabled: false}
	newConfig := &repository.ProtectionConfig{AssetName: "openclaw", AssetID: "bot-3", Enabled: true, SandboxEnabled: true}

	applyProtectionPolicyRuntime(oldConfig, newConfig, nil)

	if syncCalls != 1 {
		t.Fatalf("expected exactly one sandbox sync, got %d", syncCalls)
	}
}

func TestProtectionPolicy_WrongMethod(t *testing.T) {
	_, ts, token := setupTestServer(t)
	defer ts.Close()

	testCases := []struct {
		name   string
		method string
	}{
		{"PUT instead of POST", "PUT"},
		{"DELETE", "DELETE"},
	}

	client := &http.Client{}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+"/api/v1/protection/policy", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			// Unsupported methods should get 404 or 405
			if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotFound {
				t.Errorf("Expected 404 or 405 for %s, got %d", tc.method, resp.StatusCode)
			}
		})
	}
}

// ========== Routing Tests ==========

func TestUnknownRoute(t *testing.T) {
	_, ts, token := setupTestServer(t)
	defer ts.Close()

	req, err := http.NewRequest("GET", ts.URL+"/api/v1/nonexistent", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	assertStatusCode(t, resp.StatusCode, http.StatusNotFound)
}

func TestRouteMethodNotAllowed(t *testing.T) {
	_, ts, token := setupTestServer(t)
	defer ts.Close()

	testCases := []struct {
		name          string
		method        string
		path          string
		allowedMethod string
	}{
		{"DELETE on status", "DELETE", "/api/v1/status", "GET"},
		{"PUT on status", "PUT", "/api/v1/status", "GET"},
		{"PATCH on export/start", "PATCH", "/api/v1/export/start", "POST"},
	}

	client := &http.Client{}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			// Should return 404 or 405
			if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotFound {
				t.Errorf("Expected 404 or 405 for %s %s, got %d", tc.method, tc.path, resp.StatusCode)
			}
		})
	}
}

// ========== Response Format Tests ==========

func TestResponseContentType(t *testing.T) {
	_, ts, token := setupTestServer(t)
	defer ts.Close()

	req, err := http.NewRequest("GET", ts.URL+"/api/v1/status", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected Content-Type to contain 'application/json', got: %s", contentType)
	}
}

func TestErrorResponseFormat(t *testing.T) {
	_, ts, _ := setupTestServer(t)
	defer ts.Close()

	// Request without auth should return error response
	resp, err := http.Get(ts.URL + "/api/v1/status")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	apiResp := parseAPIResponse(t, resp.Body)

	// Verify error response has required fields
	if apiResp.Code == 0 {
		t.Error("Error response should have non-zero code")
	}
	if apiResp.Message == "" {
		t.Error("Error response should have a message")
	}
}

// ========== ExportServiceImpl Unit Tests ==========

func TestExportService_StartStop(t *testing.T) {
	// Skip if PathManager is not initialized
	// This test can be run when the environment is properly set up
	t.Skip("Requires PathManager initialization")

	svc := NewExportService()
	if svc.IsRunning() {
		t.Error("New service should not be running")
	}

	if err := svc.Start(); err != nil {
		t.Fatalf("Failed to start export service: %v", err)
	}

	if !svc.IsRunning() {
		t.Error("Service should be running after Start")
	}

	status := svc.ExportStatus()
	if !status.Enabled {
		t.Error("Status should show enabled=true")
	}
	if status.ExportDir == "" {
		t.Error("Export directory should be set")
	}

	if err := svc.Stop(); err != nil {
		t.Fatalf("Failed to stop export service: %v", err)
	}

	if svc.IsRunning() {
		t.Error("Service should not be running after Stop")
	}
}

func TestExportStatusInfo_Structure(t *testing.T) {
	info := ExportStatusInfo{
		Enabled:    true,
		ExportDir:  "/tmp/export",
		StatusFile: "status.json",
		AuditFile:  "audit.jsonl",
		EventsFile: "events.jsonl",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal ExportStatusInfo: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	expectedFields := []string{"enabled", "exportDir", "statusFile", "auditFile", "eventsFile"}
	for _, field := range expectedFields {
		if _, exists := parsed[field]; !exists {
			t.Errorf("Expected field %q in JSON output", field)
		}
	}
}

// ========== StatusData Structure Tests ==========

func TestStatusData_JSONStructure(t *testing.T) {
	status := &StatusData{
		BotInfo:       []BotInfo{},
		RiskInfo:      []RiskInfo{},
		SkillResult:   []SkillResultInfo{},
		SecurityModel: &SecurityModelInfo{},
		Timestamp:     time.Now().UnixMilli(),
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Failed to marshal StatusData: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify expected fields
	expectedFields := []string{"botInfo", "riskInfo", "skillResult", "securityModel", "timestamp"}
	for _, field := range expectedFields {
		if _, exists := parsed[field]; !exists {
			t.Errorf("Expected field %q in StatusData JSON", field)
		}
	}
}

func TestBotInfo_JSONStructure(t *testing.T) {
	info := BotInfo{
		Name:       "TestBot",
		ID:         "bot:123",
		PID:        "1234",
		Image:      "testbot",
		Conf:       "/etc/testbot.json",
		Bind:       "127.0.0.1:8080",
		Protection: "enabled",
		BotModel: &BotModelInfo{
			Provider: "openai",
			ID:       "gpt-4",
			URL:      "https://api.openai.com",
			Key:      "sk-test",
		},
		Metrics: &MetricsInfo{
			AnalysisCount: 10,
			MessageCount:  100,
		},
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal BotInfo: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed["name"] != "TestBot" {
		t.Errorf("Expected name='TestBot', got %v", parsed["name"])
	}
	if parsed["protection"] != "enabled" {
		t.Errorf("Expected protection='enabled', got %v", parsed["protection"])
	}
	for _, field := range []string{"pid", "image", "conf", "bind", "botModel", "metrics"} {
		if _, exists := parsed[field]; !exists {
			t.Errorf("Expected field %q in BotInfo JSON", field)
		}
	}
	botModel, ok := parsed["botModel"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected botModel to be an object")
	}
	if _, exists := botModel["key"]; !exists {
		t.Errorf("Expected botModel.key field in JSON output")
	}
}

func TestRiskInfo_JSONStructure(t *testing.T) {
	info := RiskInfo{
		Name:       "risk",
		Level:      "high",
		Source:     "openclaw",
		BotID:      "",
		Mitigation: []MitigationInfo{},
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal RiskInfo: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	requiredFields := []string{"name", "level", "source", "botId", "mitigation"}
	for _, field := range requiredFields {
		if _, exists := parsed[field]; !exists {
			t.Errorf("Expected field %q in RiskInfo JSON", field)
		}
	}
}

func TestSkillIssue_JSONStructure(t *testing.T) {
	issue := SkillIssue{
		Type:     "prompt_injection",
		Desc:     "desc",
		Evidence: "",
	}
	data, err := json.Marshal(issue)
	if err != nil {
		t.Fatalf("Failed to marshal SkillIssue: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	requiredFields := []string{"type", "desc", "evidence"}
	for _, field := range requiredFields {
		if _, exists := parsed[field]; !exists {
			t.Errorf("Expected field %q in SkillIssue JSON", field)
		}
	}
}

func TestToSkillIssue_ParsesStructuredJSON(t *testing.T) {
	raw := `{"type":"prompt_injection","desc":"Skill 包含可注入模板","evidence":"prompt = f'Execute {user_input}'"}`
	issue := toSkillIssue(raw)
	if issue.Type != "prompt_injection" {
		t.Fatalf("Type = %q, want %q", issue.Type, "prompt_injection")
	}
	if issue.Desc == "" {
		t.Fatalf("Desc should not be empty")
	}
	if issue.Evidence == "" {
		t.Fatalf("Evidence should not be empty")
	}
}

func TestToSkillIssue_ParsesDescriptionField(t *testing.T) {
	raw := `{"type":"prompt_injection","description":"Skill 包含可注入模板","evidence":"prompt = f'Execute {user_input}'"}`
	issue := toSkillIssue(raw)
	if issue.Type != "prompt_injection" {
		t.Fatalf("Type = %q, want %q", issue.Type, "prompt_injection")
	}
	if issue.Desc != "Skill 包含可注入模板" {
		t.Fatalf("Desc = %q, want %q", issue.Desc, "Skill 包含可注入模板")
	}
	if issue.Evidence == "" {
		t.Fatalf("Evidence should not be empty")
	}
}

func TestResolveRiskBotID_ByConfigPath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "openclaw.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("Write config failed: %v", err)
	}

	assets := []core.Asset{
		{
			ID:           "bot-openclaw-1",
			SourcePlugin: "openclaw",
			Metadata: map[string]string{
				"config_path": configPath,
				"bind":        "127.0.0.1:18789",
			},
		},
	}
	risk := core.Risk{
		SourcePlugin: "openclaw",
		Args: map[string]interface{}{
			"config_path": configPath,
		},
	}

	if got := resolveRiskBotID(risk, assets); got != "bot-openclaw-1" {
		t.Fatalf("resolveRiskBotID() = %q, want %q", got, "bot-openclaw-1")
	}
}

func TestExpandSkillResultEntries_InferFromAssetSkillDir(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".openclaw")
	skillDir := filepath.Join(configDir, "workspace", "skills", "danger-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("Create skill dir failed: %v", err)
	}
	configPath := filepath.Join(configDir, "openclaw.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("Write config failed: %v", err)
	}

	assets := []core.Asset{
		{
			ID:           "bot-openclaw-1",
			SourcePlugin: "openclaw",
			Metadata: map[string]string{
				"config_path": configPath,
			},
		},
	}
	record := repository.SkillScanRecord{
		SkillName:    "danger-skill",
		SkillHash:    "hash-1",
		SourcePlugin: "openclaw",
		RiskLevel:    "high",
	}

	entries := expandSkillResultEntries(record, assets, tmpDir)
	if len(entries) != 1 {
		t.Fatalf("expandSkillResultEntries() count = %d, want 1", len(entries))
	}
	if entries[0].BotID != "bot-openclaw-1" {
		t.Fatalf("BotID = %q, want %q", entries[0].BotID, "bot-openclaw-1")
	}
	if entries[0].Source != skillDir {
		t.Fatalf("Source = %q, want %q", entries[0].Source, skillDir)
	}
}

// ========== ProtectionPolicyRequest Validation Tests ==========

func TestProtectionPolicyRequest_Parsing(t *testing.T) {
	testCases := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name:    "valid minimal",
			json:    `{"botId":["test"],"protection":"enabled"}`,
			wantErr: false,
		},
		{
			name:    "valid with tokenLimit",
			json:    `{"botId":["test"],"protection":"enabled","tokenLimit":{"session":1000,"daily":10000}}`,
			wantErr: false,
		},
		{
			name:    "valid with all fields",
			json:    `{"botId":["test"],"protection":"bypass","userRules":["rule1"],"tokenLimit":{"session":100,"daily":1000},"botModel":{"provider":"openai","id":"gpt-4","url":"https://api.openai.com"}}`,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			json:    `{"botId": test}`,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var req ProtectionPolicyRequest
			err := json.Unmarshal([]byte(tc.json), &req)
			if (err != nil) != tc.wantErr {
				t.Errorf("Unmarshal error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestProtectionPolicyRequest_Parsing_OpenAndModelKey(t *testing.T) {
	jsonBody := `{
		"botId": ["test"],
		"protection": "enabled",
		"permission": {
			"open": true,
			"path": {"mode": "blacklist", "paths": ["/etc"]}
		},
		"botModel": {
			"provider": "openai",
			"id": "gpt-4.1",
			"url": "https://api.openai.com/v1",
			"key": "sk-test"
		}
	}`

	var req ProtectionPolicyRequest
	if err := json.Unmarshal([]byte(jsonBody), &req); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if req.Permission == nil || req.Permission.Open == nil || !*req.Permission.Open {
		t.Fatalf("permission.open should be true")
	}
	if len(req.BotID) != 1 || req.BotID[0] != "test" {
		t.Fatalf("botId should be parsed as array")
	}
	if req.BotModel == nil || req.BotModel.Key != "sk-test" {
		t.Fatalf("botModel.key should be parsed")
	}
}

func TestConvertToExternalPolicy_IncludesPermissionOpenAndBotModelKey(t *testing.T) {
	cfg := &repository.ProtectionConfig{
		AssetID:        "bot-1",
		Enabled:        true,
		SandboxEnabled: true,
		BotModelConfig: &repository.BotModelConfigData{
			Provider: "openai",
			Model:    "gpt-4.1",
			BaseURL:  "https://api.openai.com/v1",
			APIKey:   "sk-key",
		},
	}

	resp := convertToExternalPolicy(cfg, "bot-1")
	if resp.Permission == nil || resp.Permission.Open == nil || !*resp.Permission.Open {
		t.Fatalf("permission.open should be true")
	}
	if resp.BotModel == nil || resp.BotModel.Key != "sk-key" {
		t.Fatalf("botModel.key should be exported")
	}
}

func TestConvertToExternalPolicy_DefaultShape(t *testing.T) {
	cfg := &repository.ProtectionConfig{
		AssetID: "bot-2",
	}

	resp := convertToExternalPolicy(cfg, "bot-2")
	if resp.UserRules == nil {
		t.Fatalf("userRules should always be present")
	}
	if resp.TokenLimit == nil {
		t.Fatalf("tokenLimit should always be present")
	}
	if resp.TokenLimit.Session != 0 || resp.TokenLimit.Daily != 0 {
		t.Fatalf("tokenLimit should default to zero values, got %+v", resp.TokenLimit)
	}
	if resp.Permission == nil || resp.Permission.Open == nil {
		t.Fatalf("permission.open should always be present")
	}
	if *resp.Permission.Open {
		t.Fatalf("permission.open should default to false")
	}
	if resp.Permission.Path == nil || resp.Permission.Path.Mode != "blacklist" {
		t.Fatalf("permission.path should default to blacklist")
	}
	if resp.Permission.Network == nil || resp.Permission.Network.Inbound == nil || resp.Permission.Network.Outbound == nil {
		t.Fatalf("permission.network should include inbound and outbound defaults")
	}
	if resp.Permission.Shell == nil || resp.Permission.Shell.Mode != "blacklist" {
		t.Fatalf("permission.shell should default to blacklist")
	}
	if resp.BotModel == nil {
		t.Fatalf("botModel should always be present")
	}
	if resp.BotModel.Key != "" {
		t.Fatalf("botModel.key should default to empty string, got %q", resp.BotModel.Key)
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response failed: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	botModel, ok := parsed["botModel"].(map[string]interface{})
	if !ok {
		t.Fatalf("botModel should be an object in JSON")
	}
	if _, exists := botModel["key"]; !exists {
		t.Fatalf("botModel.key should always be present in JSON output")
	}
}

// ========== Logging Middleware Tests ==========

func TestLoggingMiddleware(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := loggingMiddleware(next)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("Next handler should have been called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestResponseCapture(t *testing.T) {
	rec := httptest.NewRecorder()
	rc := newResponseCapture(rec)

	// Default status should be 200
	if rc.statusCode != http.StatusOK {
		t.Errorf("Default status should be 200, got %d", rc.statusCode)
	}

	rc.WriteHeader(http.StatusNotFound)

	if rc.statusCode != http.StatusNotFound {
		t.Errorf("Status should be 404 after WriteHeader, got %d", rc.statusCode)
	}
}

// ========== APIResponse Helper Tests ==========

func TestSuccess_ResponseFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	data := map[string]string{"key": "value"}

	Success(rec, data)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Code != CodeSuccess {
		t.Errorf("Expected code %d, got %d", CodeSuccess, resp.Code)
	}
	if resp.Message != "success" {
		t.Errorf("Expected message 'success', got %s", resp.Message)
	}
}

func TestError_ResponseFormat(t *testing.T) {
	rec := httptest.NewRecorder()

	Error(rec, http.StatusBadRequest, CodeInvalidParam, "test error message")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Code != CodeInvalidParam {
		t.Errorf("Expected code %d, got %d", CodeInvalidParam, resp.Code)
	}
	if resp.Message != "test error message" {
		t.Errorf("Expected message 'test error message', got %s", resp.Message)
	}
}

// ========== ScanResponse Structure Tests ==========

func TestScanResponse_JSONStructure(t *testing.T) {
	resp := ScanResponse{
		Message:       "scan completed",
		ScanTime:      time.Now().Format(time.RFC3339),
		BotInfo:       []BotInfo{},
		RiskInfo:      []RiskInfo{},
		SkillResult:   []SkillResultInfo{},
		SecurityModel: &SecurityModelInfo{},
		Timestamp:     time.Now().UnixMilli(),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal ScanResponse: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	expectedFields := []string{"message", "scanTime", "botInfo", "riskInfo", "skillResult", "securityModel", "timestamp"}
	for _, field := range expectedFields {
		if _, exists := parsed[field]; !exists {
			t.Errorf("Expected field %q in ScanResponse JSON", field)
		}
	}
}

func TestEnsureStatusDataShape_DefaultsEmptyCollections(t *testing.T) {
	status := ensureStatusDataShape(&StatusData{
		BotInfo: []BotInfo{{
			Name: "bot-a",
			ID:   "bot-a",
		}},
		RiskInfo: []RiskInfo{{
			Name:  "risk-a",
			Level: "low",
		}},
		SkillResult:   nil,
		SecurityModel: nil,
	})

	if status.SkillResult == nil {
		t.Fatal("skillResult should be an empty array, not nil")
	}
	if status.SecurityModel == nil {
		t.Fatal("securityModel should default to an empty object")
	}
	if status.BotInfo[0].BotModel == nil {
		t.Fatal("botInfo.botModel should default to an empty object")
	}
	if status.BotInfo[0].Metrics == nil {
		t.Fatal("botInfo.metrics should default to an empty object")
	}
	if status.RiskInfo[0].Mitigation == nil {
		t.Fatal("riskInfo.mitigation should default to an empty array")
	}
}

func TestCollectBotInfoFromAssets_DefaultsProtectionDisabledAndFillsFields(t *testing.T) {
	svc := &ExportServiceImpl{}
	infos := svc.collectBotInfoFromAssets([]core.Asset{
		{
			ID:           "bot-openclaw-1",
			Name:         "OpenClaw Bot",
			SourcePlugin: "openclaw",
			ProcessPaths: []string{"/usr/local/bin/openclaw-gateway"},
			DisplaySections: []core.DisplaySection{
				{
					Title: "Gateway Configuration",
					Items: []core.DisplayItem{
						{Label: "Bind", Value: "127.0.0.1"},
						{Label: "Port", Value: "18789"},
					},
				},
			},
			Metadata: map[string]string{
				"config_path":  "/tmp/openclaw.json",
				"gateway_bind": "127.0.0.1",
				"gateway_port": "18789",
			},
		},
	})

	if len(infos) != 1 {
		t.Fatalf("collectBotInfoFromAssets() count = %d, want 1", len(infos))
	}

	info := infos[0]
	if info.PID != "N/A" {
		t.Fatalf("PID = %q, want %q", info.PID, "N/A")
	}
	if info.Image != "/usr/local/bin/openclaw-gateway" {
		t.Fatalf("Image = %q, want %q", info.Image, "/usr/local/bin/openclaw-gateway")
	}
	if info.Bind != "127.0.0.1:18789" {
		t.Fatalf("Bind = %q, want %q", info.Bind, "127.0.0.1:18789")
	}
	if info.Protection != "disabled" {
		t.Fatalf("Protection = %q, want %q", info.Protection, "disabled")
	}
}

func TestCollectBotInfoFromAssets_UsesDisplaySectionsAsFallback(t *testing.T) {
	svc := &ExportServiceImpl{}
	infos := svc.collectBotInfoFromAssets([]core.Asset{
		{
			ID:           "bot-dintalclaw-1",
			Name:         "DintalClaw",
			SourcePlugin: "dintalclaw",
			DisplaySections: []core.DisplaySection{
				{
					Title: "Process Info",
					Items: []core.DisplayItem{
						{Label: "Image Path", Value: "/opt/dintalclaw/python"},
						{Label: "PID", Value: "4321"},
						{Label: "Listener Address", Value: "127.0.0.1:8080, 0.0.0.0:8081"},
					},
				},
			},
		},
	})

	if len(infos) != 1 {
		t.Fatalf("collectBotInfoFromAssets() count = %d, want 1", len(infos))
	}

	info := infos[0]
	if info.PID != "4321" {
		t.Fatalf("PID = %q, want %q", info.PID, "4321")
	}
	if info.Image != "/opt/dintalclaw/python" {
		t.Fatalf("Image = %q, want %q", info.Image, "/opt/dintalclaw/python")
	}
	if info.Bind != "127.0.0.1:8080" {
		t.Fatalf("Bind = %q, want %q", info.Bind, "127.0.0.1:8080")
	}
	if info.Protection != "disabled" {
		t.Fatalf("Protection = %q, want %q", info.Protection, "disabled")
	}
}

func TestCollectBotInfoFromAssets_UsesRuntimeProtectionStatus(t *testing.T) {
	setupPolicyRuntimeTestDB(t)

	repo := repository.NewProtectionRepository(nil)
	if err := repo.SaveProtectionConfig(&repository.ProtectionConfig{
		AssetName: "openclaw",
		AssetID:   "bot-openclaw-2",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("SaveProtectionConfig failed: %v", err)
	}

	originalProxyRunningByAsset := exportProxyRunningByAsset
	exportProxyRunningByAsset = func(assetID string) bool {
		return false
	}
	t.Cleanup(func() {
		exportProxyRunningByAsset = originalProxyRunningByAsset
	})

	svc := &ExportServiceImpl{}
	infos := svc.collectBotInfoFromAssets([]core.Asset{{
		ID:           "bot-openclaw-2",
		Name:         "OpenClaw Bot",
		SourcePlugin: "openclaw",
	}})

	if len(infos) != 1 {
		t.Fatalf("collectBotInfoFromAssets() count = %d, want 1", len(infos))
	}
	if infos[0].Protection != "disabled" {
		t.Fatalf("Protection = %q, want %q when proxy is stopped", infos[0].Protection, "disabled")
	}

	exportProxyRunningByAsset = func(assetID string) bool {
		return true
	}
	if err := repo.SaveProtectionConfig(&repository.ProtectionConfig{
		AssetName: "openclaw",
		AssetID:   "bot-openclaw-2",
		Enabled:   true,
		AuditOnly: true,
	}); err != nil {
		t.Fatalf("SaveProtectionConfig failed: %v", err)
	}

	infos = svc.collectBotInfoFromAssets([]core.Asset{{
		ID:           "bot-openclaw-2",
		Name:         "OpenClaw Bot",
		SourcePlugin: "openclaw",
	}})
	if infos[0].Protection != "bypass" {
		t.Fatalf("Protection = %q, want %q when proxy is running in audit mode", infos[0].Protection, "bypass")
	}
}

func TestExportService_StopWritesFinalStatusFile(t *testing.T) {
	statusFile := filepath.Join(t.TempDir(), "status.json")
	svc := &ExportServiceImpl{
		running:    true,
		statusFile: statusFile,
		stopChan:   make(chan struct{}),
	}

	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

	raw, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatalf("expected status file to be written on stop: %v", err)
	}

	var status map[string]interface{}
	if err := json.Unmarshal(raw, &status); err != nil {
		t.Fatalf("failed to parse status file: %v", err)
	}
	if _, ok := status["timestamp"]; !ok {
		t.Fatalf("expected timestamp in final status file, got %#v", status)
	}
}

// ========== AuditLog and SecurityEvent Structure Tests ==========

func TestAuditLogEntry_JSONStructure(t *testing.T) {
	entry := &AuditLogEntry{
		BotID:         "bot-001",
		LogID:         "log-123",
		LogTimestamp:  time.Now().Format(time.RFC3339),
		RequestID:     "req-456",
		Model:         "gpt-4",
		Action:        "generate",
		RiskLevel:     "low",
		RiskCauses:    "",
		DurationMs:    150,
		TokenCount:    500,
		UserRequest:   "Hello",
		ToolCallCount: 0,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to marshal AuditLogEntry: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	requiredFields := []string{"botId", "logId", "logTimestamp", "requestId", "model", "action", "riskLevel"}
	for _, field := range requiredFields {
		if _, exists := parsed[field]; !exists {
			t.Errorf("Expected field %q in AuditLogEntry JSON", field)
		}
	}
}

func TestSecurityEventEntry_JSONStructure(t *testing.T) {
	entry := &SecurityEventEntry{
		BotID:      "bot-001",
		EventID:    "event-123",
		Timestamp:  time.Now().Format(time.RFC3339),
		EventType:  "warning",
		ActionDesc: "suspicious action detected",
		RiskType:   "command_injection",
		Detail:     "rm -rf /",
		Source:     "shepherd",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to marshal SecurityEventEntry: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	requiredFields := []string{"botId", "eventId", "timestamp", "event_type", "action_desc", "risk_type", "detail", "source"}
	for _, field := range requiredFields {
		if _, exists := parsed[field]; !exists {
			t.Errorf("Expected field %q in SecurityEventEntry JSON", field)
		}
	}
}

// ========== ExportService File Writing Tests ==========

func TestExportService_WriteAuditLog_NotRunning(t *testing.T) {
	svc := NewExportService()

	entry := &AuditLogEntry{
		BotID: "bot-test",
		LogID: "test",
	}

	err := svc.WriteAuditLog(entry)
	if err == nil {
		t.Error("Expected error when writing to non-running service")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("Expected 'not running' error, got: %v", err)
	}
}

func TestExportService_WriteSecurityEvent_NotRunning(t *testing.T) {
	svc := NewExportService()

	event := &SecurityEventEntry{
		BotID:   "bot-test",
		EventID: "test",
	}

	err := svc.WriteSecurityEvent(event)
	if err == nil {
		t.Error("Expected error when writing to non-running service")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("Expected 'not running' error, got: %v", err)
	}
}

// ========== ExportError Tests ==========

func TestExportError(t *testing.T) {
	err := &exportError{msg: "test error"}
	if err.Error() != "test error" {
		t.Errorf("Expected 'test error', got %s", err.Error())
	}
}

// ========== DiscoveryInfo Tests ==========

func TestDiscoveryInfo_JSONStructure(t *testing.T) {
	info := DiscoveryInfo{
		PID:       12345,
		Port:      8080,
		Token:     "abc123",
		URL:       "http://127.0.0.1:8080",
		StartedAt: time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal DiscoveryInfo: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	requiredFields := []string{"pid", "port", "token", "url", "startedAt"}
	for _, field := range requiredFields {
		if _, exists := parsed[field]; !exists {
			t.Errorf("Expected field %q in DiscoveryInfo JSON", field)
		}
	}
}

// TestAPIServer_StartStop_DiscoveryAndHTTP 在本机真实监听 TCP、写入 api.json，并用 Bearer 调用 /api/v1/status。
func TestAPIServer_StartStop_DiscoveryAndHTTP(t *testing.T) {
	tmpHome := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "api_e2e_ws")
	pm := core.GetPathManager()
	if err := pm.ResetForTest(workspace, tmpHome); err != nil {
		t.Fatalf("ResetForTest: %v", err)
	}

	var srv *APIServer
	t.Cleanup(func() {
		if srv != nil {
			_ = srv.Stop()
		}
		_ = repository.CloseDB()
		_ = pm.ResetForTest("", "")
	})

	dbPath := filepath.Join(workspace, "bot_sec_manager.db")
	if err := repository.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	srv = NewAPIServer()
	if err := srv.Start(0); err != nil {
		t.Fatalf("Start: %v", err)
	}

	discoveryPath := filepath.Join(workspace, "api.json")
	raw, err := os.ReadFile(discoveryPath)
	if err != nil {
		t.Fatalf("read discovery file: %v", err)
	}
	var disc map[string]interface{}
	if err := json.Unmarshal(raw, &disc); err != nil {
		t.Fatalf("discovery JSON: %v", err)
	}
	token, _ := disc["token"].(string)
	if token == "" {
		t.Fatal("discovery token empty")
	}
	portF, ok := disc["port"].(float64)
	if !ok || portF <= 0 {
		t.Fatalf("discovery port invalid: %#v", disc["port"])
	}
	port := int(portF)

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("http://127.0.0.1:%d/api/v1/status", port),
		nil,
	)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status HTTP %d: %s", resp.StatusCode, string(body))
	}
	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if apiResp.Code != CodeSuccess {
		t.Fatalf("API code=%d msg=%s", apiResp.Code, apiResp.Message)
	}

	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := os.Stat(discoveryPath); !os.IsNotExist(err) {
		t.Errorf("api.json should be removed after Stop, stat err=%v", err)
	}
}

// ========== APIServer State Tests ==========

func TestAPIServer_NewServer(t *testing.T) {
	server := NewAPIServer()
	if server == nil {
		t.Fatal("NewAPIServer returned nil")
	}
	if server.running {
		t.Error("New server should not be running")
	}
	if server.token != "" {
		t.Error("New server should have empty token")
	}
	if server.port != 0 {
		t.Error("New server should have port 0")
	}
}

func TestAPIServer_Token(t *testing.T) {
	server := NewAPIServer()
	server.token = "test-token"

	if got := server.Token(); got != "test-token" {
		t.Errorf("Token() = %q, want %q", got, "test-token")
	}
}

func TestAPIServer_Port(t *testing.T) {
	server := NewAPIServer()
	server.port = 8080

	if got := server.Port(); got != 8080 {
		t.Errorf("Port() = %d, want %d", got, 8080)
	}
}

func TestAPIServer_IsRunning(t *testing.T) {
	server := NewAPIServer()

	if server.IsRunning() {
		t.Error("New server should not be running")
	}

	server.running = true

	if !server.IsRunning() {
		t.Error("Server should be running after setting flag")
	}
}

func TestAPIServer_SetExportService(t *testing.T) {
	server := NewAPIServer()

	if server.exportService != nil {
		t.Error("New server should have nil export service")
	}

	mockSvc := &ExportServiceImpl{}
	server.SetExportService(mockSvc)

	if server.exportService == nil {
		t.Error("Export service should be set")
	}
}

// ========== Integration-style Tests for Handler Recording ==========

func TestHandler_StatusRecording(t *testing.T) {
	server := NewAPIServer()
	server.token = "test-token"

	req := newAuthRequest("GET", "/api/v1/status", "test-token", nil)
	rec := httptest.NewRecorder()

	handler := server.setupRoutes()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	// Verify response is valid JSON
	var resp APIResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
}

func TestHandler_StatusHasExportFocus(t *testing.T) {
	server := NewAPIServer()
	server.token = "test-token"

	req := newAuthRequest("GET", "/api/v1/status", "test-token", nil)
	rec := httptest.NewRecorder()

	handler := server.setupRoutes()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Code != CodeSuccess {
		t.Errorf("Expected code %d, got %d", CodeSuccess, resp.Code)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be a map, got %T", resp.Data)
	}
	if _, exists := data["enabled"]; !exists {
		t.Error("Expected 'enabled' in status data")
	}
	if _, exists := data["exportDir"]; !exists {
		t.Error("Expected 'exportDir' in status data")
	}
}

func TestIsSkillContentRisk(t *testing.T) {
	if !isSkillContentRisk(core.Risk{ID: "riskSkillSecurityIssue"}) {
		t.Fatal("riskSkillSecurityIssue should be treated as skill content risk")
	}
	if isSkillContentRisk(core.Risk{ID: "riskNoAuth"}) {
		t.Fatal("non-skill risk should not be treated as skill content risk")
	}
}

// ========== Concurrent Access Tests ==========

func TestAPIServer_ConcurrentTokenAccess(t *testing.T) {
	server := NewAPIServer()
	server.token = "initial-token"

	done := make(chan bool)

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			_ = server.Token()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestAPIServer_ConcurrentIsRunning(t *testing.T) {
	server := NewAPIServer()

	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func() {
			_ = server.IsRunning()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// ========== File Operations Tests ==========

func TestWriteJSON_ValidData(t *testing.T) {
	rec := httptest.NewRecorder()
	resp := APIResponse{
		Code:    0,
		Message: "success",
		Data:    map[string]string{"test": "data"},
	}

	writeJSON(rec, http.StatusOK, resp)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected application/json content type, got %s", contentType)
	}
}

// ========== Path Value Tests ==========

func TestProtectionPolicy_RequestBodyRoute(t *testing.T) {
	server := NewAPIServer()
	server.token = "test-token"
	handler := server.setupRoutes()

	req := newAuthRequest("GET", "/api/v1/protection/policy", "test-token", bytes.NewBufferString(`{"botId":["my-bot-123"]}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// The response may be 404 when config does not exist, but should not be 400 for request parsing
	if rec.Code == http.StatusBadRequest {
		body := rec.Body.String()
		if strings.Contains(body, "invalid JSON") {
			t.Error("Valid body should not trigger JSON parsing error")
		}
	}
}

// ========== Body Reading Tests ==========

func TestSetProtectionPolicy_BodyReading(t *testing.T) {
	server := NewAPIServer()
	server.token = "test-token"
	handler := server.setupRoutes()

	// Test with valid JSON body
	body := bytes.NewBufferString(`{"botId":["test-bot"],"protection":"enabled"}`)
	req := newAuthRequest("POST", "/api/v1/protection/policy", "test-token", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should not get bad request for valid body
	if rec.Code == http.StatusBadRequest {
		var resp APIResponse
		json.NewDecoder(rec.Body).Decode(&resp)
		if strings.Contains(resp.Message, "failed to read request body") {
			t.Error("Should be able to read valid request body")
		}
	}
}

// ========== Export Directory Tests ==========

func TestExportServiceImpl_DirectorySetup(t *testing.T) {
	tmpDir := t.TempDir()
	exportDir := filepath.Join(tmpDir, "export")

	// Verify directory can be created
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		t.Fatalf("Failed to create export directory: %v", err)
	}

	// Verify directory exists
	info, err := os.Stat(exportDir)
	if err != nil {
		t.Fatalf("Export directory does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("Export path should be a directory")
	}
}

func TestExportServiceImpl_FileWriting(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.jsonl")

	// Write test data
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open test file: %v", err)
	}

	testData := `{"test": "data"}` + "\n"
	if _, err := f.WriteString(testData); err != nil {
		f.Close()
		t.Fatalf("Failed to write test data: %v", err)
	}
	f.Close()

	// Read and verify
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	if string(content) != testData {
		t.Errorf("File content = %q, want %q", string(content), testData)
	}
}

// ========== Permission Config Tests ==========

func TestPermissionConfig_Parsing(t *testing.T) {
	testCases := []struct {
		name string
		json string
	}{
		{
			name: "path permission",
			json: `{"path": {"mode": "blacklist", "paths": ["/tmp"]}}`,
		},
		{
			name: "network permission",
			json: `{"network": {"outbound": {"mode": "whitelist", "addresses": ["api.openai.com"]}}}`,
		},
		{
			name: "shell permission",
			json: `{"shell": {"mode": "blacklist", "commands": ["rm", "dd"]}}`,
		},
		{
			name: "all permissions",
			json: `{"path": {"mode": "blacklist", "paths": []}, "network": {}, "shell": {"mode": "whitelist", "commands": ["ls"]}}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var perm PermissionConfig
			if err := json.Unmarshal([]byte(tc.json), &perm); err != nil {
				t.Errorf("Failed to parse %s: %v", tc.name, err)
			}
		})
	}
}
