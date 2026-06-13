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

func GenerateTellerEditPlan(ctx context.Context, cfg *config.Config, instruction string, tellers []interactive.Teller, targetID string, references []string, history []*schema.Message) (TellerEditPlan, error) {
	content, err := generateTellerEditPlanContent(ctx, cfg, instruction, tellers, targetID, references, history, nil)
	if err != nil {
		return TellerEditPlan{}, err
	}
	return parseTellerEditPlan(content, tellers, targetID, references, instruction)
}

func StreamTellerEditPlan(ctx context.Context, cfg *config.Config, instruction string, tellers []interactive.Teller, targetID string, references []string, history []*schema.Message, emit func(Event)) (TellerEditPlan, error) {
	content, err := generateTellerEditPlanContent(ctx, cfg, instruction, tellers, targetID, references, history, emit)
	if err != nil {
		return TellerEditPlan{}, err
	}
	return parseTellerEditPlan(content, tellers, targetID, references, instruction)
}

func generateTellerEditPlanContent(ctx context.Context, cfg *config.Config, instruction string, tellers []interactive.Teller, targetID string, references []string, history []*schema.Message, emit func(Event)) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return "", fmt.Errorf("导演编辑指令不能为空")
	}
	modelCfg := chatModelConfigForAgent(cfg, config.AgentKindTellerEditor)
	modelCfg.ResponseFormat = &openai.ChatCompletionResponseFormat{
		Type: openai.ChatCompletionResponseFormatTypeJSONObject,
	}
	cm, err := openai.NewChatModel(ctx, &modelCfg)
	if err != nil {
		return "", fmt.Errorf("创建导演编辑模型失败: %w", err)
	}
	userPrompt, err := buildTellerUserPrompt(instruction, tellers, targetID, references, history)
	if err != nil {
		return "", err
	}
	log.Printf("[teller-editor-agent] generate begin instruction=%s tellers=%d target_id=%s references=%d stream=%t", promptPartSummary(instruction), len(tellers), targetID, len(references), emit != nil)
	messages := []*schema.Message{
		schema.SystemMessage(protectedSystemInstruction(cfg, config.AgentKindTellerEditor, tellerEditorSystemInstruction())),
		schema.UserMessage(userPrompt),
	}
	if emit == nil {
		msg, err := cm.Generate(ctx, messages)
		if err != nil {
			return "", fmt.Errorf("生成导演编辑方案失败: %w", err)
		}
		if msg == nil {
			return "", fmt.Errorf("导演编辑模型返回为空")
		}
		return strings.TrimSpace(msg.Content), nil
	}

	stream, err := cm.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("生成导演编辑方案失败: %w", err)
	}
	defer stream.Close()

	var content strings.Builder
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("接收导演编辑方案失败: %w", err)
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

func buildTellerUserPrompt(instruction string, tellers []interactive.Teller, targetID string, references []string, history []*schema.Message) (string, error) {
	tellersJSON, err := json.MarshalIndent(tellers, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化导演列表失败: %w", err)
	}
	referencedTellers := collectTellerReferences(instruction, targetID, references, tellers)
	userPrompt := fmt.Sprintf("用户编辑指令：\n%s\n\n界面当前选中的导演 ID（仅作为上下文参考，不强制修改）：%s\n\n当前导演列表 JSON：\n%s", instruction, strings.TrimSpace(targetID), string(tellersJSON))
	if len(referencedTellers) > 0 {
		refsJSON, err := json.MarshalIndent(referencedTellers, "", "  ")
		if err != nil {
			return "", fmt.Errorf("序列化引用导演失败: %w", err)
		}
		userPrompt = fmt.Sprintf("用户编辑指令：\n%s\n\n用户显式 @ 引用或界面选中的导演 JSON（优先作为 update 候选，但用户明确要求新建时仍应 create）：\n%s\n\n界面当前选中的导演 ID（仅作为上下文参考，不强制修改）：%s\n\n当前导演列表 JSON：\n%s", instruction, string(refsJSON), strings.TrimSpace(targetID), string(tellersJSON))
	}
	if historyText := formatLoreHistory(history); historyText != "" {
		userPrompt = fmt.Sprintf("以下是 /clear 之后的导演 Agent 有效对话上下文，仅用于理解用户连续指令，不要把历史意图当成本轮任务：\n%s\n\n%s", historyText, userPrompt)
	}
	return userPrompt, nil
}

