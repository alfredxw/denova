package agent

import (
	"context"
	"errors"
	"testing"

	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestAbortReceiptSurvivesRunRetirementAndColdReopen(t *testing.T) {
	model := &gatedLifecycleModel{started: make(chan struct{}), release: make(chan struct{})}
	store := agentsession.Memory()
	owner, err := New(t.Context(), Definition{Name: "abort-retry", Model: model}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	key := NamedSession("abort-retry")
	sess, err := owner.Session(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	run, err := sess.Run(t.Context(), Text("wait"))
	if err != nil {
		t.Fatal(err)
	}
	<-model.started
	request := AbortRequest{IdempotencyKey: "cancel-once", Reason: "user requested"}
	receipt, err := run.Abort(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(t.Context()); err != nil || result.Status != ResultAborted {
		t.Fatalf("abort: %+v, %v", result, err)
	}
	assertRetry := func(t *testing.T, handle *Run) {
		t.Helper()
		revision := handle.session.revision
		actual, err := handle.Abort(t.Context(), request)
		if err != nil || actual != receipt {
			t.Fatalf("retry: %+v, %v; want %+v", actual, err, receipt)
		}
		changed := request
		changed.Reason = "different request"
		if _, err := handle.Abort(t.Context(), changed); !errors.Is(err, ErrIdempotencyConflict) {
			t.Fatalf("conflicting retry: %v", err)
		}
		if _, err := handle.Abort(t.Context(), AbortRequest{IdempotencyKey: "new-cancel"}); !errors.Is(err, ErrRunSettled) {
			t.Fatalf("new command against terminal Run: %v", err)
		}
		if handle.session.revision != revision || len(handle.session.runs) != 0 {
			t.Fatal("terminal retry wrote records or restored a live Run")
		}
	}
	t.Run("original handle", func(t *testing.T) { assertRetry(t, run) })
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	owner, err = New(t.Context(), Definition{Name: "abort-retry", Model: model}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	sess, err = owner.Session(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	historical, found, err := sess.AttachRun(t.Context(), run.ID())
	if err != nil || !found {
		t.Fatalf("attach terminal Run: %v, %v", found, err)
	}
	t.Run("cold reopen", func(t *testing.T) { assertRetry(t, historical) })
	if calls := model.callCount(); calls != 1 {
		t.Fatalf("terminal retry executed the model: %d calls", calls)
	}
}
