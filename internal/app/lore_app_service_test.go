package app

import (
	"context"
	apptask "denova/internal/app/task"
	"errors"
	"testing"
)

func TestLoreImageBatchRejectsAbortedTaskUntilWorkerExits(t *testing.T) {
	active, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer active.RejectStart(errors.New("test cleanup"))
	application := &App{
		activeLoreImageTask: active,
	}

	_, err = application.StartLoreImagesGenerateTask(context.Background(), LoreImagesGenerateRequest{ItemIDs: []string{"hero"}})
	if !errors.Is(err, ErrLoreImageTaskRunning) {
		t.Fatalf("StartLoreImagesGenerateTask error = %v, want %v", err, ErrLoreImageTaskRunning)
	}
}

func TestAbortLoreImagesGenerateTaskForWorkspaceChecksIdentityWithCommand(t *testing.T) {
	workspace := t.TempDir()
	staleWorkspace := t.TempDir()
	task, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer task.RejectStart(errors.New("test cleanup"))
	application := &App{workspace: workspace, activeLoreImageTask: task}

	err = application.AbortLoreImagesGenerateTaskForWorkspace(staleWorkspace)
	if !errors.Is(err, ErrWorkspaceChanged) {
		t.Fatalf("stale abort error = %v, want %v", err, ErrWorkspaceChanged)
	}
	if task.Snapshot().CancelRequested {
		t.Fatal("stale workspace abort canceled the active task")
	}

	if err := application.AbortLoreImagesGenerateTaskForWorkspace(workspace); err != nil {
		t.Fatalf("matching workspace abort failed: %v", err)
	}
	if !task.Snapshot().CancelRequested {
		t.Fatal("matching workspace abort did not cancel the active task")
	}
}
