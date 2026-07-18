package interactive

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPrepareTurnSubmissionRetainsAcceptedModuleAcrossRetry(t *testing.T) {
	system, state := turnSubmissionTestState()
	updates := []StateUpdate{{Op: TurnStateUpdateReplace, Path: "/protagonist/当前处境", Value: "废弃哨站"}}
	invalidChoices := []string{"检查楼梯"}

	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState:   system,
		CurrentState: state,
		ChoiceCount:  5,
	}, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &invalidChoices})
	if receipt.Ready || receipt.ModuleStatus.StateChanges != TurnSubmissionModuleAccepted || receipt.ModuleStatus.Choices != TurnSubmissionModuleRejected {
		t.Fatalf("unexpected partial receipt: %#v", receipt)
	}
	if got := prepared.TurnResult(); len(got.StateUpdates) != 1 || len(got.Choices) != 0 {
		t.Fatalf("only state_updates should be retained: %#v", got)
	}

	choices := testTurnChoices()
	prepared, receipt = PrepareTurnSubmission(TurnSubmissionContext{
		ActorState:   system,
		CurrentState: state,
		ChoiceCount:  5,
	}, prepared, TurnSubmissionInput{Choices: &choices})
	if !receipt.Ready || receipt.ModuleStatus.StateChanges != TurnSubmissionModuleAccepted || receipt.ModuleStatus.Choices != TurnSubmissionModuleAccepted {
		t.Fatalf("retry should complete the draft: %#v", receipt)
	}
	if got := prepared.TurnResult(); len(got.StateUpdates) != 1 || len(got.Choices) != 5 {
		t.Fatalf("accepted state module was not retained: %#v", got)
	}
}

func TestPrepareTurnSubmissionIgnoresResubmittedAcceptedModule(t *testing.T) {
	system, state := turnSubmissionTestState()
	updates := []StateUpdate{{Op: TurnStateUpdateReplace, Path: "/protagonist/当前处境", Value: "废弃哨站"}}
	invalidChoices := []string{"只有一个"}
	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &invalidChoices})
	if receipt.ModuleStatus.StateChanges != TurnSubmissionModuleAccepted {
		t.Fatalf("state_updates should be accepted first: %#v", receipt)
	}

	choices := testTurnChoices()
	prepared, receipt = PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, prepared, TurnSubmissionInput{
		Choices: &choices,
		Diagnostics: []TurnSubmissionDiagnostic{{
			Module: TurnSubmissionModuleStateChanges, Code: TurnSubmissionDiagnosticInvalidModule,
		}},
	})
	if !receipt.Ready || len(receipt.Diagnostics) != 0 || !prepared.Ready() {
		t.Fatalf("an already accepted module must not be revalidated: receipt=%#v", receipt)
	}
}

func TestPrepareTurnSubmissionRejectsStateModuleAtomically(t *testing.T) {
	system, state := turnSubmissionTestState()
	updates := []StateUpdate{
		{Op: TurnStateUpdateReplace, Path: "/protagonist/当前处境", Value: "废弃哨站"},
		{Op: TurnStateUpdateReplace, Path: "/protagonist/生命值", Value: "很多"},
	}
	choices := testTurnChoices()

	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &choices})
	if receipt.Ready || receipt.ModuleStatus.StateChanges != TurnSubmissionModuleRejected || receipt.ModuleStatus.Choices != TurnSubmissionModuleAccepted {
		t.Fatalf("unexpected atomic rejection: %#v", receipt)
	}
	if got := prepared.TurnResult(); len(got.StateUpdates) != 0 || len(got.Choices) != 5 {
		t.Fatalf("invalid state module must not be partially staged: %#v", got)
	}
	if len(receipt.Diagnostics) != 1 || receipt.Diagnostics[0].Index == nil || *receipt.Diagnostics[0].Index != 1 {
		t.Fatalf("diagnostic should identify the failing operation: %#v", receipt.Diagnostics)
	}
}

