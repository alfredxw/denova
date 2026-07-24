package runtime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
)

func (h *Harness) startEngine(state *harnessState, snapshot TurnSnapshot) {
	controls := make(chan EngineControl, 8)
	state.engineControls = controls
	request := EngineRequest{Binding: h.binding.Clone(), Snapshot: snapshot, Controls: controls}
	go func() {
		returned := false
		defer func() {
			if recovered := recover(); recovered != nil {
				h.postEngineDone(engineDoneRequest{
					operation: snapshot.OperationID,
					cycle:     snapshot.Cycle,
					err:       fmt.Errorf("agent engine panic: %v", recovered),
				})
				return
			}
			if !returned {
				// runtime.Goexit is used by crash simulations and behaves like a
				// vanished worker: durable recovery, not this goroutine, resolves it.
				log.Printf("runtime: binding=%+v operation=%s cycle=%d engine exited without a result", h.binding, snapshot.OperationID, snapshot.Cycle)
			}
		}()
		result, err := h.engine.Run(h.lifecycle, request, func(event EngineEvent) error {
			response := make(chan error, 1)
			engineRequest := engineEventRequest{
				operation: snapshot.OperationID,
				cycle:     snapshot.Cycle,
				event:     event,
				response:  response,
			}
			select {
			case h.requests <- engineRequest:
			case <-h.done:
				return h.terminalError()
			}
			select {
			case err := <-response:
				return err
			case <-h.done:
				return h.terminalError()
			}
		})
		returned = true
		h.postEngineDone(engineDoneRequest{
			operation: snapshot.OperationID,
			cycle:     snapshot.Cycle,
			result:    result,
			err:       err,
		})
	}()
}

func (h *Harness) postEngineDone(request engineDoneRequest) {
	select {
	case h.requests <- request:
	case <-h.done:
	}
}

