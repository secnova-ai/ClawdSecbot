package proxy

import (
	"encoding/json"
	"strings"
	"testing"

	"go_lib/core/shepherd"
)

// drainSecurityEvents empties the global buffer so tests do not contaminate each other.
func drainSecurityEvents() []shepherd.SecurityEvent {
	return shepherd.GetSecurityEventBuffer().GetAndClearSecurityEvents()
}

// newProxyForSecurityEventTest builds the thinnest ProxyProtection instance needed
// to exercise emitSecurityEvent — the helper only reads assetName/assetID and the
// shepherd buffer, so we skip heavyweight construction.
func newProxyForSecurityEventTest(assetName, assetID string) *ProxyProtection {
	return &ProxyProtection{assetName: assetName, assetID: assetID}
}

// TestEmitSecurityEvent_FixedSourceAndFields verifies every proxy-authored
// SecurityEvent carries the canonical fields required by the monitor panel
// (source=react_agent, request/asset binding, payload passthrough).
func TestEmitSecurityEvent_FixedSourceAndFields(t *testing.T) {
	_ = drainSecurityEvents()

	pp := newProxyForSecurityEventTest("openclaw", "asset-123")
	pp.emitSecurityEvent("req-1", "blocked", "Blocked tool_shell", "CRITICAL", "reason text")

	events := drainSecurityEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Source != "react_agent" {
		t.Errorf("expected Source=react_agent, got %q", ev.Source)
	}
	if ev.EventType != "blocked" {
		t.Errorf("expected EventType=blocked, got %q", ev.EventType)
	}
	if ev.ActionDesc != "Blocked tool_shell" {
		t.Errorf("unexpected ActionDesc: %q", ev.ActionDesc)
	}
	if ev.RiskType != "CRITICAL" {
		t.Errorf("unexpected RiskType: %q", ev.RiskType)
	}
	if ev.Detail != "reason text" {
		t.Errorf("unexpected Detail: %q", ev.Detail)
	}
	if ev.RequestID != "req-1" || ev.AssetID != "asset-123" || ev.AssetName != "openclaw" {
		t.Errorf("binding fields mismatch: %+v", ev)
	}
	if ev.ID == "" || ev.Timestamp == "" {
		t.Errorf("expected ID and Timestamp to be auto-filled, got %+v", ev)
	}
}

func TestEmitSecurityEvent_IncludesInstructionChainMetadataInDetail(t *testing.T) {
	_ = drainSecurityEvents()

	pp := newProxyForSecurityEventTest("openclaw", "asset-123")
	chain := pp.createSecurityChain("req-chain", securityChainSourceTool, "ambiguous_tool_call_id")
	pp.emitSecurityEvent("req-chain", "blocked", "Blocked tool result", "CRITICAL", "reason text")

	events := drainSecurityEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].InstructionChainID != chain.ChainID {
		t.Fatalf("expected top-level chain id %s, got %s", chain.ChainID, events[0].InstructionChainID)
	}
	var detail map[string]interface{}
	if err := json.Unmarshal([]byte(events[0].Detail), &detail); err != nil {
		t.Fatalf("expected JSON detail, got %q: %v", events[0].Detail, err)
	}
	if detail["message"] != "reason text" {
		t.Fatalf("expected original detail message, got %#v", detail["message"])
	}
	if detail["instruction_chain_id"] != chain.ChainID || detail["chain_source"] != securityChainSourceTool {
		t.Fatalf("expected chain metadata in detail, got %#v", detail)
	}
	if detail["chain_degraded"] != true || detail["chain_degrade_reason"] != "ambiguous_tool_call_id" {
		t.Fatalf("expected degraded metadata in detail, got %#v", detail)
	}
}

