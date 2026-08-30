package interactive

import (
	"context"
	interactivestate "denova/internal/interactive/state"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

const turnSubmissionStateChangesField = "state_changes"

// TurnStateChangeInput is the model-facing state mutation shape. Stable IDs
// are separate fields so the model never has to construct or escape a JSON
// Pointer; the backend compiles this shape to the persisted interactivestate.Update.
type TurnStateChangeInput struct {
	Op           string         `json:"op" jsonschema:"enum=replace,enum=delta,enum=create,enum=archive,enum=restore" jsonschema_description:"replace writes a field's complete new value; delta changes an existing number; create adds an Actor; archive removes an Actor from runtime participation; restore resumes participation."`
	ActorID      string         `json:"actor_id" jsonschema_description:"For an existing Actor, copy the ID from the state handbook exactly. For create, use the character name in the story's language and make it identical to name."`
	FieldID      string         `json:"field_id,omitempty" jsonschema_description:"Required for replace/delta. Copy the Field ID from the Actor State Handbook exactly."`
	Subpath      []string       `json:"subpath,omitempty" jsonschema_description:"Use only for nested updates inside object fields. Supply one string segment per level; do not construct a path string."`
	Value        any            `json:"value,omitempty" jsonschema_description:"The complete replacement value or numeric delta. It must match the field type; number, bool, object, and list fields require native JSON values, not quoted strings."`
	TemplateID   string         `json:"template_id,omitempty" jsonschema_description:"Required only for create. Copy a Template ID from Templates Available to New Actors exactly."`
	Name         string         `json:"name,omitempty" jsonschema_description:"Required for create. Use the character name in the story's language and make it identical to actor_id."`
	Role         string         `json:"role,omitempty" jsonschema_description:"Optional role used only for create."`
	Description  string         `json:"description,omitempty" jsonschema_description:"Brief Actor description used only for create."`
	InitialState map[string]any `json:"initial_state,omitempty" jsonschema_description:"Used only for create. Keys must be exact Field IDs from the selected template; number, bool, object, and list fields require native JSON values, not quoted strings."`
	Reason       string         `json:"reason,omitempty" jsonschema_description:"Required for archive/restore. Briefly state the factual basis for archiving or restoring the Actor."`
}

// DecodeInteractiveTurnSubmissionInput independently decodes state_changes
// and choices from one model-facing tool call. A malformed module does not
// discard a valid sibling module, and later calls may provide only retry_modules.
func DecodeInteractiveTurnSubmissionInput(arguments string) TurnSubmissionInput {
	if len([]byte(arguments)) > maxTurnSubmissionArgumentsBytes {
		return invalidUnifiedTurnSubmissionInput("submission_too_large", "", fmt.Sprintf("%d bytes", len([]byte(arguments))), fmt.Sprintf("Tool arguments exceed %d bytes.", maxTurnSubmissionArgumentsBytes))
	}
	var root map[string]json.RawMessage
	if err := decodeStrictJSON([]byte(arguments), &root, false); err != nil {
		return invalidUnifiedTurnSubmissionInput(TurnSubmissionDiagnosticInvalidJSON, "", "invalid JSON", fmt.Sprintf("Turn submission arguments are not valid JSON: %v", err))
	}
	if root == nil {
		return invalidUnifiedTurnSubmissionInput(TurnSubmissionDiagnosticInvalidTopLevel, "", "null", "Turn submission arguments must be an object.")
	}
	allowed := map[string]bool{
		turnSubmissionStateChangesField: true,
		TurnSubmissionModuleChoices:     true,
		TurnSubmissionModulePlanUpdate:  true,
	}
	unknown := make([]string, 0)
	for key := range root {
		if !allowed[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return invalidUnifiedTurnSubmissionInput(
			TurnSubmissionDiagnosticInvalidTopLevel,
			"",
			strings.Join(unknown, ","),
			"Turn submission arguments may only contain state_changes, choices, and optional plan_update.",
		)
	}

	input := TurnSubmissionInput{}
	if raw, exists := root[turnSubmissionStateChangesField]; exists {
		updates, diagnostics := decodeStructuredStateChangesModule(raw)
		input.Diagnostics = append(input.Diagnostics, diagnostics...)
		if len(diagnostics) == 0 {
			input.StateUpdates = &updates
		}
	}
	if raw, exists := root[TurnSubmissionModuleChoices]; exists {
		choices, diagnostics := decodeChoicesModule(raw)
		input.Diagnostics = append(input.Diagnostics, diagnostics...)
		if !turnSubmissionHasDiagnostic(input.Diagnostics, TurnSubmissionModuleChoices) {
			input.Choices = &choices
		}
	}
	if raw, exists := root[TurnSubmissionModulePlanUpdate]; exists {
		var plan string
		if err := decodeStrictJSON(raw, &plan, false); err != nil {
			input.Diagnostics = append(input.Diagnostics, *newTurnSubmissionDiagnostic(
				TurnSubmissionModulePlanUpdate, nil, TurnSubmissionDiagnosticInvalidModule,
				"/plan_update", "Markdown string", jsonValueKind(raw),
				fmt.Sprintf("plan_update must be a string: %v", err),
			))
		} else {
			input.PlanUpdate = &plan
		}
	}
	return input
}

func decodeStructuredStateChangesModule(raw json.RawMessage) ([]interactivestate.Update, []TurnSubmissionDiagnostic) {
	items, err := decodeStructuredStateChangeItems(raw)
	if err != nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModuleStateChanges,
			nil,
			TurnSubmissionDiagnosticInvalidModule,
			"/state_changes",
			"array",
			jsonValueKind(raw),
			fmt.Sprintf("state_changes must be a native array; only one string layer containing valid array JSON is tolerated: %v", err),
		)}
	}
	updates := make([]interactivestate.Update, 0, len(items))
	diagnostics := make([]TurnSubmissionDiagnostic, 0)
	for index, item := range items {
		var change TurnStateChangeInput
		if err := decodeStrictJSON(item, &change, true); err != nil {
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(
				TurnSubmissionModuleStateChanges,
				intPointer(index),
				TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/state_changes/%d", index),
				"structured state change",
				jsonValueKind(item),
				fmt.Sprintf("The state change shape is invalid: %v", err),
			))
			continue
		}
		update, err := stateUpdateFromStructuredInput(change)
		if err != nil {
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(
				TurnSubmissionModuleStateChanges,
				intPointer(index),
				TurnSubmissionDiagnosticInvalidModule,
				fmt.Sprintf("/state_changes/%d", index),
				"valid replace, delta, create, archive, or restore fields",
				"invalid state change",
				"The structured state change has incompatible or missing fields.",
			))
			continue
		}
		updates = append(updates, update)
	}
	if len(diagnostics) > 0 {
		return nil, diagnostics
	}
	return updates, nil
}

