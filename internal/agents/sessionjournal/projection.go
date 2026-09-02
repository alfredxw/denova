// Package sessionjournal embeds public Agent lifecycle records in a Denova
// product conversation journal. The JSONL journal remains authoritative; this
// projection only keeps the bounded state needed to reopen one Agent Session.
package sessionjournal

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	agentsession "github.com/alfredxw/denova/agent/session"
)

const (
	RecordType = "agent_session"

	messageCheckpointKind = "session.message_checkpoint"
	capabilitySetKind     = "session.capability_set"
	capabilityDeleteKind  = "session.capability_delete"
	turnStartedKind       = "turn.started"
	turnFinishedKind      = "turn.finished"
	turnInterruptedKind   = "turn.interrupted"

	recentTurnRecordLimit = 64
)

// Envelope is one public Agent record carried by a product journal
// transaction. Revision is logical to this exact Agent Session key and is
// deliberately independent from the physical product-journal cursor.
type Envelope struct {
	Type     string                `json:"type"`
	Key      agentsession.Key      `json:"key"`
	Revision agentsession.Revision `json:"revision"`
	Deleted  bool                  `json:"deleted,omitempty"`
	Kind     string                `json:"kind"`
	Version  uint16                `json:"version"`
	Data     json.RawMessage       `json:"data,omitempty"`
}

type streamProjection struct {
	Key               agentsession.Key               `json:"key"`
	Revision          agentsession.Revision          `json:"revision"`
	MessageCheckpoint *agentsession.Record           `json:"message_checkpoint,omitempty"`
	Capabilities      map[string]agentsession.Record `json:"capabilities,omitempty"`
	Turns             []agentsession.Record          `json:"turns,omitempty"`
}

// Projection is embedded in each product domain's rebuildable index. Root
// Session messages remain product records; self-contained child Sessions use
// their own journal and never enter this projection.
type Projection struct {
	Streams map[string]*streamProjection `json:"streams,omitempty"`
}

func (projection *Projection) Reset() {
	projection.Streams = make(map[string]*streamProjection)
}

func (projection *Projection) Normalize() error {
	if projection.Streams == nil {
		projection.Reset()
		return nil
	}
	for canonical, persisted := range projection.Streams {
		if persisted == nil {
			return fmt.Errorf("agent session projection %q is nil", canonical)
		}
		key, err := agentsession.NormalizeKey(persisted.Key)
		if err != nil {
			return fmt.Errorf("agent session projection key: %w", err)
		}
		encoded, err := agentsession.CanonicalKey(key)
		if err != nil || encoded != canonical {
			return fmt.Errorf("agent session projection key mismatch")
		}
		rebuilt := &streamProjection{Key: key, Capabilities: make(map[string]agentsession.Record)}
		var previous agentsession.Revision
		for _, record := range persisted.records() {
			if record.Revision == 0 || record.Revision <= previous || record.Revision > persisted.Revision {
				return fmt.Errorf("agent session projection record revision is invalid")
			}
			if err := agentsession.ValidateRecord(record); err != nil {
				return err
			}
			if err := validateEmbeddedRecord(record); err != nil {
				return err
			}
			if err := rebuilt.apply(record); err != nil {
				return err
			}
			previous = record.Revision
		}
		if previous != persisted.Revision {
			return fmt.Errorf("agent session projection latest revision is missing")
		}
		rebuilt.Revision = persisted.Revision
		projection.Streams[canonical] = rebuilt
	}
	return nil
}

func (projection *Projection) Apply(payload json.RawMessage) (bool, error) {
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &typed); err != nil {
		return false, err
	}
	if typed.Type != RecordType {
		return false, nil
	}
	var envelope Envelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return true, err
	}
	key, err := agentsession.NormalizeKey(envelope.Key)
	if err != nil {
		return true, err
	}
	canonical, err := agentsession.CanonicalKey(key)
	if err != nil {
		return true, err
	}
	if envelope.Deleted {
		stream := projection.Streams[canonical]
		current := agentsession.Revision(0)
		if stream != nil {
			current = stream.Revision
		}
		if envelope.Revision != current+1 || envelope.Kind != "" || envelope.Version != 0 || len(envelope.Data) != 0 {
			return true, fmt.Errorf("agent session deletion record is invalid")
		}
		delete(projection.Streams, canonical)
		return true, nil
	}
	record := agentsession.Record{
		Revision: envelope.Revision, Kind: envelope.Kind,
		Version: envelope.Version, Data: append(json.RawMessage(nil), envelope.Data...),
	}
	if err := agentsession.ValidateRecord(record); err != nil {
		return true, err
	}
	if err := validateEmbeddedRecord(record); err != nil {
		return true, err
	}
	if projection.Streams == nil {
		projection.Reset()
	}
	stream := projection.Streams[canonical]
	if stream == nil {
		stream = &streamProjection{
			Key: key, Capabilities: make(map[string]agentsession.Record),
		}
		projection.Streams[canonical] = stream
	}
	want := stream.Revision + 1
	if record.Revision != want {
		return true, fmt.Errorf("agent session revision gap: have=%d want=%d", record.Revision, want)
	}
	if err := stream.apply(record); err != nil {
		return true, err
	}
	stream.Revision = record.Revision
	return true, nil
}

