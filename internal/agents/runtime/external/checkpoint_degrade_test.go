package external

import (
	"context"
	"errors"
	"strings"
	"testing"

	externaljournal "denova/internal/agents/runtime/external/journal"
)

// oversizedHistory exceeds both reconstruction budgets with a single message.
func oversizedHistory() Input {
	return Input{History: []Message{{Role: "user", Text: strings.Repeat("history ", 20000), Cursor: 1}}}
}

// A provider that refuses the maintenance call must not fail the turn: the
// unreduced source is sent, and no coverage is published for a summary that does
// not exist.
func TestExternalHistoryKeepsTurnWhenSummaryIsRefused(t *testing.T) {
	calls, saved := 0, 0
	prepared, err := (HistoryPreparation{Input: oversizedHistory(), Adapter: adapterFunc(func(context.Context, Input, Host) (Result, error) {
		calls++
		return Result{}, errors.New("Antigravity attempt ended with status ERROR: Your previous response was blocked by content safety filters")
	}), SaveCheckpoint: func(externaljournal.Checkpoint) error { saved++; return nil }}).Prepare(t.Context())
	if err != nil {
		t.Fatalf("summary refusal failed the turn: %v", err)
	}
	if calls != 1 || saved != 0 {
		t.Fatalf("calls=%d saved=%d, want one degraded attempt and no published coverage", calls, saved)
	}
	if len(prepared.History) != 1 || !strings.Contains(prepared.History[0].Text, "history history") {
		t.Fatalf("unreduced history was not preserved: %+v", prepared.History)
	}
}

// A transient transport failure stays fatal so the runtime can retry it instead
// of silently continuing on unreduced context.
func TestExternalHistoryRetriesTransientSummaryFailure(t *testing.T) {
	_, err := (HistoryPreparation{Input: oversizedHistory(), Adapter: adapterFunc(func(context.Context, Input, Host) (Result, error) {
		return Result{}, RetryableError(errors.New("connection reset by peer"))
	})}).Prepare(t.Context())
	if err == nil || !strings.Contains(err.Error(), "maintain external context") || errors.Is(err, ErrSummaryUnavailable) {
		t.Fatalf("transient failure was degraded: %v", err)
	}
}

// Cancellation must never be converted into a degraded attempt.
func TestExternalHistoryPropagatesCancelledSummary(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, err := (HistoryPreparation{Input: oversizedHistory(), Adapter: adapterFunc(func(context.Context, Input, Host) (Result, error) {
		cancel()
		return Result{}, context.Canceled
	})}).Prepare(ctx)
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrSummaryUnavailable) {
		t.Fatalf("cancellation was degraded: %v", err)
	}
}
