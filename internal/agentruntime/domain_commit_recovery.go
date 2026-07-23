package agentruntime

import (
	"context"
	"fmt"
	"strings"
)

// reconcilePendingDomainCommits closes the only ambiguous cross-store window:
// canonical state may be durable after an accepted intent even though the
// coordinator receipt append was lost to a crash. It never replays effects.
func (h *Harness) reconcilePendingDomainCommits(ctx context.Context, state *harnessState) error {
	reconciler, ok := h.engine.(EngineDomainCommitReconciler)
	if !ok {
		return nil
	}
	for _, stage := range []DomainCommitStage{DomainCommitInput, DomainCommitOutput} {
		commit := state.domainCommit(stage)
		if commit == nil || strings.TrimSpace(commit.Revision) != "" {
			continue
		}
		request := DomainCommitReconcileRequest{
			Binding:    state.binding,
			Commit:     *cloneDomainCommitState(commit),
			Structural: cloneStructuralOperationSnapshot(state.activeStructural),
		}
		result, err := queryDomainCommit(ctx, reconciler, request)
		if err != nil {
			return fmt.Errorf("reconcile authorized %s domain commit: %w", stage, err)
		}
		result.Revision = strings.TrimSpace(result.Revision)
		if !result.Found {
			if result.Revision != "" {
				return fmt.Errorf("reconcile authorized %s domain commit: not-found result contains revision", stage)
			}
			if _, err := h.commit(ctx, state, []EventPayload{DomainCommitReconciliationAbandonedEvent{
				Identity: commit.Identity,
				Hash:     commit.Hash,
				Reason:   "canonical reconciliation authoritatively reported not found",
			}}); err != nil {
				return fmt.Errorf("persist abandoned %s domain commit reconciliation: %w", stage, err)
			}
			// Input is queried before output. A missing earlier canonical stage
			// blocks reconciliation of any later intent in a corrupt journal, but
			// its own authorization fence is now durably closed.
			return nil
		}
		if result.Revision == "" {
			return fmt.Errorf("reconcile authorized %s domain commit: found result requires revision", stage)
		}
		if _, err := h.commit(ctx, state, []EventPayload{DomainCommitReceiptEvent{
			Identity: commit.Identity,
			Hash:     commit.Hash,
			Revision: result.Revision,
		}}); err != nil {
			return fmt.Errorf("persist reconciled %s domain commit receipt: %w", stage, err)
		}
	}
	return nil
}

func queryDomainCommit(
	ctx context.Context,
	reconciler EngineDomainCommitReconciler,
	request DomainCommitReconcileRequest,
) (result DomainCommitReconcileResult, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			resultErr = fmt.Errorf("domain commit reconciler panic: %v", recovered)
		}
	}()
	return reconciler.ReconcileDomainCommit(ctx, request)
}
