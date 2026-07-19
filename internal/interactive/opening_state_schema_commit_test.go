package interactive

import "testing"

func TestCreateStorySchemaPolicyRequiresBaseStateSystem(t *testing.T) {
	store := NewStore(t.TempDir())
	policy := StoryStateSchemaPolicy{Mode: StoryStateSchemaModeGenerate}
	if _, err := store.CreateStory(CreateStoryRequest{Title: "缺失基线", StateSchemaPolicy: &policy}); err == nil {
		t.Fatal("new story schema policies must not create an unopenable story without a frozen base")
	}
}

func TestOpeningGameStateSchemaCommitsSchemaInitialStateAndTurnAtomically(t *testing.T) {
	store := NewStore(t.TempDir())
	base := GeneratedStoryActorStateCore()
	policy := StoryStateSchemaPolicy{Mode: StoryStateSchemaModeAdaptTemplate}
	story, err := store.CreateStory(CreateStoryRequest{
		Title:             "原子开局",
		ActorState:        &base,
		StateSchemaPolicy: &policy,
		ChoiceCount:       2,
		StateSchemaInitialization: &StateSchemaInitializationStatus{
			Mode: StoryStateSchemaModeAdaptTemplate, Status: StateSchemaInitializationWaitingOpening, BaseRevision: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if story.Events != 0 {
		t.Fatalf("dynamic story must not materialize state before opening: events=%d", story.Events)
	}

	proposal := openingSchemaProposalAddingIdentity()
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:            "main",
		User:                "开始故事",
		Narrative:           "雨夜里，主角在旧车站醒来。",
		StateSchemaProposal: &proposal,
		TurnResult: &TurnResult{
			StateUpdates: []StateUpdate{
				{Op: TurnStateUpdateReplace, Path: "/protagonist/身份", Value: "失忆旅人"},
				{Op: TurnStateUpdateReplace, Path: "/story/当前详细地点", Value: "旧车站月台"},
				{Op: TurnStateUpdateReplace, Path: "/story/当前事件", Value: "主角在雨夜的旧车站醒来"},
			},
			Choices: []string{"检查口袋", "走向站台出口"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if turn.StateDelta == nil || turn.StateStatus != "ready" {
		t.Fatalf("opening turn must carry bootstrap and result state: %#v", turn)
	}
	storyCtx, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := getPath(storyCtx.Snapshot.State, "actors.protagonist.state.身份"); got != "失忆旅人" {
		t.Fatalf("unexpected initialized protagonist identity: %#v", got)
	}
	if got := getPath(storyCtx.Snapshot.State, "actors.story.state.当前事件"); got != "主角在雨夜的旧车站醒来" {
		t.Fatalf("unexpected story context: %#v", got)
	}
	if len(storyCtx.Snapshot.Turns) != 1 {
		t.Fatalf("opening must be a single committed turn, got %d", len(storyCtx.Snapshot.Turns))
	}
	if storyCtx.Meta.ActorStateSchema == nil || storyCtx.Meta.ActorStateSchema.Adaptation == nil || storyCtx.Meta.ActorStateSchema.Adaptation.Source != "game_agent" {
		t.Fatalf("schema audit must identify the foreground Game Agent: %#v", storyCtx.Meta.ActorStateSchema)
	}
	status := storyCtx.Meta.StateSchemaInitialization
	if status == nil || status.Status != StateSchemaInitializationReady || status.SourceTurnID != turn.ID || status.TargetRevision != 2 {
		t.Fatalf("unexpected finalized schema status: %#v", status)
	}
}

func TestOpeningGameStateSchemaFailureLeavesNoPartialSchemaOrState(t *testing.T) {
	store := NewStore(t.TempDir())
	base := GeneratedStoryActorStateCore()
	policy := StoryStateSchemaPolicy{Mode: StoryStateSchemaModeGenerate}
	story, err := store.CreateStory(CreateStoryRequest{
		Title:             "失败不落盘",
		ActorState:        &base,
		StateSchemaPolicy: &policy,
		ChoiceCount:       2,
		StateSchemaInitialization: &StateSchemaInitializationStatus{
			Mode: StoryStateSchemaModeGenerate, Status: StateSchemaInitializationWaitingOpening, BaseRevision: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal := openingSchemaProposalAddingIdentity()
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:            "main",
		Narrative:           "这段正文不能单独落盘。",
		StateSchemaProposal: &proposal,
		TurnResult: &TurnResult{
			StateUpdates: []StateUpdate{{Op: TurnStateUpdateReplace, Path: "/protagonist/不存在", Value: "invalid"}},
			Choices:      []string{"继续", "返回"},
		},
	})
	if err == nil {
		t.Fatal("expected invalid opening state update to reject the whole commit")
	}
	storyCtx, readErr := store.StoryContext(story.ID, "main")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(storyCtx.Snapshot.Turns) != 0 || len(storyCtx.Snapshot.State[actorStateRoot].(map[string]any)) != 0 {
		t.Fatalf("failed opening leaked turn or state: turns=%d state=%#v", len(storyCtx.Snapshot.Turns), storyCtx.Snapshot.State)
	}
	if storyCtx.Meta.ActorStateSchema.Adaptation != nil || storyCtx.Meta.ActorStateSchema.Revision != 1 {
		t.Fatalf("failed opening leaked schema: %#v", storyCtx.Meta.ActorStateSchema)
	}
	if status := storyCtx.Meta.StateSchemaInitialization; status == nil || status.Status != StateSchemaInitializationWaitingOpening {
		t.Fatalf("failed opening changed initialization status: %#v", status)
	}
}

func TestOpeningSchemaBatchRejectsActorValuesWithoutPollutingDraft(t *testing.T) {
	base := GeneratedStoryActorStateCore()
	draft := NewActorStateSchemaBatchDraft(base, StoryDirectorTRPGSystem{})
	result := draft.SubmitStructureOnly(ActorStateSchemaBatch{
		Finalize: true,
		Items: []ActorStateSchemaBatchItem{{
			ItemID: "bad-values",
			Requirements: []ActorStateSchemaRequirementReview{{
				Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-draft"}, Requirement: "初始化身份", EvidenceKind: "confirmed", ValuePolicy: ActorStateSchemaValuePolicyInitialize, ActorID: DefaultActorID, ExpectedType: "string", Decision: "covered", TemplateID: DefaultActorID, FieldID: "身份",
			}},
			Adaptation: ActorStateSchemaAdaptation{ActorOps: []ActorStateRuntimeSchemaOp{{Op: "set", ActorID: DefaultActorID, FieldID: "身份", Value: "旅人"}}},
		}},
	}, ActorStateSchemaBatchAudit{OpeningSourceIDs: []string{"opening-draft"}})
	if len(result.Rejected) == 0 || result.Finalized || result.DraftAcceptedItems != 0 {
		t.Fatalf("structure-only draft accepted Actor values: %#v", result)
	}
	if _, ok := draft.FinalProposal(); ok {
		t.Fatal("rejected value item must not finalize the draft")
	}
}

func TestOpeningSchemaDraftCanSatisfyInitiallyMissingTRPGBindingAtFinalize(t *testing.T) {
	base := GeneratedStoryActorStateCore()
	trpg := StoryDirectorTRPGSystem{RuleTemplates: []RuleCheck{{ID: "rule-1", StateBindings: []RuleStateBinding{{
		ID: "strength-check", ActorTemplateID: DefaultActorID,
		Modifiers: []RuleStateBindingModifier{{Source: "actor", FieldID: "力量", Effect: "advantage"}},
	}}}}}
	draft := NewOpeningActorStateSchemaBatchDraft(base, trpg)
	result := draft.SubmitStructureOnly(ActorStateSchemaBatch{Finalize: true, Items: []ActorStateSchemaBatchItem{{
		ItemID: "trpg-strength",
		Requirements: []ActorStateSchemaRequirementReview{{
			Source: ActorStateSchemaRequirementSource{Kind: "trpg", ID: "rule-1"}, Requirement: "力量检定需要数值字段", EvidenceKind: "default", ValuePolicy: ActorStateSchemaValuePolicySchemaOnly, ExpectedType: "number", Decision: "add", TemplateID: DefaultActorID, FieldID: "力量",
		}},
		Adaptation: ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{
			Op: "fields", TemplateID: DefaultActorID, FieldOps: []ActorStateFieldSchemaOp{{Op: "add", Field: ActorStateField{Name: "力量", Type: "number", Visibility: "visible"}}},
		}}},
	}}}, ActorStateSchemaBatchAudit{TRPGSourceIDs: []string{"rule-1"}})
	if !result.Finalized || len(result.Rejected) > 0 {
		t.Fatalf("opening draft did not make generated schema TRPG-compatible: %#v", result)
	}
}

func TestUpdateStoryRebuildsSchemaPolicyOnlyBeforeFirstTurn(t *testing.T) {
	store := NewStore(t.TempDir())
	fixedBase := GeneratedStoryActorStateCore()
	fixed := StoryStateSchemaPolicy{Mode: StoryStateSchemaModeFixedTemplate}
	story, err := store.CreateStory(CreateStoryRequest{Title: "返回配置", ActorState: &fixedBase, StateSchemaPolicy: &fixed})
	if err != nil {
		t.Fatal(err)
	}
	if story.Events != 1 {
		t.Fatalf("fixed template should materialize its initial Actors: events=%d", story.Events)
	}
	generated := StoryStateSchemaPolicy{Mode: StoryStateSchemaModeGenerate}
	updated, err := store.UpdateStory(story.ID, UpdateStoryRequest{
		Title:             story.Title,
		StateSchemaPolicy: &generated,
		ActorState:        &fixedBase,
		TRPGSystem:        &StoryDirectorTRPGSystem{},
		StateSchemaInitialization: &StateSchemaInitializationStatus{
			Mode: StoryStateSchemaModeGenerate, Status: StateSchemaInitializationWaitingOpening, BaseRevision: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Events != 0 || updated.StateSchemaPolicy == nil || updated.StateSchemaPolicy.Mode != StoryStateSchemaModeGenerate {
		t.Fatalf("pre-opening rebuild did not reset story index: %#v", updated)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if actors := snapshot.State[actorStateRoot].(map[string]any); len(actors) != 0 {
		t.Fatalf("dynamic rebuild materialized Actors too early: %#v", actors)
	}

	proposal := openingSchemaProposalAddingIdentity()
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", Narrative: "开局完成", StateSchemaProposal: &proposal,
		TurnResult: &TurnResult{StateUpdates: []StateUpdate{{Op: TurnStateUpdateReplace, Path: "/story/当前详细地点", Value: "门厅"}, {Op: TurnStateUpdateReplace, Path: "/story/当前事件", Value: "主角进入门厅"}}, Choices: []string{"前进", "观察", "交谈", "等待", "返回"}},
	}); err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdateStory(story.ID, UpdateStoryRequest{Title: story.Title, StateSchemaPolicy: &fixed, ActorState: &fixedBase})
	if err == nil {
		t.Fatal("schema policy must freeze after the first committed turn")
	}
}

func openingSchemaProposalAddingIdentity() ActorStateSchemaProposal {
	return ActorStateSchemaProposal{
		Summary: "为主角增加开局身份字段",
		Requirements: []ActorStateSchemaRequirementReview{{
			Source:       ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-draft"},
			Requirement:  "长期记录主角在开局确认的身份",
			EvidenceKind: "confirmed",
			ValuePolicy:  ActorStateSchemaValuePolicySchemaOnly,
			ExpectedType: "string",
			Decision:     "add",
			TemplateID:   DefaultActorID,
			FieldID:      "身份",
		}},
		Adaptation: ActorStateSchemaAdaptation{
			TemplateOps: []ActorStateTemplateSchemaOp{{
				Op: "fields", TemplateID: DefaultActorID,
				FieldOps: []ActorStateFieldSchemaOp{{Op: "add", Field: ActorStateField{Name: "身份", Type: "string", Visibility: "visible", Group: "人物设定", Display: "inline"}}},
			}},
		},
	}
}
