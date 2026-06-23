package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/prompts"
)

type interactiveHotChoicesPayload struct {
	Choices []string `json:"choices"`
}

const interactiveHotChoicesAgentLabel = "interactive-hot-choices-agent"

func GenerateInteractiveHotChoices(ctx context.Context, cfg *config.Config, instruction string) ([]string, error) {
	if cfg == nil {
		return nil, fmt.Errorf("配置不存在")
	}
	maxTokens := 3000
	modelCfg := chatModelConfigForAgent(cfg, config.AgentKindInteractiveHotChoices)
	modelCfg.MaxTokens = &maxTokens
	log.Printf("[%s] generate begin instruction=%s", interactiveHotChoicesAgentLabel, promptPartSummary(instruction))
	messages := []*schema.Message{
		schema.SystemMessage(protectedSystemInstruction(cfg, config.AgentKindInteractiveHotChoices, prompts.BuildInteractiveHotChoicesSystemInstruction())),
		schema.UserMessage(instruction),
	}
	content, err := generateWithJSONFallback(ctx, modelCfg, messages, interactiveHotChoicesAgentLabel)
	if err != nil {
		return nil, fmt.Errorf("生成互动快捷选择失败: %w", err)
	}
	choices, err := parseInteractiveHotChoices(content)
	if err != nil {
		log.Printf("[%s] parse failed err=%v output=%q", interactiveHotChoicesAgentLabel, err, content)
		return nil, err
	}
	log.Printf("[%s] generate done choices=%d output=%s", interactiveHotChoicesAgentLabel, len(choices), promptPartSummary(content))
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
