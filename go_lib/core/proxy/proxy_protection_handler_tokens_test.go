package proxy

import (
	"context"
	"strings"
	"testing"

	"github.com/openai/openai-go"
)

func TestOnResponse_EstimatesUsageWhenMissing(t *testing.T) {
	pp := &ProxyProtection{
		streamBuffer: NewStreamBuffer(),
	}
	pp.streamBuffer.requestID = "req-response"
	pp.streamBuffer.requestMessages = []ConversationMessage{
		{
			Role:    "user",
			Content: "hello, please summarize this text",
		},
	}

	resp := &openai.ChatCompletion{
		Model: "gpt-test",
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: "summary output",
				},
			},
		},
		// Usage intentionally omitted (all zero).
	}

	if !pp.onResponse(context.Background(), resp) {
		t.Fatalf("expected onResponse to pass")
	}

	pp.metricsMu.Lock()
	defer pp.metricsMu.Unlock()

	if pp.totalPromptTokens <= 0 {
		t.Fatalf("expected prompt tokens to be estimated, got %d", pp.totalPromptTokens)
	}
	if pp.totalCompletionTokens <= 0 {
		t.Fatalf("expected completion tokens to be estimated, got %d", pp.totalCompletionTokens)
	}
	if pp.totalTokens != pp.totalPromptTokens+pp.totalCompletionTokens {
		t.Fatalf("expected total=%d+%d, got %d", pp.totalPromptTokens, pp.totalCompletionTokens, pp.totalTokens)
	}
}

func TestOnResponse_UsesStableRequestIDForTruthRecordCompletion(t *testing.T) {
	pp := &ProxyProtection{
		streamBuffer:     &StreamBuffer{requestID: "req-newer"},
		records:          NewRecordStore(),
		currentRequestID: "req-newer",
	}
	ctx := context.WithValue(context.Background(), "k", "v")
	pp.bindRequestContext(ctx, "req-finished")

	pp.updateTruthRecord("req-finished", func(r *TruthRecord) {
		r.Model = "gpt-test"
		r.Phase = advanceRecordPhase(r.Phase, RecordPhaseStarting)
	})
	pp.updateTruthRecord("req-newer", func(r *TruthRecord) {
		r.Model = "gpt-other"
		r.Phase = advanceRecordPhase(r.Phase, RecordPhaseStarting)
	})
	_ = pp.records.Pending()

	resp := &openai.ChatCompletion{
		Model: "gpt-test",
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "stop",
				Message: openai.ChatCompletionMessage{
					Content: "done",
				},
			},
		},
	}

	if !pp.onResponse(ctx, resp) {
		t.Fatalf("expected onResponse to pass")
	}

	pending := pp.records.Pending()
	if len(pending) == 0 {
		t.Fatalf("expected truth record snapshots")
	}

	var finished, newer *TruthRecord
	for i := range pending {
		r := pending[i]
		if r.RequestID == "req-finished" {
			finished = &r
		}
		if r.RequestID == "req-newer" {
			newer = &r
		}
	}

	if finished == nil {
		t.Fatalf("expected completed snapshot for req-finished")
	}
	if !isRecordComplete(finished) {
		t.Fatalf("expected req-finished to be completed, got phase=%s", finished.Phase)
	}
	if newer != nil && isRecordComplete(newer) {
		t.Fatalf("expected req-newer to remain incomplete")
	}
}

func TestOnStreamChunk_UsageAccumulatesByDeltaForCumulativeReports(t *testing.T) {
	pp := &ProxyProtection{
		streamBuffer: NewStreamBuffer(),
	}

	chunk1 := &openai.ChatCompletionChunk{
		Usage: openai.CompletionUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}
	chunk2 := &openai.ChatCompletionChunk{
		Usage: openai.CompletionUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}
	chunk3 := &openai.ChatCompletionChunk{
		Usage: openai.CompletionUsage{
			PromptTokens:     12,
			CompletionTokens: 7,
			TotalTokens:      19,
		},
	}

	_ = pp.onStreamChunk(context.Background(), chunk1)
	_ = pp.onStreamChunk(context.Background(), chunk2)
	_ = pp.onStreamChunk(context.Background(), chunk3)

	pp.metricsMu.Lock()
	defer pp.metricsMu.Unlock()

	if pp.totalPromptTokens != 12 {
		t.Fatalf("expected prompt tokens=12, got %d", pp.totalPromptTokens)
	}
	if pp.totalCompletionTokens != 7 {
		t.Fatalf("expected completion tokens=7, got %d", pp.totalCompletionTokens)
	}
	if pp.totalTokens != 19 {
		t.Fatalf("expected total tokens=19, got %d", pp.totalTokens)
	}
}

