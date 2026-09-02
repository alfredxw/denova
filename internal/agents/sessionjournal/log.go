package sessionjournal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"denova/internal/agents/conversationjournal"

	agentsession "github.com/alfredxw/denova/agent/session"
)

// Log adapts one exact embedded stream to the public Agent storage contract.
// The caller owns the per-Session execution lease passed as release.
type Log struct {
	journal           *conversationjournal.Journal
	projection        *Projection
	key               agentsession.Key
	canonicalMessages bool
	release           func()
	closeOnce         sync.Once
	closed            bool
	mu                sync.Mutex
}

func NewLog(
	journal *conversationjournal.Journal,
	projection *Projection,
	key agentsession.Key,
	canonicalMessages bool,
	release func(),
) (*Log, error) {
	if journal == nil || projection == nil {
		return nil, fmt.Errorf("embedded agent session journal is unavailable")
	}
	normalized, err := agentsession.NormalizeKey(key)
	if err != nil {
		return nil, err
	}
	return &Log{
		journal: journal, projection: projection, key: normalized,
		canonicalMessages: canonicalMessages, release: release,
	}, nil
}

func (log *Log) CanonicalMessages() bool { return log != nil && log.canonicalMessages }

func (log *Log) Replay(ctx context.Context, apply func(agentsession.Record) error) (agentsession.ReplayStats, error) {
	if apply == nil {
		return agentsession.ReplayStats{}, fmt.Errorf("replay embedded agent session: reducer is required")
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.closed {
		return agentsession.ReplayStats{}, agentsession.ErrLogClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// Refresh the physical tail through the journal before reading its derived
	// stream projection. No payload is read from a separate authority.
	if _, err := log.journal.ReadRange(ctx, conversationjournal.Range{After: log.journal.Head().Cursor}); err != nil {
		return agentsession.ReplayStats{}, err
	}
	records, err := log.projection.Records(log.key)
	if err != nil {
		return agentsession.ReplayStats{}, err
	}
	stats := agentsession.ReplayStats{RecordsRead: int64(len(records))}
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		stats.BytesRead += int64(len(record.Kind) + len(record.Data))
		if err := apply(record); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

func (log *Log) Append(ctx context.Context, expected agentsession.Revision, records ...agentsession.Record) (agentsession.Revision, error) {
	log.mu.Lock()
	defer log.mu.Unlock()
	return log.appendLocked(ctx, expected, records...)
}

func (log *Log) appendLocked(ctx context.Context, expected agentsession.Revision, records ...agentsession.Record) (agentsession.Revision, error) {
	if log.closed {
		return 0, agentsession.ErrLogClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for _, record := range records {
		if err := agentsession.ValidateRecord(record); err != nil {
			return 0, err
		}
		if err := validateEmbeddedRecord(record); err != nil {
			return 0, err
		}
	}
	if len(records) == 0 {
		current, err := log.projection.Revision(log.key)
		if err != nil {
			return 0, err
		}
		if current != expected {
			return current, &agentsession.RevisionConflictError{Expected: expected, Actual: current}
		}
		return current, nil
	}
	for {
		current, err := log.projection.Revision(log.key)
		if err != nil {
			return 0, err
		}
		if current != expected {
			return current, &agentsession.RevisionConflictError{Expected: expected, Actual: current}
		}
		payloads := make([]json.RawMessage, len(records))
		next := current
		for index, record := range records {
			next++
			payload, marshalErr := json.Marshal(Envelope{
				Type: RecordType, Key: log.key, Revision: next,
				Kind: record.Kind, Version: record.Version, Data: record.Data,
			})
			if marshalErr != nil {
				return current, marshalErr
			}
			payloads[index] = payload
		}
		head := log.journal.Head()
		_, appendErr := log.journal.Append(ctx, conversationjournal.Guard{
			Cursor: head.Cursor, RecordSHA256: head.RecordSHA256,
		}, payloads...)
		if appendErr == nil {
			return next, nil
		}
		if !errors.Is(appendErr, conversationjournal.ErrConflict) {
			return current, appendErr
		}
		// A product record won the physical cursor. Append refresh already
		// reduced it, so retry if this exact Agent stream is still unchanged.
		if err := ctx.Err(); err != nil {
			return current, err
		}
	}
}

func (log *Log) Close() error {
	if log == nil {
		return nil
	}
	var result error
	log.closeOnce.Do(func() {
		log.mu.Lock()
		log.closed = true
		if log.journal != nil {
			result = log.journal.Close()
		}
		log.mu.Unlock()
		if log.release != nil {
			log.release()
		}
	})
	return result
}

// Delete appends a tombstone to the owning product journal. Replaying the
// JSONL removes all earlier records for this exact logical Agent Session.
func (log *Log) Delete(ctx context.Context) error {
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.closed {
		return agentsession.ErrLogClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		current, err := log.projection.Revision(log.key)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(Envelope{
			Type: RecordType, Key: log.key, Revision: current + 1, Deleted: true,
		})
		if err != nil {
			return err
		}
		head := log.journal.Head()
		_, err = log.journal.Append(ctx, conversationjournal.Guard{
			Cursor: head.Cursor, RecordSHA256: head.RecordSHA256,
		}, payload)
		if err == nil {
			return nil
		}
		if !errors.Is(err, conversationjournal.ErrConflict) {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

var _ agentsession.CanonicalMessageLog = (*Log)(nil)
