package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/interactive"
)

type TellerEditPlan struct {
	Message string             `json:"message"`
	Action  string             `json:"action"`
	Teller  interactive.Teller `json:"teller"`
}

func GenerateTellerEditPlan(ctx context.Context, cfg *config.Config, instruction string, tellers []interactive.Teller, targetID string, history []*schema.Message) (TellerEditPlan, error) {
	content, err := generateTellerEditPlanContent(ctx, cfg, instruction, tellers, targetID, history, nil)
	if err != nil {
		return TellerEditPlan{}, err
	}
	return parseTellerEditPlan(content, targetID)
}

func StreamTellerEditPlan(ctx context.Context, cfg *config.Config, instruction string, tellers []interactive.Teller, targetID string, history []*schema.Message, emit func(Event)) (TellerEditPlan, error) {
	content, err := generateTellerEditPlanContent(ctx, cfg, instruction, tellers, targetID, history, emit)
	if err != nil {
		return TellerEditPlan{}, err
	}
	return parseTellerEditPlan(content, targetID)
}

func generateTellerEditPlanContent(ctx context.Context, cfg *config.Config, instruction string, tellers []interactive.Teller, targetID string, history []*schema.Message, emit func(Event)) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return "", fmt.Errorf("讲述者编辑指令不能为空")
	}
	temperature := float32(0.1)
	cm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:      cfg.OpenAIAPIKey,
		Model:       cfg.OpenAIModel,
		BaseURL:     cfg.OpenAIBaseURL,
		Temperature: &temperature,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return "", fmt.Errorf("创建讲述者编辑模型失败: %w", err)
	}
	userPrompt, err := buildTellerUserPrompt(instruction, tellers, targetID, history)
	if err != nil {
		return "", err
	}
	log.Printf("[teller-editor-agent] generate begin instruction=%s tellers=%d target_id=%s stream=%t", promptPartSummary(instruction), len(tellers), targetID, emit != nil)
	messages := []*schema.Message{
		schema.SystemMessage(tellerEditorSystemInstruction()),
		schema.UserMessage(userPrompt),
	}
	if emit == nil {
		msg, err := cm.Generate(ctx, messages)
		if err != nil {
			return "", fmt.Errorf("生成讲述者编辑方案失败: %w", err)
		}
		if msg == nil {
			return "", fmt.Errorf("讲述者编辑模型返回为空")
		}
		return strings.TrimSpace(msg.Content), nil
	}

	stream, err := cm.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("生成讲述者编辑方案失败: %w", err)
	}
	defer stream.Close()

	var content strings.Builder
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("接收讲述者编辑方案失败: %w", err)
		}
		if msg == nil {
			continue
		}
		if msg.ReasoningContent != "" {
			emit(Event{Type: "thinking", Data: map[string]string{"content": msg.ReasoningContent}})
		}
		if msg.Content != "" {
			content.WriteString(msg.Content)
			emit(Event{Type: "chunk", Data: map[string]string{"content": msg.Content}})
		}
	}
	return strings.TrimSpace(content.String()), nil
}

func buildTellerUserPrompt(instruction string, tellers []interactive.Teller, targetID string, history []*schema.Message) (string, error) {
	tellersJSON, err := json.MarshalIndent(tellers, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化讲述者列表失败: %w", err)
	}
	mode := "create"
	if strings.TrimSpace(targetID) != "" {
		mode = "update"
	}
	userPrompt := fmt.Sprintf("用户编辑指令：\n%s\n\n固定执行模式：%s\n目标讲述者 ID：%s\n\n当前讲述者列表 JSON：\n%s", instruction, mode, strings.TrimSpace(targetID), string(tellersJSON))
	if historyText := formatLoreHistory(history); historyText != "" {
		userPrompt = fmt.Sprintf("以下是 /clear 之后的讲述者 Agent 有效对话上下文，仅用于理解用户连续指令，不要把历史意图当成本轮任务：\n%s\n\n%s", historyText, userPrompt)
	}
	return userPrompt, nil
}

