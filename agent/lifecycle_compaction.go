package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
)

func (session *Session) Compact(ctx context.Context, request CompactionRequest) (CompactionResult, error) {
	commandID := strings.TrimSpace(request.IdempotencyKey)
	if commandID == "" {
		commandID = newPublicID("compact")
	}
	preparation, release, err := session.prepareStructuralDefinition(ctx, runstate.CommandID(commandID))
	if err != nil {
		return CompactionResult{}, err
	}
	defer release()
	if preparation.prepared.definition.Compaction == nil {
		return CompactionResult{}, ErrCapabilityUnsupported
	}
	current, present := preparation.compaction, preparation.compactionPresent
	if request.ExpectedID != "" && (!present || current.ID != request.ExpectedID) ||
		request.ExpectedRevision != 0 && (!present || current.Revision != request.ExpectedRevision) {
		return CompactionResult{}, ErrDefinitionMismatch
	}
	forkCtx, err := contextWithProviderCacheKey(ctx, session.key, session.agent.cacheKeys)
	if err != nil {
		return CompactionResult{}, err
	}
	modelSnapshot, err := prepareStructuralCompactionSnapshot(
		forkCtx, preparation.prepared,
		SessionView{Key: session.key, Revision: uint64(preparation.cursor)},
		structuralDefinitionRun(runstate.CommandID(commandID)),
		preparation.transcript.Messages, preparation.cleanup, preparation.cleanupPresent,
		current, present,
	)
	if err != nil {
		return CompactionResult{}, err
	}
	modelFingerprint, err := modelRequestSnapshotFingerprint(modelSnapshot)
	if err != nil {
		return CompactionResult{}, err
	}
	envelope := compactionCommandEnvelope{
		Version: compactionCommandVersion, DefinitionKey: preparation.prepared.definitionKey,
		BehaviorKey:             preparation.prepared.behaviorKey,
		MaterializedFingerprint: preparation.prepared.materializedFingerprint,
		ModelRequestFingerprint: modelFingerprint,
		Manager:                 preparation.prepared.definition.Compaction.Identity(), Compact: &request,
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return CompactionResult{}, err
	}
	operationID := runstate.OperationID(newPublicID("compaction"))
	ref, err := session.compactionRef(runstate.Cursor(preparation.cursor), encoded, "create bounded conversation checkpoint", "")
	if err != nil {
		return CompactionResult{}, err
	}
	session.publishSessionEvent(CompactionStarted{ID: string(operationID)})
	if err := session.executeStructural(ctx, preparation, runstate.StructuralOperationSnapshot{
		Binding: session.binding.Clone(), CommandID: runstate.CommandID(commandID), OperationID: operationID,
		Cycle: 1, Kind: runstate.StructuralCompactContext, Ref: ref, ContextCursor: runstate.Cursor(preparation.cursor),
	}); err != nil {
		return CompactionResult{}, err
	}
	updated, exists, err := session.compactionState(ctx)
	if err != nil {
		return CompactionResult{}, err
	}
	return CompactionResult{Changed: exists && (!present || updated.Revision != current.Revision), State: updated}, nil
}

func (session *Session) RemoveCompaction(ctx context.Context, request CompactionRemoveRequest) (bool, error) {
	commandID := strings.TrimSpace(request.IdempotencyKey)
	if commandID == "" {
		commandID = newPublicID("remove-compaction")
	}
	preparation, release, err := session.prepareStructuralDefinition(ctx, runstate.CommandID(commandID))
	if err != nil {
		return false, err
	}
	defer release()
	if preparation.prepared.definition.Compaction == nil {
		return false, ErrCapabilityUnsupported
	}
	current, present := preparation.compaction, preparation.compactionPresent
	if !present || current.Removed {
		return false, nil
	}
	if strings.TrimSpace(request.ID) == "" {
		request.ID = current.ID
	}
	if request.ID != current.ID || request.ExpectedRevision != 0 && request.ExpectedRevision != current.Revision {
		return false, ErrDefinitionMismatch
	}
	envelope := compactionCommandEnvelope{
		Version: compactionCommandVersion, DefinitionKey: preparation.prepared.definitionKey,
		BehaviorKey:             preparation.prepared.behaviorKey,
		MaterializedFingerprint: preparation.prepared.materializedFingerprint,
		Manager:                 preparation.prepared.definition.Compaction.Identity(), Remove: &request,
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return false, err
	}
	operationID := runstate.OperationID(newPublicID("compaction"))
	ref, err := session.compactionRef(runstate.Cursor(preparation.cursor), encoded, "restore raw conversation history", current.ID)
	if err != nil {
		return false, err
	}
	session.publishSessionEvent(CompactionStarted{ID: current.ID, Remove: true})
	if err := session.executeStructural(ctx, preparation, runstate.StructuralOperationSnapshot{
		Binding: session.binding.Clone(), CommandID: runstate.CommandID(commandID), OperationID: operationID,
		Cycle: 1, Kind: runstate.StructuralRemoveCompaction, Ref: ref, ContextCursor: runstate.Cursor(preparation.cursor),
	}); err != nil {
		return false, err
	}
	updated, exists, err := session.compactionState(ctx)
	return exists && updated.Removed && updated.Revision > current.Revision, err
}

type structuralDefinitionPreparation struct {
	prepared          preparedDefinition
	transcript        engineTranscript
	cursor            uint64
	state             json.RawMessage
	capabilities      map[string]json.RawMessage
	compaction        CompactionState
	compactionPresent bool
	cleanup           CleanupState
	cleanupPresent    bool
}

