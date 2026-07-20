package interactive

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
)

const maxTurnStateUpdateValueBytes = maxInteractiveTextBytes

// TurnStateUpdateCompileOptions keeps validation inputs explicit at the
// model-to-state boundary.
type TurnStateUpdateCompileOptions struct {
	SourceTurnID             string
	RuleResolution           *RuleResolution
	RuleStateConsumptionMode string
}

// CompiledTurnStateUpdates contains canonical audit input and deterministic
// reducer operations. Reducer operations, rather than model paths, remain the
// replay source of truth.
type CompiledTurnStateUpdates struct {
	Updates  []StateUpdate
	Ops      []StateOp
	ActorOps []ActorStateOp
}

// StateUpdateValidationError identifies the exact operation that made the
// atomic state_updates module invalid.
type StateUpdateValidationError struct {
	Index          int
	Code           string
	Path           string
	DiagnosticPath string
	Expected       string
	Actual         string
	Cause          error
}

func (e *StateUpdateValidationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Code
}

// StateUpdateValidationErrors keeps independent failures from one atomic
// module together so callers can repair them in one retry. It still unwraps to
// individual errors for existing fail-fast callers using errors.As.
type StateUpdateValidationErrors struct {
	Items []*StateUpdateValidationError
}

func (e *StateUpdateValidationErrors) Error() string {
	if e == nil || len(e.Items) == 0 {
		return ""
	}
	return e.Items[0].Error()
}

func (e *StateUpdateValidationErrors) Unwrap() []error {
	if e == nil {
		return nil
	}
	errors := make([]error, 0, len(e.Items))
	for _, item := range e.Items {
		if item != nil {
			errors = append(errors, item)
		}
	}
	return errors
}

