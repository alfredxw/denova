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
	maxInteractiveTextBytes = 4000
	maxInteractiveListItems = 24
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
	Action       string                `json:"action" jsonschema_description:"The player's actual attempted action this turn."`
	Intent       string                `json:"intent" jsonschema_description:"The goal the player intends to achieve through this action."`
	Challenge    string                `json:"challenge" jsonschema_description:"The risk, obstacle, or conflict that requires a fixed d20 check."`
	Cost         string                `json:"cost" jsonschema_description:"Potential consequences such as failure, exposure, resource loss, or relationship damage."`
	State        string                `json:"state" jsonschema_description:"Only visible state, resources, position, relationships, or restrictions directly relevant to this check."`
	Adjudication TurnCheckAdjudication `json:"adjudication,omitempty" jsonschema_description:"Pre-roll adjudication: why a check is required, the stakes, the difficulty basis, and the advantage/disadvantage basis. Reference state with actor_id and field_id."`
	Rule         TurnCheckRule         `json:"rule,omitempty" jsonschema_description:"Optional rule settings. Defaults are template=dice_check, roll_mode=normal, and modifier=0. For a TRPG template, include template_id, label, and failure_policy. For a state binding, include binding_id, actor_id, and target_actor_id when required."`
	Bonuses      []TurnCheckBonus      `json:"bonuses,omitempty" jsonschema_description:"Runtime bonuses and penalties. Positive values are favorable and negative values are unfavorable; both are added to the fixed d20 total."`
	Difficulty   string                `json:"difficulty" jsonschema:"enum=very_easy,enum=easy,enum=normal,enum=hard,enum=very_hard" jsonschema_description:"Use exactly one of very_easy/easy/normal/hard/very_hard. Use normal for ordinary difficulty, never medium or moderate."`
	Outcomes     TurnCheckOutcomes     `json:"outcomes" jsonschema_description:"Define result for all four tiers: critical_success, success, failure, and critical_failure. Optional state_changes are returned only for the selected outcome."`
}

type TurnCheckAdjudication struct {
	Reason           string          `json:"reason,omitempty" jsonschema_description:"Why this action requires a fixed check instead of direct adjudication."`
	Stakes           string          `json:"stakes,omitempty" jsonschema_description:"The explicit risk, cost, or irreversible consequence of this check."`
	DifficultyReason string          `json:"difficulty_reason,omitempty" jsonschema_description:"The basis for the selected difficulty."`
	RollModeReason   string          `json:"roll_mode_reason,omitempty" jsonschema_description:"The basis for advantage, disadvantage, or a normal roll."`
	StateRefs        []ActorStateRef `json:"state_refs,omitempty" jsonschema_description:"State fields directly used in adjudication. Each item uses actor_id and a field_id from the story's frozen schema."`
}

// ActorStateRef identifies one field without encoding it into a dotted path.
type ActorStateRef struct {
	ActorID string `json:"actor_id" jsonschema_description:"Actor ID."`
	FieldID string `json:"field_id" jsonschema_description:"State name/ID from the story's frozen schema."`
}

type TurnCheckRule struct {
	Template      string  `json:"template,omitempty" jsonschema:"enum=dice_check" jsonschema_description:"Optional rule template; when present it must be dice_check."`
	TemplateID    string  `json:"template_id,omitempty" jsonschema_description:"Matched TRPG check configuration ID for auditing."`
	Label         string  `json:"label,omitempty" jsonschema_description:"Matched TRPG check configuration label for auditing."`
	FailurePolicy string  `json:"failure_policy,omitempty" jsonschema:"enum=fail_forward,enum=success_at_cost,enum=blocked,enum=hard_failure" jsonschema_description:"Failure policy from the matched template for auditing."`
	RollMode      string  `json:"roll_mode,omitempty" jsonschema:"enum=normal,enum=advantage,enum=disadvantage" jsonschema_description:"Optional roll mode. normal rolls once; advantage keeps the higher roll; disadvantage keeps the lower roll."`
	Modifier      float64 `json:"modifier,omitempty" jsonschema_description:"Template difficulty modifier. Positive values are harder and negative values are easier; under fixed d20 this adjusts the target upward or downward."`
	BindingID     string  `json:"binding_id,omitempty" jsonschema_description:"Optional State Binding scenario ID. When set, the tool reads state and computes modifiers from the TRPG configuration."`
	ActorID       string  `json:"actor_id,omitempty" jsonschema_description:"Acting Actor ID for State Binding; required when binding_id is set."`
	TargetActorID string  `json:"target_actor_id,omitempty" jsonschema_description:"Target Actor ID for State Binding; required when the binding defines target_template_id."`
}