func TestOnStreamChunk_EmitsRealtimeContentAndToolLogs(t *testing.T) {
	logChan := make(chan string, 20)
	pp := &ProxyProtection{
		streamBuffer: NewStreamBuffer(),
		logChan:      logChan,
	}
	pp.currentRequestID = "req_test"

	chunk := &openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Delta: openai.ChatCompletionChunkChoiceDelta{
					Content: "hello",
					ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
						{
							Index: 0,
							ID:    "call_1",
							Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
								Name:      "search_web",
								Arguments: `{"q":"weather"}`,
							},
						},
					},
				},
			},
		},
	}

	if !pp.onStreamChunk(context.Background(), chunk) {
		t.Fatalf("expected onStreamChunk to pass")
	}

	close(logChan)
	var sawDelta, sawToolName, sawToolArgs bool
	var sawMonitorStart, sawMonitorDelta, sawMonitorTool bool
	for log := range logChan {
		if strings.Contains(log, `"key":"proxy_stream_delta"`) &&
			strings.Contains(log, `"content":"hello"`) {
			sawDelta = true
		}
		if strings.Contains(log, `"key":"proxy_tool_call_name"`) &&
			strings.Contains(log, `"name":"search_web"`) {
			sawToolName = true
		}
		if strings.Contains(log, `"key":"proxy_tool_call_args"`) &&
			strings.Contains(log, `weather`) {
			sawToolArgs = true
		}
		if strings.Contains(log, `"key":"monitor_upstream_stream_started"`) {
			sawMonitorStart = true
		}
		if strings.Contains(log, `"key":"monitor_upstream_stream_delta"`) &&
			strings.Contains(log, `"content":"hello"`) {
			sawMonitorDelta = true
		}
		if strings.Contains(log, `"key":"monitor_upstream_tool_call"`) &&
			strings.Contains(log, `"name":"search_web"`) {
			sawMonitorTool = true
		}
	}

	if !sawDelta {
		t.Fatalf("expected realtime stream delta log")
	}
	if !sawToolName {
		t.Fatalf("expected realtime tool name log")
	}
	if !sawToolArgs {
		t.Fatalf("expected realtime tool args log")
	}
	if !sawMonitorStart {
		t.Fatalf("expected monitor stream started log")
	}
	if !sawMonitorDelta {
		t.Fatalf("expected monitor stream delta log")
	}
	if !sawMonitorTool {
		t.Fatalf("expected monitor tool call log")
	}
}

// TestOnStreamChunk_UsesStableRequestIDForCompletion verifies stream completion
// uses context-bound request_id even if global/request buffer points elsewhere.
func TestOnStreamChunk_UsesStableRequestIDForCompletion(t *testing.T) {
	pp := &ProxyProtection{
		streamBuffer:     &StreamBuffer{requestID: "req-newer"},
		records:          NewRecordStore(),
		currentRequestID: "req-newer",
	}
	ctx := context.WithValue(context.Background(), "k", "v")
	pp.bindRequestContext(ctx, "req-stream")

	pp.updateTruthRecord("req-stream", func(r *TruthRecord) {
		r.Phase = advanceRecordPhase(r.Phase, RecordPhaseStarting)
	})
	pp.updateTruthRecord("req-newer", func(r *TruthRecord) {
		r.Phase = advanceRecordPhase(r.Phase, RecordPhaseStarting)
	})
	_ = pp.records.Pending()

	chunk := &openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				FinishReason: "stop",
			},
		},
	}

	if !pp.onStreamChunk(ctx, chunk) {
		t.Fatalf("expected onStreamChunk to pass")
	}

	pending := pp.records.Pending()
	if len(pending) == 0 {
		t.Fatalf("expected truth record snapshots")
	}

	var streamRec, newer *TruthRecord
	for i := range pending {
		r := pending[i]
		if r.RequestID == "req-stream" {
			streamRec = &r
		}
		if r.RequestID == "req-newer" {
			newer = &r
		}
	}

	if streamRec == nil {
		t.Fatalf("expected completed snapshot for req-stream")
	}
	if !isRecordComplete(streamRec) {
		t.Fatalf("expected req-stream to be completed, got phase=%s", streamRec.Phase)
	}
	if newer != nil && isRecordComplete(newer) {
		t.Fatalf("expected req-newer to remain incomplete")
	}
}

func TestOnResponse_EmitsMonitorCompletionAndReturn(t *testing.T) {
	logChan := make(chan string, 20)
	pp := &ProxyProtection{
		streamBuffer: NewStreamBuffer(),
		logChan:      logChan,
	}
	pp.currentRequestID = "req_test"
	pp.streamBuffer.requestMessages = []ConversationMessage{
		{Role: "user", Content: "hello"},
	}

	resp := &openai.ChatCompletion{
		Model: "gpt-test",
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: "world",
				},
			},
		},
	}

	if !pp.onResponse(context.Background(), resp) {
		t.Fatalf("expected onResponse to pass")
	}

	close(logChan)
	var sawCompleted, sawReturned bool
	for log := range logChan {
		if strings.Contains(log, `"key":"monitor_upstream_completed"`) &&
			strings.Contains(log, `"final_text":"world"`) {
			sawCompleted = true
		}
		if strings.Contains(log, `"key":"monitor_response_returned"`) &&
			strings.Contains(log, `"returned_to_user_text":"world"`) {
			sawReturned = true
		}
	}

	if !sawCompleted {
		t.Fatalf("expected monitor upstream completed log")
	}
	if !sawReturned {
		t.Fatalf("expected monitor response returned log")
	}
}

