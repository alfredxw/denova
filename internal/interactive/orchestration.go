package interactive

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
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
	Weight                  float64  `json:"weight,omitempty"`
	CooldownTurns           int      `json:"cooldown_turns,omitempty"`
	Intensity               string   `json:"intensity,omitempty"`
	RequiredForeshadowing   []string `json:"required_foreshadowing,omitempty"`
	PayoffTarget            string   `json:"payoff_target,omitempty"`
	Reward                  string   `json:"reward,omitempty"`
	Cost                    string   `json:"cost,omitempty"`
	FailureLevel            string   `json:"failure_level,omitempty"`
	CompatibleGenres        []string `json:"compatible_genres,omitempty"`
	IncompatibleStateFlags  []string `json:"incompatible_state_flags,omitempty"`
	UserConfigured          bool     `json:"user_configured,omitempty"`
	LastTriggeredTurnID     string   `json:"last_triggered_turn_id,omitempty"`
	NextEligibleAfterTurns  int      `json:"next_eligible_after_turns,omitempty"`
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
	ID                string    `json:"id,omitempty"`
	Label             string    `json:"label,omitempty"`
	Kind              string    `json:"kind,omitempty"`
	Mode              string    `json:"mode,omitempty"`
	AttributePath     string    `json:"attribute_path,omitempty"`
	Expression        string    `json:"expression,omitempty"`
	Dice              string    `json:"dice,omitempty"`
	Modifier          float64   `json:"modifier,omitempty"`
	Difficulty        float64   `json:"difficulty,omitempty"`
	ResourceCostPath  string    `json:"resource_cost_path,omitempty"`
	ResourceCost      float64   `json:"resource_cost,omitempty"`
	SuccessStateOps   []StateOp `json:"success_state_ops,omitempty"`
	FailureStateOps   []StateOp `json:"failure_state_ops,omitempty"`
	TerminalOnFailure bool      `json:"terminal_on_failure,omitempty"`
	TerminalType      string    `json:"terminal_type,omitempty"`
	TerminalReason    string    `json:"terminal_reason,omitempty"`
	Seed              int64     `json:"seed,omitempty"`
}

type TurnCheckRequest struct {
	Action     string            `json:"action"`
	Intent     string            `json:"intent"`
	Challenge  string            `json:"challenge"`
	Cost       string            `json:"cost"`
	State      string            `json:"state"`
	Rule       TurnCheckRule     `json:"rule,omitempty"`
	Bonuses    []TurnCheckBonus  `json:"bonuses,omitempty"`
	Difficulty string            `json:"difficulty"`
	Outcomes   TurnCheckOutcomes `json:"outcomes"`
}

type TurnCheckRule struct {
	Template string `json:"template,omitempty"`
	Dice     string `json:"dice,omitempty"`
	RollMode string `json:"roll_mode,omitempty"`
}

type TurnCheckBonus struct {
	Reason string  `json:"reason"`
	Value  float64 `json:"value"`
}

type TurnCheckOutcomes struct {
	CriticalSuccess TurnCheckOutcome `json:"critical_success"`
	Success         TurnCheckOutcome `json:"success"`
	Failure         TurnCheckOutcome `json:"failure"`
	CriticalFailure TurnCheckOutcome `json:"critical_failure"`
}

type TurnCheckOutcome struct {
	Result       string            `json:"result"`
	StateChanges []TurnStateChange `json:"state_changes,omitempty"`
}

type TurnStateChange struct {
	Path   string  `json:"path"`
	Change float64 `json:"change"`
}

type RuleResolution struct {
	ID                string             `json:"id,omitempty"`
	Request           TurnCheckRequest   `json:"request"`
	Result            RuleResult         `json:"result"`
	TerminalCandidate *TerminalCandidate `json:"terminal_candidate,omitempty"`
	RuleConstraints   []string           `json:"rule_constraints,omitempty"`
	CreatedAt         string             `json:"created_at,omitempty"`
	Seed              int64              `json:"seed,omitempty"`
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
	Target          float64           `json:"target,omitempty"`
	Result          string            `json:"result,omitempty"`
	StateChanges    []TurnStateChange `json:"state_changes,omitempty"`
}

