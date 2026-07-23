package agentruntime

import (
	"context"
	"fmt"
	"log"
)

func (h *Harness) handleSubmit(state *harnessState, ctx context.Context, command Command) (Receipt, error) {
	if err := validateCommandEnvelope(command, h.inputLimits); err != nil {
		return Receipt{}, err
	}
	// Structural restore descriptors are opaque but potentially large. Bound and
	// validate them before fingerprinting so rejected transport input is never
	// formatted or hashed as durable identity.
	switch typed := command.(type) {
	case CompactIfNeeded:
		if err := validateContextCompactionRef(typed.Ref, h.inputLimits, false); err != nil {
			return Receipt{}, err
		}
	case RemoveCompaction:
		if err := validateContextCompactionRef(typed.Ref, h.inputLimits, true); err != nil {
			return Receipt{}, err
		}
	}
	fingerprint, err := CommandFingerprint(command)
	if err != nil {
		return Receipt{}, err
	}
	// An unacknowledged host effect is a strict durable head-of-line fence.
	// Even an exact command retry can resume accepted work, so resolve the
	// outbox before consulting hot or cold idempotency records.
	if len(state.pendingHostEffects) > 0 {
		return Receipt{}, fmt.Errorf("%w: reconcile %d pending effect(s) before accepting or resuming a command", ErrHostEffectRequired, len(state.pendingHostEffects))
	}
	commandID := command.commandID()
	if receipt, ok := state.receipts[commandID]; ok {
		if state.fingerprints[commandID] != fingerprint {
			return Receipt{}, fmt.Errorf("%w: command id %q was already used", ErrInvalidCommand, commandID)
		}
		if err := h.resumeReplayedCommand(state, command); err != nil {
			return Receipt{}, err
		}
		receipt.Replayed = true
		return receipt, nil
	}
	if lookup, ok := h.journal.(CommandJournalLookup); ok {
		record, found, err := lookup.LookupCommand(ctx, commandID)
		if err != nil {
			return Receipt{}, fmt.Errorf("lookup durable agent command %q: %w", commandID, err)
		}
		if found {
			if record.Fingerprint != fingerprint {
				return Receipt{}, fmt.Errorf("%w: command id %q was already used", ErrInvalidCommand, commandID)
			}
			if err := h.resumeReplayedCommand(state, command); err != nil {
				return Receipt{}, err
			}
			record.Receipt.Replayed = true
			return record.Receipt, nil
		}
	}
	switch command := command.(type) {
	case StartTurn:
		if err := validateUserInput(command.Input, h.inputLimits); err != nil {
			return Receipt{}, err
		}
		if state.phase != PhaseIdle || state.hasQueued(DeliveryNextTurn) {
			return Receipt{}, ErrBusy
		}
		return h.beginOperation(ctx, state, commandID, "start_turn", fingerprint, OperationID(newID("operation")), command.Input)
	case Steer:
		return h.enqueueCurrent(ctx, state, commandID, fingerprint, command.OperationID, DeliverySteer, command.Input)
	case FollowUp:
		return h.enqueueCurrent(ctx, state, commandID, fingerprint, command.OperationID, DeliveryFollowUp, command.Input)
	case NextTurn:
		if err := validateUserInput(command.Input, h.inputLimits); err != nil {
			return Receipt{}, err
		}
		if command.AfterOperationID != state.activeOperation {
			return Receipt{}, ErrStaleOperation
		}
		operationID := OperationID(newID("operation"))
		if state.phase == PhaseIdle {
			return h.beginOperation(ctx, state, commandID, "next_turn", fingerprint, operationID, command.Input)
		}
		if state.hasQueued(DeliveryNextTurn) {
			return Receipt{}, ErrQueueConflict
		}
		if err := state.admitPendingInput(command.Input); err != nil {
			return Receipt{}, err
		}
		item := QueuedInput{
			CommandID: commandID, OperationID: operationID,
			Delivery: DeliveryNextTurn, Input: cloneUserInput(command.Input),
		}
		committed, err := h.commit(ctx, state, []EventPayload{
			CommandAcceptedEvent{CommandID: commandID, CommandKind: "next_turn", OperationID: operationID, Fingerprint: fingerprint},
			QueueEnqueuedEvent{Item: item},
		})
		if err != nil {
			return Receipt{}, err
		}
		receipt := receiptFromEvents(committed)
		if state.recoveryPaused && state.phase == PhaseRunning {
			if err := h.abandonRecoveredParentForNextTurn(ctx, state, item); err != nil {
				return receipt, err
			}
		}
		return receipt, nil
	case Abort:
		if state.phase == PhaseIdle {
			return Receipt{}, ErrInvalidCommand
		}
		if command.OperationID == "" || command.OperationID != state.activeOperation {
			return Receipt{}, ErrStaleOperation
		}
		if state.outputCommitFinalizing() {
			return Receipt{}, fmt.Errorf("%w: domain commit is already authorized", ErrDomainCommitRejected)
		}
		committed, err := h.commit(ctx, state, []EventPayload{
			CommandAcceptedEvent{CommandID: commandID, CommandKind: "abort", OperationID: state.activeOperation, Fingerprint: fingerprint},
			AbortRequestedEvent{OperationID: state.activeOperation, Reason: command.Reason},
		})
		if err != nil {
			return Receipt{}, err
		}
		state.sendControl(EngineControl{Kind: EngineControlAbort})
		receipt := receiptFromEvents(committed)
		if state.engineControls == nil {
			if err := h.ensureInputMaterialized(h.lifecycle, state); err != nil {
				return receipt, err
			}
			h.failActiveOperation(state, engineDoneRequest{
				operation: state.activeOperation,
				cycle:     state.activeCycle,
				result:    EngineResult{Status: EngineAborted},
			})
		}
		return receipt, nil
	case CompactIfNeeded:
		if state.phase != PhaseIdle {
			return Receipt{}, ErrBusy
		}
		return h.beginStructuralOperation(ctx, state, commandID, "compact_context", fingerprint, StructuralCompactContext, command.Ref)
	case RemoveCompaction:
		if state.phase != PhaseIdle {
			return Receipt{}, ErrBusy
		}
		return h.beginStructuralOperation(ctx, state, commandID, "remove_compaction", fingerprint, StructuralRemoveCompaction, command.Ref)
	default:
		return Receipt{}, ErrInvalidCommand
	}
}

