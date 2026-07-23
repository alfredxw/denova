package agent

import (
	"context"
	"errors"
	"fmt"

	runstate "denova/internal/agent/runtime"
)

const (
	RuntimeRecoveryRequiredEventType = "runtime_recovery_required"
	RuntimeRecoveryRequiredEventCode = "agent_runtime.recovery_required"
)

// followAcceptedSuccessors keeps the display Task attached when the operation
// it originally accepted atomically settles and starts an accepted NextTurn.
// The durable binding is authoritative: a successor may itself select another
// successor, pause for exact recovery, or receive Task.Abort.
func (h *chatHarness) followAcceptedSuccessors(
	caller context.Context,
	harness *runstate.Harness,
	observation runstate.Observation,
	settledOperation runstate.OperationID,
	initial RunOutcome,
	emit func(Event),
) (RunOutcome, bool) {
	if h == nil || harness == nil {
		return initial, false
	}
	if caller == nil {
		caller = context.Background()
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		emitHarnessError(emit, err)
		return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), initial.Content, initial.Thinking), false
	}
	current, followed, terminal := acceptedSuccessorFromStatus(status, settledOperation)
	if terminal != nil {
		return replayedOutcomeForSettlement(context.Background(), harness, terminal.OperationID, runstate.OperationSettledEvent{
			OperationID: terminal.OperationID, Status: terminal.Status, Reason: terminal.Reason,
		}), true
	}
	if !followed {
		return initial, false
	}

	events := observation.Events
	errorsCh := observation.Errors
	callerDone := caller.Done()
	abortSentFor := runstate.OperationID("")
	for {
		if status.RecoveryPaused {
			if caller.Err() == nil {
				err := ErrRecoveryRequired
				emitRuntimeRecoveryRequired(emit, status)
				return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), initial.Content, initial.Thinking), true
			}
			callerDone = nil
		}
		if caller.Err() != nil && abortSentFor != current {
			abortSentFor = current
			_, abortErr := harness.Submit(context.Background(), runstate.Abort{
				ID: runstate.CommandID(newHarnessIdentity("successor-abort")), OperationID: current,
				Reason: "agent display task canceled while following accepted NextTurn",
			})
			if abortErr != nil && !errors.Is(abortErr, runstate.ErrInvalidCommand) &&
				!errors.Is(abortErr, runstate.ErrStaleOperation) &&
				!errors.Is(abortErr, runstate.ErrDomainCommitRejected) {
				emitHarnessError(emit, abortErr)
				return outcomeFromOutput(RunOutcomeFailed, abortErr, abortErr.Error(), initial.Content, initial.Thinking), true
			}
		}

		select {
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			switch payload := event.Payload.(type) {
			case runstate.OperationRecoveryPausedEvent, runstate.InputMaterializationRecoveryPendingEvent:
				status, err = harness.Status(context.Background())
				if err != nil {
					emitHarnessError(emit, err)
					return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), initial.Content, initial.Thinking), true
				}
			case runstate.OperationSettledEvent:
				if payload.OperationID != current {
					continue
				}
				outcome := replayedOutcomeForSettlement(context.Background(), harness, current, payload)
				status, err = harness.Status(context.Background())
				if err != nil {
					emitHarnessError(emit, err)
					return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), outcome.Content, outcome.Thinking), true
				}
				next, hasNext, _ := acceptedSuccessorFromStatus(status, current)
				if !hasNext {
					return outcome, true
				}
				current = next
			case runstate.OperationInterruptedEvent:
				if payload.OperationID != current {
					continue
				}
				status, err = harness.Status(context.Background())
				if err != nil {
					emitHarnessError(emit, err)
					return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), initial.Content, initial.Thinking), true
				}
				next, hasNext, _ := acceptedSuccessorFromStatus(status, current)
				if hasNext {
					current = next
					continue
				}
				reason := payload.Reason
				if reason == "" {
					reason = "accepted NextTurn operation was interrupted"
				}
				return outcomeFromOutput(RunOutcomeFailed, errors.New(reason), reason, initial.Content, initial.Thinking), true
			}
		case observationErr, ok := <-errorsCh:
			if !ok {
				errorsCh = nil
				continue
			}
			if observationErr != nil {
				emitHarnessError(emit, observationErr)
				return outcomeFromOutput(RunOutcomeFailed, observationErr, observationErr.Error(), initial.Content, initial.Thinking), true
			}
		case <-callerDone:
			callerDone = nil
		case <-h.lifecycle.Done():
			err := h.lifecycle.Err()
			if err == nil {
				err = runstate.ErrRuntimeClosed
			}
			return outcomeFromOutput(RunOutcomeAborted, err, err.Error(), initial.Content, initial.Thinking), true
		}
		if events == nil && errorsCh == nil {
			err := fmt.Errorf("agent observation closed while following accepted NextTurn operation %s", current)
			emitHarnessError(emit, err)
			return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), initial.Content, initial.Thinking), true
		}
	}
}

func acceptedSuccessorFromStatus(
	status runstate.StatusSnapshot,
	settledOperation runstate.OperationID,
) (runstate.OperationID, bool, *runstate.OperationSummary) {
	if status.ActiveOperation != "" && status.ActiveOperation != settledOperation {
		return status.ActiveOperation, true, nil
	}
	if status.Phase == runstate.PhaseIdle && status.LastOperation != nil && status.LastOperation.OperationID != settledOperation {
		for _, summary := range status.RecentOperations {
			if summary.OperationID == settledOperation {
				terminal := *status.LastOperation
				return "", true, &terminal
			}
		}
	}
	return "", false, nil
}

func emitRuntimeRecoveryRequired(emit func(Event), status runstate.StatusSnapshot) {
	if emit == nil {
		return
	}
	emit(Event{Type: RuntimeRecoveryRequiredEventType, Data: map[string]any{
		"code":         RuntimeRecoveryRequiredEventCode,
		"message":      "运行再次停在恢复边界，请刷新活动状态后继续 / The run reached another recovery boundary; refresh the active projection to continue",
		"operation_id": status.ActiveOperation,
		"cursor":       status.Cursor,
	}})
}
