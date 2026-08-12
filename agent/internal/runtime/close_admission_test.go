package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestHarnessCloseCancellationCanStopBeforeAdmission(t *testing.T) {
	t.Parallel()

	harness := &Harness{
		requests: make(chan any, 1),
		done:     make(chan struct{}),
	}
	// Hold the only queue slot so cancellation deterministically wins before
	// admission. Runtime-owned closes use a non-cancelled owner context; direct
	// callers must not become stuck behind a saturated actor request lane.
	harness.requests <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	closed := make(chan error, 1)
	runInternalErrorTestGoroutine(closed, "close before admission", func() error {
		return harness.Close(ctx)
	})
	select {
	case err := <-closed:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Close error = %v, want context.Canceled", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("cancelled Close remained blocked before request admission")
	}
	if got := <-harness.requests; got == nil {
		t.Fatal("queue sentinel unexpectedly missing")
	}
}

func TestHarnessCloseCancellationCanStopAfterAdmission(t *testing.T) {
	t.Parallel()

	engine := &closeAdmissionEngine{
		started:   make(chan struct{}),
		abortSeen: make(chan struct{}),
		release:   make(chan struct{}),
	}
	runtime, err := NewRuntime(
		EngineFactoryFunc(func(context.Context, BindingRef) (Engine, error) { return engine, nil }),
		NewMemoryJournalStore(),
		RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		engine.releaseOnce.Do(func() { close(engine.release) })
		_ = runtime.Close(context.Background())
	}()

	harness, err := runtime.Open(context.Background(), testBindingAt("/workspace", "session-close-cancel"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.Submit(context.Background(), StartTurn{
		ID: "start-close-cancel", Input: UserInput{Text: "keep running"},
	}); err != nil {
		t.Fatal(err)
	}
	<-engine.started

	ctx, cancel := context.WithCancel(context.Background())
	closed := make(chan error, 1)
	runInternalErrorTestGoroutine(closed, "close after admission", func() error {
		return harness.Close(ctx)
	})
	<-engine.abortSeen
	cancel()

	select {
	case err := <-closed:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Close error = %v, want context.Canceled", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("cancelled Close remained blocked after request admission")
	}

	engine.releaseOnce.Do(func() { close(engine.release) })
	if err := harness.Close(context.Background()); err != nil {
		t.Fatalf("subsequent owner Close error = %v", err)
	}
}

type closeAdmissionEngine struct {
	started     chan struct{}
	abortSeen   chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	abortOnce   sync.Once
	releaseOnce sync.Once
}

func (e *closeAdmissionEngine) Run(ctx context.Context, request EngineRequest, _ EngineEventSink) (EngineResult, error) {
	e.startedOnce.Do(func() { close(e.started) })
	for {
		select {
		case control := <-request.Controls:
			if control.Kind == EngineControlAbort {
				e.abortOnce.Do(func() { close(e.abortSeen) })
				select {
				case <-e.release:
					return EngineResult{Status: EngineAborted}, nil
				case <-ctx.Done():
					return EngineResult{Status: EngineAborted}, ctx.Err()
				}
			}
		case <-ctx.Done():
			return EngineResult{Status: EngineAborted}, ctx.Err()
		}
	}
}
