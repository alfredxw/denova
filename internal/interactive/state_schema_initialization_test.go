package interactive

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestBuildStateSchemaMigrationPreservesExistingValuesAndSkipsMissingDefaults(t *testing.T) {
	base := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			{ID: "protagonist", Fields: []ActorStateField{{Name: "生命", Type: "number", Default: 100}}},
			{ID: ActorStateStoryContextTemplateID, Fields: []ActorStateField{{Name: "当前地点", Type: "string"}}},
		},
	}
	target := base
	target.Templates = append([]ActorStateTemplate(nil), base.Templates...)
	target.Templates[1].Fields = append(target.Templates[1].Fields, ActorStateField{Name: "未确认的威胁", Type: "string"})
	state := map[string]any{actorStateRoot: map[string]any{
		"protagonist": map[string]any{
			"id": "protagonist", "name": "林风", "template_id": "protagonist",
			"state": map[string]any{"生命": float64(73)},
		},
		DefaultStoryContextActorID: map[string]any{
			"id": DefaultStoryContextActorID, "template_id": ActorStateStoryContextTemplateID,
			"state": map[string]any{},
		},
	}}
	adaptation := ActorStateSchemaAdaptation{
		TemplateOps: []ActorStateTemplateSchemaOp{{
			Op: "fields", TemplateID: ActorStateStoryContextTemplateID,
			FieldOps: []ActorStateFieldSchemaOp{{Op: "add", Field: ActorStateField{Name: "未确认的威胁", Type: "string"}}},
		}},
		InitialActorOps: []ActorStateInitialActorSchemaOp{{
			Op: "replace", ActorID: "protagonist",
			Actor: ActorStateInitialActor{ID: "protagonist", TemplateID: "protagonist", State: map[string]any{"生命": nil}},
		}},
	}
	_, actorOps, _, _, err := buildStateSchemaMigration(base, target, state, adaptation, "turn-opening")
	if err != nil {
		t.Fatal(err)
	}
	sets := map[string]ActorStateOp{}
	for _, op := range actorOps {
		if op.Op != "set" {
			continue
		}
		if op.Value == nil {
			t.Fatalf("schema migration must not generate set operations without a value: %#v", actorOps)
		}
		key := op.ActorID + "/" + op.FieldID
		if previous, exists := sets[key]; exists {
			t.Fatalf("schema migration emitted duplicate sets for %s: previous=%#v current=%#v all=%#v", key, previous, op, actorOps)
		}
		sets[key] = op
	}
	if got := sets["protagonist/生命"].Value; got != float64(73) {
		t.Fatalf("null or omitted replacement state must preserve the existing confirmed value, got %#v in %#v", got, actorOps)
	}
	if _, exists := sets[DefaultStoryContextActorID+"/未确认的威胁"]; exists {
		t.Fatalf("a story field with neither a current nor default value must stay absent: %#v", actorOps)
	}
}

func TestBuildStateSchemaMigrationExplicitValueOverridesExistingOnce(t *testing.T) {
	base := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Fields: []ActorStateField{{Name: "生命", Type: "number", Default: 100}},
	}}}
	state := map[string]any{actorStateRoot: map[string]any{
		"protagonist": map[string]any{"template_id": "protagonist", "state": map[string]any{"生命": float64(73)}},
	}}
	adaptation := ActorStateSchemaAdaptation{InitialActorOps: []ActorStateInitialActorSchemaOp{{
		Op: "replace", ActorID: "protagonist",
		Actor: ActorStateInitialActor{ID: "protagonist", TemplateID: "protagonist", State: map[string]any{"生命": float64(61)}},
	}}}
	_, actorOps, _, _, err := buildStateSchemaMigration(base, base, state, adaptation, "turn-opening")
	if err != nil {
		t.Fatal(err)
	}
	var lifeSets []ActorStateOp
	for _, op := range actorOps {
		if op.Op == "set" && op.ActorID == "protagonist" && op.FieldID == "生命" {
			lifeSets = append(lifeSets, op)
		}
	}
	if len(lifeSets) != 1 || lifeSets[0].Value != float64(61) {
		t.Fatalf("an explicit non-null replacement must override the current/default value exactly once: %#v", actorOps)
	}
}

