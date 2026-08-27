package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
)

func (run *Run) execute() {
	input := cloneInput(run.input)
	delivery := run.delivery
	autonomous := false
	for {
		result, continuation, err := run.executeCycle(input, delivery, autonomous)
		if err != nil {
			if errors.Is(err, context.Canceled) && run.ctx.Err() != nil {
				run.finish(Result{Status: ResultAborted, Reason: "Agent Run cancelled"}, nil)
			} else {
				run.finish(Result{Status: ResultFailed, Reason: err.Error()}, &RunError{Result: Result{Status: ResultFailed, Reason: err.Error()}})
			}
			return
		}
		switch result.Status {
		case runstate.EngineAborted:
			run.finish(Result{Status: ResultAborted, Reason: run.currentAbortReason()}, nil)
			return
		case runstate.EnginePreempted:
			if next, nextDelivery, ok := run.nextQueuedInput(); ok {
				input, delivery = next, nextDelivery
				autonomous = false
				continue
			}
			run.finish(Result{Status: ResultIncomplete, Reason: "Agent Run interrupted"}, &RunError{Result: Result{Status: ResultIncomplete, Reason: "Agent Run interrupted"}})
			return
		case runstate.EngineCompleted:
			if continuation != nil {
				next, decodeErr := decodeInput(continuation.Input)
				if decodeErr != nil {
					run.finish(Result{Status: ResultFailed, Reason: decodeErr.Error()}, decodeErr)
					return
				}
				next.IdempotencyKey = string(continuation.CommandID)
				input, delivery = next, runstate.DeliveryFollowUp
				autonomous = continuation.Autonomous
				continue
			}
			if next, nextDelivery, ok := run.nextQueuedInput(); ok {
				input, delivery = next, nextDelivery
				autonomous = false
				continue
			}
			run.finish(Result{Status: ResultCompleted}, nil)
			return
		default:
			err := errors.New("Agent engine returned an invalid status")
			run.finish(Result{Status: ResultFailed, Reason: err.Error()}, err)
			return
		}
	}
}

func (run *Run) executeCycle(input Input, delivery runstate.DeliveryKind, autonomous bool) (runstate.EngineResult, *runstate.EngineContinuation, error) {
	run.mu.Lock()
	run.cycle++
	run.content.Reset()
	run.thinking.Reset()
	clear(run.openTools)
	cycle := run.cycle
	run.mu.Unlock()
	commandID := strings.TrimSpace(input.IdempotencyKey)
	if commandID == "" {
		commandID = run.commandID
	}
	encoded, runInput, err := encodeInput(input)
	if err != nil {
		return runstate.EngineResult{}, nil, err
	}
	runInput.Envelope = encoded
	snapshot := runstate.TurnSnapshot{
		ID: runstate.SnapshotID(fmt.Sprintf("%s:%d", run.id, cycle)), Binding: run.session.binding.Clone(),
		CommandID: runstate.CommandID(commandID), OperationID: runstate.OperationID(run.id), Cycle: cycle,
		StartedAt: time.Now().UTC(), Delivery: delivery, Autonomous: autonomous, Input: runInput,
	}
	run.session.mu.Lock()
	snapshot.ContextCursor = runstate.Cursor(run.session.revision)
	snapshot.State = append(json.RawMessage(nil), run.session.engineState...)
	snapshot.Capabilities = cloneRawStateMap(run.session.capabilities)
	run.session.mu.Unlock()

	if preparer, ok := run.session.engine.(runstate.EngineAdmissionPreparer); ok {
		updates, prepareErr := preparer.PrepareAdmission(run.ctx, runstate.TurnAdmissionRequest{Snapshot: snapshot})
		if prepareErr != nil {
			return runstate.EngineResult{}, nil, prepareErr
		}
		if err := run.applyCapabilityUpdates(updates); err != nil {
			return runstate.EngineResult{}, nil, err
		}
		run.session.mu.RLock()
		snapshot.Capabilities = cloneRawStateMap(run.session.capabilities)
		run.session.mu.RUnlock()
	}
	if materializer, ok := run.session.engine.(runstate.EngineInputMaterializer); ok {
		request := runstate.InputMaterializationRequest{Binding: run.session.binding.Clone(), Snapshot: snapshot}
		plan, planErr := materializer.PlanInputMaterialization(run.ctx, request)
		if planErr != nil {
			return runstate.EngineResult{}, nil, planErr
		}
		if plan.Required {
			receipt, materializeErr := materializer.MaterializeInput(run.ctx, request, plan)
			if materializeErr != nil {
				return runstate.EngineResult{}, nil, materializeErr
			}
			snapshot.InputCommit = &runstate.DomainCommitState{
				Identity: runstate.DomainCommitIdentity{CommandID: snapshot.CommandID, OperationID: snapshot.OperationID, Cycle: cycle, Stage: runstate.DomainCommitInput},
				Hash:     plan.Hash, Revision: receipt.Revision,
			}
		}
	}
	run.mu.Lock()
	run.snapshot = snapshot
	run.mu.Unlock()
	startedAt := run.startedAtValue()
	if startedAt.IsZero() {
		// Defensive fallback for alternate Run constructors: the first cycle is
		// already executing, so its immutable snapshot time is the correct edge.
		startedAt = snapshot.StartedAt
		run.markStarted(startedAt)
	}
	run.publish(RunStarted{Cycle: cycle, CommandID: commandID, Delivery: string(snapshot.Delivery), StartedAt: startedAt})
	var continuation *runstate.EngineContinuation
	result, err := run.session.engine.Run(run.ctx, runstate.EngineRequest{
		Binding: run.session.binding.Clone(), Snapshot: snapshot, Controls: run.controls,
	}, func(event runstate.EngineEvent) error {
		if final, ok := event.(runstate.EngineAssistantFinal); ok {
			continuation = final.Continuation
		}
		return run.handleEngineEvent(event)
	})
	if err == nil && result.Status == runstate.EngineAborted {
		err = run.persistEngineTranscript()
	}
	return result, continuation, err
}

