package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSuspendReopenResumesSameRunWithoutRepeatingInput(t *testing.T) {
	store := newObservingSessionStore()
	first := &gatedLifecycleModel{started: make(chan struct{}), release: make(chan struct{})}
	owner, err := New(context.Background(), Definition{Name: "test", Model: first}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	session, err := owner.Session(context.Background(), NamedSession("suspended"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Input{Text: "original task", IdempotencyKey: "start"})
	if err != nil {
		t.Fatal(err)
	}
	<-first.started
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	suspension, err := session.SuspendAndClose(ctx, SuspendRequest{RunID: run.ID(), IdempotencyKey: "pause"})
	if err != nil || suspension.Status != ResultSuspended {
		t.Fatalf("suspension=%#v error=%v", suspension, err)
	}
	if result, err := run.Wait(ctx); err != nil || result.Status != ResultSuspended {
		t.Fatalf("wait=%#v error=%v", result, err)
	}
	if repeated, err := session.SuspendAndClose(ctx, SuspendRequest{RunID: run.ID(), IdempotencyKey: "pause"}); err != nil || repeated.Receipt != suspension.Receipt {
		t.Fatalf("repeated pause=%#v error=%v", repeated, err)
	}
	if _, err := session.SuspendAndClose(ctx, SuspendRequest{RunID: run.ID(), IdempotencyKey: "unaccepted"}); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("closed Session accepted a new pause command: %v", err)
	}
	if store.count(turnFinishedRecord) != 0 || store.count(turnInterruptedRecord) != 0 {
		t.Fatal("pause settled the logical task")
	}
	close(first.release)
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	model := &lifecycleModel{responses: []*Message{AssistantMessage("done", nil), AssistantMessage("added", nil)}}
	owner, err = New(context.Background(), Definition{Name: "test", Model: model}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err = owner.Session(context.Background(), NamedSession("suspended"))
	if err != nil {
		t.Fatal(err)
	}
	if len(model.calls()) != 0 {
		t.Fatal("open executed the Run")
	}
	if _, err := session.Run(ctx, Text("new task")); !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("new Run error=%v", err)
	}
	if _, err := session.Queue(ctx, Input{Text: "added input", IdempotencyKey: "message"}); err != nil {
		t.Fatal(err)
	}
	if len(model.calls()) != 0 {
		t.Fatal("Queue resumed the Run")
	}
	resumed, err := session.ResumeRun(ctx, ResumeRequest{RunID: run.ID(), IdempotencyKey: "resume"})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ID() != run.ID() {
		t.Fatal("resume changed the logical Run")
	}
	if result, err := resumed.Wait(ctx); err != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	calls := model.calls()
	if len(calls) != 2 || len(calls[0]) != 1 || calls[0][0].Content != "original task" {
		t.Fatalf("resumed model messages=%#v", calls)
	}
	if store.count(turnFinishedRecord) != 1 {
		t.Fatalf("settlements=%d", store.count(turnFinishedRecord))
	}
}

func TestRetriedPauseDoesNotStopAResumedRun(t *testing.T) {
	for _, tree := range []bool{false, true} {
		name := "session"
		if tree {
			name = "tree"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			model := &gatedLifecycleModel{started: make(chan struct{}), release: make(chan struct{})}
			owner, err := New(ctx, Definition{Name: "test", Model: model}, WithSessionStore(newObservingSessionStore()))
			if err != nil {
				t.Fatal(err)
			}
			defer owner.Close(context.Background())
			key := NamedSession("replayed-control")
			sess, err := owner.Session(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			run, err := sess.Run(ctx, Text("original task"))
			if err != nil {
				t.Fatal(err)
			}
			<-model.started
			pause := func() (Suspension, error) {
				request := SuspendRequest{RunID: run.ID(), IdempotencyKey: "pause-once"}
				if tree {
					return owner.SuspendTree(ctx, key, request)
				}
				return sess.SuspendAndClose(ctx, request)
			}
			stopped, err := pause()
			if err != nil {
				t.Fatal(err)
			}
			sess, err = owner.Session(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			request := ResumeRequest{RunID: run.ID(), IdempotencyKey: "resume-once"}
			if tree {
				run, err = owner.ResumeTree(ctx, key, request)
			} else {
				run, err = sess.ResumeRun(ctx, request)
			}
			if err != nil {
				t.Fatal(err)
			}
			for model.callCount() != 2 {
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(time.Millisecond):
				}
			}
			repeated, err := pause()
			if err != nil || repeated.Receipt != stopped.Receipt {
				t.Fatalf("pause receipt=%+v error=%v", repeated, err)
			}
			current, err := sess.Snapshot(ctx)
			if err != nil || current.ActiveRunID != run.ID() || current.ActiveStatus == ResultSuspended {
				t.Fatalf("old pause stopped current execution: %+v error=%v", current, err)
			}
			close(model.release)
			if result, err := run.Wait(ctx); err != nil || result.Status != ResultCompleted {
				t.Fatalf("resumed=%+v error=%v", result, err)
			}
		})
	}
}
