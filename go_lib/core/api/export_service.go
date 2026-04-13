package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go_lib/core"
	"go_lib/core/logging"
	"go_lib/core/repository"
	"go_lib/core/service"
)

const (
	exportDirName        = "export"
	statusFileName       = "status.json"
	auditFileName        = "audit.jsonl"
	eventsFileName       = "events.jsonl"
	statusRefreshSeconds = 30
)

// ExportServiceImpl implements the ExportService interface for data export.
type ExportServiceImpl struct {
	mu        sync.Mutex
	running   bool
	exportDir string

	// Status file
	statusFile string
	// Audit log file
	auditFile string
	// Events file
	eventsFile string

	// File write locks
	auditFileMu  sync.Mutex
	eventsFileMu sync.Mutex

	// Background goroutine control
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewExportService creates a new ExportServiceImpl instance.
func NewExportService() *ExportServiceImpl {
	return &ExportServiceImpl{}
}

// Start initializes and starts the export service.
func (s *ExportServiceImpl) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	// Get export directory path
	pm := core.GetPathManager()
	workspaceDir := pm.GetWorkspaceDir()
	if workspaceDir == "" {
		return &exportError{"PathManager not initialized, cannot determine export directory"}
	}

	s.exportDir = filepath.Join(workspaceDir, exportDirName)
	if err := os.MkdirAll(s.exportDir, 0755); err != nil {
		return &exportError{"failed to create export directory: " + err.Error()}
	}

	s.statusFile = filepath.Join(s.exportDir, statusFileName)
	s.auditFile = filepath.Join(s.exportDir, auditFileName)
	s.eventsFile = filepath.Join(s.exportDir, eventsFileName)

	s.stopChan = make(chan struct{})
	s.running = true

	// Start background status refresh goroutine
	s.wg.Add(1)
	go s.statusRefreshLoop()

	logging.Info("export service starting, exportDir=%s", s.exportDir)
	return nil
}

// Stop gracefully stops the export service.
func (s *ExportServiceImpl) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	close(s.stopChan)
	s.wg.Wait()

	s.running = false
	logging.Info("Export service stopped")
	return nil
}

// IsRunning returns whether the export service is running.
func (s *ExportServiceImpl) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// GetExportDir returns the export directory path.
func (s *ExportServiceImpl) GetExportDir() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exportDir
}

// ExportStatus returns the current export status info.
func (s *ExportServiceImpl) ExportStatus() ExportStatusInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ExportStatusInfo{
		Enabled:    s.running,
		ExportDir:  s.exportDir,
		StatusFile: statusFileName,
		AuditFile:  auditFileName,
		EventsFile: eventsFileName,
	}
}

// ExportStatusInfo contains export status details.
type ExportStatusInfo struct {
	Enabled    bool   `json:"enabled"`
	ExportDir  string `json:"exportDir"`
	StatusFile string `json:"statusFile"`
	AuditFile  string `json:"auditFile"`
	EventsFile string `json:"eventsFile"`
}

// WriteAuditLog appends an audit log entry to the audit.jsonl file.
func (s *ExportServiceImpl) WriteAuditLog(log *AuditLogEntry) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return &exportError{"export service not running"}
	}
	auditFile := s.auditFile
	s.mu.Unlock()

	data, err := json.Marshal(log)
	if err != nil {
		return &exportError{"failed to marshal audit log: " + err.Error()}
	}

	s.auditFileMu.Lock()
	defer s.auditFileMu.Unlock()

	f, err := os.OpenFile(auditFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return &exportError{"failed to open audit file: " + err.Error()}
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return &exportError{"failed to write audit log: " + err.Error()}
	}

	return nil
}

