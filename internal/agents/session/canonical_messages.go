package session

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/conversationjournal"
	externaljournal "denova/internal/agents/external/journal"
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
	starts := map[string]externaljournal.StartedTool{}
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
			clear(starts)
		case externaljournal.RecordType:
			var event externaljournal.Record
			if err := json.Unmarshal(record.Payload, &event); err != nil {
				return nil, err
			}
			if event.Kind == externaljournal.ToolStarted {
				var started externaljournal.StartedTool
				if err := json.Unmarshal(event.Data, &started); err != nil {
					return nil, err
				}
				starts[event.OperationID+"\x00"+started.ExecutionID] = started
				continue
			}
			if event.Kind != externaljournal.ToolFinished {
				continue
			}
			var finished externaljournal.FinishedTool
			if err := json.Unmarshal(event.Data, &finished); err != nil {
				return nil, err
			}
			key := event.OperationID + "\x00" + finished.ExecutionID
			started, ok := starts[key]
			if !ok {
				return nil, fmt.Errorf("external tool start is missing")
			}
			delete(starts, key)
			// Completed host facts become a matched observation pair for Native.
			// IDs derive from durable host identity, never a vendor continuation.
			id := fmt.Sprintf("external-%x", sha256.Sum256([]byte(finished.ExecutionID)))[:41]
			call := agent.AssistantMessage("", []agent.ToolCall{{ID: id, Type: "function", Function: agent.FunctionCall{Name: started.Tool, Arguments: string(started.Arguments)}}})
			result := agent.TextToolResult(finished.Result)
			if !finished.Success {
				result = agent.ToolErrorResult(finished.Result, finished.Result)
			}
			if finished.Receipt != nil {
				result.Attachments, result.Artifacts = finished.Receipt.Attachments, finished.Receipt.Artifacts
			}
			messages = append(messages, call, agent.ToolMessage(result, id, agent.WithToolName(started.Tool)))
		}
	}
	return messages, nil
}