type TurnCheckBonus struct {
	Kind    string  `json:"kind,omitempty" jsonschema_description:"Modifier source type, such as attribute/state/equipment/environment/help/other."`
	ActorID string  `json:"actor_id,omitempty" jsonschema_description:"Actor ID supplying structured state; omit when there is no state source."`
	FieldID string  `json:"field_id,omitempty" jsonschema_description:"Field ID supplying structured state; omit when there is no state source."`
	Reason  string  `json:"reason" jsonschema_description:"Reason for the bonus or penalty, grounded in current state or known setting facts."`
	Value   float64 `json:"value" jsonschema_description:"Modifier value. Positive values increase the check total; negative values decrease it."`
}

type TurnCheckOutcomes struct {
	CriticalSuccess TurnCheckOutcome `json:"critical_success" jsonschema_description:"Critical-success consequence, selected on a natural 20 or when the total exceeds the target by at least 10."`
	Success         TurnCheckOutcome `json:"success" jsonschema_description:"Success consequence, selected when the d20 total reaches the target."`
	Failure         TurnCheckOutcome `json:"failure" jsonschema_description:"Failure consequence, selected when neither success nor critical failure applies."`
	CriticalFailure TurnCheckOutcome `json:"critical_failure" jsonschema_description:"Critical-failure consequence, selected on a natural 1 or when the total is at least 10 below the target."`
}

type TurnCheckOutcome struct {
	Result       string            `json:"result" jsonschema_description:"Final consequence that the narrative must follow when this tier is selected."`
	StateChanges []TurnStateChange `json:"state_changes,omitempty" jsonschema_description:"Optional structured state deltas caused directly by this check."`
}

type TurnStateChange struct {
	ActorID string  `json:"actor_id" jsonschema_description:"Actor ID to change."`
	FieldID string  `json:"field_id" jsonschema_description:"Number-state name/ID from the story's frozen schema."`
	Change  float64 `json:"change" jsonschema_description:"Numeric delta; negative decreases and positive increases."`
	Reason  string  `json:"reason,omitempty" jsonschema_description:"Why this outcome causes the state change."`
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
	ID                  string            `json:"id,omitempty"`
	Label               string            `json:"label,omitempty"`
	Kind                string            `json:"kind,omitempty"`
	Mode                string            `json:"mode,omitempty"`
	Dice                string            `json:"dice,omitempty"`
	Rolls               []int             `json:"rolls,omitempty"`
	RollTotal           float64           `json:"roll_total,omitempty"`
	Modifier            float64           `json:"modifier,omitempty"`
	Difficulty          float64           `json:"difficulty,omitempty"`
	Total               float64           `json:"total,omitempty"`
	Outcome             string            `json:"outcome"`
	Seed                int64             `json:"seed,omitempty"`
	Constraints         []string          `json:"constraints,omitempty"`
	Error               string            `json:"error,omitempty"`
	RollMode            string            `json:"roll_mode,omitempty"`
	KeptRoll            float64           `json:"kept_roll,omitempty"`
	BonusTotal          float64           `json:"bonus_total,omitempty"`
	BonusDetails        []TurnCheckBonus  `json:"bonus_details,omitempty"`
	BaseTarget          float64           `json:"base_target,omitempty"`
	Target              float64           `json:"target,omitempty"`
	RequestedDifficulty string            `json:"requested_difficulty,omitempty"`
	EffectiveDifficulty string            `json:"effective_difficulty,omitempty"`
	DifficultyShift     int               `json:"difficulty_shift,omitempty"`
	StoryRollModifier   int               `json:"story_roll_modifier,omitempty"`
	Result              string            `json:"result,omitempty"`
	StateChanges        []TurnStateChange `json:"state_changes,omitempty"`
}

