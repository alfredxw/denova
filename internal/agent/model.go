package agent

import (
	"github.com/cloudwego/eino-ext/components/model/openai"

	"nova/config"
)

func chatModelConfigForAgent(cfg *config.Config, agentKind string) openai.ChatModelConfig {
	resolved := config.ResolveAgentModel(cfg, agentKind)
	modelCfg := openai.ChatModelConfig{
		APIKey:  resolved.OpenAIAPIKey,
		Model:   resolved.OpenAIModel,
		BaseURL: resolved.OpenAIBaseURL,
	}
	if resolved.Temperature != nil {
		temperature := float32(*resolved.Temperature)
		modelCfg.Temperature = &temperature
	}
	return modelCfg
}
