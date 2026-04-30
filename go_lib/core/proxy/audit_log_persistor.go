package proxy

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"go_lib/core/logging"
	"go_lib/core/repository"
)

const auditPersistQueueSize = 4096

type auditPersistTask struct {
	Log     AuditLog
	Attempt int
	Seq     int64
}

type auditLogPersistor struct {
	queue         chan auditPersistTask
	overflowQueue chan auditPersistTask
	mu            sync.Mutex
	// latestSeq tracks the latest snapshot sequence observed for each log id.
	// Any task with a smaller sequence is stale and must not be persisted.
	latestSeq map[string]auditPersistSeqState
}

var (
	auditPersistorOnce sync.Once
	auditPersistorInst *auditLogPersistor
)

func getAuditLogPersistor() *auditLogPersistor {
	auditPersistorOnce.Do(func() {
		auditPersistorInst = &auditLogPersistor{
			queue:         make(chan auditPersistTask, auditPersistQueueSize),
			overflowQueue: make(chan auditPersistTask, auditPersistOverflowQueueSize),
			latestSeq:     make(map[string]auditPersistSeqState),
		}
		go auditPersistorInst.loop()
		go auditPersistorInst.loopOverflow()
	})
	return auditPersistorInst
}

func (p *auditLogPersistor) loop() {
	for task := range p.queue {
		p.processTask(task, "queue")
	}
}

func (p *auditLogPersistor) retry(task auditPersistTask) {
	const maxAttempts = 3
	if p == nil || task.Attempt+1 >= maxAttempts {
		return
	}
	nextTask := task
	nextTask.Attempt++
	_ = p.enqueuePersistTask(nextTask, "retry")
}

func (p *auditLogPersistor) rememberLatestSeq(logID string, seq int64) {
	if p == nil {
		return
	}
	logID = strings.TrimSpace(logID)
	if logID == "" || seq <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rememberLatestSeqLocked(logID, seq, time.Now())
}

// rememberLatestSeqLocked advances the latest sequence while p.mu is held.
func (p *auditLogPersistor) rememberLatestSeqLocked(logID string, seq int64, now time.Time) {
	if p == nil {
		return
	}
	logID = strings.TrimSpace(logID)
	if logID == "" || seq <= 0 {
		return
	}
	if p.latestSeq == nil || len(p.latestSeq) == 0 {
		p.latestSeq = make(map[string]auditPersistSeqState)
	}
	current := p.latestSeq[logID]
	if seq > current.Seq {
		p.latestSeq[logID] = auditPersistSeqState{
			Seq:       seq,
			UpdatedAt: now,
		}
	} else {
		current.UpdatedAt = now
		p.latestSeq[logID] = current
	}
	p.cleanupLatestSeqLocked(now)
}