// WriteSecurityEvent appends a security event to the events.jsonl file.
func (s *ExportServiceImpl) WriteSecurityEvent(event *SecurityEventEntry) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return &exportError{"export service not running"}
	}
	eventsFile := s.eventsFile
	s.mu.Unlock()

	data, err := json.Marshal(event)
	if err != nil {
		return &exportError{"failed to marshal security event: " + err.Error()}
	}

	s.eventsFileMu.Lock()
	defer s.eventsFileMu.Unlock()

	f, err := os.OpenFile(eventsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return &exportError{"failed to open events file: " + err.Error()}
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return &exportError{"failed to write security event: " + err.Error()}
	}

	return nil
}

// statusRefreshLoop periodically writes status.json
func (s *ExportServiceImpl) statusRefreshLoop() {
	defer s.wg.Done()

	// Write initial status
	s.writeStatusFile()

	ticker := time.NewTicker(statusRefreshSeconds * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.writeStatusFile()
		}
	}
}

// writeStatusFile generates and writes the status.json file.
func (s *ExportServiceImpl) writeStatusFile() {
	s.mu.Lock()
	statusFile := s.statusFile
	running := s.running
	s.mu.Unlock()

	if !running || statusFile == "" {
		return
	}

	status := s.buildStatus()
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		logging.Warning("Export: failed to marshal status: %v", err)
		return
	}

	if err := atomicWriteFile(statusFile, data, 0644); err != nil {
		logging.Warning("Export: failed to write status file: %v", err)
	}
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".status-*.tmp")
	if err != nil {
		return err
	}

	tmpPath := tmpFile.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(perm); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	_ = os.Remove(path)
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	cleanupTmp = false
	return nil
}

// buildStatus collects all status information.
func (s *ExportServiceImpl) buildStatus() *StatusData {
	assets := s.getLatestAssets()

	status := &StatusData{
		Timestamp: time.Now().UnixMilli(),
	}

	// Get bot info from scan results and protection configs
	status.BotInfo = s.collectBotInfoFromAssets(assets)

	// Get risk info from latest scan
	status.RiskInfo = s.collectRiskInfo(assets)

	// Get skill results from skill scans
	status.SkillResult = s.collectSkillResults(assets)

	// Get security model config
	status.SecurityModel = s.collectSecurityModel()

	return ensureStatusDataShape(status)
}

// collectBotInfo collects bot information from assets and protection configs.
func (s *ExportServiceImpl) getLatestAssets() []core.Asset {
	scanResult := service.GetLatestScanResult()
	if scanResult["success"] != true {
		return []core.Asset{}
	}

	scanData, _ := scanResult["data"].(*repository.ScanRecord)
	if scanData == nil || scanData.Assets == nil {
		return []core.Asset{}
	}

	return scanData.Assets
}

// collectBotInfo collects bot information from assets and protection configs.
func (s *ExportServiceImpl) collectBotInfoFromAssets(assets []core.Asset) []BotInfo {
	botInfos := make([]BotInfo, 0, len(assets))

	for _, asset := range assets {
		info := BotInfo{
			Name:       asset.Name,
			ID:         asset.ID,
			PID:        resolveBotPID(asset),
			Image:      resolveBotImage(asset),
			Conf:       strings.TrimSpace(asset.Metadata["config_path"]),
			Bind:       resolveBotBind(asset),
			Protection: "disabled",
			BotModel:   &BotModelInfo{},
			Metrics:    &MetricsInfo{},
		}

		// Get protection config and statistics
		protRepo := repository.NewProtectionRepository(nil)
		config, err := protRepo.GetProtectionConfig(asset.SourcePlugin, asset.ID)
		if err == nil && config != nil {
			if config.Enabled {
				if config.AuditOnly {
					info.Protection = "bypass"
				} else {
					info.Protection = "enabled"
				}
			} else {
				info.Protection = "disabled"
			}

			// Bot model config
			if config.BotModelConfig != nil {
				info.BotModel = &BotModelInfo{
					Provider: config.BotModelConfig.Provider,
					ID:       config.BotModelConfig.Model,
					URL:      config.BotModelConfig.BaseURL,
					Key:      config.BotModelConfig.APIKey,
				}
			}
		}

		// Get statistics
		stats, err := protRepo.GetProtectionStatistics(asset.SourcePlugin, asset.ID)
		if err == nil && stats != nil {
			info.Metrics = &MetricsInfo{
				AnalysisCount:         stats.AnalysisCount,
				MessageCount:          stats.MessageCount,
				WarningCount:          stats.WarningCount,
				BlockCount:            stats.BlockedCount,
				TotalToken:            stats.TotalTokens,
				InputToken:            stats.TotalPromptTokens,
				OutputToken:           stats.TotalCompletionTokens,
				ProtectionTotalToken:  stats.AuditTokens,
				ProtectionInputToken:  stats.AuditPromptTokens,
				ProtectionOutputToken: stats.AuditCompletionTokens,
				ToolCallCount:         stats.TotalToolCalls,
			}
		}

		botInfos = append(botInfos, info)
	}

	return botInfos
}

