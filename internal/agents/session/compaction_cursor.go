package session

import (
	"context"
	"encoding/json"
	"fmt"

	"denova/internal/conversationjournal"
)

const compactionCursorScanBatch = 128

// resolveMessageCursorLocked resolves a legacy zero-based message index from
// sparse message anchors. It is used only by compaction maintenance and legacy
// index migration, never by ordinary append or recent-history reads.
func (s *Session) resolveMessageCursorLocked(ctx context.Context, index int) (conversationjournal.Cursor, error) {
	if index < 0 || s.projection == nil || s.journal == nil {
		return 0, fmt.Errorf("cannot resolve message cursor at index %d", index)
	}
	if cursor := s.projection.messageCursorAt(index); cursor > 0 {
		return cursor, nil
	}
	if index >= s.projection.MessageCount {
		return 0, fmt.Errorf("message index %d exceeds count %d", index, s.projection.MessageCount)
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
			return 0, err
		}
		if len(records) == 0 {
			break
		}
		for _, record := range records {
			message, err := isConversationMessagePayload(record.Payload)
			if err != nil {
				return 0, fmt.Errorf("inspect message cursor %d: %w", record.Location.Cursor, err)
			}
			if !message {
				continue
			}
			if nextIndex == index {
				return record.Location.Cursor, nil
			}
			nextIndex++
		}
		after = records[len(records)-1].Location.Cursor
	}
	return 0, fmt.Errorf("message cursor at index %d was not found", index)
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
