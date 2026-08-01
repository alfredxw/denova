package session

import (
	"context"
	"encoding/json"
	"fmt"

	"denova/internal/agents/conversationjournal"
)

const compactionCursorScanBatch = 128

// resolveMessageLocationLocked resolves a zero-based logical message index
// from sparse anchors. RecordIndex is part of the identity because one
// physical journal transaction may contain more than one domain message.
func (s *Session) resolveMessageLocationLocked(ctx context.Context, index int) (conversationjournal.Location, error) {
	if index < 0 || s.projection == nil || s.journal == nil {
		return conversationjournal.Location{}, fmt.Errorf("cannot resolve message location at index %d", index)
	}
	if location := s.projection.messageLocationAt(index); location.Cursor > 0 {
		return location, nil
	}
	if index >= s.projection.MessageCount {
		return conversationjournal.Location{}, fmt.Errorf("message index %d exceeds count %d", index, s.projection.MessageCount)
	}
	anchor := s.projection.messageAnchorAt(index)
	after := anchor.Cursor - 1
	nextIndex := anchor.Index
	through := s.journal.Head().Cursor
	for after < through {
		records, err := s.journal.ReadRange(ctx, conversationjournal.Range{
			After: after, Through: through, Limit: compactionCursorScanBatch,
		})
		if err != nil {
			return conversationjournal.Location{}, err
		}
		if len(records) == 0 {
			break
		}
		for _, record := range records {
			if record.Location.Cursor == anchor.Cursor && record.Location.RecordIndex < anchor.RecordIndex {
				continue
			}
			message, err := isConversationMessagePayload(record.Payload)
			if err != nil {
				return conversationjournal.Location{}, fmt.Errorf("inspect message cursor %d: %w", record.Location.Cursor, err)
			}
			if !message {
				continue
			}
			if nextIndex == index {
				return record.Location, nil
			}
			nextIndex++
		}
		after = records[len(records)-1].Location.Cursor
	}
	return conversationjournal.Location{}, fmt.Errorf("message location at index %d was not found", index)
}

// resolveMessageCursorLocked preserves the cursor-only compaction record wire
// format while all logical reads use the precise journal location above.
func (s *Session) resolveMessageCursorLocked(ctx context.Context, index int) (conversationjournal.Cursor, error) {
	location, err := s.resolveMessageLocationLocked(ctx, index)
	return location.Cursor, err
}

func isConversationMessagePayload(payload json.RawMessage) (bool, error) {
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &typed); err != nil {
		return false, err
	}
	return typed.Type == "" || typed.Type == historyTypeMessage || typed.Type == historyTypeContextMessage, nil
}

func (s *Session) migrateProjectionCompactionCursorsLocked(ctx context.Context) error {
	if s.projection == nil {
		return nil
	}
	for index := range s.projection.Structural {
		structural := &s.projection.Structural[index]
		if structural.Compaction != nil {
			value := *structural.Compaction
			if err := s.fillCompactionCursorsLocked(ctx, &value); err != nil {
				return err
			}
			structural.Compaction = &value
		}
		if structural.Removal != nil {
			value := *structural.Removal
			if value.SourceStartCursor == 0 && value.SourceStartIndex < value.SourceEndIndex {
				cursor, err := s.resolveMessageCursorLocked(ctx, value.SourceStartIndex)
				if err != nil {
					return err
				}
				value.SourceStartCursor = cursor
			}
			if value.SourceEndCursor == 0 && value.SourceEndIndex > 0 {
				cursor, err := s.resolveMessageCursorLocked(ctx, value.SourceEndIndex-1)
				if err != nil {
					return err
				}
				value.SourceEndCursor = cursor
			}
			structural.Removal = &value
		}
	}
	return nil
}

func (s *Session) fillCompactionCursorsLocked(ctx context.Context, record *ContextCompaction) error {
	if record.SourceStartCursor == 0 && record.SourceStartIndex < record.SourceEndIndex {
		cursor, err := s.resolveMessageCursorLocked(ctx, record.SourceStartIndex)
		if err != nil {
			return err
		}
		record.SourceStartCursor = cursor
	}
	if record.SourceEndCursor == 0 && record.SourceEndIndex > 0 {
		cursor, err := s.resolveMessageCursorLocked(ctx, record.SourceEndIndex-1)
		if err != nil {
			return err
		}
		record.SourceEndCursor = cursor
	}
	return nil
}
