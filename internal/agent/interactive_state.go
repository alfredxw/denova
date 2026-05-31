package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/prompts"
)

func GenerateInteractiveState(ctx context.Context, cfg *config.Config, instruction string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	modelCfg := chatModelConfigForAgent(cfg, config.AgentKindInteractiveState)
	modelCfg.ResponseFormat = &openai.ChatCompletionResponseFormat{
		Type: openai.ChatCompletionResponseFormatTypeJSONObject,
	}
	cm, err := openai.NewChatModel(ctx, &modelCfg)
	if err != nil {
		return "", fmt.Errorf("创建互动状态模型失败: %w", err)
	}
	log.Printf("[interactive-state-agent] generate begin instruction=%s", promptPartSummary(instruction))
	msg, err := cm.Generate(ctx, []*schema.Message{
		schema.SystemMessage(prompts.BuildInteractiveStateSystemInstruction()),
		schema.UserMessage(instruction),
	})
	if err != nil {
		return "", fmt.Errorf("生成互动状态失败: %w", err)
	}
	if msg == nil {
		return "", fmt.Errorf("互动状态模型返回为空")
	}
	log.Printf("[interactive-state-agent] generate done output=%s", promptPartSummary(msg.Content))
	return msg.Content, nil
}