// CompileTurnStateUpdates validates the complete state_updates module against
// the frozen Actor schema and current replayed state. Any invalid operation
// rejects the whole module.
func CompileTurnStateUpdates(system StoryDirectorActorStateSystem, currentState map[string]any, updates []StateUpdate, options TurnStateUpdateCompileOptions) (CompiledTurnStateUpdates, error) {
	updates = normalizeTurnStateUpdates(updates)
	if len(updates) > maxInteractiveListItems {
		return CompiledTurnStateUpdates{}, &StateUpdateValidationError{
			Index:    maxInteractiveListItems,
			Code:     "too_many_state_updates",
			Expected: fmt.Sprintf("at most %d operations", maxInteractiveListItems),
			Actual:   fmt.Sprintf("%d operations", len(updates)),
			Cause:    fmt.Errorf("state_updates 不能超过 %d 项", maxInteractiveListItems),
		}
	}

	system = normalizeActorStateSystem(system)
	workingState := cloneActorStateRoot(currentState)
	compiled := CompiledTurnStateUpdates{Updates: []StateUpdate{}, Ops: []StateOp{}, ActorOps: []ActorStateOp{}}
	canonicalPaths := make([][]string, 0, len(updates))

	for index, update := range updates {
		deltaNormalized := false
		if update.Op == TurnStateUpdateDelta {
			if converted, changed := normalizeTurnSubmissionFieldValue(ActorStateField{Type: "number"}, update.Value); changed {
				update.Value = converted
				deltaNormalized = true
			}
		}
		if err := validateStateUpdateShape(update); err != nil {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "invalid_state_update", update.Path, "replace, delta, or create with a non-null value", stateUpdateActual(update.Value), err)
		}
		segments, err := parseStateUpdatePath(update.Path)
		if err != nil {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "invalid_state_path", update.Path, "schema-bound JSON Pointer", update.Path, err)
		}
		actorID := segments[0]
		if actorID == "" || normalizeStatePanelActorID(actorID) != actorID {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "invalid_actor_id", update.Path, "normalized actor_id", actorID, fmt.Errorf("状态路径包含无效 actor_id: %q", actorID))
		}
		if err := validateStateUpdateValueSize(update.Value); err != nil {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "state_value_too_large", update.Path, fmt.Sprintf("at most %d JSON bytes", maxTurnStateUpdateValueBytes), stateUpdateActual(update.Value), err)
		}

		if update.Op == TurnStateUpdateCreate {
			if len(segments) != 1 {
				return CompiledTurnStateUpdates{}, stateUpdateError(index, "invalid_create_path", update.Path, "/<actor_id>", update.Path, fmt.Errorf("create 只能作用于 Actor 根路径"))
			}
			canonical := []string{actorID}
			if conflict := overlappingStateUpdatePath(canonicalPaths, canonical); conflict != "" {
				return CompiledTurnStateUpdates{}, stateUpdateError(index, "overlapping_state_path", update.Path, "non-overlapping paths", conflict, fmt.Errorf("同一次提交不能包含重复或相互覆盖的状态路径: %s", conflict))
			}
			patch, err := actorPatchFromCreateUpdate(actorID, update.Value)
			if err != nil {
				return CompiledTurnStateUpdates{}, stateUpdateError(index, "invalid_actor_create", update.Path, "actor create object", stateUpdateActual(update.Value), err)
			}
			configuredInitialActor := actorStateInitialActorIndex(system.InitialActors, actorID) >= 0
			if !configuredInitialActor && (patch.ActorName == "" || normalizeStatePanelActorID(patch.ActorName) != actorID) {
				return CompiledTurnStateUpdates{}, stateUpdateError(index, "actor_name_id_mismatch", update.Path, "actor_id identical to name", patch.ActorName, fmt.Errorf("新建 Actor 的 actor_id 必须与 name 完全相同，并直接使用故事语言中的角色名称: actor_id=%q name=%q", actorID, patch.ActorName))
			}
			if !configuredInitialActor {
				patch.ActorName = actorID
			}
			if template := actorStateTemplateByID(system, patch.TemplateID); template.ID != "" {
				patch.State = normalizeTurnSubmissionActorStateValues(actorID, template, patch.State)
				if validationErrors := validateTurnSubmissionActorInitialState(index, update.Path, template, patch.State); len(validationErrors) > 0 {
					return CompiledTurnStateUpdates{}, &StateUpdateValidationErrors{Items: validationErrors}
				}
			}
			patch.SourceTurnID = options.SourceTurnID
			normalized, ops, actorOps, _, _, err := validateActorStatePatch(system, workingState, patch)
			if err != nil {
				return CompiledTurnStateUpdates{}, stateUpdateError(index, "actor_create_invalid", update.Path, "valid template and state", stateUpdateActual(update.Value), err)
			}
			if err := validateActorStateRecordNameIDs(actorStateTemplateByID(system, normalized.TemplateID), normalized.State); err != nil {
				return CompiledTurnStateUpdates{}, stateUpdateError(index, "state_record_name_id_mismatch", update.Path, "record ID identical to its name", stateUpdateActual(normalized.State), err)
			}
			for _, op := range ops {
				applyStateOp(workingState, op)
			}
			for _, op := range actorOps {
				applyActorStateOp(workingState, op)
			}
			canonicalPaths = append(canonicalPaths, canonical)
			compiled.Updates = append(compiled.Updates, StateUpdate{Op: TurnStateUpdateCreate, Path: formatStateUpdatePath(canonical), Value: actorCreateAuditValue(normalized)})
			compiled.Ops = append(compiled.Ops, ops...)
			compiled.ActorOps = append(compiled.ActorOps, actorOps...)
			continue
		}

		if len(segments) < 2 {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "missing_state_field", update.Path, "/<actor_id>/<field_id>", update.Path, fmt.Errorf("replace 和 delta 必须指定状态字段"))
		}
		templateID, found := actorTemplateIDFromStateOrSystem(workingState, system, actorID)
		if !found {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "actor_not_found", update.Path, "existing actor_id", actorID, fmt.Errorf("状态路径中的 Actor 不存在: %s", actorID))
		}
		template := actorStateTemplateByID(system, templateID)
		field, found := actorStateFieldByID(template, segments[1])
		if !found {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "state_field_not_found", update.Path, strings.Join(turnSubmissionAllowedFields(template), ", "), segments[1], fmt.Errorf("Actor 状态字段不在模板中: actor=%s field=%s", actorID, segments[1]))
		}
		if field.Visibility == "hidden" {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "state_field_hidden", update.Path, "model-writable field", actorStateFieldID(field), fmt.Errorf("隐藏状态字段不能由 Game Agent 直接修改"))
		}
		fieldID := actorStateFieldID(field)
		if deltaNormalized {
			log.Printf("[interactive-turn-submission] normalized lossless delta actor_id=%q field_id=%q from=string to=number location=internal/interactive/turn_state_updates.go", actorID, fieldID)
		}
		canonical := append([]string{actorID, fieldID}, segments[2:]...)
		if conflict := overlappingStateUpdatePath(canonicalPaths, canonical); conflict != "" {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "overlapping_state_path", update.Path, "non-overlapping paths", conflict, fmt.Errorf("同一次提交不能包含重复或相互覆盖的状态路径: %s", conflict))
		}
		if stateUpdateConflictsWithRuleResolution(options, actorID, fieldID) {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "duplicate_rule_state_update", update.Path, "a field not consumed by RuleResolution", fieldID, fmt.Errorf("该字段已由本轮 RuleResolution 自动消费，不能在 state_updates 中重复修改"))
		}
		if len(segments) == 2 && update.Op == TurnStateUpdateReplace {
			if converted, changed := normalizeTurnSubmissionFieldValue(field, update.Value); changed {
				log.Printf("[interactive-turn-submission] normalized lossless field value actor_id=%q field_id=%q from=string to=%s location=internal/interactive/turn_state_updates.go", actorID, fieldID, field.Type)
				update.Value = converted
			}
		}

		currentValue := actorStateFieldValue(workingState, actorID, fieldID)
		nextValue, auditValue, err := applyStateUpdateValue(field, currentValue, segments[2:], update)
		if err != nil {
			code := "state_value_invalid"
			actualValue := update.Value
			if update.Op == TurnStateUpdateDelta {
				code = "delta_target_not_number"
				actualValue = currentValue
				if len(segments) > 2 {
					if currentObject, ok := currentValue.(map[string]any); ok {
						if leaf, found := stateUpdateNestedValue(currentObject, segments[2:]); found {
							actualValue = leaf
						} else {
							actualValue = nil
						}
					}
				}
			} else if len(segments) > 2 {
				actualValue = currentValue
			}
			return CompiledTurnStateUpdates{}, stateUpdateError(index, code, update.Path, stateUpdateExpected(field, segments[2:], update.Op), stateUpdateActual(actualValue), err)
		}
		if update.Op == TurnStateUpdateReplace && len(segments) <= 3 {
			if normalized, changed := normalizeStatePanelRecordNameIDs(fieldID, nextValue); changed {
				nextValue = normalized
				if len(segments) == 2 {
					auditValue = normalized
				} else if normalizedObject, ok := normalized.(map[string]any); ok {
					if leaf, found := stateUpdateNestedValue(normalizedObject, segments[2:]); found {
						auditValue = leaf
					}
				}
			}
		}
		if err := validateStatePanelRecordNameIDs(fieldID, nextValue); err != nil {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "state_record_name_id_mismatch", update.Path, "record ID identical to its name", stateUpdateActual(nextValue), err)
		}
		if reflect.DeepEqual(currentValue, nextValue) {
			continue
		}

		patch := ActorStatePatch{
			ActorID:      actorID,
			State:        map[string]any{fieldID: nextValue},
			SourceTurnID: options.SourceTurnID,
		}
		if getPath(workingState, actorStateRoot+"."+actorID) == nil {
			patch.TemplateID = templateID
			for _, initialActor := range system.InitialActors {
				if initialActor.ID != actorID {
					continue
				}
				patch.ActorName = initialActor.Name
				patch.Role = initialActor.Role
				patch.Description = initialActor.Description
				break
			}
		}
		_, ops, actorOps, _, _, err := validateActorStatePatch(system, workingState, patch)
		if err != nil {
			return CompiledTurnStateUpdates{}, stateUpdateError(index, "actor_state_invalid", update.Path, field.Type, stateUpdateActual(nextValue), err)
		}
		for _, op := range ops {
			applyStateOp(workingState, op)
		}
		for _, op := range actorOps {
			applyActorStateOp(workingState, op)
		}
		canonicalPaths = append(canonicalPaths, canonical)
		compiled.Updates = append(compiled.Updates, StateUpdate{Op: update.Op, Path: formatStateUpdatePath(canonical), Value: auditValue})
		compiled.Ops = append(compiled.Ops, ops...)
		compiled.ActorOps = append(compiled.ActorOps, actorOps...)
	}

	compiled.Ops = normalizeStateOps(compiled.Ops)
	compiled.ActorOps = normalizeActorStateOps(compiled.ActorOps)
	return compiled, nil
}

