package agent

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

func publicCleanupMetrics(metrics runstate.CleanupMetrics) CleanupMetrics {
	return CleanupMetrics{
		EstimatedTokensBefore:      metrics.EstimatedTokensBefore,
		LocalProjectedTokens:       metrics.LocalProjectedTokens,
		ObservedPromptTokens:       metrics.ObservedPromptTokens,
		EffectiveTokens:            metrics.EffectiveTokens,
		EstimatedTokensAfter:       metrics.EstimatedTokensAfter,
		ReclaimedTokens:            metrics.ReclaimedTokens,
		ContextWindowTokens:        metrics.ContextWindowTokens,
		PressureBefore:             metrics.PressureBefore,
		PressureAfter:              metrics.PressureAfter,
		BodyPressureBefore:         metrics.BodyPressureBefore,
		BodyPressureAfter:          metrics.BodyPressureAfter,
		StablePrefixTokens:         metrics.StablePrefixTokens,
		CandidateTokens:            metrics.CandidateTokens,
		CacheViableCandidateTokens: metrics.CacheViableCandidateTokens,
		SkippedBelowMinimumCount:   metrics.SkippedBelowMinimumCount,
		SkippedWarmSuffixCount:     metrics.SkippedWarmSuffixCount,
		EagerCandidateCount:        metrics.EagerCandidateCount,
		EagerSelectedCount:         metrics.EagerSelectedCount,
		SupersededCandidateCount:   metrics.SupersededCandidateCount,
		DiscardableCandidateCount:  metrics.DiscardableCandidateCount,
		MinimumCleanupTokens:       metrics.MinimumCleanupTokens,
		ProtectedResults:           metrics.ProtectedResults,
		EarliestChanged:            metrics.EarliestChanged,
		WarmSuffixTokens:           metrics.WarmSuffixTokens,
		PlaceholderTokens:          metrics.PlaceholderTokens,
		ReplacementCount:           metrics.ReplacementCount,
		EagerOnly:                  metrics.EagerOnly,
		PressureScope:              metrics.PressureScope,
		ProviderCacheState:         metrics.ProviderCacheState,
		ExecutionMode:              metrics.ExecutionMode,
		RendererVersion:            metrics.RendererVersion,
	}
}

func publicCompactionMetrics(metrics runstate.CompactionMetrics) CompactionMetrics {
	return CompactionMetrics{
		EstimatedTokensBefore:     metrics.EstimatedTokensBefore,
		ObservedPromptTokens:      metrics.ObservedPromptTokens,
		ObservedEstimateTokens:    metrics.ObservedEstimateTokens,
		EstimatedTokensAfter:      metrics.EstimatedTokensAfter,
		ProjectedTokensBefore:     metrics.ProjectedTokensBefore,
		ProjectedTokensAfter:      metrics.ProjectedTokensAfter,
		ReservedTokens:            metrics.ReservedTokens,
		ContextWindowTokens:       metrics.ContextWindowTokens,
		Threshold:                 metrics.Threshold,
		RecoveryBand:              metrics.RecoveryBand,
		RecoveryTargetTokens:      metrics.RecoveryTargetTokens,
		RecoveryBandMet:           metrics.RecoveryBandMet,
		Degraded:                  metrics.Degraded,
		StablePrefixTokens:        metrics.StablePrefixTokens,
		SourceMessageCount:        metrics.SourceMessageCount,
		MessageCountBefore:        metrics.MessageCountBefore,
		MessageCountAfter:         metrics.MessageCountAfter,
		CacheExpectedPrefixTokens: metrics.CacheExpectedPrefixTokens,
		CacheReadTokens:           metrics.CacheReadTokens,
		CandidateFingerprint:      metrics.CandidateFingerprint,
		CandidateGeneration:       metrics.CandidateGeneration,
	}
}

