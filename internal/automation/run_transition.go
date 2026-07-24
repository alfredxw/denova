package automation

import (
	"fmt"
	"strings"
)

// preserveMonotonicRunReceipt makes same-operation state transitions robust
// to a projection cursor advancing between a caller's read and its append.
// It never merges across operation identities; stale op1 writers therefore
// still conflict after op2 has been promoted.
func preserveMonotonicRunReceipt(existing, next RunRecord, allowCompletionReopen bool) RunRecord {
	if next.RuntimeCommandID == existing.PendingRuntimeCommandID && next.RuntimeOperationID != existing.RuntimeOperationID {
		if next.RuntimeCommandFingerprint == "" {
			next.RuntimeCommandFingerprint = existing.PendingRuntimeCommandFingerprint
		}
		if next.RuntimeIntentHash == "" {
			next.RuntimeIntentHash = existing.PendingRuntimeIntentHash
		}
		if next.RuntimeReceiptCursor < existing.RuntimeReceiptCursor {
			next.RuntimeReceiptCursor = existing.RuntimeReceiptCursor
		}
	}
	if next.RuntimeCommandID == existing.RuntimeCommandID && next.RuntimeOperationID == existing.RuntimeOperationID {
		if next.RuntimeReceiptCursor < existing.RuntimeReceiptCursor {
			next.RuntimeReceiptCursor = existing.RuntimeReceiptCursor
		}
		if next.RuntimeCommandFingerprint == "" {
			next.RuntimeCommandFingerprint = existing.RuntimeCommandFingerprint
		}
		if next.RuntimeIntentHash == "" {
			next.RuntimeIntentHash = existing.RuntimeIntentHash
		}
		if next.CompletionEffectsOperationID == "" {
			next.CompletionEffectsOperationID = existing.CompletionEffectsOperationID
		}
		if existing.WriteConfirmationPolicyCaptured {
			next.WriteConfirmationPolicyCaptured = true
			next.WriteConfirmationRequired = existing.WriteConfirmationRequired
		}
		callerCoveredMutationPaths := runMutationPathsSubset(existing.CompletionMutationPaths, next.CompletionMutationPaths)
		callerCoveredMutationEffects := runMutationPathsSubset(existing.CompletionMutationEffectIDs, next.CompletionMutationEffectIDs)
		next.CompletionMutationPaths = mergeRunMutationPaths(existing.CompletionMutationPaths, next.CompletionMutationPaths)
		next.CompletionMutationEffectIDs = mergeRunMutationPaths(existing.CompletionMutationEffectIDs, next.CompletionMutationEffectIDs)
		reopeningCompletedEffects := allowCompletionReopen && completionEffectsWereExplicitlyReopened(existing, next)
		if existing.CompletionEffectsCompleted && !reopeningCompletedEffects {
			next.CompletionEffectsCompleted = true
			next.CompletionEffectsPending = false
		} else if existing.CompletionEffectsPending && !next.CompletionEffectsCompleted {
			next.CompletionEffectsPending = true
		}
		// A terminal writer may have started from a pre-effect snapshot. Derive
		// the pending plan from the merged durable paths so it cannot accidentally
		// acknowledge work admitted between its read and append.
		if isTerminalRunStatus(next.Status) && len(next.CompletionMutationPaths) > 0 && !existing.CompletionEffectsCompleted &&
			next.CompletionEffectsCompleted && (!callerCoveredMutationPaths || !callerCoveredMutationEffects) {
			if next.CompletionEffectsOperationID == "" {
				next.CompletionEffectsOperationID = next.RuntimeOperationID
			}
			next.CompletionEffectsPending = true
			next.CompletionEffectsCompleted = false
		}
	}
	return next
}

func completionEffectsWereExplicitlyReopened(existing, next RunRecord) bool {
	return existing.CompletionEffectsCompleted && next.CompletionEffectsPending && !next.CompletionEffectsCompleted &&
		next.CompletionEffectsOperationID == existing.RuntimeOperationID &&
		(!runMutationPathsSubset(next.CompletionMutationPaths, existing.CompletionMutationPaths) ||
			!runMutationPathsSubset(next.CompletionMutationEffectIDs, existing.CompletionMutationEffectIDs))
}