// collectRiskInfo collects risk information from latest scan.
func (s *ExportServiceImpl) collectRiskInfo(assets []core.Asset) []RiskInfo {
	riskInfos := make([]RiskInfo, 0)

	scanResult := service.GetLatestScanResult()
	if scanResult["success"] != true {
		return riskInfos
	}

	scanData, _ := scanResult["data"].(*repository.ScanRecord)
	if scanData == nil {
		return riskInfos
	}

	for _, risk := range scanData.Risks {
		if isSkillContentRisk(risk) {
			continue
		}

		info := RiskInfo{
			Name:   risk.Title,
			Level:  risk.Level,
			Source: risk.SourcePlugin,
		}

		info.BotID = resolveRiskBotID(risk, assets)

		// Convert mitigation to export format
		if risk.Mitigation != nil {
			for _, sg := range risk.Mitigation.Suggestions {
				for _, item := range sg.Items {
					info.Mitigation = append(info.Mitigation, MitigationInfo{
						Desc:    item.Action + ": " + item.Detail,
						Command: item.Command,
					})
				}
			}
		}
		if info.Mitigation == nil {
			info.Mitigation = []MitigationInfo{}
		}

		riskInfos = append(riskInfos, info)
	}

	return riskInfos
}

// collectSkillResults collects skill scan results.
func (s *ExportServiceImpl) collectSkillResults(assets []core.Asset) []SkillResultInfo {
	skillResults := make([]SkillResultInfo, 0)

	result := service.GetRiskySkills()
	if result["success"] != true {
		return skillResults
	}

	skills, ok := result["data"].([]repository.SkillScanRecord)
	if !ok {
		return skillResults
	}

	workspaceDir := core.GetPathManager().GetWorkspaceDir()
	seen := make(map[string]struct{})

	for _, skill := range skills {
		entries := expandSkillResultEntries(skill, assets, workspaceDir)
		if len(entries) == 0 {
			entries = []SkillResultInfo{{
				Name:   skill.SkillName,
				Level:  skill.RiskLevel,
				Source: fallbackSkillSource(skill, workspaceDir),
				BotID:  strings.TrimSpace(skill.AssetID),
			}}
		}

		for _, entry := range entries {
			for _, issue := range skill.Issues {
				entry.Issue = append(entry.Issue, toSkillIssue(issue))
			}
			if entry.Issue == nil {
				entry.Issue = []SkillIssue{}
			}

			key := strings.ToLower(strings.TrimSpace(entry.BotID) + "|" + strings.TrimSpace(entry.Source) + "|" + strings.TrimSpace(entry.Name))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			skillResults = append(skillResults, entry)
		}
	}

	return skillResults
}

// collectSecurityModel collects security model configuration.
func (s *ExportServiceImpl) collectSecurityModel() *SecurityModelInfo {
	result := service.GetSecurityModelConfig()
	if result["success"] != true {
		return &SecurityModelInfo{}
	}

	config, ok := result["data"].(*repository.SecurityModelConfig)
	if !ok || config == nil {
		return &SecurityModelInfo{}
	}

	return &SecurityModelInfo{
		Provider: config.Provider,
		ID:       config.Model,
		URL:      config.Endpoint,
		Key:      config.APIKey,
	}
}

