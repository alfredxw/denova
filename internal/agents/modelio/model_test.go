package modelio

import (
	"strings"
	"testing"

	"github.com/alfredxw/denova/agent/providers"

	"denova/config"
)

func TestChatModelConfigFromResolvedKeepsNeutralThinkingLevel(t *testing.T) {
	mapping := &providers.SessionKeyMapping{Location: providers.SessionKeyLocationHeader, Name: "X-Session-Id"}
	modelCfg, err := ConfigFromResolved(config.ResolvedModelSettings{
		BaseURL:           "https://generativelanguage.googleapis.com/v1beta/openai/",
		Model:             "gemini-3.5-flash",
		ThinkingLevel:     "xhigh",
		SessionKeyMapping: mapping,
	})
	if err != nil {
		t.Fatal(err)
	}
	if modelCfg.ThinkingLevel != providers.ThinkingLevelXHigh {
		t.Fatalf("thinking level = %q, want xhigh", modelCfg.ThinkingLevel)
	}
	if modelCfg.SessionKeyMapping != mapping {
		t.Fatalf("session key mapping = %#v", modelCfg.SessionKeyMapping)
	}
}

func TestConfigFromResolvedKeepsProfileMaxTokens(t *testing.T) {
	maxTokens := 16384
	modelCfg, err := ConfigFromResolved(config.ResolvedModelSettings{MaxTokens: &maxTokens})
	if err != nil {
		t.Fatal(err)
	}
	if modelCfg.MaxOutputTokens == nil || *modelCfg.MaxOutputTokens != maxTokens {
		t.Fatalf("max output tokens = %#v, want %d", modelCfg.MaxOutputTokens, maxTokens)
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

func TestLoggableModelBaseURLRemovesCredentialsAndQuery(t *testing.T) {
	got := loggableModelBaseURL("https://user:password@api.example.test/v1?api_key=secret#fragment")
	if got != "https://api.example.test/v1" {
		t.Fatalf("loggable base URL = %q", got)
	}
	if got := loggableModelBaseURL("not a URL?api_key=secret"); got != "<invalid>" {
		t.Fatalf("invalid loggable base URL = %q", got)
	}
}