func (run *Run) handleEngineEvent(event runstate.EngineEvent) error {
	switch value := event.(type) {
	case runstate.EngineAssistantDelta:
		if !value.DisplayOnly {
			run.mu.Lock()
			run.content.WriteString(value.Delta)
			run.mu.Unlock()
		}
		run.publish(AssistantDelta{Source: publicEventSource(value.Source), Delta: value.Delta, DisplayOnly: value.DisplayOnly})
	case runstate.EngineThinkingDelta:
		if !value.DisplayOnly {
			run.mu.Lock()
			run.thinking.WriteString(value.Delta)
			run.mu.Unlock()
		}
		run.publish(ThinkingDelta{Source: publicEventSource(value.Source), Delta: value.Delta, DisplayOnly: value.DisplayOnly})
	case runstate.EngineNestedEvent:
		nested, err := decodeNestedEvent(nestedEventRecord{
			Source: publicEventSource(value.Source), ParentCallID: value.ParentCallID, SessionID: value.SessionID,
			ChildCursor: Cursor(value.ChildCursor), ChildRunID: value.ChildRunID,
			PayloadType: value.PayloadType, Payload: value.Payload,
		})
		if err != nil {
			return err
		}
		run.publish(nested)
	case runstate.EngineModelCompleted:
		run.publish(ModelCompleted{Usage: TokenUsage{
			PromptTokens:       value.Usage.PromptTokens,
			PromptTokenDetails: PromptTokenDetails{CachedTokens: value.Usage.CachedPromptTokens},
			CompletionTokens:   value.Usage.CompletionTokens, TotalTokens: value.Usage.TotalTokens,
			CompletionTokensDetails: CompletionTokensDetails{ReasoningTokens: value.Usage.ReasoningTokens},
		}, FinishReason: value.FinishReason, RequestedTools: append([]string(nil), value.RequestedTools...), Source: publicEventSource(value.Source)})
	case runstate.EngineTranscriptUpdated:
		return run.updateEngineTranscript(value.State, false)
	case runstate.EngineCapabilityState:
		return run.applyCapabilityUpdates([]runstate.EngineCapabilityState{value})
	case runstate.EngineContextNormalized:
		run.publish(ContextNormalized{RepairCount: value.RepairCount, MessagesBefore: value.MessagesBefore, MessagesAfter: value.MessagesAfter})
	case runstate.EngineCleanupStarted:
		run.publish(CleanupStarted{ID: value.ID, Reason: value.Reason, Automatic: value.Automatic, Transient: value.Transient, Metrics: publicCleanupMetrics(value.Metrics)})
	case runstate.EngineCleanupCompleted:
		run.publish(CleanupCompleted{ID: value.ID, Reason: value.Reason, Automatic: value.Automatic, Transient: value.Transient, Metrics: publicCleanupMetrics(value.Metrics)})
	case runstate.EngineCleanupFailed:
		run.publish(CleanupFailed{ID: value.ID, Reason: value.Reason, Automatic: value.Automatic, Metrics: publicCleanupMetrics(value.Metrics)})
	case runstate.EngineCleanupSkipped:
		run.publish(CleanupSkipped{ID: value.ID, Reason: value.Reason, Automatic: value.Automatic, Metrics: publicCleanupMetrics(value.Metrics)})
	case runstate.EngineCompactionStarted:
		run.publish(CompactionStarted{ID: value.ID, Automatic: value.Automatic, Metrics: publicCompactionMetrics(value.Metrics)})
	case runstate.EngineCompactionFailed:
		run.publish(CompactionFailed{ID: value.ID, Reason: value.Reason, Automatic: value.Automatic, ConsecutiveFailures: value.ConsecutiveFailures, FailureFuseOpen: value.FailureFuseOpen, Metrics: publicCompactionMetrics(value.Metrics)})
	case runstate.EngineCompactionSkipped:
		run.publish(CompactionSkipped{ID: value.ID, Reason: value.Reason, Automatic: value.Automatic, ConsecutiveFailures: value.ConsecutiveFailures, FailureFuseOpen: value.FailureFuseOpen, Metrics: publicCompactionMetrics(value.Metrics)})
	case runstate.EngineGoalEvaluationFailed:
		run.publish(GoalEvaluationFailed{
			GoalID: value.GoalID, GoalRevision: value.GoalRevision,
			Code: value.Code, Detail: value.Detail,
		})
	case runstate.EngineInteractionRequested:
		var request InteractionRequest
		if err := json.Unmarshal(value.Request, &request); err != nil {
			return err
		}
		run.mu.Lock()
		run.interactions[value.ID] = pendingInteraction{
			request: request,
			snapshot: runstate.InteractionSnapshot{
				ID: value.ID, OperationID: runstate.OperationID(run.id), Cycle: run.cycle,
				ToolCallID: value.ToolCallID, Request: append(json.RawMessage(nil), value.Request...),
			},
		}
		run.mu.Unlock()
		run.publish(InteractionRequested{Request: request})
	case runstate.EngineAssistantFinal:
		if err := run.updateEngineTranscript(value.State, true); err != nil {
			return err
		}
		if err := run.applyCapabilityUpdates(value.CapabilityUpdates); err != nil {
			return err
		}
		if value.CleanupCompleted != nil {
			completed := value.CleanupCompleted
			run.publish(CleanupCompleted{ID: completed.ID, Reason: completed.Reason, Automatic: completed.Automatic, Metrics: publicCleanupMetrics(completed.Metrics)})
		}
		run.publish(AssistantFinal{Content: value.Content, Thinking: value.Thinking})
	case runstate.EngineToolInputStarted:
		source := publicEventSource(value.Source)
		run.mu.Lock()
		run.toolSources[value.CallID] = source
		run.mu.Unlock()
		run.publish(ToolInputStarted{
			CallID: value.CallID, ProviderCallID: value.ProviderCallID, ParentCallID: value.ParentCallID,
			Name: value.Name, Index: value.Index, Descriptor: decodeToolDescriptorMetadata(value.Metadata), Source: source,
		})
	case runstate.EngineToolInputDelta:
		run.publish(ToolInputDelta{CallID: value.CallID, ProviderCallID: value.ProviderCallID, Name: value.Name, Delta: value.Delta, Source: publicEventSource(value.Source)})
	case runstate.EngineToolStarted:
		if value.ExecutionAuthorized {
			run.mu.Lock()
			run.openTools[value.CallID] = OpenToolSnapshot{
				CallID: value.CallID, Name: value.Name, RunID: run.id,
				Cycle: run.cycle, Source: publicEventSource(value.Source),
			}
			run.mu.Unlock()
			run.publish(ToolStarted{CallID: value.CallID, ProviderCallID: value.ProviderCallID, Name: value.Name, Index: value.Index, Arguments: append(json.RawMessage(nil), value.Arguments...), Descriptor: decodeToolDescriptorMetadata(value.Metadata), Source: publicEventSource(value.Source)})
		}
	case runstate.EngineToolProgress:
		run.publish(ToolProgress{CallID: value.CallID, ProviderCallID: value.ProviderCallID, Name: value.Name, Index: value.Index, Delta: value.Delta, Descriptor: decodeToolDescriptorMetadata(value.Metadata), Source: publicEventSource(value.Source)})
	case runstate.EngineArtifactProduced:
		var artifact ToolArtifactRef
		if err := json.Unmarshal(value.Artifact, &artifact); err != nil {
			return err
		}
		run.mu.RLock()
		source := run.toolSources[value.CallID]
		run.mu.RUnlock()
		run.publish(ArtifactProduced{CallID: value.CallID, Artifact: artifact, Source: source})
	case runstate.EngineToolFinished:
		run.mu.Lock()
		delete(run.openTools, value.CallID)
		run.mu.Unlock()
		var projection *ToolResult
		if len(value.Projection) != 0 {
			var decoded ToolResult
			if json.Unmarshal(value.Projection, &decoded) == nil {
				projection = &decoded
			}
		}
		run.publish(ToolFinished{CallID: value.CallID, ProviderCallID: value.ProviderCallID, Name: value.Name, Index: value.Index, IsError: value.IsError, Result: value.Result, Descriptor: decodeToolDescriptorMetadata(value.Metadata), Projection: projection, Source: publicEventSource(value.Source)})
	default:
		return fmt.Errorf("unsupported Agent engine event %T", event)
	}
	return nil
}

