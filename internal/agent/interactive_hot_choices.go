package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/prompts"
)

type interactiveHotChoicesPayload struct {
	Choices []string `json:"choices"`
}

func GenerateInteractiveHotChoices(ctx context.Context, cfg *config.Config, instruction string) ([]string, error) {
	if cfg == nil {
		return nil, fmt.Errorf("配置不存在")
	}
	maxTokens := 3000
	modelCfg := chatModelConfigForAgent(cfg, config.AgentKindInteractiveHotChoices)
	modelCfg.MaxTokens = &maxTokens
	modelCfg.ResponseFormat = &openai.ChatCompletionResponseFormat{
		Type: openai.ChatCompletionResponseFormatTypeJSONObject,
	}
	cm, err := openai.NewChatModel(ctx, &modelCfg)
	if err != nil {
		return nil, fmt.Errorf("创建互动快捷选择模型失败: %w", err)
	}
	log.Printf("[interactive-hot-choices-agent] generate begin instruction=%s", promptPartSummary(instruction))
	messages := []*schema.Message{
		schema.SystemMessage(protectedSystemInstruction(cfg, config.AgentKindInteractiveHotChoices, prompts.BuildInteractiveHotChoicesSystemInstruction())),
		schema.UserMessage(instruction),
	}
	logFullModelInput(modelInputLogOptions{
		AgentKind: config.AgentKindInteractiveHotChoices,
		Source:    "interactive_hot_choices",
		Mode:      "generate",
		Config:    modelCfg,
		Messages:  messages,
	})
	msg, err := cm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("生成互动快捷选择失败: %w", err)
	}
	if msg == nil {
		return nil, fmt.Errorf("互动快捷选择模型返回为空")
	}
	choices, err := parseInteractiveHotChoices(msg.Content)
	if err != nil {
		log.Printf("[interactive-hot-choices-agent] parse failed err=%v output=%q", err, msg.Content)
		return nil, err
	}
	log.Printf("[interactive-hot-choices-agent] generate done choices=%d output=%s", len(choices), promptPartSummary(msg.Content))
	return choices, nil
}

func parseInteractiveHotChoices(content string) ([]string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("互动快捷选择模型返回为空")
	}
	var payload interactiveHotChoicesPayload
	if err := json.Unmarshal([]byte(extractJSONContent(content)), &payload); err != nil {
		return nil, fmt.Errorf("解析互动快捷选择失败: %w", err)
	}
	choices := make([]string, 0, len(payload.Choices))
	seen := map[string]bool{}
	for _, choice := range payload.Choices {
		choice = strings.TrimSpace(choice)
		if choice == "" || seen[choice] {
			continue
		}
		choices = append(choices, choice)
		seen[choice] = true
		if len(choices) >= 5 {
			break
		}
	}
	if len(choices) == 0 {
		return nil, fmt.Errorf("互动快捷选择模型返回 choices 为空")
	}
	return choices, nil
}

func extractJSONContent(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSpace(content)
		content = strings.TrimSuffix(content, "```")
	}
	return strings.TrimSpace(content)
}