func mergeRunMutationPaths(existing, next []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(next))
	merged := make([]string, 0, len(existing)+len(next))
	for _, list := range [][]string{existing, next} {
		for _, path := range list {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			merged = append(merged, path)
		}
	}
	return merged
}

func validateRunAppendTransition(existing, next RunRecord, allowCompletionReopen bool) error {
	conflict := func(reason string) error {
		return fmt.Errorf("%w: run_id=%s reason=%s", ErrRunIdentityConflict, strings.TrimSpace(existing.ID), reason)
	}
	if strings.TrimSpace(existing.ID) == "" || existing.ID != next.ID {
		return conflict("run identity changed")
	}
	if existing.TaskID != "" && next.TaskID != existing.TaskID {
		return conflict("task identity changed")
	}
	if existing.SessionID != "" && next.SessionID != existing.SessionID {
		return conflict("session identity changed")
	}
	if existing.Scope != "" && next.Scope != existing.Scope {
		return conflict("scope changed")
	}
	if existing.Workspace != "" && canonicalStoreRoot(next.Workspace) != canonicalStoreRoot(existing.Workspace) {
		return conflict("workspace changed")
	}
	if existing.Trigger != "" && next.Trigger != existing.Trigger {
		return conflict("trigger changed")
	}
	if existing.SourceRunID != "" && next.SourceRunID != existing.SourceRunID {
		return conflict("source run changed")
	}

	existingRootSet := existing.RootRuntimeCommandID != "" || existing.RootRuntimeOperationID != "" || existing.RootRuntimeReceiptCursor != 0
	if existingRootSet && (next.RootRuntimeCommandID != existing.RootRuntimeCommandID ||
		next.RootRuntimeOperationID != existing.RootRuntimeOperationID ||
		next.RootRuntimeReceiptCursor != existing.RootRuntimeReceiptCursor) {
		return conflict("root runtime receipt changed")
	}

	existingCurrentSet := existing.RuntimeCommandID != "" || existing.RuntimeOperationID != "" || existing.RuntimeReceiptCursor != 0
	preAdmissionRetry := existing.Status == RunStatusFailed && next.Status == RunStatusRunning &&
		next.RuntimeAdmissionPending && !runHasRuntimeReceipt(existing) && !runHasRuntimeReceipt(next)
	operationAdvanced := false
	if existingCurrentSet {
		if next.RuntimeCommandID == existing.RuntimeCommandID && next.RuntimeOperationID == existing.RuntimeOperationID {
			if next.RuntimeReceiptCursor < existing.RuntimeReceiptCursor {
				return conflict("current runtime receipt cursor regressed")
			}
			if existing.RuntimeCommandFingerprint != "" && next.RuntimeCommandFingerprint != existing.RuntimeCommandFingerprint {
				return conflict("current runtime command fingerprint changed")
			}
			if existing.RuntimeIntentHash != "" && next.RuntimeIntentHash != existing.RuntimeIntentHash {
				return conflict("current runtime intent changed")
			}
		} else {
			if strings.TrimSpace(existing.PendingRuntimeCommandID) == "" || next.RuntimeCommandID != existing.PendingRuntimeCommandID {
				return conflict("current runtime operation changed without its pending successor intent")
			}
			if next.RuntimeOperationID == "" || next.RuntimeReceiptCursor == 0 {
				return conflict("successor runtime receipt is incomplete or stale")
			}
			operationAdvanced = true
			if existing.PendingRuntimeCommandFingerprint != "" && next.RuntimeCommandFingerprint != existing.PendingRuntimeCommandFingerprint {
				return conflict("successor runtime command fingerprint differs from pending intent")
			}
			if next.RuntimeIntentHash != existing.PendingRuntimeIntentHash {
				return conflict("successor runtime intent differs from pending intent")
			}
		}
	}
	if isTerminalRunStatus(existing.Status) {
		if next.Status == RunStatusRunning && !operationAdvanced && !preAdmissionRetry {
			return conflict("terminal run regressed to running without successor promotion")
		}
		if isTerminalRunStatus(next.Status) && next.Status != existing.Status && !operationAdvanced {
			return conflict("terminal status changed for the same runtime operation")
		}
	}
	if !operationAdvanced {
		if existing.CompletionEffectsOperationID != "" && next.CompletionEffectsOperationID != existing.CompletionEffectsOperationID {
			return conflict("completion-effects operation epoch changed")
		}
		if existing.CompletionEffectsCompleted && !(allowCompletionReopen && completionEffectsWereExplicitlyReopened(existing, next)) {
			if !next.CompletionEffectsCompleted || next.CompletionEffectsPending {
				return conflict("completed effects regressed")
			}
			if !runMutationPathsSubset(next.CompletionMutationPaths, existing.CompletionMutationPaths) {
				return conflict("completed effects acquired an unacknowledged mutation path")
			}
		}
		if existing.WriteConfirmationPolicyCaptured && (!next.WriteConfirmationPolicyCaptured || next.WriteConfirmationRequired != existing.WriteConfirmationRequired) {
			return conflict("captured write-confirmation policy changed")
		}
	}
	if next.RuntimeAdmissionPending && (runHasRuntimeReceipt(existing) || runHasRuntimeReceipt(next)) {
		return conflict("initial runtime admission intent overlaps a durable receipt")
	}
	if existing.RuntimeAdmissionPending && !next.RuntimeAdmissionPending && !runHasRuntimeReceipt(next) && !isTerminalRunStatus(next.Status) {
		return conflict("initial runtime admission intent cleared without receipt or terminal proof")
	}

	existingPending := strings.TrimSpace(existing.PendingRuntimeCommandID)
	nextPending := strings.TrimSpace(next.PendingRuntimeCommandID)
	if (nextPending == "") != (strings.TrimSpace(next.PendingRuntimeIntentHash) == "") ||
		(nextPending == "" && strings.TrimSpace(next.PendingRuntimeCommandFingerprint) != "") {
		return conflict("pending successor identity is incomplete")
	}
	if existingPending != "" {
		switch {
		case nextPending == existingPending && next.PendingRuntimeIntentHash == existing.PendingRuntimeIntentHash &&
			(existing.PendingRuntimeCommandFingerprint == "" || next.PendingRuntimeCommandFingerprint == existing.PendingRuntimeCommandFingerprint):
		case nextPending == "" && next.RuntimeCommandID == existingPending:
		case nextPending == "" && next.RuntimeCommandID == existing.RuntimeCommandID &&
			next.RuntimeOperationID == existing.RuntimeOperationID && strings.TrimSpace(next.RuntimeSuccessorConflict) != "":
		default:
			return conflict("pending successor intent changed or was cleared without promotion")
		}
	}
	if operationAdvanced && nextPending != "" {
		return conflict("promoted successor retained its pending intent")
	}
	return nil
}

