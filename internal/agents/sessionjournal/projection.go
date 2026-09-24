// Package sessionjournal embeds Agent lifecycle records in the sole product
// journal. Its index retains unfinished facts and historical record locators.
package sessionjournal

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"denova/internal/agents/conversationjournal"
	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

const (
	RecordType            = "agent_session"
	messageCheckpointKind = "session.message_checkpoint"
	capabilitySetKind     = "session.capability_set"
	capabilityDeleteKind  = "session.capability_delete"
	turnStartedKind       = "turn.started"
	turnFinishedKind      = "turn.finished"
	turnInterruptedKind   = "turn.interrupted"
)

type Envelope struct {
	Type     string                `json:"type"`
	Key      agentsession.Key      `json:"key"`
	Revision agentsession.Revision `json:"revision"`
	Deleted  bool                  `json:"deleted,omitempty"`
	Kind     string                `json:"kind"`
	Version  uint16                `json:"version"`
	Data     json.RawMessage       `json:"data,omitempty"`
}

type recordLocator struct {
	Cursor      conversationjournal.Cursor `json:"cursor"`
	RecordIndex int                        `json:"record_index,omitempty"`
}

type streamProjection struct {
	Start     recordLocator                           `json:"start"`
	Key       agentsession.Key                        `json:"key"`
	Recovery  agent.RecoveryIndex                     `json:"recovery"`
	Locations map[agentsession.Revision]recordLocator `json:"locations,omitempty"`
}

// Projection is reconstructible from canonical JSONL in both product domains.
// Locations refer to immutable physical transactions, never host paths.
type Projection struct {
	Streams map[string]*streamProjection `json:"streams,omitempty"`
}

func (projection *Projection) Reset() { projection.Streams = make(map[string]*streamProjection) }

func (projection *Projection) Normalize() error {
	if projection.Streams == nil {
		projection.Reset()
	}
	for canonical, stream := range projection.Streams {
		if stream == nil {
			return fmt.Errorf("Agent recovery stream is nil")
		}
		key, err := agentsession.CanonicalKey(stream.Key)
		if err != nil || key != canonical {
			return fmt.Errorf("Agent recovery key mismatch")
		}
		if stream.Start.Cursor == 0 || stream.Start.RecordIndex < 0 {
			return fmt.Errorf("Agent recovery stream start is invalid")
		}
		for _, entry := range stream.Recovery.Records {
			if entry.Record.Revision == 0 || entry.Record.Revision > stream.Recovery.Revision {
				return fmt.Errorf("Agent recovery revision is invalid")
			}
			if err := validateEmbeddedRecord(entry.Record); err != nil {
				return err
			}
		}
		if stream.Locations == nil {
			stream.Locations = make(map[agentsession.Revision]recordLocator)
		}
		for revision, location := range stream.Locations {
			if revision == 0 || revision > stream.Recovery.Revision || location.Cursor == 0 || location.RecordIndex < 0 {
				return fmt.Errorf("Agent history locator is invalid")
			}
		}
	}
	return nil
}

