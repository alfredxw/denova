package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	runstate "denova/internal/agent/runtime"
)

func TestPendingInputAggregateBudgetRejectsBeforeDurableAcceptance(t *testing.T) {
	release := make(chan struct{})
	engine := runstate.NewScriptedEngine(runstate.EngineScript{Continue: release})
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{
		MemoryLimits: runstate.BindingMemoryLimits{
			MaxRetainedBytes: 1 << 20, MaxPendingInputBytes: 420, MaxActiveOutputBytes: 1 << 20,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	harness, err := runtime.Open(context.Background(), runstate.WritingBinding{Workspace: "/book", SessionID: "pending-byte-budget"})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), runstate.StartTurn{
		ID: "start", Input: runstate.UserInput{Text: "write", RestoreDescriptor: json.RawMessage(`{"version":1,"data":"active"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = harness.Submit(context.Background(), runstate.FollowUp{
		ID: "too-large", OperationID: receipt.OperationID,
		Input: runstate.UserInput{Text: strings.Repeat("q", 220), RestoreDescriptor: json.RawMessage(`{"version":1}`)},
	})
	var budget *runstate.ByteBudgetError
	if !errors.Is(err, runstate.ErrByteBudgetExceeded) || !errors.As(err, &budget) || budget.Scope != runstate.ByteBudgetPendingInput {
		t.Fatalf("aggregate admission error = %#v, want typed pending-input budget", err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Queue) != 0 || status.Memory.PendingInputBytes > status.Memory.Limits.MaxPendingInputBytes {
		t.Fatalf("rejected input changed actor state: %#v", status)
	}
	close(release)
}

func TestActiveStreamOverflowSettlesTypedIncomplete(t *testing.T) {
	engine := runstate.NewScriptedEngine(runstate.EngineScript{Events: []runstate.EngineEvent{
		runstate.EngineThinkingDelta{Delta: "12345"},
		runstate.EngineAssistantDelta{Delta: "6789"},
	}})
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{
		MemoryLimits: runstate.BindingMemoryLimits{
			MaxRetainedBytes: 1 << 20, MaxPendingInputBytes: 1 << 20, MaxActiveOutputBytes: 8,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	harness, err := runtime.Open(context.Background(), runstate.WritingBinding{Workspace: "/book", SessionID: "active-byte-budget"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	observation, err := harness.ObserveFromNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	var sawBoundary bool
	for {
		select {
		case event := <-observation.Events:
			switch payload := event.Payload.(type) {
			case runstate.ByteBudgetExceededEvent:
				sawBoundary = payload.Scope == runstate.ByteBudgetActiveOutput && payload.OperationID == receipt.OperationID
			case runstate.OperationSettledEvent:
				if payload.OperationID == receipt.OperationID {
					if payload.Status != runstate.OperationIncomplete || !sawBoundary {
						t.Fatalf("overflow settlement = %#v boundary=%v", payload, sawBoundary)
					}
					return
				}
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for incomplete settlement: %v", ctx.Err())
		}
	}
}

func TestRetainedPayloadBytesStayWithinPerBindingBudget(t *testing.T) {
	engine := runstate.NewScriptedEngine(
		runstate.EngineScript{Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: strings.Repeat("a", 300)}}},
		runstate.EngineScript{Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: strings.Repeat("b", 300)}}},
	)
	runtime, err := runstate.NewRuntime(engine, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{
		RetainedEventLimit: 128, RetainedMessageLimit: 128, RetainedCommandLimit: 128,
		MemoryLimits: runstate.BindingMemoryLimits{
			MaxRetainedBytes: 900, MaxPendingInputBytes: 1 << 20, MaxActiveOutputBytes: 1 << 20,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	harness, err := runtime.Open(context.Background(), runstate.WritingBinding{Workspace: "/book", SessionID: "retained-byte-budget"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []runstate.CommandID{"one", "two"} {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		observation, err := harness.ObserveFromNow(ctx)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		receipt, err := harness.Submit(context.Background(), runstate.StartTurn{ID: id, Input: runstate.UserInput{Text: strings.Repeat(string(id[0]), 250)}})
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		waitForRetainedTestSettlement(t, ctx, observation, receipt.OperationID)
		cancel()
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Memory.RetainedBytes > status.Memory.Limits.MaxRetainedBytes {
		t.Fatalf("retained bytes = %d, limit = %d", status.Memory.RetainedBytes, status.Memory.Limits.MaxRetainedBytes)
	}
}