func (s *ExportServiceImpl) getPrimaryBotID() string {
	assets := s.getLatestAssets()
	if len(assets) == 0 {
		return ""
	}
	return assets[0].ID
}

func resolveRiskBotID(risk core.Risk, assets []core.Asset) string {
	if botID, ok := risk.Args["asset_id"].(string); ok && strings.TrimSpace(botID) != "" {
		return strings.TrimSpace(botID)
	}

	pluginAssets := filterAssetsByPlugin(assets, risk.SourcePlugin)
	if len(pluginAssets) == 1 {
		return pluginAssets[0].ID
	}

	matched := findAssetsMatchingRisk(risk, pluginAssets)
	if len(matched) == 1 {
		return matched[0].ID
	}

	return ""
}

func findAssetsMatchingRisk(risk core.Risk, assets []core.Asset) []core.Asset {
	var matches []core.Asset
	for _, asset := range assets {
		if assetMatchesRisk(asset, risk) {
			matches = append(matches, asset)
		}
	}
	return matches
}

func assetMatchesRisk(asset core.Asset, risk core.Risk) bool {
	if asset.ID == "" {
		return false
	}

	if configPath, ok := stringArg(risk.Args, "config_path"); ok {
		if samePath(asset.Metadata["config_path"], configPath) {
			return true
		}
	}
	if pathValue, ok := stringArg(risk.Args, "path"); ok {
		if samePath(asset.Metadata["config_path"], pathValue) || pathBelongsToAsset(asset, pathValue) {
			return true
		}
	}
	if bind, ok := stringArg(risk.Args, "bind"); ok && strings.TrimSpace(asset.Metadata["bind"]) == strings.TrimSpace(bind) {
		return true
	}

	return false
}

func expandSkillResultEntries(skill repository.SkillScanRecord, assets []core.Asset, workspaceDir string) []SkillResultInfo {
	entries := make([]SkillResultInfo, 0)
	pluginAssets := filterAssetsByPlugin(assets, skill.SourcePlugin)
	if len(pluginAssets) == 0 {
		pluginAssets = assets
	}

	if strings.TrimSpace(skill.AssetID) != "" {
		for _, asset := range pluginAssets {
			if asset.ID != skill.AssetID {
				continue
			}
			source := firstNonEmpty(strings.TrimSpace(skill.SkillPath), detectInstalledSkillPath(asset, skill))
			if source == "" {
				source = fallbackSkillSource(skill, workspaceDir)
			}
			entries = append(entries, SkillResultInfo{
				Name:   skill.SkillName,
				Level:  skill.RiskLevel,
				Source: source,
				BotID:  asset.ID,
			})
			return entries
		}
	}

	for _, asset := range pluginAssets {
		source := detectInstalledSkillPath(asset, skill)
		if source == "" {
			continue
		}
		entries = append(entries, SkillResultInfo{
			Name:   skill.SkillName,
			Level:  skill.RiskLevel,
			Source: source,
			BotID:  asset.ID,
		})
	}

	if len(entries) == 0 && strings.TrimSpace(skill.SkillPath) != "" {
		entries = append(entries, SkillResultInfo{
			Name:   skill.SkillName,
			Level:  skill.RiskLevel,
			Source: skill.SkillPath,
			BotID:  strings.TrimSpace(skill.AssetID),
		})
	}

	return entries
}

func detectInstalledSkillPath(asset core.Asset, skill repository.SkillScanRecord) string {
	candidates := candidateSkillPathsForAsset(asset, skill.SkillName)
	for _, candidate := range candidates {
		if strings.TrimSpace(skill.SkillPath) != "" && samePath(candidate, skill.SkillPath) {
			return candidate
		}
		if dirExists(candidate) {
			return candidate
		}
	}
	return ""
}

