package session

import (
	"context"
	"fmt"
	"io"

	"denova/internal/agents/conversationjournal"
)

const sessionExportBatchTransactions = 8

// ExportHistoryJSONL writes every durable domain record in chronological
// order. It is the explicit full-history path: normal UI and model-context
// reads remain bounded. The small fixed transaction batch keeps memory
// independent from the total session length.
func (s *Session) ExportHistoryJSONL(ctx context.Context, destination io.Writer) error {
	if s == nil {
		return fmt.Errorf("session is nil")
	}
	if destination == nil {
		return fmt.Errorf("session export destination is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshCanonicalTailLocked(); err != nil {
		return fmt.Errorf("refresh session before export: %w", err)
	}
	if s.journal == nil {
		return fmt.Errorf("session journal is unavailable")
	}
	through := s.journal.Head().Cursor
	after := conversationjournal.Cursor(0)
	for after < through {
		if err := ctx.Err(); err != nil {
			return err
		}
		records, err := s.journal.ReadRange(ctx, conversationjournal.Range{
			After: after, Through: through, Limit: sessionExportBatchTransactions,
		})
		if err != nil {
			return fmt.Errorf("read session export after cursor %d: %w", after, err)
		}
		if len(records) == 0 {
			return fmt.Errorf("session export stopped before cursor %d", through)
		}
		for _, record := range records {
			if err := writeExportBytes(destination, record.Payload); err != nil {
				return fmt.Errorf("write session export cursor %d: %w", record.Location.Cursor, err)
			}
			if err := writeExportBytes(destination, []byte{'\n'}); err != nil {
				return fmt.Errorf("write session export delimiter: %w", err)
			}
		}
		after = records[len(records)-1].Location.Cursor
	}
	return nil
}

func writeExportBytes(destination io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := destination.Write(data)
		if err != nil {
			return err
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}
