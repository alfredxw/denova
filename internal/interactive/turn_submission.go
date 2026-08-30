package interactive

import (
	"bytes"
	interactivestate "denova/internal/interactive/state"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	TurnSubmissionModuleStateChanges = "state_changes"
	TurnSubmissionModuleChoices      = "choices"
	TurnSubmissionModulePlanUpdate   = "plan_update"

	TurnSubmissionModuleAccepted = "accepted"
	TurnSubmissionModuleRejected = "rejected"
	TurnSubmissionModuleMissing  = "missing"

	TurnSubmissionDiagnosticInvalidJSON            = "invalid_json"
	TurnSubmissionDiagnosticInvalidTopLevel        = "invalid_top_level"
	TurnSubmissionDiagnosticInvalidModule          = "invalid_module"
	TurnSubmissionDiagnosticChoiceCountMismatch    = "choice_count_mismatch"
	TurnSubmissionDiagnosticDuplicateChoice        = "duplicate_choice"
	TurnSubmissionDiagnosticEmptyChoice            = "empty_choice"
	TurnSubmissionDiagnosticStoryContextRequired   = "story_context_required"
	TurnSubmissionDiagnosticInitialStateIncomplete = "initial_state_incomplete"

	turnSubmissionSeverityError = "error"

	maxTurnSubmissionDiagnostics       = 8
	maxTurnSubmissionDiagnosticMessage = 1024
	maxTurnSubmissionAllowedFields     = 16
	maxTurnSubmissionArgumentsBytes    = 256 * 1024
	maxTurnSubmissionChoiceBytes       = 512
)

// TurnSubmissionDiagnostic is bounded, English-only, and points to the exact
// independently retryable module and operation.
type TurnSubmissionDiagnostic struct {
	Module    string `json:"module"`
	Index     *int   `json:"index,omitempty"`
	Code      string `json:"code"`
	Severity  string `json:"severity"`
	Path      string `json:"path,omitempty"`
	Expected  string `json:"expected,omitempty"`
	Actual    string `json:"actual,omitempty"`
	Retryable bool   `json:"retryable"`
	Message   string `json:"message"`
}

type TurnSubmissionModuleStatus struct {
	StateChanges string `json:"state_changes"`
	Choices      string `json:"choices"`
	PlanUpdate   string `json:"plan_update"`
}

// TurnSubmissionReceipt reports independent module acceptance. Ready becomes
// true only after all required modules have been accepted, possibly across calls.
type TurnSubmissionReceipt struct {
	Ready                bool                       `json:"ready"`
	ModuleStatus         TurnSubmissionModuleStatus `json:"module_status"`
	Diagnostics          []TurnSubmissionDiagnostic `json:"diagnostics,omitempty"`
	RetryModules         []string                   `json:"retry_modules,omitempty"`
	MissingModules       []string                   `json:"missing_modules,omitempty"`
	DiagnosticsTruncated bool                       `json:"diagnostics_truncated,omitempty"`
}

// TurnSubmissionInput holds independently retryable fields decoded from one
// submit_interactive_turn call. Either field may be absent on a targeted retry.
type TurnSubmissionInput struct {
	StateUpdates *[]interactivestate.Update
	Choices      *[]string
	PlanUpdate   *string
	Diagnostics  []TurnSubmissionDiagnostic
}

// TurnSubmissionContext contains all story-scoped validation inputs. IDs and
// current state are backend-bound and never supplied by the model.
type TurnSubmissionContext struct {
	ActorState                  StoryDirectorActorStateSystem
	CurrentState                map[string]any
	ChoiceCount                 int
	RuleResolution              *RuleResolution
	RuleStateConsumptionMode    string
	RequireCompleteInitialState bool
	PlanningMode                string
	CurrentPlan                 *BranchPlan
}

// PreparedTurnSubmission holds accepted modules while failed modules are
// retried. It is immutable after construction.
type PreparedTurnSubmission struct {
	result               TurnResult
	stateUpdatesAccepted bool
	choicesAccepted      bool
	planUpdateAccepted   bool
}

func (s *PreparedTurnSubmission) TurnResult() TurnResult {
	if s == nil {
		return TurnResult{}
	}
	return TurnResult{
		StateUpdates: append([]interactivestate.Update(nil), s.result.StateUpdates...),
		Choices:      append([]string(nil), s.result.Choices...),
		PlanUpdate:   cloneStringPointer(s.result.PlanUpdate),
	}
}

