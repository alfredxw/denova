package runtime

import (
	"context"
	"fmt"
)

// pauseRecoveredOperation records that recovery deliberately did not rerun an
// unfinished execution attempt. A Running operation remains actor-owned so an
// explicit fresh transient, Abort, or NextTurn can decide its future without
// replaying the interrupted StartTurn. Structural work still requires exact
// command replay. Open tool effects are closed as unknown before the pause
// becomes visible.
func (h *Harness) pauseRecoveredOperation(ctx context.Context, state *harnessState) error {
	payloads := h.uncertainToolResults(state)
	if !state.recoveryPaused {
		reason := "runtime recovered an unfinished cycle; no model or tool effect was retried"
		if state.phase == PhaseCompacting {
			reason = "runtime recovered an unfinished structural operation; exact command replay may resume after canonical reconciliation"
		} else if state.hasQueued(DeliverySteer) || state.hasQueued(DeliveryFollowUp) {
			reason += "; accepted steer/follow-up input remains queued for exact command replay"
		} else {
			reason += "; attach a display task, then submit a fresh steer, follow-up, abort, or next-turn decision"
		}
		payloads = append(payloads, OperationRecoveryPausedEvent{
			OperationID: state.activeOperation,
			Cycle:       state.activeCycle,
			Reason:      reason,
		})
	}
	if len(payloads) == 0 {
		return nil
	}
	if _, err := h.commit(ctx, state, payloads); err != nil {
		return fmt.Errorf("pause recovered operation %s: %w", state.activeOperation, err)
	}
	return nil
}

func (h *Harness) resumeRecoveryPausedCommand(state *harnessState, command Command) error {
	if !state.recoveryPaused || state.engineControls != nil {
		return nil
	}
	if recovery := state.inputRecovery; recovery != nil && recovery.CommandID == command.commandID() {
		return h.resumePendingInputMaterialization(h.lifecycle, state)
	}
	switch command := command.(type) {
	case CompactIfNeeded:
		if state.phase != PhaseCompacting || state.activeStructural == nil ||
			state.activeStructural.Kind != StructuralCompactContext || state.activeCommandID != command.ID {
			return nil
		}
		if err := h.restoreStructuralOperation(*state.activeStructural); err != nil {
			return err
		}
		h.startStructuralEngine(state, *state.activeStructural)
		return nil
	case RemoveCompaction:
		if state.phase != PhaseCompacting || state.activeStructural == nil ||
			state.activeStructural.Kind != StructuralRemoveCompaction || state.activeCommandID != command.ID {
			return nil
		}
		if err := h.restoreStructuralOperation(*state.activeStructural); err != nil {
			return err
		}
		h.startStructuralEngine(state, *state.activeStructural)
		return nil
	case Steer:
		return h.resumeRecoveryPausedInput(h.lifecycle, state, command.ID, DeliverySteer)
	case FollowUp:
		return h.resumeRecoveryPausedInput(h.lifecycle, state, command.ID, DeliveryFollowUp)
	case NextTurn:
		item, ok := pendingRecoveryInput(state, RecoveryAction{
			Kind: DeliveryNextTurn, CommandID: command.ID, OperationID: command.AfterOperationID,
		})
		if !ok {
			// A queued NextTurn owns its successor OperationID, while the command
			// envelope names the uncertain parent in AfterOperationID.
			for _, queued := range state.queue {
				if queued.Delivery == DeliveryNextTurn && queued.CommandID == command.ID {
					item, ok = cloneQueuedInput(queued), true
					break
				}
			}
		}
		if ok {
			return h.abandonRecoveredParentForNextTurn(h.lifecycle, state, item)
		}
		return nil
	case Abort:
		if command.OperationID != state.activeOperation || !state.abortRequested || state.engineControls != nil {
			return nil
		}
		if err := h.ensureInputMaterialized(h.lifecycle, state); err != nil {
			return err
		}
		h.failActiveOperation(state, engineDoneRequest{
			operation: state.activeOperation,
			cycle:     state.activeCycle,
			result:    EngineResult{Status: EngineAborted},
		})
		return nil
	default:
		// Replaying the original StartTurn must never rerun the interrupted
		// cycle merely because a later accepted command is waiting in its queue.
		return nil
	}
}

