package interactive

import (
	"path/filepath"
	"testing"
)

func TestApplyStateSchemaInitializationMigratesOpeningStateAndKeepsAudit(t *testing.T) {
	workspace := t.TempDir()
	novaDir := filepath.Join(workspace, ".denova")
	store := NewStoreWithNovaDir(workspace, novaDir)
	base := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{{
			ID: "protagonist", Name: "主角", Fields: []ActorStateField{
				{Name: "功力", Type: "string", Default: "7", Visibility: "visible"},
				{Name: "旧秘密", Type: "string", Default: "保留在备份", Visibility: "hidden"},
			},
		}, {ID: "npc", Name: "临时角色", Fields: []ActorStateField{{Name: "态度", Type: "string", Default: "中立"}}}},
		InitialActors: []ActorStateInitialActor{{ID: "protagonist", Name: "主角", TemplateID: "protagonist"}},
	}
	story, err := store.CreateStory(CreateStoryRequest{
		Title:      "首轮后适配",
		ActorState: &base,
		StateSchemaInitialization: &StateSchemaInitializationStatus{
			Mode: StateSchemaAdaptationModeAfterOpening, Status: StateSchemaInitializationWaitingOpening, BaseRevision: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "运转功法", Narrative: "灵力沿经脉汇聚，临时向导随即离场。",
		Ops: []StateOp{
			{Op: "set", Path: "actors.guide.id", Value: "guide"},
			{Op: "set", Path: "actors.guide.name", Value: "向导"},
			{Op: "set", Path: "actors.guide.template_id", Value: "npc"},
		},
		ActorOps: []ActorStateOp{{Op: "set", ActorID: "guide", FieldID: "态度", Value: "友善"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := store.ClaimStateSchemaInitialization(story.ID, turn.ID); err != nil || !claimed {
		t.Fatalf("claim initialization: claimed=%v err=%v", claimed, err)
	}
	status, err := store.ApplyStateSchemaInitialization(story.ID, "main", turn.ID, ActorStateSchemaAdaptation{
		Summary: "根据真实开局调整修炼状态",
		TemplateOps: []ActorStateTemplateSchemaOp{{
			Op: "fields", TemplateID: "protagonist", FieldOps: []ActorStateFieldSchemaOp{
				{Op: "replace", FieldID: "功力", Field: ActorStateField{Name: "战力", Type: "number", Default: 0, Visibility: "visible"}, Reason: "数值检定需要"},
				{Op: "remove", FieldID: "旧秘密", Reason: "开局未采用"},
				{Op: "add", Field: ActorStateField{Name: "士气", Type: "number", Default: 10, Visibility: "visible"}, Reason: "首轮明确出现"},
			},
		}, {Op: "remove", TemplateID: "npc", Reason: "首轮临时角色已离场"}},
		ActorOps: []ActorStateRuntimeSchemaOp{{Op: "remove", ActorID: "guide", Reason: "不再长期追踪"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StateSchemaInitializationReady || status.TargetRevision != 2 || len(status.Changes) != 5 {
		t.Fatalf("unexpected initialization status: %#v", status)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActorStateSchema == nil || snapshot.ActorStateSchema.Revision != 2 || snapshot.ActorStateSchema.Adaptation == nil {
		t.Fatalf("adapted schema missing: %#v", snapshot.ActorStateSchema)
	}
	if got := snapshot.ActorStateSchema.LegacyFieldPaths["protagonist"]["功力"]; got != "战力" {
		t.Fatalf("rename alias mismatch: %q", got)
	}
	actors, _ := snapshot.State[actorStateRoot].(map[string]any)
	if _, exists := actors["guide"]; exists {
		t.Fatalf("runtime Actor migration should remove the departed guide: %#v", actors["guide"])
	}
	actor, _ := actors["protagonist"].(map[string]any)
	values, _ := actor["state"].(map[string]any)
	if values["战力"] != float64(7) || values["士气"] != float64(10) {
		t.Fatalf("migrated values mismatch: %#v", values)
	}
	if _, exists := values["功力"]; exists {
		t.Fatalf("renamed field must be removed from active state: %#v", values)
	}
	if _, exists := values["旧秘密"]; exists {
		t.Fatalf("removed field must be removed from active state: %#v", values)
	}
	backups, err := filepath.Glob(filepath.Join(novaDir, "backups", "state-schema-adaptation", "*", "story-"+story.ID+".jsonl"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("expected one pre-migration backup: paths=%#v err=%v", backups, err)
	}
	if err := store.RewindToTurnParent(story.ID, RewindTurnRequest{BranchID: "main", TurnID: turn.ID}); err != nil {
		t.Fatal(err)
	}
	rewound, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	rewoundActors, _ := rewound.State[actorStateRoot].(map[string]any)
	rewoundActor, _ := rewoundActors["protagonist"].(map[string]any)
	rewoundValues, _ := rewoundActor["state"].(map[string]any)
	if rewoundValues["战力"] != float64(7) {
		t.Fatalf("rewind must replay renamed and converted fields: %#v", rewoundValues)
	}
}

func TestStateSchemaInitializationFailureAndSkipKeepRevisionOne(t *testing.T) {
	workspace := t.TempDir()
	store := NewStoreWithNovaDir(workspace, filepath.Join(workspace, ".denova"))
	base := StoryDirectorActorStateSystem{
		Templates:     []ActorStateTemplate{{ID: "npc", Name: "NPC", Fields: []ActorStateField{{Name: "态度", Type: "string", Default: "中立"}}}},
		InitialActors: []ActorStateInitialActor{{ID: "guide", Name: "向导", TemplateID: "npc"}},
	}
	story, err := store.CreateStory(CreateStoryRequest{Title: "失败降级", ActorState: &base, StateSchemaInitialization: &StateSchemaInitializationStatus{Mode: StateSchemaAdaptationModeAfterOpening, Status: StateSchemaInitializationWaitingOpening, BaseRevision: 1}})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "问路", Narrative: "向导指向北方。"})
	if err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := store.ClaimStateSchemaInitialization(story.ID, turn.ID); err != nil || !claimed {
		t.Fatalf("claim initialization: claimed=%v err=%v", claimed, err)
	}
	_, err = store.ApplyStateSchemaInitialization(story.ID, "main", turn.ID, ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{Op: "remove", TemplateID: "npc"}}})
	if err == nil {
		t.Fatal("removing an in-use template without Actor migration must fail")
	}
	if err := store.MarkStateSchemaInitializationFailed(story.ID, turn.ID, err); err != nil {
		t.Fatal(err)
	}
	status, err := store.SkipStateSchemaInitialization(story.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StateSchemaInitializationSkipped || status.Mode != StateSchemaAdaptationModeOff {
		t.Fatalf("skip status mismatch: %#v", status)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActorStateSchema == nil || snapshot.ActorStateSchema.Revision != 1 || snapshot.CurrentTurn == nil {
		t.Fatalf("failed adaptation must preserve revision one and opening: %#v", snapshot)
	}
}

func TestApplyUnchangedStateSchemaProposalKeepsRevisionAndStoresReview(t *testing.T) {
	workspace := t.TempDir()
	novaDir := filepath.Join(workspace, ".denova")
	store := NewStoreWithNovaDir(workspace, novaDir)
	minValue, maxValue := 0.0, 100.0
	base := StoryDirectorActorStateSystem{
		Templates:     []ActorStateTemplate{{ID: "protagonist", Name: "主角", Fields: []ActorStateField{{Name: "生命", Type: "number", Default: 100, Min: &minValue, Max: &maxValue}}}},
		InitialActors: []ActorStateInitialActor{{ID: "protagonist", Name: "主角", TemplateID: "protagonist"}},
	}
	story, err := store.CreateStory(CreateStoryRequest{
		Title: "无需变更", ActorState: &base,
		StateSchemaInitialization: &StateSchemaInitializationStatus{Mode: StateSchemaAdaptationModeAfterOpening, Status: StateSchemaInitializationWaitingOpening, BaseRevision: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "检查状态", Narrative: "主角确认身体无恙。"})
	if err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := store.ClaimStateSchemaInitialization(story.ID, turn.ID); err != nil || !claimed {
		t.Fatalf("claim initialization: claimed=%v err=%v", claimed, err)
	}
	status, err := store.ApplyStateSchemaProposal(story.ID, "main", turn.ID, ActorStateSchemaProposal{
		Summary: "现有生命字段完整覆盖规则",
		Requirements: []ActorStateSchemaRequirementReview{{
			Source: ActorStateSchemaRequirementSource{Kind: "lore", ID: "生命规则"}, Requirement: "生命为 0-100",
			ExpectedType: "number", Min: &minValue, Max: &maxValue, Decision: "covered", TemplateID: "protagonist", FieldID: "生命",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StateSchemaInitializationReady || status.TargetRevision != 1 || status.Outcome != "unchanged" || len(status.Requirements) != 1 {
		t.Fatalf("unchanged review status mismatch: %#v", status)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActorStateSchema == nil || snapshot.ActorStateSchema.Revision != 1 || snapshot.ActorStateSchema.Adaptation == nil || len(snapshot.ActorStateSchema.Adaptation.Requirements) != 1 {
		t.Fatalf("unchanged review must preserve schema revision and audit coverage: %#v", snapshot.ActorStateSchema)
	}
	backups, err := filepath.Glob(filepath.Join(novaDir, "backups", "state-schema-adaptation", "*", "story-"+story.ID+".jsonl"))
	if err != nil || len(backups) != 0 {
		t.Fatalf("unchanged review must not create a migration backup: paths=%#v err=%v", backups, err)
	}
	reopened, err := store.ReopenStateSchemaReview(story.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Status != StateSchemaInitializationWaitingOpening || reopened.BaseRevision != 1 || reopened.Outcome != "" || reopened.Summary != "" || len(reopened.Requirements) != 0 {
		t.Fatalf("manual re-review should start from the current schema and clear the previous run status: %#v", reopened)
	}
}
