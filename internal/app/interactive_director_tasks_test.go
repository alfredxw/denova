package app

import (
	"context"
	"fmt"
	"testing"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
)

func TestLegacyStoryDirectorMaintenanceKeepsStateSchemaFixed(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	stateSystem := interactive.GeneratedStoryActorStateCore()
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "旧故事", ActorState: &stateSystem})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "继续", Narrative: "故事继续。"})
	if err != nil {
		t.Fatal(err)
	}
	var maintenanceTasks []string
	generator := func(_ context.Context, _ *config.Config, _ *book.State, toolContext agent.InteractiveStoryToolContext, _ string) (string, error) {
		maintenanceTasks = append(maintenanceTasks, toolContext.MaintenanceTask)
		return "", fmt.Errorf("stop after inspecting Director maintenance task")
	}
	conversation := newInteractiveConversation(store, "", workspace, story.ID, "main", turn.User, story.ReplyTargetChars, &config.Config{}).bindDirectorRuntime(newWorkspaceDirectorTaskGroup(), generator)
	<-startInteractiveDirectorMaintenanceTask(&config.Config{}, book.NewState(workspace), conversation, turn, nil, true)

	if len(maintenanceTasks) != 1 || maintenanceTasks[0] != interactiveDirectorTaskDirectorPlanUpdate {
		t.Fatalf("legacy story Director tasks = %#v, want only %q", maintenanceTasks, interactiveDirectorTaskDirectorPlanUpdate)
	}
	storyCtx, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if storyCtx.Meta.StateSchemaPolicy == nil || storyCtx.Meta.StateSchemaPolicy.Mode != interactive.StoryStateSchemaModeFixedTemplate || storyCtx.Meta.ActorStateSchema == nil || storyCtx.Meta.ActorStateSchema.Revision != 1 {
		t.Fatalf("legacy story must keep revision 1 fixed schema: %#v", storyCtx.Meta)
	}
}

func TestWorkspaceDirectorTaskGroupCancelsAndWaits(t *testing.T) {
	tasks := newWorkspaceDirectorTaskGroup()
	started := make(chan struct{})
	finished := make(chan struct{})
	done, ok := tasks.Go(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(finished)
	})
	if !ok {
		t.Fatal("new workspace task group rejected its first task")
	}
	<-started
	tasks.Close()
	<-done
	<-finished

	if _, ok := tasks.Go(func(context.Context) {}); ok {
		t.Fatal("closed workspace task group accepted a new task")
	}
}

func TestWorkspaceDirectorTaskGroupSerializesSameBranch(t *testing.T) {
	tasks := newWorkspaceDirectorTaskGroup()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})

	firstDone, ok := tasks.GoKeyed("story-1:main", func(context.Context) {
		close(firstStarted)
		<-releaseFirst
	})
	if !ok {
		t.Fatal("first keyed task rejected")
	}
	<-firstStarted
	secondDone, ok := tasks.GoKeyed("story-1:main", func(context.Context) {
		close(secondStarted)
	})
	if !ok {
		t.Fatal("second keyed task rejected")
	}
	select {
	case <-secondStarted:
		t.Fatal("second task started before the first task completed")
	default:
	}
	close(releaseFirst)
	<-firstDone
	<-secondStarted
	<-secondDone
	tasks.Close()
}

func TestWorkspaceDirectorTaskGroupWaitKeyWaitsForQueuedWork(t *testing.T) {
	tasks := newWorkspaceDirectorTaskGroup()
	release := make(chan struct{})
	_, ok := tasks.GoKeyed("story-1:main", func(context.Context) { <-release })
	if !ok {
		t.Fatal("keyed task rejected")
	}
	waitDone := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				waitDone <- fmt.Errorf("WaitKey test goroutine panic: %v", recovered)
			}
		}()
		waitDone <- tasks.WaitKey(context.Background(), "story-1:main")
	}()
	select {
	case err := <-waitDone:
		t.Fatalf("WaitKey returned before queued work completed: %v", err)
	default:
	}
	close(release)
	if err := <-waitDone; err != nil {
		t.Fatal(err)
	}
	tasks.Close()
}
