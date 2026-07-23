package runtime_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	runstate "denova/internal/agent/runtime"
)

func TestOpenLeaderCancellationDoesNotPoisonWaiters(t *testing.T) {
	t.Parallel()

	factory := newBlockingEngineFactory("leader-cancel")
	runtime, err := runstate.NewRuntime(factory, runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	binding := runstate.WritingBinding{Workspace: "/book", SessionID: "leader-cancel"}
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	runExternalErrorTestGoroutine(leaderDone, "leader binding open", func() error {
		_, openErr := runtime.Open(leaderCtx, binding)
		return openErr
	})
	<-factory.blocked
	cancelLeader()
	if err := <-leaderDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader Open error = %v, want context.Canceled", err)
	}

	waiterDone := make(chan error, 1)
	runExternalErrorTestGoroutine(waiterDone, "waiting binding open", func() error {
		_, openErr := runtime.Open(context.Background(), binding)
		return openErr
	})
	close(factory.release)
	if err := <-waiterDone; err != nil {
		t.Fatalf("waiter inherited leader cancellation: %v", err)
	}
}

func TestCloseBindingCallerCancellationDoesNotReleaseFenceEarly(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	runtime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(runstate.EngineScript{Continue: release}),
		runstate.NewMemoryJournalStore(),
		runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	binding := runstate.WritingBinding{Workspace: "/book", SessionID: "cancelled-close"}
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "write"}}); err != nil {
		t.Fatal(err)
	}

	caller, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runtime.CloseBinding(caller, binding); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled CloseBinding error = %v, want context.Canceled", err)
	}
	blocked, cancelBlocked := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelBlocked()
	if _, err := runtime.Open(blocked, binding); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Open crossed an unfinished close fence: %v", err)
	}
	close(release)
	if err := runtime.CloseBinding(context.Background(), binding); err != nil {
		t.Fatalf("wait for owner close: %v", err)
	}
	if _, err := runtime.Open(context.Background(), binding); err != nil {
		t.Fatalf("Open after actual lease release: %v", err)
	}
}

func TestCloseBindingsEvictsOnlySelectedWorkspace(t *testing.T) {
	t.Parallel()

	runtime, err := runstate.NewRuntime(runstate.NewScriptedEngine(), runstate.NewMemoryJournalStore(), runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	aBinding := runstate.WritingBinding{Workspace: "/book-a", SessionID: "session"}
	bBinding := runstate.WritingBinding{Workspace: "/book-b", SessionID: "session"}
	a, err := runtime.Open(context.Background(), aBinding)
	if err != nil {
		t.Fatal(err)
	}
	b, err := runtime.Open(context.Background(), bBinding)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.CloseBindings(context.Background(), runstate.BindingSelector{Workspace: "/book-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Status(context.Background()); !errors.Is(err, runstate.ErrHarnessClosed) {
		t.Fatalf("selected actor status error = %v, want ErrHarnessClosed", err)
	}
	stillB, err := runtime.Open(context.Background(), bBinding)
	if err != nil || stillB != b {
		t.Fatalf("unselected actor changed: same=%t err=%v", stillB == b, err)
	}
	reopenedA, err := runtime.Open(context.Background(), aBinding)
	if err != nil || reopenedA == a {
		t.Fatalf("selected actor was not freshly opened: fresh=%t err=%v", reopenedA != a, err)
	}
}

func TestProjectDoesNotCreateEngineOrRetainBindingLease(t *testing.T) {
	t.Parallel()

	var engines atomic.Int32
	factory := runstate.EngineFactoryFunc(func(context.Context, runstate.BindingRef) (runstate.Engine, error) {
		engines.Add(1)
		return runstate.NewScriptedEngine(runstate.EngineScript{
			Events: []runstate.EngineEvent{runstate.EngineAssistantFinal{Content: "done"}},
		}), nil
	})
	store := runstate.NewMemoryJournalStore()
	runtime, err := runstate.NewRuntime(factory, store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	binding := runstate.WritingBinding{Workspace: "/book", SessionID: "projection"}
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := harness.Submit(context.Background(), runstate.StartTurn{ID: "start", Input: runstate.UserInput{Text: "write"}})
	if err != nil {
		t.Fatal(err)
	}
	waitForSettled(t, harness, receipt.Cursor)
	if err := runtime.CloseBinding(context.Background(), binding); err != nil {
		t.Fatal(err)
	}

	status, err := runtime.Project(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != runstate.PhaseIdle || status.LastOperation == nil || status.LastOperation.CommandID != "start" {
		t.Fatalf("status projection = %#v", status)
	}
	if got := engines.Load(); got != 1 {
		t.Fatalf("Project created an Engine: count=%d", got)
	}
	second, err := runstate.NewRuntime(runstate.NewScriptedEngine(), store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.Open(context.Background(), binding); err != nil {
		t.Fatalf("Project retained the binding lease: %v", err)
	}
}

func TestRuntimeOwnerPanicCompletesOpenAndProjectWaiters(t *testing.T) {
	t.Parallel()

	runtime, err := runstate.NewRuntime(runstate.NewScriptedEngine(), panickingJournalStore{}, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	binding := runstate.WritingBinding{Workspace: "/book", SessionID: "panic-owner"}
	if _, err := runtime.Open(context.Background(), binding); !errors.Is(err, runstate.ErrHarnessFailed) {
		t.Fatalf("Open panic error = %v, want ErrHarnessFailed", err)
	}
	if _, err := runtime.Project(context.Background(), binding); !errors.Is(err, runstate.ErrHarnessFailed) {
		t.Fatalf("Project panic error = %v, want ErrHarnessFailed", err)
	}
}

func TestHarnessContainsJournalClosePanicAtActorBoundary(t *testing.T) {
	t.Parallel()

	runtime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(),
		&closePanickingJournalStore{base: runstate.NewMemoryJournalStore()},
		runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), runstate.WritingBinding{
		Workspace: "/book", SessionID: "close-panic",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.Close(context.Background()); !errors.Is(err, runstate.ErrHarnessFailed) {
		t.Fatalf("Close panic error = %v, want ErrHarnessFailed", err)
	}
}

type panickingJournalStore struct{}

func (panickingJournalStore) OpenJournal(context.Context, string) (runstate.Journal, error) {
	panic("journal store panic")
}

type closePanickingJournalStore struct {
	base *runstate.MemoryJournalStore
}

func (s *closePanickingJournalStore) OpenJournal(ctx context.Context, key string) (runstate.Journal, error) {
	journal, err := s.base.OpenJournal(ctx, key)
	if err != nil {
		return nil, err
	}
	return closePanickingJournal{Journal: journal}, nil
}

type closePanickingJournal struct {
	runstate.Journal
}

func (closePanickingJournal) Close() error {
	panic("journal close panic")
}
