package interactive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStructuredRuleStateChangeUsesExactFieldID(t *testing.T) {
	fieldID := "体力与行动.值"
	system := normalizeActorStateSystem(StoryDirectorActorStateSystem{
		Templates:     []ActorStateTemplate{{ID: "protagonist", Fields: []ActorStateField{{Name: fieldID, Type: "number", Default: 5.0}}}},
		InitialActors: []ActorStateInitialActor{{ID: "protagonist", TemplateID: "protagonist", State: map[string]any{fieldID: 5.0}}},
	})
	state := initialStoryState()
	ops, actorOps, err := BuildActorStateInitialChanges(system, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		applyStateOp(state, op)
	}
	for _, op := range actorOps {
		applyActorStateOp(state, op)
	}
	resolution := RuleResolution{ID: "rr-1", Result: RuleResult{StateChanges: []TurnStateChange{{ActorID: "protagonist", FieldID: fieldID, Change: -2}}}}
	_, applied := applyRuleStateConsumptionV2(state, system, "turn-1", &resolution, RuleStateConsumptionModeHybridAuto)
	if len(applied) != 1 || applied[0].FieldID != fieldID {
		t.Fatalf("structured rule change should preserve exact field ID: %#v", applied)
	}
	if got := actorStateFieldValue(state, "protagonist", fieldID); got != 3.0 {
		t.Fatalf("structured rule change should update the flat state key, got %#v", got)
	}
	data, err := json.Marshal(resolution.Result.StateChanges[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"path"`) || !strings.Contains(string(data), `"field_id":"体力与行动.值"`) {
		t.Fatalf("new rule interface must not emit paths: %s", data)
	}
}

func TestLegacyStoryFreezesSchemaAfterBackupOnFirstLoad(t *testing.T) {
	root := t.TempDir()
	novaDir := filepath.Join(root, ".nova")
	legacyStore := NewStore(root)
	story, err := legacyStore.CreateStory(CreateStoryRequest{
		Title: "旧故事",
		InitialStateOps: []StateOp{
			{Op: "set", Path: "actors.protagonist.id", Value: "protagonist"},
			{Op: "set", Path: "actors.protagonist.template_id", Value: "protagonist"},
			{Op: "set", Path: "actors.protagonist.state.旧系统.自定义值", Value: "仍然存在"},
			{Op: "set", Path: "actors.orphan.id", Value: "orphan"},
			{Op: "set", Path: "actors.orphan.name", Value: "旧角色"},
			{Op: "set", Path: "actors.orphan.state.旧字段", Value: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyData, err := os.ReadFile(legacyStore.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(legacyData), "actor_state_schema") {
		t.Fatal("legacy fixture unexpectedly contains a frozen schema")
	}

	migratedStore := NewStoreWithNovaDir(root, novaDir)
	snapshot, err := migratedStore.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActorStateSchema == nil || len(snapshot.ActorStateSchema.System.Templates) == 0 {
		t.Fatalf("first load should freeze the story schema: %#v", snapshot.ActorStateSchema)
	}
	if got := actorStateFieldValue(snapshot.State, "protagonist", "旧系统.自定义值"); got != "仍然存在" {
		t.Fatalf("unmatched legacy state must be preserved as a flat story-only field, got %#v", got)
	}
	legacyTemplate := actorStateTemplateByID(snapshot.ActorStateSchema.System, "protagonist")
	if field, ok := actorStateFieldByID(legacyTemplate, "旧系统.自定义值"); !ok || field.Name != "旧系统.自定义值" {
		t.Fatalf("unmatched legacy field should be added to the frozen story schema: %#v", legacyTemplate.Fields)
	}
	if got := actorStateFieldValue(snapshot.State, "orphan", "旧字段"); got != true {
		t.Fatalf("legacy actor without a template must keep its state, got %#v", got)
	}
	if got := getPath(snapshot.State, "actors.orphan.template_id"); got != "legacy_orphan" {
		t.Fatalf("legacy actor should be bound to a story-only template, got %#v", got)
	}
	legacyActorTemplate := actorStateTemplateByID(snapshot.ActorStateSchema.System, "legacy_orphan")
	if field, ok := actorStateFieldByID(legacyActorTemplate, "旧字段"); !ok || field.Type != "bool" {
		t.Fatalf("legacy actor template should infer and expose its fields: %#v", legacyActorTemplate.Fields)
	}
	backups, err := filepath.Glob(filepath.Join(novaDir, "backups", "state-system-v6", "*", "story-"+story.ID+".jsonl"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("expected one pre-migration story backup, paths=%#v err=%v", backups, err)
	}
	backupData, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(backupData) != string(legacyData) {
		t.Fatal("migration backup must preserve the original JSONL byte-for-byte")
	}
	currentData, err := os.ReadFile(legacyStore.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	if string(currentData) != string(legacyData) {
		t.Fatal("freezing a legacy schema must not rewrite historical JSONL")
	}
	if _, err := os.Stat(migratedStore.actorStateSchemaPath(story.ID)); err != nil {
		t.Fatalf("legacy story schema sidecar was not persisted: %v", err)
	}
}

func TestLegacyStoryReplaysFrozenInitialActorsWithoutRewritingHistory(t *testing.T) {
	root := t.TempDir()
	novaDir := filepath.Join(root, ".nova")
	legacyStore := NewStore(root)
	story, err := legacyStore.CreateStory(CreateStoryRequest{Title: "没有 Actor Delta 的旧故事"})
	if err != nil {
		t.Fatal(err)
	}
	legacyData, err := os.ReadFile(legacyStore.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}

	migratedStore := NewStoreWithNovaDir(root, novaDir)
	snapshot, err := migratedStore.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActorStateSchema == nil || len(snapshot.ActorStateSchema.System.InitialActors) == 0 {
		t.Fatalf("legacy story should freeze a schema with initial Actors: %#v", snapshot.ActorStateSchema)
	}
	actors, _ := snapshot.State[actorStateRoot].(map[string]any)
	for _, initial := range snapshot.ActorStateSchema.System.InitialActors {
		record, _ := actors[initial.ID].(map[string]any)
		if record == nil || record["template_id"] != initial.TemplateID {
			t.Fatalf("frozen initial Actor %q should be available in every replayed snapshot: %#v", initial.ID, record)
		}
	}
	currentData, err := os.ReadFile(legacyStore.storyPath(story.ID))
	if err != nil {
		t.Fatal(err)
	}
	if string(currentData) != string(legacyData) {
		t.Fatal("replaying frozen initial Actors must not rewrite legacy history")
	}
}

func TestStoryFreezesChineseActorStateFieldIDs(t *testing.T) {
	system := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{{
			ID:   "protagonist",
			Name: "主角",
			Fields: []ActorStateField{{
				Name:    "当前可用能力",
				Type:    "list",
				Default: []any{},
			}},
		}},
		InitialActors: []ActorStateInitialActor{{ID: "protagonist", Name: "主角", TemplateID: "protagonist"}},
	}
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "冻结状态", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}

	// Editing the reusable template after story creation must not change the
	// story-local field identity.
	system.Templates[0].Fields[0].Name = "当前能力"
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "查看能力",
		Narrative: "主角确认了道渊的存在。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			StateUpdates: []StateUpdate{{
				Op: TurnStateUpdateReplace, Path: "/protagonist/当前可用能力", Value: []any{"道渊"},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if turn.StateDelta == nil || turn.StateDelta.SchemaVersion != stateOpSchemaVersion || len(turn.StateDelta.ActorOps) != 1 {
		t.Fatalf("new turn should persist a v2 structured Actor op: %#v", turn.StateDelta)
	}
	if op := turn.StateDelta.ActorOps[0]; op.ActorID != "protagonist" || op.FieldID != "当前可用能力" {
		t.Fatalf("structured Actor op must preserve exact field ID: %#v", op)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := actorStateFieldValue(snapshot.State, "protagonist", "当前可用能力"); len(got.([]any)) != 1 || got.([]any)[0] != "道渊" {
		t.Fatalf("frozen Chinese field id was not persisted: %#v", snapshot.State)
	}
	if snapshot.ActorStateSchema == nil || snapshot.ActorStateSchema.System.Templates[0].Fields[0].Name != "当前可用能力" {
		t.Fatalf("snapshot should expose the frozen schema: %#v", snapshot.ActorStateSchema)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "能力支线"})
	if err != nil {
		t.Fatal(err)
	}
	branchSnapshot, err := store.Snapshot(story.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if branchSnapshot.ActorStateSchema == nil || branchSnapshot.ActorStateSchema.System.Templates[0].Fields[0].Name != "当前可用能力" {
		t.Fatalf("branches must share the story's frozen schema: %#v", branchSnapshot.ActorStateSchema)
	}

	second, err := store.CreateStory(CreateStoryRequest{Title: "改名后状态", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	secondSnapshot, err := store.Snapshot(second.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := secondSnapshot.ActorStateSchema.System.Templates[0].Fields[0].Name; got != "当前能力" {
		t.Fatalf("new story should use renamed field id, got %q", got)
	}
}

func TestActorStateFieldIDSupportsNonPathPunctuation(t *testing.T) {
	fieldID := "精神与意志.状态"
	system := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{{
			ID:     "protagonist",
			Fields: []ActorStateField{{Name: fieldID, Type: "string"}},
		}},
		InitialActors: []ActorStateInitialActor{{ID: "protagonist", TemplateID: "protagonist"}},
	}
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "特殊字段", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "坚持",
		Narrative: "主角稳住了心神。",
		TurnResult: &TurnResult{
			Choices:      testTurnChoices(),
			StateUpdates: []StateUpdate{{Op: TurnStateUpdateReplace, Path: formatStateUpdatePath([]string{"protagonist", fieldID}), Value: "镇定"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := actorStateFieldValue(snapshot.State, "protagonist", fieldID); got != "镇定" {
		t.Fatalf("punctuated field id should be an exact map key, got %#v state=%#v", got, snapshot.State)
	}
}

func TestActorStateRejectsPathSeparatorInFieldNames(t *testing.T) {
	for _, fieldID := range []string{"精神/意志状态", "精神／意志状态"} {
		t.Run(fieldID, func(t *testing.T) {
			_, err := NewActorStateLibrary(t.TempDir()).Create(ActorStateModule{
				ID:   "invalid-field-name",
				Name: "非法字段名",
				ActorState: StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
					ID:     "protagonist",
					Name:   "主角",
					Fields: []ActorStateField{{Name: fieldID, Type: "string"}},
				}}},
			})
			if err == nil || !strings.Contains(err.Error(), "路径分隔符") {
				t.Fatalf("field name %q should reject the path separator, got %v", fieldID, err)
			}
		})
	}
}

func TestCreateStoryRejectsPathSeparatorInActorStateFieldNames(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID:     "protagonist",
		Name:   "主角",
		Fields: []ActorStateField{{Name: "当前精神/意志状态", Type: "string"}},
	}}}
	_, err := NewStore(t.TempDir()).CreateStory(CreateStoryRequest{Title: "非法状态字段", ActorState: &system})
	if err == nil || !strings.Contains(err.Error(), "路径分隔符") {
		t.Fatalf("story creation must reject slash-delimited field names, got %v", err)
	}
}

func TestBuiltinActorStateFieldNamesExcludePathSeparator(t *testing.T) {
	systems := []struct {
		name   string
		system StoryDirectorActorStateSystem
	}{{name: "default", system: defaultActorStateSystem()}}
	for _, module := range builtinActorStateModules() {
		systems = append(systems, struct {
			name   string
			system StoryDirectorActorStateSystem
		}{name: module.ID, system: module.ActorState})
	}
	for _, item := range systems {
		for _, template := range item.system.Templates {
			for _, field := range template.Fields {
				if strings.Contains(normalizeActorStateFieldName(field.Name), "/") {
					t.Fatalf("built-in state system %s template %s contains path separator in field %q", item.name, template.ID, field.Name)
				}
			}
		}
	}
}

func TestActorStateRejectsDuplicateNormalizedNames(t *testing.T) {
	_, err := NewActorStateLibrary(t.TempDir()).Create(ActorStateModule{
		ID:   "duplicate-fields",
		Name: "重复字段",
		ActorState: StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
			ID: "protagonist",
			Fields: []ActorStateField{
				{Name: "当前状态", Type: "string"},
				{Name: "  当前状态  ", Type: "string"},
			},
		}}},
	})
	if err == nil || !strings.Contains(err.Error(), "状态名称重复") {
		t.Fatalf("duplicate normalized names should be rejected, got %v", err)
	}
}

func TestLegacyActorStatePathReplaysIntoFrozenFieldID(t *testing.T) {
	system := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{{
			ID:     "protagonist",
			Fields: []ActorStateField{{Name: "当前可用能力", Type: "list", LegacyPath: "abilities.available"}},
		}},
		InitialActors: []ActorStateInitialActor{{ID: "protagonist", TemplateID: "protagonist"}},
	}
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:      "旧状态回放",
		ActorState: &system,
		InitialStateOps: []StateOp{
			{Op: "set", Path: "actors.protagonist.id", Value: "protagonist"},
			{Op: "set", Path: "actors.protagonist.template_id", Value: "protagonist"},
			{Op: "set", Path: "actors.protagonist.state.abilities.available", Value: []any{"旧能力"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := actorStateFieldValue(snapshot.State, "protagonist", "当前可用能力"); len(got.([]any)) != 1 || got.([]any)[0] != "旧能力" {
		t.Fatalf("legacy path was not adapted to frozen field id: %#v", snapshot.State)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "掌握新能力",
		Narrative: "主角在旧能力之上掌握了新能力。",
		TurnResult: &TurnResult{
			Choices:      testTurnChoices(),
			StateUpdates: []StateUpdate{{Op: TurnStateUpdateReplace, Path: "/protagonist/当前可用能力", Value: []any{"旧能力", "新能力"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if turn.StateDelta == nil || len(turn.StateDelta.ActorOps) != 1 || turn.StateDelta.ActorOps[0].FieldID != "当前可用能力" {
		t.Fatalf("turn after a v1 delta should persist a v2 structured op: %#v", turn.StateDelta)
	}
	snapshot, err = store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	abilities, _ := actorStateFieldValue(snapshot.State, "protagonist", "当前可用能力").([]any)
	if len(abilities) != 2 || abilities[0] != "旧能力" || abilities[1] != "新能力" {
		t.Fatalf("mixed v1/v2 replay should preserve and advance state: %#v", abilities)
	}
}
