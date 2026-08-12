package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

func (h *Harness) startEngine(state *harnessState, snapshot TurnSnapshot) {
	controls := make(chan EngineControl, state.memoryLimits.normalized().MaxPendingInteractions+4)
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
				slog.InfoContext(context.Background(), fmt.Sprintf("runtime: binding=%+v operation=%s cycle=%d engine exited without a result", h.binding, snapshot.OperationID, snapshot.Cycle))
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
	case EngineCapabilityState:
		payload := CapabilityStateCommittedEvent{
			Capability: event.Capability, Expected: event.Expected,
			State: cloneRawMessage(event.State), Deleted: event.Delete,
			OperationID: request.operation, Cycle: request.cycle,
		}
		if err := state.validateCapabilityStateEvent(payload); err != nil {
			return err
		}
		_, err := h.commit(context.Background(), state, []EventPayload{payload})
		return err
	}
	if state.phase == PhaseCompacting {
		return fmt.Errorf("unsupported structural engine event %T", request.event)
	}
	if state.abortRequested || state.closing {
		// A known completion for a tool that was already running still improves
		// recovery accuracy. Every other late event is rejected so accepted abort
		// cannot commit new assistant output or begin another side effect.
		_, toolFinished := request.event.(EngineToolFinished)
		_, stateCheckpoint := request.event.(EngineStateCheckpoint)
		if !toolFinished && !stateCheckpoint {
			return fmt.Errorf("engine event %T rejected after operation abort", request.event)
		}
	}
	switch event := request.event.(type) {
	case EngineAssistantDelta:
		if err := validateEventSource(event.Source); err != nil {
			return err
		}
		if !event.DisplayOnly {
			if err := state.admitActiveBytes(ByteBudgetActiveOutput, len(event.Delta)); err != nil {
				h.publishByteBudgetExceeded(state, err)
				return err
			}
			state.activeContent.WriteString(event.Delta)
		} else if int64(len(event.Delta)) > state.memoryLimits.normalized().MaxActiveOutputBytes {
			return &ByteBudgetError{Scope: ByteBudgetActiveOutput, Incoming: int64(len(event.Delta)), Limit: state.memoryLimits.normalized().MaxActiveOutputBytes}
		}
		state.publish(Event{
			Cursor: state.cursor, Durability: EventEphemeral,
			Payload: AssistantDeltaEvent{
				OperationID: state.activeOperation, Source: cloneEventSource(event.Source),
				Delta: event.Delta, DisplayOnly: event.DisplayOnly,
			},
		})
		return nil
	case EngineThinkingDelta:
		if err := validateEventSource(event.Source); err != nil {
			return err
		}
		if !event.DisplayOnly {
			if err := state.admitActiveBytes(ByteBudgetActiveOutput, len(event.Delta)); err != nil {
				h.publishByteBudgetExceeded(state, err)
				return err
			}
			state.activeThinking.WriteString(event.Delta)
		} else if int64(len(event.Delta)) > state.memoryLimits.normalized().MaxActiveOutputBytes {
			return &ByteBudgetError{Scope: ByteBudgetActiveOutput, Incoming: int64(len(event.Delta)), Limit: state.memoryLimits.normalized().MaxActiveOutputBytes}
		}
		state.publish(Event{
			Cursor: state.cursor, Durability: EventEphemeral,
			Payload: ThinkingDeltaEvent{
				OperationID: state.activeOperation, Source: cloneEventSource(event.Source),
				Delta: event.Delta, DisplayOnly: event.DisplayOnly,
			},
		})
		return nil
	case EngineToolInputStarted:
		if strings.TrimSpace(event.CallID) == "" || strings.TrimSpace(event.Name) == "" {
			return fmt.Errorf("tool input start requires call id and name")
		}
		if err := validateEventSource(event.Source); err != nil {
			return err
		}
		state.publish(Event{
			Cursor: state.cursor, Durability: EventEphemeral,
			Payload: ToolInputStartedEvent{
				OperationID: state.activeOperation, Cycle: state.activeCycle,
				CallID: event.CallID, ProviderCallID: event.ProviderCallID, Name: event.Name,
				Source: cloneEventSource(event.Source),
			},
		})
		return nil
	case EngineToolInputDelta:
		if strings.TrimSpace(event.CallID) == "" || strings.TrimSpace(event.Name) == "" {
			return fmt.Errorf("tool input delta requires call id and name")
		}
		if event.Delta == "" {
			return nil
		}
		if err := validateEventSource(event.Source); err != nil {
			return err
		}
		if int64(len(event.Delta)) > state.memoryLimits.normalized().MaxActiveOutputBytes {
			err := &ByteBudgetError{
				Scope: ByteBudgetActiveOutput, Incoming: int64(len(event.Delta)),
				Limit: state.memoryLimits.normalized().MaxActiveOutputBytes,
			}
			state.activeOutputError = err
			h.publishByteBudgetExceeded(state, err)
			return err
		}
		state.publish(Event{
			Cursor: state.cursor, Durability: EventEphemeral,
			Payload: ToolInputDeltaEvent{
				OperationID: state.activeOperation, Cycle: state.activeCycle,
				CallID: event.CallID, ProviderCallID: event.ProviderCallID, Name: event.Name,
				Delta: event.Delta, Source: cloneEventSource(event.Source),
			},
		})
		return nil
	case EngineModelCompleted:
		if err := validateEventSource(event.Source); err != nil {
			return err
		}
		if event.Usage.PromptTokens < 0 || event.Usage.CachedPromptTokens < 0 ||
			event.Usage.CompletionTokens < 0 || event.Usage.ReasoningTokens < 0 || event.Usage.TotalTokens < 0 {
			return fmt.Errorf("model usage cannot contain negative token counts")
		}
		if event.Usage.CachedPromptTokens > event.Usage.PromptTokens ||
			event.Usage.ReasoningTokens > event.Usage.CompletionTokens {
			return fmt.Errorf("model usage token details exceed their totals")
		}
		requested := append([]string(nil), event.RequestedTools...)
		state.publish(Event{Cursor: state.cursor, Durability: EventEphemeral, Payload: ModelCompletedEvent{
			OperationID: state.activeOperation, Cycle: state.activeCycle, Usage: event.Usage,
			FinishReason: event.FinishReason, RequestedTools: requested, Source: cloneEventSource(event.Source),
		}})
		return nil
	case EngineStateCheckpoint:
		if err := state.validateEngineState(event.State); err != nil {
			h.publishByteBudgetExceeded(state, err)
			return err
		}
		if event.State == nil {
			return fmt.Errorf("engine state checkpoint cannot be nil")
		}
		_, err := h.commit(context.Background(), state, []EventPayload{EngineStateCommittedEvent{
			State: cloneRawMessage(event.State), Descriptor: describePayload(event.State),
		}})
		return err
	case EngineInteractionRequested:
		payload := InteractionRequestedEvent{
			ID: event.ID, OperationID: request.operation, Cycle: request.cycle,
			ToolCallID: event.ToolCallID, Request: cloneRawMessage(event.Request),
			Descriptor: describePayload(event.Request),
		}
		if existing, exists := state.interactions[event.ID]; exists {
			if existing.OperationID == request.operation && existing.Cycle == request.cycle &&
				existing.ToolCallID == event.ToolCallID && string(existing.Request) == string(event.Request) {
				return nil
			}
			return fmt.Errorf("%w: interaction id belongs to another request", ErrInvalidCommand)
		}
		if err := state.validateInteractionRequest(payload); err != nil {
			return err
		}
		_, err := h.commit(context.Background(), state, []EventPayload{payload})
		return err
	case EngineAssistantFinal:
		if err := state.admitFinalOutput(event.Content, event.Thinking); err != nil {
			h.publishByteBudgetExceeded(state, err)
			return err
		}
		if err := state.validateEngineState(event.State); err != nil {
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
		payloads := make([]EventPayload, 0, 4)
		if event.State != nil {
			payloads = append(payloads, EngineStateCommittedEvent{
				State: cloneRawMessage(event.State), Descriptor: describePayload(event.State),
			})
		}
		payloads = append(payloads, AssistantMessageCommittedEvent{Message: Message{
			ID: newID("message"), Role: RoleAssistant,
			Content: event.Content, Thinking: event.Thinking,
			Operation: state.activeOperation,
		}})
		if event.Continuation != nil && !state.hasQueued(DeliverySteer) && !state.hasQueued(DeliveryFollowUp) {
			continuation := *event.Continuation
			if !continuation.Autonomous {
				return fmt.Errorf("engine continuation must be autonomous")
			}
			if err := ValidateCommandID(string(continuation.CommandID), h.inputLimits); err != nil {
				return fmt.Errorf("invalid engine continuation command: %w", err)
			}
			if err := validateUserInput(continuation.Input, h.inputLimits); err != nil {
				return fmt.Errorf("invalid engine continuation input: %w", err)
			}
			command := FollowUp{
				ID: continuation.CommandID, OperationID: state.activeOperation,
				Input: cloneUserInput(continuation.Input),
			}
			fingerprint, err := CommandFingerprint(command)
			if err != nil {
				return fmt.Errorf("fingerprint engine continuation: %w", err)
			}
			if existing, ok := state.receipts[continuation.CommandID]; ok {
				if existing.OperationID != state.activeOperation || state.fingerprints[continuation.CommandID] != fingerprint {
					return fmt.Errorf("%w: engine continuation command id was already used", ErrInvalidCommand)
				}
			} else {
				if err := state.admitPendingInput(continuation.Input); err != nil {
					return err
				}
				payloads = append(payloads,
					CommandAcceptedEvent{
						CommandID: continuation.CommandID, CommandKind: "engine_continuation",
						OperationID: state.activeOperation, Fingerprint: fingerprint,
					},
					QueueEnqueuedEvent{Item: QueuedInput{
						CommandID: continuation.CommandID, OperationID: state.activeOperation,
						Delivery: DeliveryFollowUp, Input: cloneUserInput(continuation.Input), Autonomous: true,
					}},
				)
			}
		}
		_, err := h.commit(context.Background(), state, payloads)
		return err
	case EngineToolStarted:
		if strings.TrimSpace(event.CallID) == "" || strings.TrimSpace(event.Name) == "" {
			return fmt.Errorf("tool start requires call id and name")
		}
		if err := validateEventSource(event.Source); err != nil {
			return err
		}
		if existing, exists := state.openToolCalls[event.CallID]; exists {
			if existing.OperationID == state.activeOperation && existing.Cycle == state.activeCycle && existing.Name == event.Name &&
				existing.ArgumentsDescriptor == describePayload(event.Arguments) && eventSourcesEqual(existing.Source, event.Source) {
				state.publish(Event{Cursor: state.cursor, Durability: EventEphemeral, Payload: ToolStartedEvent{
					OperationID: state.activeOperation, Cycle: state.activeCycle,
					CallID: event.CallID, Name: event.Name, Arguments: cloneRawMessage(event.Arguments),
					Source: cloneEventSource(event.Source),
				}})
				return nil
			}
			return fmt.Errorf("tool call %q already started with a different identity", event.CallID)
		}
		_, err := h.commit(context.Background(), state, []EventPayload{
			ToolCallStartedEvent{Call: ToolCallState{
				CallID: event.CallID, Name: event.Name,
				ArgumentsDescriptor: describePayload(event.Arguments),
				OperationID:         state.activeOperation, Cycle: state.activeCycle,
				Source: cloneEventSource(event.Source),
			}},
		})
		if err == nil {
			state.publish(Event{Cursor: state.cursor, Durability: EventEphemeral, Payload: ToolStartedEvent{
				OperationID: state.activeOperation, Cycle: state.activeCycle,
				CallID: event.CallID, Name: event.Name, Arguments: cloneRawMessage(event.Arguments),
				Source: cloneEventSource(event.Source),
			}})
		}
		return err
	case EngineToolProgress:
		call, exists := state.openToolCalls[event.CallID]
		if !exists {
			return fmt.Errorf("progress for unknown tool call %q", event.CallID)
		}
		if err := validateEventSource(event.Source); err != nil {
			return err
		}
		if !eventSourcesEqual(call.Source, event.Source) {
			return fmt.Errorf("tool progress source does not match call %q", event.CallID)
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
			Payload: ToolProgressEvent{CallID: event.CallID, Delta: event.Delta, Source: cloneEventSource(event.Source)},
		})
		return nil
	case EngineArtifactProduced:
		call, exists := state.openToolCalls[event.CallID]
		if !exists || call.OperationID != state.activeOperation || call.Cycle != state.activeCycle {
			return fmt.Errorf("artifact for unknown tool call %q", event.CallID)
		}
		if len(event.Artifact) == 0 || len(event.Artifact) > 256<<10 || !json.Valid(event.Artifact) {
			return fmt.Errorf("tool artifact metadata must contain valid JSON of at most %d bytes", 256<<10)
		}
		_, err := h.commit(context.Background(), state, []EventPayload{ArtifactProducedEvent{
			OperationID: state.activeOperation, Cycle: state.activeCycle,
			CallID: event.CallID, Artifact: cloneRawMessage(event.Artifact),
		}})
		return err
	case EngineToolFinished:
		call, exists := state.openToolCalls[event.CallID]
		if !exists {
			return fmt.Errorf("finish unknown tool call %q", event.CallID)
		}
		if err := validateEventSource(event.Source); err != nil {
			return err
		}
		if !eventSourcesEqual(call.Source, event.Source) {
			return fmt.Errorf("tool finish source does not match call %q", event.CallID)
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
		if err == nil {
			state.publish(Event{Cursor: state.cursor, Durability: EventEphemeral, Payload: ToolOutputEvent{
				OperationID: state.activeOperation, Cycle: state.activeCycle,
				CallID: event.CallID, Name: name, Result: event.Result, IsError: event.IsError,
				Source: cloneEventSource(event.Source), Projection: cloneRawMessage(event.Projection),
			}})
		}
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
		if state.preemptQueuedCommandID != "" {
			next, hasNext = state.queued(state.preemptQueuedCommandID)
		} else {
			next, hasNext = state.firstQueued(DeliverySteer)
		}
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
	capabilities, err := h.prepareAdmission(context.Background(), state, item.CommandID, state.activeOperation, nextCycle, item.Input)
	if err != nil {
		h.failActiveOperation(state, engineDoneRequest{
			operation: state.activeOperation, cycle: state.activeCycle,
			err: fmt.Errorf("prepare queued cycle admission: %w", err),
		})
		return
	}
	payloads := []EventPayload{
		SavePointCommittedEvent{OperationID: state.activeOperation, Cycle: state.activeCycle},
		QueueConsumedEvent{CommandID: item.CommandID, Delivery: item.Delivery},
	}
	payloads = append(payloads, capabilities...)
	payloads = append(payloads,
		UserMessageCommittedEvent{Message: newUserMessage(state.activeOperation, item.Input)},
		CycleStartedEvent{OperationID: state.activeOperation, Cycle: nextCycle, SnapshotID: snapshotID},
		InputMaterializationRecoveryPendingEvent{
			OperationID: state.activeOperation, Cycle: nextCycle,
			CommandID: item.CommandID, Delivery: item.Delivery, Autonomous: item.Autonomous,
		},
	)
	_, err = h.commit(context.Background(), state, payloads)
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
		slog.InfoContext(context.Background(), fmt.Sprintf(
			"runtime: binding=%+v command=%s operation=%s cycle=%d accepted input remains pending: %v",
			h.binding,
			item.CommandID,
			state.activeOperation,
			state.activeCycle,
			err,
		))
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
