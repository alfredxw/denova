package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/prompts"
)

const interactiveStateAgentLabel = "interactive-state-agent"

func GenerateInteractiveState(ctx context.Context, cfg *config.Config, instruction string) (string, error) {
	return generateInteractiveStateContent(ctx, cfg, instruction, nil)
}

func StreamInteractiveState(ctx context.Context, cfg *config.Config, instruction string, emit func(Event)) (string, error) {
	return generateInteractiveStateContent(ctx, cfg, instruction, emit)
}

func generateInteractiveStateContent(ctx context.Context, cfg *config.Config, instruction string, emit func(Event)) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	modelCfg := chatModelConfigForAgent(cfg, config.AgentKindInteractiveState)
	log.Printf("[%s] generate begin instruction=%s stream=%t", interactiveStateAgentLabel, promptPartSummary(instruction), emit != nil)
	messages := []*schema.Message{
		schema.SystemMessage(protectedSystemInstruction(cfg, config.AgentKindInteractiveState, prompts.BuildInteractiveStateSystemInstruction())),
		schema.UserMessage(instruction),
	}
	if emit == nil {
		content, err := generateWithJSONFallback(ctx, modelCfg, messages, config.AgentKindInteractiveState, "interactive_state", interactiveStateAgentLabel)
		if err != nil {
			return "", fmt.Errorf("生成互动状态失败: %w", err)
		}
		log.Printf("[%s] generate done output=%s", interactiveStateAgentLabel, promptPartSummary(content))
		return content, nil
	}
	content, err := streamWithJSONFallback(ctx, modelCfg, messages, emit, config.AgentKindInteractiveState, "interactive_state", interactiveStateAgentLabel)
	if err != nil {
		return "", fmt.Errorf("生成互动状态失败: %w", err)
	}
	log.Printf("[%s] generate done output=%s", interactiveStateAgentLabel, promptPartSummary(content))
	return content, nil
}
