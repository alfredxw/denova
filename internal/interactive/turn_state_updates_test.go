package interactive

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestCompileTurnStateUpdatesSupportsNestedReplaceAndDelta(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist",
		Fields: []ActorStateField{
			{Name: "行踪", Type: "object"},
			{Name: "好感度", Type: "number"},
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
		ID: "protagonist", Fields: []ActorStateField{{Name: "行踪", Type: "object"}},
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
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{ID: "protagonist", Fields: []ActorStateField{{Name: fieldID, Type: "string"}}}}}
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

func TestCompileTurnStateUpdatesWritesHistoricalHiddenField(t *testing.T) {
	var system StoryDirectorActorStateSystem
	if err := json.Unmarshal([]byte(`{"templates":[{"id":"protagonist","fields":[{"name":"秘密身份","type":"string","visibility":"hidden"}]}]}`), &system); err != nil {
		t.Fatalf("decode historical hidden field: %v", err)
	}
	state := map[string]any{"actors": map[string]any{"protagonist": map[string]any{
		"id": "protagonist", "template_id": "protagonist", "state": map[string]any{"秘密身份": "未知"},
	}}}

	compiled, err := CompileTurnStateUpdates(system, state, []StateUpdate{{
		Op: TurnStateUpdateReplace, Path: "/protagonist/秘密身份", Value: "宗门少主",
	}}, TurnStateUpdateCompileOptions{})
	if err != nil {
		t.Fatalf("historical hidden fields should remain writable: %v", err)
	}
	if len(compiled.Updates) != 1 || compiled.Updates[0].Value != "宗门少主" {
		t.Fatalf("hidden field update was not preserved: %#v", compiled.Updates)
	}
}

func TestCompileTurnStateUpdatesCreatesNamedActorWithoutInventingOtherOptionalStrings(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "opponent", Fields: []ActorStateField{{Name: "生命值", Type: "number"}},
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

func TestCompileTurnStateUpdatesArchivesActorWithoutDeletingState(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{
		{ID: "story_context", Fields: []ActorStateField{{Name: "在场角色", LegacyPath: "scene.present_actors", Type: "list"}}},
		{ID: "opponent", Fields: []ActorStateField{{Name: "生命值", Type: "number"}}},
	}}
	state := map[string]any{
		actorStateRoot: map[string]any{
			"story": map[string]any{"id": "story", "template_id": "story_context", "state": map[string]any{"在场角色": []any{"protagonist", "狼王"}}},
			"狼王":    map[string]any{"id": "狼王", "name": "狼王", "template_id": "opponent", "state": map[string]any{"生命值": float64(4)}},
		},
	}
	compiled, err := CompileTurnStateUpdates(system, state, []StateUpdate{
		{Op: TurnStateUpdateArchive, Path: "/狼王", Value: map[string]any{"reason": "本回合已确认死亡"}},
		{Op: TurnStateUpdateDelta, Path: "/狼王/生命值", Value: -4},
	}, TurnStateUpdateCompileOptions{SourceTurnID: "turn-death"})
	if err != nil {
		t.Fatal(err)
	}
	working := cloneActorStateRoot(state)
	for _, op := range compiled.Ops {
		applyStateOp(working, op)
	}
	for _, op := range compiled.ActorOps {
		applyActorStateOp(working, op)
	}
	if getPath(working, actorStateRoot+".狼王") == nil {
		t.Fatalf("archive must retain the complete Actor record: %#v", working)
	}
	if got := actorStateFieldValue(working, "狼王", "生命值"); got != float64(0) {
		t.Fatalf("final same-turn updates must apply before the Actor becomes inactive, got %#v", got)
	}
	archive, ok := actorArchiveRecordFromState(working, "狼王")
	if !ok || archive.Reason != "本回合已确认死亡" || archive.SourceTurnID != "turn-death" {
		t.Fatalf("archive provenance was not persisted: %#v", working[actorArchiveRoot])
	}
	present := actorStateFieldValue(working, "story", "在场角色")
	if values, ok := present.([]any); !ok || len(values) != 1 || values[0] != "protagonist" {
		t.Fatalf("archived Actor should leave the standard present_actors field: %#v", present)
	}
}

func TestCompileTurnStateUpdatesRestoresActorAndMakesBatchOrderIndependent(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "opponent", Fields: []ActorStateField{{Name: "存续", Type: "string"}},
	}}}
	state := map[string]any{
		actorStateRoot: map[string]any{
			"狼王": map[string]any{"id": "狼王", "name": "狼王", "template_id": "opponent", "state": map[string]any{"存续": "死亡"}},
		},
		actorArchiveRoot: map[string]any{
			"狼王": map[string]any{"reason": "此前确认死亡", "source_turn_id": "turn-death"},
		},
	}
	compiled, err := CompileTurnStateUpdates(system, state, []StateUpdate{
		{Op: TurnStateUpdateReplace, Path: "/狼王/存续", Value: "重伤幸存"},
		{Op: TurnStateUpdateRestore, Path: "/狼王", Value: map[string]any{"reason": "发现此前死亡判断有误"}},
	}, TurnStateUpdateCompileOptions{SourceTurnID: "turn-return"})
	if err != nil {
		t.Fatal(err)
	}
	working := cloneActorStateRoot(state)
	for _, op := range compiled.Ops {
		applyStateOp(working, op)
	}
	for _, op := range compiled.ActorOps {
		applyActorStateOp(working, op)
	}
	if _, archived := actorArchiveRecordFromState(working, "狼王"); archived {
		t.Fatalf("restore should remove only the archive marker: %#v", working[actorArchiveRoot])
	}
	if got := actorStateFieldValue(working, "狼王", "存续"); got != "重伤幸存" {
		t.Fatalf("restored Actor update should be applied in the same batch, got %#v", got)
	}
}

func TestCompileTurnStateUpdatesRejectsWritesToArchivedAndProtectedActors(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{ID: "opponent", Fields: []ActorStateField{{Name: "生命值", Type: "number"}}}}}
	state := map[string]any{
		actorStateRoot:   map[string]any{"狼王": map[string]any{"id": "狼王", "template_id": "opponent", "state": map[string]any{"生命值": float64(0)}}},
		actorArchiveRoot: map[string]any{"狼王": map[string]any{"reason": "死亡", "source_turn_id": "turn-death"}},
	}
	_, err := CompileTurnStateUpdates(system, state, []StateUpdate{{Op: TurnStateUpdateReplace, Path: "/狼王/生命值", Value: 1}}, TurnStateUpdateCompileOptions{})
	var validationError *StateUpdateValidationError
	if !errors.As(err, &validationError) || validationError.Code != "actor_archived" {
		t.Fatalf("archived Actors must be read-only until explicitly restored, got %v", err)
	}

	_, err = CompileTurnStateUpdates(system, state, []StateUpdate{{Op: TurnStateUpdateArchive, Path: "/protagonist", Value: map[string]any{"reason": "永久退场"}}}, TurnStateUpdateCompileOptions{})
	if !errors.As(err, &validationError) || validationError.Code != "protected_actor_archive" {
		t.Fatalf("system Actors must not be archived, got %v", err)
	}
}
