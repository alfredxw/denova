package agent

import (
	"context"
	"errors"
	"testing"

	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestSessionQueuesInputAcrossCloseWithoutStartingModel(t *testing.T) {
	store := agentsession.Memory()
	model := &lifecycleModel{responses: []*Message{AssistantMessage("first", nil), AssistantMessage("supplement", nil)}}
	open := func() (*Agent, *Session) {
		owner, err := New(context.Background(), Definition{Name: "test", Model: model}, WithSessionStore(store))
		if err != nil {
			t.Fatal(err)
		}
		session, err := owner.Session(context.Background(), NamedSession("inbox"))
		if err != nil {
			t.Fatal(err)
		}
		return owner, session
	}
	owner, session := open()
	input := Input{Text: "remember this", IdempotencyKey: "message-1"}
	queued, err := session.Queue(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	receipt := queued.Receipt()
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	owner, session = open()
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	if len(model.calls()) != 0 {
		t.Fatal("opening the inbox called the model")
	}
	duplicate, err := session.Queue(context.Background(), input)
	if err != nil || duplicate.Receipt() != receipt {
		t.Fatalf("receipt=%#v err=%v", duplicate, err)
	}
	input.Text = "different content"
	if _, err := session.Queue(context.Background(), input); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting command error=%v", err)
	}
	run, err := session.Run(context.Background(), Input{Text: "start", IdempotencyKey: "start-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := model.calls()
	if len(calls) != 2 || calls[1][len(calls[1])-1].Content != "remember this" {
		t.Fatalf("provider calls=%#v", calls)
	}
	if _, err := duplicate.Cancel(context.Background(), QueueControlRequest{IdempotencyKey: "cancel-consumed"}); !errors.Is(err, ErrInputConsumed) {
		t.Fatalf("consumed cancellation=%v", err)
	}
}

func TestSessionFollowUpReturnsDurableRunReceipt(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{AssistantMessage("done", nil)}}
	owner, err := New(context.Background(), Definition{Name: "test", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("follow-up"))
	if err != nil {
		t.Fatal(err)
	}
	input := Input{Text: "new task", IdempotencyKey: "task-1"}
	receipt, err := session.FollowUp(context.Background(), input)
	if err != nil || receipt.RunID == "" {
		t.Fatalf("receipt=%#v err=%v", receipt, err)
	}
	run, found, err := session.AttachRun(context.Background(), receipt.RunID)
	if err != nil || !found {
		t.Fatalf("attach found=%v err=%v", found, err)
	}
	if _, err := run.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	again, err := session.FollowUp(context.Background(), input)
	if err != nil || again != receipt || len(model.calls()) != 1 {
		t.Fatalf("repeat=%#v err=%v calls=%d", again, err, len(model.calls()))
	}
}
