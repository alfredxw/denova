package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"

	"denova/config"
)

// GenerateAutomationTriggerEvaluation asks the model-only Automation Agent to judge one bounded trigger context.
func GenerateAutomationTriggerEvaluation(ctx context.Context, cfg *config.Config, instruction string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	var runErr error
	traceCtx, finishTrace := withStandaloneRunTrace(ctx, cfg, config.AgentKindAutomation, "automation_trigger", "generate", map[string]any{
		"instruction_chars": len([]rune(instruction)),
	})
	defer func() { finishTrace(runErr) }()
	modelCfg := chatModelConfigForAgent(cfg, config.AgentKindAutomation)
	modelCfg.ResponseFormat = &openai.ChatCompletionResponseFormat{
		Type: openai.ChatCompletionResponseFormatTypeJSONObject,
	}
	cm, err := openai.NewChatModel(traceCtx, &modelCfg)
	if err != nil {
		runErr = err
		return "", fmt.Errorf("创建自动化触发评估模型失败: %w", err)
	}
	system := "你是 Denova 的自动化触发评估器。你的唯一任务是根据用户提供的有界创作上下文判断语义触发条件是否已经满足。不要使用工具，不要假设未给出的剧情，不要输出 JSON 以外的内容。"
	log.Printf("[automation-trigger-agent] evaluate begin instruction=%s", promptPartSummary(instruction))
	messages := []*schema.Message{
		schema.SystemMessage(protectedSystemInstruction(cfg, config.AgentKindAutomation, system)),
		schema.UserMessage(instruction),
	}
	span, callID, llmTraceCtx := beginLLMCallTrace(traceCtx, config.AgentKindAutomation, "automation_trigger", "generate", modelCfg, messages, nil, false)
	msg, err := cm.Generate(llmTraceCtx, messages)
	if err != nil {
		finishLLMCallTrace(span, callID, config.AgentKindAutomation, "automation_trigger", "generate", modelCfg.Model, 0, nil, err, nil)
		runErr = err
		return "", fmt.Errorf("生成自动化触发评估失败: %w", err)
	}
	if msg == nil {
		runErr = fmt.Errorf("自动化触发评估模型返回为空")
		finishLLMCallTrace(span, callID, config.AgentKindAutomation, "automation_trigger", "generate", modelCfg.Model, 0, nil, runErr, nil)
		return "", runErr
	}
	finishLLMCallTrace(span, callID, config.AgentKindAutomation, "automation_trigger", "generate", modelCfg.Model, 0, msg, nil, nil)
	log.Printf("[automation-trigger-agent] evaluate done output=%s", promptPartSummary(msg.Content))
	return msg.Content, nil
}