func candidateSkillPathsForAsset(asset core.Asset, skillName string) []string {
	configPath := strings.TrimSpace(asset.Metadata["config_path"])
	if configPath == "" || strings.TrimSpace(skillName) == "" {
		return []string{}
	}

	configDir := filepath.Dir(configPath)
	switch strings.ToLower(strings.TrimSpace(asset.SourcePlugin)) {
	case "openclaw":
		return []string{
			filepath.Join(configDir, "skills", skillName),
			filepath.Join(configDir, "workspace", "skills", skillName),
		}
	default:
		return []string{}
	}
}

func filterAssetsByPlugin(assets []core.Asset, sourcePlugin string) []core.Asset {
	sourcePlugin = strings.ToLower(strings.TrimSpace(sourcePlugin))
	if sourcePlugin == "" {
		return assets
	}

	filtered := make([]core.Asset, 0)
	for _, asset := range assets {
		if strings.ToLower(strings.TrimSpace(asset.SourcePlugin)) == sourcePlugin {
			filtered = append(filtered, asset)
		}
	}
	return filtered
}

func fallbackSkillSource(skill repository.SkillScanRecord, workspaceDir string) string {
	if strings.TrimSpace(skill.SkillPath) != "" {
		return skill.SkillPath
	}
	return filepath.Join(workspaceDir, "skills", skill.SkillName)
}

func pathBelongsToAsset(asset core.Asset, target string) bool {
	configPath := strings.TrimSpace(asset.Metadata["config_path"])
	if configPath == "" || strings.TrimSpace(target) == "" {
		return false
	}

	configDir := filepath.Dir(configPath)
	absConfigDir, err := filepath.Abs(configDir)
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	return absTarget == absConfigDir || strings.HasPrefix(absTarget, absConfigDir+string(filepath.Separator))
}

