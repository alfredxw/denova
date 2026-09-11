package sessionjournal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentsession "github.com/alfredxw/denova/agent/session"
)

// HasCapabilityRecord reports whether this embedded stream already has a
// latest set or delete record for a capability.
func (log *Log) HasCapabilityRecord(capability string) (bool, error) {
	if log == nil {
		return false, fmt.Errorf("embedded Agent Session log is unavailable")
	}
	capability = strings.TrimSpace(capability)
	if capability == "" {
		return false, fmt.Errorf("Agent capability identity is empty")
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.closed {
		return false, agentsession.ErrLogClosed
	}
	stream, err := log.projection.stream(log.key)
	if err != nil || stream == nil {
		return false, err
	}
	_, exists := stream.Capabilities[capability]
	return exists, nil
}

// ImportCapabilityIfAbsent appends one explicit compatibility conversion
// before an embedded Log is handed to Agent. A latest set or delete record is
// itself the migration receipt, so retries never overwrite newer Agent state.
func (log *Log) ImportCapabilityIfAbsent(
	ctx context.Context,
	capability string,
	state json.RawMessage,
	deleted bool,
) (bool, error) {
	if log == nil {
		return false, fmt.Errorf("embedded Agent Session log is unavailable")
	}
	capability = strings.TrimSpace(capability)
	if capability == "" || !deleted && !json.Valid(state) {
		return false, fmt.Errorf("Agent capability migration is invalid")
	}

	log.mu.Lock()
	defer log.mu.Unlock()
	stream, err := log.projection.stream(log.key)
	if err != nil {
		return false, err
	}
	if stream != nil {
		if _, exists := stream.Capabilities[capability]; exists {
			return false, nil
		}
	}
	data, err := json.Marshal(struct {
		Capability string          `json:"capability"`
		State      json.RawMessage `json:"state,omitempty"`
	}{Capability: capability, State: append(json.RawMessage(nil), state...)})
	if err != nil {
		return false, err
	}
	kind := capabilitySetKind
	if deleted {
		kind = capabilityDeleteKind
	}
	current, err := log.projection.Revision(log.key)
	if err != nil {
		return false, err
	}
	_, err = log.appendLocked(ctx, current, agentsession.Record{
		Kind: kind, Version: 1, Data: data,
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// Released Game checkpoints selected whole Story Turns independently of Agent's
// raw message range. That coverage cannot be recovered reliably. Ignore only
// that projection on read; the original JSONL remains intact and the next Run
// can build a checkpoint from canonical history. No model or write occurs here.
func projectReleasedGameCompaction(record agentsession.Record) (agentsession.Record, error) {
	if record.Kind != capabilitySetKind {
		return record, nil
	}
	var payload struct {
		Capability string `json:"capability"`
		State      struct {
			Version     uint16 `json:"version"`
			ContextData *struct {
				Type    string `json:"type"`
				Version uint16 `json:"version"`
			} `json:"context_data"`
		} `json:"state"`
	}
	if err := json.Unmarshal(record.Data, &payload); err != nil {
		return record, err
	}
	if payload.Capability != "agent.compaction" || payload.State.Version != 0 || payload.State.ContextData == nil || payload.State.ContextData.Type != "denova.interactive.compaction" || payload.State.ContextData.Version != 1 {
		return record, nil
	}
	record.Kind = capabilityDeleteKind
	record.Data = json.RawMessage(`{"capability":"agent.compaction"}`)
	return record, nil
}

func usesIncrementalCompaction(record agentsession.Record) bool {
	if record.Kind != capabilitySetKind {
		return false
	}
	var payload struct {
		Capability string `json:"capability"`
		State      struct {
			Version uint16 `json:"version"`
		} `json:"state"`
	}
	return json.Unmarshal(record.Data, &payload) == nil && payload.Capability == "agent.compaction" && payload.State.Version == 2
}