func (p *auditLogPersistor) isTaskStale(task auditPersistTask) (bool, int64) {
	if p == nil {
		return false, 0
	}
	logID := strings.TrimSpace(task.Log.ID)
	if logID == "" || task.Seq <= 0 {
		return false, 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cleanupLatestSeqLocked(time.Now())
	latest := p.latestSeq[logID].Seq
	return latest > 0 && task.Seq < latest, latest
}

func (p *auditLogPersistor) processTask(task auditPersistTask, source string) {
	if p == nil {
		return
	}
	if stale, latest := p.isTaskStale(task); stale {
		logging.Info(
			"[AuditLog] Skip stale persist task: log_id=%s seq=%d latest_seq=%d source=%s",
			strings.TrimSpace(task.Log.ID),
			task.Seq,
			latest,
			source,
		)
		return
	}
	if err := p.persist(task.Log); err != nil {
		if stale, latest := p.isTaskStale(task); stale {
			logging.Info(
				"[AuditLog] Persist failed but task became stale: log_id=%s seq=%d latest_seq=%d source=%s",
				strings.TrimSpace(task.Log.ID),
				task.Seq,
				latest,
				source,
			)
			return
		}
		logging.Warning(
			"[AuditLog] Failed to persist log %s (attempt=%d seq=%d source=%s): %v",
			task.Log.ID,
			task.Attempt+1,
			task.Seq,
			source,
			err,
		)
		p.retry(task)
	}
}

func (p *auditLogPersistor) persist(log AuditLog) error {
	if p == nil {
		return fmt.Errorf("audit log persistor is not initialized")
	}
	record, err := toRepositoryAuditLog(log)
	if err != nil {
		return err
	}
	// Acquire repository on each write so late DB initialization can still work.
	repo := repository.NewAuditLogRepository(nil)
	if repo == nil {
		return fmt.Errorf("audit log repository is not initialized")
	}
	return repo.SaveAuditLog(record)
}

func toRepositoryAuditLog(log AuditLog) (*repository.AuditLog, error) {
	logID := strings.TrimSpace(log.ID)
	if logID == "" {
		return nil, fmt.Errorf("missing audit log id")
	}

	timestamp := strings.TrimSpace(log.Timestamp)
	if timestamp == "" {
		timestamp = time.Now().Format(time.RFC3339Nano)
	}

	toolCalls := "[]"
	if len(log.ToolCalls) > 0 {
		toolCallsBytes, err := json.Marshal(log.ToolCalls)
		if err != nil {
			return nil, fmt.Errorf("marshal tool calls: %w", err)
		}
		toolCalls = string(toolCallsBytes)
	}

	messages := make([]map[string]interface{}, 0, 2)
	if userText := strings.TrimSpace(log.RequestContent); userText != "" {
		messages = append(messages, map[string]interface{}{
			"index":   0,
			"role":    "user",
			"content": userText,
		})
	}
	if outputText := strings.TrimSpace(log.OutputContent); outputText != "" {
		messages = append(messages, map[string]interface{}{
			"index":   len(messages),
			"role":    "assistant",
			"content": outputText,
		})
	}
	messagesJSON := "[]"
	if len(messages) > 0 {
		b, err := json.Marshal(messages)
		if err != nil {
			return nil, fmt.Errorf("marshal messages: %w", err)
		}
		messagesJSON = string(b)
	}

	action := strings.TrimSpace(log.Action)
	if action == "" {
		action = "ALLOW"
	}
	totalTokens := log.TotalTokens
	if totalTokens <= 0 {
		totalTokens = log.PromptTokens + log.CompletionTokens
	}

	return &repository.AuditLog{
		ID:        logID,
		Timestamp: timestamp,
		// request_id is kept only to satisfy legacy table schema NOT NULL.
		// Chain identity is log UUID (id), not request ID.
		RequestID:          logID,
		InstructionChainID: strings.TrimSpace(log.InstructionChainID),
		AssetName:          strings.TrimSpace(log.AssetName),
		AssetID:            strings.TrimSpace(log.AssetID),
		Model:              strings.TrimSpace(log.Model),
		RequestContent:     truncateToBytes(strings.TrimSpace(log.RequestContent), maxRecordMessageBytes),
		ToolCalls:          toolCalls,
		OutputContent:      truncateToBytes(strings.TrimSpace(log.OutputContent), maxRecordOutputBytes),
		HasRisk:            log.HasRisk,
		RiskLevel:          strings.TrimSpace(log.RiskLevel),
		RiskReason:         strings.TrimSpace(log.RiskReason),
		Confidence:         log.Confidence,
		Action:             action,
		PromptTokens:       log.PromptTokens,
		CompletionTokens:   log.CompletionTokens,
		TotalTokens:        totalTokens,
		DurationMs:         int(log.Duration),
		Messages:           messagesJSON,
		MessageCount:       len(messages),
	}, nil
}

func enqueueAuditLogPersist(log AuditLog) {
	if strings.TrimSpace(log.ID) == "" {
		return
	}
	seq := log.PersistSeq
	if seq <= 0 {
		seq = time.Now().UnixNano()
		log.PersistSeq = seq
	}
	persistor := getAuditLogPersistor()
	if persistor == nil {
		return
	}
	task := auditPersistTask{Log: log, Attempt: 0, Seq: seq}
	persistor.enqueuePersistTask(task, "enqueue")
}
