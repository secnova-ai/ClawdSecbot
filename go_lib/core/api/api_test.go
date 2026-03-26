package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go_lib/core"
	"go_lib/core/repository"
)

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
		Message:  "scan completed",
		ScanTime: time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal ScanResponse: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	expectedFields := []string{"message", "scanTime"}
	for _, field := range expectedFields {
		if _, exists := parsed[field]; !exists {
			t.Errorf("Expected field %q in ScanResponse JSON", field)
		}
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