func (s *PreparedTurnSubmission) Ready() bool {
	return s != nil && s.stateUpdatesAccepted && s.choicesAccepted && s.planUpdateAccepted
}

// PrepareTurnSubmission accepts valid modules independently and retains any
// module accepted by an earlier call. state_changes remains atomic internally.
func PrepareTurnSubmission(validation TurnSubmissionContext, current *PreparedTurnSubmission, input TurnSubmissionInput) (*PreparedTurnSubmission, TurnSubmissionReceipt) {
	prepared := clonePreparedTurnSubmission(current)
	planningEnabled := normalizeStoryPlanningMode(validation.PlanningMode) == StoryPlanningModeEnabled
	diagnostics := make([]TurnSubmissionDiagnostic, 0, len(input.Diagnostics))
	rejected := map[string]bool{}
	for _, diagnostic := range input.Diagnostics {
		if diagnostic.Module == TurnSubmissionModulePlanUpdate && !planningEnabled {
			continue
		}
		if (diagnostic.Module == TurnSubmissionModuleStateChanges && prepared.stateUpdatesAccepted) ||
			(diagnostic.Module == TurnSubmissionModuleChoices && prepared.choicesAccepted) ||
			(diagnostic.Module == TurnSubmissionModulePlanUpdate && prepared.planUpdateAccepted) {
			continue
		}
		diagnostics = append(diagnostics, diagnostic)
		if diagnostic.Module == TurnSubmissionModuleStateChanges || diagnostic.Module == TurnSubmissionModuleChoices || diagnostic.Module == TurnSubmissionModulePlanUpdate {
			rejected[diagnostic.Module] = true
		}
	}
	if input.StateUpdates != nil && !prepared.stateUpdatesAccepted && !rejected[TurnSubmissionModuleStateChanges] {
		updates := normalizeTurnSubmissionStateUpdateTargets(validation.ActorState, validation.CurrentState, *input.StateUpdates)
		compileOptions := TurnStateUpdateCompileOptions{
			RuleResolution:           validation.RuleResolution,
			RuleStateConsumptionMode: validation.RuleStateConsumptionMode,
		}
		compiled, err := CompileTurnStateUpdates(validation.ActorState, validation.CurrentState, updates, compileOptions)
		if err != nil {
			validationErrors := flattenStateUpdateValidationErrors(err)
			collected := collectTurnStateUpdateValidationErrors(validation.ActorState, validation.CurrentState, updates, compileOptions)
			validationErrors = mergeStateUpdateValidationErrors(validationErrors, collected)
			if len(validationErrors) > 0 {
				err = &StateUpdateValidationErrors{Items: validationErrors}
			}
			diagnostics = append(diagnostics, diagnosticsForStateUpdateError(err)...)
			rejected[TurnSubmissionModuleStateChanges] = true
		} else {
			moduleDiagnostics := make([]TurnSubmissionDiagnostic, 0, 2)
			if diagnostic := storyContextSubmissionDiagnostic(validation.ActorState, validation.CurrentState, updates); diagnostic != nil {
				moduleDiagnostics = append(moduleDiagnostics, *diagnostic)
			}
			if validation.RequireCompleteInitialState {
				if diagnostic := openingInitialStateSubmissionDiagnostic(validation.ActorState, validation.CurrentState, compiled); diagnostic != nil {
					moduleDiagnostics = append(moduleDiagnostics, *diagnostic)
				}
			}
			if len(moduleDiagnostics) > 0 {
				diagnostics = append(diagnostics, moduleDiagnostics...)
				rejected[TurnSubmissionModuleStateChanges] = true
			} else {
				prepared.result.StateUpdates = compiled.Updates
				prepared.stateUpdatesAccepted = true
			}
		}
	}

	if input.Choices != nil && !prepared.choicesAccepted && !rejected[TurnSubmissionModuleChoices] {
		choices, diagnostic := validateSubmittedChoices(*input.Choices, validation.ChoiceCount, validation.RuleResolution != nil && validation.RuleResolution.TerminalCandidate != nil)
		if diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			rejected[TurnSubmissionModuleChoices] = true
		} else {
			prepared.result.Choices = choices
			prepared.choicesAccepted = true
		}
	}

	if !planningEnabled {
		// The story-level switch is authoritative. A stale or mistaken model
		// field must not make an otherwise valid turn retry, and must never be
		// retained for a story whose planning feature is disabled.
		prepared.result.PlanUpdate = nil
		prepared.planUpdateAccepted = true
	} else if input.PlanUpdate != nil && !prepared.planUpdateAccepted && !rejected[TurnSubmissionModulePlanUpdate] {
		value := normalizeBranchPlanMarkdown(*input.PlanUpdate)
		if err := validateBranchPlanMarkdown(value); err != nil {
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(
				TurnSubmissionModulePlanUpdate, nil, TurnSubmissionDiagnosticInvalidModule,
				"/plan_update", fmt.Sprintf("non-empty Markdown up to %d bytes", maxBranchPlanBytes),
				fmt.Sprintf("%d bytes", len([]byte(value))), err.Error(),
			))
			rejected[TurnSubmissionModulePlanUpdate] = true
		} else {
			prepared.result.PlanUpdate = &value
			prepared.planUpdateAccepted = true
		}
	}
	if planningEnabled && !prepared.planUpdateAccepted && !rejected[TurnSubmissionModulePlanUpdate] && input.PlanUpdate == nil && validation.CurrentPlan != nil {
		prepared.planUpdateAccepted = true
	}

	receipt := buildTurnSubmissionReceipt(prepared, rejected, diagnostics)
	return prepared, receipt
}

