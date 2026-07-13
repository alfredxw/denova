package interactive

import "testing"

func TestPrepareTurnSubmissionDropsUnknownActorStateFields(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist",
		Fields: []ActorStateField{
			{Name: "当前处境", Type: "string"},
			{Name: "生命值", Type: "number"},
		},
	}}}
	state := map[string]any{"actors": map[string]any{
		"protagonist": map[string]any{
			"id":          "protagonist",
			"template_id": "protagonist",
			"state":       map[string]any{"当前处境": "林地", "生命值": float64(10)},
		},
	}}
	result := validTurnSubmissionResult()
	result.ActorStatePatches = []ActorStatePatch{{
		ActorID: "protagonist",
		State: map[string]any{
			"当前处境":   "废弃哨站",
			"当前详细地点": "哨站二层",
		},
	}}

	prepared, receipt := PrepareTurnSubmission(system, state, result)
	if prepared == nil || !receipt.Accepted || receipt.Retryable {
		t.Fatalf("submission should be accepted with warnings: prepared=%#v receipt=%#v", prepared, receipt)
	}
	if len(receipt.Diagnostics) != 1 || receipt.Diagnostics[0].Code != TurnSubmissionDiagnosticUnknownActorStateField {
		t.Fatalf("unexpected diagnostics: %#v", receipt.Diagnostics)
	}
	got := prepared.TurnResult()
	if len(got.ActorStatePatches) != 1 {
		t.Fatalf("expected one retained patch: %#v", got.ActorStatePatches)
	}
	if _, ok := got.ActorStatePatches[0].State["当前详细地点"]; ok {
		t.Fatalf("unknown field survived normalization: %#v", got.ActorStatePatches[0].State)
	}
	if got.ActorStatePatches[0].State["当前处境"] != "废弃哨站" {
		t.Fatalf("known field was not retained: %#v", got.ActorStatePatches[0].State)
	}
}

func TestPrepareTurnSubmissionDropsPatchWhenOnlyUnknownFieldsRemain(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID:     "protagonist",
		Fields: []ActorStateField{{Name: "当前处境", Type: "string"}},
	}}}
	state := map[string]any{"actors": map[string]any{
		"protagonist": map[string]any{
			"id":          "protagonist",
			"template_id": "protagonist",
			"state":       map[string]any{"当前处境": "林地"},
		},
	}}
	result := validTurnSubmissionResult()
	result.ActorStatePatches = []ActorStatePatch{{
		ActorID: "protagonist",
		State:   map[string]any{"当前详细地点": "哨站二层"},
	}}

	prepared, receipt := PrepareTurnSubmission(system, state, result)
	if prepared == nil || !receipt.Accepted {
		t.Fatalf("submission should survive an optional unknown-only patch: prepared=%#v receipt=%#v", prepared, receipt)
	}
	if got := prepared.TurnResult(); len(got.ActorStatePatches) != 0 {
		t.Fatalf("unknown-only patch should be removed: %#v", got.ActorStatePatches)
	}
}

func TestPrepareTurnSubmissionRejectsKnownFieldWithInvalidType(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID:     "protagonist",
		Fields: []ActorStateField{{Name: "生命值", Type: "number"}},
	}}}
	state := map[string]any{"actors": map[string]any{
		"protagonist": map[string]any{
			"id":          "protagonist",
			"template_id": "protagonist",
			"state":       map[string]any{"生命值": float64(10)},
		},
	}}
	result := validTurnSubmissionResult()
	result.ActorStatePatches = []ActorStatePatch{{
		ActorID: "protagonist",
		State:   map[string]any{"生命值": "很多"},
	}}

	prepared, receipt := PrepareTurnSubmission(system, state, result)
	if prepared != nil || receipt.Accepted || !receipt.Retryable {
		t.Fatalf("invalid known field should request correction: prepared=%#v receipt=%#v", prepared, receipt)
	}
	if len(receipt.Diagnostics) != 1 || receipt.Diagnostics[0].Code != TurnSubmissionDiagnosticActorStateInvalid {
		t.Fatalf("unexpected diagnostics: %#v", receipt.Diagnostics)
	}
}

func validTurnSubmissionResult() TurnResult {
	return TurnResult{
		Contract:    TurnContract{PlayerIntent: "继续探索", SceneGoal: "进入哨站"},
		SceneResult: TurnSceneResult{Status: "continued"},
		PlanSignals: TurnPlanSignals{DeviationLevel: "none"},
		Choices:     []string{"检查楼梯", "搜索房间"},
	}
}
