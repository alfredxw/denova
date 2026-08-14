package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"
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
	session.mu.Lock()
	defer session.mu.Unlock()
	raw, present := session.capabilities[clearCapability]
	var state ClearState
	var err error
	if present {
		state, err = decodeClearState(raw)
		if err != nil {
			return err
		}
	}
	compaction, compactionPresent, _, err := compactionStateFrom(session.capabilities)
	if err != nil {
		return err
	}
	state.Revision++
	state.ClearedAt = time.Now().UTC()
	if compactionPresent {
		state.CompactionRevisionAtClear = compaction.Revision
	}
	cleanup, cleanupPresent, _, err := cleanupStateFrom(session.capabilities)
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
	// Todo belongs to the cleared conversation. Goal intentionally survives so
	// a user-controlled long-running objective can continue into the fresh transcript.
	for _, capability := range []string{TodoCapability, compactionHealthCapability} {
		delete(session.capabilities, capability)
	}
	session.capabilities[clearCapability] = encoded
	if err := session.persistCapabilitiesLocked(ctx); err != nil {
		return err
	}
	session.publishLocked(Event{Payload: SessionCleared{Revision: state.Revision}})
	return nil
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
