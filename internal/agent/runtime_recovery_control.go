package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"denova/internal/agentruntime"
)

// recoveryCancellationAbortIdentity preserves idempotency while cancellation
// retries one operation, then rotates identity when the durable binding advances
// to an accepted successor operation.
type recoveryCancellationAbortIdentity struct {
	operationID agentruntime.OperationID
	commandID   agentruntime.CommandID
}

func (i *recoveryCancellationAbortIdentity) forOperation(operationID agentruntime.OperationID) agentruntime.CommandID {
	if operationID == "" {
		return ""
	}
	if i.operationID != operationID {
		i.operationID = operationID
		i.commandID = agentruntime.CommandID(newHarnessIdentity("recovery-abort"))
	}
	return i.commandID
}

func (r *RecoveryObservation) Resume(
	ctx context.Context,
	action RuntimeRecoveryAction,
	taskID string,
	emit func(Event),
) (agentruntime.Receipt, error) {
	if r == nil || r.harness == nil {
		return agentruntime.Receipt{}, ErrRuntimeProjectionUnavailable
	}
	status, err := r.harness.Status(ctx)
	if err != nil {
		return agentruntime.Receipt{}, err
	}
	if !containsRuntimeRecoveryAction(RuntimeRecoveryActions(status), action) {
		return agentruntime.Receipt{}, fmt.Errorf("%w: kind=%q command_id=%q operation_id=%q", ErrRecoveryActionChanged, action.Kind, action.CommandID, action.OperationID)
	}
	recoveryCtx := withRecoveryDisplayRoute(ctx, recoveryDisplayRoute{TaskID: taskID, Emit: emit})
	r.harness.BindRecoveryContext(recoveryCtx)
	r.mu.Lock()
	r.boundRoute = recoveryDisplayRoute{TaskID: strings.TrimSpace(taskID), Emit: emit}
	r.mu.Unlock()
	if action.Kind == RuntimeRecoveryAttach {
		return agentruntime.Receipt{
			CommandID: action.CommandID, OperationID: action.OperationID,
			Cursor: status.Cursor, Replayed: true,
		}, nil
	}
	if action.Kind == RuntimeRecoveryAbort {
		receipt, err := r.harness.Submit(recoveryCtx, agentruntime.Abort{
			ID: action.CommandID, OperationID: action.OperationID,
			Reason: "user explicitly aborted a recovery-paused operation",
		})
		return receipt, err
	}
	var delivery agentruntime.DeliveryKind
	switch action.Kind {
	case RuntimeRecoverySteer:
		delivery = agentruntime.DeliverySteer
	case RuntimeRecoveryFollowUp:
		delivery = agentruntime.DeliveryFollowUp
	case RuntimeRecoveryNextTurn:
		delivery = agentruntime.DeliveryNextTurn
	default:
		return agentruntime.Receipt{}, fmt.Errorf("%w: structural action requires the structural recovery seam", ErrRecoveryActionChanged)
	}
	receipt, err := r.harness.RecoverAcceptedInput(recoveryCtx, agentruntime.RecoveryAction{
		Kind: delivery, CommandID: action.CommandID, OperationID: action.OperationID,
	})
	if errors.Is(err, agentruntime.ErrRecoveryActionChanged) {
		return receipt, fmt.Errorf("%w: %v", ErrRecoveryActionChanged, err)
	}
	return receipt, err
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
func (r *RecoveryObservation) Wait(ctx context.Context, emit func(Event)) RunOutcome {
	if r == nil || r.harness == nil {
		err := ErrRuntimeProjectionUnavailable
		return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), "", "")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	initial := r.InitialStatus()
	if initial.Phase == agentruntime.PhaseIdle && len(initial.Queue) == 0 && initial.LastOperation != nil {
		outcome := replayedOutcomeForSettlement(ctx, r.harness, initial.LastOperation.OperationID, agentruntime.OperationSettledEvent{
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
		if status.Phase == agentruntime.PhaseIdle || status.ActiveOperation == "" {
			return
		}
		_, abortErr := r.harness.Submit(context.Background(), agentruntime.Abort{
			ID: cancellationAbortIdentity.forOperation(status.ActiveOperation), OperationID: status.ActiveOperation,
			Reason: "recovery display task canceled",
		})
		if abortErr != nil && !errors.Is(abortErr, agentruntime.ErrInvalidCommand) &&
			!errors.Is(abortErr, agentruntime.ErrStaleOperation) &&
			!errors.Is(abortErr, agentruntime.ErrDomainCommitRejected) {
			log.Printf("[agent-recovery] abort after display cancellation failed operation_id=%s err=%v", status.ActiveOperation, abortErr)
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
			case agentruntime.OperationRecoveryPausedEvent, agentruntime.InputMaterializationRecoveryPendingEvent:
				status, statusErr := r.harness.Status(context.Background())
				if statusErr != nil {
					emitHarnessError(emit, statusErr)
					return outcomeFromOutput(RunOutcomeFailed, statusErr, statusErr.Error(), "", "")
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
			case agentruntime.OperationSettledEvent, agentruntime.OperationInterruptedEvent:
				status, statusErr := r.harness.Status(context.Background())
				if statusErr != nil {
					emitHarnessError(emit, statusErr)
					return outcomeFromOutput(RunOutcomeFailed, statusErr, statusErr.Error(), "", "")
				}
				if status.Phase != agentruntime.PhaseIdle || len(status.Queue) != 0 || status.LastOperation == nil {
					continue
				}
				outcome := replayedOutcomeForSettlement(context.Background(), r.harness, status.LastOperation.OperationID, agentruntime.OperationSettledEvent{
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
				emitHarnessError(emit, observationErr)
				return outcomeFromOutput(RunOutcomeFailed, observationErr, observationErr.Error(), "", "")
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
			emitHarnessError(emit, err)
			return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), "", "")
		}
	}
}

func emitRecoveryTerminal(emit func(Event), outcome RunOutcome) {
	if emit == nil {
		return
	}
	switch outcome.Status {
	case RunOutcomeCompleted:
		emit(Event{Type: "done", Data: map[string]string{}})
	case RunOutcomeAborted:
		emit(Event{Type: "aborted", Data: map[string]string{"reason": outcome.Reason}})
	default:
		message := outcome.Reason
		if message == "" && outcome.Error != nil {
			message = outcome.Error.Error()
		}
		emit(Event{Type: "error", Data: map[string]string{"message": message}})
	}
}
