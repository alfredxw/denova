package sessionjournal

import (
	"bytes"
	"encoding/json"
	"fmt"

	agentsession "github.com/alfredxw/denova/agent/session"
)

// Capability reads the latest versioned state from the owning product journal.
// Applications may reuse state schemas across peer runtimes; only the selected
// runtime may mutate a capability, after the previous executor has drained.
func (projection *Projection) Capability(key agentsession.Key, capability string) (json.RawMessage, bool, error) {
	stream, err := projection.stream(key)
	if err != nil || stream == nil {
		return nil, false, err
	}
	record, found := stream.Recovery.CapabilityRecord(capability)
	if !found || record.Kind == capabilityDeleteKind {
		return nil, false, nil
	}
	var value struct {
		State json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(record.Data, &value); err != nil {
		return nil, false, err
	}
	return value.State, true, nil
}

// CapabilityUpdate prepares an atomic state replacement under the product's
// canonical mutation fence. The callback is pure and may be retried after CAS.
// A nil result leaves state unchanged; explicit tombstones use their schema.
func (projection *Projection) CapabilityUpdate(key agentsession.Key, capability string, update func(json.RawMessage, bool) (json.RawMessage, error)) (*Envelope, error) {
	raw, present, err := projection.Capability(key, capability)
	if err != nil {
		return nil, err
	}
	next, err := update(raw, present)
	if err != nil || next == nil || bytes.Equal(next, raw) {
		return nil, err
	}
	if !json.Valid(next) {
		return nil, fmt.Errorf("invalid capability state for %s", capability)
	}
	data, err := json.Marshal(struct {
		Capability string          `json:"capability"`
		State      json.RawMessage `json:"state"`
	}{capability, next})
	if err != nil {
		return nil, err
	}
	revision, err := projection.Revision(key)
	if err != nil {
		return nil, err
	}
	return &Envelope{Type: RecordType, Key: key, Revision: revision + 1, Kind: capabilitySetKind, Version: 1, Data: data}, nil
}
