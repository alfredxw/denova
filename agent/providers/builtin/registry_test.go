package builtin

import (
	"testing"

	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/protocols/openairesponses"
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