func (h *Harness) handleEngineEvent(state *harnessState, request engineEventRequest) error {
	if state.phase == PhaseIdle || request.operation != state.activeOperation || request.cycle != state.activeCycle {
		return fmt.Errorf("stale engine event for operation %q cycle %d", request.operation, request.cycle)
	}
	switch event := request.event.(type) {
	case EngineDomainCommitIntent:
		return h.authorizeDomainCommit(state, event)
	case EngineDomainCommitReceipt:
		return h.recordDomainCommitReceipt(state, event)
	case EngineHostEffectAcknowledged:
		effect, pending := state.pendingHostEffects[event.ID]
		if !pending {
			return fmt.Errorf("acknowledge unknown host effect %q", event.ID)
		}
		if !hostEffectHasExactOutputReceipt(state, effect) {
			return fmt.Errorf("%w: host effect %q cannot be acknowledged before its exact output-domain commit receipt", ErrHostEffectRequired, event.ID)
		}
		_, err := h.commit(context.Background(), state, []EventPayload{HostEffectAcknowledgedEvent{ID: event.ID}})
		return err
	}
	if state.phase == PhaseCompacting {
		return fmt.Errorf("unsupported structural engine event %T", request.event)
	}
	if state.abortRequested || state.closing {
		// A known completion for a tool that was already running still improves
		// recovery accuracy. Every other late event is rejected so accepted abort
		// cannot commit new assistant output or begin another side effect.
		if _, toolFinished := request.event.(EngineToolFinished); !toolFinished {
			return fmt.Errorf("engine event %T rejected after operation abort", request.event)
		}
	}
	switch event := request.event.(type) {
	case EngineAssistantDelta:
		if err := state.admitActiveBytes(ByteBudgetActiveOutput, len(event.Delta)); err != nil {
			h.publishByteBudgetExceeded(state, err)
			return err
		}
		state.activeContent.WriteString(event.Delta)
		state.publish(Event{
			Cursor: state.cursor, Durability: EventEphemeral,
			Payload: AssistantDeltaEvent{OperationID: state.activeOperation, Delta: event.Delta},
		})
		return nil
	case EngineThinkingDelta:
		if err := state.admitActiveBytes(ByteBudgetActiveOutput, len(event.Delta)); err != nil {
			h.publishByteBudgetExceeded(state, err)
			return err
		}
		state.activeThinking.WriteString(event.Delta)
		state.publish(Event{
			Cursor: state.cursor, Durability: EventEphemeral,
			Payload: ThinkingDeltaEvent{OperationID: state.activeOperation, Delta: event.Delta},
		})
		return nil
	case EngineAssistantFinal:
		if err := state.admitFinalOutput(event.Content, event.Thinking); err != nil {
			h.publishByteBudgetExceeded(state, err)
			return err
		}
		if output := state.domainCommit(DomainCommitOutput); output != nil && output.Revision == "" {
			return fmt.Errorf("%w: assistant final arrived before domain commit receipt", ErrDomainCommitRejected)
		}
		state.activeContent.Reset()
		state.activeContent.WriteString(event.Content)
		state.activeThinking.Reset()
		state.activeThinking.WriteString(event.Thinking)
		state.activeOutputRehydrated = false
		_, err := h.commit(context.Background(), state, []EventPayload{
			AssistantMessageCommittedEvent{Message: Message{
				ID: newID("message"), Role: RoleAssistant,
				Content: event.Content, Thinking: event.Thinking,
				Operation: state.activeOperation,
			}},
		})
		return err
	case EngineToolStarted:
		if strings.TrimSpace(event.CallID) == "" || strings.TrimSpace(event.Name) == "" {
			return fmt.Errorf("tool start requires call id and name")
		}
		if _, exists := state.openToolCalls[event.CallID]; exists {
			return fmt.Errorf("tool call %q already started", event.CallID)
		}
		_, err := h.commit(context.Background(), state, []EventPayload{
			ToolCallStartedEvent{Call: ToolCallState{
				CallID: event.CallID, Name: event.Name,
				ArgumentsDescriptor: describePayload(event.Arguments),
				OperationID:         state.activeOperation, Cycle: state.activeCycle,
			}},
		})
		return err
	case EngineToolProgress:
		if _, exists := state.openToolCalls[event.CallID]; !exists {
			return fmt.Errorf("progress for unknown tool call %q", event.CallID)
		}
		if int64(len(event.Delta)) > state.memoryLimits.normalized().MaxActiveOutputBytes {
			err := &ByteBudgetError{
				Scope: ByteBudgetToolProgress, Incoming: int64(len(event.Delta)),
				Limit: state.memoryLimits.normalized().MaxActiveOutputBytes,
			}
			state.activeOutputError = err
			h.publishByteBudgetExceeded(state, err)
			return err
		}
		state.publish(Event{
			Cursor: state.cursor, Durability: EventEphemeral,
			Payload: ToolProgressEvent{CallID: event.CallID, Delta: event.Delta},
		})
		return nil
	case EngineToolFinished:
		call, exists := state.openToolCalls[event.CallID]
		if !exists {
			return fmt.Errorf("finish unknown tool call %q", event.CallID)
		}
		name := event.Name
		if name == "" {
			name = call.Name
		}
		retrySafety := event.RetrySafety
		if retrySafety == "" {
			retrySafety = RetryUnknown
		}
		effects := make([]HostEffect, len(event.HostEffects))
		for index, effect := range event.HostEffects {
			effect = cloneHostEffect(effect)
			if effect.OperationID != state.activeOperation || effect.Cycle != state.activeCycle || effect.CallID != call.CallID || effect.Index != index {
				return fmt.Errorf("%w: host effect does not match active tool identity", ErrInvalidCommand)
			}
			effects[index] = effect
		}
		if err := state.validateHostEffectAdmission(state.binding, effects); err != nil {
			var budget *ByteBudgetError
			if errors.As(err, &budget) && budget != nil {
				cloned := *budget
				state.activeOutputError = &cloned
				h.publishByteBudgetExceeded(state, &cloned)
			}
			return err
		}
		_, err := h.commit(context.Background(), state, []EventPayload{
			ToolCallFinishedEvent{
				CallID: event.CallID, Name: name, ResultDescriptor: describePayload([]byte(event.Result)),
				IsError: event.IsError, RetrySafety: retrySafety, HostEffects: effects,
			},
		})
		return err
	default:
		return fmt.Errorf("unsupported engine event %T", request.event)
	}
}

