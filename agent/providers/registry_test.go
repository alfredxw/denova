package providers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type stubAdapter struct{ id ProtocolID }

func (adapter stubAdapter) ID() ProtocolID { return adapter.id }

func (stubAdapter) New(context.Context, ModelConfig) (agent.ToolCallingChatModel, error) {
	return stubModel{}, nil
}

type stubListingAdapter struct{ stubAdapter }

func (stubListingAdapter) ListModels(context.Context, ModelConfig) ([]ModelInfo, error) {
	return []ModelInfo{{ID: "suggested-model"}}, nil
}

type stubModel struct{}

func (stubModel) Generate(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.Message, error) {
	return agent.AssistantMessage("ok", nil), nil
}

func (stubModel) Stream(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	return agent.StreamReaderFromArray([]*agent.Message{agent.AssistantMessage("ok", nil)}), nil
}

func (model stubModel) WithTools([]*agent.ToolInfo) (agent.ToolCallingChatModel, error) {
	return model, nil
}

func TestRegistryMergesPresetDefaultsWithoutRestrictingOverrides(t *testing.T) {
	registry := NewRegistry()
	for _, protocol := range []ProtocolID{ProtocolOpenAIChatCompletions, ProtocolOpenAIResponses} {
		if err := registry.RegisterProtocol(stubAdapter{id: protocol}); err != nil {
			t.Fatal(err)
		}
	}
	presetOptions := json.RawMessage(`{"nested":{"preset":true,"value":"default"},"mode":"preset"}`)
	if err := registry.RegisterProviderPreset(ProviderPreset{
		ID: "known", Name: "Known", DefaultProtocol: ProtocolOpenAIChatCompletions,
		Endpoints: map[ProtocolID]EndpointPreset{
			ProtocolOpenAIChatCompletions: {
				BaseURL:         "https://known.example/v1",
				Headers:         map[string]string{"X-Route": "preset", "X-Keep": "yes"},
				ProtocolOptions: presetOptions,
				SessionKeyMapping: &SessionKeyMapping{
					Location: SessionKeyLocationHeader, Name: "x-session-id",
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	resolved, err := registry.Resolve(ModelConfig{
		Provider: "known", Model: "model",
		Headers:         map[string]string{"x-route": "profile"},
		ProtocolOptions: json.RawMessage(`{"nested":{"value":"override"},"mode":"profile"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Protocol != ProtocolOpenAIChatCompletions || resolved.BaseURL != "https://known.example/v1" {
		t.Fatalf("route = %#v", resolved)
	}
	if resolved.Headers["X-Route"] != "profile" || resolved.Headers["X-Keep"] != "yes" || len(resolved.Headers) != 2 {
		t.Fatalf("headers = %#v", resolved.Headers)
	}
	var options map[string]any
	if err := json.Unmarshal(resolved.ProtocolOptions, &options); err != nil {
		t.Fatal(err)
	}
	nested := options["nested"].(map[string]any)
	if nested["preset"] != true || nested["value"] != "override" || options["mode"] != "profile" {
		t.Fatalf("protocol options = %#v", options)
	}
	if resolved.SessionKeyMapping == nil || resolved.SessionKeyMapping.Location != SessionKeyLocationHeader || resolved.SessionKeyMapping.Name != "X-Session-Id" {
		t.Fatalf("session key mapping = %#v", resolved.SessionKeyMapping)
	}

	resolved, err = registry.Resolve(ModelConfig{
		Provider: "known", Model: "model",
		SessionKeyMapping: &SessionKeyMapping{Location: SessionKeyLocationBody, Name: "session_id"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.SessionKeyMapping == nil || resolved.SessionKeyMapping.Location != SessionKeyLocationBody || resolved.SessionKeyMapping.Name != "session_id" {
		t.Fatalf("overridden session key mapping = %#v", resolved.SessionKeyMapping)
	}

	resolved, err = registry.Resolve(ModelConfig{
		Provider: "known", Model: "model",
		SessionKeyMapping: &SessionKeyMapping{Location: SessionKeyLocationNone},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.SessionKeyMapping == nil || resolved.SessionKeyMapping.Location != SessionKeyLocationNone {
		t.Fatalf("disabled session key mapping = %#v", resolved.SessionKeyMapping)
	}

	// A known provider may use an installed protocol absent from its preset as
	// long as the caller supplies the complete route.
	resolved, err = registry.Resolve(ModelConfig{
		Provider: "known", Protocol: ProtocolOpenAIResponses,
		BaseURL: "https://custom.example/api", Model: "model",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.BaseURL != "https://custom.example/api" {
		t.Fatalf("custom route = %#v", resolved)
	}
}

func TestRegistryRejectsInvalidSessionKeyMapping(t *testing.T) {
	registry := NewRegistry()
	if err := registry.RegisterProtocol(stubAdapter{id: ProtocolOpenAIChatCompletions}); err != nil {
		t.Fatal(err)
	}
	err := registry.RegisterProviderPreset(ProviderPreset{
		ID: "invalid", Name: "Invalid", DefaultProtocol: ProtocolOpenAIChatCompletions,
		Endpoints: map[ProtocolID]EndpointPreset{
			ProtocolOpenAIChatCompletions: {
				BaseURL:           "https://invalid.example",
				SessionKeyMapping: &SessionKeyMapping{Location: SessionKeyLocationHeader, Name: "bad header"},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid session key header name") {
		t.Fatalf("registration error = %v", err)
	}
}

func TestRegistryRejectsUnregisteredProvider(t *testing.T) {
	registry := NewRegistry()
	if err := registry.RegisterProtocol(stubAdapter{id: ProtocolAnthropicMessages}); err != nil {
		t.Fatal(err)
	}
	_, err := registry.Resolve(ModelConfig{
		Provider: "private-cloud", Protocol: ProtocolAnthropicMessages,
		BaseURL: "https://private.example", Model: "model",
	})
	if err == nil || !strings.Contains(err.Error(), "has no registered preset") {
		t.Fatalf("unregistered provider error = %v", err)
	}
}

func TestRegistryModelListingIsOptionalAndDoesNotRequireConfiguredModel(t *testing.T) {
	registry := NewRegistry()
	if err := registry.RegisterProtocol(stubListingAdapter{stubAdapter{id: ProtocolOpenAIResponses}}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProtocol(stubAdapter{id: ProtocolAnthropicMessages}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProviderPreset(ProviderPreset{
		ID: "known", Name: "Known", DefaultProtocol: ProtocolOpenAIResponses,
		Endpoints: map[ProtocolID]EndpointPreset{
			ProtocolOpenAIResponses:   {BaseURL: "https://known.example/v1"},
			ProtocolAnthropicMessages: {BaseURL: "https://known.example/anthropic"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	models, resolved, err := registry.ListModelsWithResolvedConfig(context.Background(), ModelConfig{Provider: "known"})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "suggested-model" || resolved.Model != "" {
		t.Fatalf("models = %#v resolved = %#v", models, resolved)
	}
	_, _, err = registry.ListModelsWithResolvedConfig(context.Background(), ModelConfig{
		Provider: "known", Protocol: ProtocolAnthropicMessages,
	})
	if !errors.Is(err, ErrModelListingUnsupported) {
		t.Fatalf("unsupported listing error = %v", err)
	}
}

func TestCatalogIsStableDetachedAndSafeForJSON(t *testing.T) {
	registry := NewRegistry()
	if err := registry.RegisterProtocol(stubAdapter{id: ProtocolOpenAIResponses}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProviderPreset(ProviderPreset{
		ID: "z", Name: "Zulu", DefaultProtocol: ProtocolOpenAIResponses,
		Endpoints: map[ProtocolID]EndpointPreset{ProtocolOpenAIResponses: {
			BaseURL: "https://z.example", Headers: map[string]string{"Authorization": "secret"},
			ProtocolOptions: json.RawMessage(`{"secret":"hidden"}`),
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProviderPreset(ProviderPreset{
		ID: "a", Name: "Alpha", DefaultProtocol: ProtocolOpenAIResponses,
		Endpoints: map[ProtocolID]EndpointPreset{ProtocolOpenAIResponses: {BaseURL: "https://a.example"}},
	}); err != nil {
		t.Fatal(err)
	}

	catalog := registry.Catalog()
	if len(catalog.Providers) != 2 || catalog.Providers[0].ID != "a" || catalog.Providers[1].ID != "z" {
		t.Fatalf("catalog order = %#v", catalog.Providers)
	}
	encoded, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "hidden") {
		t.Fatalf("catalog exposed internal endpoint data: %s", encoded)
	}
	catalog.Providers[1].Endpoints[ProtocolOpenAIResponses] = EndpointPreset{BaseURL: "mutated"}
	if got := registry.Catalog().Providers[1].Endpoints[ProtocolOpenAIResponses].BaseURL; got != "https://z.example" {
		t.Fatalf("catalog mutation leaked into registry: %q", got)
	}
}

func TestDecodeProtocolOptionsRejectsUnknownFields(t *testing.T) {
	var target struct {
		Known string `json:"known"`
	}
	err := DecodeProtocolOptions(json.RawMessage(`{"known":"ok","unknown":true}`), &target)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
	}
}
