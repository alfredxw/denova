package interactive

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	mathrand "math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	maxTurnBriefTextBytes = 4000
	maxTurnBriefListItems = 24
	maxRuleChecksPerTurn  = 12
	maxRuleStateBindings  = 12
)

const (
	turnCheckAllowedDifficulties = "very_easy/easy/normal/hard/very_hard"
	turnCheckAllowedTemplates    = "dice_check"
	turnCheckAllowedDice         = "1d20"
	turnCheckAllowedRollModes    = "normal/advantage/disadvantage"
)

var diceExprPattern = regexp.MustCompile(`^\s*(\d*)d(\d+)\s*$`)

type DirectorEvent struct {
	ID                      string   `json:"id,omitempty"`
	Name                    string   `json:"name,omitempty"`
	Category                string   `json:"category,omitempty"`
	Status                  string   `json:"status,omitempty"`
	Enabled                 bool     `json:"enabled"`
	Summary                 string   `json:"summary,omitempty"`
	PublicSummary           string   `json:"public_summary,omitempty"`
	HiddenTruth             string   `json:"hidden_truth,omitempty"`
	Template                string   `json:"template,omitempty"`
	NormalizedTrigger       string   `json:"normalized_trigger,omitempty"`
	Intensity               string   `json:"intensity,omitempty"`
	RequiredForeshadowing   []string `json:"required_foreshadowing,omitempty"`
	PayoffTarget            string   `json:"payoff_target,omitempty"`
	Reward                  string   `json:"reward,omitempty"`
	Cost                    string   `json:"cost,omitempty"`
	FailureLevel            string   `json:"failure_level,omitempty"`
	CompatibleGenres        []string `json:"compatible_genres,omitempty"`
	IncompatibleStateFlags  []string `json:"incompatible_state_flags,omitempty"`
	UserConfigured          bool     `json:"user_configured,omitempty"`
	DirectorInstructionNote string   `json:"director_instruction_note,omitempty"`
}

// TurnBrief is retained for rule-template editor compatibility. The
// prepare_interactive_turn tool now uses TurnCheckRequest instead of asking the
// prose agent to submit a full brief.
type TurnBrief struct {
	UserAction       string      `json:"user_action,omitempty"`
	Intent           string      `json:"intent,omitempty"`
	TurnGoal         string      `json:"turn_goal,omitempty"`
	Pressure         string      `json:"pressure,omitempty"`
	EventIntents     []string    `json:"event_intents,omitempty"`
	CostPolicy       string      `json:"cost_policy,omitempty"`
	RuleChecks       []RuleCheck `json:"rule_checks,omitempty"`
	StateExpectation string      `json:"state_expectation,omitempty"`
	ContinuityNotes  string      `json:"continuity_notes,omitempty"`
}

type RuleCheck struct {
	ID                  string             `json:"id,omitempty"`
	Label               string             `json:"label,omitempty"`
	Dice                string             `json:"dice,omitempty"`
	Modifier            float64            `json:"modifier,omitempty"`
	FailurePolicy       string             `json:"failure_policy,omitempty"`
	DifficultyGuidance  string             `json:"difficulty_guidance,omitempty"`
	StateEffectGuidance string             `json:"state_effect_guidance,omitempty"`
	Trigger             string             `json:"trigger,omitempty"`
	MustCheckExamples   []string           `json:"must_check_examples,omitempty"`
	SkipCheckExamples   []string           `json:"skip_check_examples,omitempty"`
	SuccessHint         string             `json:"success_hint,omitempty"`
	FailureHint         string             `json:"failure_hint,omitempty"`
	StateBindings       []RuleStateBinding `json:"state_bindings,omitempty"`
}

type TurnCheckRequest struct {
	Action       string                `json:"action" jsonschema_description:"用户行为：本回合玩家实际尝试做什么。"`
	Intent       string                `json:"intent" jsonschema_description:"行动意图：玩家希望通过本行动达成的目标。"`
	Challenge    string                `json:"challenge" jsonschema_description:"检定挑战：需要 d20 固定裁定的风险、阻碍或冲突。"`
	Cost         string                `json:"cost" jsonschema_description:"潜在代价：失败、暴露、资源消耗或关系损失等后果。"`
	State        string                `json:"state" jsonschema_description:"当前状态说明：只写与本次检定直接相关的可见状态、资源、位置、关系或限制。"`
	Adjudication TurnCheckAdjudication `json:"adjudication,omitempty" jsonschema_description:"投前裁定依据：说明为什么需要检定、风险 stakes、难度依据、优势/劣势依据；状态引用使用 actor_id + field_id。"`
	Rule         TurnCheckRule         `json:"rule,omitempty" jsonschema_description:"可选规则设置；省略时默认 template=dice_check、roll_mode=normal、modifier=0。若来自 TRPG 模板，填写 template_id、label、failure_policy；如使用 state binding，填写 binding_id、actor_id 和必要的 target_actor_id。"`
	Bonuses      []TurnCheckBonus      `json:"bonuses,omitempty" jsonschema_description:"运行时加成或减值列表。正数表示有利条件，负数表示不利条件；固定 d20 下会加入检定总值。"`
	Difficulty   string                `json:"difficulty" jsonschema:"enum=very_easy,enum=easy,enum=normal,enum=hard,enum=very_hard" jsonschema_description:"五档难度枚举，只能使用 very_easy/easy/normal/hard/very_hard；普通难度用 normal，不要写 medium 或 moderate。"`
	Outcomes     TurnCheckOutcomes     `json:"outcomes" jsonschema_description:"四档后果定义。必须分别提供 critical_success、success、failure、critical_failure 的 result；可选 state_changes 会从命中的后果返回。"`
}

