package interactive

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
)

const turnSubmissionStateChangesField = "state_changes"

// TurnStateChangeInput is the model-facing state mutation shape. Stable IDs
// are separate fields so the model never has to construct or escape a JSON
// Pointer; the backend compiles this shape to the persisted StateUpdate.
type TurnStateChangeInput struct {
	Op           string         `json:"op" jsonschema:"enum=replace,enum=delta,enum=create" jsonschema_description:"replace 写入字段完整新值，delta 增减已有数值，create 创建新的 Actor。"`
	ActorID      string         `json:"actor_id" jsonschema_description:"Actor 状态手册中反引号标记的稳定 Actor ID；create 时填写新的稳定 ASCII ID。"`
	FieldID      string         `json:"field_id,omitempty" jsonschema_description:"replace/delta 必填，逐字使用 Actor 状态手册中的字段 ID。"`
	Subpath      []string       `json:"subpath,omitempty" jsonschema_description:"仅 object 字段的嵌套更新使用；按层级填写字符串段，不要自行拼接路径字符串。"`
	Value        any            `json:"value,omitempty" jsonschema_description:"replace 的完整新值或 delta 的数值变化量；类型必须匹配字段说明。"`
	TemplateID   string         `json:"template_id,omitempty" jsonschema_description:"仅 create 必填，逐字使用新 Actor 可用模板中的 Template ID。"`
	Name         string         `json:"name,omitempty" jsonschema_description:"仅 create 使用的角色展示名称。"`
	Role         string         `json:"role,omitempty" jsonschema_description:"仅 create 使用的角色定位。"`
	Description  string         `json:"description,omitempty" jsonschema_description:"仅 create 使用的简短角色说明。"`
	InitialState map[string]any `json:"initial_state,omitempty" jsonschema_description:"仅 create 使用；key 必须是所选模板的精确字段 ID。"`
}

// DecodeInteractiveTurnSubmissionInput independently decodes state_changes
// and choices from one model-facing tool call. A malformed module does not
// discard a valid sibling module, and later calls may provide only retry_modules.
func DecodeInteractiveTurnSubmissionInput(arguments string) TurnSubmissionInput {
	if len([]byte(arguments)) > maxTurnSubmissionArgumentsBytes {
		return invalidUnifiedTurnSubmissionInput("submission_too_large", "", fmt.Sprintf("%d bytes", len([]byte(arguments))), fmt.Sprintf("工具参数超过 %d bytes", maxTurnSubmissionArgumentsBytes), fmt.Sprintf("Tool arguments exceed %d bytes.", maxTurnSubmissionArgumentsBytes))
	}
	var root map[string]json.RawMessage
	if err := decodeStrictJSON([]byte(arguments), &root, false); err != nil {
		return invalidUnifiedTurnSubmissionInput(TurnSubmissionDiagnosticInvalidJSON, "", "invalid JSON", fmt.Sprintf("回合提交参数不是有效 JSON：%v", err), fmt.Sprintf("Turn submission arguments are not valid JSON: %v", err))
	}
	if root == nil {
		return invalidUnifiedTurnSubmissionInput(TurnSubmissionDiagnosticInvalidTopLevel, "", "null", "回合提交参数必须是 object", "Turn submission arguments must be an object.")
	}
	allowed := map[string]bool{
		turnSubmissionStateChangesField:   true,
		TurnSubmissionModuleChoices:       true,
		turnSubmissionDirectorUpdateField: true,
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
			"回合提交参数只能包含 state_changes、choices 和可选 director_update",
			"Turn submission arguments may only contain state_changes, choices, and optional director_update.",
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
		if rawHint, hintExists := root[turnSubmissionDirectorUpdateField]; hintExists {
			hint, hintDiagnostics := decodeDirectorUpdateHint(rawHint)
			input.Diagnostics = append(input.Diagnostics, hintDiagnostics...)
			if len(hintDiagnostics) == 0 {
				input.DirectorUpdate = hint
			}
		}
		if !turnSubmissionHasDiagnostic(input.Diagnostics, TurnSubmissionModuleChoices) {
			input.Choices = &choices
		}
	} else if _, hintExists := root[turnSubmissionDirectorUpdateField]; hintExists {
		input.Diagnostics = append(input.Diagnostics, *newTurnSubmissionDiagnostic(
			TurnSubmissionModuleChoices,
			nil,
			TurnSubmissionDiagnosticInvalidTopLevel,
			"/director_update",
			"director_update together with choices",
			"choices missing",
			"director_update 只能与 choices 在同一次模块提交中提供",
			"director_update may only be submitted together with choices.",
		))
	}
	return input
}

