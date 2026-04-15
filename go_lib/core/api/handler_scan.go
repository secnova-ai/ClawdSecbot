package api

import (
	"net/http"
	"time"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/repository"
	"go_lib/core/service"
)

// ScanResponse represents the response for a scan request.
type ScanResponse struct {
	Message       string             `json:"message"`
	ScanTime      string             `json:"scanTime"`
	BotInfo       []BotInfo          `json:"botInfo"`
	RiskInfo      []RiskInfo         `json:"riskInfo"`
	SkillResult   []SkillResultInfo  `json:"skillResult"`
	SecurityModel *SecurityModelInfo `json:"securityModel"`
	Timestamp     int64              `json:"timestamp"`
}

// handleScan handles POST /api/v1/scan
// Triggers a synchronous security scan and returns the results.
func (s *APIServer) handleScan(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	logging.Info("API: Starting security scan")

	// Step 1: Scan all assets
	pm := core.GetPluginManager()
	assets, err := pm.ScanAllAssets()
	if err != nil {
		logging.Error("API: Asset scan failed: %v", err)
		Error(w, http.StatusInternalServerError, CodeInternalError, "asset scan failed: "+err.Error())
		return
	}
	logging.Info("API: Asset scan completed, found %d assets", len(assets))

	// Step 2: Get scanned skill hashes for risk assessment
	hashResult := service.GetScannedSkillHashes()
	var scannedHashes map[string]bool
	if hashResult["success"] == true {
		if hashList, ok := hashResult["data"].([]string); ok {
			scannedHashes = make(map[string]bool)
			for _, h := range hashList {
				scannedHashes[h] = true
			}
		}
	}
	if scannedHashes == nil {
		scannedHashes = make(map[string]bool)
	}

	// Step 3: Assess all risks
	risks, err := pm.AssessAllRisks(scannedHashes)
	if err != nil {
		logging.Error("API: Risk assessment failed: %v", err)
		Error(w, http.StatusInternalServerError, CodeInternalError, "risk assessment failed: "+err.Error())
		return
	}
	logging.Info("API: Risk assessment completed, found %d risks", len(risks))

	// Ensure arrays are not nil for JSON serialization
	if assets == nil {
		assets = []core.Asset{}
	}
	if risks == nil {
		risks = []core.Risk{}
	}

	// Step 4: Persist scan result so export/status.json can read latest data.
	scanRepo := repository.NewScanRepository(nil)
	if err := scanRepo.SaveScanResult(&repository.ScanRecord{
		Assets: assets,
		Risks:  risks,
	}); err != nil {
		logging.Error("API: Failed to save scan result: %v", err)
		Error(w, http.StatusInternalServerError, CodeInternalError, "save scan result failed: "+err.Error())
		return
	}

	// Step 5: Materialize the default protection policy onto newly discovered
	// assets so later protection/runtime flows operate on concrete bot IDs.
	protectionRepo := repository.NewProtectionRepository(nil)
	if err := applyDefaultProtectionPolicyToAssets(protectionRepo, assets); err != nil {
		logging.Error("API: Failed to apply default protection policy after scan: %v", err)
		Error(w, http.StatusInternalServerError, CodeInternalError, "apply default protection policy failed: "+err.Error())
		return
	}

	// Step 6: If export is running, write status.json immediately.
	s.mu.Lock()
	exportService := s.exportService
	s.mu.Unlock()
	if impl, ok := exportService.(*ExportServiceImpl); ok && impl.IsRunning() {
		impl.writeStatusFile()
	}

	// Calculate scan duration
	scanDuration := time.Since(startTime)
	scanTime := startTime.Format(time.RFC3339)

	logging.Info("API: Security scan completed in %v, assets=%d, risks=%d", scanDuration, len(assets), len(risks))

	status := (&ExportServiceImpl{}).buildStatus()
	Success(w, ScanResponse{
		Message:       "scan completed",
		ScanTime:      scanTime,
		BotInfo:       status.BotInfo,
		RiskInfo:      status.RiskInfo,
		SkillResult:   status.SkillResult,
		SecurityModel: status.SecurityModel,
		Timestamp:     status.Timestamp,
	})
}