func (projection *Projection) Apply(physical conversationjournal.Record) (bool, error) {
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(physical.Payload, &typed); err != nil {
		return false, err
	}
	if typed.Type != RecordType {
		return false, nil
	}
	var envelope Envelope
	if err := json.Unmarshal(physical.Payload, &envelope); err != nil {
		return true, err
	}
	key, err := agentsession.NormalizeKey(envelope.Key)
	if err != nil {
		return true, err
	}
	canonical, _ := agentsession.CanonicalKey(key)
	if projection.Streams == nil {
		projection.Reset()
	}
	stream := projection.Streams[canonical]
	current := agentsession.Revision(0)
	if stream != nil {
		current = stream.Recovery.Revision
	}
	if envelope.Revision != current+1 {
		return true, fmt.Errorf("Agent session revision gap: have=%d want=%d", envelope.Revision, current+1)
	}
	if envelope.Deleted {
		if envelope.Kind != "" || envelope.Version != 0 || len(envelope.Data) != 0 {
			return true, fmt.Errorf("Agent session deletion record is invalid")
		}
		delete(projection.Streams, canonical)
		return true, nil
	}
	record := agentsession.Record{Revision: envelope.Revision, Kind: envelope.Kind, Version: envelope.Version, Data: envelope.Data}
	if err := validateEmbeddedRecord(record); err != nil {
		return true, err
	}
	if stream == nil {
		stream = &streamProjection{Key: key, Start: recordLocator{Cursor: physical.Location.Cursor, RecordIndex: physical.Location.RecordIndex}, Locations: make(map[agentsession.Revision]recordLocator)}
		projection.Streams[canonical] = stream
	}
	// Only historical bodies read by an explicit lookup need physical locators.
	// Active tool results remain in Recovery until their Run settles.
	switch record.Kind {
	case "session.input", turnFinishedKind, turnInterruptedKind, "turn.interaction", "turn.interaction_response":
		stream.Locations[record.Revision] = recordLocator{Cursor: physical.Location.Cursor, RecordIndex: physical.Location.RecordIndex}
	}
	if err := stream.Recovery.Apply(record); err != nil {
		return true, err
	}
	return true, nil
}

func (projection *Projection) Revision(key agentsession.Key) (agentsession.Revision, error) {
	stream, err := projection.stream(key)
	if err != nil || stream == nil {
		return 0, err
	}
	return stream.Recovery.Revision, nil
}

func (projection *Projection) Keys() []agentsession.Key {
	keys := make([]agentsession.Key, 0, len(projection.Streams))
	for _, stream := range projection.Streams {
		key := stream.Key
		key.Attributes = cloneStringMap(key.Attributes)
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, _ := agentsession.CanonicalKey(keys[i])
		right, _ := agentsession.CanonicalKey(keys[j])
		return left < right
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

// ReadRecord resolves only the requested historical body through the canonical
// journal. It is also used by the product Ask display without opening an Agent.
func (projection *Projection) ReadRecord(ctx context.Context, journal *conversationjournal.Journal, key agentsession.Key, revision agentsession.Revision) (agentsession.Record, error) {
	stream, err := projection.stream(key)
	if err != nil {
		return agentsession.Record{}, err
	}
	if stream == nil {
		return agentsession.Record{}, fmt.Errorf("Agent history stream is missing")
	}
	for _, entry := range stream.Recovery.Records {
		if entry.Record.Revision == revision {
			return cloneRecord(entry.Record), nil
		}
	}
	location, ok := stream.Locations[revision]
	if !ok {
		return agentsession.Record{}, fmt.Errorf("Agent history revision %d is missing", revision)
	}
	records, err := journal.ReadRange(ctx, conversationjournal.Range{After: location.Cursor - 1, Through: location.Cursor})
	if err != nil {
		return agentsession.Record{}, err
	}
	for _, physical := range records {
		if physical.Location.Cursor != location.Cursor || physical.Location.RecordIndex != location.RecordIndex {
			continue
		}
		var envelope Envelope
		if err := json.Unmarshal(physical.Payload, &envelope); err != nil {
			return agentsession.Record{}, err
		}
		canonical, _ := agentsession.CanonicalKey(envelope.Key)
		expected, _ := agentsession.CanonicalKey(key)
		if envelope.Revision != revision || canonical != expected || envelope.Deleted {
			return agentsession.Record{}, fmt.Errorf("Agent history locator identity mismatch")
		}
		return agentsession.Record{Revision: envelope.Revision, Kind: envelope.Kind, Version: envelope.Version, Data: envelope.Data}, nil
	}
	return agentsession.Record{}, fmt.Errorf("Agent history locator record is missing")
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
	case "session.input", "session.input_update", "session.control", "turn.checkpoint",
		"turn.tool", "turn.interaction", "turn.interaction_response", "session.task_completion_delivery":
		if !json.Valid(record.Data) {
			return fmt.Errorf("invalid Agent continuation record %q", record.Kind)
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
