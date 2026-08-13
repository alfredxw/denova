package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

const clearCapability = "agent.clear"

type ClearState struct {
	Revision                  uint64    `json:"revision"`
	CompactionRevisionAtClear uint64    `json:"compaction_revision_at_clear,omitempty"`
	CleanupRevisionAtClear    uint64    `json:"cleanup_revision_at_clear,omitempty"`
	ClearedAt                 time.Time `json:"cleared_at"`
}

func (session *Session) Clear(ctx context.Context) error {
	if err := session.usable(); err != nil {
		return err
	}
	current, err := session.harness.CapabilityState(ctx, clearCapability)
	if err != nil {
		return mapRuntimeError(err)
	}
	var state ClearState
	if current.Exists {
		state, err = decodeClearState(current.State)
		if err != nil {
			return err
		}
	}
	compaction, present, err := session.compactionState(ctx)
	if err != nil {
		return err
	}
	state.Revision++
	state.ClearedAt = time.Now().UTC()
	if present {
		state.CompactionRevisionAtClear = compaction.Revision
	}
	cleanup, cleanupPresent, err := session.cleanupState(ctx)
	if err != nil {
		return err
	}
	if cleanupPresent {
		state.CleanupRevisionAtClear = cleanup.Revision
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	changes := make([]runstate.EngineCapabilityState, 0, 3)
	// Todo belongs to the cleared conversation. Goal intentionally survives so
	// a user-controlled long-running objective can continue into the fresh
	// transcript. Raw Cleanup/Compaction checkpoints remain recoverable and are
	// hidden by the clear generation; their failure fuse must not leak across it.
	for _, capability := range []string{TodoCapability, compactionHealthCapability} {
		snapshot, snapshotErr := session.harness.CapabilityState(ctx, capability)
		if snapshotErr != nil {
			return mapRuntimeError(snapshotErr)
		}
		if snapshot.Exists {
			changes = append(changes, runstate.EngineCapabilityState{
				Capability: capability, Expected: snapshot.Descriptor, Delete: true,
			})
		}
	}
	// SessionCleared is deliberately the final event in the atomic journal
	// append. Consumers may observe individual projections in order, and this
	// makes that event the visible barrier after every reset component.
	changes = append(changes, runstate.EngineCapabilityState{
		Capability: clearCapability, Expected: current.Descriptor, State: encoded,
	})
	return mapRuntimeError(session.harness.SetCapabilityStates(ctx, changes...))
}

func decodeClearState(raw json.RawMessage) (ClearState, error) {
	var state ClearState
	if err := json.Unmarshal(raw, &state); err != nil {
		return ClearState{}, err
	}
	if state.Revision == 0 || state.ClearedAt.IsZero() {
		return ClearState{}, errors.New("durable Clear state is invalid")
	}
	return state, nil
}

func clearStateFrom(states map[string]json.RawMessage) (ClearState, bool, error) {
	raw, present := states[clearCapability]
	if !present {
		return ClearState{}, false, nil
	}
	state, err := decodeClearState(raw)
	return state, true, err
}

func applyClearToTranscript(transcript *engineTranscript, capabilities map[string]json.RawMessage) (ClearState, bool, error) {
	clearState, present, err := clearStateFrom(capabilities)
	if err != nil || !present || transcript == nil {
		return clearState, present, err
	}
	if clearState.Revision > transcript.ClearRevision {
		transcript.Messages = nil
		transcript.ContextState = contextStateSnapshot{}
		transcript.ClearRevision = clearState.Revision
	}
	return clearState, true, nil
}

func clearCompaction(current CompactionState, present bool, clearState ClearState, clearPresent bool) (CompactionState, bool) {
	if clearPresent && present && current.Revision <= clearState.CompactionRevisionAtClear {
		return CompactionState{}, false
	}
	return current, present
}
