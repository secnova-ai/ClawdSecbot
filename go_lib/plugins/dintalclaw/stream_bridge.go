package dintalclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go_lib/core/logging"
	"go_lib/core/repository"

	"github.com/cloudwego/eino/schema"
)

// sendLog sends a log message to a channel without blocking
func sendLog(ch chan string, msg string) {
	select {
	case ch <- msg:
	default:
	}
}

// ScanSession represents an active skill scan session
type ScanSession struct {
	ScanID    string
	SkillPath string
	SkillName string
	Agent     *SkillSecurityAnalyzer
	Ctx       context.Context
	Cancel    context.CancelFunc
	LogChan   chan string
	Logs      []string
	LogMu     sync.Mutex
	Result    *SkillAnalysisResult
	Completed bool
	Error     error
	Done      chan struct{}
}

var (
	activeSessionsMu sync.RWMutex
	activeSessions   = make(map[string]*ScanSession)
	sessionCounter   int64
)

func generateScanID() string {
	sessionCounter++
	return fmt.Sprintf("scan_%d_%d", time.Now().UnixNano(), sessionCounter)
}

func registerSession(session *ScanSession) {
	activeSessionsMu.Lock()
	defer activeSessionsMu.Unlock()
	activeSessions[session.ScanID] = session
}

func getSession(scanID string) (*ScanSession, bool) {
	activeSessionsMu.RLock()
	defer activeSessionsMu.RUnlock()
	session, ok := activeSessions[scanID]
	return session, ok
}

func removeSession(scanID string) {
	activeSessionsMu.Lock()
	defer activeSessionsMu.Unlock()
	delete(activeSessions, scanID)
}

func (s *ScanSession) collectLogs() {
	for {
		select {
		case log, ok := <-s.LogChan:
			if !ok {
				return
			}
			s.LogMu.Lock()
			s.Logs = append(s.Logs, log)
			s.LogMu.Unlock()
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
				default:
					return
				}
			}
		}
	}
}

func (s *ScanSession) getAndClearLogs() []string {
	s.LogMu.Lock()
	defer s.LogMu.Unlock()
	logs := s.Logs
	s.Logs = nil
	return logs
}

type StartScanResponse struct {
	Success bool   `json:"success"`
	ScanID  string `json:"scan_id,omitempty"`
	Error   string `json:"error,omitempty"`
}

type ScanLogResponse struct {
	ScanID    string   `json:"scan_id"`
	Logs      []string `json:"logs"`
	Completed bool     `json:"completed"`
	Error     string   `json:"error,omitempty"`
}

type ScanResultResponse struct {
	Success   bool                 `json:"success"`
	Result    *SkillAnalysisResult `json:"result,omitempty"`
	SkillName string               `json:"skill_name,omitempty"`
	SkillHash string               `json:"skill_hash,omitempty"`
	Error     string               `json:"error,omitempty"`
}

type TestConnectionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// StartSkillSecurityScanInternal 启动 Skill 安全分析扫描
func StartSkillSecurityScanInternal(skillPath, modelConfigJSON string) string {
	var config repository.SecurityModelConfig
	if err := json.Unmarshal([]byte(modelConfigJSON), &config); err != nil {
		return toJSONString(StartScanResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid security model config: %v", err),
		})
	}

	agent, err := NewSkillSecurityAnalyzer(&config)
	if err != nil {
		return toJSONString(StartScanResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to create agent: %v", err),
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	session := &ScanSession{
		ScanID:    generateScanID(),
		SkillPath: skillPath,
		SkillName: filepath.Base(skillPath),
		Agent:     agent,
		Ctx:       ctx,
		Cancel:    cancel,
		LogChan:   make(chan string, 1000),
		Done:      make(chan struct{}),
	}

	registerSession(session)
	go session.collectLogs()

	go func() {
		defer close(session.Done)
		defer close(session.LogChan)
		defer agent.Close()

		result, err := agent.AnalyzeSkillStream(ctx, skillPath, session.LogChan)
		if err != nil {
			session.Error = err
		} else {
			session.Result = result
		}
		session.Completed = true
	}()

	return toJSONString(StartScanResponse{
		Success: true,
		ScanID:  session.ScanID,
	})
}

// GetSkillSecurityScanLogInternal 获取安全分析扫描日志
func GetSkillSecurityScanLogInternal(scanID string) string {
	session, ok := getSession(scanID)
	if !ok {
		return toJSONString(ScanLogResponse{
			ScanID:    scanID,
			Logs:      []string{},
			Completed: true,
			Error:     "scan session not found",
		})
	}

	logs := session.getAndClearLogs()

	select {
	case <-session.Done:
		errMsg := ""
		if session.Error != nil {
			errMsg = session.Error.Error()
		}
		return toJSONString(ScanLogResponse{
			ScanID:    scanID,
			Logs:      logs,
			Completed: true,
			Error:     errMsg,
		})
	default:
		return toJSONString(ScanLogResponse{
			ScanID:    scanID,
			Logs:      logs,
			Completed: false,
		})
	}
}

// GetSkillSecurityScanResultInternal 获取安全分析扫描结果
func GetSkillSecurityScanResultInternal(scanID string) string {
	session, ok := getSession(scanID)
	if !ok {
		return toJSONString(ScanResultResponse{
			Success: false,
			Error:   "scan session not found",
		})
	}

	select {
	case <-session.Done:
	case <-time.After(30 * time.Second):
		return toJSONString(ScanResultResponse{
			Success: false,
			Error:   "timeout waiting for scan completion",
		})
	}

	if session.Error != nil {
		return toJSONString(ScanResultResponse{
			Success: false,
			Error:   session.Error.Error(),
		})
	}

	hash, _ := calculateSkillHash(session.SkillPath)
	removeSession(scanID)

	return toJSONString(ScanResultResponse{
		Success:   true,
		Result:    session.Result,
		SkillName: session.SkillName,
		SkillHash: hash,
	})
}

// CancelSkillSecurityScanInternal 取消安全分析扫描
func CancelSkillSecurityScanInternal(scanID string) string {
	session, ok := getSession(scanID)
	if !ok {
		return `{"success": false, "error": "scan session not found"}`
	}

	session.Cancel()
	removeSession(scanID)
	return `{"success": true}`
}

// TestModelConnectionInternal 测试模型连接
func TestModelConnectionInternal(configJSON string) string {
	var config repository.SecurityModelConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return toJSONString(TestConnectionResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid JSON: %v", err),
		})
	}

	if err := ValidateSecurityModelConfig(&config); err != nil {
		return toJSONString(TestConnectionResponse{
			Success: false,
			Error:   err.Error(),
		})
	}

	ctx := context.Background()

	chatModel, err := CreateChatModelFromConfig(ctx, &config)
	if err != nil {
		return toJSONString(TestConnectionResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to create model: %v", err),
		})
	}

	testMessages := []*schema.Message{
		schema.UserMessage("Hi, respond with just 'OK' to confirm connection."),
	}

	_, err = chatModel.Generate(ctx, testMessages)
	if err != nil {
		return toJSONString(TestConnectionResponse{
			Success: false,
			Error:   fmt.Sprintf("Connection test failed: %v", err),
		})
	}

	return toJSONString(TestConnectionResponse{
		Success: true,
		Message: "Connection successful",
	})
}

// DeleteSkillInternal 删除 Skill
func DeleteSkillInternal(skillPath string) string {
	if !isWithinSkillsDirs(skillPath) {
		return `{"success": false, "error": "skill path is not within skills directory"}`
	}

	if _, err := os.Stat(skillPath); err != nil {
		if os.IsNotExist(err) {
			return `{"success": true, "already_missing": true, "message": "skill path already missing"}`
		}
		return fmt.Sprintf(`{"success": false, "error": "failed to stat skill path: %v"}`, err)
	}

	if err := removeSkillDirectory(skillPath); err != nil {
		return fmt.Sprintf(`{"success": false, "error": "failed to delete skill: %v"}`, err)
	}

	return `{"success": true}`
}

func convertSkillIssuesToStrings(issues []SkillSecurityIssue) []string {
	result := make([]string, len(issues))
	for i, issue := range issues {
		result[i] = fmt.Sprintf("[%s] %s in %s: %s", issue.Severity, issue.Type, issue.File, issue.Description)
	}
	return result
}

