package builtin

import (
	"testing"

	"github.com/alfredxw/denova/agent/model/providers"
	"github.com/alfredxw/denova/agent/model/providers/protocols/openairesponses"
)

func TestOpenAIResponsesPresetEnablesStatelessAllTurnReasoning(t *testing.T) {
	presets, err := providerPresets()
	if err != nil {
		t.Fatal(err)
	}
	for _, preset := range presets {
		if preset.ID != providers.ProviderOpenAI {
			continue
		}
		endpoint, ok := preset.Endpoints[providers.ProtocolOpenAIResponses]
		if !ok {
			t.Fatal("OpenAI preset has no Responses endpoint")
		}
		var compatibility openairesponses.Compatibility
		if err := providers.DecodeProtocolOptions(endpoint.ProtocolOptions, &compatibility); err != nil {
			t.Fatal(err)
		}
		if compatibility.Store != openairesponses.StoreModeFalse ||
			!compatibility.IncludeEncryptedReasoning ||
			compatibility.ReasoningContext != openairesponses.ReasoningContextAllTurns {
			t.Fatalf("OpenAI Responses continuation preset = %#v", compatibility)
		}
		return
	}
	t.Fatal("OpenAI provider preset is missing")
}

func TestRuncratePresetUsesChatCompletions(t *testing.T) {
	presets, err := providerPresets()
	if err != nil {
		t.Fatal(err)
	}
	for _, preset := range presets {
		if preset.ID != "runcrate" {
			continue
		}
		if preset.DefaultProtocol != providers.ProtocolOpenAIChatCompletions {
			t.Fatalf("Runcrate default protocol = %q", preset.DefaultProtocol)
		}
		endpoint, ok := preset.Endpoints[providers.ProtocolOpenAIChatCompletions]
		if !ok {
			t.Fatal("Runcrate preset has no Chat Completions endpoint")
		}
		if endpoint.BaseURL != "https://api.runcrate.ai/v1" {
			t.Fatalf("Runcrate base URL = %q", endpoint.BaseURL)
		}
		return
	}
	t.Fatal("Runcrate provider preset is missing")
}