type TurnCheckAdjudication struct {
	Reason           string          `json:"reason,omitempty" jsonschema_description:"为什么本行动需要固定检定，而不是直接裁定。"`
	Stakes           string          `json:"stakes,omitempty" jsonschema_description:"这次检定的明确风险、代价或不可逆后果。"`
	DifficultyReason string          `json:"difficulty_reason,omitempty" jsonschema_description:"本次 difficulty 的判断依据。"`
	RollModeReason   string          `json:"roll_mode_reason,omitempty" jsonschema_description:"本次优势/劣势/正常投骰的判断依据。"`
	StateRefs        []ActorStateRef `json:"state_refs,omitempty" jsonschema_description:"本次裁定直接参考的状态字段；每项使用 actor_id 和故事冻结 schema 中的 field_id。"`
	StatePaths       []string        `json:"-"`
}

// ActorStateRef identifies one field without encoding it into a dotted path.
type ActorStateRef struct {
	ActorID string `json:"actor_id" jsonschema_description:"Actor ID。"`
	FieldID string `json:"field_id" jsonschema_description:"故事冻结 schema 中的状态名称/ID。"`
}

type TurnCheckRule struct {
	Template      string  `json:"template,omitempty" jsonschema:"enum=dice_check" jsonschema_description:"规则模板，可省略；如填写只能是 dice_check。"`
	TemplateID    string  `json:"template_id,omitempty" jsonschema_description:"命中的 TRPG 检定配置 ID，用于审计。"`
	Label         string  `json:"label,omitempty" jsonschema_description:"命中的 TRPG 检定配置名称，用于审计。"`
	FailurePolicy string  `json:"failure_policy,omitempty" jsonschema:"enum=fail_forward,enum=success_at_cost,enum=blocked,enum=hard_failure" jsonschema_description:"命中模板的失败处理策略，用于审计。"`
	Dice          string  `json:"dice,omitempty" jsonschema:"enum=1d20" jsonschema_description:"兼容字段；检定固定使用 1d20，可省略。"`
	RollMode      string  `json:"roll_mode,omitempty" jsonschema:"enum=normal,enum=advantage,enum=disadvantage" jsonschema_description:"投骰模式，可省略；normal 掷一次，advantage/disadvantage 分别取高/取低。"`
	Modifier      float64 `json:"modifier,omitempty" jsonschema_description:"模板难度修正值，正数更难、负数更容易；固定 d20 下会提高目标值。"`
	BindingID     string  `json:"binding_id,omitempty" jsonschema_description:"可选 State Binding 场景 ID；显式填写后工具按 TRPG 配置读取状态并计算修正。"`
	ActorID       string  `json:"actor_id,omitempty" jsonschema_description:"State Binding 行动 Actor ID；填写 binding_id 时必填。"`
	TargetActorID string  `json:"target_actor_id,omitempty" jsonschema_description:"State Binding 目标 Actor ID；binding 配置 target_template_id 时必填。"`
}

type TurnCheckBonus struct {
	Kind       string  `json:"kind,omitempty" jsonschema_description:"修正来源类型，例如 attribute/state/equipment/environment/help/other。"`
	ActorID    string  `json:"actor_id,omitempty" jsonschema_description:"结构化状态来源的 Actor ID；没有状态来源时可省略。"`
	FieldID    string  `json:"field_id,omitempty" jsonschema_description:"结构化状态来源的字段 ID；没有状态来源时可省略。"`
	SourcePath string  `json:"-"`
	Reason     string  `json:"reason" jsonschema_description:"加成或减值原因，必须能从当前状态或已知设定解释。"`
	Value      float64 `json:"value" jsonschema_description:"加成值，正数加到检定总值，负数从检定总值扣除。"`
}

type TurnCheckOutcomes struct {
	CriticalSuccess TurnCheckOutcome `json:"critical_success" jsonschema_description:"大成功后果：自然 20 或总值超过目标 10 以上时命中。"`
	Success         TurnCheckOutcome `json:"success" jsonschema_description:"成功后果：d20 总值达到目标时命中。"`
	Failure         TurnCheckOutcome `json:"failure" jsonschema_description:"失败后果：未达到成功且未达到大失败时命中。"`
	CriticalFailure TurnCheckOutcome `json:"critical_failure" jsonschema_description:"大失败后果：自然 1 或总值低于目标 10 以上时命中。"`
}

type TurnCheckOutcome struct {
	Result       string            `json:"result" jsonschema_description:"命中该档位时必须遵守的最终后果，用于指导正文。"`
	StateChanges []TurnStateChange `json:"state_changes,omitempty" jsonschema_description:"可选结构化状态增减，只写本次检定直接导致的数值变化。"`
}

type TurnStateChange struct {
	ActorID string  `json:"actor_id" jsonschema_description:"需要改变的 Actor ID。"`
	FieldID string  `json:"field_id" jsonschema_description:"故事冻结 schema 中的 number 状态名称/ID。"`
	Path    string  `json:"-"`
	Change  float64 `json:"change" jsonschema_description:"数值变化量，负数表示扣减，正数表示增加。"`
	Reason  string  `json:"reason,omitempty" jsonschema_description:"为什么该结果会导致这项状态变化。"`
}

type RuleResolution struct {
	ID                string                 `json:"id,omitempty"`
	Request           TurnCheckRequest       `json:"request"`
	Result            RuleResult             `json:"result"`
	StateConsumption  *RuleStateConsumption  `json:"state_consumption,omitempty"`
	StateBinding      *RuleStateBindingAudit `json:"state_binding,omitempty"`
	TerminalCandidate *TerminalCandidate     `json:"terminal_candidate,omitempty"`
	RuleConstraints   []string               `json:"rule_constraints,omitempty"`
	CreatedAt         string                 `json:"created_at,omitempty"`
	Seed              int64                  `json:"seed,omitempty"`
}

