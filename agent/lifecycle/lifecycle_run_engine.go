package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	agenthistory "github.com/alfredxw/denova/agent/context/history"
	agentengine "github.com/alfredxw/denova/agent/engine"
	agentexecution "github.com/alfredxw/denova/agent/engine/execution"
	agentgoal "github.com/alfredxw/denova/agent/engine/goal"
	agentasync "github.com/alfredxw/denova/agent/internal/async"
	agentretry "github.com/alfredxw/denova/agent/internal/retry"
	agentevent "github.com/alfredxw/denova/agent/lifecycle/event"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agentcanonical "github.com/alfredxw/denova/agent/session/canonical"
	agenttool "github.com/alfredxw/denova/agent/tool"
)

func (run *Run) execute() {
	defer close(run.executionDone)
	if run.resuming {
		status, err := run.awaitRecoveryInput()
		if err != nil {
			run.finish(agentschema.Result{Status: agentschema.ResultFailed, Reason: err.Error()}, err)
			return
		}
		switch status {
		case agentengine.Aborted:
			run.finish(agentschema.Result{Status: agentschema.ResultAborted, Reason: run.currentAbortReason()}, nil)
			return
		case agentengine.Suspended:
			run.suspendExecution()
			return
		case "":
		default:
			run.finish(agentschema.Result{Status: agentschema.ResultFailed, Reason: "invalid recovery gate status"}, errors.New("invalid recovery gate status"))
			return
		}
	}
	input := cloneInput(run.input)
	delivery := run.delivery
	autonomous := false
	if run.cycle > 0 {
		input, _ = agentengine.DecodeInput(run.snapshot.Input)
		input.IdempotencyKey = string(run.snapshot.CommandID)
		delivery, autonomous = run.snapshot.Delivery, run.snapshot.Autonomous
	}
	for {
		if err := run.admitWork(run.ctx); err != nil {
			if run.suspensionRequested() {
				run.suspendExecution()
			} else {
				run.finish(agentschema.Result{Status: agentschema.ResultFailed, Reason: err.Error()}, err)
			}
			return
		}
		if run.suspensionRequested() {
			run.suspendExecution()
			return
		}
		result, continuation, err := run.executeCycle(input, delivery, autonomous)
		if err != nil {
			if run.suspensionRequested() && errors.Is(err, context.Canceled) {
				run.suspendExecution()
				return
			}
			if errors.Is(err, context.Canceled) && run.ctx.Err() != nil {
				run.finish(agentschema.Result{Status: agentschema.ResultAborted, Reason: "Agent Run cancelled"}, nil)
			} else {
				reason := err.Error()
				// Adapters may supply a stable localized reason; persist that
				// code while retaining the detailed failure in diagnostics.
				var coded interface{ ModelErrorReason() string }
				if errors.As(err, &coded) && coded.ModelErrorReason() != "" {
					reason = coded.ModelErrorReason()
					slog.ErrorContext(run.ctx, "Agent model input rejected", "reason", reason, "error", err)
				}
				result := agentschema.Result{Status: agentschema.ResultFailed, Reason: reason}
				run.finish(result, &agentevent.RunError{Result: result, Cause: err})
			}
			return
		}
		switch result.Status {
		case agentengine.Suspended:
			run.suspendExecution()
			return
		case agentengine.Aborted:
			run.finish(agentschema.Result{Status: agentschema.ResultAborted, Reason: run.currentAbortReason()}, nil)
			return
		case agentengine.Preempted:
			if next, nextDelivery, ok := run.nextQueuedInput(); ok {
				input, delivery = next, nextDelivery
				autonomous = false
				continue
			}
			run.finish(agentschema.Result{Status: agentschema.ResultIncomplete, Reason: "Agent Run interrupted"}, &agentevent.RunError{Result: agentschema.Result{Status: agentschema.ResultIncomplete, Reason: "Agent Run interrupted"}})
			return
		case agentengine.Incomplete:
			run.finish(agentschema.Result{Status: agentschema.ResultIncomplete, Reason: result.Reason}, nil)
			return
		case agentengine.Completed:
			if continuation != nil {
				next, decodeErr := agentengine.DecodeInput(continuation.Input)
				if decodeErr != nil {
					run.finish(agentschema.Result{Status: agentschema.ResultFailed, Reason: decodeErr.Error()}, decodeErr)
					return
				}
				next.IdempotencyKey = string(continuation.CommandID)
				input, delivery = next, agentengine.DeliveryFollowUp
				autonomous = continuation.Autonomous
				continue
			}
			if next, nextDelivery, ok := run.nextQueuedInput(); ok {
				input, delivery = next, nextDelivery
				autonomous = false
				continue
			}
			run.finish(agentschema.Result{Status: agentschema.ResultCompleted}, nil)
			return
		default:
			err := errors.New("Agent engine returned an invalid status")
			run.finish(agentschema.Result{Status: agentschema.ResultFailed, Reason: err.Error()}, err)
			return
		}
	}
}

