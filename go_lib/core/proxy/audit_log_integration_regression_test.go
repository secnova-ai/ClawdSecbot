package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/openai/openai-go"
	"go_lib/core/shepherd"
)

// TestGetAuditLogsInternal_AggregatesWithoutPerTrackerTruncation 验证聚合查询不会被每个 tracker 的默认上限截断。
func TestGetAuditLogsInternal_AggregatesWithoutPerTrackerTruncation(t *testing.T) {
	tracker := NewAuditChainTracker()
	for i := 0; i < 150; i++ {
		requestID := fmt.Sprintf("req_%03d", i)
		messages := []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(fmt.Sprintf("task-%03d", i)),
		}
		tracker.StartFromRequest(requestID, "openclaw", "openclaw:a1", "gpt-test", messages)
	}

	pp := &ProxyProtection{auditTracker: tracker}

	proxyInstanceMu.Lock()
	oldProxyInstance := proxyInstance
	oldProxyByAssetKey := proxyByAssetKey
	proxyInstance = pp
	proxyByAssetKey = map[string]*ProxyProtection{"test": pp}
	proxyInstanceMu.Unlock()
	defer func() {
		proxyInstanceMu.Lock()
		proxyInstance = oldProxyInstance
		proxyByAssetKey = oldProxyByAssetKey
		proxyInstanceMu.Unlock()
	}()

	raw := GetAuditLogsInternal(0, 0, false)
	var payload struct {
		Logs  []AuditLog `json:"logs"`
		Total int        `json:"total"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("expected valid json response, got err=%v raw=%s", err, raw)
	}
	if payload.Total != 150 {
		t.Fatalf("expected total=150, got %d", payload.Total)
	}
	if len(payload.Logs) != 150 {
		t.Fatalf("expected logs length=150, got %d", len(payload.Logs))
	}
}

// TestAuditChainTracker_GetAuditLogsSnapshotUsesSingleView 验证单次加锁快照会返回一致的 logs 和 total。
func TestAuditChainTracker_GetAuditLogsSnapshotUsesSingleView(t *testing.T) {
	tracker := NewAuditChainTracker()
	tracker.StartFromRequest("req_1", "openclaw", "openclaw:a1", "gpt-test", []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("task one"),
	})
	tracker.StartFromRequest("req_2", "openclaw", "openclaw:a1", "gpt-test", []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("task two"),
	})
	tracker.SetRequestDecision("req_1", "BLOCK", "WARN", "risk", 80)

	logs, total := tracker.getAuditLogsSnapshot(true)
	if total != 1 {
		t.Fatalf("expected total=1 for risk-only snapshot, got %d", total)
	}
	if len(logs) != 1 {
		t.Fatalf("expected logs length=1 for risk-only snapshot, got %d", len(logs))
	}
	if logs[0].RequestContent != "task one" {
		t.Fatalf("expected risk snapshot to contain task one, got %q", logs[0].RequestContent)
	}
}

// TestEnqueuePersistTask_DroppedTaskDoesNotAdvanceLatestSeq 验证入队失败时 latestSeq 不会被错误推进。
func TestEnqueuePersistTask_DroppedTaskDoesNotAdvanceLatestSeq(t *testing.T) {
	persistor := &auditLogPersistor{
		queue:         make(chan auditPersistTask, 1),
		overflowQueue: make(chan auditPersistTask, 1),
		latestSeq:     make(map[string]auditPersistSeqState),
	}
	persistor.queue <- auditPersistTask{Log: AuditLog{ID: "queued"}, Seq: 1}
	persistor.overflowQueue <- auditPersistTask{Log: AuditLog{ID: "overflow"}, Seq: 1}

	ok := persistor.enqueuePersistTask(auditPersistTask{
		Log: AuditLog{
			ID: "audit_drop",
		},
		Seq: 9,
	}, "enqueue")
	if ok {
		t.Fatalf("expected saturated queues to reject task")
	}

	if _, ok := persistor.latestSeq["audit_drop"]; ok {
		t.Fatalf("expected dropped task to NOT advance latest sequence")
	}
}

// TestEnqueuePersistTask_AdvancesLatestSeqBeforeConsumersObserveTask 验证成功入队后旧快照会立即变 stale。
func TestEnqueuePersistTask_AdvancesLatestSeqBeforeConsumersObserveTask(t *testing.T) {
	persistor := &auditLogPersistor{
		queue:         make(chan auditPersistTask, 1),
		overflowQueue: make(chan auditPersistTask, 1),
		latestSeq:     make(map[string]auditPersistSeqState),
	}

	ok := persistor.enqueuePersistTask(auditPersistTask{
		Log: AuditLog{
			ID: "audit_atomic",
		},
		Seq: 2,
	}, "enqueue")
	if !ok {
		t.Fatalf("expected task to enqueue successfully")
	}

	stale, latest := persistor.isTaskStale(auditPersistTask{
		Log: AuditLog{
			ID: "audit_atomic",
		},
		Seq: 1,
	})
	if !stale {
		t.Fatalf("expected older task to become stale immediately after enqueue")
	}
	if latest != 2 {
		t.Fatalf("expected latest seq=2, got %d", latest)
	}
}

// TestOnRequest_SandboxShortCircuitWaitsForUpstreamResponse 验证 sandbox 短路只记录决策，真正收尾由后续响应完成。
func TestOnRequest_SandboxShortCircuitWaitsForUpstreamResponse(t *testing.T) {
	tracker := NewAuditChainTracker()
	startMessages := buildMessagesFromRaw(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {"role":"user","content":"run sandboxed tool"}
	  ]
	}`)
	tracker.StartFromRequest("req_start", "openclaw", "openclaw:a1", "gpt-test", startMessages)
	tracker.RecordToolCallsForRequest("req_start", "openclaw:a1", []openai.ChatCompletionMessageToolCall{
		{
			ID: "call_sandbox_1",
			Function: openai.ChatCompletionMessageToolCallFunction{
				Name:      "exec",
				Arguments: "{\"command\":\"dangerous command\"}",
			},
		},
	}, nil)

	pp := &ProxyProtection{
		assetName:    "openclaw",
		assetID:      "openclaw:a1",
		streamBuffer: NewStreamBuffer(),
		shepherdGate: &shepherd.ShepherdGate{},
		auditTracker: tracker,
		ctx:          context.Background(),
	}

	ctx := context.WithValue(context.Background(), "test", "sandbox-short-circuit")

	req, rawBody := mustParseChatRequest(t, `{
	  "model":"gpt-test",
	  "stream":false,
	  "messages":[
	    {
	      "role":"assistant",
	      "tool_calls":[
	        {"id":"call_sandbox_1","type":"function","function":{"name":"exec","arguments":"{\"command\":\"dangerous command\"}"}}
	      ]
	    },
	    {
	      "role":"tool",
	      "tool_call_id":"call_sandbox_1",
	      "content":"[ClawdSecbot] ACTION=BLOCK reason=sandbox deny"
	    }
	  ]
	}`)

	result, passed := pp.onRequest(ctx, req, rawBody)
	if !passed {
		t.Fatalf("expected sandbox short-circuit path to pass request, got block result=%+v", result)
	}
	if result != nil {
		t.Fatalf("expected nil result when sandbox short-circuit returns early, got %+v", result)
	}

	logs := tracker.GetAuditLogs(10, 0, false)
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(logs))
	}
	if logs[0].OutputContent != "" {
		t.Fatalf("expected audit chain to wait for actual assistant output, got %q", logs[0].OutputContent)
	}
	if len(tracker.requestToLog) == 0 {
		t.Fatalf("expected request bindings to remain before upstream response")
	}
	if len(tracker.toolCallToLog) == 0 {
		t.Fatalf("expected tool bindings to remain before upstream response")
	}

	resp := &openai.ChatCompletion{
		Model: "gpt-test",
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "stop",
				Message: openai.ChatCompletionMessage{
					Content: "assistant final response after sandbox block",
				},
			},
		},
	}
	if !pp.onResponse(ctx, resp) {
		t.Fatalf("expected onResponse to pass")
	}

	logs = tracker.GetAuditLogs(10, 0, false)
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log after response, got %d", len(logs))
	}
	if logs[0].OutputContent != "assistant final response after sandbox block" {
		t.Fatalf("expected actual assistant output after response, got %q", logs[0].OutputContent)
	}
	if logs[0].Duration < 0 {
		t.Fatalf("expected non-negative duration, got %d", logs[0].Duration)
	}
	if len(tracker.requestToLog) != 0 {
		t.Fatalf("expected request bindings to be released after response, got %d", len(tracker.requestToLog))
	}
	if len(tracker.toolCallToLog) != 0 {
		t.Fatalf("expected tool bindings to be released after response, got %d", len(tracker.toolCallToLog))
	}
}
