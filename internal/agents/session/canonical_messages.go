package session

import (
	"context"
	"encoding/json"
	"fmt"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/conversationjournal"
)

// ReadCanonicalMessages rebuilds the complete model-visible lane after the
// latest clear marker. The Session's resident window is intentionally bounded
// for UI work, so Agent recovery must read the canonical JSONL instead of
// treating that window as the complete transcript.
func (s *Session) ReadCanonicalMessages(ctx context.Context) ([]*agent.Message, error) {
	if s == nil {
		return nil, fmt.Errorf("session is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshCanonicalTailLocked(); err != nil {
		return nil, fmt.Errorf("refresh canonical messages: %w", err)
	}
	if s.journal == nil || s.projection == nil {
		return nil, fmt.Errorf("session canonical journal is unavailable")
	}

	// Start immediately before the projected clear cursor so its transaction is
	// included. Replaying the clear record itself remains correct if one physical
	// transaction ever contains records on both sides of that marker.
	after := conversationjournal.Cursor(0)
	if s.projection.ClearCursor > 0 {
		after = s.projection.ClearCursor - 1
	}
	through := s.journal.Head().Cursor
	records, err := s.journal.ReadRange(ctx, conversationjournal.Range{After: after, Through: through})
	if err != nil {
		return nil, fmt.Errorf("read canonical message range: %w", err)
	}
	messages := make([]*agent.Message, 0)
	for _, record := range records {
		var typed struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(record.Payload, &typed); err != nil {
			return nil, fmt.Errorf("decode canonical message type at cursor %d: %w", record.Location.Cursor, err)
		}
		switch typed.Type {
		case "":
			var message agent.Message
			if err := json.Unmarshal(record.Payload, &message); err != nil {
				return nil, fmt.Errorf("decode legacy canonical message at cursor %d: %w", record.Location.Cursor, err)
			}
			messages = append(messages, message.Clone())
		case historyTypeMessage, historyTypeContextMessage:
			var persisted messageRecord
			if err := json.Unmarshal(record.Payload, &persisted); err != nil {
				return nil, fmt.Errorf("decode canonical message at cursor %d: %w", record.Location.Cursor, err)
			}
			messages = append(messages, persisted.Message.Clone())
		case historyTypeContextBatch:
			var batch contextBatchRecord
			if err := json.Unmarshal(record.Payload, &batch); err != nil {
				return nil, fmt.Errorf("decode canonical context batch at cursor %d: %w", record.Location.Cursor, err)
			}
			for index := range batch.Messages {
				messages = append(messages, batch.Messages[index].Clone())
			}
		case historyTypeClear:
			messages = messages[:0]
		}
	}
	return messages, nil
}
