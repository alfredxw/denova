package conversationjournal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"denova/internal/localfs"
)

// Append durably publishes one transaction. The reducer is updated only after
// the canonical append succeeds, and index failures never roll back the log.
func (journal *Journal) Append(ctx context.Context, guard Guard, payloads ...json.RawMessage) (Commit, error) {
	if journal == nil {
		return Commit{}, fmt.Errorf("conversation journal is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if len(payloads) == 0 {
		return Commit{}, fmt.Errorf("conversation journal append requires at least one record")
	}
	for index, payload := range payloads {
		if !json.Valid(payload) {
			return Commit{}, fmt.Errorf("conversation journal append record %d is invalid JSON", index+1)
		}
	}
	release, err := localfs.AcquireLease(ctx, journal.path+".domain.lock")
	if err != nil {
		return Commit{}, fmt.Errorf("acquire conversation journal append lease: %w", err)
	}
	defer release()
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.closed {
		return Commit{}, fmt.Errorf("conversation journal is closed")
	}
	if journal.projectionInvalid {
		return Commit{}, fmt.Errorf("conversation projection is invalid; reopen the journal")
	}
	if err := journal.refreshLocked(ctx, true); err != nil {
		return Commit{}, err
	}
	if guard.Cursor != journal.head.Cursor || (guard.RecordSHA256 != "" && guard.RecordSHA256 != journal.head.RecordSHA256) {
		return Commit{}, &ConflictError{Expected: guard, Actual: journal.head}
	}
	nextCursor := journal.head.Cursor + 1
	line, err := encodeTransaction(journal.identity, nextCursor, journal.head.RecordSHA256, payloads)
	if err != nil {
		return Commit{}, err
	}
	location, nextOffset, appendErr := appendAndSync(journal.path, journal.validOffset, journal.needsNewline, line)
	if appendErr != nil {
		return Commit{}, fmt.Errorf("append conversation journal: %w", appendErr)
	}
	location.Cursor = nextCursor
	location.PreviousRecordSHA256 = journal.head.RecordSHA256
	if err := journal.applyLineLocked(line, location.Offset, location.Length, nextOffset, true); err != nil {
		journal.projectionInvalid = true
		return Commit{}, fmt.Errorf("conversation transaction committed but projection failed: %w", err)
	}
	journal.journalSize = nextOffset
	journal.dirtyTransactions++
	if journal.dirtyTransactions >= journal.options.FlushEvery {
		if err := journal.persistIndexLocked(); err != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("[conversation-journal] index checkpoint deferred path=%s cursor=%d error=%v", journal.indexPath, journal.head.Cursor, err))
		}
	}
	records := make([]Record, len(payloads))
	for index, payload := range payloads {
		records[index] = Record{
			Location: location, Payload: append(json.RawMessage(nil), payload...),
		}
		records[index].Location.RecordIndex = index
	}
	return Commit{Head: journal.head, Records: records}, nil
}

func (journal *Journal) refreshLocked(ctx context.Context, repair bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	initialSHA, err := firstRecordSHA256(journal.path)
	if err != nil {
		return err
	}
	if journal.initialRecordSHA256 != "" && initialSHA != journal.initialRecordSHA256 {
		return fmt.Errorf("conversation journal incarnation changed: %s", journal.path)
	}
	info, err := os.Stat(journal.path)
	if err != nil {
		return err
	}
	if info.Size() < journal.validOffset {
		return fmt.Errorf("conversation journal shrank: indexed=%d actual=%d", journal.validOffset, info.Size())
	}
	if len(journal.tornTail) > 0 && repair {
		if info.Size() != journal.journalSize {
			return fmt.Errorf("conversation journal changed after a torn tail was observed")
		}
		if err := preserveAndTruncateTail(journal.path, journal.validOffset, journal.tornTail); err != nil {
			return err
		}
		journal.journalSize = journal.validOffset
		journal.tornTail = nil
		journal.needsNewline = false
		info, err = os.Stat(journal.path)
		if err != nil {
			return err
		}
	}
	if info.Size() == journal.journalSize && journal.validOffset == journal.journalSize {
		return nil
	}
	return journal.scanLocked(ctx, journal.validOffset)
}

// RepairTail preserves and removes only a syntactically incomplete final
// record. Complete JSON or checksum corruption remains a hard error.
func (journal *Journal) RepairTail(ctx context.Context) error {
	if journal == nil {
		return fmt.Errorf("conversation journal is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := localfs.AcquireLease(ctx, journal.path+".domain.lock")
	if err != nil {
		return err
	}
	defer release()
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.closed {
		return fmt.Errorf("conversation journal is closed")
	}
	if err := journal.refreshLocked(ctx, true); err != nil {
		return err
	}
	// A newly observed malformed EOF is discovered by the first scan; the
	// second pass performs the already-validated preserve-and-truncate step.
	if len(journal.tornTail) > 0 {
		if err := journal.refreshLocked(ctx, true); err != nil {
			return err
		}
	}
	if journal.indexDirty {
		if err := journal.persistIndexLocked(); err != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("[conversation-journal] repaired tail but index checkpoint failed path=%s error=%v", journal.indexPath, err))
		}
	}
	return nil
}

// Close flushes the derived index. The canonical JSONL remains valid even if
// the sidecar flush fails.
func (journal *Journal) Close() (resultErr error) {
	if journal == nil {
		return nil
	}
	release, err := localfs.AcquireLease(context.Background(), journal.path+".domain.lock")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, release()) }()
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.closed {
		return nil
	}
	if err := journal.refreshLocked(context.Background(), false); err != nil {
		return err
	}
	journal.closed = true
	if !journal.indexDirty {
		return nil
	}
	return journal.persistIndexLocked()
}