func decodeStructuredStateChangesModule(raw json.RawMessage) ([]StateUpdate, []TurnSubmissionDiagnostic) {
	items, err := decodeStructuredStateChangeItems(raw)
	if err != nil {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModuleStateChanges,
			nil,
			TurnSubmissionDiagnosticInvalidModule,
			"/state_changes",
			"array",
			jsonValueKind(raw),
			fmt.Sprintf("state_changes 必须是原生数组；仅兼容一层包含合法数组 JSON 的字符串：%v", err),
			fmt.Sprintf("state_changes must be a native array; only one string layer containing valid array JSON is tolerated: %v", err),
		)}
	}
	if len(items) > maxInteractiveListItems {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(
			TurnSubmissionModuleStateChanges,
			nil,
			"too_many_state_updates",
			"/state_changes",
			fmt.Sprintf("at most %d operations", maxInteractiveListItems),
			fmt.Sprintf("%d operations", len(items)),
			fmt.Sprintf("state_changes 不能超过 %d 项", maxInteractiveListItems),
			fmt.Sprintf("state_changes cannot exceed %d operations.", maxInteractiveListItems),
		)}
	}
	updates := make([]StateUpdate, 0, len(items))
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
				fmt.Sprintf("状态变化结构无效：%v", err),
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
				"valid replace, delta, or create fields",
				"invalid state change",
				err.Error(),
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
	log.Printf("[interactive-turn-submission] accepted one-layer string-encoded state_changes bytes=%d location=internal/interactive/turn_submission_decode.go", len(encoded))
	return items, nil
}

func stateUpdateFromStructuredInput(change TurnStateChangeInput) (StateUpdate, error) {
	change.Op = strings.ToLower(strings.TrimSpace(change.Op))
	change.ActorID = strings.TrimSpace(change.ActorID)
	change.FieldID = strings.TrimSpace(change.FieldID)
	change.TemplateID = strings.TrimSpace(change.TemplateID)
	if change.ActorID == "" {
		return StateUpdate{}, fmt.Errorf("state_changes 缺少 actor_id")
	}
	switch change.Op {
	case TurnStateUpdateReplace, TurnStateUpdateDelta:
		if change.FieldID == "" {
			return StateUpdate{}, fmt.Errorf("%s 状态变化缺少 field_id", change.Op)
		}
		if change.Value == nil {
			return StateUpdate{}, fmt.Errorf("%s 状态变化缺少非空 value", change.Op)
		}
		if change.TemplateID != "" || change.Name != "" || change.Role != "" || change.Description != "" || change.InitialState != nil {
			return StateUpdate{}, fmt.Errorf("%s 不能包含 create 专用字段", change.Op)
		}
		segments := []string{change.ActorID, change.FieldID}
		for _, segment := range change.Subpath {
			segment = strings.TrimSpace(segment)
			if segment == "" {
				return StateUpdate{}, fmt.Errorf("subpath 不能包含空段")
			}
			segments = append(segments, segment)
		}
		return StateUpdate{Op: change.Op, Path: formatStateUpdatePath(segments), Value: change.Value}, nil
	case TurnStateUpdateCreate:
		if change.TemplateID == "" {
			return StateUpdate{}, fmt.Errorf("create 状态变化缺少 template_id")
		}
		if change.FieldID != "" || len(change.Subpath) > 0 || change.Value != nil {
			return StateUpdate{}, fmt.Errorf("create 不能包含 field_id、subpath 或 value")
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
		return StateUpdate{Op: TurnStateUpdateCreate, Path: formatStateUpdatePath([]string{change.ActorID}), Value: value}, nil
	default:
		return StateUpdate{}, fmt.Errorf("op 必须是 replace、delta 或 create")
	}
}

func invalidUnifiedTurnSubmissionInput(code, path, actual, messageZH, messageEN string) TurnSubmissionInput {
	diagnostics := make([]TurnSubmissionDiagnostic, 0, 2)
	for _, module := range []string{TurnSubmissionModuleStateChanges, TurnSubmissionModuleChoices} {
		diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(
			module,
			nil,
			code,
			path,
			"object containing state_changes and/or choices",
			actual,
			messageZH,
			messageEN,
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
