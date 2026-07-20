package interactive

import (
	"strings"
	"testing"
)

func TestPrepareTurnSubmissionLosslesslyNormalizesStringEncodedTypedValues(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "important_character",
		Fields: []ActorStateField{
			{Name: "好感度", Type: "number", Visibility: "visible"},
			{Name: "已知信息", Type: "list", Visibility: "visible"},
			{Name: "关系", Type: "object", Visibility: "visible"},
			{Name: "在场", Type: "bool", Visibility: "visible"},
			{Name: "备注", Type: "string", Visibility: "visible"},
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

func TestPrepareTurnSubmissionCanonicalizesNamedRecordFromItsID(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "world_entities",
		Fields: []ActorStateField{
			{Name: "地点记录", Type: "object", Visibility: "visible"},
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
				"清月居":{"名称":"清月居","类型":"建筑"}
			}
		}],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, input)
	if !receipt.Ready || !prepared.Ready() {
		t.Fatalf("record name should be derived from its identical ID: receipt=%#v", receipt)
	}
	updates := prepared.TurnResult().StateUpdates
	locations, ok := updates[0].Value.(map[string]any)
	if !ok {
		t.Fatalf("canonical locations = %#v, want object", updates[0].Value)
	}
	location, ok := locations["清月居"].(map[string]any)
	if !ok {
		t.Fatalf("canonical location = %#v, want object", locations["清月居"])
	}
	if location["地点名称"] != "清月居" {
		t.Fatalf("地点名称 = %#v, want 清月居", location["地点名称"])
	}
	if _, exists := location["名称"]; exists {
		t.Fatalf("generic 名称 alias should be removed after canonicalization: %#v", location)
	}
}

func TestPrepareTurnSubmissionRejectsConflictingNamedRecordAlias(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID:     "world_entities",
		Fields: []ActorStateField{{Name: "地点记录", Type: "object", Visibility: "visible"}},
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
				"清月居":{"地点名称":"清月居","名称":"别院","类型":"建筑"}
			}
		}],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	_, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, input)
	if receipt.ModuleStatus.StateChanges != TurnSubmissionModuleRejected || len(receipt.Diagnostics) != 1 {
		t.Fatalf("conflicting record aliases must remain rejected: %#v", receipt)
	}
	if !strings.Contains(receipt.Diagnostics[0].MessageZH, "别院") {
		t.Fatalf("diagnostic should identify the conflicting alias: %#v", receipt.Diagnostics[0])
	}
}

func TestPrepareTurnSubmissionCanonicalizesNamedRecordsInNewActorState(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID:     "important_character",
		Fields: []ActorStateField{{Name: "技能与能力", Type: "object", Visibility: "visible"}},
	}}}
	input := DecodeInteractiveTurnSubmissionInput(`{
		"state_changes":[{
			"op":"create",
			"actor_id":"柳寒衣",
			"template_id":"important_character",
			"name":"柳寒衣",
			"initial_state":{
				"技能与能力":{"寒冰诀":{"类型":"修炼"}}
			}
		}],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: map[string]any{}, ChoiceCount: 5,
	}, nil, input)
	if !receipt.Ready || !prepared.Ready() {
		t.Fatalf("new Actor records should derive names from their IDs: receipt=%#v", receipt)
	}
	created := prepared.TurnResult().StateUpdates[0].Value.(map[string]any)
	actorState := created["state"].(map[string]any)
	abilities := actorState["技能与能力"].(map[string]any)
	ability := abilities["寒冰诀"].(map[string]any)
	if ability["名称"] != "寒冰诀" {
		t.Fatalf("ability name = %#v, want 寒冰诀", ability["名称"])
	}
}

func TestPrepareTurnSubmissionRejectsConflictingNamedRecordInNewActorState(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID:     "important_character",
		Fields: []ActorStateField{{Name: "技能与能力", Type: "object", Visibility: "visible"}},
	}}}
	input := DecodeInteractiveTurnSubmissionInput(`{
		"state_changes":[{
			"op":"create",
			"actor_id":"柳寒衣",
			"template_id":"important_character",
			"name":"柳寒衣",
			"initial_state":{
				"技能与能力":{"寒冰诀":{"名称":"火球术","类型":"修炼"}}
			}
		}],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	_, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: map[string]any{}, ChoiceCount: 5,
	}, nil, input)
	if receipt.ModuleStatus.StateChanges != TurnSubmissionModuleRejected || len(receipt.Diagnostics) != 1 {
		t.Fatalf("conflicting new Actor record must be rejected: %#v", receipt)
	}
	if !strings.Contains(receipt.Diagnostics[0].MessageZH, "火球术") {
		t.Fatalf("diagnostic should identify the conflicting name: %#v", receipt.Diagnostics[0])
	}
}

func TestPrepareTurnSubmissionReportsAllInvalidNewActorFields(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "important_character",
		Fields: []ActorStateField{
			{Name: "好感度", Type: "number", Visibility: "visible"},
			{Name: "已知信息", Type: "list", Visibility: "visible"},
			{Name: "关系", Type: "object", Visibility: "visible"},
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
			{Name: "生命值", Type: "number", Visibility: "visible"},
			{Name: "关系", Type: "object", Visibility: "visible"},
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
			Fields: []ActorStateField{{Name: "地点记录", Type: "object", Visibility: "visible"}},
		},
		{
			ID: "important_character",
			Fields: []ActorStateField{
				{Name: "对主角好感度", Type: "number", Visibility: "visible"},
				{Name: "对主角的已知信息", Type: "list", Default: []any{}, Visibility: "visible"},
				{Name: "技能与能力", Type: "object", Default: map[string]any{}, Visibility: "visible"},
				{Name: "重要物品", Type: "object", Default: map[string]any{}, Visibility: "visible"},
				{Name: "关系", Type: "object", Default: map[string]any{}, Visibility: "visible"},
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
		if location["地点名称"] != id {
			t.Fatalf("location %q canonical name = %#v", id, location["地点名称"])
		}
	}
	created := updates[1].Value.(map[string]any)
	actorState := created["state"].(map[string]any)
	if actorState["对主角好感度"] != float64(0) {
		t.Fatalf("canonical favorability = %#v, want numeric zero", actorState["对主角好感度"])
	}
}
