package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	runstate "github.com/alfredxw/denova/agent/runtime"
	agentsession "github.com/alfredxw/denova/agent/session"
)

const (
	runtimeRecordKind    = "agent.runtime.event"
	runtimeRecordVersion = 1
)

// runtimeStoreAdapter keeps durable runtime codecs behind the public opaque
// session.Store seam.
type runtimeStoreAdapter struct{ store agentsession.Store }

func (adapter runtimeStoreAdapter) OpenJournal(ctx context.Context, encodedKey string) (runstate.Journal, error) {
	if adapter.store == nil {
		return nil, fmt.Errorf("open Agent Session: Store is nil")
	}
	var binding runstate.BindingRef
	if err := json.Unmarshal([]byte(encodedKey), &binding); err != nil {
		return nil, fmt.Errorf("decode Agent Session binding: %w", err)
	}
	key := agentsession.Key{
		Namespace:  binding.Kind,
		ID:         binding.Key,
		Attributes: maps.Clone(binding.Labels),
	}
	if binding.Profile != "" {
		if key.Attributes == nil {
			key.Attributes = make(map[string]string)
		}
		if _, exists := key.Attributes["agent.profile"]; exists {
			return nil, fmt.Errorf("Agent Session attribute %q is reserved", "agent.profile")
		}
		key.Attributes["agent.profile"] = binding.Profile
	}
	log, err := adapter.store.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	return &runtimeLogAdapter{log: log}, nil
}

type runtimeLogAdapter struct{ log agentsession.Log }

func (adapter *runtimeLogAdapter) Replay(ctx context.Context, reduce func(runstate.Event) error) (runstate.JournalReplayStats, error) {
	if adapter == nil || adapter.log == nil {
		return runstate.JournalReplayStats{}, agentsession.ErrLogClosed
	}
	stats, err := adapter.log.Replay(ctx, func(record agentsession.Record) error {
		if record.Kind != runtimeRecordKind || record.Version != runtimeRecordVersion {
			return fmt.Errorf("unsupported Agent Session record %q version %d", record.Kind, record.Version)
		}
		event, err := runstate.UnmarshalJournalEvent(record.Data)
		if err != nil {
			return err
		}
		if runstate.Cursor(record.Revision) != event.Cursor {
			return fmt.Errorf("Agent Session record revision %d does not match event cursor %d", record.Revision, event.Cursor)
		}
		return reduce(event)
	})
	return runstate.JournalReplayStats{
		BytesRead: stats.BytesRead, RecordsRead: stats.RecordsRead, EventsRead: stats.RecordsRead,
	}, err
}

func (adapter *runtimeLogAdapter) Append(
	ctx context.Context,
	expected runstate.Cursor,
	payloads []runstate.EventPayload,
) ([]runstate.Event, error) {
	if adapter == nil || adapter.log == nil {
		return nil, agentsession.ErrLogClosed
	}
	events := make([]runstate.Event, len(payloads))
	records := make([]agentsession.Record, len(payloads))
	for index, payload := range payloads {
		event := runstate.Event{
			Cursor:     expected + runstate.Cursor(index) + 1,
			Durability: runstate.EventDurable,
			Payload:    payload,
		}
		encoded, err := runstate.MarshalJournalEvent(event)
		if err != nil {
			return nil, err
		}
		events[index] = event
		records[index] = agentsession.Record{
			Kind: runtimeRecordKind, Version: runtimeRecordVersion, Data: encoded,
		}
	}
	revision, err := adapter.log.Append(ctx, agentsession.Revision(expected), records...)
	if err != nil {
		return nil, err
	}
	want := agentsession.Revision(expected) + agentsession.Revision(len(records))
	if revision != want {
		return nil, fmt.Errorf("Agent Session Log returned revision %d, want %d", revision, want)
	}
	return events, nil
}

func (adapter *runtimeLogAdapter) LookupCommand(
	ctx context.Context,
	commandID runstate.CommandID,
) (runstate.CommandRecord, bool, error) {
	var result runstate.CommandRecord
	var found bool
	_, err := adapter.Replay(ctx, func(event runstate.Event) error {
		accepted, ok := event.Payload.(runstate.CommandAcceptedEvent)
		if !ok || accepted.CommandID != commandID {
			return nil
		}
		result = runstate.CommandRecord{
			Receipt: runstate.Receipt{
				CommandID: commandID, OperationID: accepted.OperationID, Cursor: event.Cursor,
			},
			Fingerprint: accepted.Fingerprint,
		}
		found = true
		return nil
	})
	return result, found, err
}

func (adapter *runtimeLogAdapter) Close() error {
	if adapter == nil || adapter.log == nil {
		return nil
	}
	return adapter.log.Close()
}

var _ runstate.JournalStore = runtimeStoreAdapter{}
var _ runstate.Journal = (*runtimeLogAdapter)(nil)
var _ runstate.CommandJournalLookup = (*runtimeLogAdapter)(nil)
