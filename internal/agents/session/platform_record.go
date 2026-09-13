package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"denova/internal/agents/conversationjournal"
)

const platformRecordType = "platform_session"

// platformRecord stores the platform's frozen configuration or command receipt
// in the Product Session that also owns the Agent transcript. The platform owns
// payload interpretation; the journal owns versioning and compare-and-swap.
type platformRecord struct {
	Type    string          `json:"type"`
	Version int             `json:"version"`
	Key     string          `json:"key"`
	Value   json.RawMessage `json:"value"`
}

func decodePlatformRecord(raw []byte) (platformRecord, error) {
	var record platformRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return record, err
	}
	if record.Type != platformRecordType || record.Version != 1 || (record.Key != "configuration" && !strings.HasPrefix(record.Key, "request/")) || len(record.Key) > 256 || len(record.Value) > 1<<20 || !json.Valid(record.Value) {
		return record, fmt.Errorf("invalid platform Session record")
	}
	return record, nil
}

// PlatformRecord reads an indexed record from its canonical journal location.
// A missing index is reconstructed by the ordinary Product Session replay.
func (s *Session) PlatformRecord(ctx context.Context, key string) (json.RawMessage, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshCanonicalTailLocked(); err != nil {
		return nil, 0, err
	}
	return s.platformRecordLocked(ctx, key)
}

func (s *Session) platformRecordLocked(ctx context.Context, key string) (json.RawMessage, uint64, error) {
	cursor := s.projection.PlatformRecords[key]
	if cursor == 0 {
		return nil, 0, nil
	}
	records, err := s.journal.ReadRange(ctx, conversationjournal.Range{After: cursor - 1, Through: cursor})
	if err != nil {
		return nil, 0, err
	}
	for _, item := range records {
		var discriminator struct {
			Type string `json:"type"`
			Key  string `json:"key"`
		}
		if err := json.Unmarshal(item.Payload, &discriminator); err != nil {
			return nil, 0, err
		}
		if discriminator.Type == platformRecordType && discriminator.Key == key {
			record, err := decodePlatformRecord(item.Payload)
			return record.Value, uint64(cursor), err
		}
	}
	return nil, 0, fmt.Errorf("platform Session record locator is invalid")
}

// SetPlatformRecord durably admits a request before model or tool execution.
// expected=0 creates only; later replacements require the returned revision.
func (s *Session) SetPlatformRecord(ctx context.Context, key string, expected uint64, value json.RawMessage) (uint64, error) {
	record := platformRecord{Type: platformRecordType, Version: 1, Key: key, Value: value}
	raw, err := json.Marshal(record)
	if err != nil {
		return 0, err
	}
	if _, err := decodePlatformRecord(raw); err != nil {
		return 0, err
	}
	var revision uint64
	err = s.withCanonicalMutation(ctx, "commit platform Session record", func() error {
		if uint64(s.projection.PlatformRecords[key]) != expected {
			return fmt.Errorf("%w: platform Session record changed", conversationjournal.ErrConflict)
		}
		commit, err := s.appendJournalRecordsLocked(record)
		if err != nil {
			return err
		}
		revision = uint64(commit.Head.Cursor)
		return nil
	})
	return revision, err
}