func applyStateUpdateValue(field ActorStateField, current any, nestedPath []string, update StateUpdate) (any, any, error) {
	if len(nestedPath) == 0 {
		switch update.Op {
		case TurnStateUpdateReplace:
			next, err := normalizeActorStateValue(field, update.Value)
			return next, next, err
		case TurnStateUpdateDelta:
			if field.Type != "number" {
				return nil, nil, fmt.Errorf("delta 只能用于 number 字段")
			}
			currentNumber, ok := actorStateNumber(current)
			if !ok {
				return nil, nil, fmt.Errorf("delta 目标必须是已有数值，不能把缺失值当作 0")
			}
			delta, _ := actorStateNumber(update.Value)
			next, err := normalizeActorStateValue(field, currentNumber+delta)
			return next, delta, err
		}
	}
	if field.Type != "object" {
		return nil, nil, fmt.Errorf("只有 object 字段允许继续访问子路径")
	}
	root, ok := cloneStateUpdateObject(current)
	if !ok {
		if current != nil {
			return nil, nil, fmt.Errorf("目标 object 字段的当前值无效")
		}
		root = map[string]any{}
	}
	switch update.Op {
	case TurnStateUpdateReplace:
		if err := setStateUpdateNestedValue(root, nestedPath, update.Value, false); err != nil {
			return nil, nil, err
		}
		return root, update.Value, nil
	case TurnStateUpdateDelta:
		currentLeaf, found := stateUpdateNestedValue(root, nestedPath)
		if !found {
			return nil, nil, fmt.Errorf("delta 目标叶子不存在，不能把缺失值当作 0")
		}
		currentNumber, ok := actorStateNumber(currentLeaf)
		if !ok {
			return nil, nil, fmt.Errorf("delta 目标叶子必须是已有数值")
		}
		delta, _ := actorStateNumber(update.Value)
		if err := setStateUpdateNestedValue(root, nestedPath, currentNumber+delta, true); err != nil {
			return nil, nil, err
		}
		return root, delta, nil
	default:
		return nil, nil, fmt.Errorf("不支持的状态操作: %s", update.Op)
	}
}