func (session *Session) prepareStructuralDefinition(ctx context.Context, commandID runstate.CommandID) (structuralDefinitionPreparation, func(), error) {
	if err := session.usable(); err != nil {
		return structuralDefinitionPreparation{}, nil, err
	}
	session.mu.Lock()
	if session.active != nil || session.maintenance {
		session.mu.Unlock()
		return structuralDefinitionPreparation{}, nil, ErrSessionBusy
	}
	session.maintenance = true
	state := append(json.RawMessage(nil), session.engineState...)
	capabilities := cloneRawStateMap(session.capabilities)
	cursor := uint64(session.revision)
	session.mu.Unlock()
	release := func() {
		session.mu.Lock()
		session.maintenance = false
		session.mu.Unlock()
	}
	failed := func(err error) (structuralDefinitionPreparation, func(), error) {
		release()
		return structuralDefinitionPreparation{}, nil, err
	}
	transcript, err := decodeEngineTranscript(state)
	if err != nil {
		return failed(err)
	}
	clearState, clearPresent, err := applyClearToTranscript(&transcript, capabilities)
	if err != nil {
		return failed(err)
	}
	current, present, _, err := compactionStateFrom(capabilities)
	if err != nil {
		return failed(err)
	}
	current, present = clearCompaction(current, present, clearState, clearPresent)
	cleanup, cleanupPresent, _, err := cleanupStateFrom(capabilities)
	if err != nil {
		return failed(err)
	}
	cleanup, cleanupPresent = clearCleanup(cleanup, cleanupPresent, clearState, clearPresent)
	cleanup, cleanupPresent = cleanupAfterCompaction(cleanup, cleanupPresent, current, present)
	prepared, err := prepareDefinition(ctx, session.agent.source, PrepareRequest{
		Session: SessionView{Key: session.key, Revision: cursor}, Run: structuralDefinitionRun(commandID),
		Reason: TurnReasonStructural, HostData: cloneHostData(transcript.HostData),
		Compaction: compactionStatePointer(current, present), Cleanup: cloneCleanupStateIfPresent(cleanup, cleanupPresent),
	})
	if err != nil {
		return failed(err)
	}
	materialized, err := materializedDefinitionFingerprint(prepared)
	if err != nil {
		return failed(err)
	}
	prepared.materializedFingerprint = materialized
	prepared.contextState = cloneContextStateSnapshot(transcript.ContextState)
	return structuralDefinitionPreparation{
		prepared: prepared, transcript: transcript, cursor: cursor, state: state, capabilities: capabilities,
		compaction: current, compactionPresent: present, cleanup: cleanup, cleanupPresent: cleanupPresent,
	}, release, nil
}

func (session *Session) executeStructural(ctx context.Context, preparation structuralDefinitionPreparation, snapshot runstate.StructuralOperationSnapshot) error {
	engine, ok := session.engine.(runstate.StructuralEngine)
	if !ok {
		return ErrCapabilityUnsupported
	}
	result, err := engine.RunStructural(ctx, runstate.StructuralEngineRequest{
		Binding: session.binding.Clone(), Snapshot: snapshot, State: preparation.state,
		Capabilities: preparation.capabilities, Controls: make(chan runstate.EngineControl),
	}, func(event runstate.EngineEvent) error {
		update, ok := event.(runstate.EngineCapabilityState)
		if !ok {
			return fmt.Errorf("unsupported structural Agent event %T", event)
		}
		session.mu.Lock()
		if update.Delete {
			delete(session.capabilities, update.Capability)
		} else {
			session.capabilities[update.Capability] = append(json.RawMessage(nil), update.State...)
		}
		persistErr := session.persistCapabilitiesLocked(context.Background())
		session.mu.Unlock()
		if persistErr != nil {
			return persistErr
		}
		if update.Capability == compactionCapability && !update.Delete {
			state, decodeErr := decodeCompactionState(update.State)
			if decodeErr != nil {
				return decodeErr
			}
			if state.Removed {
				session.publishSessionEvent(CompactionRemoved{ID: state.ID, Revision: state.Revision})
			} else {
				session.publishSessionEvent(CompactionCommitted{State: state})
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if result.Status != runstate.EngineCompleted {
		return &RunError{Result: Result{Status: ResultFailed, Reason: "Compaction did not complete"}}
	}
	return nil
}

func (session *Session) compactionRef(cursor runstate.Cursor, descriptor json.RawMessage, purpose, id string) (runstate.ContextCompactionRef, error) {
	resource, err := agentsessionCanonical(session.key)
	if err != nil {
		return runstate.ContextCompactionRef{}, err
	}
	spec, err := hashCanonical(struct {
		Version uint16
		Cursor  runstate.Cursor
		Data    json.RawMessage
	}{1, cursor, descriptor})
	if err != nil {
		return runstate.ContextCompactionRef{}, err
	}
	return runstate.ContextCompactionRef{
		SpecRef: spec, Source: "agent.session.transcript", Purpose: purpose, Resource: resource,
		ExpectedRevision: fmt.Sprintf("cursor:%d", cursor), CompactionID: id,
		Envelope: append(json.RawMessage(nil), descriptor...),
	}, nil
}

func (session *Session) compactionState(_ context.Context) (CompactionState, bool, error) {
	if err := session.usable(); err != nil {
		return CompactionState{}, false, err
	}
	session.mu.RLock()
	state, present, _, err := compactionStateFrom(session.capabilities)
	clear, clearPresent, clearErr := clearStateFrom(session.capabilities)
	session.mu.RUnlock()
	if err != nil {
		return CompactionState{}, false, err
	}
	if clearErr != nil {
		return CompactionState{}, false, clearErr
	}
	state, present = clearCompaction(state, present, clear, clearPresent)
	return state, present, nil
}

func (session *Session) publishSessionEvent(payload EventPayload) {
	session.mu.Lock()
	session.publishLocked(Event{Payload: payload})
	session.mu.Unlock()
}