func (h *Harness) publishByteBudgetExceeded(state *harnessState, err error) {
	var budget *ByteBudgetError
	if !errors.As(err, &budget) || budget == nil {
		return
	}
	state.publish(Event{
		Cursor: state.cursor, Durability: EventEphemeral,
		Payload: ByteBudgetExceededEvent{
			OperationID: state.activeOperation, Scope: budget.Scope,
			Current: budget.Current, Incoming: budget.Incoming, Limit: budget.Limit,
		},
	})
}

func (h *Harness) authorizeDomainCommit(state *harnessState, event EngineDomainCommitIntent) error {
	// Input is the idempotent canonical projection of the already-durable
	// UserMessageCommitted event. It must survive an immediate Abort/Close so a
	// refresh never loses the user's attempted turn. Output remains the actual
	// side-effect race and is rejected when Abort/Close won ordering.
	if event.Identity.Stage != DomainCommitInput && (state.abortRequested || state.closing) {
		return fmt.Errorf("%w: abort or close won domain commit admission", ErrDomainCommitRejected)
	}
	if err := state.validateDomainCommitIdentity(event.Identity); err != nil {
		return err
	}
	if strings.TrimSpace(event.Hash) == "" || len(event.Hash) > 256 {
		return fmt.Errorf("%w: domain commit hash is invalid", ErrDomainCommitRejected)
	}
	if len(state.activeToolCalls()) != 0 {
		return fmt.Errorf("%w: tool effects are still open", ErrDomainCommitRejected)
	}
	commit := state.domainCommit(event.Identity.Stage)
	if commit != nil {
		if commit.Identity == event.Identity && commit.Hash == event.Hash {
			return nil
		}
		return fmt.Errorf("%w: a different domain commit is already authorized", ErrDomainCommitRejected)
	}
	_, err := h.commit(context.Background(), state, []EventPayload{
		DomainCommitIntentAcceptedEvent{Identity: event.Identity, Hash: event.Hash},
	})
	return err
}

func (h *Harness) recordDomainCommitReceipt(state *harnessState, event EngineDomainCommitReceipt) error {
	commit := state.domainCommit(event.Identity.Stage)
	if commit == nil {
		return fmt.Errorf("%w: domain commit receipt has no accepted intent", ErrDomainCommitRejected)
	}
	if err := state.validateDomainCommitIdentity(event.Identity); err != nil {
		return err
	}
	if commit.Identity != event.Identity || commit.Hash != event.Hash {
		return fmt.Errorf("%w: domain commit receipt identity or hash mismatch", ErrDomainCommitRejected)
	}
	if strings.TrimSpace(event.Revision) == "" || len(event.Revision) > 4096 {
		return fmt.Errorf("%w: domain commit revision is invalid", ErrDomainCommitRejected)
	}
	if commit.Revision != "" {
		if commit.Revision == event.Revision {
			return nil
		}
		return fmt.Errorf("%w: domain commit already has a different revision", ErrDomainCommitRejected)
	}
	_, err := h.commit(context.Background(), state, []EventPayload{
		DomainCommitReceiptEvent{Identity: event.Identity, Hash: event.Hash, Revision: event.Revision},
	})
	return err
}

