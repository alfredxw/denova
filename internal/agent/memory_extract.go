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
	"denova/internal/interactive"
)

const (
	memoryExtractionInputMaxBytes = 24 * 1024
	memoryExtractionMaxRecords    = 12
	memoryExtractionEvidenceRunes = 160
)

// MemoryExtractionInput 是一次叙事记忆抽取的有界输入。
type MemoryExtractionInput struct {
	StoryID string
	BranchID string
	// Turn 是刚定稿的回合(唯一抽取对象)。
	Turn interactive.TurnEvent
	// OpenPromises 是当前仍悬置的伏笔目录(每条一行),供抽取器
	// 判断本回合是否兑现了某个已有伏笔。
	OpenPromises []string
	// Roster 是本分支已出现过的实体名册。抽取器被要求复用其中的写法,
	// 让同一实体在跨回合的记录里保持同一个字符串 —— 检索侧的实体匹配
	// 是纯字面的,写法漂移会直接把关系图切断。
	Roster []interactive.MemoryEntity
}

// MemoryExtractionResult 是抽取产出:合规记录 + 被丢弃记录的原因。
// 实体对齐不在这里做 —— 它是 Store 写入路径的不变量,覆盖所有注入途径。
type MemoryExtractionResult struct {
	Records []interactive.NarrativeMemoryRecord
	Dropped []interactive.NarrativeMemoryDropRecord
}

type memoryExtractionPayload struct {
	Records []interactive.NarrativeMemoryRecord `json:"records"`
}

// GenerateNarrativeMemory 对一个定稿回合执行单次模型抽取,产出类型化
// 叙事记忆记录。逐条校验,不合规记录丢弃并记录原因,不整体失败;
// 调用方(后台任务)对返回的 error 只做日志,不影响主流程。
func GenerateNarrativeMemory(ctx context.Context, cfg *config.Config, input MemoryExtractionInput) (MemoryExtractionResult, error) {
	if cfg == nil {
		return MemoryExtractionResult{}, fmt.Errorf("配置不存在")
	}
	instruction, err := buildMemoryExtractionInstruction(input)
	if err != nil {
		return MemoryExtractionResult{}, err
	}
	traceCtx, finishTrace := withStandaloneRunTrace(ctx, cfg, config.AgentKindNarrativeMemory, "narrative_memory_extract", "generate", map[string]any{
		"story_id": input.StoryID, "branch_id": input.BranchID, "turn_id": input.Turn.ID,
	})
	var runErr error
	defer func() { finishTrace(runErr) }()

	jsonCfg := chatModelConfigForAgent(cfg, config.AgentKindNarrativeMemory)
	jsonCfg.ResponseFormat = &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject}
	result, err := generateMemoryRecords(traceCtx, cfg, jsonCfg, instruction, input.Turn.ID, "json_mode")
	if err == nil {
		return result, nil
	}
	if traceCtx.Err() != nil {
		runErr = err
		return MemoryExtractionResult{}, err
	}
	log.Printf("[narrative-memory] extraction json_mode failed, retry without response_format err=%v", err)
	plainCfg := chatModelConfigForAgent(cfg, config.AgentKindNarrativeMemory)
	result, runErr = generateMemoryRecords(traceCtx, cfg, plainCfg, instruction, input.Turn.ID, "plain_text_retry")
	return result, runErr
}

