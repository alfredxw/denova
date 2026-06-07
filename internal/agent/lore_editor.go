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
	"nova/internal/book"
)

type LoreEditPlan struct {
	Message string               `json:"message"`
	Ops     []book.LoreOperation `json:"ops"`
}

func GenerateLoreEditPlan(ctx context.Context, cfg *config.Config, instruction string, items []book.LoreItem, references []string, history []*schema.Message) (LoreEditPlan, error) {
	content, err := generateLoreEditPlanContent(ctx, cfg, instruction, items, references, history, nil)
	if err != nil {
		return LoreEditPlan{}, err
	}
	return parseLoreEditPlan(content)
}

func StreamLoreEditPlan(ctx context.Context, cfg *config.Config, instruction string, items []book.LoreItem, references []string, history []*schema.Message, emit func(Event)) (LoreEditPlan, error) {
	content, err := generateLoreEditPlanContent(ctx, cfg, instruction, items, references, history, emit)
	if err != nil {
		return LoreEditPlan{}, err
	}
	return parseLoreEditPlan(content)
}

func generateLoreEditPlanContent(ctx context.Context, cfg *config.Config, instruction string, items []book.LoreItem, references []string, history []*schema.Message, emit func(Event)) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return "", fmt.Errorf("资料库编辑指令不能为空")
	}
	modelCfg := chatModelConfigForAgent(cfg, config.AgentKindLoreEditor)
	modelCfg.ResponseFormat = &openai.ChatCompletionResponseFormat{
		Type: openai.ChatCompletionResponseFormatTypeJSONObject,
	}
	cm, err := openai.NewChatModel(ctx, &modelCfg)
	if err != nil {
		return "", fmt.Errorf("创建资料库编辑模型失败: %w", err)
	}
	userPrompt, referencedItems, err := buildLoreUserPrompt(instruction, items, references, history)
	if err != nil {
		return "", err
	}
	log.Printf("[lore-editor-agent] generate begin instruction=%s items=%d references=%d stream=%t", promptPartSummary(instruction), len(items), len(referencedItems), emit != nil)
	messages := []*schema.Message{
		schema.SystemMessage(protectedSystemInstruction(cfg, config.AgentKindLoreEditor, loreEditorSystemInstruction())),
		schema.UserMessage(userPrompt),
	}
	if emit == nil {
		msg, err := cm.Generate(ctx, messages)
		if err != nil {
			return "", fmt.Errorf("生成资料库编辑方案失败: %w", err)
		}
		if msg == nil {
			return "", fmt.Errorf("资料库编辑模型返回为空")
		}
		return strings.TrimSpace(msg.Content), nil
	}

	stream, err := cm.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("生成资料库编辑方案失败: %w", err)
	}
	defer stream.Close()

	var content strings.Builder
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("接收资料库编辑方案失败: %w", err)
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

func buildLoreUserPrompt(instruction string, items []book.LoreItem, references []string, history []*schema.Message) (string, []book.LoreItem, error) {
	itemsJSON, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return "", nil, fmt.Errorf("序列化资料库失败: %w", err)
	}
	referencedItems := collectLoreReferencedItems(instruction, references, items)
	historyText := formatLoreHistory(history)
	userPrompt := fmt.Sprintf("用户编辑指令：\n%s\n\n当前资料库 JSON：\n%s", instruction, string(itemsJSON))
	if len(referencedItems) > 0 {
		refsJSON, err := json.MarshalIndent(referencedItems, "", "  ")
		if err != nil {
			return "", nil, fmt.Errorf("序列化引用资料失败: %w", err)
		}
		userPrompt = fmt.Sprintf("用户编辑指令：\n%s\n\n用户明确 @ 引用的资料条目 JSON：\n%s\n\n当前资料库 JSON：\n%s", instruction, string(refsJSON), string(itemsJSON))
	}
	if historyText != "" {
		userPrompt = fmt.Sprintf("以下是 /clear 之后的资料库 Agent 有效对话上下文，仅用于理解用户连续指令，不要把历史意图当成本轮任务：\n%s\n\n%s", historyText, userPrompt)
	}
	return userPrompt, referencedItems, nil
}

