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

func TestOnRequest_QuotaBlockKeepsAuditRequestAndAssistantMessage(t *testing.T) {
	pp := &ProxyProtection{
		records:                  NewRecordStore(),
		singleSessionTokenLimit:  100,
		totalTokens:              180,
		baselineTotalTokens:      0,
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
	if !strings.EqualFold(record.Messages[1].Role, "user") {
		t.Fatalf("expected second message role=user, got %s", record.Messages[1].Role)
	}
	if strings.TrimSpace(record.Messages[1].Content) == "" {
		t.Fatalf("expected non-empty user message in truth record")
	}
	foundAssistant := false
	for _, msg := range record.Messages {
		if strings.EqualFold(msg.Role, "assistant") && strings.Contains(msg.Content, "QUOTA_EXCEEDED") {
			foundAssistant = true
			break
		}
	}
	if !foundAssistant {
		t.Fatalf("expected assistant quota message in truth record")
	}

	compat := truthRecordsToAuditCompat(completed)
	if len(compat) != 1 {
		t.Fatalf("expected 1 compat audit entry, got %d", len(compat))
	}
	requestContent, _ := compat[0]["request_content"].(string)
	if strings.TrimSpace(requestContent) == "" {
		t.Fatalf("expected non-empty audit request_content for blocked request")
	}
}

func TestOnRequest_RuntimeSessionQuotaBlocksAfterConversationReset(t *testing.T) {
	pp := &ProxyProtection{
		records:                      NewRecordStore(),
		singleSessionTokenLimit:      100,
		totalTokens:                  120,
		baselineTotalTokens:          0,
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
	if passed {
		t.Fatalf("expected request to be blocked when runtime session quota is already exhausted")
	}
	if result == nil || !strings.Contains(result.MockContent, "QUOTA_EXCEEDED") {
		t.Fatalf("expected QUOTA_EXCEEDED mock content")
	}

	completed := pp.records.GetCompletedRecords(10, 0, false)
	if len(completed) != 1 {
		t.Fatalf("expected 1 completed truth record, got %d", len(completed))
	}
	record := completed[0]
	if record.Decision == nil || !strings.EqualFold(record.Decision.Action, "BLOCK") {
		t.Fatalf("expected blocked security decision")
	}
	if !strings.EqualFold(record.Phase, RecordPhaseStopped) {
		t.Fatalf("expected phase=stopped, got %s", record.Phase)
	}
}
