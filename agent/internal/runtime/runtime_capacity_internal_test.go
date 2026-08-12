package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type observedCloseWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

// Done is evaluated when waitCloseCall enters its select. Recording that point
// makes the eviction/explicit-close ordering deterministic without production hooks.
func (c *observedCloseWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return nil
}

func TestCloseBindingRetriesAfterBusyIdleEviction(t *testing.T) {
	runtime, err := NewRuntime(NewScriptedEngine(), NewMemoryJournalStore(), RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	binding := testBindingAt("/workspace", "busy-idle-eviction")
	harness, err := runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	key := bindingJournalKey(binding)
	eviction := &closeCall{
		ready: make(chan struct{}),
		ref:   binding.Clone(),
		kind:  closeCallIdleEviction,
	}
	runtime.mu.Lock()
	runtime.closing[key] = eviction
	runtime.mu.Unlock()

	closed := make(chan error, 1)
	waitContext := &observedCloseWaitContext{Context: context.Background(), waiting: make(chan struct{})}
	runInternalErrorTestGoroutine(closed, "close binding after idle eviction", func() error {
		return runtime.CloseBinding(waitContext, binding)
	})
	<-waitContext.waiting

	runtime.mu.Lock()
	delete(runtime.closing, key)
	close(eviction.ready)
	runtime.mu.Unlock()

	if err := <-closed; err != nil {
		t.Fatalf("CloseBinding error = %v", err)
	}
	if _, err := harness.Status(context.Background()); !errors.Is(err, ErrHarnessClosed) {
		t.Fatalf("harness status error = %v, want ErrHarnessClosed", err)
	}
}
