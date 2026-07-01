package app

import (
	"errors"
	"testing"
)

func TestLoreImageBatchRejectsAbortedTaskUntilWorkerExits(t *testing.T) {
	application := &App{
		activeLoreImageTask: &Task{status: TaskAborted},
	}

	_, err := application.StartLoreImagesGenerateTask(LoreImagesGenerateRequest{ItemIDs: []string{"hero"}})
	if !errors.Is(err, ErrLoreImageTaskRunning) {
		t.Fatalf("StartLoreImagesGenerateTask error = %v, want %v", err, ErrLoreImageTaskRunning)
	}
}
