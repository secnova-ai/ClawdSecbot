package proxy

import (
	"testing"

	"github.com/openai/openai-go"
)

func buildMessagesFromRaw(t *testing.T, raw string) []openai.ChatCompletionMessageParamUnion {
	t.Helper()
	req, _ := mustParseChatRequest(t, raw)
	return req.Messages
}

func TestAuditChainTracker_StartFromRequestCreatesLogOnlyWhenLastRoleUser(t *testing.T) {
	tracker := NewAuditChainTracker()

	userLast := buildMessagesFromRaw(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {"role":"system","content":"s"},
	    {"role":"user","content":"list files"}
	  ]
	}`)
	tracker.StartFromRequest("req_1", "openclaw", "openclaw:a1", "gpt-test", userLast)

	assistantLast := buildMessagesFromRaw(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {"role":"user","content":"do X"},
	    {"role":"assistant","content":"thinking"}
	  ]
	}`)
	tracker.StartFromRequest("req_2", "openclaw", "openclaw:a1", "gpt-test", assistantLast)

	logs := tracker.GetAuditLogs(10, 0, false)
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].RequestContent != "list files" {
		t.Fatalf("expected request_content=list files, got %q", logs[0].RequestContent)
	}
}

func TestAuditChainTracker_ToolCallAndToolResultCorrelateAcrossRequests(t *testing.T) {
	tracker := NewAuditChainTracker()

	startMessages := buildMessagesFromRaw(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {"role":"user","content":"scan workspace"}
	  ]
	}`)
	tracker.StartFromRequest("req_start", "openclaw", "openclaw:a1", "gpt-test", startMessages)

	toolCalls := []openai.ChatCompletionMessageToolCall{
		{
			ID: "call_1",
			Function: openai.ChatCompletionMessageToolCallFunction{
				Name:      "search_files",
				Arguments: "{\"pattern\":\"TODO\"}",
			},
		},
	}
	tracker.RecordToolCallsForRequest("req_start", "openclaw:a1", toolCalls, nil)

	tracker.LinkRequestByToolResults("req_follow", "openclaw:a1", map[string]string{
		"call_1": "{\"matches\":3}",
	})
	tracker.RecordToolResults("openclaw:a1", map[string]string{
		"call_1": "{\"matches\":3}",
	})
	tracker.FinalizeRequestOutput("req_follow", "done")

	logs := tracker.GetAuditLogs(10, 0, false)
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	logEntry := logs[0]
	if logEntry.OutputContent != "done" {
		t.Fatalf("expected output=done, got %q", logEntry.OutputContent)
	}
	if len(logEntry.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(logEntry.ToolCalls))
	}
	if logEntry.ToolCalls[0].Name != "search_files" {
		t.Fatalf("expected tool name search_files, got %s", logEntry.ToolCalls[0].Name)
	}
	if logEntry.ToolCalls[0].Result != "{\"matches\":3}" {
		t.Fatalf("unexpected tool result: %q", logEntry.ToolCalls[0].Result)
	}
}

func TestAuditChainTracker_OutOfOrderToolResultBeforeToolBinding(t *testing.T) {
	tracker := NewAuditChainTracker()

	startMessages := buildMessagesFromRaw(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {"role":"user","content":"run echo 4"}
	  ]
	}`)
	tracker.StartFromRequest("req_start", "openclaw", "openclaw:a1", "gpt-test", startMessages)

	// Request with tool_result can arrive before previous stream-finish binds tool_call_id.
	toolResults := map[string]string{
		"call_race_1": "4",
	}
	tracker.LinkRequestByToolResults("req_follow", "openclaw:a1", toolResults)
	tracker.RecordToolResults("openclaw:a1", toolResults)

	toolCalls := []openai.ChatCompletionMessageToolCall{
		{
			ID: "call_race_1",
			Function: openai.ChatCompletionMessageToolCallFunction{
				Name:      "exec",
				Arguments: "{\"command\":\"echo \\\"4\\\"\"}",
			},
		},
	}
	tracker.RecordToolCallsForRequest("req_start", "openclaw:a1", toolCalls, nil)
	tracker.FinalizeRequestOutput("req_follow", "done")

	logs := tracker.GetAuditLogs(10, 0, false)
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	logEntry := logs[0]
	if len(logEntry.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(logEntry.ToolCalls))
	}
	if logEntry.ToolCalls[0].Result != "4" {
		t.Fatalf("expected pending tool result to be recovered, got %q", logEntry.ToolCalls[0].Result)
	}
	if logEntry.OutputContent != "done" {
		t.Fatalf("expected output=done, got %q", logEntry.OutputContent)
	}
}

