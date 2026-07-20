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
	contribjsonschema "github.com/eino-contrib/jsonschema"

	"denova/internal/interactive"
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
	OnLoreItemsRead          func([]string)
	SubmitStateSchemaBatch   func(context.Context, interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error)
	SubmitDirectorPlanUpdate func(context.Context, interactive.DirectorPlanUpdateSubmission) (interactive.DirectorPlanUpdateReceipt, error)
	// DisplayConversation receives display-only progress for background helper
	// agents. It must not receive final assistant text as model-visible context.
	DisplayConversation Conversation
	PrepareTurn         func(context.Context, interactive.TurnCheckRequest) (interactive.RuleResolution, error)
	SubmitTurnResult    func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error)
	TurnResultReady     func() bool
}

type searchStoryHistoryInput struct {
	Keywords     []string `json:"keywords,omitempty" jsonschema:"description=要检索的人物、地点、物品、线索或事件关键词；最多 8 个。留空时浏览最近回合。"`
	Match        string   `json:"match,omitempty" jsonschema:"description=关键词匹配方式：any 匹配任一关键词，all 要求全部匹配。默认 any。"`
	BeforeTurnID string   `json:"before_turn_id,omitempty" jsonschema:"description=只检索该 turn_id 之前的当前分支历史；用于避免把当前回合当作旧事实。"`
	Limit        int      `json:"limit,omitempty" jsonschema:"description=最多返回多少个历史回合，默认 8，最大 12。"`
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

func newInteractiveHistoryTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	ctx.StoryID = strings.TrimSpace(ctx.StoryID)
	ctx.BranchID = strings.TrimSpace(ctx.BranchID)
	if ctx.Store == nil || ctx.StoryID == "" {
		return nil, nil
	}
	searchTool, err := utils.InferTool("search_story_history", "检索当前分支已经提交的历史回合。Turn 事件是历史事实真源；结果只返回有界的玩家行动、叙事片段、状态变化和精确 turn_id，可随时从事件日志重建。需要承接较早人物、地点、线索、承诺或因果时使用；不要把检索结果当作当前 Actor State 或未来 Director 计划。", func(callCtx context.Context, input searchStoryHistoryInput) (string, error) {
		_ = callCtx
		result, err := ctx.Store.SearchStoryHistory(ctx.StoryID, ctx.BranchID, interactive.StoryHistorySearchRequest{
			Keywords:     input.Keywords,
			Match:        input.Match,
			BeforeTurnID: input.BeforeTurnID,
			Limit:        input.Limit,
		})
		if err != nil {
			return "", err
		}
		data, err := json.MarshalIndent(result, "", "  ")
		return string(data), err
	})
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{searchTool}, nil
}

func newInteractiveTurnTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	if ctx.PrepareTurn == nil && ctx.SubmitTurnResult == nil {
		return nil, nil
	}
	tools := make([]tool.BaseTool, 0, 2)
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
		desc := strings.Join([]string{
			"在完整玩家可见正文已经输出后，通过一个入口提交本回合 state_changes 与 choices。首次调用同时提供两者；工具返回 ready=false 时，只重交 retry_modules 指定的字段，已 accepted 的模块会保留。ready=true 后立即结束，不要重复或改写正文。",
			"如果当前回合提供 initialize_story_state_schema，必须在输出正文前先让结构草案 finalized=true；首次 state_changes 必须一次填写其回执 initialization_guide.required_state_changes 列出的全部字段，连同正文已经确定的其它主要状态，不能用空字符串、未设置、未知或待定占位。本工具不会代替结构初始化。",
			"state_changes 必须直接提交原生 JSON array，禁止把数组 JSON.stringify 后作为 string；单次最多 24 项。每一项严格三选一：replace={op,actor_id,field_id,value,可选 subpath}，delta={op,actor_id,field_id,value,可选 subpath}，create={op,actor_id,template_id,name,可选 role/description/initial_state}。create 的 schema 中不存在 field_id、subpath 或 value；新 Actor 的初始字段全部合并进同一个 create.initial_state，不要先对尚未创建的 Actor 连续 replace。只填写正文中确实发生变化的字段，使用 Actor 状态手册中的精确 ID；不要重复 RuleResolution 已消费的字段。",
			"新建 Actor 时 name 必填，actor_id 与 name 必须完全相同，直接使用故事语言中的角色名称，不得另造英文、拼音或 slug ID；引用已有 Actor 时逐字复用状态手册中的现有 actor_id。",
			"story_context 每回合至少 replace actor_id=story、field_id=当前事件；当前详细地点尚未初始化或正文确定地点变化时，同时 replace 当前详细地点。没有变化的其他字段不要写空值。",
			"choices 必须与已输出正文结尾一致，并提供当前故事配置要求的恰好数量个不同建议；只有 prepare_interactive_turn 返回 terminal_candidate 的终局回合才提交空数组。",
			"模块被 rejected 时修复同一批原定状态事实，不要通过删除已在正文成立的重要角色、能力、物品、地点或局势来绕过校验；可以合并同一新 Actor 的 initial_state 或压缩冗余描述。",
			"director_update 是可选的低频导演更新提示。普通承接、同一场景内的小变化、常规资源消耗或既定冲突推进必须省略。只有当前目标或阶段改变、关键关系/势力发生重大变化、重要秘密揭示、不可逆结果，或现有简报已经无法指导下一回合时才设置 needed=true，并只说明已发生事实；不要替 Director 决定 patch/replan 或具体文件。",
			"完整参数模板、当前可用 ID、字段类型，以及与本故事 choice_count 一致的 choices 占位数量，均以本轮 Actor 状态手册为准。",
		}, "\n")
		submitTool, err := newSubmitInteractiveTurnTool(desc, ctx.SubmitTurnResult)
		if err != nil {
			return nil, err
		}
		tools = append(tools, submitTool)
	}
	return tools, nil
}

type submitInteractiveTurnToolSchema struct {
	StateChanges   []interactive.TurnStateChangeInput `json:"state_changes,omitempty" jsonschema:"maxItems=24" jsonschema_description:"本轮正文已经发生的增量 Actor 状态变化。必须直接提交 JSON array，不能提交序列化后的 string；没有变化时提交空数组，单次最多 24 项。"`
	Choices        []string                           `json:"choices,omitempty" jsonschema_description:"当前故事配置数量的不同下一步行动建议；仅 RuleResolution 已声明 terminal_candidate 时为空数组。"`
	DirectorUpdate *interactive.DirectorUpdateHint    `json:"director_update,omitempty" jsonschema_description:"仅在本轮已发生事实让后续规划发生实质变化时提交；普通回合必须省略。"`
}

// The provider receives these three disjoint variants as state_changes.items
// oneOf. Runtime decoding remains centralized in interactive.TurnStateChangeInput,
// while the model cannot infer that create accepts replace-only fields.
type submitInteractiveTurnReplaceChangeSchema struct {
	Op      string   `json:"op" jsonschema:"required,enum=replace" jsonschema_description:"固定为 replace。"`
	ActorID string   `json:"actor_id" jsonschema:"required" jsonschema_description:"Actor 状态手册中的稳定 Actor ID。"`
	FieldID string   `json:"field_id" jsonschema:"required" jsonschema_description:"目标 Actor 模板中的精确 Field ID。"`
	Subpath []string `json:"subpath,omitempty" jsonschema:"maxItems=16" jsonschema_description:"仅 object 字段的嵌套更新使用；按层级填写字符串段。"`
	Value   any      `json:"value" jsonschema:"required" jsonschema_description:"本轮结束后的完整非空字段值，类型必须匹配 schema。"`
}