type RuleResolutionToolOutput struct {
	ResolutionID string            `json:"resolution_id"`
	Dice         string            `json:"dice"`
	RollMode     string            `json:"roll_mode"`
	Rolls        []int             `json:"rolls"`
	KeptRoll     int               `json:"kept_roll"`
	BonusTotal   float64           `json:"bonus_total"`
	Total        float64           `json:"total"`
	Difficulty   string            `json:"difficulty"`
	Target       float64           `json:"target"`
	Outcome      string            `json:"outcome"`
	Result       string            `json:"result"`
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

func ValidateTurnBrief(brief TurnBrief) error {
	if strings.TrimSpace(brief.UserAction) == "" {
		return fmt.Errorf("TurnBrief 缺少 user_action")
	}
	if strings.TrimSpace(brief.Intent) == "" {
		return fmt.Errorf("TurnBrief 缺少 intent")
	}
	if strings.TrimSpace(brief.TurnGoal) == "" {
		return fmt.Errorf("TurnBrief 缺少 turn_goal")
	}
	for _, check := range brief.RuleChecks {
		if err := validateRuleCheck(check); err != nil {
			return err
		}
	}
	return nil
}

func NormalizeTurnCheckRequest(req TurnCheckRequest) TurnCheckRequest {
	req.Action = trimBytes(req.Action, maxTurnBriefTextBytes)
	req.Intent = trimBytes(req.Intent, maxTurnBriefTextBytes)
	req.Challenge = trimBytes(req.Challenge, maxTurnBriefTextBytes)
	req.Cost = trimBytes(req.Cost, maxTurnBriefTextBytes)
	req.State = trimBytes(req.State, maxTurnBriefTextBytes)
	req.Rule.Template = trimBytes(firstNonEmptyString(req.Rule.Template, "dice_check"), 128)
	req.Rule.Dice = strings.TrimSpace(firstNonEmptyString(req.Rule.Dice, "1d20"))
	req.Rule.RollMode = normalizeTurnCheckRollMode(req.Rule.RollMode)
	req.Difficulty = normalizeTurnCheckDifficulty(req.Difficulty)
	if len(req.Bonuses) > maxTurnBriefListItems {
		req.Bonuses = req.Bonuses[:maxTurnBriefListItems]
	}
	for i := range req.Bonuses {
		req.Bonuses[i].Reason = trimBytes(req.Bonuses[i].Reason, 512)
	}
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
	if req.Rule.Template != "" && req.Rule.Template != "dice_check" && req.Rule.Template != "d20_check" {
		return fmt.Errorf("prepare_interactive_turn rule.template 仅支持 dice_check")
	}
	if req.Rule.Dice != "" && req.Rule.Dice != "1d20" {
		return fmt.Errorf("prepare_interactive_turn rule.dice 仅支持 1d20")
	}
	if _, ok := turnCheckDifficultyTargets[normalizeTurnCheckDifficulty(req.Difficulty)]; !ok {
		return fmt.Errorf("prepare_interactive_turn difficulty 无效: %s", req.Difficulty)
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
			if !validStatePathSyntax(change.Path) {
				return fmt.Errorf("prepare_interactive_turn outcomes.%s.state_changes path 无效: %s", name, change.Path)
			}
		}
	}
	return nil
}

func ResolveTurnRules(storyID, branchID string, state map[string]any, req TurnCheckRequest) (RuleResolution, error) {
	return resolveTurnRulesWithSeed(storyID, branchID, state, req, 0)
}

