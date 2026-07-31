// Package builtin assembles Denova's built-in provider catalog. Provider
// identity and protocol implementations remain separate registrations so a
// provider may support more than one wire protocol.
package builtin

import (
	"fmt"

	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/protocols/openaichatcompletions"
	"github.com/alfredxw/denova/agent/providers/protocols/openairesponses"
)

// NewRegistry returns a fully validated, independent built-in registry.
func NewRegistry() (*providers.Registry, error) {
	registry := providers.NewRegistry()
	for _, adapter := range []providers.ProtocolAdapter{
		openaichatcompletions.NewAdapter(),
		openairesponses.NewAdapter(),
	} {
		if err := registry.RegisterProtocol(adapter); err != nil {
			return nil, fmt.Errorf("build provider registry: %w", err)
		}
	}

	definitions := []providers.Provider{
		{
			ID:              providers.ProviderOpenAI,
			Name:            "OpenAI",
			DefaultBaseURL:  "https://api.openai.com/v1",
			DefaultProtocol: providers.ProtocolOpenAIResponses,
			Protocols: []providers.ProtocolID{
				providers.ProtocolOpenAIResponses,
				providers.ProtocolOpenAIChatCompletions,
			},
		},
		{
			ID:              providers.ProviderDeepSeek,
			Name:            "DeepSeek",
			DefaultBaseURL:  "https://api.deepseek.com",
			DefaultProtocol: providers.ProtocolOpenAIChatCompletions,
			Protocols: []providers.ProtocolID{
				providers.ProtocolOpenAIChatCompletions,
			},
		},
		{
			ID:              providers.ProviderOpenAICompatible,
			Name:            "OpenAI Compatible",
			DefaultProtocol: providers.ProtocolOpenAIChatCompletions,
			Protocols: []providers.ProtocolID{
				providers.ProtocolOpenAIChatCompletions,
				providers.ProtocolOpenAIResponses,
			},
		},
	}
	for _, definition := range definitions {
		if err := registry.RegisterProvider(definition); err != nil {
			return nil, fmt.Errorf("build provider registry: %w", err)
		}
	}
	return registry, nil
}
