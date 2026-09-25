package session

import (
	"context"
	"encoding/json"
	"errors"

	"denova/internal/agents/conversationjournal"
	externaljournal "denova/internal/agents/runtime/external/journal"
	agent "github.com/alfredxw/denova/agent"
)

// ExternalContextRecord is a canonical content source, not a UI row. Native
// continuation state and reasoning are deliberately excluded at this boundary.
type ExternalContextRecord struct {
	Cursor  conversationjournal.Cursor
	Message *agent.Message
	Runtime *externaljournal.Record
}

// ExternalContextSource captures an immutable interval before admitting new
// input. It can outlive a ReadExternal callback, unlike its journal readers.
// Appends do not extend the interval; clear or journal replacement invalidates it.
type ExternalContextSource struct {
	incarnation string
	after       conversationjournal.Cursor
	through     conversationjournal.Cursor
}

func (s *Session) scanExternalContextLocked(ctx context.Context, source ExternalContextSource, visit func(ExternalContextRecord) error) error {
	if visit == nil || s.journal == nil || s.projection == nil {
		return errors.New("external context visitor requires a canonical journal")
	}
	if source.incarnation != s.journalIncarnation || source.after != s.projection.ClearCursor || source.through > s.materializedCursor || source.after > source.through {
		return ErrContextRevisionConflict
	}
	after, through := source.after, source.through
	for after < through {
		records, err := s.journal.ReadRange(ctx, conversationjournal.Range{After: after, Through: through, Limit: 64})
		if err != nil {
			return err
		}
		// ReadRange refreshes the shared projection. An independent Session can
		// clear between pages even while this handle holds its own Session lock.
		if source.after != s.projection.ClearCursor {
			return ErrContextRevisionConflict
		}
		if len(records) == 0 {
			return errors.New("external context source interval is missing")
		}
		for _, source := range records {
			var typed struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(source.Payload, &typed); err != nil {
				return err
			}
			item := ExternalContextRecord{Cursor: source.Location.Cursor}
			publicMessage := func(message agent.Message) error {
				if message.Role != agent.User && message.Role != agent.Assistant && message.Role != agent.ToolRole {
					return nil
				}
				if message.Content == "" && len(message.Attachments) == 0 {
					return nil
				}
				item.Message = &agent.Message{Role: message.Role, Content: message.Content, Attachments: message.Attachments, ToolName: message.ToolName}
				return visit(item)
			}
			switch typed.Type {
			case "":
				var message agent.Message
				if err := json.Unmarshal(source.Payload, &message); err != nil {
					return err
				}
				if err := publicMessage(message); err != nil {
					return err
				}
				continue
			case historyTypeMessage, historyTypeContextMessage:
				var record messageRecord
				if err := json.Unmarshal(source.Payload, &record); err != nil {
					return err
				}
				if record.SubAgent {
					continue
				}
				message := record.Message
				// Host-only lifecycle/control messages never migrate to a different
				// engine. Canonical public prose, attachments and tool observations do.
				if record.ContextOnly && message.Role != agent.ToolRole {
					continue
				}
				if err := publicMessage(message); err != nil {
					return err
				}
				continue
			case historyTypeContextBatch:
				var batch contextBatchRecord
				if err := json.Unmarshal(source.Payload, &batch); err != nil {
					return err
				}
				for _, message := range batch.Messages {
					if err := publicMessage(message); err != nil {
						return err
					}
				}
				continue
			case externaljournal.RecordType:
				var record externaljournal.Record
				if err := json.Unmarshal(source.Payload, &record); err != nil {
					return err
				}
				item.Runtime = &record
			default:
				continue
			}
			if err := visit(item); err != nil {
				return err
			}
		}
		after = records[len(records)-1].Location.Cursor
	}
	// Also observe a clear committed while the final page was being projected.
	if err := s.refreshCanonicalTailLocked(); err != nil {
		return err
	}
	if source.after != s.projection.ClearCursor {
		return ErrContextRevisionConflict
	}
	return nil
}