func TestAuditChainTracker_PendingFinalOutputRecoveredAfterLateBinding(t *testing.T) {
	tracker := NewAuditChainTracker()

	startMessages := buildMessagesFromRaw(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {"role":"user","content":"run echo 9"}
	  ]
	}`)
	tracker.StartFromRequest("req_start", "openclaw", "openclaw:a1", "gpt-test", startMessages)

	// Follow-up request arrives before tool_call_id binding is ready.
	toolResults := map[string]string{
		"call_late_1": "9",
	}
	tracker.LinkRequestByToolResults("req_follow", "openclaw:a1", toolResults)

	// Assistant final response arrives before request->log binding resolves.
	tracker.FinalizeRequestOutput("req_follow", "done")

	// Later, tool call binding arrives from the previous response stream finish.
	tracker.RecordToolCallsForRequest("req_start", "openclaw:a1", []openai.ChatCompletionMessageToolCall{
		{
			ID: "call_late_1",
			Function: openai.ChatCompletionMessageToolCallFunction{
				Name:      "exec",
				Arguments: "{\"command\":\"echo 9\"}",
			},
		},
	}, nil)

	logs := tracker.GetAuditLogs(10, 0, false)
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].OutputContent != "done" {
		t.Fatalf("expected delayed output recovery, got %q", logs[0].OutputContent)
	}
}

func TestAuditChainTracker_ToolCallIDNormalizationAcrossRounds(t *testing.T) {
	tracker := NewAuditChainTracker()

	startMessages := buildMessagesFromRaw(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {"role":"user","content":"run echo 10"}
	  ]
	}`)
	tracker.StartFromRequest("req_start", "openclaw", "openclaw:a1", "gpt-test", startMessages)

	// First round tool_call id style (from provider response).
	tracker.RecordToolCallsForRequest("req_start", "openclaw:a1", []openai.ChatCompletionMessageToolCall{
		{
			ID: "call_function_kfbxhqe7g6xr_1",
			Function: openai.ChatCompletionMessageToolCallFunction{
				Name:      "exec",
				Arguments: "{\"command\":\"echo \\\"10\\\"\"}",
			},
		},
	}, nil)

	// Follow-up round tool_result id style (normalized by client/protocol side).
	toolResults := map[string]string{
		"callfunctionkfbxhqe7g6xr1": "10",
	}
	tracker.LinkRequestByToolResults("req_follow", "openclaw:a1", toolResults)
	tracker.RecordToolResults("openclaw:a1", toolResults)
	tracker.FinalizeRequestOutput("req_follow", "done")

	logs := tracker.GetAuditLogs(10, 0, false)
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	logEntry := logs[0]
	if len(logEntry.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(logEntry.ToolCalls))
	}
	if logEntry.ToolCalls[0].Result != "10" {
		t.Fatalf("expected tool result to be linked after id normalization, got %q", logEntry.ToolCalls[0].Result)
	}
	if logEntry.OutputContent != "done" {
		t.Fatalf("expected output=done, got %q", logEntry.OutputContent)
	}
}

