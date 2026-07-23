package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/schema"

	"denova/internal/agent"
	"denova/internal/agentruntime"
	"denova/internal/interactive"
	"denova/internal/session"
)

func TestWritingDrainRetriesPendingRecoveryRefreshBeforeSessionSwitch(t *testing.T) {
	sessionDir := t.TempDir()
	store, err := session.NewStore(sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := store.Create("selected")
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.Create("target")
	if err != nil {
		t.Fatal(err)
	}
	externalStore, err := session.NewStore(sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	external, err := externalStore.Get(selected.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := external.Append(schema.UserMessage("canonical message appended by recovered structural commit")); err != nil {
		t.Fatal(err)
	}

	chat := agent.NewEphemeralChatService()
	t.Cleanup(func() { _ = chat.Close(context.Background()) })
	task := NewTask(func(context.Context, *Task, func(agent.Event)) {})
	<-task.Done()
	run := &writingTaskRun{task: task, runtime: ideChatRuntime{workspace: "/book", sess: selected}}
	action := agent.RuntimeRecoveryAction{
		Kind: agent.RuntimeRecoveryCompactContext, CommandID: "refresh-before-switch",
		OperationID: agentruntime.OperationID("operation-refresh-before-switch"),
	}
	application := &App{
		workspace: "/book", workspaceGeneration: 1, sessionStore: store,
		session: selected, chatService: chat, activeTask: task, activeWritingRun: run,
	}
	service := &ChatAppService{app: application}
	service.markRecoveryRefreshPending("/book", selected.ID, action)

	selectedPath := filepath.Join(sessionDir, selected.ID+".jsonl")
	unavailablePath := selectedPath + ".temporarily-unavailable"
	if err := os.Rename(selectedPath, unavailablePath); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SwitchSession(target.ID); err == nil {
		t.Fatal("session switch bypassed failed recovered-session refresh")
	}
	application.mu.RLock()
	stillSelected := application.session == selected
	application.mu.RUnlock()
	if !stillSelected {
		t.Fatal("failed refresh changed the selected Session")
	}
	if err := os.Rename(unavailablePath, selectedPath); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SwitchSession(target.ID); err != nil {
		t.Fatal(err)
	}
	history := selected.History()
	foundCanonical := false
	for _, entry := range history {
		if entry.Content == "canonical message appended by recovered structural commit" {
			foundCanonical = true
			break
		}
	}
	if !foundCanonical {
		t.Fatalf("shared Writing drain left the prior selected Session stale: %#v", history)
	}
	if retried, err := service.retryAnyRecoveryRefresh(context.Background(), "/book", selected.ID, selected.RefreshCanonical); retried || err != nil {
		t.Fatalf("successful switch left refresh obligation pending: retried=%t err=%v", retried, err)
	}
}

func TestClearSessionDrainsExactWritingTaskBeforeMutation(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.Create("active")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(schema.UserMessage("keep in display history")); err != nil {
		t.Fatal(err)
	}
	chat := agent.NewEphemeralChatService()
	t.Cleanup(func() { _ = chat.Close(context.Background()) })
	started := make(chan struct{})
	task := NewTask(func(ctx context.Context, _ *Task, _ func(agent.Event)) {
		close(started)
		<-ctx.Done()
	})
	<-started
	application := &App{
		workspace: "/book", workspaceGeneration: 1, sessionStore: store,
		session: sess, chatService: chat, activeTask: task,
	}
	application.activeWritingRun = &writingTaskRun{task: task, runtime: ideChatRuntime{workspace: "/book", sess: sess}}
	service := &ChatAppService{app: application}

	if err := service.ClearSession(); err != nil {
		t.Fatal(err)
	}
	if !task.Finished() || task.Status() != TaskAborted {
		t.Fatalf("structural mutation did not drain task: finished=%t status=%s", task.Finished(), task.Status())
	}
	if got := sess.GetEffectiveMessages(); len(got) != 0 {
		t.Fatalf("clear did not move effective context boundary: %+v", got)
	}
	if got := sess.History(); len(got) < 2 {
		t.Fatalf("clear physically deleted display history: %+v", got)
	}
}

func TestAppendInteractiveTurnDrainsExactBranchTaskBeforeMutation(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "barrier", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	chat := agent.NewEphemeralChatService()
	t.Cleanup(func() { _ = chat.Close(context.Background()) })
	directorTasks := newWorkspaceDirectorTaskGroup()
	t.Cleanup(directorTasks.Close)
	started := make(chan struct{})
	task := NewTask(func(ctx context.Context, _ *Task, _ func(agent.Event)) {
		close(started)
		<-ctx.Done()
	})
	<-started
	application := &App{
		workspace: workspace, workspaceGeneration: 1, interactive: store,
		chatService: chat, workspaceDirectorTasks: directorTasks,
		activeInteractiveRun: &interactiveTaskRun{task: task, info: InteractiveTaskInfo{
			Workspace: workspace, StoryID: story.ID, BranchID: "main",
		}},
	}
	service := &InteractiveAppService{app: application}

	turn, err := service.AppendInteractiveTurn(story.ID, "main", "open", "the door opens")
	if err != nil {
		t.Fatal(err)
	}
	if !task.Finished() || task.Status() != TaskAborted {
		t.Fatalf("structural mutation did not drain game task: finished=%t status=%s", task.Finished(), task.Status())
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turn.ID {
		t.Fatalf("turn was not committed after barrier: %+v", snapshot.CurrentTurn)
	}
}
