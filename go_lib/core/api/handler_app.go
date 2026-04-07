package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"go_lib/core/logging"
)

type appShutdownRequest struct {
	RestoreConfig *bool `json:"restoreConfig,omitempty"`
}

// handleAppShutdown handles POST /api/v1/app/shutdown.
func (s *APIServer) handleAppShutdown(w http.ResponseWriter, r *http.Request) {
	req, err := parseAppShutdownRequest(r)
	if err != nil {
		Error(w, http.StatusBadRequest, CodeInvalidParam, err.Error())
		return
	}

	options := AppShutdownOptions{}
	if req.RestoreConfig != nil {
		options.RestoreConfig = *req.RestoreConfig
	}

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

func parseAppShutdownRequest(r *http.Request) (appShutdownRequest, error) {
	var req appShutdownRequest

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return req, errors.New("failed to read request body")
	}
	defer r.Body.Close()

	if len(strings.TrimSpace(string(body))) == 0 {
		return req, nil
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return req, errors.New("invalid JSON: " + err.Error())
	}

	return req, nil
}