type RuleResult struct {
	ID              string            `json:"id,omitempty"`
	Label           string            `json:"label,omitempty"`
	Kind            string            `json:"kind,omitempty"`
	Mode            string            `json:"mode,omitempty"`
	AttributePath   string            `json:"attribute_path,omitempty"`
	AttributeValue  float64           `json:"attribute_value,omitempty"`
	Expression      string            `json:"expression,omitempty"`
	ExpressionValue float64           `json:"expression_value,omitempty"`
	Dice            string            `json:"dice,omitempty"`
	Rolls           []int             `json:"rolls,omitempty"`
	RollTotal       float64           `json:"roll_total,omitempty"`
	Modifier        float64           `json:"modifier,omitempty"`
	Difficulty      float64           `json:"difficulty,omitempty"`
	Total           float64           `json:"total,omitempty"`
	Outcome         string            `json:"outcome"`
	Seed            int64             `json:"seed,omitempty"`
	Constraints     []string          `json:"constraints,omitempty"`
	Error           string            `json:"error,omitempty"`
	RollMode        string            `json:"roll_mode,omitempty"`
	KeptRoll        float64           `json:"kept_roll,omitempty"`
	BonusTotal      float64           `json:"bonus_total,omitempty"`
	BonusDetails    []TurnCheckBonus  `json:"bonus_details,omitempty"`
	BaseTarget      float64           `json:"base_target,omitempty"`
	Target          float64           `json:"target,omitempty"`
	Result          string            `json:"result,omitempty"`
	StateChanges    []TurnStateChange `json:"state_changes,omitempty"`
}

type RuleResolutionToolOutput struct {
	ResolutionID string            `json:"resolution_id"`
	Label        string            `json:"label,omitempty"`
	Dice         string            `json:"dice"`
	RollMode     string            `json:"roll_mode"`
	Rolls        []int             `json:"rolls"`
	KeptRoll     int               `json:"kept_roll"`
	BonusTotal   float64           `json:"bonus_total"`
	BonusDetails []TurnCheckBonus  `json:"bonus_details,omitempty"`
	BaseTarget   float64           `json:"base_target"`
	Total        float64           `json:"total"`
	Difficulty   string            `json:"difficulty"`
	Target       float64           `json:"target"`
	Outcome      string            `json:"outcome"`
	Result       string            `json:"result"`
	Cost         string            `json:"cost,omitempty"`
	Stakes       string            `json:"stakes,omitempty"`
	StateChanges []TurnStateChange `json:"state_changes,omitempty"`
}

type TerminalCandidate struct {
	Type    string `json:"type,omitempty"`
	Reason  string `json:"reason,omitempty"`
	CheckID string `json:"check_id,omitempty"`
}

type TerminalOutcome struct {
	Terminal              bool     `json:"terminal"`
	Type                  string   `json:"type,omitempty"`
	Reason                string   `json:"reason,omitempty"`
	FinalNarrativeSummary string   `json:"final_narrative_summary,omitempty"`
	CausedByTurnID        string   `json:"caused_by_turn_id,omitempty"`
	RuleResolutionID      string   `json:"rule_resolution_id,omitempty"`
	RestartSuggestions    []string `json:"restart_suggestions,omitempty"`
}

func NormalizeTurnBrief(brief TurnBrief) TurnBrief {
	brief.UserAction = trimBytes(brief.UserAction, maxTurnBriefTextBytes)
	brief.Intent = trimBytes(brief.Intent, 256)
	brief.TurnGoal = trimBytes(brief.TurnGoal, maxTurnBriefTextBytes)
	brief.Pressure = trimBytes(brief.Pressure, maxTurnBriefTextBytes)
	brief.EventIntents = normalizeStringListLimit(brief.EventIntents, maxTurnBriefListItems)
	brief.CostPolicy = trimBytes(brief.CostPolicy, maxTurnBriefTextBytes)
	brief.StateExpectation = trimBytes(brief.StateExpectation, maxTurnBriefTextBytes)
	brief.ContinuityNotes = trimBytes(brief.ContinuityNotes, maxTurnBriefTextBytes)
	if len(brief.RuleChecks) > maxRuleChecksPerTurn {
		brief.RuleChecks = brief.RuleChecks[:maxRuleChecksPerTurn]
	}
	for i := range brief.RuleChecks {
		brief.RuleChecks[i] = normalizeRuleCheck(brief.RuleChecks[i], i)
	}
	return brief
}

func normalizeTurnBriefPointer(brief *TurnBrief) *TurnBrief {
	if brief == nil {
		return nil
	}
	normalized := NormalizeTurnBrief(*brief)
	if strings.TrimSpace(normalized.UserAction) == "" &&
		strings.TrimSpace(normalized.Intent) == "" &&
		strings.TrimSpace(normalized.TurnGoal) == "" &&
		len(normalized.RuleChecks) == 0 {
		return nil
	}
	return &normalized
}

func normalizeRuleResolutionPointer(resolution *RuleResolution) *RuleResolution {
	if resolution == nil {
		return nil
	}
	normalized := *resolution
	normalized.Request = NormalizeTurnCheckRequest(normalized.Request)
	normalized.Result.BonusDetails = normalizeTurnCheckBonuses(normalized.Result.BonusDetails)
	normalized.Result.StateChanges = normalizeTurnStateChanges(normalized.Result.StateChanges)
	normalized.StateConsumption = normalizeRuleStateConsumptionPointer(normalized.StateConsumption)
	normalized.StateBinding = normalizeRuleStateBindingAuditPointer(normalized.StateBinding)
	normalized.RuleConstraints = normalizeStringListLimit(normalized.RuleConstraints, maxTurnBriefListItems)
	return &normalized
}