func (projection *Projection) Revision(key agentsession.Key) (agentsession.Revision, error) {
	stream, err := projection.stream(key)
	if err != nil || stream == nil {
		return 0, err
	}
	return stream.Revision, nil
}

func (projection *Projection) Records(key agentsession.Key) ([]agentsession.Record, error) {
	stream, err := projection.stream(key)
	if err != nil || stream == nil {
		return nil, err
	}
	return stream.records(), nil
}

func (projection *Projection) Keys() []agentsession.Key {
	keys := make([]agentsession.Key, 0, len(projection.Streams))
	for _, stream := range projection.Streams {
		if stream == nil {
			continue
		}
		key := stream.Key
		key.Attributes = cloneStringMap(key.Attributes)
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		leftKey, _ := agentsession.CanonicalKey(keys[left])
		rightKey, _ := agentsession.CanonicalKey(keys[right])
		return leftKey < rightKey
	})
	return keys
}

func (projection *Projection) stream(key agentsession.Key) (*streamProjection, error) {
	canonical, err := agentsession.CanonicalKey(key)
	if err != nil {
		return nil, err
	}
	return projection.Streams[canonical], nil
}

func (stream *streamProjection) apply(record agentsession.Record) error {
	switch record.Kind {
	case messageCheckpointKind:
		stream.MessageCheckpoint = cloneRecordPtr(record)
	case capabilitySetKind, capabilityDeleteKind:
		var value struct {
			Capability string `json:"capability"`
		}
		if err := json.Unmarshal(record.Data, &value); err != nil {
			return fmt.Errorf("decode agent capability projection: %w", err)
		}
		if strings.TrimSpace(value.Capability) == "" {
			return fmt.Errorf("agent capability projection has an empty identity")
		}
		stream.Capabilities[value.Capability] = cloneRecord(record)
	case turnStartedKind, turnFinishedKind, turnInterruptedKind:
		stream.Turns = append(stream.Turns, cloneRecord(record))
		if overflow := len(stream.Turns) - recentTurnRecordLimit; overflow > 0 {
			stream.Turns = append([]agentsession.Record(nil), stream.Turns[overflow:]...)
		}
	default:
		return fmt.Errorf("unsupported agent session record %q", record.Kind)
	}
	return nil
}

func (stream *streamProjection) records() []agentsession.Record {
	if stream == nil {
		return nil
	}
	records := make([]agentsession.Record, 0, 3+len(stream.Capabilities)+len(stream.Turns))
	if stream.MessageCheckpoint != nil {
		records = append(records, cloneRecord(*stream.MessageCheckpoint))
	}
	for _, record := range stream.Capabilities {
		records = append(records, cloneRecord(record))
	}
	for _, record := range stream.Turns {
		records = append(records, cloneRecord(record))
	}
	sort.SliceStable(records, func(left, right int) bool { return records[left].Revision < records[right].Revision })
	return records
}

func validateEmbeddedRecord(record agentsession.Record) error {
	if record.Version != 1 {
		return fmt.Errorf("unsupported embedded Agent Session record version %d", record.Version)
	}
	switch record.Kind {
	case messageCheckpointKind:
		var value struct {
			Hash         string `json:"hash"`
			MessageCount int    `json:"message_count"`
		}
		if err := json.Unmarshal(record.Data, &value); err != nil {
			return fmt.Errorf("decode Agent message checkpoint: %w", err)
		}
		if strings.TrimSpace(value.Hash) == "" || value.MessageCount < 0 {
			return fmt.Errorf("Agent message checkpoint is invalid")
		}
	case capabilitySetKind, capabilityDeleteKind:
		var value struct {
			Capability string          `json:"capability"`
			State      json.RawMessage `json:"state"`
		}
		if err := json.Unmarshal(record.Data, &value); err != nil {
			return fmt.Errorf("decode Agent capability record: %w", err)
		}
		if strings.TrimSpace(value.Capability) == "" {
			return fmt.Errorf("Agent capability identity is empty")
		}
		if record.Kind == capabilitySetKind && !json.Valid(value.State) {
			return fmt.Errorf("Agent capability state is invalid")
		}
	case turnStartedKind, turnFinishedKind, turnInterruptedKind:
		var value struct {
			RunID     string `json:"run_id"`
			CommandID string `json:"command_id"`
			At        string `json:"at"`
		}
		if err := json.Unmarshal(record.Data, &value); err != nil {
			return fmt.Errorf("decode Agent turn record: %w", err)
		}
		if strings.TrimSpace(value.RunID) == "" || strings.TrimSpace(value.CommandID) == "" || strings.TrimSpace(value.At) == "" {
			return fmt.Errorf("Agent turn record is invalid")
		}
	default:
		return fmt.Errorf("unsupported Agent Session record %q", record.Kind)
	}
	return nil
}

func cloneRecord(record agentsession.Record) agentsession.Record {
	record.Data = append(json.RawMessage(nil), record.Data...)
	return record
}

func cloneRecordPtr(record agentsession.Record) *agentsession.Record {
	cloned := cloneRecord(record)
	return &cloned
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
