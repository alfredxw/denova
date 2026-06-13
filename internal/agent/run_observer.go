package agent

import (
	"context"
	"sync"
)

type runObserverKey struct{}

// RunObserver records durable state for one Agent run without changing model-visible behavior.
type RunObserver struct {
	ledger *RunLedger
	mu     sync.Mutex
}

func newRunObserver(ledger *RunLedger) *RunObserver {
	return &RunObserver{ledger: ledger}
}

func ContextWithRunObserver(ctx context.Context, observer *RunObserver) context.Context {
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, runObserverKey{}, observer)
}

func RunObserverFromContext(ctx context.Context) *RunObserver {
	if ctx == nil {
		return nil
	}
	observer, _ := ctx.Value(runObserverKey{}).(*RunObserver)
	return observer
}

func (o *RunObserver) RecordToolDecision(decision ToolDecision) {
	if o == nil || o.ledger == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_ = o.ledger.RecordToolDecision(decision)
}

func (o *RunObserver) RecordToolExecution(result ToolExecutionRecord) {
	if o == nil || o.ledger == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_ = o.ledger.RecordToolExecution(result)
}

func (o *RunObserver) RecordMutations(mutations []ToolMutation) {
	if o == nil || o.ledger == nil || len(mutations) == 0 {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_ = o.ledger.RecordMutations(mutations)
}

func (o *RunObserver) RecordVerification(verification PostRunVerification) {
	if o == nil || o.ledger == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_ = o.ledger.RecordVerification(verification)
}