func (h *Harness) beginStructuralOperation(
	ctx context.Context,
	state *harnessState,
	commandID CommandID,
	commandKind string,
	fingerprint string,
	kind StructuralOperationKind,
	ref ContextCompactionRef,
) (Receipt, error) {
	if _, ok := h.engine.(StructuralEngine); !ok {
		return Receipt{}, fmt.Errorf("%w: binding engine does not support structural context operations", ErrInvalidCommand)
	}
	if err := state.admitStructuralRef(ref); err != nil {
		return Receipt{}, err
	}
	operationID := OperationID(newID("operation"))
	snapshot := StructuralOperationSnapshot{
		Binding: h.binding, CommandID: commandID, OperationID: operationID,
		Cycle: 1, Kind: kind, Ref: cloneContextCompactionRef(ref), ContextCursor: state.cursor + 2,
	}
	committed, err := h.commit(ctx, state, []EventPayload{
		CommandAcceptedEvent{CommandID: commandID, CommandKind: commandKind, OperationID: operationID, Fingerprint: fingerprint},
		OperationStartedEvent{OperationID: operationID, Phase: PhaseCompacting, Structural: &snapshot},
	})
	if err != nil {
		return Receipt{}, err
	}
	h.startStructuralEngine(state, *state.activeStructural)
	return receiptFromEvents(committed), nil
}

func (h *Harness) beginOperation(
	ctx context.Context,
	state *harnessState,
	commandID CommandID,
	commandKind string,
	fingerprint string,
	operationID OperationID,
	input UserInput,
) (Receipt, error) {
	if err := state.admitPendingInput(input); err != nil {
		return Receipt{}, err
	}
	snapshotID := SnapshotID(newID("snapshot"))
	message := newUserMessage(operationID, input)
	committed, err := h.commit(ctx, state, []EventPayload{
		CommandAcceptedEvent{CommandID: commandID, CommandKind: commandKind, OperationID: operationID, Fingerprint: fingerprint},
		OperationStartedEvent{OperationID: operationID},
		UserMessageCommittedEvent{Message: message},
		CycleStartedEvent{OperationID: operationID, Cycle: 1, SnapshotID: snapshotID},
	})
	if err != nil {
		return Receipt{}, err
	}
	receipt := receiptFromEvents(committed)
	if err := h.ensureInputMaterialized(h.lifecycle, state); err != nil {
		return receipt, err
	}
	h.startEngine(state, state.turnSnapshot(snapshotID))
	return receipt, nil
}

