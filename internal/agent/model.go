package agent

import (
	"github.com/cloudwego/eino-ext/components/model/openai"

	"nova/config"
	"nova/internal/providercompat"
)

func chatModelConfigForAgent(cfg *config.Config, agentKind string) openai.ChatModelConfig {
	resolved := config.ResolveAgentModel(cfg, agentKind)
	return chatModelConfigFromResolved(resolved)
}

func chatModelConfigFromResolved(resolved config.ResolvedModelSettings) openai.ChatModelConfig {
	modelCfg := openai.ChatModelConfig{
		APIKey:  resolved.OpenAIAPIKey,
		Model:   resolved.OpenAIModel,
		BaseURL: resolved.OpenAIBaseURL,
	}
	if resolved.Temperature != nil {
		temperature := float32(*resolved.Temperature)
		modelCfg.Temperature = &temperature
	}
	extraFields := map[string]any{}
	if resolved.EnableThinking != nil {
		extraFields["enable_thinking"] = *resolved.EnableThinking
	}
	// 让 providercompat 决定是否要注入 provider 特有的请求字段。
	// agent 包不感知任何具体 provider。
	for k, v := range providercompat.ExtraRequestFields(modelCfg) {
		extraFields[k] = v
	}
	if len(extraFields) > 0 {
		modelCfg.ExtraFields = extraFields
	}
	if resolved.ReasoningEffort != "" {
		modelCfg.ReasoningEffort = openai.ReasoningEffortLevel(resolved.ReasoningEffort)
	}
	return modelCfg
}
