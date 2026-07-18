package app

import (
	"context"
	"testing"

	"denova/internal/agent"
)

func TestActiveInteractiveTaskForScopesRecoveryToCurrentStoryBranchAndWorkspace(t *testing.T) {
	workspace := t.TempDir()
	release := make(chan struct{})
	task := NewTask(func(ctx context.Context, _ *Task, _ func(agent.Event)) {
		select {
		case <-release:
		case <-ctx.Done():
		}
	})
	service := &InteractiveAppService{app: &App{workspace: workspace}}
	if !service.bindActiveInteractiveTask(task, InteractiveTaskInfo{
		Workspace:            workspace,
		StoryID:              "story-1",
		BranchID:             "branch-1",
		Message:              "推开石门",
		RegenerateFromTurnID: "turn-old",
	}) {
		t.Fatal("bind active interactive task failed")
	}
	t.Cleanup(func() {
		close(release)
		task.Abort()
	})

	gotTask, info := service.ActiveInteractiveTaskFor("story-1", "branch-1")
	if gotTask != task || info.TaskID != task.ID() || info.Message != "推开石门" || info.RegenerateFromTurnID != "turn-old" {
		t.Fatalf("matching task = %p info=%#v", gotTask, info)
	}
	if otherTask, _ := service.ActiveInteractiveTaskFor("story-2", "branch-1"); otherTask != nil {
		t.Fatalf("different story recovered task %s", otherTask.ID())
	}
	if otherTask, _ := service.ActiveInteractiveTaskFor("story-1", "branch-2"); otherTask != nil {
		t.Fatalf("different branch recovered task %s", otherTask.ID())
	}

	service.app.mu.Lock()
	service.app.workspace = t.TempDir()
	service.app.mu.Unlock()
	if otherTask, _ := service.ActiveInteractiveTaskFor("story-1", "branch-1"); otherTask != nil {
		t.Fatalf("different workspace recovered task %s", otherTask.ID())
	}
}