func TestOnResponse_AssignsMissingToolCallIDAndRecordsAudit(t *testing.T) {
	tracker := NewAuditChainTracker()
	pp := &ProxyProtection{
		streamBuffer:     NewStreamBuffer(),
		auditTracker:     tracker,
		currentRequestID: "req-missing-id",
		assetID:          "openclaw:a1",
	}

	req, _ := mustParseChatRequest(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {"role":"user","content":"run one tool"}
	  ]
	}`)
	tracker.StartFromRequest("req-missing-id", "openclaw", "openclaw:a1", "gpt-test", req.Messages)

	resp := &openai.ChatCompletion{
		Model: "gpt-test",
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					ToolCalls: []openai.ChatCompletionMessageToolCall{
						{
							Function: openai.ChatCompletionMessageToolCallFunction{
								Name:      "search_files",
								Arguments: `{"pattern":"TODO"}`,
							},
						},
					},
				},
			},
		},
	}

	if !pp.onResponse(context.Background(), resp) {
		t.Fatalf("expected onResponse to pass")
	}

	gotID := strings.TrimSpace(resp.Choices[0].Message.ToolCalls[0].ID)
	if gotID == "" {
		t.Fatalf("expected injected tool_call_id")
	}

	logs := tracker.GetAuditLogs(10, 0, false)
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(logs))
	}
	if len(logs[0].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call in audit log, got %d", len(logs[0].ToolCalls))
	}
	if logs[0].ToolCalls[0].Name != "search_files" {
		t.Fatalf("expected tool name search_files, got %s", logs[0].ToolCalls[0].Name)
	}
}

func TestOnStreamChunk_AssignsStableToolCallIDWhenMissing(t *testing.T) {
	pp := &ProxyProtection{
		streamBuffer: NewStreamBuffer(),
	}

	chunk1 := &openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Delta: openai.ChatCompletionChunkChoiceDelta{
					ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
						{
							Index: 0,
							Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
								Name: "search_files",
							},
						},
					},
				},
			},
		},
	}
	chunk2 := &openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Delta: openai.ChatCompletionChunkChoiceDelta{
					ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
						{
							Index: 0,
							Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
								Arguments: `{"pattern":"main.go"}`,
							},
						},
					},
				},
			},
		},
	}

	if !pp.onStreamChunk(context.Background(), chunk1) {
		t.Fatalf("expected first stream chunk to pass")
	}
	if !pp.onStreamChunk(context.Background(), chunk2) {
		t.Fatalf("expected second stream chunk to pass")
	}

	id1 := strings.TrimSpace(chunk1.Choices[0].Delta.ToolCalls[0].ID)
	id2 := strings.TrimSpace(chunk2.Choices[0].Delta.ToolCalls[0].ID)
	if id1 == "" {
		t.Fatalf("expected first chunk to receive generated tool_call_id")
	}
	if id2 == "" {
		t.Fatalf("expected second chunk to keep tool_call_id")
	}
	if id1 != id2 {
		t.Fatalf("expected stable tool_call_id across chunks, got %s vs %s", id1, id2)
	}
}

func TestOnStreamChunk_BindsToolCallBeforeFinishForFollowupLinking(t *testing.T) {
	tracker := NewAuditChainTracker()
	pp := &ProxyProtection{
		streamBuffer: NewStreamBuffer(),
		auditTracker: tracker,
		assetID:      "openclaw:a1",
	}

	req, _ := mustParseChatRequest(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {"role":"user","content":"run one tool"}
	  ]
	}`)
	tracker.StartFromRequest("req_stream", "openclaw", "openclaw:a1", "gpt-test", req.Messages)

	ctx := context.WithValue(context.Background(), "k", "stream")
	pp.bindRequestContext(ctx, "req_stream")

	chunk := &openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Delta: openai.ChatCompletionChunkChoiceDelta{
					ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
						{
							Index: 0,
							ID:    "call_early_1",
							Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
								Name:      "exec",
								Arguments: `{"command":"echo 9"}`,
							},
						},
					},
				},
			},
		},
	}
	if !pp.onStreamChunk(ctx, chunk) {
		t.Fatalf("expected stream delta to pass")
	}

	// Follow-up request should be able to link immediately via tool_call_id,
	// without waiting for stream finish.
	toolResults := map[string]string{"call_early_1": "9"}
	tracker.LinkRequestByToolResults("req_follow", "openclaw:a1", toolResults)
	tracker.RecordToolResults("openclaw:a1", toolResults)
	tracker.FinalizeRequestOutput("req_follow", "done")

	logs := tracker.GetAuditLogs(10, 0, false)
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(logs))
	}
	if len(logs[0].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(logs[0].ToolCalls))
	}
	if logs[0].ToolCalls[0].Result != "9" {
		t.Fatalf("expected tool result=9, got %q", logs[0].ToolCalls[0].Result)
	}
	if logs[0].OutputContent != "done" {
		t.Fatalf("expected output=done, got %q", logs[0].OutputContent)
	}
}
