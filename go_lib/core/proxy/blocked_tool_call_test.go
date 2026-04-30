package proxy

import (
	"context"
	"testing"
	"time"

	"go_lib/core/shepherd"
)

func TestBlockedToolCallIDsExpire(t *testing.T) {
	pp := &ProxyProtection{}
	requestID := "req_blocked_expire"
	prepareTestSecurityChain(t, pp, requestID)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	pp.markBlockedToolCallIDsForRequestAt(requestID, []string{" call_1 "}, now, time.Minute)

	if !pp.isBlockedToolCallIDForRequestAt(requestID, "call_1", now.Add(30*time.Second)) {
		t.Fatalf("expected call_1 to be blocked before expiry")
	}
	if pp.isBlockedToolCallIDForRequestAt(requestID, "call_1", now.Add(time.Minute)) {
		t.Fatalf("expected call_1 to expire at ttl boundary")
	}
}

func TestClearBlockedToolCallIDs(t *testing.T) {
	pp := &ProxyProtection{}
	requestID := "req_blocked_clear"
	prepareTestSecurityChain(t, pp, requestID)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	pp.markBlockedToolCallIDsForRequestAt(requestID, []string{"call_1", "call_2"}, now, time.Hour)

	if cleared := pp.clearBlockedToolCallIDsForRequestAt(requestID, []string{"call_1"}, now); cleared != 1 {
		t.Fatalf("expected 1 cleared id, got %d", cleared)
	}
	if pp.isBlockedToolCallIDForRequestAt(requestID, "call_1", now) {
		t.Fatalf("expected call_1 to be cleared")
	}
	if !pp.isBlockedToolCallIDForRequestAt(requestID, "call_2", now) {
		t.Fatalf("expected call_2 to remain blocked")
	}
}

func TestToolResultPolicyConfirmedRecoveryClearsBlockedToolCallIDs(t *testing.T) {
	pp := &ProxyProtection{
		shepherdGate: &shepherd.ShepherdGate{},
	}
	requestID := "req_confirmed"
	prepareTestSecurityChain(t, pp, requestID)
	pp.storePendingToolCallRecoveryWithIDsForRequest(requestID, nil, []string{"call_confirmed"}, "", "risk", "test")
	chainID := pp.chainIDForRequest(requestID)
	pp.chainMu.Lock()
	pp.chains[chainID].PendingRecoveryArmed = true
	pp.chainMu.Unlock()
	pp.markBlockedToolCallIDsForRequestAt(requestID, []string{"call_confirmed"}, time.Now(), time.Hour)

	result := pp.runToolResultPolicyHooks(context.Background(), toolResultPolicyContext{
		RequestID:             requestID,
		HasToolResultMessages: true,
		LatestAssistantToolCalls: []toolCallRef{
			{ID: "call_confirmed", FuncName: "exec"},
		},
		ToolResultsMap: map[string]string{"call_confirmed": "ok"},
	})
	if result.Handled {
		t.Fatalf("expected confirmed recovery to continue normal flow")
	}
	if pp.isBlockedToolCallIDForRequest(requestID, "call_confirmed") {
		t.Fatalf("expected confirmed recovery to clear blocked tool_call_id")
	}
}
