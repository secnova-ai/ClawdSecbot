package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/repository"
	"go_lib/core/service"
)

type appShutdownRequest struct {
	RestoreConfig *bool `json:"restoreConfig,omitempty"`
}

var statusRewriteAssetRefresher = func() []core.Asset {
	latestAssets := map[string]core.Asset{}
	pluginManager := core.GetPluginManager()
	if pluginManager != nil {
		if scannedAssets, err := pluginManager.ScanAllAssets(); err == nil {
			for _, asset := range scannedAssets {
				if strings.TrimSpace(asset.ID) == "" {
					continue
				}
				latestAssets[asset.ID] = asset
			}
		} else {
			logging.Warning("API: Failed to refresh assets for status rewrite by live scan: %v", err)
		}
	}

	if len(latestAssets) == 0 {
		scanResult := service.GetLatestScanResult()
		scanData, _ := scanResult["data"].(*repository.ScanRecord)
		if scanData != nil {
			for _, asset := range scanData.Assets {
				if strings.TrimSpace(asset.ID) == "" {
					continue
				}
				latestAssets[asset.ID] = asset
			}
		}
	}

	refreshed := make([]core.Asset, 0, len(latestAssets))
	for _, asset := range latestAssets {
		refreshed = append(refreshed, asset)
	}
	return refreshed
}

// handleAppShutdown handles POST /api/v1/app/shutdown.
func (s *APIServer) handleAppShutdown(w http.ResponseWriter, r *http.Request) {
	req, err := parseAppShutdownRequest(r)
	if err != nil {
		Error(w, http.StatusBadRequest, CodeInvalidParam, err.Error())
		return
	}

	if err := s.disableAllProtectionPoliciesForShutdown(); err != nil {
		logging.Error("API: Failed to disable protection policies before shutdown: %v", err)
		Error(w, http.StatusInternalServerError, CodeInternalError, "failed to disable protection policies: "+err.Error())
		return
	}

	s.syncStatusAfterShutdownPolicyUpdate()

	options := AppShutdownOptions{}
	options.RestoreConfig = *req.RestoreConfig

	Success(w, map[string]interface{}{
		"accepted":      true,
		"restoreConfig": options.RestoreConfig,
		"message":       "shutdown accepted",
	})

	go func() {
		if err := s.requestAppShutdown(options); err != nil {
			logging.Error("API: Failed to request app shutdown: %v", err)
		}
	}()
}

func (s *APIServer) disableAllProtectionPoliciesForShutdown() error {
	if repository.GetDB() == nil {
		return nil
	}

	repo := repository.NewProtectionRepository(nil)
	botIDs, err := resolveRequestedBotIDs(repo, nil)
	if err != nil {
		return err
	}

	disableReq := &ProtectionPolicyRequest{Protection: "disabled"}
	for _, botID := range botIDs {
		if err := updateProtectionPolicyForBotID(repo, botID, disableReq); err != nil {
			return err
		}
	}

	return nil
}

func (s *APIServer) syncStatusAfterShutdownPolicyUpdate() {
	if impl, ok := s.exportService.(*ExportServiceImpl); ok && impl.IsRunning() {
		impl.writeStatusFile()
		return
	}

	if err := rewriteExistingStatusFileProtectionDisabled(); err != nil {
		logging.Warning("API: Failed to rewrite existing status file after shutdown: %v", err)
	}
}

func rewriteExistingStatusFileProtectionDisabled() error {
	workspaceDir := strings.TrimSpace(core.GetPathManager().GetWorkspaceDir())
	if workspaceDir == "" {
		return nil
	}

	statusFile := filepath.Join(workspaceDir, exportDirName, statusFileName)
	if _, err := os.Stat(statusFile); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	raw, err := os.ReadFile(statusFile)
	if err != nil {
		return err
	}

	var status map[string]interface{}
	if err := json.Unmarshal(raw, &status); err != nil {
		return err
	}

	botInfoRaw, ok := status["botInfo"]
	if !ok {
		return nil
	}
	botInfo, ok := botInfoRaw.([]interface{})
	if !ok {
		return nil
	}

	refreshedAssets := statusRewriteAssetRefresher()
	assetsByID := make(map[string]core.Asset, len(refreshedAssets))
	for _, asset := range refreshedAssets {
		assetID := strings.TrimSpace(asset.ID)
		if assetID == "" {
			continue
		}
		assetsByID[assetID] = asset
	}

	for _, entry := range botInfo {
		bot, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		bot["protection"] = "disabled"

		botID, _ := bot["id"].(string)
		botID = strings.TrimSpace(botID)
		if botID == "" {
			continue
		}
		if asset, exists := assetsByID[botID]; exists {
			bot["pid"] = resolveBotPID(asset)
		}
	}

	updated, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}

	return atomicWriteFile(statusFile, updated, 0644)
}

func parseAppShutdownRequest(r *http.Request) (appShutdownRequest, error) {
	var req appShutdownRequest

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return req, errors.New("failed to read request body")
	}
	defer r.Body.Close()

	if len(strings.TrimSpace(string(body))) == 0 {
		return req, errors.New("missing required field: restoreConfig")
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return req, errors.New("invalid JSON: " + err.Error())
	}

	if req.RestoreConfig == nil {
		return req, errors.New("missing required field: restoreConfig")
	}

	return req, nil
}
