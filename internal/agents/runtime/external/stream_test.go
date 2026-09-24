package external

import (
	"context"
	"errors"
	"fmt"
	"testing"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	apptask "denova/internal/app/task"
)

func TestExternalStreamPreservesPartialOutputAfterStop(t *testing.T) {
	for _, stop := range []string{"cancel", "failure", "panic"} {
		t.Run(stop, func(t *testing.T) {
			service, request, _ := operationFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			request.Adapter = adapterFunc(func(_ context.Context, _ Input, host Host) (Result, error) {
				for _, text := range []string{"Saved prefix. ", "Unfinished sentence"} {
					if err := host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": text}}); err != nil {
						return Result{}, err
					}
				}
				if stop == "panic" {
					panic("stream interrupted")
				}
				if stop == "cancel" {
					cancel()
					return Result{}, context.Canceled
				}
				return Result{}, errors.New("provider disconnected")
			})
			operation, err := service.Start(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			outcome := operation.Wait(ctx)
			if outcome.Status == agentrun.OutcomeCompleted {
				t.Fatal("partial output marked completed")
			}
			// Reopen from the canonical journal, with no live Task or Session cache.
			store, err := session.NewStore(request.AttachmentRoot)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			reloaded, err := store.Get(request.Session.ID)
			if err != nil {
				t.Fatal(err)
			}
			page, err := reloaded.ReadHistoryPage(t.Context(), -1, 100)
			if err != nil {
				t.Fatal(err)
			}
			last := page.Entries[len(page.Entries)-1]
			if last.Content != "Saved prefix. Unfinished sentence" || last.ID != operation.id+"-output" || last.RunID != operation.id {
				t.Fatalf("partial output lost after reload: %#v", last)
			}
			history, err := ReadHistory(t.Context(), reloaded)
			if err != nil {
				t.Fatal(err)
			}
			if history.Messages[len(history.Messages)-1].Text != last.Content {
				t.Fatal("continuation lost partial output")
			}
		})
	}
}

func TestExternalStreamAnnouncesAcceptanceAndReplaysWhileRunning(t *testing.T) {
	service, request, _ := operationFixture(t)
	task, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer task.Finish()
	request.Emit = task.Emit
	var operation *Operation
	request.Adapter = adapterFunc(func(ctx context.Context, _ Input, host Host) (Result, error) {
		// Refresh during model setup must have an immediate stream boundary,
		// even when the provider has not produced any text or tool calls yet.
		replay, initial, err := task.SubscribeDisplayAfter(0)
		if err != nil {
			return Result{}, err
		}
		defer task.Unsubscribe(initial)
		if len(replay.Events) != 1 || replay.Events[0].Event.Type != "agent_cycle_started" {
			return Result{}, fmt.Errorf("missing accepted stream boundary: %#v", replay)
		}
		start := replay.Events[0].Event
		if start.DataString("operation_id") != operation.id || start.DataString("run_id") != operation.id || start.DataString("id") != operation.id+"-output" {
			return Result{}, fmt.Errorf("stream identity does not match acceptance: %#v", start)
		}
		if err := host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": "First "}}); err != nil {
			return Result{}, err
		}
		first := <-initial.Events()
		if first.Event.DataString("content") != "First " || first.Event.DataString("run_id") != operation.id {
			return Result{}, fmt.Errorf("unscoped live text: %#v", first)
		}
		view, owned, err := service.Status(ctx, request.ProjectID, request.Session)
		if err != nil || !owned || view.Phase != agentrun.RunPhaseRunning || string(view.ActiveOperation) != operation.id || view.ActiveCycle != 1 {
			return Result{}, fmt.Errorf("refresh lost running operation: %#v, %v", view, err)
		}
		task.Unsubscribe(initial)
		replay, resumed, err := task.SubscribeDisplayAfter(0)
		if err != nil {
			return Result{}, err
		}
		defer task.Unsubscribe(resumed)
		if len(replay.Events) != 2 || replay.Events[1].Event.DataString("content") != "First " {
			return Result{}, fmt.Errorf("refresh lost streamed prefix: %#v", replay)
		}
		if err := host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": "second."}}); err != nil {
			return Result{}, err
		}
		if next := <-resumed.Events(); next.Event.DataString("content") != "second." || next.Cursor != first.Cursor+1 {
			return Result{}, fmt.Errorf("resumed stream lost live suffix: %#v", next)
		}
		return Result{Text: "First second."}, nil
	})
	operation, err = service.Start(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := operation.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("stream failed: %#v", outcome)
	}
	page, err := request.Session.ReadHistoryPage(t.Context(), -1, 100)
	if err != nil {
		t.Fatal(err)
	}
	last := page.Entries[len(page.Entries)-1]
	if last.Content != "First second." || last.RunID != operation.id || last.ID != operation.id+"-output" {
		t.Fatalf("canonical output lost stream identity: %#v", last)
	}
}
