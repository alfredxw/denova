package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/prompts"
)

const interactiveStateSchemaAgentLabel = "interactive-state-schema-agent"

// GenerateInteractiveStateSchemaAdaptation runs the Director model without
// story mutation tools. The caller validates and applies the returned diff
// before the story schema is frozen.
func GenerateInteractiveStateSchemaAdaptation(ctx context.Context, cfg *config.Config, instruction string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	modelCfg := chatModelConfigForAgent(cfg, config.AgentKindInteractiveDirector)
	messages := []*schema.Message{
		schema.SystemMessage(protectedSystemInstruction(cfg, config.AgentKindInteractiveDirector, prompts.BuildInteractiveStateSchemaAdapterSystemInstruction())),
		schema.UserMessage(instruction),
	}
	log.Printf("[%s] generate begin instruction=%s", interactiveStateSchemaAgentLabel, promptPartSummary(instruction))
	content, err := generateWithJSONFallback(ctx, modelCfg, messages, config.AgentKindInteractiveDirector, "interactive_state_schema", interactiveStateSchemaAgentLabel)
	if err != nil {
		return "", fmt.Errorf("生成故事状态结构适配失败: %w", err)
	}
	log.Printf("[%s] generate done output=%s", interactiveStateSchemaAgentLabel, promptPartSummary(content))
	return content, nil
}
