package proxy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go_lib/core/repository"
	"go_lib/core/shepherd"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/openai/openai-go"
)

type stubChatModelForProxy struct {
	generateResp *schema.Message
	generateErr  error
	called       bool
	messages     []*schema.Message
}

func (m *stubChatModelForProxy) Generate(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.called = true
	m.messages = append([]*schema.Message(nil), messages...)
	if m.generateErr != nil {
		return nil, m.generateErr
	}
	if m.generateResp != nil {
		return m.generateResp, nil
	}
	return &schema.Message{}, nil
}

func (m *stubChatModelForProxy) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("not implemented in tests")
}

func (m *stubChatModelForProxy) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

func TestArmPendingRecoveryFromContext_Confirm(t *testing.T) {
	requestID := "req_recovery_confirm"
	pp := &ProxyProtection{
		shepherdGate: shepherd.NewShepherdGateForTesting(
			&stubChatModelForProxy{
				generateResp: &schema.Message{
					Content: `{"intent":"confirm","reason":"用户已明确确认继续执行。","usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14}}`,
				},
			},
			"zh",
			&repository.SecurityModelConfig{Model: "MiniMax-M2.5"},
		),
	}
	prepareTestSecurityChain(t, pp, requestID)
	pp.storePendingToolCallRecoveryWithIDsForRequest(requestID, []openai.ChatCompletionMessageToolCall{
		{
			ID:   "call_1",
			Type: "function",
			Function: openai.ChatCompletionMessageToolCallFunction{
				Name:      "delete_email",
				Arguments: `{"email_id":"m1"}`,
			},
		},
	}, nil, "", "delete action requires confirmation", "test")

	ok := pp.armPendingRecoveryFromContext(context.Background(), requestID, []ConversationMessage{
		{Role: "assistant", Content: "[ShepherdGate] 状态: NEEDS_CONFIRMATION"},
		{Role: "user", Content: "确定，继续执行"},
	})
	if !ok {
		t.Fatalf("expected recovery to be armed by security agent confirmation")
	}
	if !pendingRecoveryArmedForTest(t, pp, requestID) {
		t.Fatalf("expected pendingRecoveryArmed=true")
	}
}

func TestArmPendingRecoveryFromContext_Reject(t *testing.T) {
	requestID := "req_recovery_reject"
	pp := &ProxyProtection{
		shepherdGate: shepherd.NewShepherdGateForTesting(
			&stubChatModelForProxy{
				generateResp: &schema.Message{
					Content: `{"intent":"REJECT","reason":"用户明确取消执行。","usage":{"prompt_tokens":10,"completion_tokens":6,"total_tokens":16}}`,
				},
			},
			"zh",
			&repository.SecurityModelConfig{Model: "MiniMax-M2.5"},
		),
	}
	prepareTestSecurityChain(t, pp, requestID)
	pp.storePendingToolCallRecoveryWithIDsForRequest(requestID, []openai.ChatCompletionMessageToolCall{{ID: "call_1"}}, nil, "", "risk", "test")

	ok := pp.armPendingRecoveryFromContext(context.Background(), requestID, []ConversationMessage{
		{Role: "assistant", Content: "[ShepherdGate] 状态: NEEDS_CONFIRMATION"},
		{Role: "user", Content: "取消，不要执行"},
	})
	if ok {
		t.Fatalf("expected reject to prevent arming")
	}
	if pendingRecoveryForTest(t, pp, requestID) != nil {
		t.Fatalf("expected pending recovery cleared on reject")
	}
}

func TestPendingToolRecoveryArming(t *testing.T) {
	pp := &ProxyProtection{}
	requestID := "req_pending_recovery"
	prepareTestSecurityChain(t, pp, requestID)

	toolCalls := []openai.ChatCompletionMessageToolCall{
		{
			ID:   "call_1",
			Type: "function",
			Function: openai.ChatCompletionMessageToolCallFunction{
				Name:      "delete_email",
				Arguments: `{"email_id":"a1"}`,
			},
		},
	}
	pp.storePendingToolCallRecoveryForRequest(requestID, toolCalls, "assistant tool call", "risk reason", "non_stream")

	// Verify recovery is stored but not armed
	if pendingRecoveryForTest(t, pp, requestID) == nil {
		t.Fatalf("expected pending recovery to be stored")
	}
	if pendingRecoveryArmedForTest(t, pp, requestID) {
		t.Fatalf("expected pending recovery NOT to be armed yet")
	}

	// Simulate arming (user confirmation would trigger this)
	chainID := pp.chainIDForRequest(requestID)
	pp.chainMu.Lock()
	pp.chains[chainID].PendingRecoveryArmed = true
	pp.chainMu.Unlock()

	// Verify armed state
	armed := pendingRecoveryArmedForTest(t, pp, requestID)
	if !armed {
		t.Fatalf("expected pending recovery to be armed")
	}

	// Clear recovery (as onRequest would do when armed)
	pp.clearPendingToolCallRecoveryForRequest(requestID)

	if pendingRecoveryForTest(t, pp, requestID) != nil {
		t.Fatalf("expected pending recovery to be cleared")
	}
	if pendingRecoveryArmedForTest(t, pp, requestID) {
		t.Fatalf("expected armed flag to be cleared")
	}
}

