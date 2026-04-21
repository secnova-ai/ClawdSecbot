package proxy

import (
	"sort"
	"strings"
	"time"

	"go_lib/core/logging"
)

const (
	auditPersistLatestSeqTTL      = 2 * time.Hour
	auditPersistLatestSeqMaxCount = 20000
	auditPersistOverflowQueueSize = 2048
)

type auditPersistSeqState struct {
	Seq       int64
	UpdatedAt time.Time
}

func (p *auditLogPersistor) loopOverflow() {
	for task := range p.overflowQueue {
		p.processTask(task, "overflow")
	}
}

func (p *auditLogPersistor) enqueuePersistTask(task auditPersistTask, source string) {
	if p == nil {
		return
	}
	select {
	case p.queue <- task:
		return
	default:
	}

	select {
	case p.overflowQueue <- task:
		logging.Warning(
			"[AuditLog] Persist queue overflow, redirected task: log_id=%s seq=%d source=%s",
			strings.TrimSpace(task.Log.ID),
			task.Seq,
			source,
		)
	default:
		logging.Warning(
			"[AuditLog] Persist queue saturated, dropping task: log_id=%s seq=%d source=%s",
			strings.TrimSpace(task.Log.ID),
			task.Seq,
			source,
		)
	}
}

func (p *auditLogPersistor) cleanupLatestSeqLocked(now time.Time) {
	if p == nil || len(p.latestSeq) == 0 {
		return
	}

	for logID, state := range p.latestSeq {
		if now.Sub(state.UpdatedAt) > auditPersistLatestSeqTTL {
			delete(p.latestSeq, logID)
		}
	}

	if len(p.latestSeq) <= auditPersistLatestSeqMaxCount {
		return
	}

	type seqAge struct {
		LogID     string
		UpdatedAt time.Time
	}
	items := make([]seqAge, 0, len(p.latestSeq))
	for logID, state := range p.latestSeq {
		items = append(items, seqAge{LogID: logID, UpdatedAt: state.UpdatedAt})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.Before(items[j].UpdatedAt)
	})
	excess := len(items) - auditPersistLatestSeqMaxCount
	for i := 0; i < excess; i++ {
		delete(p.latestSeq, items[i].LogID)
	}
}

func (t *AuditChainTracker) releaseCompletedChainResourcesLocked(state *auditLogState, requestID string) {
	if state == nil {
		return
	}

	logID := strings.TrimSpace(state.Log.ID)
	assetID := strings.TrimSpace(state.Log.AssetID)

	deletedRequestBindings := 0
	for rid, binding := range t.requestToLog {
		if binding.LogID != logID {
			continue
		}
		delete(t.requestToLog, rid)
		deletedRequestBindings++
	}

	deletedToolBindings := 0
	for toolKey, binding := range t.toolCallToLog {
		if binding.LogID != logID {
			continue
		}
		delete(t.toolCallToLog, toolKey)
		deletedToolBindings++
	}

	for _, toolID := range state.ToolSeq {
		toolKey := normalizeAuditToolKey(assetID, toolID)
		if toolKey == "" {
			continue
		}
		delete(t.pendingToolResults, toolKey)
	}

	delete(t.pendingRequestLinks, strings.TrimSpace(requestID))
	delete(t.pendingFinalOutputs, strings.TrimSpace(requestID))

	state.ToolSeq = nil
	state.ToolIndex = nil

	logging.Info(
		"[AuditChain] release completed chain resources: request_id=%s log_id=%s request_bindings=%d tool_bindings=%d",
		strings.TrimSpace(requestID),
		logID,
		deletedRequestBindings,
		deletedToolBindings,
	)
}
