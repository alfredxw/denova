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
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return mapRuntimeError(session.harness.SetCapabilityState(ctx, clearCapability, current.Descriptor, encoded, false))
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