func TestUnifiedTurnSubmissionDecodesStructuredStateChangesAndIsolatesFailures(t *testing.T) {
	system, state := turnSubmissionTestState()
	complete := DecodeInteractiveTurnSubmissionInput(`{"state_changes":[{"op":"replace","actor_id":"protagonist","field_id":"当前处境","value":"废弃哨站"}],"choices":["左路","右路","检查地图","询问同伴","原地观察"]}`)
	if complete.StateUpdates == nil || len(*complete.StateUpdates) != 1 || (*complete.StateUpdates)[0].Path != "/protagonist/当前处境" || complete.Choices == nil {
		t.Fatalf("unified submission should compile structured IDs to the internal canonical update: %#v", complete)
	}
	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{ActorState: system, CurrentState: state, ChoiceCount: 5}, nil, complete)
	if !receipt.Ready || !prepared.Ready() {
		t.Fatalf("complete unified submission should settle both modules: receipt=%#v result=%#v", receipt, prepared.TurnResult())
	}
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(receiptJSON), `"state_changes":"accepted"`) || strings.Contains(string(receiptJSON), "actor_state_patches") {
		t.Fatalf("model-facing receipt should use the unified state_changes vocabulary: %s", receiptJSON)
	}

	malformed := DecodeInteractiveTurnSubmissionInput(`{"state_changes":"not-an-array","choices":["左路","右路","检查地图","询问同伴","原地观察"]}`)
	if malformed.StateUpdates != nil || len(malformed.Diagnostics) != 1 || malformed.Diagnostics[0].Module != TurnSubmissionModuleStateChanges {
		t.Fatalf("malformed state_changes must be isolated while valid choices remain available: %#v", malformed)
	}
	if malformed.Choices == nil || len(*malformed.Choices) != 5 {
		t.Fatalf("valid choices from the same tool call must survive a malformed state module: %#v", malformed)
	}
	prepared, receipt = PrepareTurnSubmission(TurnSubmissionContext{ActorState: system, CurrentState: state, ChoiceCount: 5}, nil, malformed)
	if receipt.Ready || receipt.ModuleStatus.StateChanges != TurnSubmissionModuleRejected || receipt.ModuleStatus.Choices != TurnSubmissionModuleAccepted || len(prepared.TurnResult().Choices) != 5 {
		t.Fatalf("valid choices must be retained across a state-only retry: receipt=%#v result=%#v", receipt, prepared.TurnResult())
	}
	retry := DecodeInteractiveTurnSubmissionInput(`{"state_changes":[{"op":"replace","actor_id":"protagonist","field_id":"当前处境","value":"废弃哨站"}]}`)
	prepared, receipt = PrepareTurnSubmission(TurnSubmissionContext{ActorState: system, CurrentState: state, ChoiceCount: 5}, prepared, retry)
	if !receipt.Ready || len(prepared.TurnResult().StateUpdates) != 1 || len(prepared.TurnResult().Choices) != 5 {
		t.Fatalf("retrying only state_changes should complete the retained choices: receipt=%#v result=%#v", receipt, prepared.TurnResult())
	}
}

func TestUnifiedTurnSubmissionSupportsObjectSubpathsAndExplicitActorCreation(t *testing.T) {
	system := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			{ID: "protagonist", Fields: []ActorStateField{{Name: "关系", Type: "object", Visibility: "visible"}}},
			{ID: "important_character", Fields: []ActorStateField{{Name: "状态", Type: "string", Visibility: "visible"}}},
		},
	}
	state := map[string]any{"actors": map[string]any{
		"protagonist": map[string]any{
			"id": "protagonist", "template_id": "protagonist",
			"state": map[string]any{"关系": map[string]any{}},
		},
	}}
	input := DecodeInteractiveTurnSubmissionInput(`{"state_changes":[{"op":"replace","actor_id":"protagonist","field_id":"关系","subpath":["盟友/敌人","信任~值"],"value":3},{"op":"create","actor_id":"npc_guard","template_id":"important_character","name":"守门人","initial_state":{"状态":"警惕"}}],"choices":["前进","观察","交谈","等待","后退"]}`)
	if input.StateUpdates == nil || len(*input.StateUpdates) != 2 {
		t.Fatalf("structured state changes were not decoded: %#v", input)
	}
	if got := (*input.StateUpdates)[0].Path; got != "/protagonist/关系/盟友~1敌人/信任~0值" {
		t.Fatalf("backend should escape subpath segments internally, got %q", got)
	}
	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{ActorState: system, CurrentState: state, ChoiceCount: 5}, nil, input)
	if !receipt.Ready || !prepared.Ready() || len(prepared.TurnResult().StateUpdates) != 2 {
		t.Fatalf("object update and explicit create should compile atomically: receipt=%#v result=%#v", receipt, prepared.TurnResult())
	}
}