func normalizeTerminalOutcomePointer(outcome *TerminalOutcome) *TerminalOutcome {
	if outcome == nil || !outcome.Terminal {
		return nil
	}
	normalized := *outcome
	normalized.Type = trimBytes(normalized.Type, 128)
	normalized.Reason = trimBytes(normalized.Reason, maxTurnBriefTextBytes)
	normalized.FinalNarrativeSummary = trimBytes(normalized.FinalNarrativeSummary, maxTurnBriefTextBytes)
	normalized.CausedByTurnID = trimBytes(normalized.CausedByTurnID, 128)
	normalized.RuleResolutionID = trimBytes(normalized.RuleResolutionID, 128)
	normalized.RestartSuggestions = normalizeStringListLimit(normalized.RestartSuggestions, 5)
	if len(normalized.RestartSuggestions) == 0 {
		normalized.RestartSuggestions = DefaultTerminalRestartSuggestions()
	}
	return &normalized
}

func DefaultTerminalRestartSuggestions() []string {
	return []string{
		"从上一安全回合创建新分支，改用更稳妥的行动。",
		"从关键选择前创建新分支，先收集情报、资源或盟友。",
	}
}

func NormalizeTurnCheckRequest(req TurnCheckRequest) TurnCheckRequest {
	req.Action = trimBytes(req.Action, maxTurnBriefTextBytes)
	req.Intent = trimBytes(req.Intent, maxTurnBriefTextBytes)
	req.Challenge = trimBytes(req.Challenge, maxTurnBriefTextBytes)
	req.Cost = trimBytes(req.Cost, maxTurnBriefTextBytes)
	req.State = trimBytes(req.State, maxTurnBriefTextBytes)
	req.Adjudication = normalizeTurnCheckAdjudication(req.Adjudication)
	req.Rule.Template = normalizeTurnCheckTemplate(req.Rule.Template)
	req.Rule.TemplateID = trimBytes(req.Rule.TemplateID, 128)
	req.Rule.Label = trimBytes(req.Rule.Label, 256)
	req.Rule.FailurePolicy = normalizeRuleCheckFailurePolicyOptional(req.Rule.FailurePolicy)
	req.Rule.Dice = normalizeTurnCheckDice(req.Rule.Dice)
	req.Rule.RollMode = normalizeTurnCheckRollMode(req.Rule.RollMode)
	req.Rule.BindingID = normalizeSlotID(req.Rule.BindingID)
	req.Rule.ActorID = normalizeActorStateID(req.Rule.ActorID)
	req.Rule.TargetActorID = normalizeActorStateID(req.Rule.TargetActorID)
	req.Difficulty = normalizeTurnCheckDifficulty(req.Difficulty)
	req.Bonuses = normalizeTurnCheckBonuses(req.Bonuses)
	req.Outcomes.CriticalSuccess = normalizeTurnCheckOutcome(req.Outcomes.CriticalSuccess)
	req.Outcomes.Success = normalizeTurnCheckOutcome(req.Outcomes.Success)
	req.Outcomes.Failure = normalizeTurnCheckOutcome(req.Outcomes.Failure)
	req.Outcomes.CriticalFailure = normalizeTurnCheckOutcome(req.Outcomes.CriticalFailure)
	return req
}

func ValidateTurnCheckRequest(req TurnCheckRequest) error {
	if strings.TrimSpace(req.Action) == "" {
		return fmt.Errorf("prepare_interactive_turn 缺少 action")
	}
	if strings.TrimSpace(req.Intent) == "" {
		return fmt.Errorf("prepare_interactive_turn 缺少 intent")
	}
	if strings.TrimSpace(req.Challenge) == "" {
		return fmt.Errorf("prepare_interactive_turn 缺少 challenge")
	}
	if strings.TrimSpace(req.Cost) == "" {
		return fmt.Errorf("prepare_interactive_turn 缺少 cost")
	}
	if strings.TrimSpace(req.State) == "" {
		return fmt.Errorf("prepare_interactive_turn 缺少 state")
	}
	if req.Rule.Template != "" && normalizeTurnCheckTemplate(req.Rule.Template) != "dice_check" {
		return fmt.Errorf("prepare_interactive_turn rule.template 无效: %s，合法值: %s", req.Rule.Template, turnCheckAllowedTemplates)
	}
	if req.Rule.FailurePolicy != "" && !validRuleCheckFailurePolicy(req.Rule.FailurePolicy) {
		return fmt.Errorf("prepare_interactive_turn rule.failure_policy 无效: %s", req.Rule.FailurePolicy)
	}
	if req.Rule.Dice != "" && !validTurnCheckDice(req.Rule.Dice) {
		return fmt.Errorf("prepare_interactive_turn rule.dice 无效: %s，合法值: %s", req.Rule.Dice, turnCheckAllowedDice)
	}
	if _, ok := turnCheckDifficultyTarget(req.Rule.Dice, req.Difficulty); !ok {
		return fmt.Errorf("prepare_interactive_turn difficulty 无效: %s，合法值: %s", req.Difficulty, turnCheckAllowedDifficulties)
	}
	for name, outcome := range map[string]TurnCheckOutcome{
		"critical_success": req.Outcomes.CriticalSuccess,
		"success":          req.Outcomes.Success,
		"failure":          req.Outcomes.Failure,
		"critical_failure": req.Outcomes.CriticalFailure,
	} {
		if strings.TrimSpace(outcome.Result) == "" {
			return fmt.Errorf("prepare_interactive_turn outcomes.%s 缺少 result", name)
		}
		for _, change := range outcome.StateChanges {
			if change.ActorID == "" || change.FieldID == "" {
				return fmt.Errorf("prepare_interactive_turn outcomes.%s.state_changes 必须提供 actor_id 和 field_id", name)
			}
		}
	}
	return nil
}

