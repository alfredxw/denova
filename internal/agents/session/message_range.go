package session

import (
	"context"
	"encoding/json"
	"fmt"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/conversationjournal"
)

// ReadMessageRange returns the exact canonical logical message range [start,
// end). Unlike MessageWindow, this method is not bounded by the resident
// transcript cache and is therefore the source for checkpoint compaction.
func (s *Session) ReadMessageRange(ctx context.Context, start, end int) ([]*agent.Message, error) {
	if s == nil {
		return nil, fmt.Errorf("session is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshCanonicalTailLocked(); err != nil {
		return nil, fmt.Errorf("refresh canonical message range: %w", err)
	}
	if start < 0 || end < start || end > s.messageCount {
		return nil, fmt.Errorf("invalid message range [%d,%d) for count %d", start, end, s.messageCount)
	}
	if start == end {
		return []*agent.Message{}, nil
	}

	residentEnd := s.messageBaseIndex + len(s.messages)
	if start >= s.messageBaseIndex && end <= residentEnd {
		return cloneMessageRange(s.messages[start-s.messageBaseIndex : end-s.messageBaseIndex]), nil
	}

	startLocation, err := s.resolveMessageLocationLocked(ctx, start)
	if err != nil {
		return nil, err
	}
	endLocation, err := s.resolveMessageLocationLocked(ctx, end-1)
	if err != nil {
		return nil, err
	}
	records, err := s.journal.ReadRange(ctx, conversationjournal.Range{
		After: startLocation.Cursor - 1, Through: endLocation.Cursor,
	})
	if err != nil {
		return nil, err
	}
	result := make([]*agent.Message, 0, end-start)
	for _, record := range records {
		if locationBefore(record.Location, startLocation) || locationBefore(endLocation, record.Location) {
			continue
		}
		message, ok, decodeErr := decodeConversationMessage(record.Payload)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode message at cursor %d record %d: %w", record.Location.Cursor, record.Location.RecordIndex, decodeErr)
		}
		if ok {
			result = append(result, message)
		}
	}
	if len(result) != end-start {
		return nil, fmt.Errorf("canonical message range [%d,%d) resolved %d of %d messages", start, end, len(result), end-start)
	}
	return result, nil
}

func cloneMessageRange(messages []*agent.Message) []*agent.Message {
	result := make([]*agent.Message, len(messages))
	for index, message := range messages {
		result[index] = agent.CloneMessage(message)
	}
	return result
}

func decodeConversationMessage(payload json.RawMessage) (*agent.Message, bool, error) {
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &typed); err != nil {
		return nil, false, err
	}
	switch typed.Type {
	case "":
		var message agent.Message
		if err := json.Unmarshal(payload, &message); err != nil {
			return nil, false, err
		}
		if message.Role == "" && message.Content == "" && len(message.ToolCalls) == 0 {
			return nil, false, nil
		}
		return agent.CloneMessage(&message), true, nil
	case historyTypeMessage, historyTypeContextMessage:
		var record messageRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			return nil, false, err
		}
		return agent.CloneMessage(&record.Message), true, nil
	default:
		return nil, false, nil
	}
}

func locationBefore(left, right conversationjournal.Location) bool {
	return left.Cursor < right.Cursor || (left.Cursor == right.Cursor && left.RecordIndex < right.RecordIndex)
}
