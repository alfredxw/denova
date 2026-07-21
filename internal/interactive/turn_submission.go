package interactive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	TurnSubmissionModuleStateChanges  = "state_changes"
	TurnSubmissionModuleChoices       = "choices"
	turnSubmissionDirectorUpdateField = "director_update"

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
	maxTurnSubmissionArgumentsBytes    = 64 * 1024
	maxTurnSubmissionChoiceBytes       = 512
)

// TurnSubmissionDiagnostic is bounded, bilingual, and points to the exact
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
	MessageZH string `json:"message_zh"`
	MessageEN string `json:"message_en"`
}

type TurnSubmissionModuleStatus struct {
	StateChanges string `json:"state_changes"`
	Choices      string `json:"choices"`
}

// TurnSubmissionReceipt reports independent module acceptance. Ready becomes
// true only after both modules have been accepted, possibly across calls.
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
	StateUpdates   *[]StateUpdate
	Choices        *[]string
	DirectorUpdate *DirectorUpdateHint
	Diagnostics    []TurnSubmissionDiagnostic
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
}

// PreparedTurnSubmission holds accepted modules while failed modules are
// retried. It is immutable after construction.
type PreparedTurnSubmission struct {
	result               TurnResult
	stateUpdatesAccepted bool
	choicesAccepted      bool
}

func (s *PreparedTurnSubmission) TurnResult() TurnResult {
	if s == nil {
		return TurnResult{}
	}
	return TurnResult{
		StateUpdates:   append([]StateUpdate(nil), s.result.StateUpdates...),
		Choices:        append([]string(nil), s.result.Choices...),
		DirectorUpdate: normalizeDirectorUpdateHint(s.result.DirectorUpdate),
	}
}

func (s *PreparedTurnSubmission) Ready() bool {
	return s != nil && s.stateUpdatesAccepted && s.choicesAccepted
}

func decodeDirectorUpdateHint(raw json.RawMessage) (*DirectorUpdateHint, []TurnSubmissionDiagnostic) {
	var hint DirectorUpdateHint
	if err := decodeStrictJSON(raw, &hint, false); err != nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModuleChoices, nil, TurnSubmissionDiagnosticInvalidModule, "/director_update", "{needed:true,reason:string}", "invalid director_update",
			fmt.Sprintf("director_update 无效：%v", err), fmt.Sprintf("director_update is invalid: %v", err),
		)}
	}
	normalized := normalizeDirectorUpdateHint(&hint)
	if !hint.Needed {
		return nil, nil
	}
	if err := validateDirectorUpdateHint(normalized); err != nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModuleChoices, nil, TurnSubmissionDiagnosticInvalidModule, "/director_update", "needed=true with a bounded reason", "invalid director_update",
			err.Error(), "director_update must set needed=true with a bounded non-empty reason.",
		)}
	}
	return normalized, nil
}

// PrepareTurnSubmission accepts valid modules independently and retains any
// module accepted by an earlier call. state_changes remains atomic internally.
func PrepareTurnSubmission(validation TurnSubmissionContext, current *PreparedTurnSubmission, input TurnSubmissionInput) (*PreparedTurnSubmission, TurnSubmissionReceipt) {
	prepared := clonePreparedTurnSubmission(current)
	diagnostics := make([]TurnSubmissionDiagnostic, 0, len(input.Diagnostics))
	rejected := map[string]bool{}
	for _, diagnostic := range input.Diagnostics {
		if (diagnostic.Module == TurnSubmissionModuleStateChanges && prepared.stateUpdatesAccepted) ||
			(diagnostic.Module == TurnSubmissionModuleChoices && prepared.choicesAccepted) {
			continue
		}
		diagnostics = append(diagnostics, diagnostic)
		if diagnostic.Module == TurnSubmissionModuleStateChanges || diagnostic.Module == TurnSubmissionModuleChoices {
			rejected[diagnostic.Module] = true
		}
	}
	if input.StateUpdates != nil && !prepared.stateUpdatesAccepted && !rejected[TurnSubmissionModuleStateChanges] {
		updates := normalizeTurnStateUpdates(*input.StateUpdates)
		compileOptions := TurnStateUpdateCompileOptions{
			RuleResolution:           validation.RuleResolution,
			RuleStateConsumptionMode: validation.RuleStateConsumptionMode,
		}
		compiled, err := CompileTurnStateUpdates(validation.ActorState, validation.CurrentState, updates, compileOptions)
		if err != nil {
			validationErrors := flattenStateUpdateValidationErrors(err)
			if len(validationErrors) == 0 || validationErrors[0].Code != "too_many_state_updates" {
				collected := collectTurnStateUpdateValidationErrors(validation.ActorState, validation.CurrentState, updates, compileOptions)
				validationErrors = mergeStateUpdateValidationErrors(validationErrors, collected)
			}
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
			prepared.result.DirectorUpdate = normalizeDirectorUpdateHint(input.DirectorUpdate)
			prepared.choicesAccepted = true
		}
	}

	receipt := buildTurnSubmissionReceipt(prepared, rejected, diagnostics)
	return prepared, receipt
}

func clonePreparedTurnSubmission(current *PreparedTurnSubmission) *PreparedTurnSubmission {
	if current == nil {
		return &PreparedTurnSubmission{result: TurnResult{StateUpdates: []StateUpdate{}, Choices: []string{}}}
	}
	return &PreparedTurnSubmission{
		result: TurnResult{
			StateUpdates:   append([]StateUpdate(nil), current.result.StateUpdates...),
			Choices:        append([]string(nil), current.result.Choices...),
			DirectorUpdate: normalizeDirectorUpdateHint(current.result.DirectorUpdate),
		},
		stateUpdatesAccepted: current.stateUpdatesAccepted,
		choicesAccepted:      current.choicesAccepted,
	}
}

