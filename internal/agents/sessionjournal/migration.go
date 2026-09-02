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
