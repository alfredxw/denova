package builtin

import (
	"encoding/json"
	"testing"

	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/protocols/anthropicmessages"
	"github.com/alfredxw/denova/agent/providers/protocols/openaichatcompletions"
	"github.com/alfredxw/denova/agent/providers/protocols/openairesponses"
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

func TestRegistrySupportsDeepSeekResponses(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := registry.Resolve(providers.ModelConfig{
		Provider: providers.ProviderDeepSeek,
		Protocol: providers.ProtocolOpenAIResponses,
		Model:    "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("base URL = %q", resolved.BaseURL)
	}
	var compatibility openairesponses.Compatibility
	if err := providers.DecodeProtocolOptions(resolved.ProtocolOptions, &compatibility); err != nil {
		t.Fatal(err)
	}
	if compatibility.Store != openairesponses.StoreModeOmit || compatibility.ReasoningSummary != openairesponses.ReasoningSummaryOmit || compatibility.IncludeEncryptedReasoning {
		t.Fatalf("DeepSeek Responses compatibility = %#v", compatibility)
	}
}

func TestDeepSeekPresetsKeepModelAwareXHighEffort(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, protocol := range []providers.ProtocolID{
		providers.ProtocolOpenAIChatCompletions,
		providers.ProtocolOpenAIResponses,
		providers.ProtocolAnthropicMessages,
	} {
		t.Run(string(protocol), func(t *testing.T) {
			resolved, err := registry.Resolve(providers.ModelConfig{
				Provider: providers.ProviderDeepSeek,
				Protocol: protocol,
				Model:    "deepseek-v4-pro",
			})
			if err != nil {
				t.Fatal(err)
			}
			var options struct {
				EffortMap map[string]string `json:"effort_map"`
			}
			if err := json.Unmarshal(resolved.ProtocolOptions, &options); err != nil {
				t.Fatal(err)
			}
			if mapped, exists := options.EffortMap["xhigh"]; exists {
				t.Fatalf("xhigh must reach DeepSeek unchanged for model-aware mapping, got %q", mapped)
			}
			if options.EffortMap["minimal"] != "low" || options.EffortMap["medium"] != "high" {
				t.Fatalf("unsupported neutral efforts were not normalized: %#v", options.EffortMap)
			}
		})
	}
}

func TestMiniMaxPresetsPreferAnthropicWithExplicitChatFallback(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		provider providers.ProviderID
		baseURL  string
	}{
		{provider: "minimax", baseURL: "https://api.minimax.io/anthropic"},
		{provider: "minimax-cn", baseURL: "https://api.minimaxi.com/anthropic"},
	} {
		t.Run(string(test.provider), func(t *testing.T) {
			resolved, err := registry.Resolve(providers.ModelConfig{Provider: test.provider, Model: "MiniMax-M3"})
			if err != nil {
				t.Fatal(err)
			}
			if resolved.Protocol != providers.ProtocolAnthropicMessages || resolved.BaseURL != test.baseURL {
				t.Fatalf("resolved route = %s %q", resolved.Protocol, resolved.BaseURL)
			}
			var compatibility anthropicmessages.Compatibility
			if err := providers.DecodeProtocolOptions(resolved.ProtocolOptions, &compatibility); err != nil {
				t.Fatal(err)
			}
			if compatibility.ThinkingMode != anthropicmessages.ThinkingModeAdaptive ||
				compatibility.SupportsEffort == nil || *compatibility.SupportsEffort {
				t.Fatalf("Anthropic compatibility = %#v", compatibility)
			}
		})
	}

	chat, err := registry.Resolve(providers.ModelConfig{
		Provider: "minimax", Protocol: providers.ProtocolOpenAIChatCompletions, Model: "MiniMax-M3",
	})
	if err != nil {
		t.Fatal(err)
	}
	var chatCompatibility openaichatcompletions.Compatibility
	if err := providers.DecodeProtocolOptions(chat.ProtocolOptions, &chatCompatibility); err != nil {
		t.Fatal(err)
	}
	if chatCompatibility.ThinkingToggle != openaichatcompletions.ThinkingToggleAdaptive ||
		chatCompatibility.SupportsReasoningEffort == nil || *chatCompatibility.SupportsReasoningEffort ||
		chatCompatibility.MaxTokensField != openaichatcompletions.MaxTokensFieldMaxCompletionTokens {
		t.Fatalf("Chat compatibility = %#v", chatCompatibility)
	}
}

func TestRegistryAllowsCustomProviderWithAnyInstalledProtocol(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := registry.Resolve(providers.ModelConfig{
		Provider: "private-gateway",
		Protocol: providers.ProtocolGoogleGenerativeAI,
		BaseURL:  "https://models.example.test",
		Model:    "private-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Provider != "private-gateway" || resolved.Protocol != providers.ProtocolGoogleGenerativeAI {
		t.Fatalf("resolved = %#v", resolved)
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