func generateMemoryRecords(ctx context.Context, cfg *config.Config, modelCfg openai.ChatModelConfig, instruction string, sourceTurnID string, attempt string) (MemoryExtractionResult, error) {
	cm, err := openai.NewChatModel(ctx, &modelCfg)
	if err != nil {
		return MemoryExtractionResult{}, fmt.Errorf("创建叙事记忆抽取模型失败: %w", err)
	}
	messages := []*schema.Message{
		schema.SystemMessage(protectedSystemInstruction(cfg, config.AgentKindNarrativeMemory, memoryExtractionSystemInstruction())),
		schema.UserMessage(instruction),
	}
	mode := "generate_" + attempt
	span, callID, traceCtx := beginLLMCallTrace(ctx, config.AgentKindNarrativeMemory, "narrative_memory_extract", mode, modelCfg, messages, nil, false)
	msg, err := cm.Generate(traceCtx, messages)
	if err != nil {
		finishLLMCallTrace(span, callID, config.AgentKindNarrativeMemory, "narrative_memory_extract", mode, modelCfg.Model, 0, nil, err, nil)
		return MemoryExtractionResult{}, fmt.Errorf("叙事记忆抽取调用失败: %w", err)
	}
	if msg == nil {
		err = fmt.Errorf("叙事记忆抽取返回为空")
		finishLLMCallTrace(span, callID, config.AgentKindNarrativeMemory, "narrative_memory_extract", mode, modelCfg.Model, 0, nil, err, nil)
		return MemoryExtractionResult{}, err
	}
	finishLLMCallTrace(span, callID, config.AgentKindNarrativeMemory, "narrative_memory_extract", mode, modelCfg.Model, 0, msg, nil, nil)
	content := msg.Content
	if strings.TrimSpace(content) == "" && strings.TrimSpace(msg.ReasoningContent) != "" {
		content = msg.ReasoningContent
	}
	result, err := parseMemoryExtractionContent(content, sourceTurnID)
	if err != nil {
		return MemoryExtractionResult{}, fmt.Errorf("解析叙事记忆抽取输出失败: %w", err)
	}
	log.Printf("[narrative-memory] extraction done attempt=%s records=%d dropped=%d", attempt, len(result.Records), len(result.Dropped))
	return result, nil
}

func parseMemoryExtractionContent(content string, sourceTurnID string) (MemoryExtractionResult, error) {
	var payload memoryExtractionPayload
	if err := json.Unmarshal([]byte(extractJSONContent(content)), &payload); err != nil {
		return MemoryExtractionResult{}, err
	}
	result := MemoryExtractionResult{Records: []interactive.NarrativeMemoryRecord{}, Dropped: []interactive.NarrativeMemoryDropRecord{}}
	seen := map[string]bool{}
	for i, record := range payload.Records {
		if len(result.Records) >= memoryExtractionMaxRecords {
			result.Dropped = append(result.Dropped, interactive.NarrativeMemoryDropRecord{
				Raw:    boundedMemoryExtractSummary(record),
				Reason: "max_records",
			})
			continue
		}
		record.ID = strings.TrimSpace(record.ID)
		record.Kind = strings.TrimSpace(record.Kind)
		record.Subject = strings.TrimSpace(record.Subject)
		record.Object = strings.TrimSpace(record.Object)
		record.Text = strings.TrimSpace(record.Text)
		record.Evidence = boundedTextRunes(strings.TrimSpace(record.Evidence), memoryExtractionEvidenceRunes)
		record.ValidFrom = strings.TrimSpace(record.ValidFrom)
		record.ValidTo = strings.TrimSpace(record.ValidTo)
		record.Status = strings.TrimSpace(record.Status)
		if reason := memoryRecordDropReason(record, sourceTurnID); reason != "" {
			result.Dropped = append(result.Dropped, interactive.NarrativeMemoryDropRecord{
				Raw:    boundedMemoryExtractSummary(record),
				Reason: reason,
			})
			continue
		}
		if record.ID == "" {
			record.ID = fmt.Sprintf("mem_%d", i+1)
		}
		if seen[record.ID] {
			result.Dropped = append(result.Dropped, interactive.NarrativeMemoryDropRecord{Raw: boundedMemoryExtractSummary(record), Reason: "duplicate_id"})
			continue
		}
		seen[record.ID] = true
		result.Records = append(result.Records, record)
	}
	return result, nil
}

// memoryRecordDropReason 返回单条记录的不合规原因;空 = 通过。
// 与 AppendNarrativeMemory 的服务端校验同构,丢弃在前端到落库之前完成。
func memoryRecordDropReason(record interactive.NarrativeMemoryRecord, sourceTurnID string) string {
	switch record.Kind {
	case interactive.MemoryKindKnowledge, interactive.MemoryKindReveal, interactive.MemoryKindPromise,
		interactive.MemoryKindObjectState, interactive.MemoryKindRelationship, interactive.MemoryKindBeat:
	default:
		return "kind_invalid"
	}
	if record.Subject == "" {
		return "subject_empty"
	}
	if record.Text == "" {
		return "text_empty"
	}
	if record.Evidence == "" {
		return "evidence_missing"
	}
	if record.Status != "" && record.Status != interactive.MemoryStatusOpen && record.Status != interactive.MemoryStatusPaid {
		return "status_invalid"
	}
	// valid_to 只能指向本回合(当场兑现/推翻)或留空;模型不应指定其它回合。
	if record.ValidTo != "" && record.ValidTo != sourceTurnID {
		return "valid_to_not_source_turn"
	}
	return ""
}

