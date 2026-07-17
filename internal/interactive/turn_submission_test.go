package interactive

import "testing"

func TestPrepareTurnSubmissionRetainsAcceptedModuleAcrossRetry(t *testing.T) {
	system, state := turnSubmissionTestState()
	updates := []StateUpdate{{Op: TurnStateUpdateReplace, Path: "/protagonist/当前处境", Value: "废弃哨站"}}
	invalidChoices := []string{"检查楼梯"}

	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState:   system,
		CurrentState: state,
		ChoiceCount:  5,
	}, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &invalidChoices})
	if receipt.Ready || receipt.ModuleStatus.ActorStatePatches != TurnSubmissionModuleAccepted || receipt.ModuleStatus.Choices != TurnSubmissionModuleRejected {
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
	if !receipt.Ready || receipt.ModuleStatus.ActorStatePatches != TurnSubmissionModuleAccepted || receipt.ModuleStatus.Choices != TurnSubmissionModuleAccepted {
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
	if receipt.ModuleStatus.ActorStatePatches != TurnSubmissionModuleAccepted {
		t.Fatalf("state_updates should be accepted first: %#v", receipt)
	}

	choices := testTurnChoices()
	prepared, receipt = PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, prepared, TurnSubmissionInput{
		Choices: &choices,
		Diagnostics: []TurnSubmissionDiagnostic{{
			Module: TurnSubmissionModuleActorStatePatches, Code: TurnSubmissionDiagnosticInvalidModule,
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
	if receipt.Ready || receipt.ModuleStatus.ActorStatePatches != TurnSubmissionModuleRejected || receipt.ModuleStatus.Choices != TurnSubmissionModuleAccepted {
		t.Fatalf("unexpected atomic rejection: %#v", receipt)
	}
	if got := prepared.TurnResult(); len(got.StateUpdates) != 0 || len(got.Choices) != 5 {
		t.Fatalf("invalid state module must not be partially staged: %#v", got)
	}
	if len(receipt.Diagnostics) != 1 || receipt.Diagnostics[0].Index == nil || *receipt.Diagnostics[0].Index != 1 {
		t.Fatalf("diagnostic should identify the failing operation: %#v", receipt.Diagnostics)
	}
}

func TestSeparateSubmissionToolsIsolateMalformedJSON(t *testing.T) {
	system, state := turnSubmissionTestState()
	malformed := DecodeActorStatePatchesSubmissionInput(`{"patches":[{"op":"replace","path":"/protagonist/当前处境","value":"以"路过的散修"身份"}]}`)
	if malformed.StateUpdates != nil || len(malformed.Diagnostics) != 1 || malformed.Diagnostics[0].Module != TurnSubmissionModuleActorStatePatches {
		t.Fatalf("malformed patch JSON must be isolated to its module: %#v", malformed)
	}
	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{ActorState: system, CurrentState: state, ChoiceCount: 5}, nil, malformed)
	if receipt.ModuleStatus.ActorStatePatches != TurnSubmissionModuleRejected || receipt.ModuleStatus.Choices != TurnSubmissionModuleMissing {
		t.Fatalf("unexpected malformed patch receipt: %#v", receipt)
	}

	choices := DecodeChoicesSubmissionInput(`{"choices":["左路","右路","检查地图","询问同伴","原地观察"]}`)
	prepared, receipt = PrepareTurnSubmission(TurnSubmissionContext{ActorState: system, CurrentState: state, ChoiceCount: 5}, prepared, choices)
	if receipt.Ready || receipt.ModuleStatus.ActorStatePatches != TurnSubmissionModuleMissing || receipt.ModuleStatus.Choices != TurnSubmissionModuleAccepted || len(prepared.TurnResult().Choices) != 5 {
		t.Fatalf("valid choices must survive a malformed sibling tool call: receipt=%#v result=%#v", receipt, prepared.TurnResult())
	}
}

func TestChoicesSubmissionCarriesOptionalDirectorUpdateHint(t *testing.T) {
	system, state := turnSubmissionTestState()
	updates := []StateUpdate{}
	choicesInput := DecodeChoicesSubmissionInput(`{"choices":["左路","右路","检查地图","询问同伴","原地观察"],"director_update":{"needed":true,"reason":"玩家公开了足以推翻当前阶段前提的证据"}}`)
	if choicesInput.Choices == nil || choicesInput.DirectorUpdate == nil || !choicesInput.DirectorUpdate.Needed {
		t.Fatalf("material Director hint was not decoded: %#v", choicesInput)
	}
	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, TurnSubmissionInput{StateUpdates: &updates})
	if receipt.ModuleStatus.ActorStatePatches != TurnSubmissionModuleAccepted {
		t.Fatalf("state module was not staged first: %#v", receipt)
	}
	prepared, receipt = PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, prepared, choicesInput)
	result := prepared.TurnResult()
	if !receipt.Ready || result.DirectorUpdate == nil || result.DirectorUpdate.Reason != "玩家公开了足以推翻当前阶段前提的证据" {
		t.Fatalf("Director hint did not survive module staging: receipt=%#v result=%#v", receipt, result)
	}

	routine := DecodeChoicesSubmissionInput(`{"choices":["左路","右路","检查地图","询问同伴","原地观察"]}`)
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
	input := DecodeChoicesSubmissionInput(`{"choices":["左路","右路","检查地图","询问同伴","原地观察"],"director_update":{"needed":true}}`)
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
