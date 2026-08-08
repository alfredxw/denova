package modelio

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"

	"denova/config"
)

// ConfigForAgent resolves the provider-neutral model configuration for one
// Agent kind.
func ConfigForAgent(cfg *config.Config, agentKind string) (providers.ModelConfig, error) {
	return ConfigFromResolved(config.ResolveAgentModel(cfg, agentKind))
}

func ConfigFromResolved(resolved config.ResolvedModelSettings) (providers.ModelConfig, error) {
	provider := providers.ProviderID(resolved.Provider)
	protocol := providers.ProtocolID(resolved.Protocol)
	if provider == "" {
		provider = providers.ProviderOpenAICompatible
	}
	modelConfig := providers.ModelConfig{
		Provider:          provider,
		Protocol:          protocol,
		APIKey:            resolved.APIKey,
		Model:             resolved.Model,
		BaseURL:           resolved.BaseURL,
		Headers:           resolved.Headers,
		SessionKeyMapping: resolved.SessionKeyMapping,
		ThinkingLevel:     providers.ThinkingLevel(resolved.ThinkingLevel),
	}
	if len(resolved.ProtocolOptions) != 0 {
		options, err := providers.EncodeProtocolOptions(resolved.ProtocolOptions)
		if err != nil {
			return providers.ModelConfig{}, fmt.Errorf("convert model protocol options: %w", err)
		}
		modelConfig.ProtocolOptions = options
	}
	if resolved.Temperature != nil {
		temperature := float32(*resolved.Temperature)
		modelConfig.Temperature = &temperature
	}
	return modelConfig, nil
}

func NewChatModel(ctx context.Context, config providers.ModelConfig) (agent.ToolCallingChatModel, error) {
	return defaultRuntime.NewChatModel(ctx, config)
}

func WithJSONObjectOutput(config providers.ModelConfig) providers.ModelConfig {
	config.OutputFormat = &providers.OutputFormat{Type: providers.OutputFormatJSONObject}
	return config
}