func (h *Harness) handleEngineDone(state *harnessState, request engineDoneRequest) {
	if state.phase == PhaseIdle || request.operation != state.activeOperation || request.cycle != state.activeCycle {
		return
	}
	if state.phase == PhaseCompacting {
		h.handleStructuralEngineDone(state, request)
		return
	}
	state.engineControls = nil
	if len(state.pendingHostEffects) > 0 {
		allCommitted := true
		for _, id := range state.pendingHostEffectOrder {
			effect, pending := state.pendingHostEffects[id]
			if pending && !hostEffectHasExactOutputReceipt(state, effect) {
				allCommitted = false
				break
			}
		}
		var err error
		if allCommitted {
			err = h.reconcilePendingHostEffects(h.lifecycle, state)
		} else {
			err = h.abandonUncommittedHostEffects(
				h.lifecycle,
				state,
				"engine completed without the exact output-domain commit receipt",
			)
			if err == nil && request.err == nil && request.result.Status != EngineAborted {
				request.err = fmt.Errorf("host effects were abandoned because the exact output-domain commit receipt was not recorded")
			}
		}
		if err != nil {
			state.pendingEngineDone = &request
			if !state.recoveryPaused {
				if _, commitErr := h.commit(context.Background(), state, []EventPayload{OperationRecoveryPausedEvent{
					OperationID: state.activeOperation, Cycle: state.activeCycle,
					Reason: "host_effect_reconciliation_required: " + err.Error(),
				}}); commitErr != nil {
					panic(fmt.Sprintf("operation %s failed to persist host-effect recovery pause: %v", state.activeOperation, commitErr))
				}
			}
			return
		}
	}
	state.pendingEngineDone = nil
	if state.activeOutputError != nil {
		h.failActiveOperation(state, engineDoneRequest{
			operation: request.operation, cycle: request.cycle, err: state.activeOutputError,
		})
		return
	}
	if warning, wins := acknowledgedOutputCompletionWarning(state, request); wins {
		h.settleAcknowledgedOutput(state, warning)
		return
	}
	// Abort and lifecycle close are durable actor decisions. Once accepted they
	// dominate any later engine result, including an engine that ignored the
	// control lane and raced back with a successful completion.
	if state.abortRequested || (state.closing && !state.outputCommitFinalizing()) {
		h.failActiveOperation(state, request)
		return
	}
	if pending := state.pendingDomainCommit(); pending != nil {
		h.failActiveOperation(state, engineDoneRequest{
			operation: request.operation, cycle: request.cycle,
			err: fmt.Errorf("authorized %s domain commit ended without a durable receipt", pending.Identity.Stage),
		})
		return
	}
	if request.err != nil || request.result.Status == EngineAborted || len(state.activeToolCalls()) > 0 {
		h.failActiveOperation(state, request)
		return
	}

	var next QueuedInput
	var hasNext bool
	switch request.result.Status {
	case EnginePreempted:
		next, hasNext = state.firstQueued(DeliverySteer)
		if !hasNext {
			h.failActiveOperation(state, engineDoneRequest{
				operation: request.operation, cycle: request.cycle,
				err: fmt.Errorf("engine preempted without a steering command"),
			})
			return
		}
	case EngineCompleted:
		next, hasNext = state.firstQueued(DeliverySteer, DeliveryFollowUp)
	default:
		h.failActiveOperation(state, engineDoneRequest{
			operation: request.operation, cycle: request.cycle,
			err: fmt.Errorf("engine returned unsupported status %q", request.result.Status),
		})
		return
	}

	if hasNext {
		if state.closing {
			h.settleCommittedAndClose(state)
			return
		}
		h.startQueuedCycle(state, next)
		return
	}
	if state.closing {
		h.settleCommittedAndClose(state)
		return
	}
	h.settleAndAdvance(state)
}

func (h *Harness) handleStructuralEngineDone(state *harnessState, request engineDoneRequest) {
	state.engineControls = nil
	if warning, wins := acknowledgedOutputCompletionWarning(state, request); wins {
		h.settleAcknowledgedOutput(state, warning)
		return
	}
	if state.abortRequested || (state.closing && !state.outputCommitFinalizing()) {
		h.failActiveOperation(state, request)
		return
	}
	if pending := state.pendingDomainCommit(); pending != nil {
		h.failActiveOperation(state, engineDoneRequest{
			operation: request.operation, cycle: request.cycle,
			err: fmt.Errorf("authorized structural domain commit ended without a durable receipt"),
		})
		return
	}
	if request.err != nil || request.result.Status != EngineCompleted {
		h.failActiveOperation(state, request)
		return
	}
	h.settleSuccessfulOperation(state, "", resumeAcceptedNextTurn, "structural_operation_settled")
}

