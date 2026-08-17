package providers

import "testing"

func TestLookupModelLimitsRecognizesDeepSeekV4AndCurrentAliases(t *testing.T) {
	for _, model := range []string{"deepseek-v4-flash", "deepseek-v4-pro", "deepseek-chat", "deepseek-reasoner"} {
		t.Run(model, func(t *testing.T) {
			limits, ok := LookupModelLimits(ProviderDeepSeek, model)
			if !ok || limits.MaxOutputTokens != 384*1024 {
				t.Fatalf("limits for %q = %#v, %t", model, limits, ok)
			}
		})
	}
}

func TestLookupModelLimitsRecognizesMiniMaxM2Family(t *testing.T) {
	for _, provider := range []ProviderID{ProviderMiniMax, ProviderMiniMaxCN} {
		for _, model := range []string{"MiniMax-M2", "MiniMax-M2.7", "MiniMax-M2.7-highspeed"} {
			limits, ok := LookupModelLimits(provider, model)
			if !ok || limits.MaxOutputTokens != 128*1024 {
				t.Fatalf("limits for %s/%s = %#v, %t", provider, model, limits, ok)
			}
		}
	}
}

func TestLookupModelLimitsDistinguishesClaude128KAnd64KFamilies(t *testing.T) {
	for _, model := range []string{"claude-fable-5", "claude-opus-5", "claude-sonnet-5", "claude-opus-4-8", "claude-sonnet-4-6"} {
		limits, ok := LookupModelLimits(ProviderAnthropic, model)
		if !ok || limits.MaxOutputTokens != 128*1024 {
			t.Fatalf("limits for %s = %#v, %t", model, limits, ok)
		}
	}
	for _, model := range []string{"claude-haiku-4-5", "claude-sonnet-4-5", "claude-opus-4-5"} {
		if limits, ok := LookupModelLimits(ProviderAnthropic, model); ok {
			t.Fatalf("64K model %s must keep the protocol fallback, got %#v", model, limits)
		}
	}
}

func TestLookupModelLimitsLeavesUnknownAndCustomModelsToTheirEndpoint(t *testing.T) {
	for _, test := range []struct {
		provider ProviderID
		model    string
	}{
		{provider: ProviderDeepSeek, model: "future-model"},
		{provider: ProviderOpenAICompatible, model: "deepseek-v4-pro"},
		{provider: ProviderOpenAI, model: "gpt-5"},
		{provider: ProviderMiniMax, model: "MiniMax-Text-01"},
	} {
		if limits, ok := LookupModelLimits(test.provider, test.model); ok {
			t.Fatalf("unexpected limits for %s/%s: %#v", test.provider, test.model, limits)
		}
	}
}
