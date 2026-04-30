package proxy

import "testing"

func TestFormatSecurityFlowLog(t *testing.T) {
	got := formatSecurityFlowLog(securityFlowStageToolCallResult, "decision=%s count=%d", "ALLOW", 2)
	want := "[ShepherdGate][Flow][tool_call_result] decision=ALLOW count=2"
	if got != want {
		t.Fatalf("unexpected security flow log:\nwant: %s\n got: %s", want, got)
	}
}