func resolveTurnRulesWithSeed(storyID, branchID string, state map[string]any, req TurnCheckRequest, seed int64) (RuleResolution, error) {
	_ = state
	req = NormalizeTurnCheckRequest(req)
	if err := ValidateTurnCheckRequest(req); err != nil {
		return RuleResolution{}, err
	}
	if seed == 0 {
		seed = newRuleSeed(storyID, branchID, req.Action, req.Challenge)
	}
	rolls, keptRoll, err := rollTurnCheck(seed, req.Rule.RollMode)
	if err != nil {
		return RuleResolution{}, err
	}
	bonusTotal := turnCheckBonusTotal(req.Bonuses)
	target := turnCheckDifficultyTargets[req.Difficulty]
	total := float64(keptRoll) + bonusTotal
	outcomeName := resolveTurnCheckOutcome(keptRoll, total, target)
	outcome := req.outcomeByName(outcomeName)
	constraint := fmt.Sprintf("%s：%s，总值 %.0f / 难度 %.0f。", firstNonEmptyString(req.Challenge, req.Action), turnCheckOutcomeText(outcomeName), total, target)
	result := RuleResult{
		ID:           "check_1",
		Label:        firstNonEmptyString(req.Challenge, req.Action),
		Kind:         "dice_check",
		Mode:         "d20_dc",
		Dice:         "1d20",
		Rolls:        rolls,
		RollTotal:    float64(keptRoll),
		Modifier:     bonusTotal,
		Difficulty:   target,
		Total:        total,
		Outcome:      outcomeName,
		Seed:         seed,
		Constraints:  []string{constraint},
		RollMode:     req.Rule.RollMode,
		KeptRoll:     float64(keptRoll),
		BonusTotal:   bonusTotal,
		Target:       target,
		Result:       outcome.Result,
		StateChanges: outcome.StateChanges,
	}
	resolution := RuleResolution{
		ID:              newID("rr"),
		Request:         req,
		Result:          result,
		RuleConstraints: []string{constraint},
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		Seed:            seed,
	}
	return resolution, nil
}

var turnCheckDifficultyTargets = map[string]float64{
	"very_easy": 5,
	"easy":      10,
	"normal":    15,
	"hard":      20,
	"very_hard": 25,
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
		Dice:         firstNonEmptyString(resolution.Result.Dice, "1d20"),
		RollMode:     firstNonEmptyString(resolution.Result.RollMode, "normal"),
		Rolls:        append([]int(nil), resolution.Result.Rolls...),
		KeptRoll:     keptRoll,
		BonusTotal:   resolution.Result.BonusTotal,
		Total:        resolution.Result.Total,
		Difficulty:   resolution.Request.Difficulty,
		Target:       resolution.Result.Target,
		Outcome:      resolution.Result.Outcome,
		Result:       resolution.Result.Result,
		StateChanges: append([]TurnStateChange(nil), resolution.Result.StateChanges...),
	}
}

func normalizeTurnCheckOutcome(outcome TurnCheckOutcome) TurnCheckOutcome {
	outcome.Result = trimBytes(outcome.Result, maxTurnBriefTextBytes)
	if len(outcome.StateChanges) > maxTurnBriefListItems {
		outcome.StateChanges = outcome.StateChanges[:maxTurnBriefListItems]
	}
	for i := range outcome.StateChanges {
		outcome.StateChanges[i].Path = strings.TrimSpace(outcome.StateChanges[i].Path)
	}
	return outcome
}

func normalizeTurnCheckRollMode(value string) string {
	switch strings.TrimSpace(value) {
	case "", "normal":
		return "normal"
	case "advantage", "disadvantage":
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(value)
	}
}

func normalizeTurnCheckDifficulty(value string) string {
	switch strings.TrimSpace(value) {
	case "", "normal":
		return "normal"
	case "very_easy", "easy", "hard", "very_hard":
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(value)
	}
}