func (run *Run) executeCycle(input agentschema.Input, delivery agentengine.DeliveryKind, autonomous bool) (agentengine.Result, *agentengine.Continuation, error) {
	cycleCtx := context.WithValue(run.ctx, resumeTreePermitKey{}, run.treeResumeID)
	cycleCtx = agentasync.ContextWithAdmission(cycleCtx, run.admitWork)
	cycleCtx = agentcanonical.ContextWithProductAcceptor(cycleCtx, runJournalPort{run: run})
	run.mu.Lock()
	resume := run.resuming
	run.resuming = false
	if !resume {
		run.cycle++
	}
	run.content.Reset()
	run.thinking.Reset()
	run.modelResponseOrdinal, run.modelContentStart, run.modelThinkingStart = 0, 0, 0
	clear(run.openTools)
	cycle := run.cycle
	run.mu.Unlock()
	commandID := strings.TrimSpace(input.IdempotencyKey)
	if commandID == "" {
		commandID = run.commandID
	}
	encoded, runInput, err := agentengine.EncodeInput(input)
	if err != nil {
		return agentengine.Result{}, nil, err
	}
	runInput.Envelope = encoded
	snapshot := agentengine.TurnSnapshot{
		ID: agentengine.SnapshotID(fmt.Sprintf("%s:%d", run.id, cycle)), Binding: run.session.binding.Clone(),
		CommandID: agentengine.CommandID(commandID), OperationID: agentengine.OperationID(run.id), Cycle: cycle,
		StartedAt: time.Now().UTC(), Delivery: delivery, Autonomous: autonomous, Input: runInput,
	}
	if resume {
		run.mu.RLock()
		snapshot = run.snapshot
		run.mu.RUnlock()
	}
	if autonomous && !resume {
		// Host input materialization must never precede durable acceptance, even
		// when the next input was produced by the Goal manager.
		if _, err := run.session.Queue(cycleCtx, input); err != nil {
			return agentengine.Result{}, nil, err
		}
	}
	run.session.mu.Lock()
	snapshot.ContextCursor = agentengine.Cursor(run.session.revision)
	snapshot.State = append(json.RawMessage(nil), run.session.engineState...)
	snapshot.Capabilities = agentschema.CloneRawStateMap(run.session.capabilities)
	run.session.mu.Unlock()

	if preparer, ok := run.session.engine.(agentengine.AdmissionPreparer); ok {
		updates, prepareErr := preparer.PrepareAdmission(cycleCtx, agentengine.TurnAdmissionRequest{Snapshot: snapshot})
		if prepareErr != nil {
			return agentengine.Result{}, nil, prepareErr
		}
		if err := run.applyCapabilityUpdates(updates); err != nil {
			return agentengine.Result{}, nil, err
		}
		run.session.mu.RLock()
		snapshot.Capabilities = agentschema.CloneRawStateMap(run.session.capabilities)
		run.session.mu.RUnlock()
	}
	if materializer, ok := run.session.engine.(agentengine.InputMaterializer); ok && snapshot.InputCommit == nil {
		request := agentengine.InputMaterializationRequest{Journal: runJournalPort{run: run}, Binding: run.session.binding.Clone(), Snapshot: snapshot}
		plan, planErr := materializer.PlanInputMaterialization(cycleCtx, request)
		if planErr != nil {
			return agentengine.Result{}, nil, planErr
		}
		if plan.Required {
			receipt, materializeErr := materializer.MaterializeInput(cycleCtx, request, plan)
			if materializeErr != nil {
				return agentengine.Result{}, nil, materializeErr
			}
			snapshot.InputCommit = &agentengine.DomainCommitState{
				Identity: agentengine.DomainCommitIdentity{CommandID: snapshot.CommandID, OperationID: snapshot.OperationID, Cycle: cycle, Stage: agentengine.DomainCommitInput},
				Hash:     plan.Hash, Revision: receipt.Revision,
			}
		}
	}
	run.session.mu.RLock()
	snapshot.State = append(json.RawMessage(nil), run.session.engineState...)
	run.session.mu.RUnlock()
	if err := run.checkpointCycle(run.ctx, snapshot); err != nil {
		return agentengine.Result{}, nil, err
	}
	startedAt := run.startedAtValue()
	if startedAt.IsZero() {
		// Defensive fallback for alternate Run constructors: the first cycle is
		// already executing, so its immutable snapshot time is the correct edge.
		startedAt = snapshot.StartedAt
		run.markStarted(startedAt)
	}
	run.publish(agentevent.RunStarted{Cycle: cycle, CommandID: commandID, Delivery: string(snapshot.Delivery), StartedAt: startedAt})
	var continuation *agentengine.Continuation
	engineCtx := agentengine.ContextWithTaskCompletionSession(cycleCtx, sessionTaskCompletions{session: run.session})
	result, err := run.session.engine.Run(engineCtx, agentengine.Request{
		Journal: runJournalPort{run: run}, Binding: run.session.binding.Clone(), Snapshot: snapshot, Controls: run.controls,
	}, func(event agentengine.Event) error {
		if final, ok := event.(agentengine.AssistantFinal); ok {
			continuation = final.Continuation
		}
		return run.handleEngineEvent(event)
	})
	if err == nil && result.Status == agentengine.Aborted {
		err = run.persistEngineTranscript()
	}
	return result, continuation, err
}

