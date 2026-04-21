package proxy

import (
	"encoding/json"
	"testing"
	"time"
)

func TestToRepositoryAuditLog_ConvertsChainLog(t *testing.T) {
	src := AuditLog{
		ID:             "audit_1",
		Timestamp:      "2026-04-21T12:00:00Z",
		RequestID:      "req_1",
		AssetName:      "openclaw",
		AssetID:        "openclaw:a1",
		Model:          "gpt-test",
		RequestContent: "scan workspace",
		ToolCalls: []AuditToolCall{
			{
				Name:      "search_files",
				Arguments: `{"pattern":"TODO"}`,
				Result:    `{"matches":3}`,
			},
		},
		OutputContent:    "done",
		HasRisk:          true,
		RiskLevel:        "WARN",
		RiskReason:       "sensitive action",
		Confidence:       88,
		Action:           "WARN",
		PromptTokens:     12,
		CompletionTokens: 7,
		Duration:         321,
	}

	record, err := toRepositoryAuditLog(src)
	if err != nil {
		t.Fatalf("unexpected convert error: %v", err)
	}
	if record.ID != "audit_1" {
		t.Fatalf("expected id audit_1, got %s", record.ID)
	}
	if record.TotalTokens != 19 {
		t.Fatalf("expected total tokens 19, got %d", record.TotalTokens)
	}
	if record.MessageCount != 2 {
		t.Fatalf("expected message_count 2, got %d", record.MessageCount)
	}

	var toolCalls []map[string]interface{}
	if err := json.Unmarshal([]byte(record.ToolCalls), &toolCalls); err != nil {
		t.Fatalf("invalid tool_calls json: %v", err)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}

	var messages []map[string]interface{}
	if err := json.Unmarshal([]byte(record.Messages), &messages); err != nil {
		t.Fatalf("invalid messages json: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0]["role"] != "user" {
		t.Fatalf("expected first role user, got %v", messages[0]["role"])
	}
	if messages[1]["role"] != "assistant" {
		t.Fatalf("expected second role assistant, got %v", messages[1]["role"])
	}
}

func TestToRepositoryAuditLog_RequiresID(t *testing.T) {
	if _, err := toRepositoryAuditLog(AuditLog{}); err == nil {
		t.Fatalf("expected error when id is empty")
	}
}

func TestAuditLogPersistor_RememberLatestSeqSkipsOlderTask(t *testing.T) {
	p := &auditLogPersistor{
		latestSeq: make(map[string]auditPersistSeqState),
	}

	p.rememberLatestSeq("audit_1", 2)
	p.rememberLatestSeq("audit_1", 1) // should not roll back latest sequence

	stale, latest := p.isTaskStale(auditPersistTask{
		Log: AuditLog{ID: "audit_1"},
		Seq: 1,
	})
	if !stale {
		t.Fatalf("expected task seq=1 to be stale")
	}
	if latest != 2 {
		t.Fatalf("expected latest seq=2, got %d", latest)
	}

	stale, latest = p.isTaskStale(auditPersistTask{
		Log: AuditLog{ID: "audit_1"},
		Seq: 2,
	})
	if stale {
		t.Fatalf("did not expect task seq=2 to be stale")
	}
	if latest != 2 {
		t.Fatalf("expected latest seq=2, got %d", latest)
	}

	stale, latest = p.isTaskStale(auditPersistTask{
		Log: AuditLog{ID: "audit_1"},
		Seq: 3,
	})
	if stale {
		t.Fatalf("did not expect task seq=3 to be stale")
	}
	if latest != 2 {
		t.Fatalf("expected latest seq=2, got %d", latest)
	}
}

func TestAuditLogPersistor_CleanupLatestSeqTTL(t *testing.T) {
	now := time.Now()
	p := &auditLogPersistor{
		latestSeq: map[string]auditPersistSeqState{
			"audit_old": {
				Seq:       1,
				UpdatedAt: now.Add(-auditPersistLatestSeqTTL - time.Minute),
			},
			"audit_new": {
				Seq:       2,
				UpdatedAt: now,
			},
		},
	}

	p.cleanupLatestSeqLocked(now)

	if _, ok := p.latestSeq["audit_old"]; ok {
		t.Fatalf("expected expired latestSeq entry to be removed")
	}
	if _, ok := p.latestSeq["audit_new"]; !ok {
		t.Fatalf("expected non-expired latestSeq entry to remain")
	}
}

func TestAuditChainTracker_PersistSeqIncreasesAcrossSnapshots(t *testing.T) {
	tracker := NewAuditChainTracker()

	msgs := buildMessagesFromRaw(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {"role":"user","content":"run task"}
	  ]
	}`)
	tracker.StartFromRequest("req_seq", "openclaw", "openclaw:a1", "gpt-test", msgs)

	logs := tracker.GetAuditLogs(10, 0, false)
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	seq1 := logs[0].PersistSeq
	if seq1 <= 0 {
		t.Fatalf("expected positive persist sequence, got %d", seq1)
	}

	tracker.FinalizeRequestOutput("req_seq", "done")
	logs = tracker.GetAuditLogs(10, 0, false)
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	seq2 := logs[0].PersistSeq
	if seq2 <= seq1 {
		t.Fatalf("expected persist sequence to increase, got seq1=%d seq2=%d", seq1, seq2)
	}
}