func parseTellerEditPlan(content string, targetID string) (TellerEditPlan, error) {
	if strings.TrimSpace(content) == "" {
		return TellerEditPlan{}, fmt.Errorf("讲述者编辑模型返回为空")
	}
	var plan TellerEditPlan
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &plan); err != nil {
		return TellerEditPlan{}, fmt.Errorf("解析讲述者编辑方案失败: %w", err)
	}
	action := strings.ToLower(strings.TrimSpace(plan.Action))
	if action == "delete" || action == "remove" || action == "merge" || action == "batch" {
		return TellerEditPlan{}, fmt.Errorf("讲述者 Agent 当前只支持创建或修改单个讲述者，不支持删除、批量修改或合并")
	}
	expectedAction := "create"
	if strings.TrimSpace(targetID) != "" {
		expectedAction = "update"
		plan.Teller.ID = strings.TrimSpace(targetID)
	}
	plan.Action = expectedAction
	plan.Teller.Path = ""
	plan.Teller.Custom = false
	plan.Teller.Invalid = false
	plan.Teller.Error = ""
	if strings.TrimSpace(plan.Message) == "" {
		if expectedAction == "create" {
			plan.Message = "讲述者 Agent 创建讲述者"
		} else {
			plan.Message = "讲述者 Agent 修改讲述者"
		}
	}
	if len(plan.Teller.Slots) == 0 {
		return TellerEditPlan{}, fmt.Errorf("讲述者编辑方案没有生成任何注入规则")
	}
	log.Printf("[teller-editor-agent] generate done action=%s target_id=%s message=%q slots=%d", plan.Action, targetID, plan.Message, len(plan.Teller.Slots))
	return plan, nil
}

func tellerEditorSystemInstruction() string {
	return strings.TrimSpace(`你是 Nova 的讲述者配置 Agent，负责按照用户指令创建或修改一个互动小说讲述者。

你只能输出一个 JSON object，不要输出 Markdown、解释、代码块或额外文本。
JSON 格式：
{
  "message": "一句中文变更说明",
  "action": "create | update",
  "teller": {
    "id": "create 可省略；update 会被后端强制改为目标 ID",
    "name": "讲述者名称",
    "description": "一句中文简介",
    "random_event_rate": 0.15,
    "tags": ["标签"],
    "context_policy": {
      "creator": "always",
      "lore": "relevant",
      "runtime_state": "always",
      "recent_turns": 8
    },
    "slots": [
      {
        "id": "system",
        "name": "系统提示",
        "target": "system",
        "enabled": true,
        "content": "讲述者身份、题材倾向和长期叙事原则"
      },
      {
        "id": "turn_context",
        "name": "本轮上下文",
        "target": "turn_context",
        "enabled": true,
        "content": "每轮剧情裁定、输出风格、NPC 反应和可选择规则"
      },
      {
        "id": "state_memory",
        "name": "状态记忆",
        "target": "state_memory",
        "enabled": true,
        "content": "本回合结束后状态记录应该关注的内容"
      }
    ]
  }
}

规则：
1. 每次只创建或修改一个讲述者，不能删除、批量修改、合并或影响多个讲述者。
2. 固定执行模式为 create 时，输出 action=create；固定执行模式为 update 时，输出 action=update。
3. update 必须基于目标讲述者的完整 JSON 修改后返回完整 teller，不要只返回局部字段，避免丢失规则。
4. slots 至少包含一条启用规则，target 只能使用 system、turn_context、state_memory。
5. random_event_rate 使用 0 到 1 的数字；不知道时使用 0.15。
6. context_policy 不确定时使用 creator=always、lore=relevant、runtime_state=always、recent_turns=8。
7. 所有面向用户的 name、description、tags 和 content 优先使用中文。
8. 用户要求删除或批量操作时，返回 action 之外的 unsupported 文本是不允许的；请改为用 message 说明当前只支持单个创建或修改，并给出一个低风险 create/update 方案。`)
}
