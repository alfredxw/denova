package builtin

import (
	"testing"

	"github.com/alfredxw/denova/agent/providers"
)

func TestRegistrySeparatesProviderDefaultsFromProtocolSelection(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name         string
		config       providers.ModelConfig
		wantProtocol providers.ProtocolID
		wantBaseURL  string
	}{
		{
			name:         "OpenAI defaults to Responses",
			config:       providers.ModelConfig{Provider: providers.ProviderOpenAI, Model: "gpt-5"},
			wantProtocol: providers.ProtocolOpenAIResponses,
			wantBaseURL:  "https://api.openai.com/v1",
		},
		{
			name:         "OpenAI can explicitly use Chat Completions",
			config:       providers.ModelConfig{Provider: providers.ProviderOpenAI, Protocol: providers.ProtocolOpenAIChatCompletions, Model: "gpt-4.1"},
			wantProtocol: providers.ProtocolOpenAIChatCompletions,
			wantBaseURL:  "https://api.openai.com/v1",
		},
		{
			name:         "DeepSeek defaults to Chat Completions",
			config:       providers.ModelConfig{Provider: providers.ProviderDeepSeek, Model: "deepseek-chat"},
			wantProtocol: providers.ProtocolOpenAIChatCompletions,
			wantBaseURL:  "https://api.deepseek.com",
		},
		{
			name:         "custom compatible endpoint keeps its base URL",
			config:       providers.ModelConfig{Provider: providers.ProviderOpenAICompatible, Protocol: providers.ProtocolOpenAIResponses, Model: "custom", BaseURL: "https://example.test/v1"},
			wantProtocol: providers.ProtocolOpenAIResponses,
			wantBaseURL:  "https://example.test/v1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := registry.Resolve(test.config)
			if err != nil {
				t.Fatal(err)
			}
			if resolved.Protocol != test.wantProtocol || resolved.BaseURL != test.wantBaseURL {
				t.Fatalf("resolved = %#v", resolved)
			}
		})
	}
}

func TestRegistryRejectsUnsupportedProviderProtocolPair(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Resolve(providers.ModelConfig{
		Provider: providers.ProviderDeepSeek,
		Protocol: providers.ProtocolOpenAIResponses,
		Model:    "deepseek-chat",
	})
	if err == nil {
		t.Fatal("expected unsupported provider/protocol error")
	}
}

func TestRegistryRequiresEndpointForCompatibleProvider(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Resolve(providers.ModelConfig{
		Provider: providers.ProviderOpenAICompatible,
		Model:    "custom-model",
	})
	if err == nil {
		t.Fatal("expected compatible provider without a base URL to fail")
	}
}

func TestRegistryRejectsUnknownThinkingLevel(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Resolve(providers.ModelConfig{
		Provider:      providers.ProviderOpenAI,
		Model:         "gpt-5",
		ThinkingLevel: "turbo",
	})
	if err == nil {
		t.Fatal("expected unknown thinking level to fail")
	}
}
