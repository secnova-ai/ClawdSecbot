package proxy

import (
	"context"
	"testing"
)

func TestSecurityChainRuntime_ParallelRequestsUseIndependentStreamBuffers(t *testing.T) {
	pp := &ProxyProtection{}
	ctxA := context.WithValue(context.Background(), "req", "a")
	ctxB := context.WithValue(context.Background(), "req", "b")
	bufferA := prepareTestRequestContext(t, pp, ctxA, "req_a")
	bufferB := prepareTestRequestContext(t, pp, ctxB, "req_b")

	bufferA.AppendContent("alpha")
	bufferB.AppendContent("beta")

	bufferA.mu.Lock()
	gotA := bufferA.contentChunks[0]
	bufferA.mu.Unlock()
	bufferB.mu.Lock()
	gotB := bufferB.contentChunks[0]
	bufferB.mu.Unlock()

	if gotA != "alpha" || gotB != "beta" {
		t.Fatalf("expected independent stream buffers, got a=%q b=%q", gotA, gotB)
	}
}

func TestSecurityChainRuntime_PendingRecoveryIsChainScoped(t *testing.T) {
	pp := &ProxyProtection{}
	prepareTestSecurityChain(t, pp, "req_a")
	prepareTestSecurityChain(t, pp, "req_b")

	pp.storePendingToolCallRecoveryWithIDsForRequest("req_a", nil, []string{"call_a"}, "", "risk a", "test")
	pp.storePendingToolCallRecoveryWithIDsForRequest("req_b", nil, []string{"call_b"}, "", "risk b", "test")
	pp.clearPendingToolCallRecoveryForRequest("req_a")

	if pendingRecoveryForTest(t, pp, "req_a") != nil {
		t.Fatalf("expected req_a pending recovery to be cleared")
	}
	if pendingRecoveryForTest(t, pp, "req_b") == nil {
		t.Fatalf("expected req_b pending recovery to remain isolated")
	}
}

func TestSecurityChainRuntime_ToolResultResolvesUniqueToolCallChain(t *testing.T) {
	pp := &ProxyProtection{}
	root := prepareTestSecurityChain(t, pp, "req_root")
	pp.bindToolCallIDsToSecurityChain("req_root", []string{"call_unique"})

	chain := pp.prepareSecurityChainForRequest("req_tool_result", nil, map[string]string{
		"call_unique": "result",
	})
	if chain == nil {
		t.Fatalf("expected tool result chain")
	}
	if chain.ChainID != root.ChainID {
		t.Fatalf("expected tool result to resolve root chain %s, got %s", root.ChainID, chain.ChainID)
	}
}

func TestSecurityChainRuntime_ToolResultResolvesNormalizedToolCallID(t *testing.T) {
	pp := &ProxyProtection{}
	root := prepareTestSecurityChain(t, pp, "req_root")
	pp.bindToolCallIDsToSecurityChain("req_root", []string{"call_function_db8uln6cs3ii_1"})

	chain := pp.prepareSecurityChainForRequest("req_tool_result", nil, map[string]string{
		"callfunctiondb8uln6cs3ii1": "result",
	})
	if chain == nil {
		t.Fatalf("expected tool result chain")
	}
	if chain.ChainID != root.ChainID {
		t.Fatalf("expected normalized tool result to resolve root chain %s, got %s", root.ChainID, chain.ChainID)
	}
	if chain.Degraded {
		t.Fatalf("expected normalized tool result not to degrade, reason=%s", chain.DegradeReason)
	}
}

func TestToolResultContentByToolCallIDMatchesNormalizedID(t *testing.T) {
	content, resultID, ok := toolResultContentByToolCallID(map[string]string{
		"callfunctiondb8uln6cs3ii1": "sensitive result",
	}, "call_function_db8uln6cs3ii_1")
	if !ok {
		t.Fatalf("expected normalized tool result lookup to match")
	}
	if resultID != "callfunctiondb8uln6cs3ii1" || content != "sensitive result" {
		t.Fatalf("unexpected lookup result id=%q content=%q", resultID, content)
	}
}

func TestSecurityChainRuntime_ToolCallIDConflictCreatesDegradedChain(t *testing.T) {
	pp := &ProxyProtection{}
	prepareTestSecurityChain(t, pp, "req_a")
	prepareTestSecurityChain(t, pp, "req_b")
	pp.bindToolCallIDsToSecurityChain("req_a", []string{"call_conflict"})
	pp.bindToolCallIDsToSecurityChain("req_b", []string{"call_conflict"})

	chain := pp.prepareSecurityChainForRequest("req_conflicted_result", nil, map[string]string{
		"call_conflict": "result",
	})
	if chain == nil {
		t.Fatalf("expected degraded conflict chain")
	}
	if !chain.Degraded || chain.DegradeReason != "ambiguous_tool_call_id" {
		t.Fatalf("expected ambiguous degraded chain, got degraded=%v reason=%s", chain.Degraded, chain.DegradeReason)
	}
}

