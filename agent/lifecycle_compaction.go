package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

func (session *Session) Compact(ctx context.Context, request CompactionRequest) (CompactionResult, error) {
	if err := session.usable(); err != nil {
		return CompactionResult{}, err
	}
	commandID := strings.TrimSpace(request.IdempotencyKey)
	if commandID == "" {
		commandID = newPublicID("compact")
	}
	preparation, err := session.prepareStructuralDefinition(ctx, runstate.CommandID(commandID))
	if err != nil {
		return CompactionResult{}, err
	}
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
		SessionView{Key: session.key, Revision: uint64(preparation.checkpoint.Cursor)},
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
		Version:       compactionCommandVersion,
		DefinitionKey: preparation.prepared.definitionKey, RestoreKey: preparation.prepared.restoreKey,
		MaterializedFingerprint: preparation.prepared.materializedFingerprint,
		ModelRequestFingerprint: modelFingerprint,
		Manager:                 preparation.prepared.definition.Compaction.Identity(), Compact: &request,
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return CompactionResult{}, err
	}
	ref, err := session.compactionRef(preparation.checkpoint.Cursor, encoded, "create bounded conversation checkpoint", "")
	if err != nil {
		return CompactionResult{}, err
	}
	observeCtx, stopObserving := context.WithCancel(ctx)
	defer stopObserving()
	observation, err := session.harness.Observe(observeCtx, preparation.checkpoint.Cursor)
	if err != nil {
		return CompactionResult{}, mapRuntimeError(err)
	}
	operationID, err := session.submitStructural(ctx, runstate.CompactIfNeeded{ID: runstate.CommandID(commandID), Ref: ref})
	if err != nil {
		return CompactionResult{}, err
	}
	if err := session.waitStructural(observation, operationID); err != nil {
		return CompactionResult{}, err
	}
	updated, exists, err := session.compactionState(ctx)
	if err != nil {
		return CompactionResult{}, err
	}
	changed := exists && (!present || updated.Revision != current.Revision)
	return CompactionResult{Changed: changed, State: updated}, nil
}

func (session *Session) RemoveCompaction(ctx context.Context, request CompactionRemoveRequest) (bool, error) {
	if err := session.usable(); err != nil {
		return false, err
	}
	commandID := strings.TrimSpace(request.IdempotencyKey)
	if commandID == "" {
		commandID = newPublicID("remove-compaction")
	}
	preparation, err := session.prepareStructuralDefinition(ctx, runstate.CommandID(commandID))
	if err != nil {
		return false, err
	}
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
		Version:       compactionCommandVersion,
		DefinitionKey: preparation.prepared.definitionKey, RestoreKey: preparation.prepared.restoreKey,
		MaterializedFingerprint: preparation.prepared.materializedFingerprint,
		Manager:                 preparation.prepared.definition.Compaction.Identity(), Remove: &request,
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return false, err
	}
	ref, err := session.compactionRef(preparation.checkpoint.Cursor, encoded, "restore raw conversation history", current.ID)
	if err != nil {
		return false, err
	}
	observeCtx, stopObserving := context.WithCancel(ctx)
	defer stopObserving()
	observation, err := session.harness.Observe(observeCtx, preparation.checkpoint.Cursor)
	if err != nil {
		return false, mapRuntimeError(err)
	}
	operationID, err := session.submitStructural(ctx, runstate.RemoveCompaction{ID: runstate.CommandID(commandID), Ref: ref})
	if err != nil {
		return false, err
	}
	if err := session.waitStructural(observation, operationID); err != nil {
		return false, err
	}
	updated, exists, err := session.compactionState(ctx)
	if err != nil {
		return false, err
	}
	return exists && updated.Removed && updated.Revision > current.Revision, nil
}

type structuralDefinitionPreparation struct {
	prepared          preparedDefinition
	transcript        engineTranscript
	checkpoint        runstate.EngineCheckpointSnapshot
	compaction        CompactionState
	compactionPresent bool
	cleanup           CleanupState
	cleanupPresent    bool
}

