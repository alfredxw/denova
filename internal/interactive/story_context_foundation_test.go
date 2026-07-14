package interactive

import "testing"

func TestEnsureStoryContextFoundationMigratesLegacyStoryActorOnce(t *testing.T) {
	store := NewStore(t.TempDir())
	legacy := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			{ID: DefaultActorID, Name: "主角", Fields: []ActorStateField{{Name: "当前处境", Type: "string", Visibility: "visible"}}},
			{ID: ActorStateImportantCharacterTemplateID, Name: "重要角色", Fields: []ActorStateField{
				{Name: "当前状态", Type: "string", Visibility: "visible"},
				{Name: "当前地点/去向", Type: "string", Visibility: "visible"},
				{Name: "当前目标/压力", Type: "string", Visibility: "spoiler"},
			}},
		},
		InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID}},
	}
	story, err := store.CreateStory(CreateStoryRequest{Title: "旧故事", ActorState: &legacy})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "继续", Narrative: "主角抵达废弃料场。",
		Ops: []StateOp{
			{Op: "set", Path: "actors.story.id", Value: DefaultStoryContextActorID},
			{Op: "set", Path: "actors.story.name", Value: "故事上下文"},
			{Op: "set", Path: "actors.story.template_id", Value: ActorStateImportantCharacterTemplateID},
			{Op: "set", Path: "actors.story.role", Value: "story_context"},
		},
		ActorOps: []ActorStateOp{
			{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: "当前状态", Value: "追踪黑衣人"},
			{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: "当前地点/去向", Value: "废弃灵木料场"},
			{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: "当前目标/压力", Value: "敌人相距十五丈"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	migrated, err := store.EnsureStoryContextFoundation(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !migrated {
		t.Fatal("legacy story should be migrated")
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActorStateSchema == nil || actorStateTemplateByID(snapshot.ActorStateSchema.System, ActorStateStoryContextTemplateID).ID == "" {
		t.Fatalf("story_context template missing after migration: %#v", snapshot.ActorStateSchema)
	}
	storyIndex := actorStateInitialActorIndex(snapshot.ActorStateSchema.System.InitialActors, DefaultStoryContextActorID)
	if storyIndex < 0 || snapshot.ActorStateSchema.System.InitialActors[storyIndex].TemplateID != ActorStateStoryContextTemplateID {
		t.Fatalf("story initial actor was not rebound: %#v", snapshot.ActorStateSchema.System.InitialActors)
	}
	if got := getPath(snapshot.State, "actors.story.template_id"); got != ActorStateStoryContextTemplateID {
		t.Fatalf("runtime story template = %#v", got)
	}
	for path, want := range map[string]any{
		"actors.story.state.当前事件":   "追踪黑衣人",
		"actors.story.state.当前详细地点": "废弃灵木料场",
		"actors.story.state.当前场景压力": "敌人相距十五丈",
	} {
		if got := getPath(snapshot.State, path); got != want {
			t.Fatalf("%s = %#v, want %#v", path, got, want)
		}
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turn.ID {
		t.Fatalf("migration must not replace the current turn: %#v", snapshot.CurrentTurn)
	}
	revision := snapshot.ActorStateSchema.Revision
	storyContext, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	head := storyContext.Meta.Branches["main"].Head

	migrated, err = store.EnsureStoryContextFoundation(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if migrated {
		t.Fatal("foundation migration must be idempotent")
	}
	afterContext, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if afterContext.Snapshot.ActorStateSchema.Revision != revision || afterContext.Meta.Branches["main"].Head != head {
		t.Fatalf("idempotent call changed persisted state: revision=%d/%d head=%s/%s", afterContext.Snapshot.ActorStateSchema.Revision, revision, afterContext.Meta.Branches["main"].Head, head)
	}
}
