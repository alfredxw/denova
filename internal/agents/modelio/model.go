package modelio

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/builtin"

	"denova/config"
)

// ConfigForAgent resolves the provider-neutral model configuration for one
// Agent kind.
func ConfigForAgent(cfg *config.Config, agentKind string) providers.ModelConfig {
	return ConfigFromResolved(config.ResolveAgentModel(cfg, agentKind))
}

func ConfigFromResolved(resolved config.ResolvedModelSettings) providers.ModelConfig {
	provider := providers.ProviderID(resolved.Provider)
	protocol := providers.ProtocolID(resolved.Protocol)
	if provider == "" {
		provider = providers.ProviderOpenAICompatible
	}
	if protocol == "" {
		protocol = providers.ProtocolOpenAIChatCompletions
	}
	modelConfig := providers.ModelConfig{
		Provider:      provider,
		Protocol:      protocol,
		APIKey:        resolved.OpenAIAPIKey,
		Model:         resolved.OpenAIModel,
		BaseURL:       resolved.OpenAIBaseURL,
		ThinkingLevel: providers.ThinkingLevel(resolved.ThinkingLevel),
	}
	if resolved.Temperature != nil {
		temperature := float32(*resolved.Temperature)
		modelConfig.Temperature = &temperature
	}
	return modelConfig
}

func NewChatModel(ctx context.Context, config providers.ModelConfig) (agent.ToolCallingChatModel, error) {
	registry, err := builtin.NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("创建模型 provider registry 失败: %w", err)
	}
	return registry.NewChatModel(ctx, config)
}

func WithJSONObjectOutput(config providers.ModelConfig) providers.ModelConfig {
	config.OutputFormat = &providers.OutputFormat{Type: providers.OutputFormatJSONObject}
	return config
}
