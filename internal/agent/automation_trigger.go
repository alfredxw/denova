package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"

	"nova/config"
)

// GenerateAutomationTriggerEvaluation asks the model-only Automation Agent to judge one bounded trigger context.
func GenerateAutomationTriggerEvaluation(ctx context.Context, cfg *config.Config, instruction string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	modelCfg := chatModelConfigForAgent(cfg, config.AgentKindAutomation)
	modelCfg.ResponseFormat = &openai.ChatCompletionResponseFormat{
		Type: openai.ChatCompletionResponseFormatTypeJSONObject,
	}
	cm, err := openai.NewChatModel(ctx, &modelCfg)
	if err != nil {
		return "", fmt.Errorf("创建自动化触发评估模型失败: %w", err)
	}
	system := "你是 Nova 的自动化触发评估器。你的唯一任务是根据用户提供的有界创作上下文判断语义触发条件是否已经满足。不要使用工具，不要假设未给出的剧情，不要输出 JSON 以外的内容。"
	log.Printf("[automation-trigger-agent] evaluate begin instruction=%s", promptPartSummary(instruction))
	messages := []*schema.Message{
		schema.SystemMessage(protectedSystemInstruction(cfg, config.AgentKindAutomation, system)),
		schema.UserMessage(instruction),
	}
	logFullModelInput(modelInputLogOptions{
		AgentKind: config.AgentKindAutomation,
		Source:    "automation_trigger",
		Mode:      "generate",
		Config:    modelCfg,
		Messages:  messages,
	})
	msg, err := cm.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("生成自动化触发评估失败: %w", err)
	}
	if msg == nil {
		return "", fmt.Errorf("自动化触发评估模型返回为空")
	}
	log.Printf("[automation-trigger-agent] evaluate done output=%s", promptPartSummary(msg.Content))
	return msg.Content, nil
}
