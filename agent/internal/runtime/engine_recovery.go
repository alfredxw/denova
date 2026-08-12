package runtime

import (
	"context"
	"fmt"
	"strings"
)

func (h *Harness) uncertainToolResults(state *harnessState) []EventPayload {
	calls := state.activeToolCalls()
	payloads := make([]EventPayload, 0, len(calls))
	for _, call := range calls {
		payloads = append(payloads, ToolCallFinishedEvent{
			CallID: call.CallID, Name: call.Name,
			ResultDescriptor: describePayload([]byte(UnknownToolEffectResult)),
			IsError:          true, RetrySafety: RetryUnknown,
		})
	}
	return payloads
}

func (h *Harness) recoverUnfinished(ctx context.Context, state *harnessState) error {
	if state.phase == PhaseIdle {
		if err := h.abandonUncommittedHostEffects(ctx, state, "idle runtime has no active cycle output receipt"); err != nil {
			return err
		}
		// Opening a binding is a reconciliation boundary, not permission to run
		// user work. Keep durable NextTurn input queued until the caller replays
		// that exact command; only transient inputs need deterministic cleanup.
		payloads := transientQueueCancellations(state, "runtime_recovered")
		if len(payloads) == 0 {
			return nil
		}
		if _, err := h.commit(ctx, state, payloads); err != nil {
			return fmt.Errorf("recover idle input queue: %w", err)
		}
		return nil
	}
	if state.inputRecovery != nil && !state.abortRequested {
		// The selected queued cycle has not crossed its canonical-input outbox.
		// Engine-authorized autonomous work may resume on open. Host inputs remain
		// attach-only and require their exact command/recovery action.
		if state.inputRecovery.Autonomous {
			return h.resumePendingInputMaterialization(ctx, state, true)
		}
		return nil
	}
	if err := h.ensureInputMaterialized(ctx, state); err != nil {
		return fmt.Errorf("recover accepted input: %w", err)
	}
	if err := h.reconcilePendingDomainCommits(ctx, state); err != nil {
		return err
	}
	if err := h.reconcileRecoveredHostEffects(ctx, state); err != nil {
		return err
	}
	operationID := state.activeOperation
	if continuation, ok := state.firstAutonomousContinuation(); ok && len(state.activeToolCalls()) == 0 {
		return h.resumeAutonomousContinuation(ctx, state, continuation)
	}
	// Recovery may reconcile canonical state, but it must never restore and
	// execute a queued turn implicitly. Exact command replay resumes it later.
	plan := h.planNextTurn(ctx, state, false)
	// A reconciled output receipt is terminal evidence, not an uncertain engine
	// attempt. Preserve its precedence before the general recovery-pause rule.
	if state.pendingDomainCommit() == nil && state.acknowledgedOutputCommit() != nil {
		payloads := []EventPayload{SavePointCommittedEvent{OperationID: operationID, Cycle: state.activeCycle}}
		payloads = append(payloads, OperationSettledEvent{
			OperationID: operationID,
			Status:      OperationSucceeded,
			Reason:      appendOperationReason("recovered acknowledged domain commit", plan.pendingReason()),
		})
		payloads = plan.appendStart(payloads)
		if _, err := h.commit(ctx, state, payloads); err != nil {
			return fmt.Errorf("recover acknowledged domain commit %s: %w", operationID, err)
		}
		return nil
	}
	if state.abortRequested {
		payloads := h.uncertainToolResults(state)
		payloads = append(payloads, transientQueueCancellations(state, "runtime_recovered")...)
		reason := strings.TrimSpace(state.abortReason)
		if reason == "" {
			reason = "agent operation aborted"
		}
		payloads = append(payloads, OperationSettledEvent{
			OperationID: operationID,
			Status:      OperationAborted,
			Reason:      appendOperationReason(reason, plan.pendingReason()),
		})
		payloads = plan.appendStart(payloads)
		if _, err := h.commit(ctx, state, payloads); err != nil {
			return fmt.Errorf("recover aborted operation %s: %w", operationID, err)
		}
		return nil
	}
	// A crashed Running operation remains an explicit recovery boundary even when
	// no follow-up was accepted before the process disappeared. Opening the
	// binding must not rerun its StartTurn or disguise it as idle: after a display
	// task reattaches, a fresh Steer/FollowUp/Abort/NextTurn can deterministically
	// resume, terminate, or supersede the uncertain parent.
	if state.phase == PhaseCompacting || state.phase == PhaseRunning {
		return h.pauseRecoveredOperation(ctx, state)
	}
	payloads := h.uncertainToolResults(state)
	payloads = append(payloads, transientQueueCancellations(state, "runtime_recovered")...)
	reason := "runtime recovered an unfinished operation; no engine or tool effect was retried"
	if pending := state.pendingDomainCommit(); pending != nil {
		reason = fmt.Sprintf("runtime recovered an authorized %s domain commit without a receipt; canonical state requires reconciliation", pending.Identity.Stage)
	}
	reason = appendOperationReason(reason, plan.pendingReason())
	payloads = append(payloads, OperationInterruptedEvent{
		OperationID: operationID,
		Reason:      reason,
	})
	payloads = plan.appendStart(payloads)
	if _, err := h.commit(ctx, state, payloads); err != nil {
		return fmt.Errorf("recover unfinished operation %s: %w", operationID, err)
	}
	return nil
}