func (run *Run) handleEngineEvent(event agentengine.Event) error {
	switch value := event.(type) {
	case agentengine.AssistantDelta:
		if !value.DisplayOnly {
			run.mu.Lock()
			run.beginModelResponseLocked(value.ResponseOrdinal)
			run.content.WriteString(value.Delta)
			run.mu.Unlock()
		}
		run.publish(agentevent.AssistantDelta{Source: publicEventSource(value.Source), Delta: value.Delta, DisplayOnly: value.DisplayOnly, ResponseOrdinal: value.ResponseOrdinal})
	case agentengine.ThinkingDelta:
		if !value.DisplayOnly {
			run.mu.Lock()
			run.beginModelResponseLocked(value.ResponseOrdinal)
			run.thinking.WriteString(value.Delta)
			run.mu.Unlock()
		}
		run.publish(agentevent.ThinkingDelta{Source: publicEventSource(value.Source), Delta: value.Delta, DisplayOnly: value.DisplayOnly, ResponseOrdinal: value.ResponseOrdinal})
	case agentengine.NestedEvent:
		nested, err := agentevent.DecodeNestedEvent(agentevent.NestedEventRecord{
			Source: publicEventSource(value.Source), ParentCallID: value.ParentCallID, SessionID: value.SessionID,
			ChildCursor: agentevent.Cursor(value.ChildCursor), ChildRunID: value.ChildRunID,
			PayloadType: value.PayloadType, Payload: value.Payload,
		})
		if err != nil {
			return err
		}
		run.publish(nested)
	case agentengine.ModelRetry:
		if value.OutputState != string(agentretry.ModelOutputComplete) {
			run.mu.Lock()
			if run.modelResponseOrdinal == value.ResponseOrdinal {
				content, thinking := run.content.String()[:run.modelContentStart], run.thinking.String()[:run.modelThinkingStart]
				run.content.Reset()
				run.content.WriteString(content)
				run.thinking.Reset()
				run.thinking.WriteString(thinking)
			}
			run.mu.Unlock()
		}
		run.publish(agentevent.ModelRetry{Source: publicEventSource(value.Source), Attempt: value.Attempt, MaxAttempts: value.MaxAttempts,
			ResponseOrdinal: value.ResponseOrdinal, OutputState: agentretry.ModelOutputState(value.OutputState), Delay: value.Delay, Reason: value.Reason})
	case agentengine.ModelCompleted:
		run.publish(agentevent.ModelCompleted{Usage: agentschema.TokenUsage{
			PromptTokens:       value.Usage.PromptTokens,
			PromptTokenDetails: agentschema.PromptTokenDetails{CachedTokens: value.Usage.CachedPromptTokens},
			CompletionTokens:   value.Usage.CompletionTokens, TotalTokens: value.Usage.TotalTokens,
			CompletionTokensDetails: agentschema.CompletionTokensDetails{ReasoningTokens: value.Usage.ReasoningTokens},
		}, FinishReason: value.FinishReason, RequestedTools: append([]string(nil), value.RequestedTools...), Source: publicEventSource(value.Source)})
	case agentengine.TranscriptUpdated:
		if len(value.CapabilityStates) != 0 {
			return run.commitContextCheckpoint(value)
		}
		if len(value.TaskCompletionIDs) != 0 {
			return run.persistTaskCompletionCheckpoint(value.State, value.TaskCompletionIDs)
		}
		return run.updateEngineTranscript(value.State, true)
	case agentengine.CapabilityState:
		return run.applyCapabilityUpdates([]agentengine.CapabilityState{value})
	case agentengine.ContextNormalized:
		run.publish(agentevent.ContextNormalized{RepairCount: value.RepairCount, MessagesBefore: value.MessagesBefore, MessagesAfter: value.MessagesAfter})
	case agentengine.CompactionStarted:
		run.publish(agentevent.CompactionStarted{ID: value.ID, Automatic: value.Automatic, Metrics: publicCompactionMetrics(value.Metrics)})
	case agentengine.CompactionFailed:
		run.publish(agentevent.CompactionFailed{ID: value.ID, Reason: value.Reason, Automatic: value.Automatic, ConsecutiveFailures: value.ConsecutiveFailures, FailureFuseOpen: value.FailureFuseOpen, Metrics: publicCompactionMetrics(value.Metrics)})
	case agentengine.CompactionSkipped:
		run.publish(agentevent.CompactionSkipped{ID: value.ID, Reason: value.Reason, Automatic: value.Automatic, ConsecutiveFailures: value.ConsecutiveFailures, FailureFuseOpen: value.FailureFuseOpen, Metrics: publicCompactionMetrics(value.Metrics)})
	case agentengine.GoalEvaluationFailed:
		run.publish(agentevent.GoalEvaluationFailed{
			GoalID: value.GoalID, GoalRevision: value.GoalRevision,
			Code: value.Code, Detail: value.Detail,
		})
	case agentengine.InteractionRequested:
		request, err := run.recordInteraction(value)
		if err != nil {
			return err
		}
		run.publish(agentevent.InteractionRequested{Request: request})
	case agentengine.AssistantFinal:
		if err := run.updateEngineTranscript(value.State, true); err != nil {
			return err
		}
		if err := run.applyCapabilityUpdates(value.CapabilityUpdates); err != nil {
			return err
		}
		run.publish(agentevent.AssistantFinal{Content: value.Content, Thinking: value.Thinking})
	case agentengine.ToolInputStarted:
		source := publicEventSource(value.Source)
		run.mu.Lock()
		run.toolSources[value.CallID] = source
		run.mu.Unlock()
		run.publish(agentevent.ToolInputStarted{
			CallID: value.CallID, ProviderCallID: value.ProviderCallID, ParentCallID: value.ParentCallID,
			Name: value.Name, Index: value.Index, Descriptor: agenttool.DecodeExecutionMetadata(value.Metadata), Source: source,
		})
	case agentengine.ToolInputDelta:
		run.publish(agentevent.ToolInputDelta{CallID: value.CallID, ProviderCallID: value.ProviderCallID, Name: value.Name, Delta: value.Delta, Source: publicEventSource(value.Source)})
	case agentengine.ToolStarted:
		if err := run.recordToolStart(value); err != nil {
			return err
		}
		if value.ExecutionAuthorized {
			run.mu.Lock()
			run.openTools[value.CallID] = agentevent.OpenToolSnapshot{
				CallID: value.CallID, Name: value.Name, RunID: run.id,
				Cycle: run.cycle, Source: publicEventSource(value.Source),
			}
			run.mu.Unlock()
			run.publish(agentevent.ToolStarted{CallID: value.CallID, ProviderCallID: value.ProviderCallID, Name: value.Name, Index: value.Index, Arguments: append(json.RawMessage(nil), value.Arguments...), Descriptor: agenttool.DecodeExecutionMetadata(value.Metadata), Source: publicEventSource(value.Source)})
		}
	case agentengine.ToolProgress:
		run.publish(agentevent.ToolProgress{CallID: value.CallID, ProviderCallID: value.ProviderCallID, Name: value.Name, Index: value.Index, Delta: value.Delta, Descriptor: agenttool.DecodeExecutionMetadata(value.Metadata), Source: publicEventSource(value.Source)})
	case agentengine.ArtifactProduced:
		var artifact agentschema.ToolArtifactRef
		if err := json.Unmarshal(value.Artifact, &artifact); err != nil {
			return err
		}
		run.mu.RLock()
		source := run.toolSources[value.CallID]
		run.mu.RUnlock()
		run.publish(agentevent.ArtifactProduced{CallID: value.CallID, Artifact: artifact, Source: source})
	case agentengine.ToolFinished:
		run.mu.Lock()
		delete(run.openTools, value.CallID)
		run.mu.Unlock()
		var projection *agentschema.ToolResult
		if len(value.Projection) != 0 {
			var decoded agentschema.ToolResult
			if json.Unmarshal(value.Projection, &decoded) == nil {
				projection = &decoded
			}
		}
		if err := run.recordToolResult(value, projection); err != nil {
			return err
		}
		run.publish(agentevent.ToolFinished{CallID: value.CallID, ProviderCallID: value.ProviderCallID, Name: value.Name, Index: value.Index, IsError: value.IsError, Result: value.Result, Descriptor: agenttool.DecodeExecutionMetadata(value.Metadata), Projection: projection, Source: publicEventSource(value.Source)})
	default:
		return fmt.Errorf("unsupported Agent engine event %T", event)
	}
	return nil
}

func (run *Run) applyCapabilityUpdates(updates []agentengine.CapabilityState) error {
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
			return agentexecution.ErrCapabilityStateConflict
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

func (run *Run) publishCapabilityUpdate(update agentengine.CapabilityState) {
	switch update.Capability {
	case agentgoal.GoalCapability:
		if update.Delete {
			run.publish(agentevent.GoalUpdated{})
		} else if state, err := agentgoal.DecodeGoalState(update.State); err == nil {
			run.publish(agentevent.GoalUpdated{State: state, Present: state.Visible()})
		}
	case agentexecution.TodoCapability:
		var state agentevent.TodoState
		if !update.Delete && json.Unmarshal(update.State, &state) == nil {
			run.publish(agentevent.TodoUpdated{State: state})
		}
	case agenthistory.CompactionCapability:
		if state, err := agenthistory.DecodeCompactionState(update.State); !update.Delete && err == nil {
			if state.Removed {
				run.publish(agentevent.CompactionRemoved{ID: state.ID, Revision: state.Revision})
			} else {
				run.publish(agentevent.CompactionCommitted{State: *agenthistory.CompactionStatePointer(state, true), Metrics: state.Metrics, Automatic: run.cycleValue() > 0})
			}
		}
	}
}
