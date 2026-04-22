package shepherd

import (
	"strings"
	"testing"
)

func TestNormalizeShepherdLanguage(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty defaults to english", in: "", want: "en"},
		{name: "simplified chinese", in: "zh", want: "zh"},
		{name: "regional chinese", in: "zh-CN", want: "zh"},
		{name: "traditional chinese variant", in: "zh_Hant", want: "zh"},
		{name: "cn alias", in: "cn", want: "zh"},
		{name: "english region", in: "en-US", want: "en"},
		{name: "unknown language passed through", in: "fr", want: "fr"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeShepherdLanguage(tc.in); got != tc.want {
				t.Fatalf("normalizeShepherdLanguage(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatSecurityMessageLocalizedByLanguage(t *testing.T) {
	sg := &ShepherdGate{}
	sg.SetLanguage("zh_Hant")

	msg := sg.FormatSecurityMessage(&ShepherdDecision{
		Status: "NEEDS_CONFIRMATION",
		Reason: "删除工作区外文件需要确认",
	})

	if !strings.Contains(msg, "[ShepherdGate] 状态: 需要确认 | 原因: 删除工作区外文件需要确认") {
		t.Fatalf("expected chinese formatted message, got %q", msg)
	}
	if strings.Contains(msg, "继续可回复：") {
		t.Fatalf("did not expect reply guide in ShepherdGate analysis message, got %q", msg)
	}
}

func TestFormatSecurityMockReply_Chinese(t *testing.T) {
	sg := &ShepherdGate{}
	sg.SetLanguage("zh")

	msg := sg.FormatSecurityMockReply(&ShepherdDecision{
		Status: "NEEDS_CONFIRMATION",
		Reason: "删除工作区外文件需要确认",
	})
	if !strings.HasPrefix(msg, "[ShepherdGate] :\n") {
		t.Fatalf("expected mock reply to start with [ShepherdGate] :, got %q", msg)
	}
	if !strings.Contains(msg, "需要你先确认后才能继续执行") {
		t.Fatalf("expected chinese intro text, got %q", msg)
	}
	if !strings.Contains(msg, "继续可回复：好的、继续、OK、没问题、确认、可以") {
		t.Fatalf("expected explicit continue keywords in intro, got %q", msg)
	}
	if strings.Count(msg, "继续可回复：") != 1 {
		t.Fatalf("expected exactly one continue guide, got %q", msg)
	}
	if !strings.Contains(msg, "\n\n状态: 需要确认") {
		t.Fatalf("expected analysis block after blank line, got %q", msg)
	}
	if strings.Contains(msg, "\n\n[ShepherdGate] 状态:") {
		t.Fatalf("did not expect repeated [ShepherdGate] prefix in analysis block, got %q", msg)
	}
}

func TestFormatSecurityMockReply_English(t *testing.T) {
	sg := &ShepherdGate{}
	sg.SetLanguage("en")

	msg := sg.FormatSecurityMockReply(&ShepherdDecision{
		Status: "ALLOWED",
		Reason: "test reason",
	})
	if !strings.HasPrefix(msg, "[ShepherdGate] :\n") {
		t.Fatalf("expected mock reply to start with [ShepherdGate] :, got %q", msg)
	}
	if !strings.Contains(msg, "has been blocked by security policy") {
		t.Fatalf("expected english intro text, got %q", msg)
	}
	if !strings.Contains(msg, "\n\nStatus: Allowed | Reason: test reason") {
		t.Fatalf("expected analysis block after blank line, got %q", msg)
	}
	if strings.Contains(msg, "\n\n[ShepherdGate] Status:") {
		t.Fatalf("did not expect repeated [ShepherdGate] prefix in analysis block, got %q", msg)
	}
	if strings.Contains(msg, "Continue replies:") {
		t.Fatalf("did not expect continue guide for ALLOWED status, got %q", msg)
	}
}

func TestIsPromptInjectionRisk(t *testing.T) {
	positives := []string{
		"prompt注入",
		"prompt injection",
		"Prompt Injection Attack",
		"role hijacking",
		"角色劫持",
		"tool result injection",
		"instruction inject",
	}
	for _, rt := range positives {
		if !isPromptInjectionRisk(rt) {
			t.Errorf("expected isPromptInjectionRisk(%q)=true", rt)
		}
	}

	negatives := []string{
		"data_exfiltration",
		"file_access",
		"script_execution",
		"",
		"low risk",
	}
	for _, rt := range negatives {
		if isPromptInjectionRisk(rt) {
			t.Errorf("expected isPromptInjectionRisk(%q)=false", rt)
		}
	}
}

func TestIsHighOrCriticalRisk(t *testing.T) {
	if !isHighOrCriticalRisk("high") {
		t.Error("expected high to be true")
	}
	if !isHighOrCriticalRisk("critical") {
		t.Error("expected critical to be true")
	}
	if isHighOrCriticalRisk("medium") {
		t.Error("expected medium to be false")
	}
	if isHighOrCriticalRisk("low") {
		t.Error("expected low to be false")
	}
	if isHighOrCriticalRisk("") {
		t.Error("expected empty to be false")
	}
}
