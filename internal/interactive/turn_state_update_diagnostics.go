package interactive

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// collectTurnStateUpdateValidationErrors runs only after the atomic compiler
// rejects a module. It validates each operation against a cloned working state
// so independent failures can be returned together without staging partial
// results. A successful operation is applied only to the private clone so later
// operations can still reference valid Actors created earlier in the same call.
// Paths depending on a failed operation are skipped to avoid cascading errors,
// while overlap checks are retained across successfully compiled operations.
func collectTurnStateUpdateValidationErrors(system StoryDirectorActorStateSystem, currentState map[string]any, updates []StateUpdate, options TurnStateUpdateCompileOptions) []*StateUpdateValidationError {
	workingState := cloneActorStateRoot(currentState)
	validationErrors := make([]*StateUpdateValidationError, 0)
	canonicalPaths := make([][]string, 0, len(updates))
	failedPaths := make([][]string, 0)
	for index, update := range updates {
		inputPath, inputPathValid := stateUpdateDiagnosticSegments(update.Path)
		if inputPathValid && overlappingStateUpdatePath(failedPaths, inputPath) != "" {
			continue
		}
		if inputPathValid {
			if overlap := overlappingStateUpdatePath(canonicalPaths, inputPath); overlap != "" {
				validationErrors = append(validationErrors, stateUpdateOverlapValidationError(index, update.Path, overlap))
				failedPaths = append(failedPaths, inputPath)
				if len(validationErrors) > maxTurnSubmissionDiagnostics {
					return validationErrors
				}
				continue
			}
		}
		compiled, err := CompileTurnStateUpdates(system, workingState, []StateUpdate{update}, options)
		if err != nil {
			for _, item := range flattenStateUpdateValidationErrors(err) {
				cloned := *item
				cloned.Index = index
				validationErrors = append(validationErrors, &cloned)
				if len(validationErrors) > maxTurnSubmissionDiagnostics {
					return validationErrors
				}
			}
			if inputPathValid {
				failedPaths = append(failedPaths, inputPath)
			}
			continue
		}
		compiledPaths := make([][]string, 0, len(compiled.Updates))
		overlap := ""
		for _, compiledUpdate := range compiled.Updates {
			compiledPath, ok := stateUpdateDiagnosticSegments(compiledUpdate.Path)
			if !ok {
				continue
			}
			if conflict := overlappingStateUpdatePath(canonicalPaths, compiledPath); conflict != "" {
				overlap = conflict
				break
			}
			compiledPaths = append(compiledPaths, compiledPath)
		}
		if overlap != "" {
			validationErrors = append(validationErrors, stateUpdateOverlapValidationError(index, update.Path, overlap))
			if inputPathValid {
				failedPaths = append(failedPaths, inputPath)
			}
			if len(validationErrors) > maxTurnSubmissionDiagnostics {
				return validationErrors
			}
			continue
		}
		for _, op := range compiled.Ops {
			applyStateOp(workingState, op)
		}
		for _, op := range compiled.ActorOps {
			applyActorStateOp(workingState, op)
		}
		canonicalPaths = append(canonicalPaths, compiledPaths...)
	}
	return validationErrors
}

func diagnosticsForStateUpdateError(err error) []TurnSubmissionDiagnostic {
	var validationErrors *StateUpdateValidationErrors
	if errors.As(err, &validationErrors) && len(validationErrors.Items) > 0 {
		diagnostics := make([]TurnSubmissionDiagnostic, 0, len(validationErrors.Items))
		for _, validationError := range validationErrors.Items {
			if validationError != nil {
				diagnostics = append(diagnostics, diagnosticForStateUpdateValidationError(validationError))
			}
		}
		return diagnostics
	}
	var validationError *StateUpdateValidationError
	if !errors.As(err, &validationError) {
		return []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(TurnSubmissionModuleStateChanges, nil, "state_changes_invalid", "/state_changes", "valid atomic state_changes list", "invalid", trimBytes(err.Error(), maxTurnSubmissionDiagnosticMessage), "The state_changes module is invalid.")}
	}
	return []TurnSubmissionDiagnostic{diagnosticForStateUpdateValidationError(validationError)}
}

func diagnosticForStateUpdateValidationError(validationError *StateUpdateValidationError) TurnSubmissionDiagnostic {
	path := fmt.Sprintf("/state_changes/%d", validationError.Index)
	if detail := strings.TrimSpace(validationError.DiagnosticPath); detail != "" {
		if !strings.HasPrefix(detail, "/") {
			path += "/"
		}
		path += detail
	}
	return *newTurnSubmissionDiagnostic(
		TurnSubmissionModuleStateChanges,
		intPointer(validationError.Index),
		validationError.Code,
		path,
		validationError.Expected,
		validationError.Actual,
		trimBytes(validationError.Error(), maxTurnSubmissionDiagnosticMessage),
		stateUpdateDiagnosticEnglishForError(validationError),
	)
}