// TestEmitSecurityEvent_QuotaBranch simulates the conversation/daily quota branches:
// both should record a "blocked" event with RiskType=QUOTA.
func TestEmitSecurityEvent_QuotaBranch(t *testing.T) {
	_ = drainSecurityEvents()

	pp := newProxyForSecurityEventTest("nullclaw", "asset-q")
	pp.emitSecurityEvent("req-session", "blocked", "Conversation token quota exceeded", "QUOTA", "Conversation token quota exceeded (5000/5000)")
	pp.emitSecurityEvent("req-daily", "blocked", "Daily token quota exceeded", "QUOTA", "Daily token quota exceeded (10000/10000)")

	events := drainSecurityEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 quota events, got %d", len(events))
	}
	for _, ev := range events {
		if ev.RiskType != "QUOTA" || ev.EventType != "blocked" {
			t.Errorf("expected blocked/QUOTA, got event=%+v", ev)
		}
	}
}

// TestEmitSecurityEvent_SandboxBlocked checks the SANDBOX_BLOCKED path emits
// an event instead of relying on the sandbox hook log watcher (removed per D2).
func TestEmitSecurityEvent_SandboxBlocked(t *testing.T) {
	_ = drainSecurityEvents()

	pp := newProxyForSecurityEventTest("dintalclaw", "asset-sb")
	pp.emitSecurityEvent("req-sb", "blocked", "Sandbox blocked tool execution", "SANDBOX_BLOCKED", "tool result already blocked by ClawdSecbot sandbox")

	events := drainSecurityEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 sandbox event, got %d", len(events))
	}
	if events[0].RiskType != "SANDBOX_BLOCKED" {
		t.Errorf("expected RiskType=SANDBOX_BLOCKED, got %q", events[0].RiskType)
	}
}

// TestEmitSecurityEvent_NeedsConfirmationEventType replays the ShepherdGate
// non-ALLOWED branch logic: when decision.Status == NEEDS_CONFIRMATION the
// event must carry EventType="needs_confirmation", otherwise "blocked".
// UI relies on this split for coloring and localization (待确认 vs 已拦截).
func TestEmitSecurityEvent_NeedsConfirmationEventType(t *testing.T) {
	_ = drainSecurityEvents()

	pp := newProxyForSecurityEventTest("openclaw", "asset-nc")

	// Mirror the handler's shepherdEventType selection.
	resolve := func(status string) string {
		if status == "NEEDS_CONFIRMATION" {
			return "needs_confirmation"
		}
		return "blocked"
	}

	pp.emitSecurityEvent("req-nc", resolve("NEEDS_CONFIRMATION"), "desc", "CRITICAL", "reason")
	pp.emitSecurityEvent("req-bk", resolve("BLOCKED"), "desc", "CRITICAL", "reason")

	events := drainSecurityEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].EventType != "needs_confirmation" {
		t.Errorf("expected needs_confirmation, got %q", events[0].EventType)
	}
	if events[1].EventType != "blocked" {
		t.Errorf("expected blocked, got %q", events[1].EventType)
	}
}

// TestPostValidationOverrideTag_Detectable guards the shepherd->proxy contract:
// proxy uses strings.Contains(decision.Reason, shepherd.PostValidationOverrideTag)
// to mark overridden events, so the tag must stay literal and non-empty.
func TestPostValidationOverrideTag_Detectable(t *testing.T) {
	if shepherd.PostValidationOverrideTag == "" {
		t.Fatal("PostValidationOverrideTag must not be empty")
	}
	reason := "LLM allowed but injected. " + shepherd.PostValidationOverrideTag
	if !strings.Contains(reason, shepherd.PostValidationOverrideTag) {
		t.Fatalf("tag not detectable in reason: %q", reason)
	}

	// Simulate the proxy-side Detail decoration used in the non-ALLOWED branch.
	detail := reason
	if strings.Contains(reason, shepherd.PostValidationOverrideTag) {
		detail = "post_validation_override | " + reason
	}
	if !strings.HasPrefix(detail, "post_validation_override | ") {
		t.Fatalf("expected post_validation_override prefix, got %q", detail)
	}
}
