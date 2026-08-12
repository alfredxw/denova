package agent

import (
	"encoding/json"
	"fmt"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

type trackedToolCall struct {
	runID  string
	source EventSource
}

func mapRunEvent(event runstate.Event, runID, commandID string, trackedCalls map[string]trackedToolCall) (Event, bool) {
	mapped := Event{Cursor: Cursor(event.Cursor), RunID: runID, Durability: DurableEvent}
	if event.Durability == runstate.EventEphemeral {
		mapped.Durability = EphemeralEvent
	}
	switch payload := event.Payload.(type) {
	case runstate.CommandAcceptedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = RunAccepted{CommandID: string(payload.CommandID)}
	case runstate.QueueCancelledEvent:
		if string(payload.CommandID) != commandID {
			return Event{}, false
		}
		mapped.Payload = RunSettled{Status: ResultAborted, Reason: payload.Reason}
	case runstate.CapabilityStateCommittedEvent:
		if runID != "" && string(payload.OperationID) != runID {
			return Event{}, false
		}
		switch payload.Capability {
		case goalCapability:
			updated := GoalUpdated{Present: !payload.Deleted}
			if !payload.Deleted {
				state, err := decodeGoalState(payload.State)
				if err != nil {
					return Event{}, false
				}
				updated.State = state
			}
			mapped.Payload = updated
		case compactionCapability:
			if payload.Deleted {
				return Event{}, false
			}
			state, err := decodeCompactionState(payload.State)
			if err != nil {
				return Event{}, false
			}
			if state.Removed {
				mapped.Payload = CompactionRemoved{ID: state.ID, Revision: state.Revision}
			} else {
				mapped.Payload = CompactionCommitted{State: state}
			}
		case clearCapability:
			if payload.Deleted {
				return Event{}, false
			}
			state, err := decodeClearState(payload.State)
			if err != nil {
				return Event{}, false
			}
			mapped.Payload = SessionCleared{Revision: state.Revision}
		case TodoCapability:
			if payload.Deleted {
				return Event{}, false
			}
			var state TodoState
			if err := json.Unmarshal(payload.State, &state); err != nil || state.Revision == 0 {
				return Event{}, false
			}
			mapped.Payload = TodoUpdated{State: state}
		default:
			return Event{}, false
		}
	case runstate.InteractionRequestedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		var request InteractionRequest
		if err := json.Unmarshal(payload.Request, &request); err != nil || request.ID != payload.ID {
			return Event{}, false
		}
		mapped.Payload = InteractionRequested{Request: request}
	case runstate.InteractionResolvedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		resolution, err := decodeInteractionResolution(payload.Response)
		if err != nil {
			return Event{}, false
		}
		mapped.Payload = InteractionResolved{ID: payload.ID, Resolution: resolution}
	case runstate.InteractionRecoveryResumedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = RecoveryResumed{}
	case runstate.OperationStartedEvent:
		if payload.Structural == nil {
			return Event{}, false
		}
		mapped.Payload = CompactionStarted{
			ID:     payload.Structural.Ref.CompactionID,
			Remove: payload.Structural.Kind == runstate.StructuralRemoveCompaction,
		}
	case runstate.CycleStartedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = RunStarted{Cycle: payload.Cycle}
	case runstate.AssistantDeltaEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = AssistantDelta{Source: publicEventSource(payload.Source), Delta: payload.Delta, DisplayOnly: payload.DisplayOnly}
	case runstate.ThinkingDeltaEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = ThinkingDelta{Source: publicEventSource(payload.Source), Delta: payload.Delta, DisplayOnly: payload.DisplayOnly}
	case runstate.ModelCompletedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = ModelCompleted{
			Usage: TokenUsage{
				PromptTokens:       payload.Usage.PromptTokens,
				PromptTokenDetails: PromptTokenDetails{CachedTokens: payload.Usage.CachedPromptTokens},
				CompletionTokens:   payload.Usage.CompletionTokens, TotalTokens: payload.Usage.TotalTokens,
				CompletionTokensDetails: CompletionTokensDetails{ReasoningTokens: payload.Usage.ReasoningTokens},
			},
			FinishReason: payload.FinishReason, RequestedTools: append([]string(nil), payload.RequestedTools...),
			Source: publicEventSource(payload.Source),
		}
	case runstate.AssistantMessageCommittedEvent:
		if string(payload.Message.Operation) != runID {
			return Event{}, false
		}
		mapped.Payload = AssistantFinal{Content: payload.Message.Content, Thinking: payload.Message.Thinking}
	case runstate.ToolCallStartedEvent, runstate.ToolCallFinishedEvent:
		// Public start/output events are emitted from the paired live projections
		// below so arguments and bounded display results are never misleadingly
		// represented as empty durable data.
		return Event{}, false
	case runstate.ToolInputStartedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = ToolInputStarted{
			CallID: payload.CallID, ProviderCallID: payload.ProviderCallID,
			Name: payload.Name, Source: publicEventSource(payload.Source),
		}
	case runstate.ToolInputDeltaEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = ToolInputDelta{
			CallID: payload.CallID, ProviderCallID: payload.ProviderCallID,
			Name: payload.Name, Delta: payload.Delta, Source: publicEventSource(payload.Source),
		}
	case runstate.ToolStartedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		source := publicEventSource(payload.Source)
		trackedCalls[payload.CallID] = trackedToolCall{runID: runID, source: source}
		mapped.Payload = ToolStarted{
			CallID: payload.CallID, Name: payload.Name,
			Arguments: append(json.RawMessage(nil), payload.Arguments...), Source: source,
		}
	case runstate.ToolOutputEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		tracked := trackedCalls[payload.CallID]
		delete(trackedCalls, payload.CallID)
		var projection *ToolResult
		if len(payload.Projection) != 0 {
			var decoded ToolResult
			if json.Unmarshal(payload.Projection, &decoded) == nil {
				projection = &decoded
			}
		}
		mapped.Payload = ToolFinished{
			CallID: payload.CallID, Name: payload.Name, IsError: payload.IsError,
			Result: payload.Result, Projection: projection, Source: publicEventSource(payload.Source),
		}
		if eventSourceEmpty(mapped.Payload.(ToolFinished).Source) {
			finished := mapped.Payload.(ToolFinished)
			finished.Source = tracked.source
			mapped.Payload = finished
		}
	case runstate.ToolProgressEvent:
		tracked, ok := trackedCalls[payload.CallID]
		if !ok || (runID != "" && tracked.runID != runID) {
			return Event{}, false
		}
		mapped.RunID = tracked.runID
		source := publicEventSource(payload.Source)
		if eventSourceEmpty(source) {
			source = tracked.source
		}
		mapped.Payload = ToolProgress{CallID: payload.CallID, Delta: payload.Delta, Source: source}
	case runstate.ArtifactProducedEvent:
		tracked, ok := trackedCalls[payload.CallID]
		if !ok || (runID != "" && tracked.runID != runID) {
			return Event{}, false
		}
		var artifact ToolArtifactRef
		if err := json.Unmarshal(payload.Artifact, &artifact); err != nil {
			return Event{}, false
		}
		mapped.RunID = tracked.runID
		mapped.Payload = ArtifactProduced{CallID: payload.CallID, Artifact: artifact, Source: tracked.source}
	case runstate.OperationRecoveryPausedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = RecoveryRequired{Reason: payload.Reason}
	case runstate.InputMaterializationRecoveryResumedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = RecoveryResumed{}
	case runstate.ByteBudgetExceededEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = ContextLimitReached{Scope: string(payload.Scope), Limit: payload.Limit}
	case runstate.OperationSettledEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = RunSettled{Status: mapResultStatus(payload.Status), Reason: payload.Reason}
	case runstate.OperationInterruptedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = RunSettled{Status: ResultFailed, Reason: payload.Reason}
	default:
		return Event{}, false
	}
	return mapped, true
}

func mapResultStatus(status runstate.OperationStatus) ResultStatus {
	switch status {
	case runstate.OperationSucceeded:
		return ResultCompleted
	case runstate.OperationAborted:
		return ResultAborted
	case runstate.OperationIncomplete:
		return ResultIncomplete
	case runstate.OperationFailed, runstate.OperationInterrupted:
		return ResultFailed
	default:
		return ResultFailed
	}
}

func mapObservation(key SessionKey, observation runstate.Observation) Observation {
	events := make(chan Event, publicRunEventBuffer)
	errorsChannel := make(chan error, 1)
	safeGo(func() {
		defer close(events)
		defer close(errorsChannel)
		state := sessionEventMappingState{
			trackedCalls: make(map[string]trackedToolCall), commandRuns: make(map[string]string),
		}
		for _, call := range observation.Snapshot.OpenToolCalls {
			state.trackedCalls[call.CallID] = trackedToolCall{
				runID: string(call.OperationID), source: publicEventSource(call.Source),
			}
		}
		runtimeEvents := observation.Events
		runtimeErrors := observation.Errors
		for {
			if runtimeEvents == nil && runtimeErrors == nil {
				return
			}
			select {
			case event, ok := <-runtimeEvents:
				if !ok {
					runtimeEvents = nil
					continue
				}
				mapped, ok := mapSessionEvent(event, &state)
				if ok {
					events <- mapped
				}
			case err, ok := <-runtimeErrors:
				if !ok {
					runtimeErrors = nil
					continue
				}
				if err != nil {
					errorsChannel <- err
				}
			}
		}
	}, func(err error) {
		errorsChannel <- fmt.Errorf("map Agent Observation: %w", err)
	})
	return Observation{Snapshot: mapSessionSnapshot(key, observation.Snapshot), Events: events, Errors: errorsChannel}
}

type sessionEventMappingState struct {
	trackedCalls map[string]trackedToolCall
	commandRuns  map[string]string
}

func mapSessionEvent(event runstate.Event, state *sessionEventMappingState) (Event, bool) {
	runID := runtimeEventRunID(event.Payload)
	commandID := ""
	switch payload := event.Payload.(type) {
	case runstate.CommandAcceptedEvent:
		commandID = string(payload.CommandID)
		state.commandRuns[commandID] = string(payload.OperationID)
	case runstate.QueueCancelledEvent:
		commandID = string(payload.CommandID)
		runID = state.commandRuns[commandID]
	}
	return mapRunEvent(event, runID, commandID, state.trackedCalls)
}

func runtimeEventRunID(payload runstate.EventPayload) string {
	switch payload := payload.(type) {
	case runstate.CommandAcceptedEvent:
		return string(payload.OperationID)
	case runstate.OperationStartedEvent:
		return string(payload.OperationID)
	case runstate.CycleStartedEvent:
		return string(payload.OperationID)
	case runstate.AssistantDeltaEvent:
		return string(payload.OperationID)
	case runstate.ThinkingDeltaEvent:
		return string(payload.OperationID)
	case runstate.ModelCompletedEvent:
		return string(payload.OperationID)
	case runstate.AssistantMessageCommittedEvent:
		return string(payload.Message.Operation)
	case runstate.ToolCallStartedEvent:
		return string(payload.Call.OperationID)
	case runstate.CapabilityStateCommittedEvent:
		return string(payload.OperationID)
	case runstate.InteractionRequestedEvent:
		return string(payload.OperationID)
	case runstate.InteractionResolvedEvent:
		return string(payload.OperationID)
	case runstate.InteractionRecoveryResumedEvent:
		return string(payload.OperationID)
	case runstate.ToolProgressEvent:
		return ""
	case runstate.ToolInputStartedEvent:
		return string(payload.OperationID)
	case runstate.ToolInputDeltaEvent:
		return string(payload.OperationID)
	case runstate.ToolStartedEvent:
		return string(payload.OperationID)
	case runstate.ToolOutputEvent:
		return string(payload.OperationID)
	case runstate.ToolCallFinishedEvent:
		return ""
	case runstate.ArtifactProducedEvent:
		return string(payload.OperationID)
	case runstate.OperationRecoveryPausedEvent:
		return string(payload.OperationID)
	case runstate.InputMaterializationRecoveryResumedEvent:
		return string(payload.OperationID)
	case runstate.ByteBudgetExceededEvent:
		return string(payload.OperationID)
	case runstate.OperationSettledEvent:
		return string(payload.OperationID)
	case runstate.OperationInterruptedEvent:
		return string(payload.OperationID)
	default:
		return ""
	}
}

func publicEventSource(source runstate.EventSource) EventSource {
	return EventSource{Name: source.Name, Path: append([]string(nil), source.Path...)}
}

func eventSourceEmpty(source EventSource) bool {
	return source.Name == "" && len(source.Path) == 0
}

func mapSessionSnapshot(key SessionKey, snapshot runstate.StateSnapshot) SessionSnapshot {
	result := SessionSnapshot{
		Key: key, Cursor: Cursor(snapshot.Cursor), RetentionStart: Cursor(snapshot.TimelineStartCursor),
		ActiveRunID: string(snapshot.ActiveOperation), ActiveCommandID: string(snapshot.ActiveCommandID),
		ActiveCommandFingerprint: snapshot.ActiveCommandFingerprint,
		ActiveReceiptCursor:      Cursor(snapshot.ActiveReceiptCursor), ActiveCycle: snapshot.ActiveCycle,
		RecoveryPending: snapshot.RecoveryPending, RecoveryPaused: snapshot.RecoveryPaused,
		MessagesTruncated: snapshot.MessagesTruncated,
		ActiveOutput: ActiveOutputSnapshot{
			Content: snapshot.ActiveOutput.Content, Thinking: snapshot.ActiveOutput.Thinking,
			ContentTruncated:  snapshot.ActiveOutput.ContentTruncated,
			ThinkingTruncated: snapshot.ActiveOutput.ThinkingTruncated,
			RehydrateRequired: snapshot.ActiveOutput.RehydrateRequired,
		},
	}
	result.RecoveryActions = publicRecoveryActions(recoveryCandidatesFromState(snapshot))
	for _, queued := range snapshot.Queue {
		if queued.Autonomous {
			continue
		}
		result.QueuedRuns = append(result.QueuedRuns, QueuedRunSnapshot{
			ID: string(queued.OperationID), CommandID: string(queued.CommandID),
			ReceiptCursor: Cursor(queued.ReceiptCursor), Delivery: publicRecoveryDelivery(queued.Delivery),
		})
	}
	for _, tool := range snapshot.OpenToolCalls {
		result.OpenTools = append(result.OpenTools, ToolStarted{
			CallID: tool.CallID, Name: tool.Name, Source: publicEventSource(tool.Source),
		})
	}
	for _, operation := range snapshot.RecentOperations {
		result.RecentRuns = append(result.RecentRuns, RunSummary{
			ID: string(operation.OperationID), CommandID: string(operation.CommandID),
			CommandFingerprint: operation.CommandFingerprint, ReceiptCursor: Cursor(operation.ReceiptCursor),
			Status: mapResultStatus(operation.Status), Reason: operation.Reason,
		})
	}
	if encoded, ok := snapshot.CapabilityStates[goalCapability]; ok {
		if state, err := decodeGoalState(encoded); err == nil && state.Visible() {
			result.Goal = &state
		}
	}
	if encoded, ok := snapshot.CapabilityStates[TodoCapability]; ok {
		var state TodoState
		if err := json.Unmarshal(encoded, &state); err == nil && state.Revision > 0 {
			result.Todo = &state
		}
	}
	clearState, clearPresent, _ := clearStateFrom(snapshot.CapabilityStates)
	if clearPresent {
		result.ClearRevision = clearState.Revision
	}
	if encoded, ok := snapshot.CapabilityStates[compactionCapability]; ok {
		if state, err := decodeCompactionState(encoded); err == nil && !state.Removed &&
			(!clearPresent || state.Revision > clearState.CompactionRevisionAtClear) {
			result.Compaction = &state
		}
	}
	for _, pending := range snapshot.Interactions {
		if pending.Resolved {
			continue
		}
		var request InteractionRequest
		if err := json.Unmarshal(pending.Request, &request); err == nil && request.ID == pending.ID {
			result.PendingInteractions = append(result.PendingInteractions, request)
		}
	}
	return result
}