func ResolveTurnRules(storyID, branchID string, state map[string]any, req TurnCheckRequest) (RuleResolution, error) {
	return resolveTurnRulesWithSeed(storyID, branchID, state, req, 0)
}

func resolveTurnRulesWithSeed(storyID, branchID string, state map[string]any, req TurnCheckRequest, seed int64) (RuleResolution, error) {
	return resolveTurnRulesWithSeedAndDirector(storyID, branchID, state, StoryDirector{}, req, seed)
}

func ResolveTurnRulesWithDirector(storyID, branchID string, state map[string]any, director StoryDirector, req TurnCheckRequest) (RuleResolution, error) {
	return resolveTurnRulesWithSeedAndDirector(storyID, branchID, state, director, req, 0)
}

func resolveTurnRulesWithSeedAndDirector(storyID, branchID string, state map[string]any, director StoryDirector, req TurnCheckRequest, seed int64) (RuleResolution, error) {
	req = NormalizeTurnCheckRequest(req)
	if err := ValidateTurnCheckRequest(req); err != nil {
		return RuleResolution{}, err
	}
	bindingAudit, err := resolveRuleStateBinding(state, director, req)
	if err != nil {
		return RuleResolution{}, err
	}
	if seed == 0 {
		seed = newRuleSeed(storyID, branchID, req.Action, req.Challenge)
	}
	dice := normalizeTurnCheckDice(req.Rule.Dice)
	rolls, keptRoll, err := rollTurnCheck(seed, dice, req.Rule.RollMode)
	if err != nil {
		return RuleResolution{}, err
	}
	manualBonusTotal := turnCheckBonusTotal(req.Bonuses)
	advantageTotal := 0.0
	resistanceTotal := 0.0
	bonusDetails := append([]TurnCheckBonus(nil), req.Bonuses...)
	if bindingAudit != nil {
		advantageTotal = bindingAudit.BindingBonusTotal
		resistanceTotal = bindingAudit.BindingResistanceTotal
		bindingAudit.ManualBonusTotal = manualBonusTotal
		bonusDetails = append(bonusDetails, bindingAudit.BonusDetails...)
	}
	bonusTotal := manualBonusTotal + advantageTotal
	baseTarget, _ := turnCheckDifficultyTarget(dice, req.Difficulty)
	target := turnCheckTarget(dice, baseTarget, req.Rule.Modifier+resistanceTotal, bonusTotal)
	total := turnCheckTotal(dice, keptRoll, bonusTotal)
	outcomeName := resolveTurnCheckOutcome(dice, keptRoll, total, target)
	outcome := req.outcomeByName(outcomeName)
	resultStateChanges := normalizeTurnStateChanges(outcome.StateChanges)
	if bindingAudit != nil {
		configured, warnings, err := computeBindingOutcomeStateChanges(state, director.ActorState, bindingAudit, outcomeName)
		if err != nil {
			return RuleResolution{}, err
		}
		bindingAudit.ComputedStateChanges = configured
		bindingAudit.ManualStateChanges = append([]TurnStateChange(nil), resultStateChanges...)
		bindingAudit.Warnings = append(bindingAudit.Warnings, warnings...)
		resultStateChanges = mergeBindingStateChanges(configured, resultStateChanges)
		bindingAudit.Warnings = append(bindingAudit.Warnings, duplicateStateChangeWarnings(configured, outcome.StateChanges)...)
	}
	constraint := turnCheckConstraint(firstNonEmptyString(req.Challenge, req.Action), dice, outcomeName, total, target)
	result := RuleResult{
		ID:           "check_1",
		Label:        firstNonEmptyString(req.Rule.Label, req.Challenge, req.Action),
		Kind:         "dice_check",
		Mode:         turnCheckMode(dice),
		Dice:         dice,
		Rolls:        rolls,
		RollTotal:    float64(keptRoll),
		Modifier:     req.Rule.Modifier + resistanceTotal,
		Difficulty:   target,
		Total:        total,
		Outcome:      outcomeName,
		Seed:         seed,
		Constraints:  []string{constraint},
		RollMode:     req.Rule.RollMode,
		KeptRoll:     float64(keptRoll),
		BonusTotal:   bonusTotal,
		BonusDetails: bonusDetails,
		BaseTarget:   baseTarget,
		Target:       target,
		Result:       outcome.Result,
		StateChanges: resultStateChanges,
	}
	resolution := RuleResolution{
		ID:              newID("rr"),
		Request:         req,
		Result:          result,
		StateBinding:    bindingAudit,
		RuleConstraints: []string{constraint},
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		Seed:            seed,
	}
	return resolution, nil
}

var turnCheckD20DifficultyTargets = map[string]float64{
	"very_easy": 2,
	"easy":      5,
	"normal":    10,
	"hard":      15,
	"very_hard": 20,
}

func (req TurnCheckRequest) outcomeByName(name string) TurnCheckOutcome {
	switch name {
	case "critical_success":
		return req.Outcomes.CriticalSuccess
	case "success":
		return req.Outcomes.Success
	case "critical_failure":
		return req.Outcomes.CriticalFailure
	default:
		return req.Outcomes.Failure
	}
}

