package modelio

import (
	"strings"
	"testing"

	"github.com/alfredxw/denova/agent/providers"

	"denova/config"
)

func TestChatModelConfigFromResolvedKeepsNeutralThinkingLevel(t *testing.T) {
	modelCfg, err := ConfigFromResolved(config.ResolvedModelSettings{
		BaseURL:       "https://generativelanguage.googleapis.com/v1beta/openai/",
		Model:         "gemini-3.5-flash",
		ThinkingLevel: "xhigh",
	})
	if err != nil {
		t.Fatal(err)
	}
	if modelCfg.ThinkingLevel != providers.ThinkingLevelXHigh {
		t.Fatalf("thinking level = %q, want xhigh", modelCfg.ThinkingLevel)
	}
}

func TestConfigFromResolvedRejectsUnencodableProtocolOptions(t *testing.T) {
	_, err := ConfigFromResolved(config.ResolvedModelSettings{
		ProtocolOptions: map[string]any{"invalid": func() {}},
	})
	if err == nil || !strings.Contains(err.Error(), "convert model protocol options") {
		t.Fatalf("error = %v", err)
	}
}

func TestChatModelConfigFromResolvedKeepsOffThinkingLevel(t *testing.T) {
	modelCfg, err := ConfigFromResolved(config.ResolvedModelSettings{
		BaseURL:       "https://api.deepseek.com/v1",
		Model:         "deepseek-v4-pro",
		ThinkingLevel: "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	if modelCfg.ThinkingLevel != providers.ThinkingLevelOff {
		t.Fatalf("thinking level = %q, want off", modelCfg.ThinkingLevel)
	}
}