func actorPatchFromCreateUpdate(actorID string, value any) (ActorStatePatch, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return ActorStatePatch{}, fmt.Errorf("create 的 value 必须是 object")
	}
	allowed := map[string]bool{"template_id": true, "name": true, "role": true, "description": true, "state": true}
	for key := range object {
		if !allowed[key] {
			return ActorStatePatch{}, fmt.Errorf("create 包含未知字段: %s", key)
		}
	}
	state := map[string]any(nil)
	if rawState, exists := object["state"]; exists {
		var stateOK bool
		state, stateOK = rawState.(map[string]any)
		if !stateOK {
			return ActorStatePatch{}, fmt.Errorf("create.state 必须是 object")
		}
	}
	actorName, err := actorCreateStringField(object, "name")
	if err != nil {
		return ActorStatePatch{}, err
	}
	templateID, err := actorCreateStringField(object, "template_id")
	if err != nil {
		return ActorStatePatch{}, err
	}
	role, err := actorCreateStringField(object, "role")
	if err != nil {
		return ActorStatePatch{}, err
	}
	description, err := actorCreateStringField(object, "description")
	if err != nil {
		return ActorStatePatch{}, err
	}
	return ActorStatePatch{
		ActorID:     actorID,
		ActorName:   actorName,
		TemplateID:  templateID,
		Role:        role,
		Description: description,
		State:       state,
	}, nil
}

func actorCreateStringField(object map[string]any, key string) (string, error) {
	value, exists := object[key]
	if !exists {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("create.%s 必须是 string", key)
	}
	return strings.TrimSpace(text), nil
}

func actorCreateAuditValue(patch ActorStatePatch) map[string]any {
	value := map[string]any{"template_id": patch.TemplateID}
	if patch.ActorName != "" {
		value["name"] = patch.ActorName
	}
	if patch.Role != "" {
		value["role"] = patch.Role
	}
	if patch.Description != "" {
		value["description"] = patch.Description
	}
	if len(patch.State) > 0 {
		value["state"] = patch.State
	}
	return value
}

func stateUpdateConflictsWithRuleResolution(options TurnStateUpdateCompileOptions, actorID, fieldID string) bool {
	if options.RuleResolution == nil || normalizeRuleStateConsumptionMode(options.RuleStateConsumptionMode) == RuleStateConsumptionModeDirectorOnly {
		return false
	}
	for _, change := range normalizeTurnStateChanges(options.RuleResolution.Result.StateChanges) {
		if normalizeStatePanelActorID(change.ActorID) == actorID && actorStateFieldNameKey(change.FieldID) == actorStateFieldNameKey(fieldID) {
			return true
		}
	}
	return false
}

func validateStateUpdateValueSize(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("状态值无法编码为 JSON: %w", err)
	}
	if len(data) > maxTurnStateUpdateValueBytes {
		return fmt.Errorf("状态值超过 %d bytes", maxTurnStateUpdateValueBytes)
	}
	return nil
}

func stateUpdateError(index int, code, path, expected, actual string, cause error) *StateUpdateValidationError {
	return &StateUpdateValidationError{Index: index, Code: code, Path: path, Expected: expected, Actual: actual, Cause: cause}
}

func stateUpdateExpected(field ActorStateField, nested []string, op string) string {
	if op == TurnStateUpdateDelta {
		return "existing number"
	}
	if len(nested) > 0 {
		return "value inside object field"
	}
	return field.Type
}

func stateUpdateActual(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "bool"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		if _, ok := actorStateNumber(value); ok {
			return "number"
		}
		return fmt.Sprintf("%T", value)
	}
}
