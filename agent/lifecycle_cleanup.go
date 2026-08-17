package agent

import (
	"context"
)

func (session *Session) cleanupState(ctx context.Context) (CleanupState, bool, error) {
	if err := session.usable(); err != nil {
		return CleanupState{}, false, err
	}
	session.mu.RLock()
	raw, present := session.capabilities[cleanupCapability]
	clearRaw, clearPresent := session.capabilities[clearCapability]
	session.mu.RUnlock()
	if !present {
		return CleanupState{}, false, nil
	}
	state, err := decodeCleanupState(raw)
	if err != nil {
		return CleanupState{}, false, err
	}
	if clearPresent {
		clearState, decodeErr := decodeClearState(clearRaw)
		if decodeErr != nil {
			return CleanupState{}, false, decodeErr
		}
		if state.Revision <= clearState.CleanupRevisionAtClear {
			return CleanupState{}, false, nil
		}
	}
	return state, !state.Removed, nil
}

// Cleanup returns the active reversible cleanup projection, if any.
func (session *Session) Cleanup(ctx context.Context) (CleanupState, bool, error) {
	state, present, err := session.cleanupState(ctx)
	if err != nil || !present {
		return CleanupState{}, present, err
	}
	compaction, compactionPresent, compactionErr := session.compactionState(ctx)
	if compactionErr != nil {
		return CleanupState{}, false, compactionErr
	}
	state, present = cleanupAfterCompaction(state, present, compaction, compactionPresent)
	if !present {
		return CleanupState{}, false, nil
	}
	return *cloneCleanupState(&state), true, nil
}