func TestChoicesSubmissionCarriesOptionalDirectorUpdateHint(t *testing.T) {
	system, state := turnSubmissionTestState()
	updates := []StateUpdate{}
	choicesInput := DecodeInteractiveTurnSubmissionInput(`{"choices":["左路","右路","检查地图","询问同伴","原地观察"],"director_update":{"needed":true,"reason":"玩家公开了足以推翻当前阶段前提的证据"}}`)
	if choicesInput.Choices == nil || choicesInput.DirectorUpdate == nil || !choicesInput.DirectorUpdate.Needed {
		t.Fatalf("material Director hint was not decoded: %#v", choicesInput)
	}
	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, TurnSubmissionInput{StateUpdates: &updates})
	if receipt.ModuleStatus.StateChanges != TurnSubmissionModuleAccepted {
		t.Fatalf("state module was not staged first: %#v", receipt)
	}
	prepared, receipt = PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, prepared, choicesInput)
	result := prepared.TurnResult()
	if !receipt.Ready || result.DirectorUpdate == nil || result.DirectorUpdate.Reason != "玩家公开了足以推翻当前阶段前提的证据" {
		t.Fatalf("Director hint did not survive module staging: receipt=%#v result=%#v", receipt, result)
	}

	routine := DecodeInteractiveTurnSubmissionInput(`{"choices":["左路","右路","检查地图","询问同伴","原地观察"]}`)
	prepared, receipt = PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, TurnSubmissionInput{StateUpdates: &updates})
	prepared, receipt = PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, prepared, routine)
	if !receipt.Ready || prepared.TurnResult().DirectorUpdate != nil {
		t.Fatalf("routine choices should omit the Director hint: %#v", prepared.TurnResult())
	}
}

func TestChoicesSubmissionRejectsUnexplainedDirectorUpdateHint(t *testing.T) {
	input := DecodeInteractiveTurnSubmissionInput(`{"choices":["左路","右路","检查地图","询问同伴","原地观察"],"director_update":{"needed":true}}`)
	if input.Choices != nil || input.DirectorUpdate != nil || len(input.Diagnostics) != 1 || input.Diagnostics[0].Module != TurnSubmissionModuleChoices {
		t.Fatalf("an unexplained material hint should retry only choices: %#v", input)
	}
}

func TestPrepareTurnSubmissionUsesConfiguredChoiceCountAndUnicodeDistinctness(t *testing.T) {
	system, state := turnSubmissionTestState()
	updates := []StateUpdate{}
	choices := []string{"左路", "右路", "检查地图", "询问同伴", "原地观察", "返回营地", "独自探路"}
	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 7,
	}, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &choices})
	if !receipt.Ready || !prepared.Ready() || len(prepared.TurnResult().Choices) != 7 {
		t.Fatalf("configured choices should be accepted: receipt=%#v result=%#v", receipt, prepared.TurnResult())
	}

	duplicate := []string{"Ａ", "a", "B", "C", "D"}
	_, receipt = PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &duplicate})
	if receipt.ModuleStatus.Choices != TurnSubmissionModuleRejected || len(receipt.Diagnostics) != 1 || receipt.Diagnostics[0].Code != TurnSubmissionDiagnosticDuplicateChoice {
		t.Fatalf("NFKC/case duplicate should identify the choices module: %#v", receipt)
	}
}

func TestPrepareTurnSubmissionAcceptsEmptyChoicesOnlyForDeclaredTerminal(t *testing.T) {
	system, state := turnSubmissionTestState()
	updates := []StateUpdate{}
	choices := []string{}
	_, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &choices})
	if receipt.ModuleStatus.Choices != TurnSubmissionModuleRejected || len(receipt.Diagnostics) != 1 || receipt.Diagnostics[0].Code != TurnSubmissionDiagnosticChoiceCountMismatch {
		t.Fatalf("non-terminal empty choices should be rejected: %#v", receipt)
	}

	resolution := &RuleResolution{TerminalCandidate: &TerminalCandidate{Type: "completed", Reason: "故事已结束"}}
	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5, RuleResolution: resolution,
	}, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &choices})
	if !receipt.Ready || !prepared.Ready() || len(prepared.TurnResult().Choices) != 0 {
		t.Fatalf("declared terminal empty choices should be accepted: %#v", receipt)
	}
}

func turnSubmissionTestState() (StoryDirectorActorStateSystem, map[string]any) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist",
		Fields: []ActorStateField{
			{Name: "当前处境", Type: "string", Visibility: "visible"},
			{Name: "生命值", Type: "number", Visibility: "visible"},
		},
	}}}
	state := map[string]any{"actors": map[string]any{
		"protagonist": map[string]any{
			"id":          "protagonist",
			"template_id": "protagonist",
			"state":       map[string]any{"当前处境": "林地", "生命值": float64(10)},
		},
	}}
	return system, state
}
