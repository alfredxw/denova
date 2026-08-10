package execution

import (
	"context"
	"denova/internal/agents/run"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// recoveryCancellationAbortIdentity preserves idempotency while cancellation
// retries one operation, then rotates identity when the durable binding advances
// to an accepted successor operation.
type recoveryCancellationAbortIdentity struct {
	operationID runstate.OperationID
	commandID   runstate.CommandID
}

func (i *recoveryCancellationAbortIdentity) forOperation(operationID runstate.OperationID) runstate.CommandID {
	if operationID == "" {
		return ""
	}
	if i.operationID != operationID {
		i.operationID = operationID
		i.commandID = runstate.CommandID(newOperationIdentity("recovery-abort"))
	}
	return i.commandID
}

func (r *RecoveryObservation) Resume(
	ctx context.Context,
	action RuntimeRecoveryAction,
	taskID string,
	emit func(agentrun.Event),
) (agentrun.CommandReceipt, error) {
	if r == nil || r.harness == nil {
		return agentrun.CommandReceipt{}, ErrRuntimeProjectionUnavailable
	}
	status, err := r.harness.Status(ctx)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	projected, projectionErr := agentrun.RuntimeStatusFromSnapshot(status)
	if projectionErr != nil {
		return agentrun.CommandReceipt{}, projectionErr
	}
	if !containsRuntimeRecoveryAction(RuntimeRecoveryActions(projected), action) {
		return agentrun.CommandReceipt{}, fmt.Errorf("%w: kind=%q command_id=%q operation_id=%q", ErrRecoveryActionChanged, action.Kind, action.CommandID, action.OperationID)
	}
	recoveryCtx := withRecoveryDisplayRoute(ctx, recoveryDisplayRoute{TaskID: taskID, Emit: emit})
	r.harness.BindRecoveryContext(recoveryCtx)
	r.mu.Lock()
	r.boundRoute = recoveryDisplayRoute{TaskID: strings.TrimSpace(taskID), Emit: emit}
	r.mu.Unlock()
	if action.Kind == RuntimeRecoveryAttach {
		return agentrun.CommandReceipt{
			CommandID: action.CommandID, OperationID: action.OperationID,
			Cursor: agentrun.Cursor(status.Cursor), Replayed: true,
		}, nil
	}
	if action.Kind == RuntimeRecoveryAbort {
		receipt, err := r.harness.Submit(recoveryCtx, runstate.Abort{
			ID: runstate.CommandID(action.CommandID), OperationID: runstate.OperationID(action.OperationID),
			Reason: "user explicitly aborted a recovery-paused operation",
		})
		return agentrun.CommandReceiptFromRuntime(receipt), err
	}
	var delivery runstate.DeliveryKind
	switch action.Kind {
	case RuntimeRecoverySteer:
		delivery = runstate.DeliverySteer
	case RuntimeRecoveryFollowUp:
		delivery = runstate.DeliveryFollowUp
	case RuntimeRecoveryNextTurn:
		delivery = runstate.DeliveryNextTurn
	default:
		return agentrun.CommandReceipt{}, fmt.Errorf("%w: structural action requires the structural recovery seam", ErrRecoveryActionChanged)
	}
	receipt, err := r.harness.RecoverAcceptedInput(recoveryCtx, runstate.RecoveryAction{
		Kind: delivery, CommandID: runstate.CommandID(action.CommandID), OperationID: runstate.OperationID(action.OperationID),
	})
	if errors.Is(err, runstate.ErrRecoveryActionChanged) {
		return agentrun.CommandReceiptFromRuntime(receipt), fmt.Errorf("%w: %v", ErrRecoveryActionChanged, err)
	}
	return agentrun.CommandReceiptFromRuntime(receipt), err
}

func containsRuntimeRecoveryAction(actions []RuntimeRecoveryAction, selected RuntimeRecoveryAction) bool {
	for _, action := range actions {
		if action == selected {
			return true
		}
	}
	return false
}

// Wait follows the durable binding rather than a browser connection. Existing
// partial output comes from the active projection; newly restored execution
// emits through its restored adapter spec, and this observer supplies exactly
// one terminal display event.
func (r *RecoveryObservation) Wait(ctx context.Context, emit func(agentrun.Event)) agentrun.Outcome {
	if r == nil || r.harness == nil {
		err := ErrRuntimeProjectionUnavailable
		return agentrun.NewOutcome(agentrun.OutcomeFailed, err, err.Error(), "", "")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	initial := r.initial
	if initial.Phase == runstate.PhaseIdle && len(initial.Queue) == 0 && initial.LastOperation != nil {
		outcome := replayedOutcomeForSettlement(ctx, r.harness, initial.LastOperation.OperationID, runstate.OperationSettledEvent{
			OperationID: initial.LastOperation.OperationID, Status: initial.LastOperation.Status, Reason: initial.LastOperation.Reason,
		})
		emitReplayedRunOutcome(emit, outcome)
		return outcome
	}

	events := r.observation.Events
	errorsCh := r.observation.Errors
	callerDone := ctx.Done()
	cancellationRequested := false
	var cancellationAbortIdentity recoveryCancellationAbortIdentity
	submitCancellationAbort := func() {
		if !cancellationRequested {
			return
		}
		status, _ := r.harness.Status(context.Background())
		if status.Phase == runstate.PhaseIdle || status.ActiveOperation == "" {
			return
		}
		_, abortErr := r.harness.Submit(context.Background(), runstate.Abort{
			ID: cancellationAbortIdentity.forOperation(status.ActiveOperation), OperationID: status.ActiveOperation,
			Reason: "recovery display task canceled",
		})
		if abortErr != nil && !errors.Is(abortErr, runstate.ErrInvalidCommand) &&
			!errors.Is(abortErr, runstate.ErrStaleOperation) &&
			!errors.Is(abortErr, runstate.ErrDomainCommitRejected) {
			slog.ErrorContext(ctx, fmt.Sprintf("[agent-recovery] abort after display cancellation failed operation_id=%s err=%v", status.ActiveOperation, abortErr))
		}
	}
	for {
		select {
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			switch event.Payload.(type) {
			case runstate.OperationRecoveryPausedEvent, runstate.InputMaterializationRecoveryPendingEvent:
				status, statusErr := r.harness.Status(context.Background())
				if statusErr != nil {
					emitExecutionError(emit, statusErr)
					return agentrun.NewOutcome(agentrun.OutcomeFailed, statusErr, statusErr.Error(), "", "")
				}
				if status.RecoveryPaused {
					// Cancellation owns a stronger lifecycle decision than a second
					// recovery pause. If both become visible together, preserve the
					// admitted task's abort/drain contract and wait for durable terminal
					// state instead of releasing its workspace lease as recoverable.
					if ctx.Err() != nil {
						callerDone = nil
						cancellationRequested = true
						submitCancellationAbort()
						continue
					}
					emitRuntimeRecoveryRequired(emit, status)
					// Keep this restart-scoped Task and observation alive. The next
					// server-derived action resumes through RecoveryObservation.Resume;
					// replacing the Task here creates a race where a client attaches to
					// an observer that is concurrently closing.
					continue
				}
			case runstate.OperationSettledEvent, runstate.OperationInterruptedEvent:
				status, statusErr := r.harness.Status(context.Background())
				if statusErr != nil {
					emitExecutionError(emit, statusErr)
					return agentrun.NewOutcome(agentrun.OutcomeFailed, statusErr, statusErr.Error(), "", "")
				}
				if status.Phase != runstate.PhaseIdle || len(status.Queue) != 0 || status.LastOperation == nil {
					continue
				}
				outcome := replayedOutcomeForSettlement(context.Background(), r.harness, status.LastOperation.OperationID, runstate.OperationSettledEvent{
					OperationID: status.LastOperation.OperationID, Status: status.LastOperation.Status, Reason: status.LastOperation.Reason,
				})
				emitRecoveryTerminal(emit, outcome)
				return outcome
			}
		case observationErr, ok := <-errorsCh:
			if !ok {
				errorsCh = nil
				continue
			}
			if observationErr != nil {
				emitExecutionError(emit, observationErr)
				return agentrun.NewOutcome(agentrun.OutcomeFailed, observationErr, observationErr.Error(), "", "")
			}
		case <-callerDone:
			callerDone = nil
			cancellationRequested = true
		}
		// A durable Abort receipt may precede a transient canonical-input
		// materialization failure. Retry the exact same command identity only
		// when another actor event gives us progress; fail-once dependencies then
		// settle without minting a conflicting Abort or spinning.
		if cancellationRequested {
			submitCancellationAbort()
		}
		if events == nil && errorsCh == nil {
			err := fmt.Errorf("agent recovery observation closed before the binding became idle")
			emitExecutionError(emit, err)
			return agentrun.NewOutcome(agentrun.OutcomeFailed, err, err.Error(), "", "")
		}
	}
}

func emitRecoveryTerminal(emit func(agentrun.Event), outcome agentrun.Outcome) {
	if emit == nil {
		return
	}
	switch outcome.Status {
	case agentrun.OutcomeCompleted:
		emit(agentrun.Event{Type: "done", Data: map[string]string{}})
	case agentrun.OutcomeAborted:
		emit(agentrun.Event{Type: "aborted", Data: map[string]string{"reason": outcome.Reason}})
	default:
		message := outcome.Reason
		if message == "" && outcome.Error != nil {
			message = outcome.Error.Error()
		}
		emit(agentrun.Event{Type: "error", Data: map[string]string{"message": message}})
	}
}