func TestBuildStateSchemaMigrationAppliesFieldLevelActorInitialization(t *testing.T) {
	base := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{{
			ID: DefaultActorID, Fields: []ActorStateField{{Name: "当前资源", Type: "object"}},
		}},
		InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, TemplateID: DefaultActorID}},
	}
	target := base
	target.Templates = append([]ActorStateTemplate(nil), base.Templates...)
	target.Templates[0].Fields = append([]ActorStateField(nil), base.Templates[0].Fields...)
	target.Templates[0].Fields = append(target.Templates[0].Fields, ActorStateField{Name: "境界", Type: "string", Visibility: "visible"})
	state := map[string]any{actorStateRoot: map[string]any{
		DefaultActorID: map[string]any{
			"id": DefaultActorID, "template_id": DefaultActorID,
			"state": map[string]any{"当前资源": map[string]any{"下品灵石": float64(3)}},
		},
	}}
	source := &ActorStateSchemaActorValueSource{
		SourceID: "state_schema_batch:protagonist-realm", ItemID: "protagonist-realm",
		Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-turn"}, EvidenceKind: "confirmed",
	}
	adaptation := ActorStateSchemaAdaptation{
		TemplateOps: []ActorStateTemplateSchemaOp{{
			Op: "fields", TemplateID: DefaultActorID,
			FieldOps: []ActorStateFieldSchemaOp{{Op: "add", Field: ActorStateField{Name: "境界", Type: "string", Visibility: "visible"}}},
		}},
		ActorOps: []ActorStateRuntimeSchemaOp{{
			Op: "set", ActorID: DefaultActorID, FieldID: "境界", Value: "筑基初期", Reason: "开局正文明确", ValueSource: source,
		}},
	}

	_, actorOps, _, _, err := buildStateSchemaMigration(base, target, state, adaptation, "opening-turn")
	if err != nil {
		t.Fatal(err)
	}
	sets := map[string]ActorStateOp{}
	for _, op := range actorOps {
		if op.Op != "set" || op.ActorID != DefaultActorID {
			continue
		}
		if _, exists := sets[op.FieldID]; exists {
			t.Fatalf("field-level initialization must not duplicate migration sets: %#v", actorOps)
		}
		sets[op.FieldID] = op
	}
	if got := sets["境界"]; got.Value != "筑基初期" || got.SourceID != source.SourceID {
		t.Fatalf("field-level initialization was not materialized with provenance: %#v", actorOps)
	}
	if got := sets["当前资源"].Value; got == nil {
		t.Fatalf("existing state must survive the same schema migration: %#v", actorOps)
	}
}

func TestBuildStateSchemaMigrationKeepsBatchActorValueSourceID(t *testing.T) {
	base := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "npc", Fields: []ActorStateField{{Name: "态度", Type: "string", Default: "中立"}},
	}}}
	source := &ActorStateSchemaActorValueSource{
		SourceID: "state_schema_batch:guide-attitude", ItemID: "guide-attitude",
		Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "turn-opening"}, EvidenceKind: "inferred",
	}
	adaptation := ActorStateSchemaAdaptation{ActorOps: []ActorStateRuntimeSchemaOp{{
		Op: "add", ActorID: "guide", Actor: ActorStateInitialActor{
			ID: "guide", Name: "向导", TemplateID: "npc", State: map[string]any{"态度": "警觉"},
		}, ValueSource: source,
	}}}
	ops, actorOps, _, _, err := buildStateSchemaMigration(base, base, map[string]any{actorStateRoot: map[string]any{}}, adaptation, "turn-opening")
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) == 0 || len(actorOps) == 0 {
		t.Fatalf("actor creation should emit metadata and state ops: ops=%#v actor_ops=%#v", ops, actorOps)
	}
	for _, op := range ops {
		if op.SourceID != source.SourceID {
			t.Fatalf("metadata op lost Batch provenance: %#v", op)
		}
	}
	for _, op := range actorOps {
		if op.SourceID != source.SourceID {
			t.Fatalf("actor value op lost Batch provenance: %#v", op)
		}
	}
	changes := stateSchemaAdaptationChanges(adaptation)
	if len(changes) != 1 || changes[0].ValueSource == nil || changes[0].ValueSource.SourceID != source.SourceID || changes[0].ValueSource.EvidenceKind != "inferred" {
		t.Fatalf("persisted schema adaptation change lost Actor evidence association: %#v", changes)
	}
}

func TestBuildStateSchemaMigrationRejectsLossyFallbackOverExistingValue(t *testing.T) {
	base := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Fields: []ActorStateField{{Name: "境界", Type: "string", Default: "未知"}},
	}}}
	target := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Fields: []ActorStateField{{Name: "境界", Type: "number", Default: 0}},
	}}}
	state := map[string]any{actorStateRoot: map[string]any{
		"protagonist": map[string]any{"template_id": "protagonist", "state": map[string]any{"境界": "筑基中期"}},
	}}
	adaptation := ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{
		Op: "fields", TemplateID: "protagonist",
		FieldOps: []ActorStateFieldSchemaOp{{Op: "replace", FieldID: "境界", Field: ActorStateField{Name: "境界", Type: "number", Default: 0}}},
	}}}
	if _, _, _, _, err := buildStateSchemaMigration(base, target, state, adaptation, "turn-opening"); err == nil {
		t.Fatal("an incompatible confirmed value must require an explicit migration value instead of silently falling back to a default")
	}
}

