package webbridge

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go_lib/chatmodel-routing/adapter"
	"go_lib/core"
	"go_lib/core/callback_bridge"
	"go_lib/core/modelfactory"
	"go_lib/core/proxy"
	"go_lib/core/sandbox"
	"go_lib/core/service"
	"go_lib/core/shepherd"
)

const rpcPrefix = "/api/v1/rpc/"

type Server struct {
	hub  *EventHub
	auth *WebAuthManager

	mu                 sync.Mutex
	bridge             *callback_bridge.Bridge
	versionCheckServer *service.VersionCheckService
	staticWebRoot      string
	uiSessionLock      *UISessionLock
}

type rpcRequest struct {
	Strings []string `json:"strings"`
	Ints    []int    `json:"ints"`
}

type bootstrapInitRequest struct {
	WorkspaceDirPrefix string `json:"workspace_dir_prefix"`
	HomeDir            string `json:"home_dir"`
	CurrentVersion     string `json:"current_version"`
}

type uiSessionRequest struct {
	ClientID    string `json:"client_id"`
	ClientLabel string `json:"client_label"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func NewServer(staticWebRoot string) *Server {
	return &Server{
		hub:           NewEventHub(),
		auth:          NewWebAuthManager(),
		staticWebRoot: strings.TrimSpace(staticWebRoot),
		uiSessionLock: NewUISessionLock(20 * time.Second),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/v1/bootstrap/init", s.handleBootstrapInit)
	mux.HandleFunc("/api/v1/auth/login", s.handleAuthLogin)
	mux.HandleFunc("/api/v1/auth/change-password", s.handleAuthChangePassword)
	mux.HandleFunc("/api/v1/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("/api/v1/session/claim", s.handleSessionClaim)
	mux.HandleFunc("/api/v1/session/heartbeat", s.handleSessionHeartbeat)
	mux.HandleFunc("/api/v1/session/release", s.handleSessionRelease)
	mux.HandleFunc("/api/v1/events", s.handleEvents)
	mux.HandleFunc(rpcPrefix, s.handleRPC)
	if s.staticWebRoot != "" {
		mux.HandleFunc("/", s.handleStatic)
	}
	return withCORS(mux)
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}

	root := s.staticWebRoot
	if root == "" {
		http.NotFound(w, r)
		return
	}

	cleaned := filepath.Clean("/" + strings.TrimSpace(r.URL.Path))
	relative := strings.TrimPrefix(cleaned, "/")
	target := filepath.Join(root, relative)

	if relative == "" {
		target = filepath.Join(root, "index.html")
	}

	if info, err := os.Stat(target); err == nil && !info.IsDir() {
		if serveGzipStaticFile(w, r, target, info) {
			return
		}
		http.ServeFile(w, r, target)
		return
	}

	// SPA fallback.
	indexPath := filepath.Join(root, "index.html")
	if info, err := os.Stat(indexPath); err == nil {
		if serveGzipStaticFile(w, r, indexPath, info) {
			return
		}
		http.ServeFile(w, r, indexPath)
		return
	}

	http.NotFound(w, r)
}

func serveGzipStaticFile(w http.ResponseWriter, r *http.Request, path string, info os.FileInfo) bool {
	if !shouldGzipStaticFile(r, path) {
		return false
	}

	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if contentType == "" {
		buffer := make([]byte, 512)
		n, _ := file.Read(buffer)
		contentType = http.DetectContentType(buffer[:n])
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return false
		}
	}

	appendVaryHeader(w, "Accept-Encoding")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))

	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return true
	}

	gz := gzip.NewWriter(w)
	defer gz.Close()
	_, _ = io.Copy(gz, file)
	return true
}

func shouldGzipStaticFile(r *http.Request, path string) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".js", ".css", ".json", ".wasm", ".svg", ".txt", ".xml", ".map", ".ttf", ".otf", ".woff", ".woff2":
		return true
	default:
		return false
	}
}

func appendVaryHeader(w http.ResponseWriter, value string) {
	existing := w.Header().Values("Vary")
	for _, item := range existing {
		for _, part := range strings.Split(item, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}
	w.Header().Add("Vary", value)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}

	var req loginRequest
	if err := decodeJSONRequest(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	token, err := s.auth.Login(req.Username, req.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"username": strings.TrimSpace(req.Username),
		"token":    token,
	})
}

func (s *Server) handleAuthChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}
	if !s.requireAuth(w, r) {
		return
	}

	var req changePasswordRequest
	if err := decodeJSONRequest(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	if err := s.auth.ChangePassword(req.CurrentPassword, req.NewPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}
	s.auth.Logout(r)
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.auth.AuthenticateRequest(r) {
		return true
	}
	writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
		"success":    false,
		"error_code": "auth_required",
		"error":      "authentication required",
	})
	return false
}

func (s *Server) handleSessionClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}
	if !s.requireAuth(w, r) {
		return
	}

	req, err := decodeUISessionRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	state, granted := s.uiSessionLock.Claim(req.ClientID, req.ClientLabel, time.Now())
	payload := uiSessionStatePayload(state)
	if !granted {
		payload["success"] = false
		payload["error_code"] = "ui_session_occupied"
		payload["error"] = "web ui session is already occupied by another client"
		writeJSON(w, http.StatusConflict, payload)
		return
	}

	payload["success"] = true
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleSessionHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}
	if !s.requireAuth(w, r) {
		return
	}

	req, err := decodeUISessionRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	state, ok := s.uiSessionLock.Heartbeat(req.ClientID, time.Now())
	payload := uiSessionStatePayload(state)
	if !ok {
		payload["success"] = false
		payload["error_code"] = "ui_session_not_owner"
		payload["error"] = "web ui session ownership lost"
		writeJSON(w, http.StatusConflict, payload)
		return
	}

	payload["success"] = true
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleSessionRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}
	if !s.requireAuth(w, r) {
		return
	}

	req, err := decodeUISessionRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	state, released := s.uiSessionLock.Release(req.ClientID)
	payload := uiSessionStatePayload(state)
	payload["success"] = true
	payload["released"] = released
	writeJSON(w, http.StatusOK, payload)
}

func decodeUISessionRequest(r *http.Request) (uiSessionRequest, error) {
	var req uiSessionRequest
	if err := decodeJSONRequest(r, &req); err != nil {
		return req, err
	}

	req.ClientID = strings.TrimSpace(req.ClientID)
	req.ClientLabel = strings.TrimSpace(req.ClientLabel)
	if req.ClientID == "" {
		return req, errors.New("client_id is required")
	}
	return req, nil
}

func decodeJSONRequest(r *http.Request, target interface{}) error {
	if r.Body == nil {
		return errors.New("request body is required")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return errors.New("failed to read request body")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("invalid JSON: %v", err)
	}
	return nil
}

func uiSessionStatePayload(state UISessionState) map[string]interface{} {
	leaseExpiresAt := ""
	if !state.LeaseExpiresAt.IsZero() {
		leaseExpiresAt = state.LeaseExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	return map[string]interface{}{
		"owner_client_id":    state.OwnerClientID,
		"owner_client_label": state.OwnerClientLabel,
		"lease_expires_at":   leaseExpiresAt,
		"remaining_ms":       state.RemainingMs,
		"lease_duration_ms":  state.LeaseDurationMs,
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	if err := s.ensureBridge(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	s.hub.ServeSSE(w, r)
}

func (s *Server) handleBootstrapInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "failed to read request body"})
		return
	}

	var req bootstrapInitRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": fmt.Sprintf("invalid JSON: %v", err)})
		return
	}

	if req.WorkspaceDirPrefix == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "workspace_dir_prefix is required"})
		return
	}
	if req.HomeDir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "home_dir is required"})
		return
	}
	if req.CurrentVersion == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "current_version is required"})
		return
	}

	pm := core.GetPathManager()
	if pm.IsInitialized() {
		existingWorkspace := pm.GetWorkspaceDir()
		existingHome := pm.GetHomeDir()
		if existingWorkspace != req.WorkspaceDirPrefix || existingHome != req.HomeDir {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"success": false,
				"error": fmt.Sprintf(
					"path manager already initialized: workspace_dir=%s home_dir=%s, requested workspace_dir=%s home_dir=%s; restart web bridge to switch workspace",
					existingWorkspace,
					existingHome,
					req.WorkspaceDirPrefix,
					req.HomeDir,
				),
			})
			return
		}
	}

	initPathsResult, err := core.Initialize(req.WorkspaceDirPrefix, req.HomeDir, "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	initLogResult, err := core.InitLogging("")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	dbReq := map[string]string{"current_version": req.CurrentVersion}
	dbReqJSON, _ := json.Marshal(dbReq)
	initDBResult := service.InitializeDatabase(string(dbReqJSON))
	if success, _ := initDBResult["success"].(bool); !success {
		writeJSON(w, http.StatusInternalServerError, initDBResult)
		return
	}

	authBootstrap, err := s.auth.EnsureInitialized()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	authRequired := !authBootstrap.GeneratedInitialPassword && !s.auth.AuthenticateRequest(r)

	if err := s.ensureBridge(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	authPayload := map[string]interface{}{
		"username":                   authBootstrap.Username,
		"auth_required":              authRequired,
		"generated_initial_password": authBootstrap.GeneratedInitialPassword,
	}
	if authBootstrap.Token != "" {
		authPayload["token"] = authBootstrap.Token
	}
	if authBootstrap.InitialPassword != "" {
		authPayload["initial_password"] = authBootstrap.InitialPassword
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"init_paths":   initPathsResult,
		"init_logging": initLogResult,
		"init_db":      initDBResult,
		"auth":         authPayload,
	})
}

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"success": false, "error": "method not allowed"})
		return
	}
	if !s.requireAuth(w, r) {
		return
	}

	method := strings.TrimPrefix(r.URL.Path, rpcPrefix)
	method = strings.TrimSpace(method)
	if method == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "missing method"})
		return
	}

	var req rpcRequest
	if r.Body != nil {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "failed to read request body"})
			return
		}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": fmt.Sprintf("invalid JSON: %v", err)})
				return
			}
		}
	}

	payload, err := s.dispatch(method, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	writeRawJSON(w, http.StatusOK, payload)
}

func (s *Server) ensureBridge() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.bridge != nil && s.bridge.IsRunning() {
		return nil
	}

	bridge, err := callback_bridge.NewBridge(func(message string) {
		s.hub.Publish(message)
	})
	if err != nil {
		return err
	}

	s.bridge = bridge
	proxy.SetCallbackBridge(bridge)
	shepherd.GetSecurityEventBuffer().SetCallback(func(event shepherd.SecurityEvent) {
		bridge.SendSecurityEvent(map[string]interface{}{
			"id":          event.ID,
			"timestamp":   event.Timestamp,
			"event_type":  event.EventType,
			"action_desc": event.ActionDesc,
			"risk_type":   event.RiskType,
			"detail":      event.Detail,
			"source":      event.Source,
			"asset_name":  event.AssetName,
			"asset_id":    event.AssetID,
			"request_id":  event.RequestID,
		})
	})
	return nil
}

func (s *Server) dispatch(method string, req rpcRequest) (string, error) {
	arg := func(index int) (string, error) {
		if index < 0 || index >= len(req.Strings) {
			return "", fmt.Errorf("%s requires string arg[%d]", method, index)
		}
		return req.Strings[index], nil
	}
	intArg := func(index int) (int, error) {
		if index < 0 || index >= len(req.Ints) {
			return 0, fmt.Errorf("%s requires int arg[%d]", method, index)
		}
		return req.Ints[index], nil
	}

	switch method {
	case "InitPathsFFI":
		workspaceDir, err := arg(0)
		if err != nil {
			return "", err
		}
		homeDir, err := arg(1)
		if err != nil {
			return "", err
		}
		result, callErr := core.Initialize(workspaceDir, homeDir, "")
		if callErr != nil {
			return toJSON(map[string]interface{}{"success": false, "error": callErr.Error()}), nil
		}
		return toJSON(result), nil
	case "InitLoggingFFI":
		logDir, err := arg(0)
		if err != nil {
			return "", err
		}
		result, callErr := core.InitLogging(logDir)
		if callErr != nil {
			return toJSON(map[string]interface{}{"success": false, "error": callErr.Error()}), nil
		}
		return toJSON(result), nil
	case "InitDatabase":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.InitializeDatabase(v)), nil
	case "CloseDatabase":
		return toJSON(service.CloseDatabase()), nil

	case "GetPluginsFFI":
		return toJSON(core.GetRegisteredPlugins()), nil
	case "ScanAssetsFFI":
		result, callErr := core.ScanAllAssets()
		if callErr != nil {
			return toJSON(map[string]interface{}{"success": false, "error": callErr.Error()}), nil
		}
		return toJSON(result), nil
	case "AssessRisksFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		result, callErr := core.AssessAllRisksFromString(v)
		if callErr != nil {
			return toJSON(map[string]interface{}{"success": false, "error": callErr.Error()}), nil
		}
		return toJSON(result), nil
	case "MitigateRiskFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return core.MitigateRiskByPlugin(v), nil

	case "GetSupportedProviders":
		scopeArg, err := arg(0)
		if err != nil {
			return "", err
		}
		var providers []adapter.ProviderInfo
		switch adapter.ProviderScope(scopeArg) {
		case adapter.ScopeSecurity:
			providers = adapter.GetProviders(adapter.ScopeSecurity)
		case adapter.ScopeBot:
			providers = adapter.GetProviders(adapter.ScopeBot)
		default:
			providers = adapter.GetAllProviders()
		}
		return toJSON(providers), nil
	case "GetProviderModels":
		payload, err := arg(0)
		if err != nil {
			return "", err
		}
		return modelfactory.GetProviderModelsJSON(payload), nil
	case "IsCallbackBridgeRunning":
		if s.bridge != nil && s.bridge.IsRunning() {
			return toJSON(map[string]interface{}{"success": true, "running": true}), nil
		}
		return toJSON(map[string]interface{}{"success": true, "running": false}), nil

	case "SaveScanResult":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.SaveScanResult(v)), nil
	case "GetLatestScanResult":
		return toJSON(service.GetLatestScanResult()), nil
	case "GetScannedSkillHashes":
		return toJSON(service.GetScannedSkillHashes()), nil
	case "SaveSkillScanResult":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.SaveSkillScanResult(v)), nil
	case "GetSkillScanByHash":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.GetSkillScanByHash(v)), nil
	case "DeleteSkillScanFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.DeleteSkillScan(v)), nil
	case "GetRiskySkills":
		return toJSON(service.GetRiskySkills()), nil
	case "TrustSkillScan":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.TrustSkill(v)), nil
	case "GetAllSkillScansFFI":
		return toJSON(service.GetAllSkillScans()), nil

	case "SaveProtectionStateFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.SaveProtectionState(v)), nil
	case "GetProtectionStateFFI":
		return toJSON(service.GetProtectionState()), nil
	case "ClearProtectionStateFFI":
		return toJSON(service.ClearProtectionState()), nil
	case "SaveProtectionConfigFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.SaveProtectionConfig(v)), nil
	case "GetProtectionConfigFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.GetProtectionConfig(v)), nil
	case "GetEnabledProtectionConfigsFFI":
		return toJSON(service.GetEnabledProtectionConfigs()), nil
	case "GetActiveProtectionCountFFI":
		return toJSON(service.GetActiveProtectionCount()), nil
	case "SetProtectionEnabledFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.SetProtectionEnabled(v)), nil
	case "DeleteProtectionConfigFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.DeleteProtectionConfig(v)), nil
	case "SaveProtectionStatisticsFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.SaveProtectionStatistics(v)), nil
	case "GetProtectionStatisticsFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.GetProtectionStatistics(v)), nil
	case "ClearProtectionStatisticsFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.ClearProtectionStatistics(v)), nil
		case "GetShepherdRulesFFI":
			v, err := arg(0)
			if err != nil {
				return "", err
			}
			return toJSON(service.GetShepherdRules(v)), nil
		case "SaveShepherdRulesFFI":
			v, err := arg(0)
			if err != nil {
				return "", err
			}
			return toJSON(service.SaveShepherdRules(v)), nil
	case "ClearAllDataFFI":
		return toJSON(service.ClearAllData()), nil
	case "SaveHomeDirectoryPermissionFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.SaveHomeDirectoryPermission(v)), nil

	case "SaveAuditLogFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.SaveAuditLog(v)), nil
	case "SaveAuditLogsBatchFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.SaveAuditLogsBatch(v)), nil
	case "GetAuditLogsFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.GetAuditLogs(v)), nil
	case "GetAuditLogCountFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.GetAuditLogCount(v)), nil
	case "GetAuditLogStatisticsFFI":
		return toJSON(service.GetAuditLogStatistics("{}")), nil
	case "GetAuditLogStatisticsWithFilterFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.GetAuditLogStatisticsWithFilter(v)), nil
	case "GetAuditLogAssetsFFI":
		return toJSON(service.GetAuditLogAssets()), nil
	case "CleanOldAuditLogsFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.CleanOldAuditLogs(v)), nil
	case "ClearAllAuditLogsFFI":
		return toJSON(service.ClearAllAuditLogs("{}")), nil
	case "ClearAuditLogsWithFilterFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.ClearAuditLogsWithFilter(v)), nil

	case "SaveSecurityEventsBatchFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.SaveSecurityEventsBatch(v)), nil
	case "GetSecurityEventsFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.GetSecurityEvents(v)), nil
	case "GetSecurityEventCountFFI":
		return toJSON(service.GetSecurityEventCount()), nil
	case "ClearAllSecurityEventsFFI":
		return toJSON(service.ClearAllSecurityEvents()), nil
	case "ClearSecurityEventsFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.ClearSecurityEvents(v)), nil
	case "GetSecurityEventsByRequestIDFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.GetSecurityEventsByRequestID(v)), nil
	case "GetPendingSecurityEvents":
		return shepherd.GetPendingSecurityEventsInternal(), nil
	case "ClearSecurityEventsBuffer":
		return shepherd.ClearSecurityEventsBufferInternal(), nil

	case "SaveApiMetricsFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.SaveApiMetrics(v)), nil
	case "GetApiStatisticsFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.GetApiStatistics(v)), nil
	case "GetRecentApiMetricsFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.GetRecentApiMetrics(v)), nil
	case "CleanOldApiMetricsFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.CleanOldApiMetrics(v)), nil
	case "GetDailyTokenUsageFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.GetDailyTokenUsage(v)), nil

	case "SaveSecurityModelConfigFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.SaveSecurityModelConfig(v)), nil
	case "GetSecurityModelConfigFFI":
		return toJSON(service.GetSecurityModelConfig()), nil
	case "SaveBotModelConfigFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.SaveBotModelConfig(v)), nil
	case "GetBotModelConfigFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.GetBotModelConfig(v)), nil
	case "DeleteBotModelConfigFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.DeleteBotModelConfig(v)), nil

	case "SetLanguageFFI":
		lang, err := arg(0)
		if err != nil {
			return "", err
		}
		result := service.SetLanguage(lang)
		if success, _ := result["success"].(bool); success {
			proxy.UpdateLanguage(lang)
		}
		return toJSON(result), nil
	case "GetLanguageFFI":
		return toJSON(service.GetLanguage()), nil
	case "SaveAppSettingFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.SaveAppSetting(v)), nil
	case "GetAppSettingFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return toJSON(service.GetAppSetting(v)), nil

	case "StartSandboxedGateway":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return startSandboxedGateway(v), nil
	case "StopSandboxedGateway":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return stopSandboxedGateway(v), nil
	case "GetSandboxStatus":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return getSandboxStatus(v), nil
	case "EnableProcessMonitor":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return enableProcessMonitor(v), nil
	case "DisableProcessMonitor":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return disableProcessMonitor(v), nil
	case "KillUnmanagedGateway":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return killUnmanagedGateway(v), nil
	case "GenerateSandboxPolicy":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return generateSandboxPolicy(v), nil
	case "CheckSandboxSupported":
		return toJSON(map[string]interface{}{"supported": sandbox.IsSandboxSupported()}), nil

	case "StartVersionCheckServiceFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		if err := s.ensureBridge(); err != nil {
			return toJSON(map[string]interface{}{"success": false, "error": err.Error()}), nil
		}
		s.mu.Lock()
		if s.versionCheckServer != nil {
			s.versionCheckServer.Stop()
			s.versionCheckServer = nil
		}
		var cfg service.VersionCheckConfig
		if err := json.Unmarshal([]byte(v), &cfg); err != nil {
			s.mu.Unlock()
			return toJSON(map[string]interface{}{"success": false, "error": fmt.Sprintf("invalid config: %v", err)}), nil
		}
		svc, err := service.NewVersionCheckService(&cfg, s.bridge)
		if err != nil {
			s.mu.Unlock()
			return toJSON(map[string]interface{}{"success": false, "error": err.Error()}), nil
		}
		s.versionCheckServer = svc
		s.versionCheckServer.Start()
		s.mu.Unlock()
		return toJSON(map[string]interface{}{"success": true}), nil
	case "StopVersionCheckServiceFFI":
		s.mu.Lock()
		if s.versionCheckServer != nil {
			s.versionCheckServer.Stop()
			s.versionCheckServer = nil
		}
		s.mu.Unlock()
		return toJSON(map[string]interface{}{"success": true}), nil
	case "UpdateVersionCheckLanguageFFI":
		lang, err := arg(0)
		if err != nil {
			return "", err
		}
		s.mu.Lock()
		if s.versionCheckServer == nil {
			s.mu.Unlock()
			return toJSON(map[string]interface{}{"success": false, "error": "service not running"}), nil
		}
		s.versionCheckServer.SetLanguage(lang)
		s.mu.Unlock()
		return toJSON(map[string]interface{}{"success": true}), nil

	case "StartProtectionProxy":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return proxy.StartProtectionProxyInternal(v), nil
	case "StopProtectionProxy":
		return proxy.StopProtectionProxyInternal(), nil
	case "StopProtectionProxyByAsset":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return proxy.StopProtectionProxyByAssetInternal(v), nil
	case "ResetProtectionStatistics":
		return proxy.ResetProtectionStatisticsInternal(), nil
	case "GetProtectionProxyStatus":
		return proxy.GetProtectionProxyStatusInternal(), nil
	case "GetProtectionProxyStatusByAsset":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return proxy.GetProtectionProxyStatusByAssetInternal(v), nil
	case "UpdateProtectionConfig":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return proxy.UpdateProtectionConfigInternal(v), nil
	case "UpdateProtectionConfigByAsset":
		assetID, err := arg(0)
		if err != nil {
			return "", err
		}
		cfg, err := arg(1)
		if err != nil {
			return "", err
		}
		return proxy.UpdateProtectionConfigByAssetInternal(assetID, cfg), nil
	case "UpdateSecurityModelConfig":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return proxy.UpdateSecurityModelConfigInternal(v), nil
	case "UpdateSecurityModelConfigByAsset":
		assetID, err := arg(0)
		if err != nil {
			return "", err
		}
		cfg, err := arg(1)
		if err != nil {
			return "", err
		}
		return proxy.UpdateSecurityModelConfigByAssetInternal(assetID, cfg), nil
	case "SetProtectionProxyAuditOnly":
		v, err := intArg(0)
		if err != nil {
			return "", err
		}
		return proxy.SetProtectionProxyAuditOnlyInternal(v != 0), nil
	case "SetProtectionProxyAuditOnlyByAsset":
		assetID, err := arg(0)
		if err != nil {
			return "", err
		}
		v, err := intArg(0)
		if err != nil {
			return "", err
		}
		return proxy.SetProtectionProxyAuditOnlyByAssetInternal(assetID, v != 0), nil
	case "GetProtectionProxyLogs":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return proxy.GetProtectionProxyLogsInternal(v), nil
	case "WaitForProtectionLogs":
		sessionID, err := arg(0)
		if err != nil {
			return "", err
		}
		timeoutMs, err := intArg(0)
		if err != nil {
			return "", err
		}
		return proxy.WaitForProtectionLogsInternal(sessionID, timeoutMs), nil

	case "GetAllTruthRecordSnapshots":
		return proxy.GetAllTruthRecordSnapshotsInternal(), nil
	case "GetAuditLogs":
		limit, err := intArg(0)
		if err != nil {
			return "", err
		}
		offset, err := intArg(1)
		if err != nil {
			return "", err
		}
		riskOnlyFlag, err := intArg(2)
		if err != nil {
			return "", err
		}
		return proxy.GetAuditLogsInternal(limit, offset, riskOnlyFlag != 0), nil
	case "GetPendingAuditLogs":
		return proxy.GetPendingAuditLogsInternal(), nil
	case "ClearAuditLogs":
		return proxy.ClearAuditLogsInternal(), nil
	case "ClearAuditLogsWithFilter":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return proxy.ClearAuditLogsWithFilterInternal(v), nil

	case "SyncGatewaySandbox":
		return core.SyncGatewaySandboxByPlugin(""), nil
	case "SyncGatewaySandboxByAsset":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return core.SyncGatewaySandboxByAssetAndPlugin("", v), nil
	case "HasInitialBackupFFI":
		return core.HasInitialBackupByPlugin(""), nil
	case "HasInitialBackupByAssetFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return core.HasInitialBackupByAssetID(v), nil
	case "RestoreToInitialConfigFFI":
		return core.RestoreToInitialConfigByPlugin(""), nil
	case "RestoreToInitialConfigByAssetFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return core.RestoreToInitialConfigByAssetID(v), nil
	case "NotifyPluginAppExitFFI":
		assetName, err := arg(0)
		if err != nil {
			return "", err
		}
		assetID, err := arg(1)
		if err != nil {
			return "", err
		}
		return core.NotifyAppExitByPlugin(assetName, assetID), nil
	case "RestoreBotDefaultStateFFI":
		assetName, err := arg(0)
		if err != nil {
			return "", err
		}
		assetID, err := arg(1)
		if err != nil {
			return "", err
		}
		return core.RestoreBotDefaultStateByPlugin(assetName, assetID), nil

	case "ListBundledReActSkillsFFI":
		return shepherd.ListBundledReActSkillsInternal(), nil

	case "StartSkillSecurityScan":
		skillPath, err := arg(0)
		if err != nil {
			return "", err
		}
		modelCfg, err := arg(1)
		if err != nil {
			return "", err
		}
		return core.StartSkillSecurityScanByPlugin("", skillPath, modelCfg), nil
	case "GetSkillSecurityScanLog":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return core.GetSkillSecurityScanLogByPlugin("", v), nil
	case "GetSkillSecurityScanResult":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return core.GetSkillSecurityScanResultByPlugin("", v), nil
	case "CancelSkillSecurityScan":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return core.CancelSkillSecurityScanByPlugin("", v), nil
	case "StartBatchSkillScan":
		return core.StartBatchSkillScanByPlugin(""), nil
	case "GetBatchSkillScanLog":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return core.GetBatchSkillScanLogByPlugin("", v), nil
	case "GetBatchSkillScanResults":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return core.GetBatchSkillScanResultsByPlugin("", v), nil
	case "CancelBatchSkillScan":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return core.CancelBatchSkillScanByPlugin("", v), nil

	case "TestModelConnectionFFI":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return core.TestModelConnectionByPlugin("", v), nil
	case "DeleteSkill":
		v, err := arg(0)
		if err != nil {
			return "", err
		}
		return core.DeleteSkillByPlugin("", v), nil

	default:
		return "", errors.New("unsupported rpc method: " + method)
	}
}

func startSandboxedGateway(configJSON string) string {
	var req struct {
		AssetName         string                          `json:"asset_name"`
		GatewayBinaryPath string                          `json:"gateway_binary_path"`
		GatewayConfigPath string                          `json:"gateway_config_path"`
		GatewayArgs       []string                        `json:"gateway_args"`
		GatewayEnv        []string                        `json:"gateway_env"`
		PathPermission    sandbox.PathPermissionConfig    `json:"path_permission"`
		NetworkPermission sandbox.NetworkPermissionConfig `json:"network_permission"`
		ShellPermission   sandbox.ShellPermissionConfig   `json:"shell_permission"`
		PolicyDir         string                          `json:"policy_dir"`
		LogDir            string                          `json:"log_dir"`
	}
	if err := json.Unmarshal([]byte(configJSON), &req); err != nil {
		return toJSON(map[string]interface{}{"success": false, "error": fmt.Sprintf("invalid json: %v", err)})
	}
	if !sandbox.IsSandboxSupported() {
		return toJSON(map[string]interface{}{
			"success":           false,
			"error":             "sandbox-exec not supported",
			"sandbox_supported": false,
		})
	}

	policyDir := req.PolicyDir
	if policyDir == "" {
		policyDir = sandbox.GetDefaultPolicyDir()
	}
	manager := sandbox.GetSandboxManager(req.AssetName, policyDir)
	if req.LogDir != "" {
		manager.SetLogDir(req.LogDir)
	} else {
		pm := core.GetPathManager()
		if pm.IsInitialized() {
			manager.SetLogDir(pm.GetLogDir())
		}
	}

	config := sandbox.SandboxConfig{
		AssetName:         req.AssetName,
		GatewayBinaryPath: req.GatewayBinaryPath,
		GatewayConfigPath: req.GatewayConfigPath,
		PathPermission:    req.PathPermission,
		NetworkPermission: req.NetworkPermission,
		ShellPermission:   req.ShellPermission,
	}
	if err := manager.Configure(config, req.GatewayArgs, req.GatewayEnv); err != nil {
		return toJSON(map[string]interface{}{"success": false, "error": fmt.Sprintf("configuration failed: %v", err)})
	}
	if err := manager.Start(); err != nil {
		return toJSON(map[string]interface{}{"success": false, "error": err.Error()})
	}

	status := manager.GetStatus()
	return toJSON(map[string]interface{}{
		"success":           true,
		"managed_pid":       status.ManagedPID,
		"policy_path":       status.PolicyPath,
		"asset_name":        status.AssetName,
		"sandbox_supported": true,
	})
}

func stopSandboxedGateway(assetName string) string {
	manager := sandbox.GetSandboxManager(assetName, sandbox.GetDefaultPolicyDir())
	if manager == nil {
		return toJSON(map[string]interface{}{"success": true, "message": "no sandbox manager found"})
	}
	if err := manager.Stop(); err != nil {
		return toJSON(map[string]interface{}{"success": false, "error": err.Error()})
	}
	sandbox.RemoveProcessMonitor(assetName)
	return toJSON(map[string]interface{}{"success": true})
}

func getSandboxStatus(assetName string) string {
	manager := sandbox.GetSandboxManager(assetName, sandbox.GetDefaultPolicyDir())
	return toJSON(manager.GetStatus())
}

func enableProcessMonitor(configJSON string) string {
	var req struct {
		AssetName      string `json:"asset_name"`
		GatewayPattern string `json:"gateway_pattern"`
		CheckInterval  int    `json:"check_interval_seconds"`
	}
	if err := json.Unmarshal([]byte(configJSON), &req); err != nil {
		return toJSON(map[string]interface{}{"success": false, "error": fmt.Sprintf("invalid json: %v", err)})
	}
	manager := sandbox.GetSandboxManager(req.AssetName, sandbox.GetDefaultPolicyDir())
	monitor := sandbox.GetProcessMonitor(req.AssetName, req.GatewayPattern)
	monitor.SetSandboxManager(manager)
	manager.SetMonitor(monitor)
	if err := monitor.Start(); err != nil {
		return toJSON(map[string]interface{}{"success": false, "error": err.Error()})
	}
	return toJSON(map[string]interface{}{"success": true})
}

func disableProcessMonitor(assetName string) string {
	sandbox.RemoveProcessMonitor(assetName)
	return toJSON(map[string]interface{}{"success": true})
}

func killUnmanagedGateway(configJSON string) string {
	var req struct {
		GatewayPattern string `json:"gateway_pattern"`
		ManagedPID     int    `json:"managed_pid"`
	}
	if err := json.Unmarshal([]byte(configJSON), &req); err != nil {
		return toJSON(map[string]interface{}{"success": false, "error": fmt.Sprintf("invalid json: %v", err)})
	}
	pids, err := sandbox.FindProcessesByName(req.GatewayPattern)
	if err != nil {
		return toJSON(map[string]interface{}{"success": false, "error": err.Error()})
	}

	var killedPIDs []int
	for _, pid := range pids {
		if pid != req.ManagedPID && pid != 0 {
			if err := sandbox.KillProcess(pid); err == nil {
				killedPIDs = append(killedPIDs, pid)
			}
		}
	}
	return toJSON(map[string]interface{}{"success": true, "killed_pids": killedPIDs})
}

func generateSandboxPolicy(configJSON string) string {
	var cfg sandbox.SandboxConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return toJSON(map[string]interface{}{"success": false, "error": fmt.Sprintf("invalid json: %v", err)})
	}
	policy, err := sandbox.GeneratePlatformPolicy(cfg)
	if err != nil {
		return toJSON(map[string]interface{}{"success": false, "error": err.Error()})
	}
	return toJSON(map[string]interface{}{"success": true, "policy": policy})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))

		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,Accept,Origin,X-Requested-With")
		w.Header().Set("Access-Control-Max-Age", "600")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(toJSON(v)))
}

func writeRawJSON(w http.ResponseWriter, status int, payload string) {
	if !json.Valid([]byte(payload)) {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "handler returned invalid JSON payload",
		})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(payload))
}

func toJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"success":false,"error":"marshal error"}`
	}
	return string(b)
}