type submitInteractiveTurnDeltaChangeSchema struct {
	Op      string   `json:"op" jsonschema:"required,enum=delta" jsonschema_description:"固定为 delta。"`
	ActorID string   `json:"actor_id" jsonschema:"required" jsonschema_description:"Actor 状态手册中的稳定 Actor ID。"`
	FieldID string   `json:"field_id" jsonschema:"required" jsonschema_description:"目标 Actor 模板中的精确 number Field ID。"`
	Subpath []string `json:"subpath,omitempty" jsonschema:"maxItems=16" jsonschema_description:"仅 object 内已有数值的嵌套更新使用；按层级填写字符串段。"`
	Value   float64  `json:"value" jsonschema:"required" jsonschema_description:"对已有数值增加或减少的有限数值。"`
}

type submitInteractiveTurnCreateChangeSchema struct {
	Op           string         `json:"op" jsonschema:"required,enum=create" jsonschema_description:"固定为 create。"`
	ActorID      string         `json:"actor_id" jsonschema:"required" jsonschema_description:"直接使用故事语言中的角色名称，并与 name 完全相同。"`
	TemplateID   string         `json:"template_id" jsonschema:"required" jsonschema_description:"新 Actor 可用模板中的精确 Template ID。"`
	Name         string         `json:"name" jsonschema:"required" jsonschema_description:"故事语言中的角色名称，必须与 actor_id 完全相同。"`
	Role         string         `json:"role,omitempty" jsonschema_description:"新 Actor 在当前故事中的定位。"`
	Description  string         `json:"description,omitempty" jsonschema_description:"新 Actor 的简短说明。"`
	InitialState map[string]any `json:"initial_state,omitempty" jsonschema:"maxProperties=64" jsonschema_description:"所有可靠初始字段值；key 必须是所选模板中的精确 Field ID。"`
}

type submitInteractiveTurnTool struct {
	info   *schema.ToolInfo
	submit func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error)
}

func newSubmitInteractiveTurnTool(description string, submit func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error)) (tool.InvokableTool, error) {
	info, err := utils.GoStruct2ToolInfo[submitInteractiveTurnToolSchema](interactiveTurnSubmissionToolName, description)
	if err != nil {
		return nil, err
	}
	parameters, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		return nil, err
	}
	if parameters == nil || parameters.Properties == nil {
		return nil, fmt.Errorf("submit_interactive_turn schema missing root properties")
	}
	stateChanges, ok := parameters.Properties.Get("state_changes")
	if !ok || stateChanges == nil {
		return nil, fmt.Errorf("submit_interactive_turn schema missing state_changes")
	}
	replaceVariant, err := turnToolParameterSchema[submitInteractiveTurnReplaceChangeSchema]()
	if err != nil {
		return nil, err
	}
	deltaVariant, err := turnToolParameterSchema[submitInteractiveTurnDeltaChangeSchema]()
	if err != nil {
		return nil, err
	}
	createVariant, err := turnToolParameterSchema[submitInteractiveTurnCreateChangeSchema]()
	if err != nil {
		return nil, err
	}
	stateChanges.Items = &contribjsonschema.Schema{
		OneOf:       []*contribjsonschema.Schema{replaceVariant, deltaVariant, createVariant},
		Description: "严格选择 replace、delta、create 之一；不得混用不同操作的字段。",
	}
	info.ParamsOneOf = schema.NewParamsOneOfByJSONSchema(parameters)
	return &submitInteractiveTurnTool{info: info, submit: submit}, nil
}

func turnToolParameterSchema[T any]() (*contribjsonschema.Schema, error) {
	params, err := utils.GoStruct2ParamsOneOf[T]()
	if err != nil {
		return nil, fmt.Errorf("build submit_interactive_turn variant schema: %w", err)
	}
	result, err := params.ToJSONSchema()
	if err != nil {
		return nil, fmt.Errorf("convert submit_interactive_turn variant schema: %w", err)
	}
	return result, nil
}

func (t *submitInteractiveTurnTool) Info(context.Context) (*schema.ToolInfo, error) {
	return t.info, nil
}

func (t *submitInteractiveTurnTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	input := interactive.DecodeInteractiveTurnSubmissionInput(argumentsInJSON)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