// decodeStructuredStateChangeItems keeps the model-facing contract strict while
// tolerating the one legacy shape observed in real runs: an otherwise valid
// array JSON value encoded once as a string. It intentionally does not recurse,
// repair malformed pseudo-JSON, or accept null so invalid facts still trigger a
// targeted state_changes retry.
func decodeStructuredStateChangeItems(raw json.RawMessage) ([]json.RawMessage, error) {
	var items []json.RawMessage
	directErr := decodeStrictJSON(raw, &items, false)
	if directErr == nil && items != nil {
		return items, nil
	}
	if jsonValueKind(raw) != "string" {
		if directErr != nil {
			return nil, directErr
		}
		return nil, errors.New("state_changes cannot be null")
	}

	var encoded string
	if err := decodeStrictJSON(raw, &encoded, false); err != nil {
		return nil, err
	}
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, errors.New("state_changes string cannot be empty")
	}
	items = nil
	if err := decodeStrictJSON([]byte(encoded), &items, false); err != nil {
		return nil, fmt.Errorf("state_changes string does not contain valid array JSON: %w", err)
	}
	if items == nil {
		return nil, errors.New("state_changes string cannot contain null")
	}
	slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-turn-submission] accepted one-layer string-encoded state_changes bytes=%d location=internal/interactive/turn_submission_decode.go", len(encoded)))
	return items, nil
}

