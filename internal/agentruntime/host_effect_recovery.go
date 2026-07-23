package agentruntime

import (
	"context"
	"fmt"
	"strings"
)

// ReconcileHostEffects retries every pending durable host-effect outbox item
// through the binding Engine's idempotent reconciler and durably acknowledges
// each success. It is safe to call after an ambiguous live callback result.
func (h *Harness) ReconcileHostEffects(ctx context.Context) error {
	if h == nil {
		return ErrHostEffectRequired
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := h.terminalError(); err != nil {
		return err
	}
	request := reconcileHostEffectsRequest{ctx: ctx, response: make(chan error, 1)}
	select {
	case h.requests <- request:
	case <-h.done:
		return h.terminalError()
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-request.response:
		return err
	case <-h.done:
		return h.terminalError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *Harness) reconcilePendingHostEffects(ctx context.Context, state *harnessState) error {
	if state == nil || len(state.pendingHostEffectOrder) == 0 {
		return nil
	}
	reconciler, ok := h.engine.(EngineHostEffectReconciler)
	if !ok {
		return fmt.Errorf("%w: %d pending effect(s) have no EngineHostEffectReconciler", ErrHostEffectRequired, len(state.pendingHostEffectOrder))
	}
	ids := append([]HostEffectID(nil), state.pendingHostEffectOrder...)
	for _, id := range ids {
		effect, pending := state.pendingHostEffects[id]
		if !pending {
			continue
		}
		if !hostEffectHasExactOutputReceipt(state, effect) {
			return fmt.Errorf(
				"%w: reconcile %q requires the exact output-domain commit receipt for operation %s cycle %d",
				ErrHostEffectRequired,
				id,
				effect.OperationID,
				effect.Cycle,
			)
		}
		if err := reconcileHostEffect(ctx, reconciler, effect); err != nil {
			return fmt.Errorf("%w: reconcile %q: %v", ErrHostEffectRequired, id, err)
		}
		if _, err := h.commit(context.WithoutCancel(ctx), state, []EventPayload{
			HostEffectAcknowledgedEvent{ID: id},
		}); err != nil {
			return fmt.Errorf("acknowledge reconciled host effect %q: %w", id, err)
		}
	}
	if state.pendingEngineDone != nil {
		pending := *state.pendingEngineDone
		state.pendingEngineDone = nil
		h.handleEngineDone(state, pending)
	}
	return nil
}

func hostEffectHasExactOutputReceipt(state *harnessState, effect HostEffect) bool {
	if state == nil {
		return false
	}
	commit := state.domainCommit(DomainCommitOutput)
	return commit != nil && strings.TrimSpace(commit.Revision) != "" &&
		commit.Identity.OperationID == effect.OperationID && commit.Identity.Cycle == effect.Cycle
}

// abandonUncommittedHostEffects durably closes effects from a cycle that did
// not obtain its exact canonical-output receipt. It deliberately does not call
// the host reconciler: these mutations may remain visible in workspace storage,
// but they cannot authorize downstream automation for an uncommitted cycle.
func (h *Harness) abandonUncommittedHostEffects(ctx context.Context, state *harnessState, reason string) error {
	if state == nil || len(state.pendingHostEffectOrder) == 0 {
		return nil
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "exact output-domain commit receipt was not recorded"
	}
	payloads := make([]EventPayload, 0, len(state.pendingHostEffectOrder))
	for _, id := range state.pendingHostEffectOrder {
		if _, pending := state.pendingHostEffects[id]; pending {
			payloads = append(payloads, HostEffectAbandonedEvent{ID: id, Reason: reason})
		}
	}
	if len(payloads) == 0 {
		return nil
	}
	if _, err := h.commit(context.WithoutCancel(ctx), state, payloads); err != nil {
		return fmt.Errorf("abandon uncommitted host effects: %w", err)
	}
	return nil
}

// reconcileRecoveredHostEffects runs only after canonical commit
// reconciliation. A still-pending output intent is ambiguous and remains
// fenced; an exact receipt transfers effects, while an absent/abandoned output
// commit explicitly discards them before operation recovery continues.
func (h *Harness) reconcileRecoveredHostEffects(ctx context.Context, state *harnessState) error {
	if state == nil || len(state.pendingHostEffectOrder) == 0 {
		return nil
	}
	allCommitted := true
	anyCommitted := false
	for _, id := range state.pendingHostEffectOrder {
		effect, pending := state.pendingHostEffects[id]
		if !pending {
			continue
		}
		committed := hostEffectHasExactOutputReceipt(state, effect)
		allCommitted = allCommitted && committed
		anyCommitted = anyCommitted || committed
	}
	if allCommitted {
		return h.reconcilePendingHostEffects(ctx, state)
	}
	if anyCommitted {
		return fmt.Errorf("%w: pending HostEffects span different output-commit epochs", ErrHostEffectRequired)
	}
	if pending := state.domainCommit(DomainCommitOutput); pending != nil && strings.TrimSpace(pending.Revision) == "" {
		return fmt.Errorf("%w: output-domain commit remains ambiguous", ErrHostEffectRequired)
	}
	return h.abandonUncommittedHostEffects(ctx, state, "recovered cycle has no exact output-domain commit receipt")
}

// ReconcileHostEffects opens the exact binding (which first resolves the
// output-commit fence) and retries any remaining live pending effects on the
// actor lane.
func (r *Runtime) ReconcileHostEffects(ctx context.Context, binding Binding) error {
	harness, err := r.Open(ctx, binding)
	if err != nil {
		return err
	}
	return harness.ReconcileHostEffects(ctx)
}