func TestSecurityChainRuntime_PartialToolCallIDResolutionCreatesDegradedChain(t *testing.T) {
	pp := &ProxyProtection{}
	prepareTestSecurityChain(t, pp, "req_a")
	pp.bindToolCallIDsToSecurityChain("req_a", []string{"call_known"})

	chain := pp.prepareSecurityChainForRequest("req_partial_result", nil, map[string]string{
		"call_known":   "known result",
		"call_unknown": "unknown result",
	})
	if chain == nil {
		t.Fatalf("expected degraded partial-resolution chain")
	}
	if !chain.Degraded || chain.DegradeReason != "unresolved_tool_call_id" {
		t.Fatalf("expected unresolved degraded chain, got degraded=%v reason=%s chain=%s", chain.Degraded, chain.DegradeReason, chain.ChainID)
	}
	if chain.ChainID == pp.chainIDForRequest("req_a") {
		t.Fatalf("expected partial tool result not to bind to req_a chain")
	}
}

func TestSecurityChainRuntime_BlockedToolCallIsScopedToCurrentChain(t *testing.T) {
	pp := &ProxyProtection{}
	prepareTestSecurityChain(t, pp, "req_a")
	prepareTestSecurityChain(t, pp, "req_b")
	pp.bindToolCallIDsToSecurityChain("req_a", []string{"call_same"})
	pp.bindToolCallIDsToSecurityChain("req_b", []string{"call_same"})
	pp.markBlockedToolCallIDsForRequest("req_a", []string{"call_same"})

	if !pp.isBlockedToolCallIDForRequest("req_a", "call_same") {
		t.Fatalf("expected req_a call to be blocked")
	}
	if pp.isBlockedToolCallIDForRequest("req_b", "call_same") {
		t.Fatalf("expected req_b same tool_call_id to remain unblocked")
	}
}

func TestSecurityChainRuntime_RecoversChainFromHistoricalShepherdPrompt(t *testing.T) {
	pp := &ProxyProtection{}
	req, _ := mustParseChatRequest(t, `{
	  "model":"gpt-test",
	  "messages":[
	    {"role":"user","content":"read secret"},
	    {
	      "role":"assistant",
	      "tool_calls":[
	        {"id":"call_secret","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"/etc/shadow\"}"}}
	      ]
	    },
	    {"role":"tool","tool_call_id":"call_secret","content":"root:$6$hash"},
	    {"role":"assistant","content":"[ShepherdGate] 状态: 需要确认 | 原因: 工具结果存在风险"},
	    {"role":"user","content":"取消"}
	  ]
	}`)

	chain := pp.prepareSecurityChainForRequest("req_recovered", req.Messages, nil)
	if chain == nil {
		t.Fatalf("expected recovered chain")
	}
	if chain.Source != securityChainSourceHist {
		t.Fatalf("expected history source, got %s", chain.Source)
	}
	if !pp.isBlockedToolCallIDForRequest("req_recovered", "call_secret") {
		t.Fatalf("expected historical tool_call_id to be quarantined")
	}
}

func TestSecurityChainRuntime_UnresolvedConfirmationDoesNotClearOtherChains(t *testing.T) {
	pp := &ProxyProtection{}
	prepareTestSecurityChain(t, pp, "req_a")
	prepareTestSecurityChain(t, pp, "req_b")
	prepareTestSecurityChain(t, pp, "req_new")
	pp.storePendingToolCallRecoveryWithIDsForRequest("req_a", nil, []string{"call_a"}, "", "risk a", "test")
	pp.storePendingToolCallRecoveryWithIDsForRequest("req_b", nil, []string{"call_b"}, "", "risk b", "test")

	if pp.armPendingRecoveryFromContext(context.Background(), "req_new", []ConversationMessage{{Role: "user", Content: "继续"}}) {
		t.Fatalf("expected unrelated confirmation to remain unresolved")
	}
	if pendingRecoveryForTest(t, pp, "req_a") == nil || pendingRecoveryForTest(t, pp, "req_b") == nil {
		t.Fatalf("expected unrelated pending recoveries to remain intact")
	}
}

func TestSecurityChainRuntime_MissingContextCreatesDegradedRequestInsteadOfActiveFallback(t *testing.T) {
	pp := &ProxyProtection{}
	ctxA := context.WithValue(context.Background(), "req", "a")
	ctxB := context.WithValue(context.Background(), "req", "b")
	prepareTestRequestContext(t, pp, ctxA, "req_a")
	prepareTestRequestContext(t, pp, ctxB, "req_b")
	pp.clearRequestContext(ctxA)
	pp.auditMu.Lock()
	pp.currentRequestID = "req_b"
	pp.auditMu.Unlock()

	got := pp.requestIDFromContext(ctxA)
	if got == "" {
		t.Fatalf("expected degraded request id")
	}
	if got == "req_b" {
		t.Fatalf("expected missing context not to fall back to active request")
	}
	meta := pp.chainMetadataForRequest(got)
	if !meta.Degraded || meta.DegradeReason != "missing_request_context" {
		t.Fatalf("expected degraded chain for missing context, got %+v", meta)
	}
}

func TestSecurityChainRuntime_ClearRequestRuntimeStateDropsStreamBuffer(t *testing.T) {
	pp := &ProxyProtection{}
	prepareTestSecurityChain(t, pp, "req_state")
	buffer := pp.streamBufferForRequest("req_state")
	buffer.AppendContent("large content")

	if pp.requestRuntimeState("req_state") == nil {
		t.Fatalf("expected request runtime state to exist")
	}
	pp.clearRequestRuntimeState("req_state")
	if pp.requestRuntimeState("req_state") != nil {
		t.Fatalf("expected request runtime state to be cleared")
	}
}
