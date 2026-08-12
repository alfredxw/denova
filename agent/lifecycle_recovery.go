package agent

import (
	"context"
	"errors"
	"strings"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

type recoveryCandidate struct {
	action     RecoveryAction
	commandID  runstate.CommandID
	operation  runstate.OperationID
	delivery   runstate.DeliveryKind
	structural *runstate.StructuralOperationSnapshot
}

// RecoveryActions returns only actions that are valid for the Session's
// current durable state. IDs are invalidated by any competing recovery choice.
func (session *Session) RecoveryActions(ctx context.Context) ([]RecoveryAction, error) {
	if err := session.usable(); err != nil {
		return nil, err
	}
	status, err := session.harness.Status(ctx)
	if err != nil {
		return nil, mapRuntimeError(err)
	}
	return publicRecoveryActions(recoveryCandidatesFromStatus(status)), nil
}

// Recover applies one opaque action returned by RecoveryActions or Snapshot.
// The selected action is re-derived inside the actor boundary; stale IDs never
// authorize a different queued input or structural operation.
func (session *Session) Recover(ctx context.Context, actionID string) error {
	if err := session.usable(); err != nil {
		return err
	}
	actionID = strings.TrimSpace(actionID)
	if actionID == "" {
		return ErrRecoveryStale
	}
	status, err := session.harness.Status(ctx)
	if err != nil {
		return mapRuntimeError(err)
	}
	var selected *recoveryCandidate
	for _, candidate := range recoveryCandidatesFromStatus(status) {
		if candidate.action.ID == actionID {
			copy := candidate
			selected = &copy
			break
		}
	}
	if selected == nil {
		return ErrRecoveryStale
	}
	switch selected.action.Kind {
	case RecoveryResumeInput:
		_, err = session.harness.RecoverAcceptedInput(ctx, runstate.RecoveryAction{
			Kind: selected.delivery, CommandID: selected.commandID, OperationID: selected.operation,
		})
	case RecoveryResumeCompaction:
		if selected.structural == nil {
			return ErrRecoveryStale
		}
		structural := *selected.structural
		switch structural.Kind {
		case runstate.StructuralCompactContext:
			_, err = session.harness.Submit(ctx, runstate.CompactIfNeeded{ID: structural.CommandID, Ref: structural.Ref})
		case runstate.StructuralRemoveCompaction:
			_, err = session.harness.Submit(ctx, runstate.RemoveCompaction{ID: structural.CommandID, Ref: structural.Ref})
		default:
			return ErrRecoveryStale
		}
	case RecoveryAbortRun:
		_, err = session.harness.Submit(ctx, runstate.Abort{
			ID: runstate.CommandID("recover-" + actionID), OperationID: selected.operation,
			Reason: "user selected the durable recovery abort action",
		})
	default:
		return ErrRecoveryStale
	}
	if err != nil {
		if errors.Is(err, runstate.ErrInvalidCommand) || errors.Is(err, runstate.ErrRecoveryActionChanged) {
			return errors.Join(ErrRecoveryStale, err)
		}
		return mapRuntimeError(err)
	}
	return nil
}

func recoveryCandidatesFromStatus(status runstate.StatusSnapshot) []recoveryCandidate {
	return buildRecoveryCandidates(
		status.Phase, status.RecoveryPaused, status.ActiveOperation, status.ActiveCycle,
		status.InputRecovery, status.ActiveStructural, status.Queue,
	)
}

func recoveryCandidatesFromState(state runstate.StateSnapshot) []recoveryCandidate {
	return buildRecoveryCandidates(
		state.Phase, state.RecoveryPaused, state.ActiveOperation, state.ActiveCycle,
		state.InputRecovery, state.ActiveStructural, state.Queue,
	)
}

func buildRecoveryCandidates(
	phase runstate.Phase,
	paused bool,
	active runstate.OperationID,
	activeCycle int,
	input *runstate.InputMaterializationRecovery,
	structural *runstate.StructuralOperationSnapshot,
	queue []runstate.QueuedInput,
) []recoveryCandidate {
	candidates := make([]recoveryCandidate, 0, 3)
	appendCandidate := func(kind RecoveryActionKind, command runstate.CommandID, operation runstate.OperationID, cycle int, delivery runstate.DeliveryKind, structural *runstate.StructuralOperationSnapshot) {
		if operation == "" {
			return
		}
		id, err := hashCanonical(struct {
			Version   uint16
			Kind      RecoveryActionKind
			Command   runstate.CommandID
			Operation runstate.OperationID
		}{1, kind, command, operation})
		if err != nil {
			return
		}
		candidate := recoveryCandidate{
			action: RecoveryAction{
				ID: id[:32], Kind: kind, RunID: string(operation), CommandID: string(command), Cycle: cycle,
				Delivery:   publicRecoveryDelivery(delivery),
				Compaction: publicRecoveryCompaction(structural),
			},
			commandID: command, operation: operation, delivery: delivery,
		}
		if structural != nil {
			copy := *structural
			candidate.structural = &copy
		}
		candidates = append(candidates, candidate)
	}
	if paused && phase == runstate.PhaseCompacting && structural != nil {
		appendCandidate(RecoveryResumeCompaction, structural.CommandID, structural.OperationID, structural.Cycle, "", structural)
		return candidates
	}
	if paused && phase == runstate.PhaseRunning && active != "" {
		appendCandidate(RecoveryAbortRun, "", active, activeCycle, "", nil)
	}
	if input != nil && !input.Autonomous {
		appendCandidate(RecoveryResumeInput, input.CommandID, input.OperationID, input.Cycle, input.Delivery, nil)
		return candidates
	}
	eligible := func(delivery runstate.DeliveryKind) bool {
		return paused && phase == runstate.PhaseRunning || phase == runstate.PhaseIdle && delivery == runstate.DeliveryNextTurn
	}
	for _, delivery := range []runstate.DeliveryKind{runstate.DeliverySteer, runstate.DeliveryFollowUp, runstate.DeliveryNextTurn} {
		if !eligible(delivery) {
			continue
		}
		for _, queued := range queue {
			if queued.Autonomous || queued.Delivery != delivery {
				continue
			}
			cycle := 1
			if queued.OperationID == active && queued.Delivery != runstate.DeliveryNextTurn {
				cycle = activeCycle + 1
			}
			appendCandidate(RecoveryResumeInput, queued.CommandID, queued.OperationID, cycle, queued.Delivery, nil)
			return candidates
		}
	}
	return candidates
}

func publicRecoveryCompaction(structural *runstate.StructuralOperationSnapshot) RecoveryCompactionAction {
	if structural == nil {
		return ""
	}
	switch structural.Kind {
	case runstate.StructuralCompactContext:
		return RecoveryCompactionCreate
	case runstate.StructuralRemoveCompaction:
		return RecoveryCompactionRemove
	default:
		return ""
	}
}

func publicRecoveryDelivery(delivery runstate.DeliveryKind) RecoveryInputDelivery {
	switch delivery {
	case runstate.DeliverySteer:
		return RecoveryDeliverySteer
	case runstate.DeliveryFollowUp:
		return RecoveryDeliveryFollowUp
	case runstate.DeliveryNextTurn:
		return RecoveryDeliveryNextTurn
	default:
		return ""
	}
}

func publicRecoveryActions(candidates []recoveryCandidate) []RecoveryAction {
	if len(candidates) == 0 {
		return nil
	}
	result := make([]RecoveryAction, len(candidates))
	for index, candidate := range candidates {
		result[index] = candidate.action
	}
	return result
}
