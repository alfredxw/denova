package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
	agentsession "github.com/alfredxw/denova/agent/session"
)

func contextCapabilityRecords(states map[string]json.RawMessage) ([]agentsession.Record, error) {
	keys := make([]string, 0, len(states))
	for key := range states {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	records := make([]agentsession.Record, 0, len(keys))
	for _, key := range keys {
		record, err := sessionRecord(sessionCapabilitySetRecord, persistedCapability{Capability: key, State: states[key]})
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// A compaction checkpoint and its accepted context are one recovery fact.
// Product context commits use the same records through withCanonicalCheckpoint;
// this path covers native journals and transitions with no new product messages.
func (run *Run) commitContextCheckpoint(update runstate.EngineTranscriptUpdated) error {
	session := run.session
	session.mu.Lock()
	state, err := session.commitContextCheckpointLocked(context.Background(), update)
	session.mu.Unlock()
	if err != nil {
		return err
	}
	update.State = state
	run.mu.Lock()
	run.snapshot.State = append(json.RawMessage(nil), update.State...)
	if run.snapshot.Capabilities == nil {
		run.snapshot.Capabilities = make(map[string]json.RawMessage)
	}
	for key, value := range update.CapabilityStates {
		run.snapshot.Capabilities[key] = append(json.RawMessage(nil), value...)
	}
	run.mu.Unlock()
	for key, value := range update.CapabilityStates {
		run.publishCapabilityUpdate(runstate.EngineCapabilityState{Capability: key, State: value})
	}
	return nil
}

// Commit the capability and the exact active window in the same journal batch.
// Callers hold session.mu and publish events only after this succeeds.
func (session *Session) commitContextCheckpointLocked(ctx context.Context, update runstate.EngineTranscriptUpdated) (json.RawMessage, error) {
	if session.canonicalMessages {
		capabilities := cloneRawStateMap(session.capabilities)
		for key, value := range update.CapabilityStates {
			capabilities[key] = value
		}
		state, err := decodeEngineTranscript(update.State)
		if err != nil {
			return nil, err
		}
		if len(session.messageCheckpoint.Metadata) != 0 {
			var previous engineTranscript
			if err := json.Unmarshal(session.messageCheckpoint.Metadata, &previous); err != nil {
				return nil, err
			}
			state.HistoryHead = previous.HistoryHead
		}
		update.State, err = encodeCanonicalWindow(state, capabilities)
		if err != nil {
			return nil, err
		}
	}
	checkpoint, err := canonicalMessageCheckpoint(update.State)
	if err != nil {
		return nil, err
	}
	alreadyCommitted := bytes.Equal(session.engineState, update.State)
	for key, value := range update.CapabilityStates {
		alreadyCommitted = alreadyCommitted && bytes.Equal(session.durableCapabilities[key], value)
	}
	if !alreadyCommitted {
		records, err := contextCapabilityRecords(update.CapabilityStates)
		if err == nil {
			var record agentsession.Record
			if session.canonicalMessages {
				record, err = sessionRecord(sessionMessageCheckpointRecord, checkpoint)
			} else {
				record, err = sessionRecord(sessionTranscriptRecord, persistedSessionTranscript{EngineState: update.State})
			}
			records = append(records, record)
		}
		if err == nil {
			err = session.appendRecordsLocked(ctx, records...)
		}
		if err != nil {
			return nil, err
		}
		session.engineState = append(json.RawMessage(nil), update.State...)
		if session.canonicalMessages {
			session.messageCheckpoint = checkpoint
		}
		for key, value := range update.CapabilityStates {
			session.capabilities[key] = append(json.RawMessage(nil), value...)
			session.durableCapabilities[key] = append(json.RawMessage(nil), value...)
		}
	}
	return update.State, nil
}
