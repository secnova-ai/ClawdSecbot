package adapter

import "testing"

func TestEffectiveRoutingProvider(t *testing.T) {
	if got := EffectiveRoutingProvider(ProviderMiniMaxCN); got != ProviderMiniMax {
		t.Fatalf("EffectiveRoutingProvider(minimax_cn) = %q, want minimax", got)
	}
	if got := EffectiveRoutingProvider(ProviderMoonshotCN); got != ProviderMoonshot {
		t.Fatalf("EffectiveRoutingProvider(moonshot_cn) = %q, want moonshot", got)
	}
	if got := EffectiveRoutingProvider(ProviderAnthropicCompatible); got != ProviderAnthropic {
		t.Fatalf("EffectiveRoutingProvider(anthropic_compatible) = %q, want anthropic", got)
	}
	if got := EffectiveRoutingProvider(ProviderZhipuEN); got != ProviderZhipu {
		t.Fatalf("EffectiveRoutingProvider(zhipu_en) = %q, want zhipu", got)
	}
	if got := EffectiveRoutingProvider(ProviderOpenAICompatible); got != ProviderOpenAICompatible {
		t.Fatalf("EffectiveRoutingProvider(openai_compatible) = %q, want openai_compatible", got)
	}
}

func TestIsOpenAICompatible_MiniMaxNotOpenAI(t *testing.T) {
	if IsOpenAICompatible(ProviderMiniMax) {
		t.Fatal("MiniMax must not be OpenAI-compatible for routing")
	}
	if IsOpenAICompatible(ProviderMiniMaxCN) {
		t.Fatal("MiniMax CN must not be OpenAI-compatible for routing")
	}
	if !IsOpenAICompatible(ProviderOpenAI) {
		t.Fatal("OpenAI must be OpenAI-compatible")
	}
	if IsOpenAICompatible(ProviderAnthropicCompatible) {
		t.Fatal("anthropic_compatible must not be OpenAI-compatible")
	}
}

func TestBuildEndpointURL(t *testing.T) {
	tests := []struct {
		name     string
		provider ProviderName
		baseURL  string
		want     string
	}{
		// MiniMax: 旧版 baseURL（无 /v1）应自动补全
		{
			name:     "MiniMax old baseURL without /v1",
			provider: ProviderMiniMax,
			baseURL:  "https://api.minimax.io/anthropic",
			want:     "https://api.minimax.io/anthropic/v1/messages",
		},
		// MiniMax: 新版 baseURL（含 /v1）
		{
			name:     "MiniMax new baseURL with /v1",
			provider: ProviderMiniMax,
			baseURL:  "https://api.minimax.io/anthropic/v1",
			want:     "https://api.minimax.io/anthropic/v1/messages",
		},
		// MiniMax: 已包含完整路径不重复追加
		{
			name:     "MiniMax full endpoint URL unchanged",
			provider: ProviderMiniMax,
			baseURL:  "https://api.minimax.io/anthropic/v1/messages",
			want:     "https://api.minimax.io/anthropic/v1/messages",
		},
		// MiniMax: 尾部斜杠应被清理
		{
			name:     "MiniMax baseURL with trailing slash",
			provider: ProviderMiniMax,
			baseURL:  "https://api.minimax.io/anthropic/",
			want:     "https://api.minimax.io/anthropic/v1/messages",
		},
		// Anthropic: 标准 baseURL 不受影响
		{
			name:     "Anthropic standard baseURL",
			provider: ProviderAnthropic,
			baseURL:  "https://api.anthropic.com/v1",
			want:     "https://api.anthropic.com/v1/messages",
		},
		// Anthropic: 已包含完整路径不重复追加
		{
			name:     "Anthropic full endpoint URL unchanged",
			provider: ProviderAnthropic,
			baseURL:  "https://api.anthropic.com/v1/messages",
			want:     "https://api.anthropic.com/v1/messages",
		},
		// OpenAI: 标准路径
		{
			name:     "OpenAI standard baseURL",
			provider: ProviderOpenAI,
			baseURL:  "https://api.openai.com/v1",
			want:     "https://api.openai.com/v1/chat/completions",
		},
		// 空 baseURL 返回空字符串
		{
			name:     "empty baseURL returns empty",
			provider: ProviderMiniMax,
			baseURL:  "",
			want:     "",
		},
		{
			name:     "MiniMax CN region provider uses same Anthropic path rules",
			provider: ProviderMiniMaxCN,
			baseURL:  "https://api.minimaxi.com/anthropic",
			want:     "https://api.minimaxi.com/anthropic/v1/messages",
		},
		{
			name:     "Anthropic-compatible template routes like Anthropic",
			provider: ProviderAnthropicCompatible,
			baseURL:  "https://example.com/v1",
			want:     "https://example.com/v1/messages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildEndpointURL(tt.provider, tt.baseURL)
			if got != tt.want {
				t.Errorf("BuildEndpointURL(%q, %q) = %q, want %q", tt.provider, tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestProviderARKMetadata(t *testing.T) {
	info := GetProviderInfo(ProviderARK)
	if info == nil {
		t.Fatal("ProviderARK metadata not found")
	}
	if info.NeedsEndpoint {
		t.Fatal("ProviderARK should not require endpoint field")
	}
	if info.DefaultModel == "" {
		t.Fatal("ProviderARK should provide default model hint value")
	}
}