func TestRiskTypeFromRecoveryPrompt_RoundTripsFormattedRiskTypes(t *testing.T) {
	cases := []struct {
		name     string
		riskType string
		lang     string
	}{
		{name: "direct prompt injection zh", riskType: riskPromptInjectionDirect, lang: "zh"},
		{name: "indirect prompt injection en", riskType: riskPromptInjectionIndirect, lang: "en"},
		{name: "sensitive data zh", riskType: riskSensitiveDataExfil, lang: "zh"},
		{name: "high risk en", riskType: riskHighRiskOperation, lang: "en"},
		{name: "privilege abuse zh", riskType: riskPrivilegeAbuse, lang: "zh"},
		{name: "unexpected code execution en", riskType: riskUnexpectedCodeExecution, lang: "en"},
		{name: "context poisoning zh", riskType: riskContextPoisoning, lang: "zh"},
		{name: "supply chain en", riskType: riskSupplyChain, lang: "en"},
		{name: "human trust zh", riskType: riskHumanTrustExploitation, lang: "zh"},
		{name: "cascading failure en", riskType: riskCascadingFailure, lang: "en"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sg := shepherd.NewShepherdGateForTesting(nil, tc.lang, nil)
			msg := sg.FormatSecurityMockReply(&shepherd.ShepherdDecision{
				Status:   "NEEDS_CONFIRMATION",
				Reason:   "risk requires confirmation",
				RiskType: tc.riskType,
			})

			if got := riskTypeFromRecoveryPrompt(msg); got != tc.riskType {
				t.Fatalf("riskTypeFromRecoveryPrompt() = %q, want %q; msg=%q", got, tc.riskType, msg)
			}
		})
	}
}

func TestRiskTypeFromRecoveryPrompt_RequiresStructuredRiskTypeForAliases(t *testing.T) {
	negativePrompts := []string{
		"[ShepherdGate] :\n该操作存在风险，需要你先确认后才能继续执行。\n\n状态: 需要确认 | 原因: 这个脚本执行效率很高",
		"[ShepherdGate] :\n该操作存在风险，需要你先确认后才能继续执行。\n\n状态: 需要确认 | 原因: 代码执行速度测试",
		"[ShepherdGate] :\nThis action is risky and requires your confirmation before continuing.\n\nStatus: Needs Confirmation | Reason: script execution is fast",
	}
	for _, prompt := range negativePrompts {
		if got := riskTypeFromRecoveryPrompt(prompt); got != "" {
			t.Fatalf("expected no risk type for incidental phrase, got %q; prompt=%q", got, prompt)
		}
	}

	aliasPrompts := map[string]string{
		"[ShepherdGate] :\n状态: 需要确认 | 原因: 执行 Python 脚本需要确认\n风险类型: 脚本执行风险":                                     riskUnexpectedCodeExecution,
		"[ShepherdGate] :\nStatus: Needs Confirmation | Reason: run script\nRisk Type: Script Execution Risk": riskUnexpectedCodeExecution,
	}
	for prompt, want := range aliasPrompts {
		if got := riskTypeFromRecoveryPrompt(prompt); got != want {
			t.Fatalf("riskTypeFromRecoveryPrompt() = %q, want %q; prompt=%q", got, want, prompt)
		}
	}
}

func TestArmPendingRecoveryFromContext_FormattedRiskTypeGrantsMatchingToolCallOnce(t *testing.T) {
	riskTypes := []string{
		riskPromptInjectionDirect,
		riskPromptInjectionIndirect,
		riskSensitiveDataExfil,
		riskHighRiskOperation,
		riskPrivilegeAbuse,
		riskUnexpectedCodeExecution,
		riskContextPoisoning,
		riskSupplyChain,
		riskHumanTrustExploitation,
		riskCascadingFailure,
	}

	for _, riskType := range riskTypes {
		t.Run(riskType, func(t *testing.T) {
			requestID := "req_" + strings.ToLower(riskType)
			sg := shepherd.NewShepherdGateForTesting(nil, "zh", nil)
			prompt := sg.FormatSecurityMockReply(&shepherd.ShepherdDecision{
				Status:   "NEEDS_CONFIRMATION",
				Reason:   "risk requires confirmation",
				RiskType: riskType,
			})
			pp := &ProxyProtection{shepherdGate: sg}
			prepareTestSecurityChain(t, pp, requestID)
			pp.storePendingToolCallRecoveryWithRiskForRequest(requestID, nil, nil, prompt, "risk requires confirmation", "user_input", "")

			if !pp.armPendingRecoveryFromContext(context.Background(), requestID, []ConversationMessage{
				{Role: "assistant", Content: prompt},
				{Role: "user", Content: "继续"},
			}) {
				t.Fatalf("expected formatted risk prompt to arm recovery")
			}
			if !pp.consumeConfirmedToolCallGrantForRequest(requestID, securityPolicyDecision{
				Action:   decisionActionNeedsConfirm,
				RiskType: riskType,
			}) {
				t.Fatalf("expected matching risk type %s to consume confirmation grant", riskType)
			}
			if pp.consumeConfirmedToolCallGrantForRequest(requestID, securityPolicyDecision{
				Action:   decisionActionNeedsConfirm,
				RiskType: riskType,
			}) {
				t.Fatalf("expected confirmation grant for %s to be one-shot", riskType)
			}
		})
	}
}
