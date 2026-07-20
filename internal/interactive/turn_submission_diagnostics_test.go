package interactive

import (
	"strings"
	"testing"
)

func TestPrepareTurnSubmissionDoesNotReportDependentErrorsAfterInvalidCreate(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID:     "important_character",
		Fields: []ActorStateField{{Name: "生命值", Type: "number", Visibility: "visible"}},
	}}}
	input := DecodeInteractiveTurnSubmissionInput(`{
		"state_changes":[
			{
				"op":"create",
				"actor_id":"柳寒衣",
				"template_id":"important_character",
				"name":"柳寒衣",
				"initial_state":{"生命值":"未知"}
			},
			{"op":"replace","actor_id":"柳寒衣","field_id":"生命值","value":12}
		],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	_, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: map[string]any{}, ChoiceCount: 5,
	}, nil, input)
	if len(receipt.Diagnostics) != 1 {
		t.Fatalf("a failed create should not cascade into dependent actor_not_found diagnostics: %#v", receipt.Diagnostics)
	}
	if receipt.Diagnostics[0].Path != "/state_changes/0/initial_state/生命值" {
		t.Fatalf("unexpected independent diagnostic: %#v", receipt.Diagnostics[0])
	}
}

func TestPrepareTurnSubmissionReportsOverlapAlongsideUnrelatedFailure(t *testing.T) {
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
			{"op":"replace","actor_id":"protagonist","field_id":"关系","value":"稍后补充"},
			{"op":"replace","actor_id":"protagonist","field_id":"生命值","value":9},
			{"op":"replace","actor_id":"protagonist","field_id":"生命值","value":8}
		],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	_, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, input)
	if len(receipt.Diagnostics) != 2 {
		t.Fatalf("unrelated value and overlap failures should be reported together: %#v", receipt.Diagnostics)
	}
	if receipt.Diagnostics[0].Path != "/state_changes/0" || receipt.Diagnostics[1].Path != "/state_changes/2" {
		t.Fatalf("unexpected diagnostic paths: %#v", receipt.Diagnostics)
	}
	if receipt.Diagnostics[1].Code != "overlapping_state_path" {
		t.Fatalf("second diagnostic should identify the overlap: %#v", receipt.Diagnostics[1])
	}
}

func TestPrepareTurnSubmissionPreservesOverlapErrorForDuplicateCreate(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{
		{ID: "protagonist", Fields: []ActorStateField{{Name: "关系", Type: "object", Visibility: "visible"}}},
		{ID: "important_character"},
	}}
	state := map[string]any{"actors": map[string]any{
		"protagonist": map[string]any{
			"id": "protagonist", "template_id": "protagonist",
			"state": map[string]any{"关系": map[string]any{}},
		},
	}}
	input := DecodeInteractiveTurnSubmissionInput(`{
		"state_changes":[
			{"op":"replace","actor_id":"protagonist","field_id":"关系","value":"稍后补充"},
			{"op":"create","actor_id":"柳寒衣","template_id":"important_character","name":"柳寒衣"},
			{"op":"create","actor_id":"柳寒衣","template_id":"important_character","name":"柳寒衣"}
		],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	_, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, input)
	if len(receipt.Diagnostics) != 2 || receipt.Diagnostics[1].Code != "overlapping_state_path" {
		t.Fatalf("duplicate create should retain the compiler's overlap diagnostic: %#v", receipt.Diagnostics)
	}
	if strings.Contains(receipt.Diagnostics[1].MessageEN, "Expected JSON valid") {
		t.Fatalf("non-type diagnostics must not describe a semantic contract as a JSON type: %#v", receipt.Diagnostics[1])
	}
}

func TestPrepareTurnSubmissionDoesNotDescribeSemanticCreateFailureAsJSONType(t *testing.T) {
	input := DecodeInteractiveTurnSubmissionInput(`{
		"state_changes":[
			{"op":"create","actor_id":"柳寒衣","template_id":"missing_template","name":"柳寒衣"}
		],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	_, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: StoryDirectorActorStateSystem{}, CurrentState: map[string]any{}, ChoiceCount: 5,
	}, nil, input)
	if len(receipt.Diagnostics) != 1 || receipt.Diagnostics[0].Code != "actor_create_invalid" {
		t.Fatalf("unexpected semantic create diagnostic: %#v", receipt.Diagnostics)
	}
	if strings.Contains(receipt.Diagnostics[0].MessageEN, "Expected JSON valid") {
		t.Fatalf("semantic create failure must not be presented as a JSON type mismatch: %#v", receipt.Diagnostics[0])
	}
}

func TestPrepareTurnSubmissionDeltaDiagnosticDescribesTheMissingTarget(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Fields: []ActorStateField{{Name: "生命值", Type: "number", Visibility: "visible"}},
	}}}
	state := map[string]any{"actors": map[string]any{
		"protagonist": map[string]any{
			"id": "protagonist", "template_id": "protagonist", "state": map[string]any{},
		},
	}}
	input := DecodeInteractiveTurnSubmissionInput(`{
		"state_changes":[{"op":"delta","actor_id":"protagonist","field_id":"生命值","value":1}],
		"choices":["前进","观察","交谈","等待","后退"]
	}`)

	_, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, input)
	if len(receipt.Diagnostics) != 1 || receipt.Diagnostics[0].Actual != "null" {
		t.Fatalf("delta diagnostic should describe its missing target, not its valid input: %#v", receipt.Diagnostics)
	}
}
