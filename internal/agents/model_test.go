package agents

import (
	"testing"

	"github.com/alfredxw/denova/agent/providers"

	"denova/config"
)

func TestChatModelConfigFromResolvedKeepsNeutralThinkingLevel(t *testing.T) {
	modelCfg := chatModelConfigFromResolved(config.ResolvedModelSettings{
		OpenAIBaseURL: "https://generativelanguage.googleapis.com/v1beta/openai/",
		OpenAIModel:   "gemini-3.5-flash",
		ThinkingLevel: "xhigh",
	})
	if modelCfg.ThinkingLevel != providers.ThinkingLevelXHigh {
		t.Fatalf("thinking level = %q, want xhigh", modelCfg.ThinkingLevel)
	}
}

func TestChatModelConfigFromResolvedKeepsOffThinkingLevel(t *testing.T) {
	modelCfg := chatModelConfigFromResolved(config.ResolvedModelSettings{
		OpenAIBaseURL: "https://api.deepseek.com/v1",
		OpenAIModel:   "deepseek-v4-pro",
		ThinkingLevel: "off",
	})
	if modelCfg.ThinkingLevel != providers.ThinkingLevelOff {
		t.Fatalf("thinking level = %q, want off", modelCfg.ThinkingLevel)
	}
}