func (h *Harness) settleCommittedAndClose(state *harnessState) {
	h.settleSuccessfulOperation(state, "", preserveAcceptedNextTurn, "runtime_closed_after_domain_commit")
}

func (h *Harness) startQueuedCycle(state *harnessState, item QueuedInput) {
	// A recovered operation can contain more than one accepted transient
	// input. Rebuild each input's process-local dependency at the queue-consume
	// seam; otherwise finishing the first replayed cycle could blindly consume a
	// later command that only its own exact replay can reconstruct.
	nextCycle := state.activeCycle + 1
	snapshotID := SnapshotID(newID("snapshot"))
	_, err := h.commit(context.Background(), state, []EventPayload{
		SavePointCommittedEvent{OperationID: state.activeOperation, Cycle: state.activeCycle},
		QueueConsumedEvent{CommandID: item.CommandID, Delivery: item.Delivery},
		UserMessageCommittedEvent{Message: newUserMessage(state.activeOperation, item.Input)},
		CycleStartedEvent{OperationID: state.activeOperation, Cycle: nextCycle, SnapshotID: snapshotID},
		InputMaterializationRecoveryPendingEvent{
			OperationID: state.activeOperation, Cycle: nextCycle,
			CommandID: item.CommandID, Delivery: item.Delivery,
		},
	})
	if err != nil {
		// The actor must not remain "running" without an engine. Closing the
		// harness leaves the unfinished durable operation for conservative
		// recovery on the next open; retrying this side-effect boundary here
		// could execute a cycle twice.
		panic(fmt.Sprintf("operation %s failed to commit queued cycle: %v", state.activeOperation, err))
	}
	if err := h.resumePendingInputMaterialization(h.lifecycle, state); err != nil {
		if _, fatal := terminalJournalAppendError(err); fatal {
			panic(fmt.Sprintf("operation %s failed to persist queued input materialization: %v", state.activeOperation, err))
		}
		log.Printf(
			"runtime: binding=%+v command=%s operation=%s cycle=%d accepted input remains pending: %v",
			h.binding,
			item.CommandID,
			state.activeOperation,
			state.activeCycle,
			err,
		)
		return
	}
}

func (h *Harness) settleAndAdvance(state *harnessState) {
	h.settleSuccessfulOperation(state, "", resumeAcceptedNextTurn, "operation_settled")
}

func (h *Harness) failActiveOperation(state *harnessState, request engineDoneRequest) {
	operationID := state.activeOperation
	payloads := h.uncertainToolResults(state)
	payloads = append(payloads, transientQueueCancellations(state, "operation_failed")...)
	status := OperationFailed
	reason := "agent engine failed"
	if request.err != nil {
		reason = request.err.Error()
	}
	if errors.Is(request.err, ErrByteBudgetExceeded) {
		status = OperationIncomplete
	}
	if state.abortRequested || request.result.Status == EngineAborted {
		status = OperationAborted
		reason = state.abortReason
		if strings.TrimSpace(reason) == "" {
			reason = "agent operation aborted"
		}
	}
	plan := h.planNextTurn(h.lifecycle, state, !state.closing)
	reason = appendOperationReason(reason, plan.pendingReason())
	payloads = append(payloads, OperationSettledEvent{OperationID: operationID, Status: status, Reason: reason})
	payloads = plan.appendStart(payloads)
	if _, err := h.commit(context.Background(), state, payloads); err != nil {
		panic(fmt.Sprintf("operation %s failed to record terminal state: %v", operationID, err))
	}
	_ = h.startPlannedNextTurn(state, plan)
}

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
		// Opening is attach-only; its exact safe action retries materialization.
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
