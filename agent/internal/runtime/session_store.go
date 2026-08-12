package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/alfredxw/denova/agent/session"
)

const (
	sessionRuntimeRecordKind    = "agent.runtime.event"
	sessionRuntimeRecordVersion = 1
)

// NewSessionJournalStore adapts the public opaque session.Store seam to the
// private runtime Journal contract. The public Log remains the only durable
// authority; built-in Logs may expose private, rebuildable acceleration.
func NewSessionJournalStore(store session.Store) JournalStore {
	return sessionJournalStore{store: store}
}

type sessionJournalStore struct{ store session.Store }

func (store sessionJournalStore) OpenJournal(ctx context.Context, encodedKey string) (Journal, error) {
	if store.store == nil {
		return nil, fmt.Errorf("open Agent Session: Store is nil")
	}
	var binding BindingRef
	if err := json.Unmarshal([]byte(encodedKey), &binding); err != nil {
		return nil, fmt.Errorf("decode Agent Session binding: %w", err)
	}
	key := session.Key{Namespace: binding.Kind, ID: binding.Key, Attributes: maps.Clone(binding.Labels)}
	if binding.Profile != "" {
		if key.Attributes == nil {
			key.Attributes = make(map[string]string)
		}
		if _, exists := key.Attributes["agent.profile"]; exists {
			return nil, fmt.Errorf("Agent Session attribute %q is reserved", "agent.profile")
		}
		key.Attributes["agent.profile"] = binding.Profile
	}
	log, err := store.store.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	return &sessionJournal{log: log}, nil
}

type sessionJournal struct{ log session.Log }

type sessionCheckpointLog interface {
	ReplayRuntimeCheckpoint(context.Context, JournalCheckpointState) (JournalReplayStats, error)
	MaybeRuntimeCheckpoint(context.Context, JournalCheckpointState) error
}

type sessionCommandLog interface {
	LookupRuntimeCommand(context.Context, CommandID) (CommandRecord, bool, error)
}

func (journal *sessionJournal) ReplayCheckpoint(ctx context.Context, state JournalCheckpointState) (JournalReplayStats, error) {
	if accelerated, ok := journal.log.(sessionCheckpointLog); ok {
		return accelerated.ReplayRuntimeCheckpoint(ctx, state)
	}
	return journal.Replay(ctx, state.Reduce)
}

func (journal *sessionJournal) MaybeCheckpoint(ctx context.Context, state JournalCheckpointState) error {
	if accelerated, ok := journal.log.(sessionCheckpointLog); ok {
		return accelerated.MaybeRuntimeCheckpoint(ctx, state)
	}
	return nil
}

func (journal *sessionJournal) Replay(ctx context.Context, reduce func(Event) error) (JournalReplayStats, error) {
	if journal == nil || journal.log == nil {
		return JournalReplayStats{}, session.ErrLogClosed
	}
	stats, err := journal.log.Replay(ctx, func(record session.Record) error {
		event, err := decodeSessionRuntimeRecord(record)
		if err != nil {
			return err
		}
		return reduce(event)
	})
	return JournalReplayStats{
		BytesRead: stats.BytesRead, RecordsRead: stats.RecordsRead, EventsRead: stats.RecordsRead,
	}, err
}

func (journal *sessionJournal) Append(ctx context.Context, expected Cursor, payloads []EventPayload) ([]Event, error) {
	if journal == nil || journal.log == nil {
		return nil, session.ErrLogClosed
	}
	events := make([]Event, len(payloads))
	records := make([]session.Record, len(payloads))
	for index, payload := range payloads {
		event := Event{Cursor: expected + Cursor(index) + 1, Durability: EventDurable, Payload: payload}
		encoded, err := MarshalJournalEvent(event)
		if err != nil {
			return nil, err
		}
		events[index] = event
		records[index] = session.Record{Kind: sessionRuntimeRecordKind, Version: sessionRuntimeRecordVersion, Data: encoded}
	}
	revision, err := journal.log.Append(ctx, session.Revision(expected), records...)
	if err != nil {
		return nil, err
	}
	want := session.Revision(expected) + session.Revision(len(records))
	if revision != want {
		return nil, fmt.Errorf("Agent Session Log returned revision %d, want %d", revision, want)
	}
	return events, nil
}

func (journal *sessionJournal) LookupCommand(ctx context.Context, commandID CommandID) (CommandRecord, bool, error) {
	if accelerated, ok := journal.log.(sessionCommandLog); ok {
		return accelerated.LookupRuntimeCommand(ctx, commandID)
	}
	var result CommandRecord
	var found bool
	_, err := journal.Replay(ctx, func(event Event) error {
		accepted, ok := event.Payload.(CommandAcceptedEvent)
		if ok && accepted.CommandID == commandID {
			result = CommandRecord{
				Receipt:     Receipt{CommandID: commandID, OperationID: accepted.OperationID, Cursor: event.Cursor},
				Fingerprint: accepted.Fingerprint,
			}
			found = true
		}
		return nil
	})
	return result, found, err
}

func (journal *sessionJournal) Close() error {
	if journal == nil || journal.log == nil {
		return nil
	}
	return journal.log.Close()
}

func decodeSessionRuntimeRecord(record session.Record) (Event, error) {
	if record.Kind != sessionRuntimeRecordKind || record.Version != sessionRuntimeRecordVersion {
		return Event{}, fmt.Errorf("unsupported Agent Session record %q version %d", record.Kind, record.Version)
	}
	event, err := UnmarshalJournalEvent(record.Data)
	if err != nil {
		return Event{}, err
	}
	if Cursor(record.Revision) != event.Cursor {
		return Event{}, fmt.Errorf("Agent Session record revision %d does not match event cursor %d", record.Revision, event.Cursor)
	}
	return event, nil
}

var _ JournalStore = sessionJournalStore{}
var _ Journal = (*sessionJournal)(nil)
var _ CommandJournalLookup = (*sessionJournal)(nil)
var _ checkpointJournal = (*sessionJournal)(nil)