func parseTellerEditPlan(content string, tellers []interactive.Teller, targetID string, references []string, instruction string) (TellerEditPlan, error) {
	if strings.TrimSpace(content) == "" {
		return TellerEditPlan{}, fmt.Errorf("导演编辑模型返回为空")
	}
	var plan TellerEditPlan
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &plan); err != nil {
		return TellerEditPlan{}, fmt.Errorf("解析导演编辑方案失败: %w", err)
	}
	action := strings.ToLower(strings.TrimSpace(plan.Action))
	if action == "delete" || action == "remove" || action == "merge" || action == "batch" {
		return TellerEditPlan{}, fmt.Errorf("导演 Agent 当前只支持创建或修改单个导演，不支持删除、批量修改或合并")
	}
	if action != "create" && action != "update" {
		return TellerEditPlan{}, fmt.Errorf("导演编辑方案 action 无效: %s", plan.Action)
	}
	plan.Action = action
	referencedTellers := collectTellerReferences(instruction, targetID, references, tellers)
	if plan.Action == "update" {
		plan.Teller.ID = resolveTellerUpdateID(plan.Teller.ID, plan.Teller.Name, referencedTellers, tellers)
		if strings.TrimSpace(plan.Teller.ID) == "" {
			return TellerEditPlan{}, fmt.Errorf("导演编辑方案没有指定要修改的导演")
		}
		if !tellerIDExists(tellers, plan.Teller.ID) {
			return TellerEditPlan{}, fmt.Errorf("目标导演不存在: %s", plan.Teller.ID)
		}
	} else if plan.Action == "create" && tellerIDExists(tellers, plan.Teller.ID) {
		plan.Teller.ID = ""
	}
	plan.Teller.Path = ""
	plan.Teller.Custom = false
	plan.Teller.Invalid = false
	plan.Teller.Error = ""
	if strings.TrimSpace(plan.Message) == "" {
		if plan.Action == "create" {
			plan.Message = "导演 Agent 创建导演"
		} else {
			plan.Message = "导演 Agent 修改导演"
		}
	}
	if len(plan.Teller.Slots) == 0 {
		return TellerEditPlan{}, fmt.Errorf("导演编辑方案没有生成任何注入规则")
	}
	log.Printf("[teller-editor-agent] generate done action=%s teller_id=%s message=%q slots=%d", plan.Action, plan.Teller.ID, plan.Message, len(plan.Teller.Slots))
	return plan, nil
}

func collectTellerReferences(instruction, targetID string, references []string, tellers []interactive.Teller) []interactive.Teller {
	selected := map[string]struct{}{}
	addMatch := func(ref string) {
		ref = strings.TrimSpace(strings.TrimPrefix(ref, "@"))
		if ref == "" {
			return
		}
		for _, teller := range tellers {
			if strings.EqualFold(teller.ID, ref) || teller.Name == ref {
				selected[teller.ID] = struct{}{}
			}
		}
	}
	addMatch(targetID)
	for _, ref := range references {
		addMatch(ref)
	}
	for _, teller := range tellers {
		if teller.ID != "" && strings.Contains(instruction, "@"+teller.ID) {
			selected[teller.ID] = struct{}{}
			continue
		}
		if teller.Name != "" && strings.Contains(instruction, "@"+teller.Name) {
			selected[teller.ID] = struct{}{}
		}
	}
	if len(selected) == 0 {
		return nil
	}
	result := make([]interactive.Teller, 0, len(selected))
	for _, teller := range tellers {
		if _, ok := selected[teller.ID]; ok {
			result = append(result, teller)
		}
	}
	return result
}

func resolveTellerUpdateID(planID, planName string, referencedTellers []interactive.Teller, tellers []interactive.Teller) string {
	planID = strings.TrimSpace(planID)
	planName = strings.TrimSpace(planName)
	for _, teller := range tellers {
		if strings.EqualFold(teller.ID, planID) || teller.Name == planID {
			return teller.ID
		}
	}
	for _, teller := range tellers {
		if planName != "" && teller.Name == planName {
			return teller.ID
		}
	}
	if len(referencedTellers) == 1 {
		return referencedTellers[0].ID
	}
	return ""
}

func tellerIDExists(tellers []interactive.Teller, id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, teller := range tellers {
		if teller.ID == id {
			return true
		}
	}
	return false
}

func tellerEditorSystemInstruction() string {
	return strings.TrimSpace(`你是 Nova 的导演配置 Agent，负责按照用户指令创建或修改一个互动小说导演。

你只能输出一个 JSON object，不要输出 Markdown、解释、代码块或额外文本。
JSON 格式：
{
  "message": "一句中文变更说明",
  "action": "create | update",
  "teller": {
    "id": "create 可省略；update 必须填写要修改的已有导演 ID",
    "name": "导演名称",
    "description": "一句中文简介",
    "random_event_rate": 0.15,
    "style_rules": [
      {
        "scene": "场景描述，如：激烈打斗 / 日常对话 / 压抑悬疑",
        "styles": ["风格参考.md"]
      }
    ],
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
        "content": "导演身份、题材倾向和长期叙事原则"
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
1. 每次只创建或修改一个导演，不能删除、批量修改、合并或影响多个导演。
2. 不要被界面当前选中项强制限制；必须根据用户本轮意图决定 action。用户要求新建、另做一版、增加一个风格时用 create；用户要求修改、调整、优化某个已有导演时用 update。
3. 用户用 @ 引用导演或明确写出已有导演名称/ID 时，优先把它作为 update 对象；但用户明确要求基于它新建一版时仍用 create。
4. update 必须填写已有导演 ID，并基于该导演的完整 JSON 修改后返回完整 teller，不要只返回局部字段，避免丢失规则。
5. slots 至少包含一条启用规则，target 只能使用 system、turn_context、state_memory。
6. random_event_rate 使用 0 到 1 的数字；不知道时使用 0.15。
7. style_rules 是这个导演独立的场景风格配置；用户没要求修改时必须原样保留，缺省或不确定时可省略。
8. 每轮目标字数属于互动故事运行参数，不属于导演配置，不要输出 reply_target_chars。
9. context_policy 不确定时使用 creator=always、lore=relevant、runtime_state=always、recent_turns=8。
10. 所有面向用户的 name、description、tags 和 content 优先使用中文。
11. 用户要求删除或批量操作时，返回 action 之外的 unsupported 文本是不允许的；请改为用 message 说明当前只支持单个创建或修改，并给出一个低风险 create/update 方案。`)
}
