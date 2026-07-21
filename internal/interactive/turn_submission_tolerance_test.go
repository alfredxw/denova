package interactive

import (
	"reflect"
	"strings"
	"testing"
)

func TestPrepareTurnSubmissionLosslesslyNormalizesStringEncodedTypedValues(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "important_character",
		Fields: []ActorStateField{
			{Name: "好感度", Type: "number"},
			{Name: "已知信息", Type: "list"},
			{Name: "关系", Type: "object"},
			{Name: "在场", Type: "bool"},
			{Name: "备注", Type: "string"},
		},
	}}}
	input := DecodeInteractiveTurnSubmissionInput(`{
		"state_changes":[{
			"op":"create",
			"actor_id":"柳寒衣",
			"template_id":"important_character",
			"name":"柳寒衣",
			"initial_state":{
				"好感度":"0",
				"已知信息":"[]",
				"关系":"{}",
				"在场":"false",
				"备注":"{}"
			}
		}],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: map[string]any{}, ChoiceCount: 5,
	}, nil, input)
	if !receipt.Ready || !prepared.Ready() {
		t.Fatalf("losslessly encoded values should be accepted: receipt=%#v", receipt)
	}
	updates := prepared.TurnResult().StateUpdates
	if len(updates) != 1 {
		t.Fatalf("state update count = %d, want 1", len(updates))
	}
	created, ok := updates[0].Value.(map[string]any)
	if !ok {
		t.Fatalf("create audit value = %#v, want object", updates[0].Value)
	}
	state, ok := created["state"].(map[string]any)
	if !ok {
		t.Fatalf("create state = %#v, want object", created["state"])
	}
	if state["好感度"] != float64(0) {
		t.Fatalf("好感度 = %#v, want numeric zero", state["好感度"])
	}
	if known, ok := state["已知信息"].([]any); !ok || len(known) != 0 {
		t.Fatalf("已知信息 = %#v, want empty list", state["已知信息"])
	}
	if relations, ok := state["关系"].(map[string]any); !ok || len(relations) != 0 {
		t.Fatalf("关系 = %#v, want empty object", state["关系"])
	}
	if state["在场"] != false {
		t.Fatalf("在场 = %#v, want false", state["在场"])
	}
	if state["备注"] != "{}" {
		t.Fatalf("string fields must remain strings, got %#v", state["备注"])
	}
}

func TestPrepareTurnSubmissionLosslesslyNormalizesExistingActorReplacement(t *testing.T) {
	system, state := turnSubmissionTestState()
	input := DecodeInteractiveTurnSubmissionInput(`{
		"state_changes":[{
			"op":"replace",
			"actor_id":"protagonist",
			"field_id":"生命值",
			"value":"12"
		}],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, input)
	if !receipt.Ready || !prepared.Ready() {
		t.Fatalf("losslessly encoded replacement should be accepted: receipt=%#v", receipt)
	}
	updates := prepared.TurnResult().StateUpdates
	if len(updates) != 1 || updates[0].Value != float64(12) {
		t.Fatalf("canonical replacement = %#v, want numeric 12", updates)
	}
}

