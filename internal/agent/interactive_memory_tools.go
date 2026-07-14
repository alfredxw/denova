package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"

	"denova/internal/interactive"
)

const (
	interactiveMemoryToolListLimit    = 24
	interactiveMemoryToolSummaryLimit = 800
)

// InteractiveStoryToolContext provides story-scoped read tools for one
// interactive story run. The story and branch are fixed by the backend; the
// model never supplies them.
type InteractiveStoryToolContext struct {
	Store           *interactive.Store
	StoryID         string
	BranchID        string
	TurnID          string
	MaintenanceTask string
	// StableContext is a bounded, source-labelled model prefix kept separate
	// from the changing task instruction so providers can reuse prompt caches.
	StableContextTitle       string
	StableContext            string
	StableContextMaxBytes    int
	OnStoryMemoryApplied     func(applied int)
	OnStateMaintenanceFailed func(error)
	OnLoreItemsRead          func([]string)
	SubmitStateSchemaBatch   func(context.Context, interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error)
	SubmitDirectorPlanUpdate func(context.Context, interactive.DirectorPlanUpdateSubmission) (interactive.DirectorPlanUpdateReceipt, error)
	// SubmitStateSchemaProposal remains available to in-process integrations
	// during the Batch transition. The model-facing tool uses Batch only.
	SubmitStateSchemaProposal func(context.Context, interactive.ActorStateSchemaProposal) (interactive.ActorStateSchemaProposalPreview, error)
	// DisplayConversation receives display-only progress for background helper
	// agents. It must not receive final assistant text as model-visible context.
	DisplayConversation Conversation
	PrepareTurn         func(context.Context, interactive.TurnCheckRequest) (interactive.RuleResolution, error)
	SubmitTurnResult    func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error)
	TurnResultReady     func() bool
}

type listInteractiveMemoriesInput struct {
	Query  string   `json:"query,omitempty" jsonschema:"description=可选检索词，用当前行动、人物、地点、线索或目标描述相关记忆"`
	People []string `json:"people,omitempty" jsonschema:"description=可选人物筛选，匹配记忆 people 字段"`
	Places []string `json:"places,omitempty" jsonschema:"description=可选地点筛选，匹配记忆 places 字段"`
	Tags   []string `json:"tags,omitempty" jsonschema:"description=可选标签筛选，匹配记忆 tags 字段"`
	Limit  int      `json:"limit,omitempty" jsonschema:"description=最多返回多少条索引，默认 12，最大 24"`
}

type readInteractiveMemoriesInput struct {
	IDs   []string `json:"ids" jsonschema:"description=要读取正文的互动长期记忆 ID 列表；可按需一次读取多个相关记忆"`
	Query string   `json:"query,omitempty" jsonschema:"description=可选，说明本次读取记忆是为了回答哪类当前行动或线索；用于记录最近召回"`
}

type applyStoryMemoryPatchesInput struct {
	Patches []interactive.StoryMemoryPatch `json:"patches" jsonschema:"description=要写入的故事记忆 patch。每条 patch 必须遵守当前注入的 Story Memory schema；op 仅使用 upsert、append、archive、restore。"`
}

type interactiveMemoryToolOutput struct {
	Source    interactiveMemoryToolSource `json:"source"`
	Limits    map[string]int              `json:"limits"`
	Truncated bool                        `json:"truncated"`
	Memories  any                         `json:"memories"`
}

type interactiveMemoryToolSource struct {
	Kind     string `json:"kind"`
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	Path     string `json:"path"`
}

type interactiveMemoryIndexItem struct {
	ID         string   `json:"id"`
	BranchID   string   `json:"branch_id"`
	TurnID     string   `json:"turn_id,omitempty"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	People     []string `json:"people,omitempty"`
	Places     []string `json:"places,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Importance int      `json:"importance"`
	Manual     bool     `json:"manual,omitempty"`
	UpdatedAt  string   `json:"updated_at,omitempty"`
}

type storyMemoryPatchToolOutput struct {
	AppliedRecords int    `json:"applied_records"`
	BranchID       string `json:"branch_id"`
	TurnID         string `json:"turn_id"`
}