func parseLoreEditPlan(content string) (LoreEditPlan, error) {
	if strings.TrimSpace(content) == "" {
		return LoreEditPlan{}, fmt.Errorf("资料库编辑模型返回为空")
	}
	var plan LoreEditPlan
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &plan); err != nil {
		return LoreEditPlan{}, fmt.Errorf("解析资料库编辑方案失败: %w", err)
	}
	if strings.TrimSpace(plan.Message) == "" {
		plan.Message = "资料库 Agent 批量编辑"
	}
	if len(plan.Ops) == 0 {
		return LoreEditPlan{}, fmt.Errorf("资料库编辑方案没有产生任何操作")
	}
	log.Printf("[lore-editor-agent] generate done message=%q ops=%d", plan.Message, len(plan.Ops))
	return plan, nil
}

func formatLoreHistory(history []*schema.Message) string {
	if len(history) == 0 {
		return ""
	}
	lines := make([]string, 0, len(history))
	for _, msg := range history {
		if msg == nil || strings.TrimSpace(msg.Content) == "" {
			continue
		}
		role := "assistant"
		if msg.Role == schema.User {
			role = "user"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", role, strings.TrimSpace(msg.Content)))
	}
	return strings.Join(lines, "\n")
}

func collectLoreReferencedItems(instruction string, references []string, items []book.LoreItem) []book.LoreItem {
	selected := make(map[string]struct{})
	for _, ref := range references {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		for _, item := range items {
			if strings.EqualFold(item.ID, ref) || item.Name == ref {
				selected[item.ID] = struct{}{}
			}
		}
	}
	for _, item := range items {
		if item.ID != "" && strings.Contains(instruction, "@"+item.ID) {
			selected[item.ID] = struct{}{}
			continue
		}
		if item.Name != "" && strings.Contains(instruction, "@"+item.Name) {
			selected[item.ID] = struct{}{}
		}
	}
	if len(selected) == 0 {
		return nil
	}
	result := make([]book.LoreItem, 0, len(selected))
	for _, item := range items {
		if _, ok := selected[item.ID]; ok {
			result = append(result, item)
		}
	}
	return result
}

func loreEditorSystemInstruction() string {
	return strings.TrimSpace(`你是 Nova 的资料库编辑 Agent，负责按照用户指令维护长篇小说资料库。

你只能输出一个 JSON object，不要输出 Markdown、解释、代码块或额外文本。
JSON 格式：
{
  "message": "一句中文变更说明",
  "ops": [
    {
      "op": "create | update | delete",
      "id": "已有资料 ID，update/delete 必填",
      "item": {
        "id": "create 可省略；update 必须与 id 一致",
        "type": "character | world | location | faction | rule | item | other",
        "name": "资料名称",
        "importance": "major | important | minor",
        "load_mode": "resident | auto | manual",
        "tags": ["标签"],
        "brief_description": "类型 名称。3-5句中文简介，说明身份/别名/关键事实/适用场景/触发词。上下文出现相关内容时，一定要参考本项详情。",
        "content": "Markdown 正文"
      }
    }
  ]
}

规则：
1. 必须使用已有资料的 id 来 update/delete，不要臆造已有资料 ID。
2. update 操作的 item 要给出完整条目字段，不要只给局部字段，避免丢失正文。
3. create 操作要选择准确类型、重要度、加载策略和简介；不知道类型时用 other，不知道重要度时用 important。
4. load_mode 默认规则：核心主角、世界硬规则、最高优先级设定用 resident；普通角色、地点、势力、物品、阶段性设定用 auto；只应由作者手动点名才使用的资料用 manual。
5. brief_description 必须写成可自动触发读取的资料索引：以“类型 名称。”开头，后接 3-5 句说明身份/别名/关键事实/适用场景/触发词，最后明确“上下文出现相关内容时，一定要参考本项详情。”；用户已有简介仍有效时也要按这个结构优化。
6. content 使用中文 Markdown，保留用户已有设定中仍然有效的信息。
7. 可以一次返回多个操作，以完成用户要求的全资料库整理、合并、改名、补充、删除或一致性修正。
8. 用户没有 @ 引用具体条目时，根据指令在全资料库中自行判断需要修改哪些条目，可以一次修改多个条目。
9. 用户 @ 引用具体条目时，优先围绕引用条目执行；除非指令明确要求影响其他条目、全库整理、创建新条目或删除关联条目，不要改动未引用条目。
10. 用户要求不明确时，只做低风险整理和补充，不删除资料。`)
}