func samePath(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}

	absLeft, errLeft := filepath.Abs(left)
	absRight, errRight := filepath.Abs(right)
	if errLeft == nil && errRight == nil {
		return strings.EqualFold(filepath.Clean(absLeft), filepath.Clean(absRight))
	}
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func stringArg(args map[string]interface{}, key string) (string, bool) {
	if args == nil {
		return "", false
	}
	value, ok := args[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func toSkillIssue(raw string) SkillIssue {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SkillIssue{Type: "security_risk", Desc: "", Evidence: ""}
	}

	var structured struct {
		Type        string `json:"type"`
		Desc        string `json:"desc"`
		Description string `json:"description"`
		Evidence    string `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(raw), &structured); err == nil {
		desc := strings.TrimSpace(structured.Desc)
		if desc == "" {
			desc = strings.TrimSpace(structured.Description)
		}
		if structured.Type != "" || desc != "" || structured.Evidence != "" {
			if structured.Type == "" {
				structured.Type = "security_risk"
			}
			return SkillIssue{
				Type:     structured.Type,
				Desc:     desc,
				Evidence: structured.Evidence,
			}
		}
	}

	return SkillIssue{
		Type:     "security_risk",
		Desc:     raw,
		Evidence: "",
	}
}

func isSkillContentRisk(risk core.Risk) bool {
	return strings.EqualFold(strings.TrimSpace(risk.ID), "riskSkillSecurityIssue")
}

// ========== Data Types for Export ==========

// StatusData represents the status.json structure.
type StatusData struct {
	BotInfo       []BotInfo          `json:"botInfo"`
	RiskInfo      []RiskInfo         `json:"riskInfo"`
	SkillResult   []SkillResultInfo  `json:"skillResult"`
	SecurityModel *SecurityModelInfo `json:"securityModel"`
	Timestamp     int64              `json:"timestamp"`
}

// BotInfo represents a bot's status information.
type BotInfo struct {
	Name       string        `json:"name"`
	ID         string        `json:"id"`
	PID        string        `json:"pid"`
	Image      string        `json:"image"`
	Conf       string        `json:"conf"`
	Bind       string        `json:"bind"`
	Protection string        `json:"protection"`
	BotModel   *BotModelInfo `json:"botModel"`
	Metrics    *MetricsInfo  `json:"metrics"`
}

// BotModelInfo represents bot model configuration.
type BotModelInfo struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	URL      string `json:"url"`
	Key      string `json:"key"`
}

// MetricsInfo represents protection metrics.
type MetricsInfo struct {
	AnalysisCount         int `json:"analysisCount"`
	MessageCount          int `json:"messageCount"`
	WarningCount          int `json:"warningCount"`
	BlockCount            int `json:"blockCount"`
	TotalToken            int `json:"totalToken"`
	InputToken            int `json:"inputToken"`
	OutputToken           int `json:"outputToken"`
	ProtectionTotalToken  int `json:"protectionTotalToken"`
	ProtectionInputToken  int `json:"protectionInputToken"`
	ProtectionOutputToken int `json:"protectionOutputToken"`
	ToolCallCount         int `json:"toolCallCount"`
}

// RiskInfo represents a risk entry.
type RiskInfo struct {
	Name       string           `json:"name"`
	Level      string           `json:"level"`
	Source     string           `json:"source"`
	BotID      string           `json:"botId"`
	Mitigation []MitigationInfo `json:"mitigation"`
}

// MitigationInfo represents a mitigation suggestion.
type MitigationInfo struct {
	Desc    string `json:"desc"`
	Command string `json:"command"`
}

// SkillResultInfo represents a skill scan result.
type SkillResultInfo struct {
	Name   string       `json:"name"`
	Level  string       `json:"level"`
	Source string       `json:"source"`
	BotID  string       `json:"botId"`
	Issue  []SkillIssue `json:"issue"`
}

// SkillIssue represents a skill security issue.
type SkillIssue struct {
	Type     string `json:"type"`
	Desc     string `json:"desc"`
	Evidence string `json:"evidence"`
}

// SecurityModelInfo represents security model configuration.
type SecurityModelInfo struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	URL      string `json:"url"`
	Key      string `json:"key"`
}

// AuditLogEntry represents an audit log entry for export.
type AuditLogEntry struct {
	BotID         string     `json:"botId"`
	LogID         string     `json:"logId"`
	LogTimestamp  string     `json:"logTimestamp"`
	RequestID     string     `json:"requestId"`
	Model         string     `json:"model"`
	Action        string     `json:"action"`
	RiskLevel     string     `json:"riskLevel"`
	RiskCauses    string     `json:"riskCauses"`
	DurationMs    int        `json:"durationMs"`
	TokenCount    int        `json:"tokenCount"`
	UserRequest   string     `json:"userRequest"`
	ToolCallCount int        `json:"toolCallCount"`
	ToolCalls     []ToolCall `json:"toolCalls,omitempty"`
}

// ToolCall represents a tool call in audit log.
type ToolCall struct {
	Tool       string `json:"tool"`
	Parameters string `json:"parameters"`
	Result     string `json:"result"`
}

// SecurityEventEntry represents a security event for export.
type SecurityEventEntry struct {
	BotID      string `json:"botId"`
	EventID    string `json:"eventId"`
	Timestamp  string `json:"timestamp"`
	EventType  string `json:"event_type"`
	ActionDesc string `json:"action_desc"`
	RiskType   string `json:"risk_type"`
	Detail     string `json:"detail"`
	Source     string `json:"source"`
}

// exportError is a simple error type for export operations.
type exportError struct {
	msg string
}

func (e *exportError) Error() string {
	return e.msg
}

func ensureStatusDataShape(status *StatusData) *StatusData {
	if status == nil {
		return &StatusData{
			BotInfo:       []BotInfo{},
			RiskInfo:      []RiskInfo{},
			SkillResult:   []SkillResultInfo{},
			SecurityModel: &SecurityModelInfo{},
		}
	}

	if status.BotInfo == nil {
		status.BotInfo = []BotInfo{}
	}
	if status.RiskInfo == nil {
		status.RiskInfo = []RiskInfo{}
	}
	if status.SkillResult == nil {
		status.SkillResult = []SkillResultInfo{}
	}
	if status.SecurityModel == nil {
		status.SecurityModel = &SecurityModelInfo{}
	}

	for i := range status.BotInfo {
		if status.BotInfo[i].BotModel == nil {
			status.BotInfo[i].BotModel = &BotModelInfo{}
		}
		if status.BotInfo[i].Metrics == nil {
			status.BotInfo[i].Metrics = &MetricsInfo{}
		}
	}

	for i := range status.RiskInfo {
		if status.RiskInfo[i].Mitigation == nil {
			status.RiskInfo[i].Mitigation = []MitigationInfo{}
		}
	}

	for i := range status.SkillResult {
		if status.SkillResult[i].Issue == nil {
			status.SkillResult[i].Issue = []SkillIssue{}
		}
	}

	return status
}

func resolveBotPID(asset core.Asset) string {
	for _, key := range []string{"pid", "managed_pid"} {
		if value := strings.TrimSpace(asset.Metadata[key]); value != "" {
			if first := firstCSVValue(value); first != "" {
				return first
			}
		}
	}

	if value, ok := findDisplayItemValue(asset, "PID"); ok {
		first := firstCSVValue(value)
		if first != "" {
			if _, err := strconv.Atoi(first); err == nil {
				return first
			}
		}
	}

	return "N/A"
}

func resolveBotImage(asset core.Asset) string {
	if len(asset.ProcessPaths) > 0 {
		if value := strings.TrimSpace(asset.ProcessPaths[0]); value != "" {
			return value
		}
	}

	if value, ok := findDisplayItemValue(asset, "Image Path"); ok {
		if first := firstCSVValue(value); first != "" {
			return first
		}
	}

	if value, ok := findDisplayItemValue(asset, "Process Name"); ok {
		if first := firstCSVValue(value); first != "" {
			return first
		}
	}

	return "N/A"
}

func resolveBotBind(asset core.Asset) string {
	if bind, ok := findDisplayItemValue(asset, "Bind"); ok {
		if port, ok := findDisplayItemValue(asset, "Port"); ok {
			return joinHostPort(bind, port)
		}
		if first := firstCSVValue(bind); first != "" {
			return first
		}
	}

	if host, ok := findDisplayItemValue(asset, "Host"); ok {
		if port, ok := findDisplayItemValue(asset, "Port"); ok {
			return joinHostPort(host, port)
		}
		if first := firstCSVValue(host); first != "" {
			return first
		}
	}

	if value, ok := findDisplayItemValue(asset, "Listener Address"); ok {
		if first := firstCSVValue(value); first != "" {
			return first
		}
	}

	return "N/A"
}

func findDisplayItemValue(asset core.Asset, label string) (string, bool) {
	target := strings.TrimSpace(strings.ToLower(label))
	if target == "" {
		return "", false
	}

	for _, section := range asset.DisplaySections {
		for _, item := range section.Items {
			if strings.TrimSpace(strings.ToLower(item.Label)) != target {
				continue
			}
			value := strings.TrimSpace(item.Value)
			if value == "" || strings.EqualFold(value, "N/A") {
				return "", false
			}
			return value, true
		}
	}

	return "", false
}

func firstCSVValue(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || strings.EqualFold(value, "N/A") {
		return ""
	}

	if idx := strings.Index(value, ","); idx >= 0 {
		value = value[:idx]
	}

	return strings.TrimSpace(value)
}

func joinHostPort(host, port string) string {
	host = firstCSVValue(host)
	port = firstCSVValue(port)
	if host == "" {
		return "N/A"
	}
	if port == "" || strings.Contains(host, ":") {
		return host
	}
	return host + ":" + port
}