// interactiveTurnCheckToolInput deliberately omits model-authored
// outcomes.state_changes. Deterministic State Bindings produce rule state
// changes; all remaining state mutations are submitted after the narrative.
type interactiveTurnCheckToolInput struct {
	Action       string                            `json:"action" jsonschema_description:"用户行为：本回合玩家实际尝试做什么。"`
	Intent       string                            `json:"intent" jsonschema_description:"行动意图：玩家希望通过本行动达成的目标。"`
	Challenge    string                            `json:"challenge" jsonschema_description:"检定挑战：需要 d20 固定裁定的风险、阻碍或冲突。"`
	Cost         string                            `json:"cost" jsonschema_description:"潜在代价：失败、暴露、资源消耗或关系损失等后果。"`
	State        string                            `json:"state" jsonschema_description:"只写与本次检定直接相关的可见状态、资源、位置、关系或限制。"`
	Adjudication interactive.TurnCheckAdjudication `json:"adjudication,omitempty"`
	Rule         interactive.TurnCheckRule         `json:"rule,omitempty"`
	Bonuses      []interactive.TurnCheckBonus      `json:"bonuses,omitempty"`
	Difficulty   string                            `json:"difficulty" jsonschema:"enum=very_easy,enum=easy,enum=normal,enum=hard,enum=very_hard"`
	Outcomes     interactiveTurnCheckToolOutcomes  `json:"outcomes"`
}

type interactiveTurnCheckToolOutcomes struct {
	CriticalSuccess interactiveTurnCheckToolOutcome `json:"critical_success"`
	Success         interactiveTurnCheckToolOutcome `json:"success"`
	Failure         interactiveTurnCheckToolOutcome `json:"failure"`
	CriticalFailure interactiveTurnCheckToolOutcome `json:"critical_failure"`
}

type interactiveTurnCheckToolOutcome struct {
	Result string `json:"result" jsonschema_description:"命中该档位时必须遵守的最终后果，用于指导正文。"`
}

func (input interactiveTurnCheckToolInput) request() interactive.TurnCheckRequest {
	return interactive.TurnCheckRequest{
		Action: input.Action, Intent: input.Intent, Challenge: input.Challenge, Cost: input.Cost, State: input.State,
		Adjudication: input.Adjudication, Rule: input.Rule, Bonuses: input.Bonuses, Difficulty: input.Difficulty,
		Outcomes: interactive.TurnCheckOutcomes{
			CriticalSuccess: interactive.TurnCheckOutcome{Result: input.Outcomes.CriticalSuccess.Result},
			Success:         interactive.TurnCheckOutcome{Result: input.Outcomes.Success.Result},
			Failure:         interactive.TurnCheckOutcome{Result: input.Outcomes.Failure.Result},
			CriticalFailure: interactive.TurnCheckOutcome{Result: input.Outcomes.CriticalFailure.Result},
		},
	}
}

func newInteractiveMemoryTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	ctx.StoryID = strings.TrimSpace(ctx.StoryID)
	ctx.BranchID = strings.TrimSpace(ctx.BranchID)
	if ctx.Store == nil || ctx.StoryID == "" {
		return nil, nil
	}
	listTool, err := utils.InferTool("list_interactive_memories", "列出当前互动故事分支的长期记忆轻量索引。用于根据当前行动、人物、地点、线索或标签判断本轮需要读取哪些历史事实；默认排除归档记忆和其他分支记忆。", func(callCtx context.Context, input listInteractiveMemoriesInput) (string, error) {
		_ = callCtx
		limit := normalizeInteractiveMemoryToolLimit(input.Limit, 12, interactiveMemoryToolListLimit)
		entries, err := ctx.Store.VisibleInteractiveMemories(ctx.StoryID, ctx.BranchID, interactiveMemoryToolListLimit)
		if err != nil {
			return "", err
		}
		filtered := filterInteractiveMemoryToolEntries(entries, input)
		truncated := len(filtered) > limit
		if truncated {
			filtered = filtered[:limit]
		}
		items := make([]interactiveMemoryIndexItem, 0, len(filtered))
		for _, entry := range filtered {
			items = append(items, interactiveMemoryIndexItem{
				ID:         entry.ID,
				BranchID:   entry.BranchID,
				TurnID:     entry.TurnID,
				Title:      entry.Title,
				Summary:    trimInteractiveMemoryToolText(firstNonEmpty(entry.Summary, entry.Content), interactiveMemoryToolSummaryLimit),
				People:     entry.People,
				Places:     entry.Places,
				Tags:       entry.Tags,
				Importance: entry.Importance,
				Manual:     entry.Manual,
				UpdatedAt:  entry.UpdatedAt,
			})
		}
		return marshalInteractiveMemoryToolOutput(interactiveMemoryToolOutput{
			Source:    interactiveMemoryToolSource{Kind: "interactive_memory_index", StoryID: ctx.StoryID, BranchID: ctx.BranchID, Path: fmt.Sprintf("interactive/memory/story-%s.json", ctx.StoryID)},
			Limits:    map[string]int{"max_items": interactiveMemoryToolListLimit, "returned_items": len(items), "summary_bytes_per_item": interactiveMemoryToolSummaryLimit},
			Truncated: truncated,
			Memories:  items,
		})
	})
	if err != nil {
		return nil, err
	}
	readTool, err := utils.InferTool("read_interactive_memories", "按 ID 读取当前互动故事分支的长期记忆完整正文。用于在 list_interactive_memories 判断相关后读取关键记忆；归档记忆和其他分支记忆不可读取。", func(callCtx context.Context, input readInteractiveMemoriesInput) (string, error) {
		_ = callCtx
		entries, err := ctx.Store.ReadVisibleInteractiveMemories(ctx.StoryID, ctx.BranchID, input.IDs, 0)
		if err != nil {
			return "", err
		}
		ids := make([]string, 0, len(entries))
		for _, entry := range entries {
			ids = append(ids, entry.ID)
		}
		if len(ids) > 0 {
			if err := ctx.Store.RecordInteractiveMemoryRecall(ctx.StoryID, ctx.BranchID, "", input.Query, ids); err != nil {
				return "", err
			}
		}
		return marshalInteractiveMemoryToolOutput(interactiveMemoryToolOutput{
			Source:    interactiveMemoryToolSource{Kind: "interactive_memory_entries", StoryID: ctx.StoryID, BranchID: ctx.BranchID, Path: fmt.Sprintf("interactive/memory/story-%s.json", ctx.StoryID)},
			Limits:    map[string]int{"requested_items": len(input.IDs), "returned_items": len(entries)},
			Truncated: false,
			Memories:  entries,
		})
	})
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{listTool, readTool}, nil
}

func newInteractiveStoryMemoryPatchTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	ctx.StoryID = strings.TrimSpace(ctx.StoryID)
	ctx.BranchID = strings.TrimSpace(ctx.BranchID)
	ctx.TurnID = strings.TrimSpace(ctx.TurnID)
	if ctx.Store == nil || ctx.StoryID == "" || ctx.TurnID == "" {
		return nil, nil
	}
	applyTool, err := utils.InferTool("apply_story_memory_patches", "写入当前互动故事分支的故事记忆 patch。只用于已提交 TurnResult 和正文中成立、后续需要承接的叙事事实；字段、结构、key 和 values 必须来自注入的 Story Memory schema。可计算状态、关系数值、持续状态和规则标记以已提交 StateDelta 为准，不要复制成故事记忆，也不得尝试修改 Actor State。后端会按分支和结构校验，并写入可重建的 story memory 记录。", func(callCtx context.Context, input applyStoryMemoryPatchesInput) (string, error) {
		_ = callCtx
		if len(input.Patches) == 0 {
			err := fmt.Errorf("故事记忆 patch 不能为空")
			reportStateMaintenanceFailure(ctx, err)
			return "", err
		}
		records, err := ctx.Store.ApplyStoryMemoryPatches(ctx.StoryID, ctx.BranchID, ctx.TurnID, input.Patches)
		if err != nil {
			reportStateMaintenanceFailure(ctx, err)
			return "", err
		}
		if ctx.OnStoryMemoryApplied != nil {
			ctx.OnStoryMemoryApplied(len(records))
		}
		data, err := json.MarshalIndent(storyMemoryPatchToolOutput{
			AppliedRecords: len(records),
			BranchID:       ctx.BranchID,
			TurnID:         ctx.TurnID,
		}, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	})
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{applyTool}, nil
}

func reportStateMaintenanceFailure(ctx InteractiveStoryToolContext, err error) {
	if ctx.OnStateMaintenanceFailed != nil && err != nil {
		ctx.OnStateMaintenanceFailed(err)
	}
}

func newInteractiveTurnTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	if ctx.PrepareTurn == nil && ctx.SubmitTurnResult == nil {
		return nil, nil
	}
	tools := make([]tool.BaseTool, 0, 3)
	if ctx.PrepareTurn != nil {
		desc := strings.Join([]string{
			"执行本回合一次固定 d20 规则检定。Interactive Agent 负责填写用户行为、意图、挑战、消耗、当前状态说明、投前裁定依据、运行时加成来源和值、难度等级，以及大成功/成功/失败/大失败四档后果；本工具负责掷骰、应用优势或劣势、计算目标、判定结果，并返回命中的最终后果。",
			"参数协议：difficulty 必须是 very_easy/easy/normal/hard/very_hard；普通难度使用 normal，不要使用 medium/moderate。adjudication 必须说明为什么需要检定、stakes、难度依据、优势/劣势依据；引用状态时使用 state_refs 的 actor_id + field_id。rule 可省略；如提供，template 只能是 dice_check，roll_mode 只能是 normal/advantage/disadvantage，modifier 是模板难度修正值且正数更难；来自 TRPG 模板时填写 template_id、label、failure_policy。",
			"若本轮上下文提供了 TRPG 检定配置，请先用 trigger、must_check_examples、skip_check_examples 判断是否检定，再用 difficulty_guidance 判断 difficulty/bonuses。四档 outcomes 只描述叙事后果，不提交状态操作。",
			"若配置提供 state_bindings，请选择 binding_id，并填写 actor_id 与必要的 target_actor_id；binding 中的 modifiers 和 outcome_state_changes 会由工具自动读取 Actor State 并计算，不要重复手算。narrative_state_refs 只用于帮助你投前写好四档 outcomes.*.result。",
			`最小示例：{"action":"撬锁","intent":"潜入仓库","challenge":"巡逻逼近时开锁","cost":"失败会暴露行踪","state":"主角有简易工具。","adjudication":{"reason":"开锁有时间压力且失败会改变警戒状态。","stakes":"失败会让巡逻靠近。","difficulty_reason":"旧锁简单但附近有人巡逻，维持普通难度。","roll_mode_reason":"工具合适但环境紧张，正常投骰。","state_refs":[{"actor_id":"protagonist","field_id":"体力"}]},"rule":{"template_id":"dm-osr-player-skill","label":"OSR 型 DM：玩家技巧优先","failure_policy":"blocked","modifier":0},"bonuses":[{"kind":"equipment","reason":"有简易开锁工具","value":2}],"difficulty":"normal","outcomes":{"critical_success":{"result":"无声开锁并发现额外线索。"},"success":{"result":"开锁成功但耗时。"},"failure":{"result":"没能打开，巡逻更近。"},"critical_failure":{"result":"工具折断并惊动巡逻。"}}}`,
		}, "\n")
		prepareTool, err := utils.InferTool("prepare_interactive_turn", desc, func(callCtx context.Context, input interactiveTurnCheckToolInput) (string, error) {
			resolution, err := ctx.PrepareTurn(callCtx, input.request())
			if err != nil {
				return "", err
			}
			data, err := json.MarshalIndent(resolution.ToolOutput(), "", "  ")
			if err != nil {
				return "", err
			}
			return string(data), nil
		})
		if err != nil {
			return nil, err
		}
		tools = append(tools, prepareTool)
	}
	if ctx.SubmitTurnResult != nil {
		patchDesc := strings.Join([]string{
			"在完整玩家可见正文已经输出后，独立提交本回合 Actor 状态 patch。参数只有 patches；故事、分支、当前状态与配置由后端绑定。工具返回 ready、module_status、diagnostics 和 retry_modules；已 accepted 的模块会保留。",
			"patches 是原子操作数组，只能使用 replace、delta、create。路径是 JSON Pointer：第一段必须是当前上下文中列出的稳定 actor_id，第二段必须是冻结 schema 的 field_id；展示名称不能代替 actor_id。replace 设置字段或 object 子路径，delta 只增减已有数值，create 只在 /<actor_id> 根路径创建 Actor。不要重复 RuleResolution 已消费的字段。",
			"story_context 每回合至少 replace /story/当前事件；当前详细地点尚未初始化或正文确定地点变化时，同时 replace /story/当前详细地点。没有变化的其他字段不要写空值。",
		}, "\n")
		patchTool, err := newSubmitTurnModuleTool(interactiveActorStatePatchesToolName, patchDesc, submitActorStatePatchesToolSchema{}, interactive.DecodeActorStatePatchesSubmissionInput, ctx.SubmitTurnResult)
		if err != nil {
			return nil, err
		}
		choiceDesc := strings.Join([]string{
			"在完整玩家可见正文已经输出后，独立提交本回合下一步行动建议。参数只有 choices；必须与已输出正文结尾一致，并提供当前故事配置要求的恰好数量个不同建议。",
			"只有 prepare_interactive_turn 返回 terminal_candidate 的终局回合才提交空数组。工具返回 ready=false 时只调用 retry_modules 指定的工具；ready=true 后立即结束，不要重复或改写正文。",
		}, "\n")
		choiceTool, err := newSubmitTurnModuleTool(interactiveChoicesToolName, choiceDesc, submitChoicesToolSchema{}, interactive.DecodeChoicesSubmissionInput, ctx.SubmitTurnResult)
		if err != nil {
			return nil, err
		}
		tools = append(tools, patchTool, choiceTool)
	}
	return tools, nil
}

