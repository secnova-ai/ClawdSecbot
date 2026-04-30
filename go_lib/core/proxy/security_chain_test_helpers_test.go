package proxy

import (
	"context"
	"testing"
)

func prepareTestSecurityChain(t *testing.T, pp *ProxyProtection, requestID string) *SecurityChainState {
	t.Helper()
	chain := pp.createSecurityChain(requestID, securityChainSourceUser, "")
	if chain == nil {
		t.Fatalf("expected test security chain")
	}
	return chain
}

func prepareTestRequestContext(t *testing.T, pp *ProxyProtection, ctx context.Context, requestID string) *StreamBuffer {
	t.Helper()
	prepareTestSecurityChain(t, pp, requestID)
	pp.bindRequestContext(ctx, requestID)
	return pp.streamBufferForRequest(requestID)
}

func pendingRecoveryForTest(t *testing.T, pp *ProxyProtection, requestID string) *pendingToolCallRecovery {
	t.Helper()
	chain := pp.chainByRequestID(requestID)
	if chain == nil {
		t.Fatalf("expected chain for request %s", requestID)
	}
	return chain.PendingRecovery
}

func pendingRecoveryArmedForTest(t *testing.T, pp *ProxyProtection, requestID string) bool {
	t.Helper()
	chain := pp.chainByRequestID(requestID)
	if chain == nil {
		t.Fatalf("expected chain for request %s", requestID)
	}
	return chain.PendingRecoveryArmed
}
