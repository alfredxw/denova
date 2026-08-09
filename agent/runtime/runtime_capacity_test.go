package runtime_test

import (
	"context"
	"errors"
	"testing"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestRuntimeEvictsLeastRecentlyUsedIdleBindingAtCapacity(t *testing.T) {
	runtime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(),
		runstate.NewMemoryJournalStore(),
		runstate.RuntimeConfig{MaxOpenBindings: 2},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	first, err := runtime.Open(context.Background(), testBinding("capacity-first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtime.Open(context.Background(), testBinding("capacity-second"))
	if err != nil {
		t.Fatal(err)
	}
	third, err := runtime.Open(context.Background(), testBinding("capacity-third"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Status(context.Background()); !errors.Is(err, runstate.ErrHarnessClosed) {
		t.Fatalf("least-recent idle status error = %v, want ErrHarnessClosed", err)
	}
	for name, harness := range map[string]*runstate.Harness{"second": second, "third": third} {
		if _, err := harness.Status(context.Background()); err != nil {
			t.Fatalf("%s harness was evicted: %v", name, err)
		}
	}
}

func TestRuntimeCapacityNeverEvictsObservedIdleBinding(t *testing.T) {
	runtime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(),
		runstate.NewMemoryJournalStore(),
		runstate.RuntimeConfig{MaxOpenBindings: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	first, err := runtime.Open(context.Background(), testBinding("observed-first"))
	if err != nil {
		t.Fatal(err)
	}
	observeCtx, stopObserving := context.WithCancel(context.Background())
	t.Cleanup(stopObserving)
	if _, err := first.ObserveFromNow(observeCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Open(context.Background(), testBinding("observed-second")); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Status(context.Background()); err != nil {
		t.Fatalf("observed idle harness was evicted: %v", err)
	}
}
