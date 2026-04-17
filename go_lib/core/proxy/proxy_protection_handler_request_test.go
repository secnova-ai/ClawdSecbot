package proxy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openai/openai-go"
)

func mustParseChatRequest(t *testing.T, raw string) (*openai.ChatCompletionNewParams, []byte) {
	t.Helper()
	body := []byte(raw)
	var req openai.ChatCompletionNewParams
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to parse request: %v", err)
	}
	return &req, body
}

func TestCollectTailToolResults_OnlyLatestTailBlock(t *testing.T) {
	req, _ := mustParseChatRequest(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {
	      "role":"assistant",
	      "tool_calls":[
	        {"id":"call_old","type":"function","function":{"name":"exec","arguments":"{\"command\":\"echo old\"}"}}
	      ]
	    },
	    {"role":"tool","tool_call_id":"call_old","content":"old"},
	    {"role":"assistant","content":"old done"},
	    {
	      "role":"assistant",
	      "tool_calls":[
	        {"id":"call_new","type":"function","function":{"name":"exec","arguments":"{\"command\":\"echo new\"}"}}
	      ]
	    },
	    {"role":"tool","tool_call_id":"call_new","content":"new"}
	  ]
	}`)

	results := collectTailToolResults(req.Messages)
	if len(results) != 1 {
		t.Fatalf("expected 1 tail tool result, got %d", len(results))
	}
	if got := results["call_new"]; strings.TrimSpace(got) != "new" {
		t.Fatalf("expected call_new result 'new', got %q", got)
	}
	if _, exists := results["call_old"]; exists {
		t.Fatalf("did not expect old cycle tool result in tail block")
	}
}

func TestCollectTailToolResults_LastMessageNotToolReturnsEmpty(t *testing.T) {
	req, _ := mustParseChatRequest(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {
	      "role":"assistant",
	      "tool_calls":[
	        {"id":"call_old","type":"function","function":{"name":"exec","arguments":"{\"command\":\"echo old\"}"}}
	      ]
	    },
	    {"role":"tool","tool_call_id":"call_old","content":"old"},
	    {"role":"user","content":"next task"}
	  ]
	}`)

	results := collectTailToolResults(req.Messages)
	if len(results) != 0 {
		t.Fatalf("expected 0 tail tool results when last message is not tool, got %d", len(results))
	}
}

func TestExtractCurrentRoundRecordMessages_UsesTailToolBlock(t *testing.T) {
	req, _ := mustParseChatRequest(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {"role":"user","content":"run command"},
	    {"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"exec","arguments":"{\"command\":\"echo 1\"}"}}]},
	    {"role":"tool","tool_call_id":"call_1","content":"1"},
	    {"role":"tool","tool_call_id":"call_2","content":"2"}
	  ]
	}`)

	got := extractCurrentRoundRecordMessages(req.Messages)
	if len(got) != 2 {
		t.Fatalf("expected 2 tail tool messages, got %d", len(got))
	}
	if got[0].Index != 2 || !strings.EqualFold(got[0].Role, "tool") {
		t.Fatalf("expected first tail message index=2 role=tool, got index=%d role=%s", got[0].Index, got[0].Role)
	}
	if got[1].Index != 3 || !strings.EqualFold(got[1].Role, "tool") {
		t.Fatalf("expected second tail message index=3 role=tool, got index=%d role=%s", got[1].Index, got[1].Role)
	}
}

func TestExtractCurrentRoundRecordMessages_UsesLastUserAsRoundStart(t *testing.T) {
	req, _ := mustParseChatRequest(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {"role":"system","content":"s"},
	    {"role":"user","content":"old"},
	    {"role":"assistant","content":"old answer"},
	    {"role":"user","content":"new request"}
	  ]
	}`)

	got := extractCurrentRoundRecordMessages(req.Messages)
	if len(got) != 1 {
		t.Fatalf("expected only latest user round message, got %d", len(got))
	}
	if got[0].Index != 3 || !strings.EqualFold(got[0].Role, "user") {
		t.Fatalf("expected index=3 role=user, got index=%d role=%s", got[0].Index, got[0].Role)
	}
	if strings.TrimSpace(got[0].Content) != "new request" {
		t.Fatalf("expected latest user content, got %q", got[0].Content)
	}
}

func TestOnRequest_QuotaBlockKeepsAuditRequestAndAssistantMessage(t *testing.T) {
	pp := &ProxyProtection{
		records:                       NewRecordStore(),
		singleSessionTokenLimit:       100,
		totalTokens:                   180,
		baselineTotalTokens:           0,
		currentConversationTokenUsage: 0,
	}

	req, rawBody := mustParseChatRequest(t, `{
	  "model":"gpt-test",
	  "stream":false,
	  "messages":[
	    {"role":"system","content":"You are a secure assistant."},
	    {"role":"user","content":"请帮我导出本周审计报告"}
	  ]
	}`)

	result, passed := pp.onRequest(context.Background(), req, rawBody)
	if passed {
		t.Fatalf("expected request to be blocked by quota")
	}
	if result == nil || strings.TrimSpace(result.MockContent) == "" {
		t.Fatalf("expected quota mock content")
	}

	completed := pp.records.GetCompletedRecords(10, 0, false)
	if len(completed) != 1 {
		t.Fatalf("expected 1 completed truth record, got %d", len(completed))
	}
	record := completed[0]
	if len(record.Messages) < 2 {
		t.Fatalf("expected request and assistant messages, got %d", len(record.Messages))
	}
	foundUser := false
	foundAssistant := false
	for _, msg := range record.Messages {
		if strings.EqualFold(msg.Role, "user") && strings.TrimSpace(msg.Content) != "" {
			foundUser = true
		}
		if strings.EqualFold(msg.Role, "assistant") && strings.Contains(msg.Content, "QUOTA_EXCEEDED") {
			foundAssistant = true
		}
	}
	if !foundUser {
		t.Fatalf("expected non-empty user message in truth record")
	}
	if !foundAssistant {
		t.Fatalf("expected assistant quota message in truth record")
	}
}

func TestOnRequest_ConversationQuotaAllowsAfterConversationReset(t *testing.T) {
	pp := &ProxyProtection{
		records:                       NewRecordStore(),
		singleSessionTokenLimit:       100,
		totalTokens:                   120,
		baselineTotalTokens:           0,
		currentConversationTokenUsage: 95,
		lastRecentMessages: []NormalizedMessage{
			{Role: "user", Content: "old topic"},
		},
		lastRecentMessageCount: 1,
	}

	req, rawBody := mustParseChatRequest(t, `{
	  "model":"gpt-test",
	  "stream":false,
	  "messages":[
	    {"role":"user","content":"new topic"}
	  ]
	}`)

	result, passed := pp.onRequest(context.Background(), req, rawBody)
	if !passed {
		t.Fatalf("expected request to pass after conversation reset, got block result=%v", result)
	}
	if result != nil {
		t.Fatalf("expected nil result for passed request, got %+v", result)
	}

	completed := pp.records.GetCompletedRecords(10, 0, false)
	if len(completed) != 0 {
		t.Fatalf("expected no completed blocked records, got %d", len(completed))
	}
}