func runHasRuntimeReceipt(run RunRecord) bool {
	return strings.TrimSpace(run.RuntimeCommandID) != "" && strings.TrimSpace(run.RuntimeOperationID) != "" && run.RuntimeReceiptCursor > 0
}

func runMutationPathsSubset(candidate, superset []string) bool {
	allowed := make(map[string]struct{}, len(superset))
	for _, path := range superset {
		allowed[strings.TrimSpace(path)] = struct{}{}
	}
	for _, path := range candidate {
		if _, ok := allowed[strings.TrimSpace(path)]; !ok {
			return false
		}
	}
	return true
}

func isTerminalRunStatus(status string) bool {
	return status == RunStatusSuccess || status == RunStatusFailed || status == RunStatusAborted
}

// RunHasRuntimeObligation reports whether deleting a task would hide control
// for accepted runtime work. Completion effects are intentionally excluded:
// an archived task remains in the durable ledger until that outbox settles.
func RunHasRuntimeObligation(run RunRecord) bool {
	return run.Status == RunStatusRunning || run.RuntimeAdmissionPending || run.RuntimeRecoveryRequired || strings.TrimSpace(run.PendingRuntimeCommandID) != ""
}

// RunHasDurableObligation is the exact startup-recovery predicate. Settled
// history remains queryable in the full ledger but is removed from the hot
// obligation directory as soon as every terminal effect is acknowledged.
// Legacy successes without a durable runtime receipt predate completion
// effects and must not be reinterpreted as unfinished work.
func RunHasDurableObligation(run RunRecord) bool {
	return RunHasRuntimeObligation(run) || run.CompletionEffectsPending ||
		(run.Status == RunStatusSuccess && !run.CompletionEffectsCompleted && runHasRuntimeReceipt(run))
}
