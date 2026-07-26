package app

import (
	"context"

	agents "denova/internal/agents"
	"denova/internal/agents/session"
)

// reconcileColdPendingAsk closes only an orphaned Ask owned by the exact
// durable cycle exposed by runtime recovery. Session performs the waiter check
// and journal transition under its canonical mutation lease, so calling this
// from a projection remains harmless for a healthy in-process Ask.
func reconcileColdPendingAsk(ctx context.Context, sess *session.Session, runtime agents.RuntimeStatus) (bool, error) {
	if sess == nil || (!runtime.RecoveryPaused && !runtime.RecoveryPending) || runtime.ActiveOperation == "" || runtime.ActiveCycle <= 0 {
		return false, nil
	}
	return sess.ReconcileStalePendingAsk(ctx, session.AskCycleIdentity{
		CommandID:   string(runtime.ActiveCommandID),
		OperationID: string(runtime.ActiveOperation),
		Cycle:       runtime.ActiveCycle,
	})
}
