package external

import (
	"context"
	"strings"
	"testing"

	externaljournal "denova/internal/agents/runtime/external/journal"
)

// A summary follows the source records it covers, not the provider boundary that
// happened to be current when it was written. Game boundaries move on every
// committed turn, so requiring them used to re-summarize the whole source on
// every rebuild.
func TestExternalCheckpointSurvivesBoundaryChange(t *testing.T) {
	history := []Message{{Role: "user", Text: strings.Repeat("old context ", 12000), Cursor: 1}}
	calls := 0
	adapter := adapterFunc(func(context.Context, Input, Host) (Result, error) {
		calls++
		return Result{Text: "Confirmed earlier context."}, nil
	})
	var saved externaljournal.Checkpoint
	first := HistoryPreparation{Input: Input{History: history, HistoryBoundary: "turn-1"}, Adapter: adapter,
		SaveCheckpoint: func(checkpoint externaljournal.Checkpoint) error { saved = checkpoint; return nil }}
	if _, err := first.Prepare(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls == 0 || saved.Summary == "" || saved.SourceBoundary != "turn-1" {
		t.Fatalf("first preparation: calls=%d checkpoint=%+v", calls, saved)
	}
	summaryCalls := calls
	next := HistoryPreparation{Input: Input{History: history, HistoryBoundary: "turn-2"}, Checkpoint: &saved, Adapter: adapter}
	prepared, err := next.Prepare(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if calls != summaryCalls {
		t.Fatalf("boundary change re-summarized the same source: calls=%d want %d", calls, summaryCalls)
	}
	if len(prepared.History) != 1 || !strings.Contains(prepared.History[0].Text, "Canonical conversation checkpoint") {
		t.Fatalf("checkpoint was not applied: %+v", prepared.History)
	}
}
