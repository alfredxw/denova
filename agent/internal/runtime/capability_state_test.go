package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

func TestCapabilityStateBatchIsAtomicAcrossCASConflict(t *testing.T) {
	runtime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(), runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	harness, err := runtime.Open(context.Background(), testBindingAt("/book", "capability-batch"))
	if err != nil {
		t.Fatal(err)
	}
	firstAbsent, err := harness.CapabilityState(context.Background(), "first")
	if err != nil {
		t.Fatal(err)
	}
	secondAbsent, err := harness.CapabilityState(context.Background(), "second")
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.SetCapabilityStates(context.Background(),
		runstate.EngineCapabilityState{
			Capability: "first", Expected: firstAbsent.Descriptor, State: json.RawMessage(`{"revision":1}`),
		},
		runstate.EngineCapabilityState{
			Capability: "second", Expected: secondAbsent.Descriptor, State: json.RawMessage(`{"revision":1}`),
		},
	); err != nil {
		t.Fatal(err)
	}
	first, err := harness.CapabilityState(context.Background(), "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := harness.CapabilityState(context.Background(), "second")
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.SetCapabilityState(
		context.Background(), "second", second.Descriptor, json.RawMessage(`{"revision":2}`), false,
	); err != nil {
		t.Fatal(err)
	}
	err = harness.SetCapabilityStates(context.Background(),
		runstate.EngineCapabilityState{
			Capability: "first", Expected: first.Descriptor, State: json.RawMessage(`{"revision":2}`),
		},
		runstate.EngineCapabilityState{
			Capability: "second", Expected: second.Descriptor, Delete: true,
		},
	)
	if !errors.Is(err, runstate.ErrInvalidCommand) {
		t.Fatalf("batch error = %v, want stale CAS", err)
	}
	unchanged, err := harness.CapabilityState(context.Background(), "first")
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged.State) != `{"revision":1}` {
		t.Fatalf("first capability partially committed: %s", unchanged.State)
	}
}