func buildTurnSubmissionReceipt(prepared *PreparedTurnSubmission, rejected map[string]bool, diagnostics []TurnSubmissionDiagnostic) TurnSubmissionReceipt {
	receipt := TurnSubmissionReceipt{Ready: prepared.Ready()}
	receipt.ModuleStatus.StateChanges = turnSubmissionModuleStatus(prepared.stateUpdatesAccepted, rejected[TurnSubmissionModuleStateChanges])
	receipt.ModuleStatus.Choices = turnSubmissionModuleStatus(prepared.choicesAccepted, rejected[TurnSubmissionModuleChoices])
	for _, module := range []string{TurnSubmissionModuleStateChanges, TurnSubmissionModuleChoices} {
		status := receipt.ModuleStatus.StateChanges
		if module == TurnSubmissionModuleChoices {
			status = receipt.ModuleStatus.Choices
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
		return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, "invalid_choice_count_config", "", fmt.Sprintf("%d-%d", MinStoryChoiceCount, MaxStoryChoiceCount), fmt.Sprint(configured), err.Error(), "The story choice count configuration is invalid.")
	}
	if len(values) == 0 {
		if terminal {
			return []string{}, nil
		}
		return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, TurnSubmissionDiagnosticChoiceCountMismatch, "/choices", fmt.Sprintf("exactly %d choices", configured), "0 choices", fmt.Sprintf("非终局回合必须提交恰好 %d 个不同的行动建议", configured), fmt.Sprintf("Non-terminal turns must submit exactly %d distinct choices.", configured))
	}
	seen := map[string]bool{}
	normalized := make([]string, 0, len(values))
	for index, value := range values {
		choice := strings.TrimSpace(value)
		if choice == "" {
			return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, intPointer(index), TurnSubmissionDiagnosticEmptyChoice, fmt.Sprintf("/choices/%d", index), "non-empty string", "empty string", "行动建议不能为空", "Choices must not be empty.")
		}
		if len([]byte(choice)) > maxTurnSubmissionChoiceBytes {
			return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, intPointer(index), "choice_too_large", fmt.Sprintf("/choices/%d", index), fmt.Sprintf("at most %d bytes", maxTurnSubmissionChoiceBytes), fmt.Sprintf("%d bytes", len([]byte(choice))), "行动建议文本过长", "The choice text is too long.")
		}
		key := normalizedChoiceKey(choice)
		if seen[key] {
			return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, intPointer(index), TurnSubmissionDiagnosticDuplicateChoice, fmt.Sprintf("/choices/%d", index), "distinct normalized choice", choice, "行动建议在文本标准化后重复", "Choices must remain distinct after text normalization.")
		}
		seen[key] = true
		normalized = append(normalized, choice)
	}
	if terminal {
		return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, TurnSubmissionDiagnosticChoiceCountMismatch, "/choices", "empty array for the declared terminal turn", fmt.Sprintf("%d choices", len(normalized)), "已由 RuleResolution 声明终局，choices 必须为空数组", "RuleResolution declared a terminal turn, so choices must be an empty array.")
	}
	if len(normalized) != configured {
		return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, TurnSubmissionDiagnosticChoiceCountMismatch, "/choices", fmt.Sprintf("exactly %d choices", configured), fmt.Sprintf("%d choices", len(normalized)), fmt.Sprintf("非终局回合必须提交恰好 %d 个不同的行动建议", configured), fmt.Sprintf("Non-terminal turns must submit exactly %d distinct choices.", configured))
	}
	return normalized, nil
}

func decodeChoicesModule(raw json.RawMessage) ([]string, []TurnSubmissionDiagnostic) {
	var items []json.RawMessage
	if err := decodeStrictJSON(raw, &items, false); err != nil || items == nil {
		if err == nil {
			err = errors.New("choices cannot be null")
		}
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, TurnSubmissionDiagnosticInvalidModule, "/choices", "array of strings", jsonValueKind(raw), fmt.Sprintf("choices 必须是字符串数组：%v", err), fmt.Sprintf("choices must be an array of strings: %v", err))}
	}
	if len(items) > MaxStoryChoiceCount {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, "too_many_choices", "/choices", fmt.Sprintf("at most %d choices", MaxStoryChoiceCount), fmt.Sprintf("%d choices", len(items)), fmt.Sprintf("choices 不能超过 %d 项", MaxStoryChoiceCount), fmt.Sprintf("choices cannot exceed %d items.", MaxStoryChoiceCount))}
	}
	choices := make([]string, 0, len(items))
	diagnostics := make([]TurnSubmissionDiagnostic, 0)
	for index, item := range items {
		var choice string
		if err := decodeStrictJSON(item, &choice, false); err != nil {
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, intPointer(index), TurnSubmissionDiagnosticInvalidModule, fmt.Sprintf("/choices/%d", index), "string", jsonValueKind(item), "行动建议必须是字符串", "Each choice must be a string."))
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

func newTurnSubmissionDiagnostic(module string, index *int, code, path, expected, actual, messageZH, messageEN string) *TurnSubmissionDiagnostic {
	return &TurnSubmissionDiagnostic{
		Module:    module,
		Index:     index,
		Code:      code,
		Severity:  turnSubmissionSeverityError,
		Path:      path,
		Expected:  trimBytes(expected, maxTurnSubmissionDiagnosticMessage),
		Actual:    trimBytes(actual, maxTurnSubmissionDiagnosticMessage),
		Retryable: true,
		MessageZH: trimBytes(messageZH, maxTurnSubmissionDiagnosticMessage),
		MessageEN: trimBytes(messageEN, maxTurnSubmissionDiagnosticMessage),
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