func TestPrepareTurnSubmissionLosslesslyNormalizesNumericDelta(t *testing.T) {
	system, state := turnSubmissionTestState()
	input := DecodeInteractiveTurnSubmissionInput(`{
		"state_changes":[{
			"op":"delta",
			"actor_id":"protagonist",
			"field_id":"生命值",
			"value":"2"
		}],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, input)
	if !receipt.Ready || !prepared.Ready() {
		t.Fatalf("losslessly encoded delta should be accepted: receipt=%#v", receipt)
	}
	updates := prepared.TurnResult().StateUpdates
	if len(updates) != 1 || updates[0].Value != float64(2) {
		t.Fatalf("canonical delta = %#v, want numeric 2", updates)
	}
}

func TestCompileTurnStateUpdatesLosslesslyNormalizesNumericDelta(t *testing.T) {
	system, state := turnSubmissionTestState()
	compiled, err := CompileTurnStateUpdates(system, state, []StateUpdate{{
		Op: TurnStateUpdateDelta, Path: "/protagonist/生命值", Value: "2",
	}}, TurnStateUpdateCompileOptions{})
	if err != nil {
		t.Fatalf("the model-to-state compiler should accept an unambiguous numeric string: %v", err)
	}
	if len(compiled.Updates) != 1 || compiled.Updates[0].Value != float64(2) {
		t.Fatalf("canonical delta = %#v, want numeric 2", compiled.Updates)
	}
}

func TestPrepareTurnSubmissionPreservesNamedRecordValues(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "world_entities",
		Fields: []ActorStateField{
			{Name: "地点记录", Type: "object"},
		},
	}}}
	state := map[string]any{"actors": map[string]any{
		"world": map[string]any{
			"id": "world", "template_id": "world_entities",
			"state": map[string]any{"地点记录": map[string]any{}},
		},
	}}
	input := DecodeInteractiveTurnSubmissionInput(`{
		"state_changes":[{
			"op":"replace",
			"actor_id":"world",
			"field_id":"地点记录",
			"value":{
				"清月居":"建筑",
				"旧驿站":{"地点名称":"驿站旧称","名称":"legacy alias","类型":"设施"},
				"无名洞穴":{"类型":"地点"}
			}
		}],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, input)
	if !receipt.Ready || !prepared.Ready() {
		t.Fatalf("named records should accept and preserve arbitrary child values: receipt=%#v", receipt)
	}
	updates := prepared.TurnResult().StateUpdates
	locations, ok := updates[0].Value.(map[string]any)
	if !ok {
		t.Fatalf("locations = %#v, want root object", updates[0].Value)
	}
	want := map[string]any{
		"清月居": "建筑",
		"旧驿站": map[string]any{
			"地点名称": "驿站旧称",
			"名称":   "legacy alias",
			"类型":   "设施",
		},
		"无名洞穴": map[string]any{"类型": "地点"},
	}
	if !reflect.DeepEqual(locations, want) {
		t.Fatalf("named record values changed: got %#v, want %#v", locations, want)
	}
}

func TestPrepareTurnSubmissionRequiresNamedRecordRootObject(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID:     "world_entities",
		Fields: []ActorStateField{{Name: "地点记录", Type: "object"}},
	}}}
	state := map[string]any{"actors": map[string]any{
		"world": map[string]any{
			"id": "world", "template_id": "world_entities",
			"state": map[string]any{"地点记录": map[string]any{}},
		},
	}}
	input := DecodeInteractiveTurnSubmissionInput(`{
		"state_changes":[{
			"op":"replace",
			"actor_id":"world",
			"field_id":"地点记录",
			"value":"清月居"
		}],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	_, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, input)
	if receipt.ModuleStatus.StateChanges != TurnSubmissionModuleRejected || len(receipt.Diagnostics) != 1 {
		t.Fatalf("an object-typed named record field must reject a scalar root: %#v", receipt)
	}
	if receipt.Diagnostics[0].Expected != "object" || receipt.Diagnostics[0].Actual != "string" {
		t.Fatalf("root type diagnostic should use generic object validation: %#v", receipt.Diagnostics[0])
	}
}

func TestPrepareTurnSubmissionPreservesNamedRecordsInNewActorState(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID:     "important_character",
		Fields: []ActorStateField{{Name: "技能与能力", Type: "object"}},
	}}}
	input := DecodeInteractiveTurnSubmissionInput(`{
		"state_changes":[{
			"op":"create",
			"actor_id":"柳寒衣",
			"template_id":"important_character",
			"name":"柳寒衣",
			"initial_state":{
				"技能与能力":{
					"寒冰诀":{"名称":"旧称冰心诀","类型":"修炼"},
					"洞察":"被动"
				}
			}
		}],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: map[string]any{}, ChoiceCount: 5,
	}, nil, input)
	if !receipt.Ready || !prepared.Ready() {
		t.Fatalf("new Actor named records should preserve arbitrary child values: receipt=%#v", receipt)
	}
	created := prepared.TurnResult().StateUpdates[0].Value.(map[string]any)
	actorState := created["state"].(map[string]any)
	abilities := actorState["技能与能力"].(map[string]any)
	want := map[string]any{
		"寒冰诀": map[string]any{"名称": "旧称冰心诀", "类型": "修炼"},
		"洞察":  "被动",
	}
	if !reflect.DeepEqual(abilities, want) {
		t.Fatalf("new Actor named record values changed: got %#v, want %#v", abilities, want)
	}
}