func TestApplyStateSchemaProposalMigratesOpeningStateAndKeepsAudit(t *testing.T) {
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
	status, err := store.ApplyStateSchemaProposal(story.ID, "main", turn.ID, ActorStateSchemaProposal{
		Summary: "根据真实开局调整修炼状态",
		Requirements: []ActorStateSchemaRequirementReview{
			{Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: turn.ID}, Requirement: "战力需要参与数值检定", EvidenceKind: "confirmed", ValuePolicy: ActorStateSchemaValuePolicyPreserve, ActorID: "protagonist", ExpectedType: "number", Decision: "replace", TemplateID: "protagonist", FieldID: "战力"},
			{Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: turn.ID}, Requirement: "首轮明确出现士气", EvidenceKind: "confirmed", ValuePolicy: ActorStateSchemaValuePolicySchemaOnly, ExpectedType: "number", Decision: "add", TemplateID: "protagonist", FieldID: "士气"},
		},
		Adaptation: ActorStateSchemaAdaptation{
			Summary: "根据真实开局调整修炼状态",
			TemplateOps: []ActorStateTemplateSchemaOp{{
				Op: "fields", TemplateID: "protagonist", FieldOps: []ActorStateFieldSchemaOp{
					{Op: "replace", FieldID: "功力", Field: ActorStateField{Name: "战力", Type: "number", Default: 0, Visibility: "visible"}, Reason: "数值检定需要"},
					{Op: "remove", FieldID: "旧秘密", Reason: "开局未采用"},
					{Op: "add", Field: ActorStateField{Name: "士气", Type: "number", Default: 10, Visibility: "visible"}, Reason: "首轮明确出现"},
				},
			}, {Op: "remove", TemplateID: "npc", Reason: "首轮临时角色已离场"}},
			ActorOps: []ActorStateRuntimeSchemaOp{{Op: "remove", ActorID: "guide", Reason: "不再长期追踪"}},
		},
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
	_, events, err := store.readStoryLocked(story.ID)
	if err != nil {
		t.Fatal(err)
	}
	var migrationDelta StateDeltaEvent
	for index := len(events) - 1; index >= 0; index-- {
		if events[index].Envelope.Type != StoryEventTypeStateDelta {
			continue
		}
		if err := mapToStruct(events[index].Raw, &migrationDelta); err != nil {
			t.Fatal(err)
		}
		break
	}
	seenSets := map[string]bool{}
	for _, op := range migrationDelta.ActorOps {
		if op.Op != "set" {
			continue
		}
		if op.Value == nil {
			t.Fatalf("persisted schema migration must not contain set(null): %#v", migrationDelta.ActorOps)
		}
		key := op.ActorID + "/" + op.FieldID
		if seenSets[key] {
			t.Fatalf("persisted schema migration must contain at most one set per Actor field: %#v", migrationDelta.ActorOps)
		}
		seenSets[key] = true
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
	_, err = store.ApplyStateSchemaProposal(story.ID, "main", turn.ID, ActorStateSchemaProposal{
		Requirements: []ActorStateSchemaRequirementReview{{
			Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: turn.ID}, Requirement: "临时向导不再长期追踪", EvidenceKind: "confirmed", ValuePolicy: ActorStateSchemaValuePolicySchemaOnly, Decision: "ignored", Reason: "临时角色已离场",
		}},
		Adaptation: ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{Op: "remove", TemplateID: "npc"}}},
	})
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

func TestFailedStateSchemaInitializationRequiresExplicitResetBeforeRetry(t *testing.T) {
	workspace := t.TempDir()
	store := NewStoreWithNovaDir(workspace, filepath.Join(workspace, ".denova"))
	story, err := store.CreateStory(CreateStoryRequest{
		Title: "显式重试", StateSchemaInitialization: &StateSchemaInitializationStatus{
			Mode: StateSchemaAdaptationModeAfterOpening, Status: StateSchemaInitializationWaitingOpening, BaseRevision: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "出发", Narrative: "主角离开营地。"})
	if err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := store.ClaimStateSchemaInitialization(story.ID, turn.ID); err != nil || !claimed {
		t.Fatalf("first claim failed: claimed=%t err=%v", claimed, err)
	}
	if err := store.MarkStateSchemaInitializationFailed(story.ID, turn.ID, fmt.Errorf("模型连接断开")); err != nil {
		t.Fatal(err)
	}
	if status, claimed, err := store.ClaimStateSchemaInitialization(story.ID, turn.ID); err != nil || claimed || status.Status != StateSchemaInitializationFailed {
		t.Fatalf("failed task must not auto-retry: status=%#v claimed=%t err=%v", status, claimed, err)
	}
	if _, err := store.ResetStateSchemaInitialization(story.ID); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := store.ClaimStateSchemaInitialization(story.ID, turn.ID); err != nil || !claimed {
		t.Fatalf("explicit reset should make the task claimable: claimed=%t err=%v", claimed, err)
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
		Summary: "现有生命字段完整覆盖规则", ReviewedLoreIDs: []string{"生命规则"},
		Requirements: []ActorStateSchemaRequirementReview{{
			Source: ActorStateSchemaRequirementSource{Kind: "lore", ID: "生命规则"}, Requirement: "生命为 0-100",
			EvidenceKind: "confirmed",
			ValuePolicy: ActorStateSchemaValuePolicySchemaOnly, ExpectedType: "number", Min: &minValue, Max: &maxValue, Decision: "covered", TemplateID: "protagonist", FieldID: "生命",
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
