package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/book"
)

const loreClassificationInputMaxBytes = 64 * 1024

type loreClassificationPayload struct {
	Items []book.LoreClassificationSuggestion `json:"items"`
}

// ClassifyLoreItems runs one model-only semantic pass over an already bounded
// batch. Callers keep deterministic heuristic results if this call fails.
func ClassifyLoreItems(ctx context.Context, cfg *config.Config, inputs []book.LoreClassificationInput) ([]book.LoreClassificationSuggestion, error) {
	if cfg == nil {
		return nil, fmt.Errorf("配置不存在")
	}
	if len(inputs) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(inputs)
	if err != nil {
		return nil, err
	}
	if len(data) > loreClassificationInputMaxBytes {
		return nil, fmt.Errorf("资料分类输入超过 64 KiB 上限")
	}
	traceCtx, finishTrace := withStandaloneRunTrace(ctx, cfg, config.AgentKindToolAgent, "tool_agent_lore_classification", "generate", map[string]any{"items": len(inputs), "bytes": len(data)})
	var runErr error
	defer func() { finishTrace(runErr) }()
	instruction := "请对以下资料条目进行语义分类。名称是最重要信号；只有名称不明确时才参考标签、关键词、简介和正文片段。\n\n输入 JSON：\n" + string(data)
	jsonCfg := chatModelConfigForAgent(cfg, config.AgentKindToolAgent)
	jsonCfg.ResponseFormat = &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject}
	result, err := generateLoreClassifications(traceCtx, cfg, jsonCfg, instruction, inputs, "json_mode")
	if err == nil {
		return result, nil
	}
	if traceCtx.Err() != nil {
		runErr = err
		return nil, err
	}
	log.Printf("[tool-agent] lore classification json_mode failed, retry without response_format err=%v", err)
	plainCfg := chatModelConfigForAgent(cfg, config.AgentKindToolAgent)
	result, runErr = generateLoreClassifications(traceCtx, cfg, plainCfg, instruction, inputs, "plain_text_retry")
	return result, runErr
}

func generateLoreClassifications(ctx context.Context, cfg *config.Config, modelCfg openai.ChatModelConfig, instruction string, inputs []book.LoreClassificationInput, attempt string) ([]book.LoreClassificationSuggestion, error) {
	cm, err := openai.NewChatModel(ctx, &modelCfg)
	if err != nil {
		return nil, fmt.Errorf("创建工具 Agent 模型失败: %w", err)
	}
	messages := []*schema.Message{
		schema.SystemMessage(protectedSystemInstruction(cfg, config.AgentKindToolAgent, loreClassificationSystemInstruction())),
		schema.UserMessage(instruction),
	}
	mode := "generate_" + attempt
	span, callID, traceCtx := beginLLMCallTrace(ctx, config.AgentKindToolAgent, "tool_agent_lore_classification", mode, modelCfg, messages, nil, false)
	msg, err := cm.Generate(traceCtx, messages)
	if err != nil {
		finishLLMCallTrace(span, callID, config.AgentKindToolAgent, "tool_agent_lore_classification", mode, modelCfg.Model, 0, nil, err, nil)
		return nil, fmt.Errorf("工具 Agent 资料分类失败: %w", err)
	}
	if msg == nil {
		err = fmt.Errorf("工具 Agent 返回为空")
		finishLLMCallTrace(span, callID, config.AgentKindToolAgent, "tool_agent_lore_classification", mode, modelCfg.Model, 0, nil, err, nil)
		return nil, err
	}
	finishLLMCallTrace(span, callID, config.AgentKindToolAgent, "tool_agent_lore_classification", mode, modelCfg.Model, 0, msg, nil, nil)
	result, err := parseLoreClassificationContent(msg.Content, inputs)
	if err != nil && strings.TrimSpace(msg.Content) == "" && strings.TrimSpace(msg.ReasoningContent) != "" {
		result, err = parseLoreClassificationContent(msg.ReasoningContent, inputs)
	}
	if err != nil {
		return nil, fmt.Errorf("解析工具 Agent 资料分类输出失败: %w", err)
	}
	log.Printf("[tool-agent] lore classification done attempt=%s requested=%d returned=%d", attempt, len(inputs), len(result))
	return result, nil
}

func parseLoreClassificationContent(content string, inputs []book.LoreClassificationInput) ([]book.LoreClassificationSuggestion, error) {
	var payload loreClassificationPayload
	if err := json.Unmarshal([]byte(extractJSONContent(content)), &payload); err != nil {
		return nil, err
	}
	allowedIDs := map[string]bool{}
	for _, input := range inputs {
		allowedIDs[strings.TrimSpace(input.ID)] = true
	}
	seen := map[string]bool{}
	result := make([]book.LoreClassificationSuggestion, 0, len(payload.Items))
	for _, item := range payload.Items {
		item.ID = strings.TrimSpace(item.ID)
		item.Type = strings.TrimSpace(item.Type)
		item.Confidence = strings.ToLower(strings.TrimSpace(item.Confidence))
		item.Reason = strings.TrimSpace(item.Reason)
		if !allowedIDs[item.ID] || seen[item.ID] {
			return nil, fmt.Errorf("返回了未知或重复的资料 ID: %s", item.ID)
		}
		if !isLoreClassificationType(item.Type) {
			return nil, fmt.Errorf("资料 %s 返回了无效类型: %s", item.ID, item.Type)
		}
		switch item.Confidence {
		case book.LoreClassificationConfidenceHigh, book.LoreClassificationConfidenceMedium, book.LoreClassificationConfidenceLow:
		default:
			item.Confidence = book.LoreClassificationConfidenceLow
		}
		seen[item.ID] = true
		result = append(result, item)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("未返回分类结果")
	}
	return result, nil
}

func isLoreClassificationType(value string) bool {
	switch value {
	case "character", "world", "location", "faction", "rule", "item", "other":
		return true
	default:
		return false
	}
}

func loreClassificationSystemInstruction() string {
	return strings.Join([]string{
		"你负责给 Denova 资料库条目分类。",
		"只输出 JSON object：{\"items\":[{\"id\":\"输入 id\",\"type\":\"character|world|location|faction|rule|item|other\",\"confidence\":\"high|medium|low\",\"reason\":\"简短依据\"}]}。",
		"名称优先于正文：人物详情、角色档案等名称应归为 character；地点、势力、规则、物品等明确名称同理。",
		"world 只用于跨地点的世界观、历史、文化或时代背景；无法稳定判断时使用 other 和 low，不要猜测。",
		"每个输入 id 最多返回一次，不得创造输入中不存在的 id，不要输出 Markdown。",
	}, "\n")
}
