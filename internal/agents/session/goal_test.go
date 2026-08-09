package session

import (
	"context"
	"errors"
	"testing"

	"denova/internal/agents/goal"
)

func TestConversationGoalPersistsAndRejectsStaleMutations(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("goal-session")
	if err != nil {
		t.Fatal(err)
	}
	created, err := sess.SetGoal(ctx, "Complete the release", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.PauseGoal(ctx, created.Revision+1); !errors.Is(err, goal.ErrRevisionConflict) {
		t.Fatalf("stale mutation error = %v, want revision conflict", err)
	}
	paused, err := sess.PauseGoal(ctx, created.Revision)
	if err != nil {
		t.Fatal(err)
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("goal-session")
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := reloaded.Goal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.ID != created.ID || got.Status != goal.StatusPaused || got.Revision != paused.Revision {
		t.Fatalf("reloaded goal = %#v, found=%v", got, ok)
	}
	cleared, err := reloaded.ClearGoal(ctx, got.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Status != goal.StatusCleared {
		t.Fatalf("cleared goal = %#v", cleared)
	}
	if _, ok, err := reloaded.Goal(ctx); err != nil || ok {
		t.Fatalf("cleared goal must be absent: found=%v err=%v", ok, err)
	}
	replacement, err := reloaded.SetGoal(ctx, "Verify the replacement goal", 0)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.ID == created.ID || replacement.Revision <= cleared.Revision {
		t.Fatalf("replacement goal did not advance the durable fence: old=%#v cleared=%#v replacement=%#v", created, cleared, replacement)
	}
}