type submitActorStatePatchesToolSchema struct {
	Patches []interactive.StateUpdate `json:"patches" jsonschema:"description=本轮原子 Actor 状态 patch"`
}

type submitChoicesToolSchema struct {
	Choices []string `json:"choices" jsonschema:"description=当前故事配置数量的不同下一步行动建议；仅 RuleResolution 已声明 terminal_candidate 时为空数组"`
}

type submitTurnModuleTool struct {
	info   *schema.ToolInfo
	decode func(string) interactive.TurnSubmissionInput
	submit func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error)
}

func newSubmitTurnModuleTool(name, description string, input any, decode func(string) interactive.TurnSubmissionInput, submit func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error)) (tool.InvokableTool, error) {
	var info *schema.ToolInfo
	var err error
	switch input.(type) {
	case submitActorStatePatchesToolSchema:
		info, err = utils.GoStruct2ToolInfo[submitActorStatePatchesToolSchema](name, description)
	case submitChoicesToolSchema:
		info, err = utils.GoStruct2ToolInfo[submitChoicesToolSchema](name, description)
	default:
		return nil, fmt.Errorf("未知互动回合提交模块: %s", name)
	}
	if err != nil {
		return nil, err
	}
	return &submitTurnModuleTool{info: info, decode: decode, submit: submit}, nil
}

func (t *submitTurnModuleTool) Info(context.Context) (*schema.ToolInfo, error) {
	return t.info, nil
}

func (t *submitTurnModuleTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	input := t.decode(argumentsInJSON)
	receipt, err := t.submit(ctx, input)
	if err != nil {
		return "", err
	}
	if receipt.Ready {
		requested := requestInteractiveTurnCompletion(ctx)
		log.Printf("[interactive-turn] accepted all result modules completion_requested=%t", requested)
	}
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func normalizeInteractiveMemoryToolLimit(value, fallback, max int) int {
	if value <= 0 {
		value = fallback
	}
	if value > max {
		value = max
	}
	return value
}

func filterInteractiveMemoryToolEntries(entries []interactive.InteractiveMemoryEntry, input listInteractiveMemoriesInput) []interactive.InteractiveMemoryEntry {
	out := make([]interactive.InteractiveMemoryEntry, 0, len(entries))
	query := strings.ToLower(strings.TrimSpace(input.Query))
	people := normalizeInteractiveMemoryToolTerms(input.People)
	places := normalizeInteractiveMemoryToolTerms(input.Places)
	tags := normalizeInteractiveMemoryToolTerms(input.Tags)
	for _, entry := range entries {
		if query != "" && !interactiveMemoryEntryContains(entry, query) {
			continue
		}
		if len(people) > 0 && !interactiveMemoryListIntersects(entry.People, people) {
			continue
		}
		if len(places) > 0 && !interactiveMemoryListIntersects(entry.Places, places) {
			continue
		}
		if len(tags) > 0 && !interactiveMemoryListIntersects(entry.Tags, tags) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func normalizeInteractiveMemoryToolTerms(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func interactiveMemoryListIntersects(values []string, terms map[string]bool) bool {
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if terms[value] {
			return true
		}
	}
	return false
}

func interactiveMemoryEntryContains(entry interactive.InteractiveMemoryEntry, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		entry.ID,
		entry.Title,
		entry.Summary,
		entry.Content,
		strings.Join(entry.People, " "),
		strings.Join(entry.Places, " "),
		strings.Join(entry.Tags, " "),
	}, " "))
	for _, term := range strings.Fields(query) {
		if !strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}

func trimInteractiveMemoryToolText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		if value != "" {
			return ""
		}
		return value
	}
	if len(value) <= limit {
		return value
	}
	trimmed, _ := truncateUTF8Bytes(value, limit)
	return strings.TrimSpace(trimmed)
}

func marshalInteractiveMemoryToolOutput(output interactiveMemoryToolOutput) (string, error) {
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