// ==================== 批量扫描 ====================

type BatchScanStartResponse struct {
	Success bool   `json:"success"`
	BatchID string `json:"batch_id,omitempty"`
	Total   int    `json:"total"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type BatchScanLogResponse struct {
	BatchID      string   `json:"batch_id"`
	Logs         []string `json:"logs"`
	CurrentIndex int      `json:"current_index"`
	Total        int      `json:"total"`
	CurrentSkill string   `json:"current_skill"`
	Completed    bool     `json:"completed"`
	Error        string   `json:"error,omitempty"`
}

type BatchScanResultItem struct {
	SkillName string               `json:"skill_name"`
	SkillPath string               `json:"skill_path"`
	SkillHash string               `json:"skill_hash"`
	Success   bool                 `json:"success"`
	Result    *SkillAnalysisResult `json:"result,omitempty"`
	Error     string               `json:"error,omitempty"`
}

type BatchScanResultsResponse struct {
	Success bool                            `json:"success"`
	Results map[string]*BatchScanResultItem `json:"results,omitempty"`
	Error   string                          `json:"error,omitempty"`
}

type BatchScanSession struct {
	BatchID      string
	SkillPaths   map[string]string
	SkillHashes  map[string]string
	SkillOrder   []string
	CurrentIndex int
	TotalCount   int
	CurrentSkill string
	Results      map[string]*BatchScanResultItem
	LogChan      chan string
	Logs         []string
	LogMu        sync.Mutex
	Completed    bool
	Done         chan struct{}
	Ctx          context.Context
	Cancel       context.CancelFunc
}

var (
	activeBatchSessionsMu sync.RWMutex
	activeBatchSessions   = make(map[string]*BatchScanSession)
	batchSessionCounter   int64
)

func generateBatchID() string {
	count := atomic.AddInt64(&batchSessionCounter, 1)
	return fmt.Sprintf("batch_%d_%d", time.Now().UnixNano(), count)
}

func registerBatchSession(session *BatchScanSession) {
	activeBatchSessionsMu.Lock()
	defer activeBatchSessionsMu.Unlock()
	activeBatchSessions[session.BatchID] = session
}

func getBatchSession(batchID string) *BatchScanSession {
	activeBatchSessionsMu.RLock()
	defer activeBatchSessionsMu.RUnlock()
	return activeBatchSessions[batchID]
}

func removeBatchSession(batchID string) {
	activeBatchSessionsMu.Lock()
	defer activeBatchSessionsMu.Unlock()
	delete(activeBatchSessions, batchID)
}

func (bs *BatchScanSession) collectLogs() {
	for log := range bs.LogChan {
		bs.LogMu.Lock()
		bs.Logs = append(bs.Logs, log)
		bs.LogMu.Unlock()
	}
}

func (bs *BatchScanSession) getAndClearLogs() []string {
	bs.LogMu.Lock()
	defer bs.LogMu.Unlock()
	logs := bs.Logs
	bs.Logs = nil
	return logs
}

func (bs *BatchScanSession) run(config *repository.SecurityModelConfig) {
	defer close(bs.Done)
	defer close(bs.LogChan)

	scanRepo := repository.NewSkillSecurityScanRepository(nil)
	if scanRepo == nil {
		logging.Error("[BatchScan] CRITICAL: scanRepo is nil, DB not initialized")
	} else {
		logging.Info("[BatchScan] scanRepo created, DB connection OK")
	}

	for i, skillName := range bs.SkillOrder {
		select {
		case <-bs.Ctx.Done():
			logging.Info("[BatchScan] Cancelled at skill %d/%d", i+1, bs.TotalCount)
			return
		default:
		}

		bs.CurrentIndex = i
		bs.CurrentSkill = skillName
		skillPath := bs.SkillPaths[skillName]

		sendLog(bs.LogChan, fmt.Sprintf("\n========================================"))
		sendLog(bs.LogChan, fmt.Sprintf("Scanning skill: %s (%d/%d)", skillName, i+1, bs.TotalCount))
		sendLog(bs.LogChan, fmt.Sprintf("========================================\n"))

		agent, err := NewSkillSecurityAnalyzer(config)
		if err != nil {
			logging.Error("[BatchScan] Failed to create analyzer for %s: %v", skillName, err)
			hash := bs.SkillHashes[skillName]
			if hash != "" {
				if saveErr := scanRepo.SaveSkillScanResult(&repository.SkillScanRecord{
					SkillName: skillName, SkillHash: hash, SkillPath: skillPath, SourcePlugin: dintalclawPluginID, Safe: false,
					Issues: []string{err.Error()}, RiskLevel: "error",
				}); saveErr != nil {
					logging.Error("[BatchScan] Failed to save error result for %s: %v", skillName, saveErr)
				}
			}
			bs.Results[skillName] = &BatchScanResultItem{
				SkillName: skillName, SkillPath: skillPath, SkillHash: hash, Error: err.Error(),
			}
			continue
		}

		result, err := agent.AnalyzeSkillStream(bs.Ctx, skillPath, bs.LogChan)
		agent.Close()

		hash := bs.SkillHashes[skillName]

		if err != nil {
			logging.Error("[BatchScan] Analysis failed for %s: %v", skillName, err)
			if hash != "" {
				if saveErr := scanRepo.SaveSkillScanResult(&repository.SkillScanRecord{
					SkillName: skillName, SkillHash: hash, SkillPath: skillPath, SourcePlugin: dintalclawPluginID, Safe: false,
					Issues: []string{fmt.Sprintf("Analysis failed: %v", err)}, RiskLevel: "error",
				}); saveErr != nil {
					logging.Error("[BatchScan] Failed to save error result for %s: %v", skillName, saveErr)
				}
			}
			bs.Results[skillName] = &BatchScanResultItem{
				SkillName: skillName, SkillPath: skillPath, SkillHash: hash, Error: err.Error(),
			}
			continue
		}

		safe := result != nil && result.Safe
		var issues []string
		var riskLevel string
		if result != nil {
			for _, issue := range result.Issues {
				issues = append(issues, fmt.Sprintf("%s: %s", issue.Type, issue.Description))
			}
			riskLevel = result.RiskLevel
		}
		// 兜底：若被判定为风险但未返回结构化 issues，补一条摘要问题，避免前端出现“0个安全问题”。
		if !safe && len(issues) == 0 {
			if result != nil && strings.TrimSpace(result.Summary) != "" {
				issues = append(issues, result.Summary)
			} else {
				issues = append(issues, "模型判定存在安全风险，请人工复核")
			}
		}
		if err := scanRepo.SaveSkillScanResult(&repository.SkillScanRecord{
			SkillName: skillName, SkillHash: hash, SkillPath: skillPath, SourcePlugin: dintalclawPluginID, Safe: safe, Issues: issues, RiskLevel: riskLevel,
		}); err != nil {
			logging.Error("[BatchScan] Failed to save result for %s: %v", skillName, err)
		} else {
			if saved, verr := scanRepo.GetSkillScanByHash(hash); verr != nil || saved == nil {
				logging.Error("[BatchScan] Save verification FAILED for %s: hash=%s, verr=%v, found=%v", skillName, hash[:min(12, len(hash))], verr, saved != nil)
			} else {
				logging.Info("[BatchScan] Save verified for %s: id=%d, hash=%s", skillName, saved.ID, hash[:min(12, len(hash))])
			}
		}

		bs.Results[skillName] = &BatchScanResultItem{
			SkillName: skillName, SkillPath: skillPath, SkillHash: hash,
			Success: true, Result: result,
		}
		logging.Info("[BatchScan] Completed %s (%d/%d), safe=%v", skillName, i+1, bs.TotalCount, safe)
	}
	bs.Completed = true
	logging.Info("[BatchScan] All %d skills scanned", bs.TotalCount)
}

// StartBatchSkillScanInternal 零参数启动批量技能扫描
func StartBatchSkillScanInternal() string {
	skills, err := listSkills()
	if err != nil {
		logging.Error("[BatchScan] Failed to list skills: %v", err)
		return toJSONString(BatchScanStartResponse{
			Success: false, Error: fmt.Sprintf("failed to list skills: %v", err),
		})
	}
	logging.Info("[BatchScan] Discovered %d skills total", len(skills))

	scanRepo := repository.NewSkillSecurityScanRepository(nil)
	hashes, err := scanRepo.GetScannedSkillHashes()
	if err != nil {
		logging.Warning("[BatchScan] Failed to get scanned hashes: %v, treating all as unscanned", err)
	}
	hashSet := make(map[string]bool)
	for _, h := range hashes {
		hashSet[h] = true
	}
	logging.Info("[BatchScan] DB has %d scanned hashes", len(hashSet))

	skillPaths := make(map[string]string)
	skillHashes := make(map[string]string)
	var skillOrder []string
	for _, skill := range skills {
		if skill.Hash == "" {
			continue
		}
		if hashSet[skill.Hash] {
			continue
		}
		skillPaths[skill.Name] = skill.Path
		skillHashes[skill.Name] = skill.Hash
		skillOrder = append(skillOrder, skill.Name)
	}
	logging.Info("[BatchScan] %d skills need scanning after filter", len(skillOrder))
	if len(skillOrder) == 0 {
		return toJSONString(BatchScanStartResponse{Success: true, Total: 0, Message: "no skills to scan"})
	}

	configRepo := repository.NewSecurityModelConfigRepository(nil)
	modelConfig, err := configRepo.Get()
	if err != nil || modelConfig == nil {
		return toJSONString(BatchScanStartResponse{
			Success: false, Error: "security model not configured",
		})
	}
	if err := ValidateSecurityModelConfig(modelConfig); err != nil {
		return toJSONString(BatchScanStartResponse{
			Success: false, Error: fmt.Sprintf("invalid security model config: %v", err),
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	session := &BatchScanSession{
		BatchID:     generateBatchID(),
		SkillPaths:  skillPaths,
		SkillHashes: skillHashes,
		SkillOrder:  skillOrder,
		TotalCount:  len(skillOrder),
		Results:     make(map[string]*BatchScanResultItem),
		LogChan:     make(chan string, 1000),
		Done:        make(chan struct{}),
		Ctx:         ctx,
		Cancel:      cancel,
	}
	registerBatchSession(session)
	go session.collectLogs()
	go session.run(modelConfig)

	logging.Info("[BatchScan] Started batch scan %s with %d skills", session.BatchID, session.TotalCount)
	return toJSONString(BatchScanStartResponse{
		Success: true, BatchID: session.BatchID, Total: session.TotalCount,
	})
}

// GetBatchScanLogInternal 获取批量扫描日志和进度
func GetBatchScanLogInternal(batchID string) string {
	session := getBatchSession(batchID)
	if session == nil {
		return toJSONString(BatchScanLogResponse{
			BatchID: batchID, Logs: []string{}, Completed: true, Error: "batch session not found",
		})
	}

	logs := session.getAndClearLogs()

	completed := false
	select {
	case <-session.Done:
		completed = true
	default:
	}

	return toJSONString(BatchScanLogResponse{
		BatchID:      batchID,
		Logs:         logs,
		CurrentIndex: session.CurrentIndex,
		Total:        session.TotalCount,
		CurrentSkill: session.CurrentSkill,
		Completed:    completed,
	})
}

// GetBatchScanResultsInternal 等待完成并返回所有扫描结果
func GetBatchScanResultsInternal(batchID string) string {
	session := getBatchSession(batchID)
	if session == nil {
		return toJSONString(BatchScanResultsResponse{
			Success: false, Error: "batch session not found",
		})
	}

	select {
	case <-session.Done:
	case <-time.After(30 * time.Second):
		return toJSONString(BatchScanResultsResponse{
			Success: false, Error: "timeout waiting for batch scan completion",
		})
	}

	result := toJSONString(BatchScanResultsResponse{
		Success: true, Results: session.Results,
	})

	session.Cancel()
	removeBatchSession(batchID)
	return result
}

// CancelBatchSkillScanInternal 取消批量扫描
func CancelBatchSkillScanInternal(batchID string) string {
	session := getBatchSession(batchID)
	if session == nil {
		return `{"success": false, "error": "batch session not found"}`
	}

	session.Cancel()
	removeBatchSession(batchID)
	logging.Info("[BatchScan] Cancelled batch scan %s", batchID)
	return `{"success": true}`
}
