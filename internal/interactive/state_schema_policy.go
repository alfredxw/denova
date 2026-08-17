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

	// OpeningStateSchemaFieldSelectionRules is shared by the opening prompt and
	// tool description so the Agent sees one stable, cache-friendly contract.
	OpeningStateSchemaFieldSelectionRules = "Field selection is a hard requirement. The default state system includes only broadly useful level and health fields; the absence of other fields from a template does not make them optional. When opening facts, loaded Lore, or the active TRPG state_binding show that a persistent value changes independently, is consumed or restored, triggers thresholds, participates in checks, or must be displayed separately, add or replace it with a dedicated number/string/bool/enum/object/list field. Do not bury it in current situation, current event, world state, or item descriptions. Fixed d20 resolves randomness only and does not justify D&D-style state fields. Do not add or retain strength, dexterity, constitution, intelligence, wisdom, charisma, attack AC, defense DC, mana, ongoing effects, or cooldowns merely because d20 is used. Every covered/add/replace decision requires support from opening facts, loaded Lore, or the active TRPG state_binding. Remove inherited fields that have no independent tracking value. Level and health are not mandatory for every story; remove or replace them when they do not apply."
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
	base := strings.Join([]string{
		"On the first turn of this story, call initialize_story_state_schema before producing narrative text, and wait until the tool returns finalized=true. The schema tool defines templates and fields only. Opening sources must use source.kind=opening and source.id=opening-draft exactly. value_policy must be schema_only. covered/add/replace decisions must include template_id, field_id, and an expected_type of number/string/bool/enum/object/list. remove decisions must include the existing template_id, field_id, a reason, and the matching field-removal operation. Schema requirements and template_ops use Template IDs from the state handbook, never Actor IDs. story is an actor_id whose template_id is story_context. Do not submit initial_actor_ops or actor_ops.",
		OpeningStateSchemaFieldSelectionRules,
		"Relationship type, relationship stage, and affinity are separate dimensions. Friend, romantic, family, mentor, rival, hostile, and other relationship types use stages appropriate to the story and must not be inferred automatically from affinity. Use a covered review for one concrete field only when no independent state requirement actually exists.",
		"After finalization, follow initialization_guide.required_state_changes exactly and initialize every field that still lacks a value in the first submit_interactive_turn.state_changes call. Do not use empty strings or placeholders such as unset, unknown, or pending. The schema draft, opening narrative, initial state, and choices are persisted atomically only when this turn succeeds.",
	}, " ")
	if mode == StoryStateSchemaModeGenerate {
		return base + " The current handbook contains only Denova's non-removable protagonist and story-continuity core. Add only the templates and fields that the actual opening requires for persistent tracking; do not add unused fields merely for completeness."
	}
	return base + " The current handbook comes from the user's selected state template. Keep, add, replace, or remove fields so it represents only independently tracked state that this story truly needs. Do not duplicate existing fields for formal completeness, and do not alter fields still bound by TRPG rules. Every writable field retained for an opening Actor must receive a concrete initial value from source facts, a justified inference, or an applicable template default."
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
