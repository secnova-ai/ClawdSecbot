package shepherd

import (
	"context"
	"strings"
	"testing"
)

func TestFormatSecurityMessage_Localized(t *testing.T) {
	sg := &ShepherdGate{language: "zh"}

	msg := sg.FormatSecurityMessage(&ShepherdDecision{
		Status: "NEEDS_CONFIRMATION",
		Reason: "script execution requires confirmation",
	})
	if !strings.Contains(msg, "状态: 需要确认") {
		t.Fatalf("expected localized status label, got: %s", msg)
	}
	if !strings.Contains(msg, "script execution requires confirmation") {
		t.Fatalf("expected original English reason preserved, got: %s", msg)
	}
	if strings.Contains(msg, "继续可回复：") {
		t.Fatalf("did not expect reply guide inside ShepherdGate analysis block, got: %s", msg)
	}
}

func TestEvaluateRecoveryIntent_ConfirmByKeyword(t *testing.T) {
	sg := &ShepherdGate{
		language: "zh",
	}

	got, err := sg.EvaluateRecoveryIntent(context.Background(),
		[]ConversationMessage{
			{Role: "assistant", Content: "[ShepherdGate] 状态: NEEDS_CONFIRMATION"},
			{Role: "user", Content: "确定，继续"},
		},
		[]ToolCallInfo{{Name: "bash_execute", RawArgs: `{"command":"echo hi"}`}},
		"script requires confirmation",
	)
	if err != nil {
		t.Fatalf("EvaluateRecoveryIntent returned error: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil decision")
	}
	if got.Intent != "CONFIRM" {
		t.Fatalf("expected CONFIRM intent, got=%s", got.Intent)
	}
	if got.Usage != nil {
		t.Fatalf("expected nil usage for keyword-based decision, got=%+v", got.Usage)
	}
}

func TestEvaluateRecoveryIntent_RejectHasPriority(t *testing.T) {
	sg := &ShepherdGate{
		language: "zh",
	}

	got, err := sg.EvaluateRecoveryIntent(context.Background(),
		[]ConversationMessage{
			{Role: "assistant", Content: "[ShepherdGate] 状态: NEEDS_CONFIRMATION"},
			{Role: "user", Content: "不要继续，取消执行"},
		},
		[]ToolCallInfo{{Name: "bash_execute", RawArgs: `{"command":"rm -rf /tmp/x"}`}},
		"script requires confirmation",
	)
	if err != nil {
		t.Fatalf("EvaluateRecoveryIntent returned error: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil decision")
	}
	if got.Intent != "REJECT" {
		t.Fatalf("expected REJECT intent, got=%s", got.Intent)
	}
}

func TestEvaluateRecoveryIntent_MultilingualKeywords(t *testing.T) {
	sg := &ShepherdGate{
		language: "en",
	}

	got, err := sg.EvaluateRecoveryIntent(context.Background(),
		[]ConversationMessage{
			{Role: "assistant", Content: "[ShepherdGate] Status: NEEDS_CONFIRMATION"},
			{Role: "user", Content: "好的，继续"},
		},
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("EvaluateRecoveryIntent returned error: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil decision")
	}
	if got.Intent != "CONFIRM" {
		t.Fatalf("expected CONFIRM for multilingual keyword, got=%s", got.Intent)
	}
}

func TestEvaluateRecoveryIntent_NoProblemShouldConfirm(t *testing.T) {
	sg := &ShepherdGate{
		language: "en",
	}

	got, err := sg.EvaluateRecoveryIntent(context.Background(),
		[]ConversationMessage{
			{Role: "assistant", Content: "[ShepherdGate] Status: NEEDS_CONFIRMATION"},
			{Role: "user", Content: "no problem, continue"},
		},
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("EvaluateRecoveryIntent returned error: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil decision")
	}
	if got.Intent != "CONFIRM" {
		t.Fatalf("expected CONFIRM for 'no problem, continue', got=%s", got.Intent)
	}
}

func TestEvaluateRecoveryIntent_OutOfScopeShouldBeNone(t *testing.T) {
	sg := &ShepherdGate{
		language: "en",
	}

	got, err := sg.EvaluateRecoveryIntent(context.Background(),
		[]ConversationMessage{
			{Role: "assistant", Content: "normal assistant reply"},
			{Role: "user", Content: "ok, please summarize this file"},
		},
		[]ToolCallInfo{{Name: "bash_execute", RawArgs: `{"command":"rm -rf /tmp/x"}`}},
		"script requires confirmation",
	)
	if err != nil {
		t.Fatalf("EvaluateRecoveryIntent returned error: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil decision")
	}
	if got.Intent != "NONE" {
		t.Fatalf("expected NONE for out-of-scope user message, got=%s", got.Intent)
	}
}

func TestNormalizeShepherdLanguage_ZhVariants(t *testing.T) {
	cases := []string{"zh", "zh_CN", "zh-CN", "zh-Hans", "ZH_hant", "cn", "Chinese"}
	for _, c := range cases {
		if got := normalizeShepherdLanguage(c); got != "zh" {
			t.Fatalf("expected zh for %q, got %q", c, got)
		}
	}
}