func stateUpdateFromStructuredInput(change TurnStateChangeInput) (interactivestate.Update, error) {
	change.Op = strings.ToLower(strings.TrimSpace(change.Op))
	change.ActorID = strings.TrimSpace(change.ActorID)
	change.FieldID = strings.TrimSpace(change.FieldID)
	change.TemplateID = strings.TrimSpace(change.TemplateID)
	if change.ActorID == "" {
		return interactivestate.Update{}, fmt.Errorf("state_changes 缺少 actor_id")
	}
	switch change.Op {
	case interactivestate.Replace, interactivestate.Delta:
		if change.FieldID == "" {
			return interactivestate.Update{}, fmt.Errorf("%s 状态变化缺少 field_id", change.Op)
		}
		if change.Value == nil {
			return interactivestate.Update{}, fmt.Errorf("%s 状态变化缺少非空 value", change.Op)
		}
		if change.TemplateID != "" || change.Name != "" || change.Role != "" || change.Description != "" || change.InitialState != nil || strings.TrimSpace(change.Reason) != "" {
			return interactivestate.Update{}, fmt.Errorf("%s 不能包含 create 或 lifecycle 专用字段", change.Op)
		}
		segments := []string{change.ActorID, change.FieldID}
		for _, segment := range change.Subpath {
			segment = strings.TrimSpace(segment)
			if segment == "" {
				return interactivestate.Update{}, fmt.Errorf("subpath 不能包含空段")
			}
			segments = append(segments, segment)
		}
		return interactivestate.Update{Op: change.Op, Path: interactivestate.FormatPath(segments), Value: change.Value}, nil
	case interactivestate.Create:
		if change.TemplateID == "" {
			return interactivestate.Update{}, fmt.Errorf("create 状态变化缺少 template_id")
		}
		if strings.TrimSpace(change.Name) == "" {
			return interactivestate.Update{}, fmt.Errorf("create 状态变化缺少 name；新建 Actor 的 name 必须与 actor_id 完全相同")
		}
		if change.FieldID != "" || len(change.Subpath) > 0 || change.Value != nil || strings.TrimSpace(change.Reason) != "" {
			return interactivestate.Update{}, fmt.Errorf("create 不能包含 field_id、subpath、value 或 reason")
		}
		value := map[string]any{"template_id": change.TemplateID}
		if name := strings.TrimSpace(change.Name); name != "" {
			value["name"] = name
		}
		if role := strings.TrimSpace(change.Role); role != "" {
			value["role"] = role
		}
		if description := strings.TrimSpace(change.Description); description != "" {
			value["description"] = description
		}
		if change.InitialState != nil {
			value["state"] = change.InitialState
		}
		return interactivestate.Update{Op: interactivestate.Create, Path: interactivestate.FormatPath([]string{change.ActorID}), Value: value}, nil
	case interactivestate.Archive, interactivestate.Restore:
		reason := strings.TrimSpace(change.Reason)
		if reason == "" {
			return interactivestate.Update{}, fmt.Errorf("%s 状态变化缺少非空 reason", change.Op)
		}
		if len([]byte(reason)) > maxActorArchiveReasonBytes {
			return interactivestate.Update{}, fmt.Errorf("%s reason 超过 %d bytes", change.Op, maxActorArchiveReasonBytes)
		}
		if change.FieldID != "" || len(change.Subpath) > 0 || change.Value != nil || change.TemplateID != "" || change.Name != "" || change.Role != "" || change.Description != "" || change.InitialState != nil {
			return interactivestate.Update{}, fmt.Errorf("%s 只能包含 actor_id 和 reason", change.Op)
		}
		return interactivestate.Update{Op: change.Op, Path: interactivestate.FormatPath([]string{change.ActorID}), Value: map[string]any{"reason": reason}}, nil
	default:
		return interactivestate.Update{}, fmt.Errorf("op 必须是 replace、delta、create、archive 或 restore")
	}
}

func invalidUnifiedTurnSubmissionInput(code, path, actual, message string) TurnSubmissionInput {
	diagnostics := make([]TurnSubmissionDiagnostic, 0, 2)
	for _, module := range []string{TurnSubmissionModuleStateChanges, TurnSubmissionModuleChoices, TurnSubmissionModulePlanUpdate} {
		diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(
			module,
			nil,
			code,
			path,
			"object containing state_changes and/or choices",
			actual,
			message,
		))
	}
	return TurnSubmissionInput{Diagnostics: diagnostics}
}

func turnSubmissionHasDiagnostic(diagnostics []TurnSubmissionDiagnostic, module string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Module == module {
			return true
		}
	}
	return false
}