func rollTurnCheck(seed int64, rollMode string) ([]int, int, error) {
	count := 1
	switch normalizeTurnCheckRollMode(rollMode) {
	case "normal":
		count = 1
	case "advantage", "disadvantage":
		count = 2
	default:
		return nil, 0, fmt.Errorf("prepare_interactive_turn rule.roll_mode 无效: %s", rollMode)
	}
	rolls, _, err := rollDice(seed, fmt.Sprintf("%dd20", count))
	if err != nil {
		return nil, 0, err
	}
	kept := rolls[0]
	if rollMode == "advantage" {
		for _, roll := range rolls[1:] {
			if roll > kept {
				kept = roll
			}
		}
	}
	if rollMode == "disadvantage" {
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

func resolveTurnCheckOutcome(keptRoll int, total, target float64) string {
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

func resolveRuleCheck(storyID, branchID string, state map[string]any, check RuleCheck) (RuleResult, []StateOp, *TerminalCandidate) {
	seed := check.Seed
	if seed == 0 {
		seed = newRuleSeed(storyID, branchID, check.ID)
	}
	result := RuleResult{
		ID:            check.ID,
		Label:         check.Label,
		Kind:          firstNonEmptyString(check.Kind, "check"),
		Mode:          normalizeRuleCheckMode(check.Mode),
		AttributePath: check.AttributePath,
		Expression:    check.Expression,
		Dice:          check.Dice,
		Modifier:      check.Modifier,
		Difficulty:    check.Difficulty,
		Seed:          seed,
		Outcome:       "success",
	}
	if check.AttributePath != "" {
		result.AttributeValue = numberFromAny(getPath(state, check.AttributePath))
	}
	if check.Expression != "" {
		value, err := EvalRuleExpression(check.Expression, state)
		if err != nil {
			result.Outcome = "error"
			result.Error = err.Error()
			result.Constraints = []string{fmt.Sprintf("%s 表达式无法执行：%s", firstNonEmptyString(check.Label, check.ID), err.Error())}
			return result, nil, nil
		}
		result.ExpressionValue = value
	}
	if check.Dice != "" {
		rolls, total, err := rollDice(seed, check.Dice)
		if err != nil {
			result.Outcome = "error"
			result.Error = err.Error()
			result.Constraints = []string{fmt.Sprintf("%s 检定无法执行：%s", firstNonEmptyString(check.Label, check.ID), err.Error())}
			return result, nil, nil
		}
		result.Rolls = rolls
		result.RollTotal = total
	}
	if result.Mode == "d100_under" {
		target := check.Difficulty
		if target <= 0 {
			target = result.AttributeValue + result.ExpressionValue + result.Modifier
		}
		result.Difficulty = target
		result.Total = result.RollTotal
		if result.RollTotal <= target {
			result.Outcome = d100UnderSuccessOutcome(result.RollTotal, target)
			result.Constraints = []string{fmt.Sprintf("%s 成功，d100 掷骰 %.0f / 目标 %.0f。", firstNonEmptyString(check.Label, check.ID), result.RollTotal, target)}
			return result, appendResourceCostOp(check, append([]StateOp(nil), check.SuccessStateOps...)), nil
		}
		result.Outcome = d100UnderFailureOutcome(result.RollTotal, target)
		result.Constraints = []string{fmt.Sprintf("%s 失败，d100 掷骰 %.0f / 目标 %.0f。", firstNonEmptyString(check.Label, check.ID), result.RollTotal, target)}
		ops := appendResourceCostOp(check, append([]StateOp(nil), check.FailureStateOps...))
		if check.TerminalOnFailure {
			return result, ops, &TerminalCandidate{
				Type:    firstNonEmptyString(check.TerminalType, "bad_end"),
				Reason:  firstNonEmptyString(check.TerminalReason, result.Constraints[0]),
				CheckID: check.ID,
			}
		}
		return result, ops, nil
	}
	result.Total = result.AttributeValue + result.ExpressionValue + result.RollTotal + result.Modifier
	success := true
	if check.Difficulty > 0 {
		success = result.Total >= check.Difficulty
	}
	if success {
		result.Outcome = ruleSuccessOutcome(result.Total, check.Difficulty)
		result.Constraints = []string{fmt.Sprintf("%s 成功，总值 %.0f / 难度 %.0f。", firstNonEmptyString(check.Label, check.ID), result.Total, check.Difficulty)}
		return result, appendResourceCostOp(check, append([]StateOp(nil), check.SuccessStateOps...)), nil
	}
	result.Outcome = ruleFailureOutcome(result.Total, check.Difficulty)
	result.Constraints = []string{fmt.Sprintf("%s 失败，总值 %.0f / 难度 %.0f。", firstNonEmptyString(check.Label, check.ID), result.Total, check.Difficulty)}
	ops := appendResourceCostOp(check, append([]StateOp(nil), check.FailureStateOps...))
	if check.TerminalOnFailure {
		return result, ops, &TerminalCandidate{
			Type:    firstNonEmptyString(check.TerminalType, "bad_end"),
			Reason:  firstNonEmptyString(check.TerminalReason, result.Constraints[0]),
			CheckID: check.ID,
		}
	}
	return result, ops, nil
}

func normalizeRuleCheck(check RuleCheck, index int) RuleCheck {
	check.ID = strings.TrimSpace(check.ID)
	if check.ID == "" {
		check.ID = fmt.Sprintf("check_%d", index+1)
	}
	check.Label = trimBytes(firstNonEmptyString(check.Label, check.ID), 256)
	check.Kind = trimBytes(firstNonEmptyString(check.Kind, "check"), 128)
	check.Mode = normalizeRuleCheckMode(check.Mode)
	check.AttributePath = strings.TrimSpace(check.AttributePath)
	check.Expression = trimBytes(check.Expression, 1024)
	check.Dice = strings.TrimSpace(check.Dice)
	check.ResourceCostPath = strings.TrimSpace(check.ResourceCostPath)
	check.TerminalType = trimBytes(check.TerminalType, 128)
	check.TerminalReason = trimBytes(check.TerminalReason, maxTurnBriefTextBytes)
	check.SuccessStateOps = normalizeStateOpsForRule(check.SuccessStateOps)
	check.FailureStateOps = normalizeStateOpsForRule(check.FailureStateOps)
	return check
}

func validateRuleCheck(check RuleCheck) error {
	if check.AttributePath != "" && !validStatePathSyntax(check.AttributePath) {
		return fmt.Errorf("规则检定 attribute_path 无效: %s", check.AttributePath)
	}
	if check.ResourceCostPath != "" && !validStatePathSyntax(check.ResourceCostPath) {
		return fmt.Errorf("规则检定 resource_cost_path 无效: %s", check.ResourceCostPath)
	}
	for _, op := range append(append([]StateOp(nil), check.SuccessStateOps...), check.FailureStateOps...) {
		if err := validateStateOp(op); err != nil {
			return err
		}
	}
	if check.Dice != "" {
		if _, _, err := parseDice(check.Dice); err != nil {
			return err
		}
	}
	switch normalizeRuleCheckMode(check.Mode) {
	case "", "default", "d20_dc":
	case "d100_under":
		if check.Dice == "" {
			return fmt.Errorf("d100_under 检定必须提供骰子表达式，通常为 1d100")
		}
	default:
		return fmt.Errorf("规则检定 mode 无效: %s", check.Mode)
	}
	return nil
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

func appendResourceCostOp(check RuleCheck, ops []StateOp) []StateOp {
	if check.ResourceCostPath == "" || check.ResourceCost == 0 {
		return ops
	}
	return append(ops, StateOp{Op: "inc", Path: check.ResourceCostPath, Value: -math.Abs(check.ResourceCost)})
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

func ruleSuccessOutcome(total, difficulty float64) string {
	if difficulty > 0 && total >= difficulty+10 {
		return "critical_success"
	}
	return "success"
}

func ruleFailureOutcome(total, difficulty float64) string {
	if difficulty > 0 && total <= difficulty-10 {
		return "critical_failure"
	}
	return "failure"
}

func normalizeRuleCheckMode(value string) string {
	switch strings.TrimSpace(value) {
	case "", "default":
		return "default"
	case "d20_dc", "d100_under":
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(value)
	}
}

func d100UnderSuccessOutcome(roll, target float64) string {
	if target > 0 && roll <= maxFloat(1, target/5) {
		return "critical_success"
	}
	return "success"
}

func d100UnderFailureOutcome(roll, target float64) string {
	if roll >= 96 || (target > 0 && roll >= target+50) {
		return "critical_failure"
	}
	return "failure"
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
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
		value.LastTriggeredTurnID = trimBytes(value.LastTriggeredTurnID, 128)
		value.DirectorInstructionNote = trimBytes(value.DirectorInstructionNote, maxTurnBriefTextBytes)
		if value.Weight < 0 {
			value.Weight = 0
		}
		if value.CooldownTurns < 0 {
			value.CooldownTurns = 0
		}
		if value.NextEligibleAfterTurns < 0 {
			value.NextEligibleAfterTurns = 0
		}
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
		op.Path = strings.TrimSpace(op.Path)
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