type RuleResolutionToolOutput struct {
	ResolutionID        string            `json:"resolution_id"`
	Label               string            `json:"label,omitempty"`
	Dice                string            `json:"dice"`
	RollMode            string            `json:"roll_mode"`
	Rolls               []int             `json:"rolls"`
	KeptRoll            int               `json:"kept_roll"`
	BonusTotal          float64           `json:"bonus_total"`
	BonusDetails        []TurnCheckBonus  `json:"bonus_details,omitempty"`
	BaseTarget          float64           `json:"base_target"`
	Total               float64           `json:"total"`
	Difficulty          string            `json:"difficulty"`
	RequestedDifficulty string            `json:"requested_difficulty,omitempty"`
	DifficultyShift     int               `json:"difficulty_shift,omitempty"`
	Target              float64           `json:"target"`
	Outcome             string            `json:"outcome"`
	Result              string            `json:"result"`
	Cost                string            `json:"cost,omitempty"`
	Stakes              string            `json:"stakes,omitempty"`
	StateChanges        []TurnStateChange `json:"state_changes,omitempty"`
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
	normalized.RuleConstraints = normalizeStringListLimit(normalized.RuleConstraints, maxInteractiveListItems)
	return &normalized
}

func normalizeTerminalOutcomePointer(outcome *TerminalOutcome) *TerminalOutcome {
	if outcome == nil || !outcome.Terminal {
		return nil
	}
	normalized := *outcome
	normalized.Type = trimBytes(normalized.Type, 128)
	normalized.Reason = trimBytes(normalized.Reason, maxInteractiveTextBytes)
	normalized.FinalNarrativeSummary = trimBytes(normalized.FinalNarrativeSummary, maxInteractiveTextBytes)
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
	req.Action = trimBytes(req.Action, maxInteractiveTextBytes)
	req.Intent = trimBytes(req.Intent, maxInteractiveTextBytes)
	req.Challenge = trimBytes(req.Challenge, maxInteractiveTextBytes)
	req.Cost = trimBytes(req.Cost, maxInteractiveTextBytes)
	req.State = trimBytes(req.State, maxInteractiveTextBytes)
	req.Adjudication = normalizeTurnCheckAdjudication(req.Adjudication)
	req.Rule.Template = normalizeTurnCheckTemplate(req.Rule.Template)
	req.Rule.TemplateID = trimBytes(req.Rule.TemplateID, 128)
	req.Rule.Label = trimBytes(req.Rule.Label, 256)
	req.Rule.FailurePolicy = normalizeRuleCheckFailurePolicyOptional(req.Rule.FailurePolicy)
	req.Rule.RollMode = normalizeTurnCheckRollMode(req.Rule.RollMode)
	req.Rule.BindingID = normalizeInteractiveID(req.Rule.BindingID)
	req.Rule.ActorID = normalizeStatePanelActorID(req.Rule.ActorID)
	req.Rule.TargetActorID = normalizeStatePanelActorID(req.Rule.TargetActorID)
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
	if _, ok := turnCheckDifficultyTarget("1d20", req.Difficulty); !ok {
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

// ResolveTurnRulesWithStorySettings applies story-owned tuning after the Game
// Agent proposes a fictional difficulty and before the deterministic roll.
func ResolveTurnRulesWithStorySettings(storyID, branchID string, state map[string]any, director StoryDirector, settings StoryCheckSettings, req TurnCheckRequest) (RuleResolution, error) {
	return resolveTurnRulesWithSeedAndDirectorAndSettings(storyID, branchID, state, director, settings, req, 0)
}

func resolveTurnRulesWithSeedAndDirector(storyID, branchID string, state map[string]any, director StoryDirector, req TurnCheckRequest, seed int64) (RuleResolution, error) {
	return resolveTurnRulesWithSeedAndDirectorAndSettings(storyID, branchID, state, director, StoryCheckSettings{}, req, seed)
}

func resolveTurnRulesWithSeedAndDirectorAndSettings(storyID, branchID string, state map[string]any, director StoryDirector, settings StoryCheckSettings, req TurnCheckRequest, seed int64) (RuleResolution, error) {
	req = NormalizeTurnCheckRequest(req)
	if err := ValidateTurnCheckRequest(req); err != nil {
		return RuleResolution{}, err
	}
	settings = normalizeStoryCheckSettings(settings)
	bindingAudit, err := resolveRuleStateBinding(state, director, req)
	if err != nil {
		return RuleResolution{}, err
	}
	if seed == 0 {
		seed = newRuleSeed(storyID, branchID, req.Action, req.Challenge)
	}
	const dice = "1d20"
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
	requestedDifficulty := req.Difficulty
	effectiveDifficulty := storyCheckDifficulty(requestedDifficulty, settings.DifficultyShift)
	baseTarget, _ := turnCheckDifficultyTarget(dice, effectiveDifficulty)
	if settings.RollModifier != 0 {
		bonusTotal += float64(settings.RollModifier)
		bonusDetails = append(bonusDetails, TurnCheckBonus{
			Kind: "story", Reason: "Story-wide roll modifier.", Value: float64(settings.RollModifier),
		})
	}
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
		ID:                  "check_1",
		Label:               firstNonEmptyString(req.Rule.Label, req.Challenge, req.Action),
		Kind:                "dice_check",
		Mode:                turnCheckMode(dice),
		Dice:                dice,
		Rolls:               rolls,
		RollTotal:           float64(keptRoll),
		Modifier:            req.Rule.Modifier + resistanceTotal,
		Difficulty:          target,
		Total:               total,
		Outcome:             outcomeName,
		Seed:                seed,
		Constraints:         []string{constraint},
		RollMode:            req.Rule.RollMode,
		KeptRoll:            float64(keptRoll),
		BonusTotal:          bonusTotal,
		BonusDetails:        bonusDetails,
		BaseTarget:          baseTarget,
		Target:              target,
		RequestedDifficulty: requestedDifficulty,
		EffectiveDifficulty: effectiveDifficulty,
		DifficultyShift:     settings.DifficultyShift,
		StoryRollModifier:   settings.RollModifier,
		Result:              outcome.Result,
		StateChanges:        resultStateChanges,
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
		ResolutionID:        resolution.ID,
		Label:               resolution.Result.Label,
		Dice:                firstNonEmptyString(resolution.Result.Dice, "1d20"),
		RollMode:            firstNonEmptyString(resolution.Result.RollMode, "normal"),
		Rolls:               append([]int(nil), resolution.Result.Rolls...),
		KeptRoll:            keptRoll,
		BonusTotal:          resolution.Result.BonusTotal,
		BonusDetails:        append([]TurnCheckBonus(nil), resolution.Result.BonusDetails...),
		BaseTarget:          resolution.Result.BaseTarget,
		Total:               resolution.Result.Total,
		Difficulty:          firstNonEmptyString(resolution.Result.EffectiveDifficulty, resolution.Request.Difficulty),
		RequestedDifficulty: resolution.Result.RequestedDifficulty,
		DifficultyShift:     resolution.Result.DifficultyShift,
		Target:              resolution.Result.Target,
		Outcome:             resolution.Result.Outcome,
		Result:              resolution.Result.Result,
		Cost:                resolution.Request.Cost,
		Stakes:              resolution.Request.Adjudication.Stakes,
		StateChanges:        append([]TurnStateChange(nil), resolution.Result.StateChanges...),
	}
}

func normalizeTurnCheckOutcome(outcome TurnCheckOutcome) TurnCheckOutcome {
	outcome.Result = trimBytes(outcome.Result, maxInteractiveTextBytes)
	outcome.StateChanges = normalizeTurnStateChanges(outcome.StateChanges)
	return outcome
}

func normalizeTurnCheckAdjudication(value TurnCheckAdjudication) TurnCheckAdjudication {
	value.Reason = trimBytes(value.Reason, maxInteractiveTextBytes)
	value.Stakes = trimBytes(value.Stakes, maxInteractiveTextBytes)
	value.DifficultyReason = trimBytes(value.DifficultyReason, maxInteractiveTextBytes)
	value.RollModeReason = trimBytes(value.RollModeReason, maxInteractiveTextBytes)
	value.StateRefs = normalizeActorStateRefs(value.StateRefs)
	return value
}

func normalizeTurnCheckBonuses(values []TurnCheckBonus) []TurnCheckBonus {
	if len(values) > maxInteractiveListItems {
		values = values[:maxInteractiveListItems]
	}
	out := make([]TurnCheckBonus, 0, len(values))
	for _, value := range values {
		value.Kind = normalizeTurnCheckEnumToken(value.Kind)
		value.ActorID = normalizeStatePanelActorID(value.ActorID)
		value.FieldID = normalizeActorStateFieldName(value.FieldID)
		value.Reason = trimBytes(value.Reason, 512)
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeTurnStateChanges(values []TurnStateChange) []TurnStateChange {
	if len(values) > maxInteractiveListItems {
		values = values[:maxInteractiveListItems]
	}
	out := make([]TurnStateChange, 0, len(values))
	for _, value := range values {
		value.ActorID = normalizeStatePanelActorID(value.ActorID)
		value.FieldID = normalizeActorStateFieldName(value.FieldID)
		value.Reason = trimBytes(value.Reason, 512)
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeActorStateRefs(values []ActorStateRef) []ActorStateRef {
	if len(values) > maxInteractiveListItems {
		values = values[:maxInteractiveListItems]
	}
	out := make([]ActorStateRef, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value.ActorID = normalizeStatePanelActorID(value.ActorID)
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
	case "", "normal":
		return "normal"
	case "very_easy", "easy", "hard", "very_hard":
		return normalizeTurnCheckEnumToken(value)
	default:
		return normalizeTurnCheckEnumToken(value)
	}
}

func normalizeTurnCheckTemplate(value string) string {
	switch normalizeTurnCheckEnumToken(value) {
	case "", "dice_check":
		return "dice_check"
	default:
		return normalizeTurnCheckEnumToken(value)
	}
}

func normalizeTurnCheckDice(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "1d20":
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
	check.Label = strings.TrimSpace(firstNonEmptyString(check.Label, check.ID))
	check.Dice = normalizeTurnCheckDice(check.Dice)
	check.FailurePolicy = normalizeRuleCheckFailurePolicy(check.FailurePolicy)
	check.DifficultyGuidance = strings.TrimSpace(check.DifficultyGuidance)
	check.StateEffectGuidance = strings.TrimSpace(check.StateEffectGuidance)
	check.Trigger = strings.TrimSpace(check.Trigger)
	check.MustCheckExamples = normalizeStringList(check.MustCheckExamples)
	check.SkipCheckExamples = normalizeStringList(check.SkipCheckExamples)
	check.SuccessHint = strings.TrimSpace(check.SuccessHint)
	check.FailureHint = strings.TrimSpace(check.FailureHint)
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
	case "", "fail_forward":
		return "fail_forward"
	case "success_at_cost":
		return "success_at_cost"
	case "blocked":
		return "blocked"
	case "hard_failure":
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
	if len(values) > maxInteractiveListItems {
		values = values[:maxInteractiveListItems]
	}
	out := make([]DirectorEvent, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value.ID = trimBytes(value.ID, 128)
		value.Name = trimBytes(value.Name, 256)
		value.Category = trimBytes(value.Category, 128)
		value.Status = trimBytes(value.Status, 128)
		value.Summary = trimBytes(value.Summary, maxInteractiveTextBytes)
		value.PublicSummary = trimBytes(value.PublicSummary, maxInteractiveTextBytes)
		value.HiddenTruth = trimBytes(value.HiddenTruth, maxInteractiveTextBytes)
		value.Template = trimBytes(value.Template, maxInteractiveTextBytes)
		value.NormalizedTrigger = trimBytes(value.NormalizedTrigger, maxInteractiveTextBytes)
		value.Intensity = trimBytes(value.Intensity, 128)
		value.RequiredForeshadowing = normalizeStringListLimit(value.RequiredForeshadowing, maxInteractiveListItems)
		value.PayoffTarget = trimBytes(value.PayoffTarget, maxInteractiveTextBytes)
		value.Reward = trimBytes(value.Reward, maxInteractiveTextBytes)
		value.Cost = trimBytes(value.Cost, maxInteractiveTextBytes)
		value.FailureLevel = trimBytes(value.FailureLevel, 128)
		value.CompatibleGenres = normalizeStringListLimit(value.CompatibleGenres, maxInteractiveListItems)
		value.IncompatibleStateFlags = normalizeStringListLimit(value.IncompatibleStateFlags, maxInteractiveListItems)
		value.DirectorInstructionNote = trimBytes(value.DirectorInstructionNote, maxInteractiveTextBytes)
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

func normalizeStringListLimit(values []string, limit int) []string {
	if limit <= 0 {
		limit = maxInteractiveListItems
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

// normalizeStringList canonicalizes persisted configuration collections
// without imposing runtime projection limits or truncating user content.
func normalizeStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
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
