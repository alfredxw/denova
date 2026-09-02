package app

import (
	"context"
	"errors"
	"testing"

	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"denova/internal/interactive"
)

func TestUpdateInteractiveStoryTuningDoesNotAbortActiveTurn(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	story, task := activeStoryUpdateTestFixture(t, application)

	settings := interactive.StoryCheckSettings{RollModifier: 10}
	updated, err := application.UpdateInteractiveStory(story.ID, interactive.UpdateStoryRequest{CheckSettings: &settings})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CheckSettings.RollModifier != 10 {
		t.Fatalf("updated check settings = %#v", updated.CheckSettings)
	}
	if snapshot := task.Snapshot(); snapshot.CancelRequested || snapshot.Finished {
		t.Fatalf("tuning update interrupted the active turn: %#v", snapshot)
	}
}

func TestUpdateInteractiveStoryStructuralChangeRejectsWithoutAbortingActiveTurn(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	story, task := activeStoryUpdateTestFixture(t, application)

	policy := interactive.StoryStateSchemaPolicy{Mode: interactive.StoryStateSchemaModeFixedTemplate}
	_, err := application.UpdateInteractiveStory(story.ID, interactive.UpdateStoryRequest{StateSchemaPolicy: &policy})
	if !errors.Is(err, ErrAgentOperationActive) {
		t.Fatalf("structural update error = %v, want ErrAgentOperationActive", err)
	}
	if snapshot := task.Snapshot(); snapshot.CancelRequested || snapshot.Finished {
		t.Fatalf("rejected structural update interrupted the active turn: %#v", snapshot)
	}
}

func activeStoryUpdateTestFixture(t *testing.T, application *App) (interactive.StorySummary, *apptask.Task) {
	t.Helper()
	story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{
		Title: "Active story update", StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	task := apptask.New(func(ctx context.Context, _ *apptask.Task, emit func(agentrun.Event)) {
		<-ctx.Done()
		emit(agentrun.Event{Type: "aborted", Data: map[string]string{"reason": "test cleanup"}})
	})
	application.mu.Lock()
	application.activeInteractiveRun = &interactiveTaskRun{
		task: task,
		info: InteractiveTaskInfo{
			ProjectID: application.cfg.ProjectID,
			Workspace: application.workspace,
			StoryID:   story.ID,
			BranchID:  "main",
		},
	}
	application.mu.Unlock()
	t.Cleanup(func() {
		task.Abort()
		<-task.Done()
	})
	return story, task
}
