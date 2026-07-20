package interactive

import (
	"errors"
	"testing"
)

func TestCompileTurnStateUpdatesSupportsNestedReplaceAndDelta(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist",
		Fields: []ActorStateField{
			{Name: "行踪", Type: "object", Visibility: "visible"},
			{Name: "好感度", Type: "number", Visibility: "visible"},
		},
	}}}
	state := map[string]any{"actors": map[string]any{"protagonist": map[string]any{
		"id": "protagonist", "template_id": "protagonist",
		"state": map[string]any{"行踪": map[string]any{"当前区域": "月映湖"}, "好感度": float64(3)},
	}}}

	compiled, err := CompileTurnStateUpdates(system, state, []StateUpdate{
		{Op: TurnStateUpdateReplace, Path: "/protagonist/行踪/当前区域", Value: "东苍腹地"},
		{Op: TurnStateUpdateDelta, Path: "/protagonist/好感度", Value: 2},
	}, TurnStateUpdateCompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Updates) != 2 || len(compiled.ActorOps) != 2 {
		t.Fatalf("unexpected compiled operations: %#v", compiled)
	}
	working := cloneActorStateRoot(state)
	for _, op := range compiled.ActorOps {
		applyActorStateOp(working, op)
	}
	location, _ := actorStateFieldValue(working, "protagonist", "行踪").(map[string]any)
	if location["当前区域"] != "东苍腹地" || actorStateFieldValue(working, "protagonist", "好感度") != float64(5) {
		t.Fatalf("compiled operations produced wrong state: %#v", working)
	}
}

func TestCompileTurnStateUpdatesRejectsMissingDeltaTargetAndOverlappingPaths(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Fields: []ActorStateField{{Name: "行踪", Type: "object", Visibility: "visible"}},
	}}}
	state := map[string]any{"actors": map[string]any{"protagonist": map[string]any{
		"id": "protagonist", "template_id": "protagonist", "state": map[string]any{"行踪": map[string]any{}},
	}}}
	_, err := CompileTurnStateUpdates(system, state, []StateUpdate{{Op: TurnStateUpdateDelta, Path: "/protagonist/行踪/危险度", Value: 1}}, TurnStateUpdateCompileOptions{})
	var validationError *StateUpdateValidationError
	if !errors.As(err, &validationError) || validationError.Code != "delta_target_not_number" {
		t.Fatalf("missing delta target should be explicit, got %v", err)
	}

	_, err = CompileTurnStateUpdates(system, state, []StateUpdate{
		{Op: TurnStateUpdateReplace, Path: "/protagonist/行踪", Value: map[string]any{"当前区域": "东苍"}},
		{Op: TurnStateUpdateReplace, Path: "/protagonist/行踪/危险度", Value: 30},
	}, TurnStateUpdateCompileOptions{})
	if !errors.As(err, &validationError) || validationError.Code != "overlapping_state_path" {
		t.Fatalf("overlapping paths should be rejected, got %v", err)
	}
}

func TestCompileTurnStateUpdatesUsesEscapedTildeInFieldIDs(t *testing.T) {
	fieldID := "精神~状态"
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{ID: "protagonist", Fields: []ActorStateField{{Name: fieldID, Type: "string", Visibility: "visible"}}}}}
	state := map[string]any{"actors": map[string]any{"protagonist": map[string]any{"id": "protagonist", "template_id": "protagonist", "state": map[string]any{fieldID: "动摇"}}}}
	path := formatStateUpdatePath([]string{"protagonist", fieldID})
	compiled, err := CompileTurnStateUpdates(system, state, []StateUpdate{{Op: TurnStateUpdateReplace, Path: path, Value: "镇定"}}, TurnStateUpdateCompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Updates) != 1 || compiled.Updates[0].Path != path {
		t.Fatalf("escaped canonical path was not preserved: %#v", compiled.Updates)
	}
}

func TestCompileTurnStateUpdatesCreatesNamedActorWithoutInventingOtherOptionalStrings(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "opponent", Fields: []ActorStateField{{Name: "生命值", Type: "number", Visibility: "visible"}},
	}}}
	compiled, err := CompileTurnStateUpdates(system, map[string]any{}, []StateUpdate{{
		Op: TurnStateUpdateCreate, Path: "/狼王",
		Value: map[string]any{"template_id": "opponent", "name": "狼王", "state": map[string]any{"生命值": 12}},
	}}, TurnStateUpdateCompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Updates) != 1 || len(compiled.ActorOps) == 0 {
		t.Fatalf("actor create was not compiled: %#v", compiled)
	}
	audit, ok := compiled.Updates[0].Value.(map[string]any)
	if !ok || audit["template_id"] != "opponent" {
		t.Fatalf("unexpected create audit value: %#v", compiled.Updates[0].Value)
	}
	if audit["name"] != "狼王" {
		t.Fatalf("actor name should be identical to its ID: %#v", audit)
	}
	for _, key := range []string{"role", "description"} {
		if _, exists := audit[key]; exists {
			t.Fatalf("missing optional field %q must not become an invented string: %#v", key, audit)
		}
	}

	_, err = CompileTurnStateUpdates(system, map[string]any{}, []StateUpdate{{
		Op: TurnStateUpdateCreate, Path: "/狼群首领",
		Value: map[string]any{"template_id": "opponent", "name": 2},
	}}, TurnStateUpdateCompileOptions{})
	var validationError *StateUpdateValidationError
	if !errors.As(err, &validationError) || validationError.Code != "invalid_actor_create" {
		t.Fatalf("non-string actor metadata should be rejected precisely, got %v", err)
	}
}

func TestCompileTurnStateUpdatesRejectsRuleResolutionDuplicate(t *testing.T) {
	system, state := turnSubmissionTestState()
	resolution := RuleResolution{Result: RuleResult{StateChanges: []TurnStateChange{{ActorID: "protagonist", FieldID: "生命值", Change: -1}}}}
	_, err := CompileTurnStateUpdates(system, state, []StateUpdate{{
		Op: TurnStateUpdateDelta, Path: "/protagonist/生命值", Value: -1,
	}}, TurnStateUpdateCompileOptions{RuleResolution: &resolution, RuleStateConsumptionMode: RuleStateConsumptionModeHybridAuto})
	var validationError *StateUpdateValidationError
	if !errors.As(err, &validationError) || validationError.Code != "duplicate_rule_state_update" {
		t.Fatalf("RuleResolution duplicate should be rejected, got %v", err)
	}
}