type trackedToolCall struct {
	runID          string
	providerCallID string
	name           string
	index          int
	descriptor     *ToolDescriptor
	source         EventSource
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
				mapped.Payload = CompactionCommitted{State: state, Automatic: payload.Cycle > 0}
			}
		case cleanupCapability:
			if payload.Deleted {
				return Event{}, false
			}
			state, err := decodeCleanupState(payload.State)
			if err != nil || state.Removed {
				return Event{}, false
			}
			mapped.Payload = CleanupCommitted{State: state, Automatic: payload.Cycle > 0}
		case clearCapability:
			if payload.Deleted {
				return Event{}, false
			}
			state, err := decodeClearState(payload.State)
			if err != nil {
				return Event{}, false
			}
			mapped.Payload = SessionCleared{Revision: state.Revision}
		case transcriptSyncCapability:
			if payload.Deleted {
				return Event{}, false
			}
			state, _, err := transcriptSyncStateFrom(map[string]json.RawMessage{transcriptSyncCapability: payload.State})
			if err != nil {
				return Event{}, false
			}
			mapped.Payload = TranscriptSynchronized{State: state}
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
	case runstate.CompactionStartedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = CompactionStarted{
			ID: payload.ID, Automatic: payload.Automatic, Metrics: publicCompactionMetrics(payload.Metrics),
		}
	case runstate.CleanupStartedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = CleanupStarted{ID: payload.ID, Reason: payload.Reason, Automatic: payload.Automatic, Transient: payload.Transient, Metrics: publicCleanupMetrics(payload.Metrics)}
	case runstate.ContextNormalizedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = ContextNormalized{
			RepairCount: payload.RepairCount, MessagesBefore: payload.MessagesBefore, MessagesAfter: payload.MessagesAfter,
		}
	case runstate.CleanupCompletedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = CleanupCompleted{ID: payload.ID, Reason: payload.Reason, Automatic: payload.Automatic, Transient: payload.Transient, Metrics: publicCleanupMetrics(payload.Metrics)}
	case runstate.CleanupFailedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = CleanupFailed{ID: payload.ID, Reason: payload.Reason, Automatic: payload.Automatic, Metrics: publicCleanupMetrics(payload.Metrics)}
	case runstate.CleanupSkippedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = CleanupSkipped{ID: payload.ID, Reason: payload.Reason, Automatic: payload.Automatic, Metrics: publicCleanupMetrics(payload.Metrics)}
	case runstate.CompactionFailedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = CompactionFailed{
			ID: payload.ID, Reason: payload.Reason, Automatic: payload.Automatic,
			ConsecutiveFailures: payload.ConsecutiveFailures, FailureFuseOpen: payload.FailureFuseOpen,
			Metrics: publicCompactionMetrics(payload.Metrics),
		}
	case runstate.CompactionSkippedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = CompactionSkipped{
			ID: payload.ID, Reason: payload.Reason, Automatic: payload.Automatic,
			ConsecutiveFailures: payload.ConsecutiveFailures, FailureFuseOpen: payload.FailureFuseOpen,
			Metrics: publicCompactionMetrics(payload.Metrics),
		}
	case runstate.CycleStartedEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		mapped.Payload = RunStarted{
			Cycle: payload.Cycle, CommandID: string(payload.CommandID), Delivery: payload.Delivery,
		}
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
	case runstate.NestedEventEvent:
		if string(payload.OperationID) != runID {
			return Event{}, false
		}
		nested, err := decodeNestedEvent(nestedEventRecord{
			Source: publicEventSource(payload.Source), SessionID: payload.SessionID,
			ChildCursor: Cursor(payload.ChildCursor), ChildDurability: EventDurability(payload.ChildDurability),
			ChildRunID: payload.ChildRunID, PayloadType: payload.PayloadType,
			Payload: append(json.RawMessage(nil), payload.Payload...),
		})
		if err != nil {
			return Event{}, false
		}
		mapped.Payload = nested
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
			Name: payload.Name, Index: payload.Index, Descriptor: decodeToolDescriptorMetadata(payload.Metadata),
			Source: publicEventSource(payload.Source),
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
		descriptor := decodeToolDescriptorMetadata(payload.Metadata)
		trackedCalls[payload.CallID] = trackedToolCall{
			runID: runID, providerCallID: payload.ProviderCallID, name: payload.Name,
			index: payload.Index, descriptor: descriptor, source: source,
		}
		mapped.Payload = ToolStarted{
			CallID: payload.CallID, ProviderCallID: payload.ProviderCallID, Name: payload.Name, Index: payload.Index,
			Arguments: append(json.RawMessage(nil), payload.Arguments...), Descriptor: descriptor, Source: source,
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
		descriptor := decodeToolDescriptorMetadata(payload.Metadata)
		if descriptor == nil {
			descriptor = tracked.descriptor
		}
		providerCallID := payload.ProviderCallID
		if providerCallID == "" {
			providerCallID = tracked.providerCallID
		}
		mapped.Payload = ToolFinished{
			CallID: payload.CallID, ProviderCallID: providerCallID, Name: payload.Name, Index: payload.Index,
			IsError: payload.IsError, Result: payload.Result, Descriptor: descriptor,
			Projection: projection, Source: publicEventSource(payload.Source),
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
		descriptor := decodeToolDescriptorMetadata(payload.Metadata)
		if descriptor == nil {
			descriptor = tracked.descriptor
		}
		providerCallID := payload.ProviderCallID
		if providerCallID == "" {
			providerCallID = tracked.providerCallID
		}
		name := payload.Name
		if name == "" {
			name = tracked.name
		}
		mapped.Payload = ToolProgress{
			CallID: payload.CallID, ProviderCallID: providerCallID, Name: name, Index: payload.Index,
			Delta: payload.Delta, Descriptor: descriptor, Source: source,
		}
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

func decodeToolDescriptorMetadata(metadata json.RawMessage) *ToolDescriptor {
	if len(metadata) == 0 {
		return nil
	}
	var descriptor ToolDescriptor
	if err := json.Unmarshal(metadata, &descriptor); err != nil || descriptor.Execution == "" {
		return nil
	}
	return &descriptor
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

func mapObservation(key SessionKey, observation runstate.Observation, projectionTextMaxBytes int) Observation {
	events := make(chan Event, publicRunEventBuffer)
	errorsChannel := make(chan error, 1)
	safeGo(func() {
		defer close(events)
		defer close(errorsChannel)
		state := sessionEventMappingState{
			trackedCalls: make(map[string]trackedToolCall), commandRuns: make(map[string]string),
		}
		if observation.Snapshot.ActiveCommandID != "" && observation.Snapshot.ActiveOperation != "" {
			state.commandRuns[string(observation.Snapshot.ActiveCommandID)] = string(observation.Snapshot.ActiveOperation)
		}
		for _, queued := range observation.Snapshot.Queue {
			state.commandRuns[string(queued.CommandID)] = string(queued.OperationID)
		}
		for _, settled := range observation.Snapshot.RecentOperations {
			state.commandRuns[string(settled.CommandID)] = string(settled.OperationID)
		}
		for _, call := range observation.Snapshot.OpenToolCalls {
			state.trackedCalls[call.CallID] = trackedToolCall{
				runID: string(call.OperationID), source: publicEventSource(call.Source),
			}
		}
		runtimeEvents := observation.Events
		runtimeErrors := observation.Errors
		drops := eventDropState{}
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
					publishLatestEvent(events, mapped, &drops)
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
	return Observation{Snapshot: mapSessionSnapshot(key, observation.Snapshot, projectionTextMaxBytes), Events: events, Errors: errorsChannel}
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
	case runstate.NestedEventEvent:
		return string(payload.OperationID)
	case runstate.ModelCompletedEvent:
		return string(payload.OperationID)
	case runstate.AssistantMessageCommittedEvent:
		return string(payload.Message.Operation)
	case runstate.ToolCallStartedEvent:
		return string(payload.Call.OperationID)
	case runstate.CapabilityStateCommittedEvent:
		return string(payload.OperationID)
	case runstate.CompactionStartedEvent:
		return string(payload.OperationID)
	case runstate.CleanupStartedEvent:
		return string(payload.OperationID)
	case runstate.ContextNormalizedEvent:
		return string(payload.OperationID)
	case runstate.CleanupCompletedEvent:
		return string(payload.OperationID)
	case runstate.CleanupFailedEvent:
		return string(payload.OperationID)
	case runstate.CleanupSkippedEvent:
		return string(payload.OperationID)
	case runstate.CompactionFailedEvent:
		return string(payload.OperationID)
	case runstate.CompactionSkippedEvent:
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
	return EventSource{
		Name: source.Name, Path: append([]string(nil), source.Path...),
		InvocationID: source.InvocationID, InvocationType: source.InvocationType,
	}
}

func eventSourceEmpty(source EventSource) bool {
	return source.Name == "" && len(source.Path) == 0 && source.InvocationID == "" && source.InvocationType == ""
}

func mapSessionSnapshot(key SessionKey, snapshot runstate.StateSnapshot, projectionTextMaxBytes int) SessionSnapshot {
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
		text, truncated := boundPublicProjectionText(queued.Input.Text, projectionTextMaxBytes)
		result.QueuedRuns = append(result.QueuedRuns, QueuedRunSnapshot{
			ID: string(queued.OperationID), CommandID: string(queued.CommandID),
			ReceiptCursor: Cursor(queued.ReceiptCursor), Delivery: publicRecoveryDelivery(queued.Delivery),
			Text: text, TextTruncated: truncated,
			InterruptRequested: queued.CommandID == snapshot.PreemptQueuedCommandID,
		})
	}
	for _, tool := range snapshot.OpenToolCalls {
		result.OpenTools = append(result.OpenTools, OpenToolSnapshot{
			CallID: tool.CallID, Name: tool.Name, RunID: string(tool.OperationID), Cycle: tool.Cycle,
			Source: publicEventSource(tool.Source),
		})
	}
	for _, operation := range snapshot.RecentOperations {
		result.RecentRuns = append(result.RecentRuns, RunSummary{
			ID: string(operation.OperationID), CommandID: string(operation.CommandID),
			CommandFingerprint: operation.CommandFingerprint, ReceiptCursor: Cursor(operation.ReceiptCursor),
			Status: mapResultStatus(operation.Status), Reason: operation.Reason, ReasonTruncated: operation.ReasonTruncated,
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
	var compaction CompactionState
	compactionPresent := false
	if encoded, ok := snapshot.CapabilityStates[compactionCapability]; ok {
		if state, err := decodeCompactionState(encoded); err == nil &&
			(!clearPresent || state.Revision > clearState.CompactionRevisionAtClear) {
			compaction, compactionPresent = state, true
			if !state.Removed {
				result.Compaction = &state
			}
		}
	}
	if encoded, ok := snapshot.CapabilityStates[cleanupCapability]; ok {
		if state, err := decodeCleanupState(encoded); err == nil && !state.Removed &&
			(!clearPresent || state.Revision > clearState.CleanupRevisionAtClear) {
			if visible, present := cleanupAfterCompaction(state, true, compaction, compactionPresent); present {
				result.Cleanup = cloneCleanupState(&visible)
			}
		}
	}
	if state, present, err := transcriptSyncStateFrom(snapshot.CapabilityStates); err == nil && present {
		result.TranscriptSync = &state
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

func boundPublicProjectionText(value string, limit int) (string, bool) {
	limit = normalizedProjectionTextMaxBytes(limit)
	if len(value) <= limit {
		return value, false
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value, true
}
