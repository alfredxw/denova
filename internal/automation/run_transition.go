package automation

import (
	"fmt"
	"reflect"
	"strings"
)

func validateDeliveryRecord(run RunRecord) error {
	switch run.DeliveryStatus {
	case "":
		return nil // released history
	case DeliveryPending:
		if run.Input == nil || strings.TrimSpace(run.Input.Message) == "" {
			return fmt.Errorf("pending delivery requires its input")
		}
	case DeliveryAccepted:
		if run.RootRuntimeCommandID == "" || run.RootRuntimeOperationID == "" || run.RootRuntimeReceiptCursor == 0 {
			return fmt.Errorf("accepted delivery requires its exact receipt")
		}
	default:
		return fmt.Errorf("unknown delivery status %q", run.DeliveryStatus)
	}
	return nil
}

func validateRunAppendTransition(existing, next RunRecord) error {
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

	if existing.ProjectID != "" && next.ProjectID != existing.ProjectID {
		return conflict("project identity changed")
	}
	if existing.TurnID != "" && next.TurnID != existing.TurnID {
		return conflict("command identity changed")
	}
	if existing.DeliveryStatus != "" && (!reflect.DeepEqual(existing.TriggerEvidence, next.TriggerEvidence) || existing.SourceRunID != next.SourceRunID) {
		return conflict("trigger intent changed")
	}
	if existing.Scope != "" && next.Scope != existing.Scope {
		return conflict("scope changed")
	}
	if existing.ProjectID == "" && existing.Workspace != "" && canonicalStoreRoot(next.Workspace) != canonicalStoreRoot(existing.Workspace) {
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

	if existing.DeliveryStatus != "" || next.DeliveryStatus != "" {
		if next.DeliveryStatus != DeliveryPending && next.DeliveryStatus != DeliveryAccepted {
			return conflict("invalid delivery status")
		}
		if existing.DeliveryStatus == DeliveryAccepted && next.DeliveryStatus != DeliveryAccepted {
			return conflict("accepted delivery regressed")
		}
		if existing.Input != nil && !reflect.DeepEqual(existing.Input, next.Input) {
			return conflict("delivery input changed")
		}
		if next.DeliveryStatus == DeliveryAccepted && (next.RootRuntimeCommandID == "" || next.RootRuntimeOperationID == "" || next.RootRuntimeReceiptCursor == 0) {
			return conflict("accepted delivery requires its exact receipt")
		}
		return nil
	}

	return nil
}

func isTerminalRunStatus(status string) bool {
	return status == RunStatusSuccess || status == RunStatusFailed || status == RunStatusAborted
}

// RunHasRuntimeObligation protects a pending handoff from deletion. Released
// records are adopted once; accepted delivery never owns Agent recovery.
func RunHasRuntimeObligation(run RunRecord) bool {
	if run.DeliveryStatus != "" {
		return run.DeliveryStatus == DeliveryPending
	}
	return run.Status == RunStatusRunning || run.RuntimeAdmissionPending || run.RuntimeRecoveryRequired || strings.TrimSpace(run.PendingRuntimeCommandID) != ""
}

// RunHasDurableObligation retains delivery retries and released effect outboxes.
func RunHasDurableObligation(run RunRecord) bool {
	return RunHasRuntimeObligation(run) || run.CompletionEffectsPending ||
		(run.DeliveryStatus == "" && run.Status == RunStatusSuccess && !run.CompletionEffectsCompleted && run.RuntimeOperationID != "" && run.RuntimeReceiptCursor > 0)
}

// Legacy effect receipts can only settle. No current code adds or reopens an
// Automation execution outbox; every new Agent mutation uses the generic outbox.
func preserveLegacyEffects(existing, next RunRecord) RunRecord {
	if existing.CompletionEffectsCompleted {
		next.CompletionEffectsPending = false
		next.CompletionEffectsCompleted = true
	}
	if existing.CompletionEffectsOperationID != "" {
		next.CompletionEffectsOperationID = existing.CompletionEffectsOperationID
	}
	next.CompletionMutationPaths = append([]string(nil), existing.CompletionMutationPaths...)
	next.CompletionMutationEffectIDs = append([]string(nil), existing.CompletionMutationEffectIDs...)
	return next
}