func (run *Run) applyCapabilityUpdates(updates []runstate.EngineCapabilityState) error {
	if len(updates) == 0 {
		return nil
	}
	run.session.mu.Lock()
	for _, update := range updates {
		if !update.CompareCurrent {
			continue
		}
		current, present := run.session.capabilities[update.Capability]
		if present != update.ExpectedPresent || present && !bytes.Equal(current, update.ExpectedState) {
			run.session.mu.Unlock()
			return ErrCapabilityStateConflict
		}
	}
	changed := false
	for _, update := range updates {
		if update.CheckOnly {
			continue
		}
		changed = true
		if update.Delete {
			delete(run.session.capabilities, update.Capability)
		} else {
			run.session.capabilities[update.Capability] = append(json.RawMessage(nil), update.State...)
		}
	}
	if changed {
		if err := run.session.persistCapabilitiesLocked(context.Background()); err != nil {
			run.session.mu.Unlock()
			return err
		}
	}
	run.session.mu.Unlock()
	if !changed {
		return nil
	}
	run.mu.Lock()
	if run.snapshot.Capabilities == nil {
		run.snapshot.Capabilities = make(map[string]json.RawMessage)
	}
	for _, update := range updates {
		if update.CheckOnly {
			continue
		}
		if update.Delete {
			delete(run.snapshot.Capabilities, update.Capability)
		} else {
			run.snapshot.Capabilities[update.Capability] = append(json.RawMessage(nil), update.State...)
		}
	}
	run.mu.Unlock()
	for _, update := range updates {
		if update.CheckOnly {
			continue
		}
		run.publishCapabilityUpdate(update)
	}
	return nil
}

func (run *Run) publishCapabilityUpdate(update runstate.EngineCapabilityState) {
	switch update.Capability {
	case goalCapability:
		if update.Delete {
			run.publish(GoalUpdated{})
		} else if state, err := decodeGoalState(update.State); err == nil {
			run.publish(GoalUpdated{State: state, Present: state.Visible()})
		}
	case TodoCapability:
		var state TodoState
		if !update.Delete && json.Unmarshal(update.State, &state) == nil {
			run.publish(TodoUpdated{State: state})
		}
	case cleanupCapability:
		if state, err := decodeCleanupState(update.State); !update.Delete && err == nil && !state.Removed {
			run.publish(CleanupCommitted{State: state, Automatic: run.cycleValue() > 0})
		}
	case compactionCapability:
		if state, err := decodeCompactionState(update.State); !update.Delete && err == nil {
			if state.Removed {
				run.publish(CompactionRemoved{ID: state.ID, Revision: state.Revision})
			} else {
				run.publish(CompactionCommitted{State: state, Automatic: run.cycleValue() > 0})
			}
		}
	}
}
