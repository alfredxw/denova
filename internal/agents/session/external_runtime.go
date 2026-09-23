package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/conversationjournal"
	externaljournal "denova/internal/agents/runtime/external/journal"
	agent "github.com/alfredxw/denova/agent"
)

// ExternalState is an immutable projection captured under the canonical
// Session lock. Mutations also use the journal's optimistic commit fence.
// Read resolves content from this same journal, never a sidecar.
type ExternalState struct {
	Incarnation     string
	Cursor          conversationjournal.Cursor
	ContextRevision uint64
	Config          conversationconfig.Snapshot
	Projection      externaljournal.Projection
	Read            func(externaljournal.Locator) (externaljournal.Record, error)
	// ScanContext visits the complete active canonical source interval in bounded
	// physical pages. Invoke only inside the read/prepare callback; never retain it.
	ScanContext func(func(ExternalContextRecord) error) error
}

// ExternalTransaction contains one product message and its execution facts.
// Acceptance uses the user message; settlement preserves final or partial assistant text.
// A tool transition has no canonical message. Prepare callbacks must perform no
// tool calls, network I/O or writes outside this transaction.
type ExternalTransaction struct {
	Records  []externaljournal.Record
	Message  *agent.Message
	Metadata MessageMetadata
}

// UpdateExternal serializes acceptance, tool outcomes and answers with clear,
// configuration updates and other journal mutations. Exact idempotent retries
// return an empty transaction from prepare after consulting the existing facts.
func (s *Session) UpdateExternal(ctx context.Context, expectedRevision uint64, prepare func(ExternalState) (ExternalTransaction, error)) error {
	if prepare == nil {
		return errors.New("external runtime mutation is required")
	}
	return s.withCanonicalMutation(ctx, "update external runtime", func() error {
		if s.runtimeConfig == nil || expectedRevision == 0 || s.runtimeConfigRevision != expectedRevision {
			return conversationconfig.ErrRevisionConflict
		}
		state, err := s.externalStateLocked(ctx)
		if err != nil {
			return err
		}
		change, err := prepare(state)
		if err != nil {
			return err
		}
		if len(change.Records) == 0 {
			if change.Message != nil {
				return errors.New("external message requires an execution fact")
			}
			return nil
		}
		probe := state.Projection
		payloads := make([]any, 0, len(change.Records)+1)
		now := time.Now().UTC()
		if change.Message != nil {
			if change.Metadata.MessageID == "" {
				return errors.New("external message requires a canonical ID")
			}
			metadata := sanitizeMessageMetadata(change.Metadata)
			metadata.ContextRevision = s.contextRevision + 1
			payloads = append(payloads, messageRecord{Type: historyTypeMessage, CreatedAt: now, Message: *change.Message, MessageMetadata: metadata})
		}
		for _, record := range change.Records {
			if record.ConfigRevision != expectedRevision {
				return conversationconfig.ErrRevisionConflict
			}
			if record.Kind == externaljournal.OperationAccepted {
				var accepted externaljournal.Accepted
				if err := json.Unmarshal(record.Data, &accepted); err != nil {
					return err
				}
				if state.Config.Engine().Kind == config.RuntimeNative || !reflect.DeepEqual(accepted.Runtime, state.Config.Engine()) {
					return errors.New("external acceptance must match the active conversation runtime")
				}
			}
			if err := validateExternalMessagePair(change, record); err != nil {
				return err
			}
			body, err := json.Marshal(record)
			if err != nil {
				return err
			}
			if _, err := probe.Apply(conversationjournal.Record{Payload: body, Location: conversationjournal.Location{Cursor: s.materializedCursor + 1, RecordIndex: len(payloads)}}); err != nil {
				return err
			}
			payloads = append(payloads, record)
		}
		commit, err := s.appendJournalRecordsWithUpgradeLocked("external-runtime-v1", payloads...)
		if err != nil {
			return err
		}
		for _, record := range commit.Records {
			if err := appendConversationRecord(s, record); err != nil {
				return fmt.Errorf("materialize external transaction: %w", err)
			}
		}
		return nil
	})
}

func validateExternalMessagePair(change ExternalTransaction, record externaljournal.Record) error {
	switch record.Kind {
	case externaljournal.GuidanceDelivered:
		var delivered externaljournal.DeliveredGuidance
		if err := json.Unmarshal(record.Data, &delivered); err != nil {
			return err
		}
		if change.Message == nil || change.Message.Role != agent.User || delivered.MessageID != change.Metadata.MessageID {
			return errors.New("external guidance must atomically publish its exact user message")
		}
	case externaljournal.OperationAccepted:
		var accepted externaljournal.Accepted
		if err := json.Unmarshal(record.Data, &accepted); err != nil {
			return err
		}
		if change.Message == nil || change.Message.Role != agent.User || accepted.InputMessageID != change.Metadata.MessageID {
			return errors.New("external acceptance must atomically publish its exact user message")
		}
	case externaljournal.OperationClosed:
		var closed externaljournal.Closed
		if err := json.Unmarshal(record.Data, &closed); err != nil {
			return err
		}
		if (closed.Status == externaljournal.Completed || closed.MessageID != "") && (change.Message == nil || change.Message.Role != agent.Assistant || closed.MessageID != change.Metadata.MessageID) {
			return errors.New("external completion must atomically publish its exact assistant message")
		}
	}
	return nil
}

// ReadExternal captures current facts while refreshing any independently
// committed answer. The callback executes under a read of the canonical state;
// retain the returned values, never the journal-bound Read function.
func (s *Session) ReadExternal(ctx context.Context, read func(ExternalState) error) error {
	if read == nil {
		return errors.New("external runtime reader is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshCanonicalTailLocked(); err != nil {
		return err
	}
	state, err := s.externalStateLocked(ctx)
	if err != nil {
		return err
	}
	return read(state)
}

func (s *Session) externalStateLocked(ctx context.Context) (ExternalState, error) {
	if s.projection == nil || s.journal == nil {
		return ExternalState{}, errors.New("external journal is unavailable")
	}
	// Copy metadata so a failed prepare cannot mutate the live index before
	// its transaction commits. Content remains at canonical record locations.
	body, err := json.Marshal(s.projection.External)
	if err != nil {
		return ExternalState{}, err
	}
	var projection externaljournal.Projection
	if err := json.Unmarshal(body, &projection); err != nil {
		return ExternalState{}, err
	}
	selection, _ := s.runtimeConfigLocked()
	return ExternalState{Incarnation: s.journalIncarnation, Cursor: s.materializedCursor, ContextRevision: s.contextRevision, Config: selection, Projection: projection, ScanContext: func(visit func(ExternalContextRecord) error) error { return s.scanExternalContextLocked(ctx, visit) }, Read: func(locator externaljournal.Locator) (externaljournal.Record, error) {
		if locator.Cursor == 0 || locator.Index < 0 {
			return externaljournal.Record{}, errors.New("invalid external journal locator")
		}
		records, err := s.journal.ReadRange(ctx, conversationjournal.Range{After: locator.Cursor - 1, Through: locator.Cursor})
		if err != nil {
			return externaljournal.Record{}, err
		}
		for _, source := range records {
			if source.Location.Cursor != locator.Cursor || source.Location.RecordIndex != locator.Index {
				continue
			}
			var record externaljournal.Record
			if err := json.Unmarshal(source.Payload, &record); err != nil {
				return externaljournal.Record{}, err
			}
			return record, record.Validate()
		}
		return externaljournal.Record{}, errors.New("external journal record is missing")
	}}, nil
}
