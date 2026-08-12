package runtime_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

func TestNonContextAppendErrorTerminatesHarnessAndRuntimeReopens(t *testing.T) {
	t.Parallel()

	appendErr := errors.New("journal device unavailable")
	store := newFailOnceJournalStore(appendErr)
	runtime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(runstate.EngineScript{Result: runstate.EngineResult{Status: runstate.EngineCompleted}}),
		store,
		runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	binding := testBindingAt("/book", "append-failure")
	failed, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := failed.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := failed.Submit(context.Background(), runstate.StartTurn{
		ID: "failed", Input: runstate.UserInput{Text: "write"},
	}); !errors.Is(err, runstate.ErrHarnessFailed) {
		t.Fatalf("submit error = %v, want ErrHarnessFailed", err)
	}
	if observerErr, ok := <-observation.Errors; !ok || !errors.Is(observerErr, runstate.ErrHarnessFailed) {
		t.Fatalf("observer terminal error = %v (open=%t), want ErrHarnessFailed", observerErr, ok)
	}

	reopened, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("reopen failed harness: %v", err)
	}
	if reopened == failed {
		t.Fatal("Runtime.Open returned the harness whose journal state may be stale")
	}
	if _, err := reopened.Submit(context.Background(), runstate.StartTurn{
		ID: "succeeds-after-reopen", Input: runstate.UserInput{Text: "write"},
	}); err != nil {
		t.Fatalf("submit after reopen: %v", err)
	}
}

func TestContextAppendErrorDoesNotPoisonHarness(t *testing.T) {
	t.Parallel()

	store := newFailOnceJournalStore(context.Canceled)
	runtime, err := runstate.NewRuntime(
		runstate.NewScriptedEngine(runstate.EngineScript{Result: runstate.EngineResult{Status: runstate.EngineCompleted}}),
		store,
		runstate.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	binding := testBindingAt("/book", "context-append-failure")
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), runstate.StartTurn{
		ID: "cancelled", Input: runstate.UserInput{Text: "write"},
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("submit error = %v, want context.Canceled", err)
	}
	same, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	if same != harness {
		t.Fatal("definite context cancellation unnecessarily poisoned the harness")
	}
	if _, err := harness.Submit(context.Background(), runstate.StartTurn{
		ID: "retry", Input: runstate.UserInput{Text: "write"},
	}); err != nil {
		t.Fatalf("submit after context cancellation: %v", err)
	}
}

func TestCommittedThenErroredAppendReplaysWithoutDuplicateEngineRun(t *testing.T) {
	t.Parallel()

	appendErr := errors.New("append acknowledgement was lost")
	store := newFailOnceJournalStore(appendErr)
	store.commitBeforeError = true
	engine := runstate.NewScriptedEngine()
	runtime, err := runstate.NewRuntime(engine, store, runstate.RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	binding := testBindingAt("/book", "ambiguous-append")
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	command := runstate.StartTurn{ID: "ambiguous", Input: runstate.UserInput{Text: "write"}}
	if _, err := harness.Submit(context.Background(), command); !errors.Is(err, runstate.ErrHarnessFailed) {
		t.Fatalf("ambiguous submit error = %v, want ErrHarnessFailed", err)
	}

	reopened, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	receipt, err := reopened.Submit(context.Background(), command)
	if err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if !receipt.Replayed || receipt.CommandID != command.ID || receipt.OperationID == "" {
		t.Fatalf("replayed receipt = %+v", receipt)
	}
	if got := len(engine.Requests()); got != 0 {
		t.Fatalf("engine runs = %d, want zero after ambiguous accepted command recovery", got)
	}
}

type failOnceJournalStore struct {
	base              *runstate.MemoryJournalStore
	err               error
	mu                sync.Mutex
	pending           bool
	commitBeforeError bool
}

func newFailOnceJournalStore(err error) *failOnceJournalStore {
	return &failOnceJournalStore{base: runstate.NewMemoryJournalStore(), err: err, pending: true}
}

func (s *failOnceJournalStore) OpenJournal(ctx context.Context, key string) (runstate.Journal, error) {
	journal, err := s.base.OpenJournal(ctx, key)
	if err != nil {
		return nil, err
	}
	return &failOnceJournal{Journal: journal, store: s}, nil
}

type failOnceJournal struct {
	runstate.Journal
	store *failOnceJournalStore
}

func (j *failOnceJournal) Append(ctx context.Context, expected runstate.Cursor, payloads []runstate.EventPayload) ([]runstate.Event, error) {
	j.store.mu.Lock()
	if j.store.pending {
		j.store.pending = false
		err := j.store.err
		commitBeforeError := j.store.commitBeforeError
		j.store.mu.Unlock()
		if commitBeforeError {
			if _, commitErr := j.Journal.Append(ctx, expected, payloads); commitErr != nil {
				return nil, commitErr
			}
		}
		return nil, err
	}
	j.store.mu.Unlock()
	return j.Journal.Append(ctx, expected, payloads)
}