func (resolution RuleResolution) ToolOutput() RuleResolutionToolOutput {
	keptRoll := int(resolution.Result.KeptRoll)
	if keptRoll == 0 {
		keptRoll = int(resolution.Result.RollTotal)
	}
	return RuleResolutionToolOutput{
		ResolutionID: resolution.ID,
		Label:        resolution.Result.Label,
		Dice:         firstNonEmptyString(resolution.Result.Dice, "1d20"),
		RollMode:     firstNonEmptyString(resolution.Result.RollMode, "normal"),
		Rolls:        append([]int(nil), resolution.Result.Rolls...),
		KeptRoll:     keptRoll,
		BonusTotal:   resolution.Result.BonusTotal,
		BonusDetails: append([]TurnCheckBonus(nil), resolution.Result.BonusDetails...),
		BaseTarget:   resolution.Result.BaseTarget,
		Total:        resolution.Result.Total,
		Difficulty:   resolution.Request.Difficulty,
		Target:       resolution.Result.Target,
		Outcome:      resolution.Result.Outcome,
		Result:       resolution.Result.Result,
		Cost:         resolution.Request.Cost,
		Stakes:       resolution.Request.Adjudication.Stakes,
		StateChanges: append([]TurnStateChange(nil), resolution.Result.StateChanges...),
	}
}

func normalizeTurnCheckOutcome(outcome TurnCheckOutcome) TurnCheckOutcome {
	outcome.Result = trimBytes(outcome.Result, maxTurnBriefTextBytes)
	outcome.StateChanges = normalizeTurnStateChanges(outcome.StateChanges)
	return outcome
}

func normalizeTurnCheckAdjudication(value TurnCheckAdjudication) TurnCheckAdjudication {
	value.Reason = trimBytes(value.Reason, maxTurnBriefTextBytes)
	value.Stakes = trimBytes(value.Stakes, maxTurnBriefTextBytes)
	value.DifficultyReason = trimBytes(value.DifficultyReason, maxTurnBriefTextBytes)
	value.RollModeReason = trimBytes(value.RollModeReason, maxTurnBriefTextBytes)
	value.StateRefs = normalizeActorStateRefs(value.StateRefs)
	for _, path := range normalizeStatePathList(value.StatePaths) {
		if actorID, fieldID, ok := parseActorStateFieldPath(path); ok {
			value.StateRefs = append(value.StateRefs, ActorStateRef{ActorID: actorID, FieldID: fieldID})
		}
	}
	value.StateRefs = normalizeActorStateRefs(value.StateRefs)
	value.StatePaths = normalizeStatePathList(value.StatePaths)
	return value
}

