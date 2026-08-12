package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Inspect prepares the exact provider-neutral request for one prospective
// start turn without admitting a command, mutating transcript/capabilities, or
// invoking a model or tool. It succeeds only when the Session is idle for the
// complete optimistic read. Preparation capabilities and Middleware run under
// an inspection-marked context and must remain read-only.
//
// Goal mutations are deliberately rejected: they require durable admission
// and revision fencing before they can affect model context. Inspect the
// current Goal, or commit the mutation through Run/UpdateGoal first.
func (session *Session) Inspect(ctx context.Context, input Input) (Inspection, error) {
	if err := session.usable(); err != nil {
		return Inspection{}, err
	}
	if input.Goal != nil {
		return Inspection{}, errors.New("Agent Session inspection cannot preview an uncommitted Goal mutation")
	}
	commandID := strings.TrimSpace(input.IdempotencyKey)
	if commandID == "" {
		fingerprint, err := hashCanonical(struct {
			Session SessionKey
			Input   Input
		}{Session: session.key, Input: input})
		if err != nil {
			return Inspection{}, fmt.Errorf("fingerprint Agent Session inspection: %w", err)
		}
		commandID = "inspection-" + fingerprint[:32]
		input.IdempotencyKey = commandID
	}
	if _, _, err := encodeInput(input); err != nil {
		return Inspection{}, err
	}
	ctx = contextWithInspection(ctx)
	checkpoint, err := session.harness.IdleEngineCheckpoint(ctx)
	if err != nil {
		return Inspection{}, mapRuntimeError(err)
	}
	transcript, err := decodeEngineTranscript(checkpoint.State)
	if err != nil {
		return Inspection{}, err
	}
	clearState, clearPresent, err := applyClearToTranscript(&transcript, checkpoint.Capabilities)
	if err != nil {
		return Inspection{}, err
	}
	compaction, compactionPresent, _, err := compactionStateFrom(checkpoint.Capabilities)
	if err != nil {
		return Inspection{}, err
	}
	compaction, compactionPresent = clearCompaction(compaction, compactionPresent, clearState, clearPresent)
	cleanup, cleanupPresent, _, err := cleanupStateFrom(checkpoint.Capabilities)
	if err != nil {
		return Inspection{}, err
	}
	cleanup, cleanupPresent = clearCleanup(cleanup, cleanupPresent, clearState, clearPresent)
	cleanup, cleanupPresent = cleanupAfterCompaction(cleanup, cleanupPresent, compaction, compactionPresent)

	sessionView := SessionView{Key: session.key, Revision: uint64(checkpoint.Cursor)}
	// The synthetic Run identity lets dynamic capabilities assemble the same
	// bounded provenance they use for a real start, but it is never admitted and
	// therefore grants no lifecycle control authority.
	runView := RunView{
		ID: commandID, CommandID: commandID, Cycle: 1,
		StartedAt: time.Now(), Delivery: TurnDeliveryStart,
	}
	prepareRequest := PrepareRequest{
		Session:    sessionView,
		Run:        runView,
		Input:      input,
		Reason:     TurnReasonStart,
		HostData:   cloneHostData(input.HostData),
		Compaction: compactionStatePointer(compaction, compactionPresent),
		Cleanup:    cloneCleanupStateIfPresent(cleanup, cleanupPresent),
	}
	prepared, err := prepareDefinition(ctx, session.agent.source, prepareRequest)
	if err != nil {
		return Inspection{}, err
	}
	if session.agent != nil && isPersistentStore(session.agent.store) {
		if err := validatePersistentDefinition(prepared.definition); err != nil {
			return Inspection{}, err
		}
	}
	var goal GoalState
	goalPresent := false
	if raw, present := checkpoint.Capabilities[goalCapability]; present {
		goal, err = decodeGoalState(raw)
		if err != nil {
			return Inspection{}, err
		}
		goalPresent = true
	}
	if err := applyPreparedGoal(ctx, &prepared, sessionView, runView, goal, goalPresent); err != nil {
		return Inspection{}, err
	}
	materialized, err := materializedDefinitionFingerprint(prepared)
	if err != nil {
		return Inspection{}, err
	}
	prepared.materializedFingerprint = materialized

	visible, err := effectiveCleanupMessages(
		transcript.Messages,
		cleanup,
		cleanupPresent,
		compaction,
		compactionPresent,
	)
	if err != nil {
		return Inspection{}, err
	}
	summaryLimit := 0
	if prepared.definition.Compaction != nil {
		summaryLimit = prepared.definition.Compaction.SummaryLimitBytes()
	} else if compactionPresent && !compaction.Removed {
		return Inspection{}, fmt.Errorf("%w: active Compaction has no Manager in the selected Definition", ErrDefinitionMismatch)
	}
	effective, err := effectiveCompactionMessages(visible, compaction, compactionPresent, summaryLimit)
	if err != nil {
		return Inspection{}, err
	}
	messages, _, err := assembleCycleMessages(effective, input.Text, prepared.fragments)
	if err != nil {
		return Inspection{}, err
	}
	ctx, err = contextWithProviderCacheKey(ctx, session.key, session.agent.cacheKeys)
	if err != nil {
		return Inspection{}, err
	}
	request, err := prepareDefinitionModelRequest(
		ctx,
		prepared,
		sessionView,
		runView,
		messages,
		stableContextPrefixMessages(prepared.fragments, compaction, compactionPresent),
	)
	if err != nil {
		return Inspection{}, err
	}
	verified, err := session.harness.IdleEngineCheckpoint(ctx)
	if err != nil {
		return Inspection{}, mapRuntimeError(err)
	}
	if verified.Cursor != checkpoint.Cursor || verified.StateDescriptor != checkpoint.StateDescriptor {
		return Inspection{}, fmt.Errorf("%w: Agent Session changed during inspection", ErrSessionBusy)
	}
	return Inspection{
		Session:       sessionView,
		Run:           runView,
		DefinitionKey: prepared.definitionKey, RestoreKey: prepared.restoreKey,
		MaterializedFingerprint: prepared.materializedFingerprint,
		PrefixFingerprint:       prepared.prefixFingerprint,
		ModelIdentity:           prepared.definition.ModelIdentity,
		Cleanup:                 cloneCleanupStateIfPresent(cleanup, cleanupPresent),
		Compaction:              cloneCompactionStateIfPresent(compaction, compactionPresent),
		ContextFragments:        append([]ContextFragment(nil), prepared.fragments...),
		ModelRequest:            modelRequestInspection(request),
	}, nil
}

func cloneCompactionStateIfPresent(state CompactionState, present bool) *CompactionState {
	if !present || state.Removed {
		return nil
	}
	return cloneCompactionState(&state)
}
