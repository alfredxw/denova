package app

import (
	"context"
	"sync"
	"testing"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
)

func TestOpeningTurnStaysVisibleWhileStateSchemaInitializesBeforeMaintenance(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStoreWithNovaDir(workspace, t.TempDir())
	stateSystem := interactive.StoryDirectorActorStateSystem{
		Templates:     []interactive.ActorStateTemplate{{ID: "protagonist", Name: "主角", Fields: []interactive.ActorStateField{{Name: "状态", Type: "string", Default: "平静"}}}},
		InitialActors: []interactive.ActorStateInitialActor{{ID: "protagonist", Name: "主角", TemplateID: "protagonist"}},
	}
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:      "异步开局",
		ActorState: &stateSystem,
		StateSchemaInitialization: &interactive.StateSchemaInitializationStatus{
			Mode: interactive.StateSchemaAdaptationModeAfterOpening, Status: interactive.StateSchemaInitializationWaitingOpening, BaseRevision: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "推门", Narrative: "门外是正在燃烧的长街。"})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var tasks []string
	generator := func(_ context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, _ string) (string, error) {
		tasks = append(tasks, toolContext.MaintenanceTask)
		if toolContext.MaintenanceTask == "state_schema_initialization" {
			once.Do(func() { close(started) })
			<-release
			return `{"summary":"补充危机压力","template_ops":[{"op":"fields","template_id":"protagonist","field_ops":[{"op":"add","field":{"name":"危机压力","type":"number","default":1,"min":0,"max":10,"visibility":"visible"},"reason":"首轮出现燃烧街道"}]}]}`, nil
		}
		return "maintenance complete", nil
	}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{}).bindDirectorRuntime(newWorkspaceDirectorTaskGroup(), generator)
	done := startInteractiveDirectorMaintenanceTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil, false)
	<-started
	running, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if running.CurrentTurn == nil || running.CurrentTurn.Narrative != turn.Narrative || running.StateSchemaInitialization == nil || running.StateSchemaInitialization.Status != interactive.StateSchemaInitializationRunning {
		t.Fatalf("opening must stay visible while schema adapts: %#v", running)
	}
	if _, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: turn.ID}); err == nil {
		t.Fatal("branch creation must wait until state schema migration completes")
	}
	close(release)
	<-done
	completed, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if completed.ActorStateSchema == nil || completed.ActorStateSchema.Revision != 2 || completed.StateSchemaInitialization == nil || completed.StateSchemaInitialization.Status != interactive.StateSchemaInitializationReady {
		t.Fatalf("schema initialization did not complete: %#v", completed.StateSchemaInitialization)
	}
	if len(tasks) == 0 || tasks[0] != "state_schema_initialization" {
		t.Fatalf("state schema must run before other maintenance: %#v", tasks)
	}
}