func normalizeTurnCheckBonuses(values []TurnCheckBonus) []TurnCheckBonus {
	if len(values) > maxTurnBriefListItems {
		values = values[:maxTurnBriefListItems]
	}
	out := make([]TurnCheckBonus, 0, len(values))
	for _, value := range values {
		value.Kind = normalizeTurnCheckEnumToken(value.Kind)
		value.ActorID = normalizeActorStateID(value.ActorID)
		value.FieldID = normalizeActorStateFieldName(value.FieldID)
		legacyPath := strings.TrimSpace(value.SourcePath)
		if value.ActorID == "" || value.FieldID == "" {
			if actorID, fieldID, ok := parseActorStateFieldPath(value.SourcePath); ok {
				value.ActorID = actorID
				value.FieldID = fieldID
			}
		}
		if validStatePathSyntax(legacyPath) {
			value.SourcePath = canonicalStatePath(legacyPath)
		} else {
			value.SourcePath = ""
		}
		value.Reason = trimBytes(value.Reason, 512)
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeTurnStateChanges(values []TurnStateChange) []TurnStateChange {
	if len(values) > maxTurnBriefListItems {
		values = values[:maxTurnBriefListItems]
	}
	out := make([]TurnStateChange, 0, len(values))
	for _, value := range values {
		value.ActorID = normalizeActorStateID(value.ActorID)
		value.FieldID = normalizeActorStateFieldName(value.FieldID)
		if value.ActorID == "" || value.FieldID == "" {
			if actorID, fieldID, ok := parseActorStateFieldPath(value.Path); ok {
				value.ActorID = actorID
				value.FieldID = fieldID
			}
		}
		if value.ActorID != "" && value.FieldID != "" {
			value.Path = actorStateFieldPath(value.ActorID, value.FieldID)
		} else {
			value.Path = ""
		}
		value.Reason = trimBytes(value.Reason, 512)
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeActorStateRefs(values []ActorStateRef) []ActorStateRef {
	if len(values) > maxTurnBriefListItems {
		values = values[:maxTurnBriefListItems]
	}
	out := make([]ActorStateRef, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value.ActorID = normalizeActorStateID(value.ActorID)
		value.FieldID = normalizeActorStateFieldName(value.FieldID)
		key := value.ActorID + "\x00" + actorStateFieldNameKey(value.FieldID)
		if value.ActorID == "" || value.FieldID == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeStatePathList(values []string) []string {
	if len(values) > maxTurnBriefListItems {
		values = values[:maxTurnBriefListItems]
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !validStatePathSyntax(value) {
			continue
		}
		value = canonicalStatePath(value)
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeTurnCheckRollMode(value string) string {
	switch normalizeTurnCheckEnumToken(value) {
	case "", "normal":
		return "normal"
	case "advantage", "disadvantage":
		return normalizeTurnCheckEnumToken(value)
	default:
		return normalizeTurnCheckEnumToken(value)
	}
}

func normalizeTurnCheckDifficulty(value string) string {
	switch normalizeTurnCheckEnumToken(value) {
	case "", "normal", "medium", "moderate":
		return "normal"
	case "very_easy", "easy", "hard", "very_hard":
		return normalizeTurnCheckEnumToken(value)
	default:
		return normalizeTurnCheckEnumToken(value)
	}
}

func normalizeTurnCheckTemplate(value string) string {
	switch normalizeTurnCheckEnumToken(value) {
	case "", "dice_check", "d20_check":
		return "dice_check"
	default:
		return normalizeTurnCheckEnumToken(value)
	}
}

func normalizeTurnCheckDice(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "d20", "1d20", "d100", "1d100":
		return "1d20"
	default:
		return value
	}
}

func validTurnCheckDice(value string) bool {
	switch normalizeTurnCheckDice(value) {
	case "1d20":
		return true
	default:
		return false
	}
}

func normalizeTurnCheckEnumToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", " ")
	return strings.Join(strings.Fields(value), "_")
}

func rollTurnCheck(seed int64, dice string, rollMode string) ([]int, int, error) {
	count := 1
	switch normalizeTurnCheckRollMode(rollMode) {
	case "normal":
		count = 1
	case "advantage", "disadvantage":
		count = 2
	default:
		return nil, 0, fmt.Errorf("prepare_interactive_turn rule.roll_mode 无效: %s，合法值: %s", rollMode, turnCheckAllowedRollModes)
	}
	sides := 20
	rolls, _, err := rollDice(seed, fmt.Sprintf("%dd%d", count, sides))
	if err != nil {
		return nil, 0, err
	}
	kept := rolls[0]
	normalizedRollMode := normalizeTurnCheckRollMode(rollMode)
	if normalizedRollMode == "advantage" {
		for _, roll := range rolls[1:] {
			if roll > kept {
				kept = roll
			}
		}
	}
	if normalizedRollMode == "disadvantage" {
		for _, roll := range rolls[1:] {
			if roll < kept {
				kept = roll
			}
		}
	}
	return rolls, kept, nil
}

func turnCheckBonusTotal(bonuses []TurnCheckBonus) float64 {
	total := 0.0
	for _, bonus := range bonuses {
		total += bonus.Value
	}
	return total
}

func turnCheckDifficultyTarget(dice string, difficulty string) (float64, bool) {
	normalizedDifficulty := normalizeTurnCheckDifficulty(difficulty)
	target, ok := turnCheckD20DifficultyTargets[normalizedDifficulty]
	return target, ok
}

func turnCheckTarget(dice string, baseTarget, modifier, bonusTotal float64) float64 {
	return baseTarget + modifier
}

func turnCheckTotal(dice string, keptRoll int, bonusTotal float64) float64 {
	return float64(keptRoll) + bonusTotal
}

func turnCheckMode(dice string) string {
	return "d20_dc"
}

func turnCheckConstraint(challenge, dice, outcome string, total, target float64) string {
	return fmt.Sprintf("%s：%s，总值 %.0f / 目标 %.0f。", challenge, turnCheckOutcomeText(outcome), total, target)
}

func resolveTurnCheckOutcome(dice string, keptRoll int, total, target float64) string {
	if keptRoll == 20 {
		return "critical_success"
	}
	if keptRoll == 1 {
		return "critical_failure"
	}
	if total >= target+10 {
		return "critical_success"
	}
	if total >= target {
		return "success"
	}
	if total <= target-10 {
		return "critical_failure"
	}
	return "failure"
}

func turnCheckOutcomeText(outcome string) string {
	switch outcome {
	case "critical_success":
		return "大成功"
	case "success":
		return "成功"
	case "critical_failure":
		return "大失败"
	default:
		return "失败"
	}
}

func normalizeRuleCheck(check RuleCheck, index int) RuleCheck {
	check.ID = strings.TrimSpace(check.ID)
	if check.ID == "" {
		check.ID = fmt.Sprintf("check_%d", index+1)
	}
	check.Label = trimBytes(firstNonEmptyString(check.Label, check.ID), 256)
	check.Dice = normalizeTurnCheckDice(check.Dice)
	check.FailurePolicy = normalizeRuleCheckFailurePolicy(check.FailurePolicy)
	check.DifficultyGuidance = trimBytes(check.DifficultyGuidance, maxTurnBriefTextBytes)
	check.StateEffectGuidance = trimBytes(check.StateEffectGuidance, maxTurnBriefTextBytes)
	check.Trigger = trimBytes(check.Trigger, maxTurnBriefTextBytes)
	check.MustCheckExamples = normalizeStringListLimit(check.MustCheckExamples, 8)
	check.SkipCheckExamples = normalizeStringListLimit(check.SkipCheckExamples, 8)
	check.SuccessHint = trimBytes(check.SuccessHint, maxTurnBriefTextBytes)
	check.FailureHint = trimBytes(check.FailureHint, maxTurnBriefTextBytes)
	check.StateBindings = normalizeRuleStateBindings(check.StateBindings)
	return check
}

func validateRuleCheck(check RuleCheck) error {
	if !validTurnCheckDice(check.Dice) {
		return fmt.Errorf("规则检定 dice 无效: %s，合法值: %s", check.Dice, turnCheckAllowedDice)
	}
	if !validRuleCheckFailurePolicy(check.FailurePolicy) {
		return fmt.Errorf("规则检定 failure_policy 无效: %s", check.FailurePolicy)
	}
	if err := validateRuleStateBindings(check.StateBindings); err != nil {
		return err
	}
	return nil
}

func normalizeRuleCheckFailurePolicy(value string) string {
	switch normalizeTurnCheckEnumToken(value) {
	case "", "fail_forward", "failure_forward":
		return "fail_forward"
	case "success_at_cost", "cost_success", "costly_success":
		return "success_at_cost"
	case "blocked", "temporary_blocked", "temporarily_blocked":
		return "blocked"
	case "hard_failure", "clear_failure", "failure":
		return "hard_failure"
	default:
		return normalizeTurnCheckEnumToken(value)
	}
}

func normalizeRuleCheckFailurePolicyOptional(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return normalizeRuleCheckFailurePolicy(value)
}

func validRuleCheckFailurePolicy(value string) bool {
	switch value {
	case "fail_forward", "success_at_cost", "blocked", "hard_failure":
		return true
	default:
		return false
	}
}

func rollDice(seed int64, expr string) ([]int, float64, error) {
	count, sides, err := parseDice(expr)
	if err != nil {
		return nil, 0, err
	}
	rng := mathrand.New(mathrand.NewSource(seed))
	rolls := make([]int, 0, count)
	total := 0
	for i := 0; i < count; i++ {
		roll := rng.Intn(sides) + 1
		rolls = append(rolls, roll)
		total += roll
	}
	return rolls, float64(total), nil
}

func parseDice(expr string) (int, int, error) {
	matches := diceExprPattern.FindStringSubmatch(expr)
	if matches == nil {
		return 0, 0, fmt.Errorf("骰子表达式仅支持 NdM，例如 1d20")
	}
	count := 1
	if matches[1] != "" {
		parsed, err := strconv.Atoi(matches[1])
		if err != nil {
			return 0, 0, fmt.Errorf("骰子数量无效: %s", matches[1])
		}
		count = parsed
	}
	sides, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, 0, fmt.Errorf("骰子面数无效: %s", matches[2])
	}
	if count <= 0 || count > 20 {
		return 0, 0, fmt.Errorf("骰子数量必须在 1 到 20 之间")
	}
	if sides <= 1 || sides > 1000 {
		return 0, 0, fmt.Errorf("骰子面数必须在 2 到 1000 之间")
	}
	return count, sides, nil
}

func newRuleSeed(parts ...string) int64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return int64(binary.LittleEndian.Uint64(buf[:]))
	}
	return time.Now().UnixNano()
}

func numberFromAny(value any) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float64:
		return typed
	case float32:
		return float64(typed)
	case string:
		out, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return out
	default:
		return 0
	}
}

func normalizeDirectorEvents(values []DirectorEvent) []DirectorEvent {
	if len(values) > maxTurnBriefListItems {
		values = values[:maxTurnBriefListItems]
	}
	out := make([]DirectorEvent, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value.ID = trimBytes(value.ID, 128)
		value.Name = trimBytes(value.Name, 256)
		value.Category = trimBytes(value.Category, 128)
		value.Status = trimBytes(value.Status, 128)
		value.Summary = trimBytes(value.Summary, maxTurnBriefTextBytes)
		value.PublicSummary = trimBytes(value.PublicSummary, maxTurnBriefTextBytes)
		value.HiddenTruth = trimBytes(value.HiddenTruth, maxTurnBriefTextBytes)
		value.Template = trimBytes(value.Template, maxTurnBriefTextBytes)
		value.NormalizedTrigger = trimBytes(value.NormalizedTrigger, maxTurnBriefTextBytes)
		value.Intensity = trimBytes(value.Intensity, 128)
		value.RequiredForeshadowing = normalizeStringListLimit(value.RequiredForeshadowing, maxTurnBriefListItems)
		value.PayoffTarget = trimBytes(value.PayoffTarget, maxTurnBriefTextBytes)
		value.Reward = trimBytes(value.Reward, maxTurnBriefTextBytes)
		value.Cost = trimBytes(value.Cost, maxTurnBriefTextBytes)
		value.FailureLevel = trimBytes(value.FailureLevel, 128)
		value.CompatibleGenres = normalizeStringListLimit(value.CompatibleGenres, maxTurnBriefListItems)
		value.IncompatibleStateFlags = normalizeStringListLimit(value.IncompatibleStateFlags, maxTurnBriefListItems)
		value.DirectorInstructionNote = trimBytes(value.DirectorInstructionNote, maxTurnBriefTextBytes)
		key := value.ID
		if key == "" {
			key = value.Name
		}
		if key == "" || seen[key] {
			continue
		}
		if !value.Enabled && value.Status == "" {
			value.Enabled = true
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func normalizeStateOpsForRule(ops []StateOp) []StateOp {
	out := make([]StateOp, 0, len(ops))
	for _, op := range ops {
		op.Op = strings.TrimSpace(op.Op)
		op.Path = canonicalStatePath(op.Path)
		op.Reason = trimBytes(op.Reason, maxTurnBriefTextBytes)
		op.SourceTurnID = trimBytes(op.SourceTurnID, 128)
		op.SourceKind = trimBytes(op.SourceKind, 128)
		op.SourceID = trimBytes(op.SourceID, 128)
		if op.Op == "" || op.Path == "" {
			continue
		}
		out = append(out, op)
	}
	return out
}

func normalizeStringListLimit(values []string, limit int) []string {
	if limit <= 0 {
		limit = maxTurnBriefListItems
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = trimBytes(value, 512)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func trimBytes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	trimmed := truncateUTF8(value, limit)
	return strings.TrimSpace(trimmed)
}

func validStatePathSyntax(path string) bool {
	path = strings.TrimSpace(path)
	return path != "" && !strings.HasPrefix(path, ".") && !strings.HasSuffix(path, ".") && !strings.Contains(path, "..")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func truncateUTF8(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit > len(value) {
		limit = len(value)
	}
	for limit > 0 && (value[limit]&0xC0) == 0x80 {
		limit--
	}
	return value[:limit]
}