type acceptedNextTurnSettlement uint8

const (
	resumeAcceptedNextTurn acceptedNextTurnSettlement = iota
	preserveAcceptedNextTurn
)

func (h *Harness) settleSuccessfulOperation(
	state *harnessState,
	warning string,
	nextTurn acceptedNextTurnSettlement,
	transientCancellationReason string,
) {
	operationID := state.activeOperation
	plan := h.planNextTurn(h.lifecycle, state, nextTurn == resumeAcceptedNextTurn && !state.closing)
	payloads := []EventPayload{SavePointCommittedEvent{OperationID: operationID, Cycle: state.activeCycle}}
	if transientCancellationReason != "" {
		payloads = append(payloads, transientQueueCancellations(state, transientCancellationReason)...)
	}
	payloads = append(payloads, OperationSettledEvent{
		OperationID: operationID,
		Status:      OperationSucceeded,
		Reason:      appendOperationReason(warning, plan.pendingReason()),
	})
	payloads = plan.appendStart(payloads)
	if _, err := h.commit(context.Background(), state, payloads); err != nil {
		panic(fmt.Sprintf("operation %s failed to settle successfully: %v", operationID, err))
	}
	_ = h.startPlannedNextTurn(state, plan)
}

func (h *Harness) settleAcknowledgedOutput(state *harnessState, warning string) {
	nextTurn := resumeAcceptedNextTurn
	if state.closing {
		nextTurn = preserveAcceptedNextTurn
	}
	h.settleSuccessfulOperation(state, warning, nextTurn, "late_error_after_acknowledged_output")
}

func acknowledgedOutputCompletionWarning(state *harnessState, request engineDoneRequest) (string, bool) {
	if state.acknowledgedOutputCommit() == nil {
		return "", false
	}
	var detail string
	switch {
	case request.err != nil:
		detail = request.err.Error()
	case state.abortRequested:
		detail = "abort arrived after canonical output acknowledgement"
	case request.result.Status == EngineAborted:
		detail = "engine reported aborted after canonical output acknowledgement"
	case len(state.activeToolCalls()) > 0:
		detail = "engine returned with open tool effects after canonical output acknowledgement"
	case request.result.Status != EngineCompleted && request.result.Status != EnginePreempted:
		detail = fmt.Sprintf("engine returned unsupported status %q after canonical output acknowledgement", request.result.Status)
	default:
		return "", false
	}
	return "canonical output was acknowledged; late engine warning: " + strings.TrimSpace(detail), true
}
