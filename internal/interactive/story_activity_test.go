package interactive

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"denova/internal/agents/conversationjournal"
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
	journal := store.storyJournals[story.ID].journal
	before := journal.ReplayStats().RangeBytesRead
	activity, err := store.ReadActivity(context.Background(), story.ID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil || len(activity.TurnTimes) != 550 {
		t.Fatalf("long-history activity count=%d err=%v", len(activity.TurnTimes), err)
	}
	if read := journal.ReplayStats().RangeBytesRead - before; read != 0 {
		t.Fatalf("activity reread %d historical journal bytes with an aligned index", read)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.ReadActivity(ctx, story.ID, time.Time{}, time.Now()); err != context.Canceled {
		t.Fatalf("canceled activity error = %v", err)
	}
}

func TestActivityRebuildsMissingAndReleasedIndexesWithoutChangingJournal(t *testing.T) {
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "Activity index recovery"})
	if err != nil {
		t.Fatal(err)
	}
	appendStoryTurns(t, store, story.ID, "main", []AppendTurnRequest{
		{User: "Continue", Narrative: "First turn."},
		{User: "Continue", Narrative: "Second turn."},
	})
	from, until := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	want, err := store.ReadActivity(t.Context(), story.ID, from, until)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	path := store.storyPath(story.ID)
	canonical, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	indexPath := conversationjournal.SidecarPath(path)
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the v0.6.3 projection: its locator tuples have no Turn timestamp.
	var released map[string]any
	if err := json.Unmarshal(data, &released); err != nil {
		t.Fatal(err)
	}
	projection := released["projection"].(map[string]any)
	projection["version"] = 19
	for id, raw := range projection["model_nodes"].(map[string]any) {
		fields := raw.([]any)
		projection["model_nodes"].(map[string]any)[id] = append(fields[:3], fields[4:]...)
	}
	releasedData, err := json.Marshal(released)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"missing", "released"} {
		t.Run(mode, func(t *testing.T) {
			var err error
			if mode == "missing" {
				err = os.Remove(indexPath)
			} else {
				err = os.WriteFile(indexPath, releasedData, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			reopened := NewStore(store.Root())
			defer reopened.Close()
			for range 2 {
				got, err := reopened.ReadActivity(t.Context(), story.ID, from, until)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("activity after index recovery = %+v, want %+v, err=%v", got, want, err)
				}
				stats := reopened.storyJournals[story.ID].journal.ReplayStats()
				if !stats.IndexRebuilt || stats.RangeBytesRead != 0 {
					t.Fatalf("activity recovery must rebuild once without history range reads: %+v", stats)
				}
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, canonical) {
				t.Fatalf("activity index recovery changed canonical journal: %v", err)
			}
		})
	}
}

func TestActivityRemainsConsistentDuringCommits(t *testing.T) {
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "Concurrent activity"})
	if err != nil {
		t.Fatal(err)
	}
	turns := make([]AppendTurnRequest, 200)
	for i := range turns {
		turns[i] = AppendTurnRequest{User: "Continue", Narrative: "Retained narrative."}
	}
	appendStoryTurns(t, store, story.ID, "main", turns)
	from, until := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	t.Run("concurrent", func(t *testing.T) {
		t.Run("read", func(t *testing.T) {
			t.Parallel()
			for range 50 {
				activity, err := store.ReadActivity(t.Context(), story.ID, from, until)
				if err != nil {
					t.Fatal(err)
				}
				if len(activity.TurnTimes) != activity.Story.TurnCount {
					t.Fatalf("activity mixed story revisions: %+v", activity)
				}
			}
		})
		t.Run("commit", func(t *testing.T) {
			t.Parallel()
			for range 20 {
				if _, err := store.AppendTurn(story.ID, AppendTurnRequest{User: "Continue", Narrative: "A new turn."}); err != nil {
					t.Fatal(err)
				}
			}
		})
	})
}
