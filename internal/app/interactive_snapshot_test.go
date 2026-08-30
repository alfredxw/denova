package app

import (
	"context"
	"testing"

	agentexecution "denova/internal/agents/execution"
	"denova/internal/interactive"
)

func TestInteractiveSnapshotKeepsCanonicalStoryWhenAgentProjectionFails(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "canonical story", StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID: "main", User: "continue", Narrative: "The committed turn remains visible.",
	}); err != nil {
		t.Fatal(err)
	}

	runtime := agentexecution.NewEphemeralRuntime()
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	service := &InteractiveAppService{app: &App{
		workspace: workspace, interactive: store, executionRuntime: runtime,
	}}

	snapshot, err := service.InteractiveSnapshot(story.ID, "main")
	if err != nil {
		t.Fatalf("canonical Story snapshot must not depend on Agent runtime projection: %v", err)
	}
	if snapshot.TurnCount != 1 || len(snapshot.Turns) != 1 || snapshot.Turns[0].Narrative != "The committed turn remains visible." {
		t.Fatalf("canonical Story snapshot changed after Agent projection failed: %#v", snapshot)
	}
	if snapshot.ContextCompaction != nil {
		t.Fatalf("unavailable Agent metadata must be omitted: %#v", snapshot.ContextCompaction)
	}
}

func TestInteractiveProductDefaultsEnableGameAgentPlanning(t *testing.T) {
	service := &InteractiveAppService{app: &App{}}
	req, err := service.withStoryDirectorDefaults(interactive.CreateStoryRequest{Title: "planning default"})
	if err != nil {
		t.Fatal(err)
	}
	if req.PlanningMode != interactive.StoryPlanningModeEnabled {
		t.Fatalf("product planning default = %q, want enabled", req.PlanningMode)
	}

	explicit := interactive.CreateStoryRequest{Title: "free play", PlanningMode: interactive.StoryPlanningModeDisabled}
	req, err = service.withStoryDirectorDefaults(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if req.PlanningMode != interactive.StoryPlanningModeDisabled {
		t.Fatalf("explicit disabled planning mode changed to %q", req.PlanningMode)
	}
}
