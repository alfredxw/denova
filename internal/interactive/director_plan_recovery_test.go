package interactive

import (
	"reflect"
	"testing"

	"denova/internal/interactive/director"
)

func TestRecoverInterruptedDirectorRunsPreservesPlanAndClearsRunningStatus(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "recovery"})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID:  "main",
		User:      "continue",
		Narrative: "the story advances",
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	token, err := store.DirectorPlanRunToken(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkDirectorPlanRunStarted(story.ID, "main", token, turn.ID); err != nil {
		t.Fatal(err)
	}
	waitingStory, err := store.CreateStory(CreateStoryRequest{Title: "waiting recovery"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(waitingStory.ID, AppendTurnWithStateRequest{BranchID: "main", User: "begin", Narrative: "opening"}); err != nil {
		t.Fatal(err)
	}
	emptyStory, err := store.CreateStory(CreateStoryRequest{Title: "not started"})
	if err != nil {
		t.Fatal(err)
	}

	recovered, err := store.RecoverInterruptedDirectorRuns()
	if err != nil {
		t.Fatal(err)
	}
	if recovered != 2 {
		t.Fatalf("recovered runs = %d, want 2", recovered)
	}
	after, err := store.DirectorPlan(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.Metadata.LastRun == nil || after.Metadata.LastRun.Status != director.PlanStatusFailed {
		t.Fatalf("interrupted Director status = %#v, want failed", after.Metadata.LastRun)
	}
	if !reflect.DeepEqual(after.Docs, before.Docs) {
		t.Fatal("Director recovery changed the existing plan documents")
	}
	waitingStatus, err := store.DirectorPlanStatus(waitingStory.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if waitingStatus.Status != director.PlanStatusFailed {
		t.Fatalf("persisted opening without an owner stayed pending: %#v", waitingStatus)
	}
	emptyStatus, err := store.DirectorPlanStatus(emptyStory.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if emptyStatus.Status != director.PlanStatusWaitingOpening {
		t.Fatalf("empty story status = %q, want waiting_opening", emptyStatus.Status)
	}
}
