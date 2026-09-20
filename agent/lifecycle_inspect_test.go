package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestCommandRunSurvivesRecentHistoryEvictionAndColdReopen(t *testing.T) {
	ctx := t.Context()
	store := newObservingSessionStore()
	model := &lifecycleModel{}
	for i := range 40 {
		model.responses = append(model.responses, AssistantMessage(fmt.Sprintf("answer-%d", i), nil))
	}
	owner, err := New(ctx, Definition{Name: "inspect", Model: model}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	sess, err := owner.Session(ctx, NamedSession("exact-command"))
	if err != nil {
		t.Fatal(err)
	}
	var first CommandReceipt
	for i := range 40 {
		run, err := sess.Run(ctx, Input{Text: fmt.Sprintf("message-%d", i), IdempotencyKey: fmt.Sprintf("command-%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		if result, err := run.Wait(ctx); err != nil || result.Status != ResultCompleted {
			t.Fatalf("run %d: %+v %v", i, result, err)
		}
		if i == 0 {
			first = run.Receipt()
		}
	}
	recent, err := sess.Snapshot(ctx)
	if err != nil || len(recent.RecentRuns) != 32 {
		t.Fatalf("recent: %+v %v", recent, err)
	}
	assertExact := func(sess *Session) {
		t.Helper()
		if len(sess.runs) != 0 {
			t.Fatalf("settled executions remain resident: %d", len(sess.runs))
		}
		for _, input := range sess.inputs {
			if input.status != inputPending && (input.input.Text != "" || input.Input.Text != "") {
				t.Fatal("settled input body remains resident")
			}
		}
		run, found, err := sess.CommandRun(ctx, "command-0")
		if err != nil || !found {
			t.Fatalf("old command lost: %v %v", found, err)
		}
		snapshot := run.Snapshot()
		if snapshot.Receipt != first || snapshot.Result == nil || snapshot.Result.Status != ResultCompleted || snapshot.Output != "answer-0" {
			t.Fatalf("exact snapshot: %+v", snapshot)
		}
		if _, found, err := sess.CommandRun(ctx, "missing"); found || err != nil {
			t.Fatalf("missing lookup: %v %v", found, err)
		}
		retried, err := sess.Run(ctx, Input{Text: "message-0", IdempotencyKey: "command-0"})
		if err != nil || retried.Receipt() != first {
			t.Fatalf("old command retry: %v %v", retried, err)
		}
		if _, err := sess.Run(ctx, Input{Text: "changed", IdempotencyKey: "command-0"}); !errors.Is(err, ErrIdempotencyConflict) {
			t.Fatalf("conflicting old command: %v", err)
		}
		if len(sess.runs) != 0 {
			t.Fatal("historical lookup repopulated live Run registry")
		}
	}
	assertExact(sess)
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	owner, err = New(ctx, Definition{Name: "inspect", Model: model}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	sess, err = owner.Session(ctx, NamedSession("exact-command"))
	if err != nil {
		t.Fatal(err)
	}
	assertExact(sess)
	if len(model.calls()) != 40 {
		t.Fatalf("lookup executed model: %d", len(model.calls()))
	}
}

type blockedOpenStore struct {
	agentsession.Store
	entered chan struct{}
	release chan struct{}
}

func (store *blockedOpenStore) Open(ctx context.Context, key agentsession.Key) (agentsession.Log, error) {
	if key.ID == "slow" {
		close(store.entered)
		select {
		case <-store.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return store.Store.Open(ctx, key)
}

func TestSlowSessionOpenDoesNotBlockOtherSessions(t *testing.T) {
	store := &blockedOpenStore{Store: agentsession.Memory(), entered: make(chan struct{}), release: make(chan struct{})}
	owner, err := New(t.Context(), Definition{Name: "inspect", Model: &lifecycleModel{}}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	done := make(chan error, 1)
	safeGo(func() { _, err := owner.Session(t.Context(), NamedSession("slow")); done <- err }, func(err error) { done <- err })
	<-store.entered
	defer close(store.release)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	fast := make(chan error, 1)
	safeGo(func() { _, err := owner.Session(ctx, NamedSession("fast")); fast <- err }, func(err error) { fast <- err })
	select {
	case err := <-fast:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("unrelated Session waited for slow open")
	}
}
