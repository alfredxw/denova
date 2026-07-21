package interactive

import "strings"

const (
	// StoryStateSchemaModeAdaptTemplate lets the opening Game Agent tailor a
	// selected reusable template before the first turn is committed.
	StoryStateSchemaModeAdaptTemplate = "adapt_template"
	// StoryStateSchemaModeFixedTemplate freezes the selected reusable template
	// as-is and never exposes the opening schema tool.
	StoryStateSchemaModeFixedTemplate = "fixed_template"
	// StoryStateSchemaModeGenerate starts from Denova's invariant core and lets
	// the opening Game Agent build the story-specific fields.
	StoryStateSchemaModeGenerate = "generate"
)

// StoryStateSchemaPolicy is story-owned configuration. Director presets do
// not decide whether or how the opening Game Agent initializes state schema.
type StoryStateSchemaPolicy struct {
	Mode string `json:"mode"`
}

func NormalizeStoryStateSchemaPolicy(policy StoryStateSchemaPolicy) StoryStateSchemaPolicy {
	mode := strings.TrimSpace(policy.Mode)
	switch mode {
	case StoryStateSchemaModeAdaptTemplate, StoryStateSchemaModeFixedTemplate, StoryStateSchemaModeGenerate:
	case "after_opening", "off":
		// These values belonged to the removed Director-owned flow. Stories that
		// still contain either value keep their already-frozen schema unchanged.
		mode = StoryStateSchemaModeFixedTemplate
	default:
		mode = StoryStateSchemaModeAdaptTemplate
	}
	return StoryStateSchemaPolicy{Mode: mode}
}

func cloneStoryStateSchemaPolicy(policy *StoryStateSchemaPolicy) *StoryStateSchemaPolicy {
	if policy == nil {
		return nil
	}
	normalized := NormalizeStoryStateSchemaPolicy(*policy)
	return &normalized
}

func fixedStoryStateSchemaPolicy() *StoryStateSchemaPolicy {
	return &StoryStateSchemaPolicy{Mode: StoryStateSchemaModeFixedTemplate}
}

func normalizeFixedStoryStateSchemaInitialization(meta *StoryMeta) {
	if meta == nil || meta.StateSchemaPolicy == nil || NormalizeStoryStateSchemaPolicy(*meta.StateSchemaPolicy).Mode != StoryStateSchemaModeFixedTemplate {
		return
	}
	revision := actorStateSchemaRevision(meta.ActorStateSchema)
	completedAt := firstNonEmptyString(meta.UpdatedAt, meta.CreatedAt)
	if meta.StateSchemaInitialization != nil {
		completedAt = firstNonEmptyString(meta.StateSchemaInitialization.CompletedAt, completedAt)
	}
	meta.StateSchemaInitialization = &StateSchemaInitializationStatus{
		Mode:           StoryStateSchemaModeFixedTemplate,
		Status:         StateSchemaInitializationReady,
		Outcome:        "fixed",
		BaseRevision:   revision,
		TargetRevision: revision,
		CompletedAt:    completedAt,
		UpdatedAt:      completedAt,
	}
}

func storyStateSchemaPolicyRequiresOpeningDraft(policy *StoryStateSchemaPolicy) bool {
	if policy == nil {
		return false
	}
	switch NormalizeStoryStateSchemaPolicy(*policy).Mode {
	case StoryStateSchemaModeAdaptTemplate, StoryStateSchemaModeGenerate:
		return true
	case StoryStateSchemaModeFixedTemplate:
		return false
	}
	return false
}

// StoryStateSchemaPolicyUsesOpeningGameAgent reports whether the first Game
// Agent turn must finalize a run-local schema draft before state submission.
func StoryStateSchemaPolicyUsesOpeningGameAgent(policy *StoryStateSchemaPolicy) bool {
	return storyStateSchemaPolicyRequiresOpeningDraft(policy)
}