func boundedMemoryExtractSummary(record interactive.NarrativeMemoryRecord) string {
	summary := record.Kind + " " + record.Subject + " " + record.Text
	return boundedTextRunes(summary, 80)
}

func boundedTextRunes(value string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(value))
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return string(runes)
	}
	return strings.TrimSpace(string(runes[:maxRunes-1])) + "…"
}

// buildMemoryExtractionInstruction 组装有界抽取输入:本回合行动与叙事
// (截断)、悬置伏笔目录、六类记录定义与输出协议。超限直接报错,
// 不静默截断整体输入。
func buildMemoryExtractionInstruction(input MemoryExtractionInput) (string, error) {
	var sb strings.Builder
	sb.WriteString("请从下面这个刚定稿的互动故事回合中抽取叙事记忆记录,严格输出 JSON。\n\n")
	sb.WriteString("## 本回合\n")
	sb.WriteString("玩家行动:" + boundedTextRunes(input.Turn.User, 1000) + "\n")
	sb.WriteString("叙事正文:" + boundedTextRunes(input.Turn.Narrative, 4000) + "\n")
	if len(input.OpenPromises) > 0 {
		sb.WriteString("\n## 当前悬置的伏笔\n")
		for _, promise := range input.OpenPromises {
			sb.WriteString("- " + boundedTextRunes(promise, 120) + "\n")
		}
		sb.WriteString("若本回合兑现了其中某条,输出该伏笔的新记录并把 status 设为 paid、valid_to 设为本回合 turn_id。\n")
	}
	if len(input.Roster) > 0 {
		sb.WriteString("\n## 已知实体名册\n")
		for _, entity := range input.Roster {
			line := "- " + boundedTextRunes(entity.Name, 40)
			if len(entity.Kinds) > 0 {
				line += "(" + strings.Join(entity.Kinds, "/") + ")"
			}
			sb.WriteString(line + "\n")
		}
		sb.WriteString("本回合若提到名册中已有的人物、物品或地点,subject/object 必须原样照抄名册里的写法,即使正文用的是代称(如\"那把剑\"\"他\")。只有名册里没有的全新实体才使用新名称。\n")
	}
	sb.WriteString("\n## 记录类型(kind 六选一)\n")
	sb.WriteString("- knowledge: 谁知道什么、何时知道(Subject=知情者, Object=事实)\n")
	sb.WriteString("- reveal: 读者何时得知 vs 事件何时发生(揭示顺序)\n")
	sb.WriteString("- promise: 伏笔(status=open 悬置 / paid 已兑现)\n")
	sb.WriteString("- object_state: 物品/资源的归属或位置(Subject=物, Object=持有者/地点)\n")
	sb.WriteString("- relationship: 人物关系的确立或变化(Subject/Object=双方)\n")
	sb.WriteString("- beat: 本回合的戏剧功能(Subject=角色或线索)\n\n")
	sb.WriteString("## 输出协议\n")
	sb.WriteString(`输出 {"records":[...]}。每条字段:id(可留空)、kind、subject、object(可选)、text(一句话自包含事实)、evidence(正文原文摘录,必填,禁止改写)、status(仅 promise)、valid_to(仅当场兑现/推翻时填本回合 turn_id=` + input.Turn.ID + `)。` + "\n")
	sb.WriteString("只记录本回合新确立或发生变化的事实;宁缺毋滥,常规回合 0-8 条;禁止发明正文没有的内容;Subject/Object 优先使用正文中的人物名称。\n")
	instruction := sb.String()
	if len(instruction) > memoryExtractionInputMaxBytes {
		return "", fmt.Errorf("叙事记忆抽取输入超过 %d 字节上限", memoryExtractionInputMaxBytes)
	}
	return instruction, nil
}

func memoryExtractionSystemInstruction() string {
	return strings.Join([]string{
		"你是长篇互动小说的叙事记忆管理员。你的唯一任务:从已定稿回合中抽取类型化记忆记录,供后续回合作一致性检索。",
		"铁律:每条记录必须有正文原文摘录作为 evidence;不确定的不写;只记录本回合确立的事实;不评价文风;不复述剧情。",
		"输出必须是单个 JSON 对象,不含任何其它文本。",
	}, "\n")
}