func clonePreparedTurnSubmission(current *PreparedTurnSubmission) *PreparedTurnSubmission {
	if current == nil {
		return &PreparedTurnSubmission{result: TurnResult{StateUpdates: []interactivestate.Update{}, Choices: []string{}}}
	}
	return &PreparedTurnSubmission{
		result: TurnResult{
			StateUpdates: append([]interactivestate.Update(nil), current.result.StateUpdates...),
			Choices:      append([]string(nil), current.result.Choices...),
			PlanUpdate:   cloneStringPointer(current.result.PlanUpdate),
		},
		stateUpdatesAccepted: current.stateUpdatesAccepted,
		choicesAccepted:      current.choicesAccepted,
		planUpdateAccepted:   current.planUpdateAccepted,
	}
}

func buildTurnSubmissionReceipt(prepared *PreparedTurnSubmission, rejected map[string]bool, diagnostics []TurnSubmissionDiagnostic) TurnSubmissionReceipt {
	receipt := TurnSubmissionReceipt{Ready: prepared.Ready()}
	receipt.ModuleStatus.StateChanges = turnSubmissionModuleStatus(prepared.stateUpdatesAccepted, rejected[TurnSubmissionModuleStateChanges])
	receipt.ModuleStatus.Choices = turnSubmissionModuleStatus(prepared.choicesAccepted, rejected[TurnSubmissionModuleChoices])
	receipt.ModuleStatus.PlanUpdate = turnSubmissionModuleStatus(prepared.planUpdateAccepted, rejected[TurnSubmissionModulePlanUpdate])
	for _, module := range []string{TurnSubmissionModuleStateChanges, TurnSubmissionModuleChoices, TurnSubmissionModulePlanUpdate} {
		status := receipt.ModuleStatus.StateChanges
		if module == TurnSubmissionModuleChoices {
			status = receipt.ModuleStatus.Choices
		} else if module == TurnSubmissionModulePlanUpdate {
			status = receipt.ModuleStatus.PlanUpdate
		}
		if status != TurnSubmissionModuleAccepted {
			receipt.RetryModules = append(receipt.RetryModules, module)
		}
		if status == TurnSubmissionModuleMissing {
			receipt.MissingModules = append(receipt.MissingModules, module)
		}
	}
	if len(diagnostics) > maxTurnSubmissionDiagnostics {
		receipt.DiagnosticsTruncated = true
		diagnostics = diagnostics[:maxTurnSubmissionDiagnostics]
	}
	receipt.Diagnostics = diagnostics
	return receipt
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func turnSubmissionModuleStatus(accepted, rejected bool) string {
	if accepted {
		return TurnSubmissionModuleAccepted
	}
	if rejected {
		return TurnSubmissionModuleRejected
	}
	return TurnSubmissionModuleMissing
}

func validateSubmittedChoices(values []string, configured int, terminal bool) ([]string, *TurnSubmissionDiagnostic) {
	configured = normalizeStoryChoiceCount(configured)
	if err := validateStoryChoiceCount(configured); err != nil {
		return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, "invalid_choice_count_config", "", fmt.Sprintf("%d-%d", MinStoryChoiceCount, MaxStoryChoiceCount), fmt.Sprint(configured), "The story choice count configuration is invalid.")
	}
	if len(values) == 0 {
		if terminal {
			return []string{}, nil
		}
		return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, TurnSubmissionDiagnosticChoiceCountMismatch, "/choices", fmt.Sprintf("exactly %d choices", configured), "0 choices", fmt.Sprintf("Non-terminal turns must submit exactly %d distinct choices.", configured))
	}
	seen := map[string]bool{}
	normalized := make([]string, 0, len(values))
	for index, value := range values {
		choice := strings.TrimSpace(value)
		if choice == "" {
			return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, intPointer(index), TurnSubmissionDiagnosticEmptyChoice, fmt.Sprintf("/choices/%d", index), "non-empty string", "empty string", "Choices must not be empty.")
		}
		if len([]byte(choice)) > maxTurnSubmissionChoiceBytes {
			return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, intPointer(index), "choice_too_large", fmt.Sprintf("/choices/%d", index), fmt.Sprintf("at most %d bytes", maxTurnSubmissionChoiceBytes), fmt.Sprintf("%d bytes", len([]byte(choice))), "The choice text is too long.")
		}
		key := normalizedChoiceKey(choice)
		if seen[key] {
			return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, intPointer(index), TurnSubmissionDiagnosticDuplicateChoice, fmt.Sprintf("/choices/%d", index), "distinct normalized choice", choice, "Choices must remain distinct after text normalization.")
		}
		seen[key] = true
		normalized = append(normalized, choice)
	}
	if terminal {
		return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, TurnSubmissionDiagnosticChoiceCountMismatch, "/choices", "empty array for the declared terminal turn", fmt.Sprintf("%d choices", len(normalized)), "RuleResolution declared a terminal turn, so choices must be an empty array.")
	}
	if len(normalized) != configured {
		return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, TurnSubmissionDiagnosticChoiceCountMismatch, "/choices", fmt.Sprintf("exactly %d choices", configured), fmt.Sprintf("%d choices", len(normalized)), fmt.Sprintf("Non-terminal turns must submit exactly %d distinct choices.", configured))
	}
	return normalized, nil
}