func TestPrepareTurnSubmissionReportsAllInvalidNewActorFields(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "important_character",
		Fields: []ActorStateField{
			{Name: "好感度", Type: "number"},
			{Name: "已知信息", Type: "list"},
			{Name: "关系", Type: "object"},
		},
	}}}
	input := DecodeInteractiveTurnSubmissionInput(`{
		"state_changes":[{
			"op":"create",
			"actor_id":"柳寒衣",
			"template_id":"important_character",
			"name":"柳寒衣",
			"initial_state":{
				"好感度":"未知",
				"已知信息":"尚未设置",
				"关系":"稍后补充"
			}
		}],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	_, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: map[string]any{}, ChoiceCount: 5,
	}, nil, input)
	if receipt.ModuleStatus.StateChanges != TurnSubmissionModuleRejected || receipt.ModuleStatus.Choices != TurnSubmissionModuleAccepted {
		t.Fatalf("invalid create should reject only state changes: %#v", receipt)
	}
	if len(receipt.Diagnostics) != 3 {
		t.Fatalf("diagnostic count = %d, want 3: %#v", len(receipt.Diagnostics), receipt.Diagnostics)
	}
	wantPaths := map[string]bool{
		"/state_changes/0/initial_state/好感度":  false,
		"/state_changes/0/initial_state/已知信息": false,
		"/state_changes/0/initial_state/关系":   false,
	}
	for _, diagnostic := range receipt.Diagnostics {
		if _, exists := wantPaths[diagnostic.Path]; !exists {
			t.Fatalf("unexpected diagnostic path %q: %#v", diagnostic.Path, diagnostic)
		}
		wantPaths[diagnostic.Path] = true
		if diagnostic.Expected == "" || diagnostic.Actual != "string" {
			t.Fatalf("diagnostic should expose expected and actual types: %#v", diagnostic)
		}
		if !strings.Contains(diagnostic.MessageEN, diagnostic.Expected) || !strings.Contains(diagnostic.MessageEN, diagnostic.Actual) {
			t.Fatalf("English diagnostic should explain the expected and actual types: %#v", diagnostic)
		}
	}
	for path, found := range wantPaths {
		if !found {
			t.Fatalf("missing diagnostic path %q: %#v", path, receipt.Diagnostics)
		}
	}
}

func TestPrepareTurnSubmissionReportsIndependentInvalidOperationsTogether(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist",
		Fields: []ActorStateField{
			{Name: "生命值", Type: "number"},
			{Name: "关系", Type: "object"},
		},
	}}}
	state := map[string]any{"actors": map[string]any{
		"protagonist": map[string]any{
			"id": "protagonist", "template_id": "protagonist",
			"state": map[string]any{"生命值": float64(10), "关系": map[string]any{}},
		},
	}}
	input := DecodeInteractiveTurnSubmissionInput(`{
		"state_changes":[
			{"op":"replace","actor_id":"protagonist","field_id":"生命值","value":"很多"},
			{"op":"replace","actor_id":"protagonist","field_id":"关系","value":"稍后补充"}
		],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	_, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, input)
	if receipt.ModuleStatus.StateChanges != TurnSubmissionModuleRejected || len(receipt.Diagnostics) != 2 {
		t.Fatalf("independent invalid operations should be reported together: %#v", receipt)
	}
	if receipt.Diagnostics[0].Path != "/state_changes/0" || receipt.Diagnostics[1].Path != "/state_changes/1" {
		t.Fatalf("diagnostic paths = %#v, want both operation indexes", receipt.Diagnostics)
	}
}

func TestPrepareTurnSubmissionAcceptsLosslessWeakModelPayloadInOneCall(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{
		{
			ID:     "world_entities",
			Fields: []ActorStateField{{Name: "地点记录", Type: "object"}},
		},
		{
			ID: "important_character",
			Fields: []ActorStateField{
				{Name: "对主角好感度", Type: "number"},
				{Name: "对主角的已知信息", Type: "list", Default: []any{}},
				{Name: "技能与能力", Type: "object", Default: map[string]any{}},
				{Name: "重要物品", Type: "object", Default: map[string]any{}},
				{Name: "关系", Type: "object", Default: map[string]any{}},
			},
		},
	}}
	state := map[string]any{"actors": map[string]any{
		"world": map[string]any{
			"id": "world", "template_id": "world_entities",
			"state": map[string]any{"地点记录": map[string]any{}},
		},
	}}
	input := DecodeInteractiveTurnSubmissionInput(`{
		"state_changes":[
			{
				"op":"replace",
				"actor_id":"world",
				"field_id":"地点记录",
				"value":{
					"清月居":{"名称":"清月居","类型":"建筑"},
					"青云宗外门杂役处":{"名称":"青云宗外门杂役处","类型":"设施"}
				}
			},
			{
				"op":"create",
				"actor_id":"柳寒衣",
				"template_id":"important_character",
				"name":"柳寒衣",
				"initial_state":{
					"对主角好感度":"0",
					"对主角的已知信息":"[]",
					"技能与能力":"{}",
					"重要物品":"{}",
					"关系":"{}"
				}
			}
		],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, input)
	if !receipt.Ready || !prepared.Ready() || len(receipt.Diagnostics) != 0 {
		t.Fatalf("lossless weak-model payload should settle in one call: %#v", receipt)
	}
	updates := prepared.TurnResult().StateUpdates
	if len(updates) != 2 {
		t.Fatalf("canonical state update count = %d, want 2", len(updates))
	}
	locations := updates[0].Value.(map[string]any)
	for id, raw := range locations {
		location := raw.(map[string]any)
		if location["名称"] != id {
			t.Fatalf("location %q legacy name changed: %#v", id, location)
		}
		if _, exists := location["地点名称"]; exists {
			t.Fatalf("location %q gained a derived 地点名称: %#v", id, location)
		}
	}
	created := updates[1].Value.(map[string]any)
	actorState := created["state"].(map[string]any)
	if actorState["对主角好感度"] != float64(0) {
		t.Fatalf("canonical favorability = %#v, want numeric zero", actorState["对主角好感度"])
	}
}
