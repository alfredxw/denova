package goal

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGoalLifecycleUsesRevisionFencesAndActiveTime(t *testing.T) {
	started := time.Date(2026, time.August, 10, 10, 0, 0, 0, time.UTC)
	current, err := New("Finish the complete feature", started)
	if err != nil {
		t.Fatal(err)
	}
	paused, err := Pause(current, current.Revision, started.Add(1500*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if paused.Status != StatusPaused || paused.ActiveSince != nil || paused.ActiveDurationMillis != 1500 {
		t.Fatalf("paused goal = %#v", paused)
	}
	if _, err := Resume(paused, current.Revision, started.Add(2*time.Second)); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale resume error = %v, want revision conflict", err)
	}
	resumed, err := Resume(paused, paused.Revision, started.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	completed, err := Finish(resumed, resumed.ID, resumed.Revision, StatusCompleted, "Verified", started.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != StatusCompleted || completed.Report != "Verified" || completed.ActiveDurationMillis != 4500 {
		t.Fatalf("completed goal = %#v", completed)
	}
}

func TestGoalObjectiveHasAHighExplicitHardLimit(t *testing.T) {
	now := time.Date(2026, time.August, 10, 10, 0, 0, 0, time.UTC)
	if _, err := New(strings.Repeat("a", MaxObjectiveBytes), now); err != nil {
		t.Fatalf("objective at hard limit: %v", err)
	}
	if _, err := New(strings.Repeat("a", MaxObjectiveBytes+1), now); err == nil {
		t.Fatal("objective above hard limit unexpectedly succeeded")
	}
}

func TestGoalFinishCannotAcknowledgeAReplacementGoal(t *testing.T) {
	now := time.Date(2026, time.August, 10, 10, 0, 0, 0, time.UTC)
	first, err := New("First", now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Set(first, "Replacement", first.Revision, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Finish(second, first.ID, first.Revision, StatusCompleted, "stale", now.Add(2*time.Second)); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale finish error = %v, want revision conflict", err)
	}
}