func decodeChoicesModule(raw json.RawMessage) ([]string, []TurnSubmissionDiagnostic) {
	var items []json.RawMessage
	if err := decodeStrictJSON(raw, &items, false); err != nil || items == nil {
		if err == nil {
			err = errors.New("choices cannot be null")
		}
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, TurnSubmissionDiagnosticInvalidModule, "/choices", "array of strings", jsonValueKind(raw), fmt.Sprintf("choices must be an array of strings: %v", err))}
	}
	if len(items) > MaxStoryChoiceCount {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, "too_many_choices", "/choices", fmt.Sprintf("at most %d choices", MaxStoryChoiceCount), fmt.Sprintf("%d choices", len(items)), fmt.Sprintf("choices cannot exceed %d items.", MaxStoryChoiceCount))}
	}
	choices := make([]string, 0, len(items))
	diagnostics := make([]TurnSubmissionDiagnostic, 0)
	for index, item := range items {
		var choice string
		if err := decodeStrictJSON(item, &choice, false); err != nil {
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, intPointer(index), TurnSubmissionDiagnosticInvalidModule, fmt.Sprintf("/choices/%d", index), "string", jsonValueKind(item), "Each choice must be a string."))
			continue
		}
		choices = append(choices, choice)
	}
	if len(diagnostics) > 0 {
		return nil, diagnostics
	}
	return choices, nil
}

func decodeStrictJSON(data []byte, target any, useNumber bool) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if useNumber {
		decoder.UseNumber()
	}
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func newTurnSubmissionDiagnostic(module string, index *int, code, path, expected, actual, message string) *TurnSubmissionDiagnostic {
	return &TurnSubmissionDiagnostic{
		Module:    module,
		Index:     index,
		Code:      code,
		Severity:  turnSubmissionSeverityError,
		Path:      path,
		Expected:  trimBytes(expected, maxTurnSubmissionDiagnosticMessage),
		Actual:    trimBytes(actual, maxTurnSubmissionDiagnosticMessage),
		Retryable: true,
		Message:   trimBytes(message, maxTurnSubmissionDiagnosticMessage),
	}
}

func intPointer(value int) *int {
	return &value
}

func jsonValueKind(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "empty"
	}
	switch trimmed[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 'n':
		return "null"
	case 't', 'f':
		return "bool"
	default:
		return "number or invalid JSON"
	}
}

func turnSubmissionAllowedFields(template ActorStateTemplate) []string {
	fields := make([]string, 0, len(template.Fields))
	for _, field := range template.Fields {
		fields = append(fields, actorStateFieldID(field))
		if len(fields) >= maxTurnSubmissionAllowedFields {
			break
		}
	}
	return fields
}
