package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"go_lib/core/logging"
	"go_lib/core/modelfactory"
	"go_lib/core/proxy"
	"go_lib/core/repository"
)

// SecurityModelRequest represents the external API format for security model config.
type SecurityModelRequest struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	URL      string `json:"url"`
	Key      string `json:"key"`
}

// handleSetSecurityModel handles POST /api/v1/security/model.
func (s *APIServer) handleSetSecurityModel(w http.ResponseWriter, r *http.Request) {
	req, err := parseSecurityModelRequest(r)
	if err != nil {
		if err == io.EOF {
			Error(w, http.StatusBadRequest, CodeInvalidParam, "request body is required")
			return
		}
		Error(w, http.StatusBadRequest, CodeInvalidParam, "invalid JSON: "+err.Error())
		return
	}

	config := &repository.SecurityModelConfig{
		Provider: strings.TrimSpace(req.Provider),
		Model:    strings.TrimSpace(req.ID),
		Endpoint: strings.TrimSpace(req.URL),
		APIKey:   strings.TrimSpace(req.Key),
	}
	if err := modelfactory.ValidateSecurityModelConfig(config); err != nil {
		Error(w, http.StatusBadRequest, CodeInvalidParam, "invalid security model config: "+err.Error())
		return
	}

	repo := repository.NewSecurityModelConfigRepository(nil)
	if err := repo.Save(config); err != nil {
		logging.Error("API: Failed to save security model config: %v", err)
		Error(w, http.StatusInternalServerError, CodeInternalError, "failed to save security model config")
		return
	}

	applySecurityModelRuntime(config)
	s.mu.Lock()
	exportService := s.exportService
	s.mu.Unlock()
	if impl, ok := exportService.(*ExportServiceImpl); ok && impl.IsRunning() {
		impl.writeStatusFile()
	}

	Success(w, map[string]interface{}{
		"message": "security model updated",
	})
}

func parseSecurityModelRequest(r *http.Request) (SecurityModelRequest, error) {
	var req SecurityModelRequest

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return req, err
	}
	defer r.Body.Close()

	if len(strings.TrimSpace(string(body))) == 0 {
		return req, io.EOF
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return req, err
	}

	return req, nil
}
func applySecurityModelRuntime(config *repository.SecurityModelConfig) {
	if config == nil {
		return
	}

	repo := repository.NewProtectionRepository(nil)
	configs, err := repo.GetAllProtectionConfigs()
	if err != nil {
		logging.Warning("API: Failed to query protection configs for security model hot reload: %v", err)
		return
	}

	for _, protectionConfig := range configs {
		if protectionConfig == nil {
			continue
		}
		runningProxy := proxy.GetProxyProtectionByAsset(protectionConfig.AssetName, protectionConfig.AssetID)
		if runningProxy == nil || !runningProxy.IsRunning() {
			continue
		}
		if err := runningProxy.UpdateSecurityModelConfig(config); err != nil {
			logging.Warning("API: Failed to hot reload security model for botId=%s: %v", protectionConfig.AssetID, err)
		}
	}
}