func (h *Harness) restoreStructuralOperation(snapshot StructuralOperationSnapshot) (resultErr error) {
	restorer, ok := h.engine.(EngineStructuralOperationRestorer)
	if !ok {
		return nil
	}
	snapshot = *cloneStructuralOperationSnapshot(&snapshot)
	defer func() {
		if recovered := recover(); recovered != nil {
			resultErr = fmt.Errorf("structural operation restorer panic: %v", recovered)
		}
	}()
	return restorer.RestoreStructuralOperation(h.lifecycle, snapshot)
}

func (h *Harness) resumeRecoveryPausedInput(ctx context.Context, state *harnessState, commandID CommandID, delivery DeliveryKind) error {
	if state.phase != PhaseRunning {
		return nil
	}
	var item QueuedInput
	found := false
	for _, queued := range state.queue {
		if queued.CommandID == commandID && queued.Delivery == delivery {
			item = cloneQueuedInput(queued)
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	nextCycle := state.activeCycle + 1
	snapshotID := SnapshotID(newID("snapshot"))
	if _, err := h.commit(context.Background(), state, []EventPayload{
		SavePointCommittedEvent{OperationID: state.activeOperation, Cycle: state.activeCycle},
		QueueConsumedEvent{CommandID: item.CommandID, Delivery: item.Delivery},
		UserMessageCommittedEvent{Message: newUserMessage(state.activeOperation, item.Input)},
		CycleStartedEvent{OperationID: state.activeOperation, Cycle: nextCycle, SnapshotID: snapshotID},
		InputMaterializationRecoveryPendingEvent{
			OperationID: state.activeOperation, Cycle: nextCycle,
			CommandID: item.CommandID, Delivery: item.Delivery,
		},
	}); err != nil {
		return err
	}
	return h.resumePendingInputMaterialization(ctx, state)
}

func (h *Harness) resumePendingInputMaterialization(ctx context.Context, state *harnessState) error {
	if state == nil || state.inputRecovery == nil || state.phase != PhaseRunning || !state.recoveryPaused {
		return fmt.Errorf("%w: accepted input is not awaiting materialization", ErrRecoveryActionChanged)
	}
	recovery := *state.inputRecovery
	item := QueuedInput{
		CommandID: recovery.CommandID, OperationID: recovery.OperationID,
		Delivery: recovery.Delivery, Input: cloneUserInput(state.activeInput),
	}
	if err := h.restorePendingInput(ctx, item); err != nil {
		return err
	}
	if err := h.ensureInputMaterialized(ctx, state); err != nil {
		return err
	}
	if _, err := h.commit(context.Background(), state, []EventPayload{
		InputMaterializationRecoveryResumedEvent{OperationID: recovery.OperationID, Cycle: recovery.Cycle},
	}); err != nil {
		return err
	}
	h.startEngine(state, state.turnSnapshot(state.activeSnapshotID))
	return nil
}

// abandonRecoveredParentForNextTurn makes the crash boundary explicit: the
// uncertain parent is never rerun, while its already accepted successor starts
// from the durable NextTurn input in the same actor transition.
func (h *Harness) abandonRecoveredParentForNextTurn(ctx context.Context, state *harnessState, item QueuedInput) error {
	if state == nil || state.phase != PhaseRunning || !state.recoveryPaused || item.Delivery != DeliveryNextTurn {
		return fmt.Errorf("%w: NextTurn parent is not recovery-paused", ErrRecoveryActionChanged)
	}
	plan := nextTurnPlan{item: cloneQueuedInput(item), has: true, start: true, snapshotID: SnapshotID(newID("snapshot"))}
	payloads := h.uncertainToolResults(state)
	payloads = append(payloads, transientQueueCancellations(state, "runtime_recovered")...)
	payloads = append(payloads, OperationInterruptedEvent{
		OperationID: state.activeOperation,
		Reason:      "runtime recovery abandoned an uncertain parent before its accepted NextTurn successor",
	})
	payloads = plan.appendStart(payloads)
	if _, err := h.commit(context.Background(), state, payloads); err != nil {
		return err
	}
	return h.startPlannedNextTurn(state, plan)
}