func stateUpdateDiagnosticEnglishForError(validationError *StateUpdateValidationError) string {
	if validationError == nil {
		return "The state change failed frozen-schema validation."
	}
	typeMismatch := validationError.Expected == "number" || validationError.Expected == "bool" || validationError.Expected == "object" || validationError.Expected == "list"
	if typeMismatch {
		switch validationError.Code {
		case "actor_create_invalid", "actor_state_invalid", "state_value_invalid":
			return fmt.Sprintf("Expected JSON %s but received %s. Use native JSON values such as 0, false, {}, or [] instead of quoted encodings.", validationError.Expected, validationError.Actual)
		}
	}
	return stateUpdateDiagnosticEnglish(validationError.Code)
}

func stateUpdateDiagnosticEnglish(code string) string {
	switch code {
	case "invalid_state_path":
		return "The structured actor_id, field_id, or subpath is invalid."
	case "invalid_actor_id":
		return "actor_id must be a normalized state-panel Actor ID."
	case "actor_not_found":
		return "The referenced Actor ID does not exist in the current state."
	case "actor_name_id_mismatch":
		return "A newly created Actor must use its story-language name unchanged as both actor_id and name."
	case "state_field_not_found":
		return "The state field does not exist in the Actor's frozen schema."
	case "delta_target_not_number":
		return "delta requires an existing numeric target and never treats a missing value as zero."
	case "duplicate_rule_state_update":
		return "RuleResolution already consumes this field in the current turn."
	case "overlapping_state_path":
		return "State changes in one atomic module must not duplicate or overlap."
	case "state_value_too_large":
		return "The state change value exceeds the bounded payload limit."
	default:
		return "The state change failed frozen-schema validation."
	}
}

func stateUpdateDiagnosticSegments(path string) ([]string, bool) {
	segments, err := parseStateUpdatePath(strings.TrimSpace(path))
	return segments, err == nil
}

func stateUpdateOverlapValidationError(index int, path, overlap string) *StateUpdateValidationError {
	return stateUpdateError(
		index,
		"overlapping_state_path",
		path,
		"non-overlapping paths",
		overlap,
		fmt.Errorf("同一次提交不能包含重复或相互覆盖的状态路径: %s", overlap),
	)
}

func mergeStateUpdateValidationErrors(primary, additional []*StateUpdateValidationError) []*StateUpdateValidationError {
	merged := make([]*StateUpdateValidationError, 0, len(primary)+len(additional))
	seen := map[string]bool{}
	appendUnique := func(items []*StateUpdateValidationError) {
		for _, item := range items {
			if item == nil {
				continue
			}
			key := fmt.Sprintf("%d\x00%s\x00%s", item.Index, item.Code, item.DiagnosticPath)
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, item)
		}
	}
	appendUnique(primary)
	appendUnique(additional)
	return merged
}

func flattenStateUpdateValidationErrors(err error) []*StateUpdateValidationError {
	var multiple *StateUpdateValidationErrors
	if errors.As(err, &multiple) && len(multiple.Items) > 0 {
		return multiple.Items
	}
	var single *StateUpdateValidationError
	if errors.As(err, &single) {
		return []*StateUpdateValidationError{single}
	}
	return nil
}

// validateTurnSubmissionActorInitialState reports every independently invalid
// explicit field before Actor creation reaches the fail-fast state builder.
// Defaults are intentionally omitted because the builder remains authoritative
// for resolving them after this preflight succeeds.
func validateTurnSubmissionActorInitialState(index int, updatePath string, template ActorStateTemplate, state map[string]any) []*StateUpdateValidationError {
	if len(state) == 0 || template.ID == "" {
		return nil
	}
	fieldsByReference := actorStateFieldsByReference(template)
	keys := make([]string, 0, len(state))
	for key := range state {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	validationErrors := make([]*StateUpdateValidationError, 0)
	for _, rawKey := range keys {
		if len(validationErrors) > maxTurnSubmissionDiagnostics {
			return validationErrors
		}
		value := state[rawKey]
		if value == nil {
			continue
		}
		diagnosticPath := formatStateUpdatePath([]string{"initial_state", rawKey})
		field, found := fieldsByReference[actorStateFieldNameKey(rawKey)]
		if !found {
			err := stateUpdateError(
				index,
				"actor_create_invalid",
				updatePath,
				strings.Join(turnSubmissionAllowedFields(template), ", "),
				rawKey,
				fmt.Errorf("Actor 状态字段不在模板中: template=%s field=%s", template.ID, rawKey),
			)
			err.DiagnosticPath = diagnosticPath
			validationErrors = append(validationErrors, err)
			continue
		}
		_, err := normalizeActorStateValue(field, value)
		if err != nil {
			validationError := stateUpdateError(index, "actor_create_invalid", updatePath, field.Type, stateUpdateActual(value), err)
			validationError.DiagnosticPath = diagnosticPath
			validationErrors = append(validationErrors, validationError)
			continue
		}
	}
	return validationErrors
}