func (h *Harness) enqueueCurrent(
	ctx context.Context,
	state *harnessState,
	commandID CommandID,
	fingerprint string,
	operationID OperationID,
	delivery DeliveryKind,
	input UserInput,
) (Receipt, error) {
	if err := validateUserInput(input, h.inputLimits); err != nil {
		return Receipt{}, err
	}
	if state.phase != PhaseRunning {
		return Receipt{}, ErrInvalidCommand
	}
	if operationID == "" || operationID != state.activeOperation {
		return Receipt{}, ErrStaleOperation
	}
	if delivery == DeliverySteer && state.outputCommitFinalizing() {
		return Receipt{}, fmt.Errorf("%w: domain commit is already authorized", ErrDomainCommitRejected)
	}
	if state.hasQueued(delivery) {
		return Receipt{}, ErrQueueConflict
	}
	if err := state.admitPendingInput(input); err != nil {
		return Receipt{}, err
	}
	item := QueuedInput{
		CommandID: commandID, OperationID: state.activeOperation,
		Delivery: delivery, Input: cloneUserInput(input),
	}
	committed, err := h.commit(ctx, state, []EventPayload{
		CommandAcceptedEvent{CommandID: commandID, CommandKind: string(delivery), OperationID: state.activeOperation, Fingerprint: fingerprint},
		QueueEnqueuedEvent{Item: item},
	})
	if err != nil {
		return Receipt{}, err
	}
	if delivery == DeliverySteer {
		state.sendControl(EngineControl{Kind: EngineControlPreempt})
	}
	if state.recoveryPaused {
		if err := h.resumeRecoveryPausedInput(ctx, state, commandID, delivery); err != nil {
			return receiptFromEvents(committed), err
		}
	}
	return receiptFromEvents(committed), nil
}

func (h *Harness) commit(ctx context.Context, state *harnessState, payloads []EventPayload) ([]Event, error) {
	committed, err := h.journal.Append(ctx, state.cursor, payloads)
	if err != nil {
		return nil, &journalAppendError{err: err}
	}
	pendingReleases := make([]UserInput, 0)
	for _, event := range committed {
		if cancelled, ok := event.Payload.(QueueCancelledEvent); ok {
			for _, item := range state.queue {
				if item.CommandID == cancelled.CommandID && item.Input.TurnSpecRef != "" {
					pendingReleases = append(pendingReleases, cloneUserInput(item.Input))
					break
				}
			}
		}
		if err := state.reduce(event); err != nil {
			panic(fmt.Sprintf("journal committed an unreducible event at cursor %d: %v", event.Cursor, err))
		}
	}
	if checkpointer, ok := h.journal.(harnessStateCheckpointJournal); ok {
		if err := checkpointer.MaybeCheckpoint(context.WithoutCancel(ctx), state); err != nil {
			// The canonical append is already durable. A failed generation switch
			// leaves the old manifest/tail authoritative and is retried at a later
			// safe transaction boundary; it must not become a false command error.
			log.Printf("agentruntime: binding=%+v cursor=%d checkpoint deferred: %v", h.binding, state.cursor, err)
		}
	}
	if releaser, ok := h.engine.(EnginePendingInputReleaser); ok {
		releaseCtx := context.WithoutCancel(ctx)
		for _, input := range pendingReleases {
			releaser.ReleasePendingInput(releaseCtx, input)
		}
	}
	for _, event := range committed {
		state.publish(displayEventForRetention(event))
	}
	return committed, nil
}

func receiptFromEvents(events []Event) Receipt {
	for _, event := range events {
		if accepted, ok := event.Payload.(CommandAcceptedEvent); ok {
			return Receipt{CommandID: accepted.CommandID, OperationID: accepted.OperationID, Cursor: event.Cursor}
		}
	}
	return Receipt{}
}