// OpeningGameStateSchemaInstruction is a bounded, story-meta-derived runtime
// contract. It never contains growing history or user content.
func OpeningGameStateSchemaInstruction(meta StoryMeta) string {
	if !storyStateSchemaPolicyRequiresOpeningDraft(meta.StateSchemaPolicy) || meta.StateSchemaInitialization == nil || meta.StateSchemaInitialization.Status != StateSchemaInitializationWaitingOpening {
		return ""
	}
	mode := NormalizeStoryStateSchemaPolicy(*meta.StateSchemaPolicy).Mode
	base := "本故事的首回合必须先调用 initialize_story_state_schema，并在工具 finalized=true 后再输出正文。结构工具只定义模板与字段：开局来源必须精确填写 source.kind=opening、source.id=opening-draft；value_policy 固定为 schema_only；covered/add/replace 必须填写 template_id、field_id 与 number/string/bool/enum/object/list 之一的 expected_type。结构 requirement 与 template_ops 使用状态手册中的 Template ID，不能使用 Actor ID；story 是 actor_id，对应的 template_id 是 story_context。不要提交 initial_actor_ops 或 actor_ops。先审查开局中会独立变化、消耗、触发阈值、参与检定或需要单独展示的长期状态；通用的当前处境、当前事件、世界局势或物品描述只能覆盖整体摘要，不能覆盖氧气、完整度、警戒值、倒计时等有独立更新节奏的资源，这些状态必须使用专用的 number/enum/bool 等字段。只有确实不存在独立状态需求时才用一个具体字段的 covered 审查项。finalize 后严格按回执 initialization_guide.required_state_changes，在首次 submit_interactive_turn.state_changes 中一次补齐所有仍缺初值的字段；不得使用空字符串、未设置、未知或待定占位。结构草案、开局正文、初始状态和 choices 只会在本轮成功结束时一起原子落盘。"
	if mode == StoryStateSchemaModeGenerate {
		return base + " 当前手册只有 Denova 不可删除的主角与故事连续性核心；请根据实际开局补齐真正需要长期追踪的模板和字段，不要为了完整感添加无用途字段。"
	}
	return base + " 当前手册来自用户选择的状态模板；添加或替换本故事真正需要的独立字段，不为形式完整重复现有字段，也不要改动仍被 TRPG 规则绑定的字段。最终保留的每个开局 Actor 可写字段都必须能由来源事实、合理推断或模板默认值获得具体初值。"
}

// GeneratedStoryActorStateCore is the non-removable platform contract used by
// fully generated stories. Everything beyond these two Actor identities and
// the two scene continuity fields is decided by the opening Game Agent.
func GeneratedStoryActorStateCore() StoryDirectorActorStateSystem {
	return normalizeActorStateSystem(StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			{
				ID:          DefaultActorID,
				Name:        "主角状态",
				Description: "当前故事的可玩主角；开局 Game Agent 按故事需要补充长期状态字段。",
			},
			{
				ID:          ActorStateStoryContextTemplateID,
				Name:        "故事状态",
				Description: "维持每回合可承接的最小场景连续性。",
				Fields: []ActorStateField{
					textStateField("scene.location", storyContextCurrentLocationField, "当前可行动场景的具体地点。", "当前场景", "inline"),
					textStateField("scene.current_event", storyContextCurrentEventField, "正在发生的事件、直接压力和下一步必须面对的问题。", "当前场景", "block"),
				},
			},
		},
		InitialActors: []ActorStateInitialActor{
			{
				ID:          DefaultActorID,
				Name:        "主角",
				TemplateID:  DefaultActorID,
				Role:        "protagonist",
				Description: "当前故事的可玩主角。",
			},
			{
				ID:          DefaultStoryContextActorID,
				Name:        "故事状态",
				TemplateID:  ActorStateStoryContextTemplateID,
				Role:        "story_context",
				Description: "当前场景的最小连续性状态。",
			},
		},
	})
}

// BuildActorStateInitialSnapshot materializes the same bounded initial state
// that will be prepended to an opening atomic commit. It is used for validating
// the Game Agent's staged state_changes before anything is persisted.
func BuildActorStateInitialSnapshot(system StoryDirectorActorStateSystem, rolls []InitialActorTraitRoll) (map[string]any, error) {
	state := initialStoryState()
	ops, actorOps, err := BuildActorStateInitialChanges(system, rolls)
	if err != nil {
		return nil, err
	}
	for _, op := range ops {
		applyStateOp(state, op)
	}
	for _, op := range actorOps {
		applyActorStateOp(state, op)
	}
	return state, nil
}