func TestAuditChainTracker_PartialMatchKeepsPendingForUnresolvedToolCall(t *testing.T) {
	tracker := NewAuditChainTracker()

	msgA := buildMessagesFromRaw(t, `{
	  "model":"gpt-test",
	  "messages":[{"role":"user","content":"task A"}]
	}`)
	msgB := buildMessagesFromRaw(t, `{
	  "model":"gpt-test",
	  "messages":[{"role":"user","content":"task B"}]
	}`)
	tracker.StartFromRequest("req_a", "openclaw", "openclaw:a1", "gpt-test", msgA)
	tracker.StartFromRequest("req_b", "openclaw", "openclaw:a1", "gpt-test", msgB)

	// Existing bound tool_call_id on older chain (req_a).
	tracker.RecordToolCallsForRequest("req_a", "openclaw:a1", []openai.ChatCompletionMessageToolCall{
		{
			ID: "call_old",
			Function: openai.ChatCompletionMessageToolCallFunction{
				Name:      "exec",
				Arguments: "{\"command\":\"echo old\"}",
			},
		},
	}, nil)

	// Follow-up request mixes one old bound id + one unresolved id.
	toolResults := map[string]string{
		"call_old": "old",
		"call_new": "new",
	}
	tracker.LinkRequestByToolResults("req_follow", "openclaw:a1", toolResults)
	tracker.RecordToolResults("openclaw:a1", toolResults)

	// New tool_call_id binds to req_b later (stream-finish of previous round).
	tracker.RecordToolCallsForRequest("req_b", "openclaw:a1", []openai.ChatCompletionMessageToolCall{
		{
			ID: "call_new",
			Function: openai.ChatCompletionMessageToolCallFunction{
				Name:      "exec",
				Arguments: "{\"command\":\"echo new\"}",
			},
		},
	}, nil)
	tracker.FinalizeRequestOutput("req_follow", "done")

	logs := tracker.GetAuditLogs(10, 0, false)
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}

	var logA, logB *AuditLog
	for i := range logs {
		switch logs[i].RequestContent {
		case "task A":
			logA = &logs[i]
		case "task B":
			logB = &logs[i]
		}
	}
	if logA == nil || logB == nil {
		t.Fatalf("expected logs for task A and task B")
	}
	if logB.OutputContent != "done" {
		t.Fatalf("expected task B output to be finalized by req_follow, got %q", logB.OutputContent)
	}
	if len(logB.ToolCalls) != 1 || logB.ToolCalls[0].Result != "new" {
		t.Fatalf("expected req_b tool result 'new', got %+v", logB.ToolCalls)
	}
}

func TestAuditChainTracker_FinalizeReleasesRuntimeBindings(t *testing.T) {
	tracker := NewAuditChainTracker()

	msgs := buildMessagesFromRaw(t, `{
	  "model":"gpt-test",
	  "messages":[{"role":"user","content":"run cleanup"}]
	}`)
	tracker.StartFromRequest("req_start", "openclaw", "openclaw:a1", "gpt-test", msgs)
	tracker.RecordToolCallsForRequest("req_start", "openclaw:a1", []openai.ChatCompletionMessageToolCall{
		{
			ID: "call_clean_1",
			Function: openai.ChatCompletionMessageToolCallFunction{
				Name:      "exec",
				Arguments: "{\"command\":\"echo done\"}",
			},
		},
	}, nil)

	tracker.LinkRequestByToolResults("req_follow", "openclaw:a1", map[string]string{
		"call_clean_1": "done",
	})
	tracker.RecordToolResults("openclaw:a1", map[string]string{
		"call_clean_1": "done",
	})
	tracker.FinalizeRequestOutput("req_follow", "finished")

	if len(tracker.toolCallToLog) != 0 {
		t.Fatalf("expected toolCallToLog to be released after finalize, got %d", len(tracker.toolCallToLog))
	}
	if len(tracker.pendingRequestLinks) != 0 {
		t.Fatalf("expected pendingRequestLinks to be released after finalize, got %d", len(tracker.pendingRequestLinks))
	}

	logID := ""
	for _, binding := range tracker.requestToLog {
		logID = binding.LogID
		break
	}
	if logID == "" {
		logs := tracker.GetAuditLogs(1, 0, false)
		if len(logs) == 0 {
			t.Fatalf("expected at least one log after finalize")
		}
		logID = logs[0].ID
	}
	state := tracker.logs[logID]
	if state == nil {
		t.Fatalf("expected state to still exist after finalize")
	}
	if state.ToolIndex != nil {
		t.Fatalf("expected ToolIndex memory to be released after finalize")
	}
	if state.ToolSeq != nil {
		t.Fatalf("expected ToolSeq memory to be released after finalize")
	}
}
