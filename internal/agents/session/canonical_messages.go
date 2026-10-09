package session

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"

	"denova/internal/agents/conversationjournal"
	externaljournal "denova/internal/agents/runtime/external/journal"

	agentschema "github.com/alfredxw/denova/agent/schema"
	agentcanonical "github.com/alfredxw/denova/agent/session/canonical"
	agenttool "github.com/alfredxw/denova/agent/tool"
)

// CanonicalHistoryHead reads only the reducer. Clear and journal replacement
// start a different lane; ordinary message appends advance its existing revision.
func (s *Session) CanonicalHistoryHead(ctx context.Context) (agentcanonical.CanonicalHistoryHead, error) {
	if err := ctx.Err(); err != nil {
		return agentcanonical.CanonicalHistoryHead{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshCanonicalTailLocked(); err != nil {
		return agentcanonical.CanonicalHistoryHead{}, err
	}
	if s.journal == nil || s.projection == nil {
		return agentcanonical.CanonicalHistoryHead{}, fmt.Errorf("session canonical journal is unavailable")
	}
	head := s.journal.Head()
	return agentcanonical.CanonicalHistoryHead{
		Identity: fmt.Sprintf("%s/%s/%d", head.Identity.ID, head.Identity.Generation, s.projection.ClearCursor),
		Revision: strconv.FormatUint(s.contextCursorLocked().Revision, 10),
	}, nil
}

// ReadCanonicalMessages rebuilds the complete model-visible lane after the
// latest clear marker. The Session's resident window is intentionally bounded
// for UI work, so Agent recovery must read the canonical JSONL instead of
// treating that window as the complete transcript.
func (s *Session) ReadCanonicalMessages(ctx context.Context) ([]*agentschema.Message, error) {
	messages := make([]*agentschema.Message, 0)
	err := s.VisitCanonicalMessages(ctx, func(message *agentschema.Message) error {
		messages = append(messages, message)
		return nil
	})
	return messages, err
}

// VisitCanonicalMessages streams detached messages after the latest Clear.
// It never retains diagnostic or checkpoint payloads alongside the transcript.
func (s *Session) VisitCanonicalMessages(ctx context.Context, visit func(*agentschema.Message) error) error {
	if s == nil {
		return fmt.Errorf("session is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshCanonicalTailLocked(); err != nil {
		return fmt.Errorf("refresh canonical messages: %w", err)
	}
	if s.journal == nil || s.projection == nil {
		return fmt.Errorf("session canonical journal is unavailable")
	}

	// Start immediately before the projected clear cursor so its transaction is
	// included. Replaying the clear record itself remains correct if one physical
	// transaction ever contains records on both sides of that marker.
	after := conversationjournal.Cursor(0)
	if s.projection.ClearCursor > 0 {
		after = s.projection.ClearCursor - 1
	}
	through := s.journal.Head().Cursor
	starts := map[string]externaljournal.StartedTool{}
	afterClear := s.projection.ClearCursor == 0
	return s.journal.VisitRange(ctx, conversationjournal.Range{After: after, Through: through}, func(record conversationjournal.Record) error {
		var typed struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(record.Payload, &typed); err != nil {
			return fmt.Errorf("decode canonical message type at cursor %d: %w", record.Location.Cursor, err)
		}
		if !afterClear {
			afterClear = typed.Type == historyTypeClear
			return nil
		}
		switch typed.Type {
		case "":
			var message agentschema.Message
			if err := json.Unmarshal(record.Payload, &message); err != nil {
				return fmt.Errorf("decode legacy canonical message at cursor %d: %w", record.Location.Cursor, err)
			}
			return visit(&message)
		case historyTypeMessage, historyTypeContextMessage:
			var persisted messageRecord
			if err := json.Unmarshal(record.Payload, &persisted); err != nil {
				return fmt.Errorf("decode canonical message at cursor %d: %w", record.Location.Cursor, err)
			}
			return visit(&persisted.Message)
		case historyTypeContextBatch:
			var batch contextBatchRecord
			if err := json.Unmarshal(record.Payload, &batch); err != nil {
				return fmt.Errorf("decode canonical context batch at cursor %d: %w", record.Location.Cursor, err)
			}
			// Retaining one message must not pin the array containing the rest
			// of this batch, which may belong to the archived prefix.
			for _, message := range batch.Messages {
				if err := visit(&message); err != nil {
					return err
				}
			}
		case historyTypeClear:
			clear(starts)
		case externaljournal.RecordType:
			var event externaljournal.Record
			if err := json.Unmarshal(record.Payload, &event); err != nil {
				return err
			}
			if event.Kind == externaljournal.ToolStarted {
				var started externaljournal.StartedTool
				if err := json.Unmarshal(event.Data, &started); err != nil {
					return err
				}
				starts[event.OperationID+"\x00"+started.ExecutionID] = started
				return nil
			}
			if event.Kind != externaljournal.ToolFinished {
				return nil
			}
			var finished externaljournal.FinishedTool
			if err := json.Unmarshal(event.Data, &finished); err != nil {
				return err
			}
			key := event.OperationID + "\x00" + finished.ExecutionID
			started, ok := starts[key]
			if !ok {
				return fmt.Errorf("external tool start is missing")
			}
			delete(starts, key)
			// Completed host facts become a matched observation pair for Native.
			// IDs derive from durable host identity, never a vendor continuation.
			id := fmt.Sprintf("external-%x", sha256.Sum256([]byte(finished.ExecutionID)))[:41]
			call := agentschema.AssistantMessage("", []agentschema.ToolCall{{ID: id, Type: "function", Function: agentschema.FunctionCall{Name: started.Tool, Arguments: string(started.Arguments)}}})
			result := agentschema.TextToolResult(finished.Result)
			if !finished.Success {
				result = agenttool.ToolErrorResult(finished.Result, finished.Result)
			}
			if finished.Receipt != nil {
				result.Attachments, result.Artifacts = finished.Receipt.Attachments, finished.Receipt.Artifacts
			}
			if err := visit(call); err != nil {
				return err
			}
			return visit(agentschema.ToolMessage(result, id, agentschema.WithToolName(started.Tool)))
		}
		return nil
	})
}
