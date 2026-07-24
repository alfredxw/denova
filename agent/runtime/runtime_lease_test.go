package runtime_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestRuntimeOpenHoldsMemoryBindingLeaseUntilClose(t *testing.T) {
	t.Parallel()

	store := runstate.NewMemoryJournalStore()
	first := newTestRuntime(t, store)
	second := newTestRuntime(t, store)
	binding := testBindingAt("/book", "memory-lease")

	if _, err := first.Open(context.Background(), binding); err != nil {
		t.Fatalf("open first runtime: %v", err)
	}
	assertOpenWaitsForLease(t, second, binding)
	if err := first.CloseBinding(context.Background(), binding); err != nil {
		t.Fatalf("close first binding: %v", err)
	}
	if _, err := second.Open(context.Background(), binding); err != nil {
		t.Fatalf("open after lease release: %v", err)
	}
}

func TestRuntimeOpenHoldsFileBindingLeaseAcrossStores(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	firstStore, err := runstate.NewFileJournalStore(filepath.Join(root, "journals"))
	if err != nil {
		t.Fatal(err)
	}
	secondStore, err := runstate.NewFileJournalStore(filepath.Join(root, "journals"))
	if err != nil {
		t.Fatal(err)
	}
	first := newTestRuntime(t, firstStore)
	second := newTestRuntime(t, secondStore)
	binding := testBindingAt("/book", "file-lease")

	if _, err := first.Open(context.Background(), binding); err != nil {
		t.Fatalf("open first runtime: %v", err)
	}
	assertOpenWaitsForLease(t, second, binding)
	if err := first.CloseBinding(context.Background(), binding); err != nil {
		t.Fatalf("close first binding: %v", err)
	}
	if _, err := second.Open(context.Background(), binding); err != nil {
		t.Fatalf("open after lease release: %v", err)
	}
}

func TestCloseBindingWaitsForPendingOpenAndPreventsOverlappingHarness(t *testing.T) {
	t.Parallel()

	factory := newBlockingEngineFactory("pending-close")
	runtime, err := runstate.NewRuntime(factory, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	binding := testBindingAt("/book", "pending-close")
	openDone := make(chan error, 1)
	runExternalErrorTestGoroutine(openDone, "pending binding open", func() error {
		_, openErr := runtime.Open(context.Background(), binding)
		return openErr
	})
	<-factory.blocked

	closeDone := make(chan error, 1)
	runExternalErrorTestGoroutine(closeDone, "close pending binding", func() error {
		return runtime.CloseBinding(context.Background(), binding)
	})
	select {
	case err := <-closeDone:
		t.Fatalf("CloseBinding returned before pending Open resolved: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(factory.release)
	if err := <-openDone; err != nil {
		t.Fatalf("pending Open: %v", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatalf("CloseBinding: %v", err)
	}

	// The close owns the pending lane. A subsequent Open must create a fresh,
	// usable Harness rather than returning the Harness that was just closed.
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("reopen after pending close: %v", err)
	}
	if _, err := harness.ObserveFromNow(context.Background()); err != nil {
		t.Fatalf("fresh harness is not usable: %v", err)
	}
}

func assertOpenWaitsForLease(t *testing.T, runtime *runstate.Runtime, binding runstate.BindingRef) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := runtime.Open(ctx, binding); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Open error = %v, want binding lease wait cancellation", err)
	}
}

func newTestRuntime(t *testing.T, store runstate.JournalStore) *runstate.Runtime {
	t.Helper()
	runtime, err := runstate.NewRuntime(runstate.NewScriptedEngine(), store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}
