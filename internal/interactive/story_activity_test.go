package interactive

import (
	"context"
	"testing"
	"time"
)

func TestActivityCountsRetainedTurnsAcrossBranchesAndVersions(t *testing.T) {
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "Activity"})
	if err != nil {
		t.Fatal(err)
	}
	appendTurn := func(branch string) TurnEvent {
		t.Helper()
		turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: branch, User: "Continue", Narrative: "The story continues."})
		if err != nil {
			t.Fatal(err)
		}
		return turn
	}
	first := appendTurn("main")
	second := appendTurn("main")
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: first.ID, Title: "Alternative"})
	if err != nil {
		t.Fatal(err)
	}
	old := appendTurn(branch.ID)
	read := func(want int) StoryActivity {
		t.Helper()
		activity, err := store.ReadActivity(context.Background(), story.ID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if len(activity.TurnTimes) != want || activity.NewBranches != 1 {
			t.Fatalf("activity = %+v, want %d distinct retained turns and one branch", activity, want)
		}
		return activity
	}
	read(3) // The inherited first turn must not count twice.
	replacement, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: branch.ID, ExpectedParentID: &old.ID, ReplaceTurnID: old.ID,
		User: "Continue", Narrative: "A different outcome.",
	})
	if err != nil {
		t.Fatal(err)
	}
	read(3) // Regenerating one turn does not add a fourth retained turn.
	if err := store.RewindToTurnParent(story.ID, RewindTurnRequest{BranchID: "main", TurnID: second.ID}); err != nil {
		t.Fatal(err)
	}
	read(2)
	if err := store.RewindToTurnParent(story.ID, RewindTurnRequest{BranchID: branch.ID, TurnID: replacement.ID}); err != nil {
		t.Fatal(err)
	}
	read(1)
	indexBefore, err := store.Index()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(store.Root())
	defer reopened.Close()
	from, err := time.Parse(time.RFC3339Nano, first.Ts)
	if err != nil {
		t.Fatal(err)
	}
	activity, err := reopened.ReadActivity(context.Background(), story.ID, from, from.Add(time.Nanosecond))
	if err != nil || len(activity.TurnTimes) != 1 || activity.NewBranches != 0 {
		t.Fatalf("reopened exact timestamp interval = %+v, err=%v", activity, err)
	}
	activity, err = reopened.ReadActivity(context.Background(), story.ID, from.Add(-time.Nanosecond), from)
	if err != nil || len(activity.TurnTimes) != 0 {
		t.Fatalf("exclusive interval end = %+v, err=%v", activity, err)
	}
	indexAfter, err := reopened.Index()
	if err != nil || indexAfter.CurrentStoryID != indexBefore.CurrentStoryID {
		t.Fatalf("activity changed story selection: %+v, err=%v", indexAfter, err)
	}
}

func TestActivityReadsBeyondDisplayWindowAndIgnoresSideEvents(t *testing.T) {
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "Long history"})
	if err != nil {
		t.Fatal(err)
	}
	turns := make([]AppendTurnRequest, 550)
	for i := range turns {
		turns[i] = AppendTurnRequest{User: "Continue", Narrative: "Retained narrative."}
	}
	appendStoryTurns(t, store, story.ID, "main", turns)
	appendModelInvisibleStoryEvents(t, store, story.ID, "main", 800)
	activity, err := store.ReadActivity(context.Background(), story.ID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil || len(activity.TurnTimes) != 550 {
		t.Fatalf("long-history activity count=%d err=%v", len(activity.TurnTimes), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.ReadActivity(ctx, story.ID, time.Time{}, time.Now()); err != context.Canceled {
		t.Fatalf("canceled activity error = %v", err)
	}
}
