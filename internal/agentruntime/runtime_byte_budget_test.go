package agentruntime_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"denova/internal/agentruntime"
)

func TestPendingInputAggregateBudgetRejectsBeforeDurableAcceptance(t *testing.T) {
	release := make(chan struct{})
	engine := agentruntime.NewScriptedEngine(agentruntime.EngineScript{Continue: release})
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{
		MemoryLimits: agentruntime.BindingMemoryLimits{
			MaxRetainedBytes: 1 << 20, MaxPendingInputBytes: 420, MaxActiveOutputBytes: 1 << 20,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "pending-byte-budget"})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), agentruntime.StartTurn{
		ID: "start", Input: agentruntime.UserInput{Text: "write", RestoreDescriptor: json.RawMessage(`{"version":1,"data":"active"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = harness.Submit(context.Background(), agentruntime.FollowUp{
		ID: "too-large", OperationID: receipt.OperationID,
		Input: agentruntime.UserInput{Text: strings.Repeat("q", 220), RestoreDescriptor: json.RawMessage(`{"version":1}`)},
	})
	var budget *agentruntime.ByteBudgetError
	if !errors.Is(err, agentruntime.ErrByteBudgetExceeded) || !errors.As(err, &budget) || budget.Scope != agentruntime.ByteBudgetPendingInput {
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
	engine := agentruntime.NewScriptedEngine(agentruntime.EngineScript{Events: []agentruntime.EngineEvent{
		agentruntime.EngineThinkingDelta{Delta: "12345"},
		agentruntime.EngineAssistantDelta{Delta: "6789"},
	}})
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{
		MemoryLimits: agentruntime.BindingMemoryLimits{
			MaxRetainedBytes: 1 << 20, MaxPendingInputBytes: 1 << 20, MaxActiveOutputBytes: 8,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "active-byte-budget"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	observation, err := harness.ObserveFromNow(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: "start", Input: agentruntime.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	var sawBoundary bool
	for {
		select {
		case event := <-observation.Events:
			switch payload := event.Payload.(type) {
			case agentruntime.ByteBudgetExceededEvent:
				sawBoundary = payload.Scope == agentruntime.ByteBudgetActiveOutput && payload.OperationID == receipt.OperationID
			case agentruntime.OperationSettledEvent:
				if payload.OperationID == receipt.OperationID {
					if payload.Status != agentruntime.OperationIncomplete || !sawBoundary {
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
	engine := agentruntime.NewScriptedEngine(
		agentruntime.EngineScript{Events: []agentruntime.EngineEvent{agentruntime.EngineAssistantFinal{Content: strings.Repeat("a", 300)}}},
		agentruntime.EngineScript{Events: []agentruntime.EngineEvent{agentruntime.EngineAssistantFinal{Content: strings.Repeat("b", 300)}}},
	)
	runtime, err := agentruntime.NewRuntime(engine, agentruntime.NewMemoryJournalStore(), agentruntime.RuntimeConfig{
		RetainedEventLimit: 128, RetainedMessageLimit: 128, RetainedCommandLimit: 128,
		MemoryLimits: agentruntime.BindingMemoryLimits{
			MaxRetainedBytes: 900, MaxPendingInputBytes: 1 << 20, MaxActiveOutputBytes: 1 << 20,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{Workspace: "/book", SessionID: "retained-byte-budget"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []agentruntime.CommandID{"one", "two"} {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		observation, err := harness.ObserveFromNow(ctx)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		receipt, err := harness.Submit(context.Background(), agentruntime.StartTurn{ID: id, Input: agentruntime.UserInput{Text: strings.Repeat(string(id[0]), 250)}})
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