func (session *Session) prepareStructuralDefinition(ctx context.Context, commandID runstate.CommandID) (structuralDefinitionPreparation, error) {
	checkpoint, err := session.harness.EngineCheckpoint(ctx)
	if err != nil {
		return structuralDefinitionPreparation{}, mapRuntimeError(err)
	}
	transcript, err := decodeEngineTranscript(checkpoint.State)
	if err != nil {
		return structuralDefinitionPreparation{}, err
	}
	clearState, clearPresent, err := applyClearToTranscript(&transcript, checkpoint.Capabilities)
	if err != nil {
		return structuralDefinitionPreparation{}, err
	}
	current, present, _, err := compactionStateFrom(checkpoint.Capabilities)
	if err != nil {
		return structuralDefinitionPreparation{}, err
	}
	current, present = clearCompaction(current, present, clearState, clearPresent)
	cleanup, cleanupPresent, _, err := cleanupStateFrom(checkpoint.Capabilities)
	if err != nil {
		return structuralDefinitionPreparation{}, err
	}
	cleanup, cleanupPresent = clearCleanup(cleanup, cleanupPresent, clearState, clearPresent)
	cleanup, cleanupPresent = cleanupAfterCompaction(cleanup, cleanupPresent, current, present)
	prepared, err := prepareDefinition(ctx, session.agent.source, PrepareRequest{
		Session:    SessionView{Key: session.key, Revision: uint64(checkpoint.Cursor)},
		Run:        structuralDefinitionRun(commandID),
		Reason:     TurnReasonStructural,
		HostData:   cloneHostData(transcript.HostData),
		Compaction: compactionStatePointer(current, present),
		Cleanup:    cloneCleanupStateIfPresent(cleanup, cleanupPresent),
	})
	if err != nil {
		return structuralDefinitionPreparation{}, err
	}
	if session.agent != nil && isPersistentStore(session.agent.store) {
		if err := validatePersistentDefinition(prepared.definition); err != nil {
			return structuralDefinitionPreparation{}, err
		}
	}
	materialized, err := materializedDefinitionFingerprint(prepared)
	if err != nil {
		return structuralDefinitionPreparation{}, err
	}
	prepared.materializedFingerprint = materialized
	return structuralDefinitionPreparation{
		prepared: prepared, transcript: transcript, checkpoint: checkpoint,
		compaction: current, compactionPresent: present,
		cleanup: cleanup, cleanupPresent: cleanupPresent,
	}, nil
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
		SpecRef: spec, Source: "agent.session.transcript", Purpose: purpose,
		Resource: resource, ExpectedRevision: fmt.Sprintf("cursor:%d", cursor),
		CompactionID: id, RestoreDescriptor: append(json.RawMessage(nil), descriptor...),
	}, nil
}

func (session *Session) submitStructural(ctx context.Context, command runstate.Command) (runstate.OperationID, error) {
	receipt, err := session.harness.Submit(ctx, command)
	if err != nil {
		return "", mapRuntimeError(err)
	}
	return receipt.OperationID, nil
}

func (session *Session) waitStructural(observation runstate.Observation, operationID runstate.OperationID) error {
	if observation.Snapshot.LastOperation != nil && observation.Snapshot.LastOperation.OperationID == operationID {
		return structuralResultError(*observation.Snapshot.LastOperation)
	}
	for event := range observation.Events {
		if settled, ok := event.Payload.(runstate.OperationSettledEvent); ok && settled.OperationID == operationID {
			return structuralResultError(runstate.OperationSummary{OperationID: operationID, Status: settled.Status, Reason: settled.Reason})
		}
	}
	select {
	case err := <-observation.Errors:
		if err != nil {
			return mapRuntimeError(err)
		}
	default:
	}
	return errors.New("Compaction observation closed before settlement")
}

func structuralResultError(summary runstate.OperationSummary) error {
	if summary.Status == runstate.OperationSucceeded {
		return nil
	}
	status := ResultFailed
	if summary.Status == runstate.OperationAborted {
		status = ResultAborted
	}
	return &RunError{Result: Result{Status: status, Reason: summary.Reason}}
}

func (session *Session) compactionState(ctx context.Context) (CompactionState, bool, error) {
	snapshot, err := session.harness.CapabilityState(ctx, compactionCapability)
	if err != nil {
		return CompactionState{}, false, mapRuntimeError(err)
	}
	if !snapshot.Exists {
		return CompactionState{}, false, nil
	}
	state, err := decodeCompactionState(snapshot.State)
	if err != nil {
		return CompactionState{}, false, err
	}
	clearSnapshot, clearErr := session.harness.CapabilityState(ctx, clearCapability)
	if clearErr != nil {
		return CompactionState{}, false, mapRuntimeError(clearErr)
	}
	if clearSnapshot.Exists {
		clearState, decodeErr := decodeClearState(clearSnapshot.State)
		if decodeErr != nil {
			return CompactionState{}, false, decodeErr
		}
		if state.Revision <= clearState.CompactionRevisionAtClear {
			return CompactionState{}, false, nil
		}
	}
	return state, true, nil
}
