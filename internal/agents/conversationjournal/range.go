package conversationjournal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"denova/internal/localfs"
)

// ReadRange reads a stable physical range without touching the domain
// projection. Domain adapters use their own logical locators to choose cursors.
func (journal *Journal) ReadRange(ctx context.Context, selected Range) ([]Record, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := journal.lockForRead(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	through := selected.Through
	if through == 0 || through > journal.head.Cursor {
		through = journal.head.Cursor
	}
	if selected.After >= through {
		return []Record{}, nil
	}
	limit := selected.Limit
	if limit <= 0 {
		limit = int(through - selected.After)
	}
	result, bytesRead, err := journal.readFromAnchorLocked(ctx, selected.After+1, func(cursor Cursor) bool {
		return cursor > selected.After && cursor <= through
	}, through, limit, nil)
	journal.stats.LastRangeBytesRead = bytesRead
	return result, err
}

// VisitRange holds a stable read lease and visits one record at a time. Payload
// bytes belong to the scan; copy them if retained. The visitor must not reenter
// this journal. Unlike ReadRange, memory does not grow with the selected range.
func (journal *Journal) VisitRange(ctx context.Context, selected Range, visit func(Record) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := journal.lockForRead(ctx)
	if err != nil {
		return err
	}
	defer release()
	through := selected.Through
	if through == 0 || through > journal.head.Cursor {
		through = journal.head.Cursor
	}
	if selected.After >= through {
		return nil
	}
	limit := selected.Limit
	if limit <= 0 {
		limit = int(through - selected.After)
	}
	_, bytesRead, err := journal.readFromAnchorLocked(ctx, selected.After+1, func(cursor Cursor) bool {
		return cursor > selected.After && cursor <= through
	}, through, limit, visit)
	journal.stats.LastRangeBytesRead = bytesRead
	return err
}

// ReadTransactions reads the given transactions in cursor order with one
// forward scan. Opening a long session restores up to 200 older message
// transactions; reading each separately rescanned from its sparse anchor.
func (journal *Journal) ReadTransactions(ctx context.Context, cursors []Cursor) ([]Record, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := journal.lockForRead(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	wanted := map[Cursor]bool{}
	last := Cursor(0)
	first := Cursor(0)
	for _, cursor := range cursors {
		if cursor == 0 || cursor > journal.head.Cursor {
			return nil, fmt.Errorf("conversation journal transaction %d is outside the journal", cursor)
		}
		wanted[cursor] = true
		last = max(last, cursor)
		if first == 0 || cursor < first {
			first = cursor
		}
	}
	if len(wanted) == 0 {
		return []Record{}, nil
	}
	result, bytesRead, err := journal.readFromAnchorLocked(ctx, first, func(cursor Cursor) bool { return wanted[cursor] }, last, len(wanted), nil)
	journal.stats.LastRangeBytesRead = bytesRead
	if err != nil {
		return nil, err
	}
	found := map[Cursor]bool{}
	for _, record := range result {
		found[record.Location.Cursor] = true
	}
	if len(found) != len(wanted) {
		missing := make([]Cursor, 0, len(wanted))
		for cursor := range wanted {
			if !found[cursor] {
				missing = append(missing, cursor)
			}
		}
		slices.Sort(missing)
		return nil, fmt.Errorf("conversation journal transactions missing: %v", missing)
	}
	return result, nil
}

// lockForRead takes the domain lease and the handle lock and refreshes the
// head. The returned release must run once the read is complete.
func (journal *Journal) lockForRead(ctx context.Context) (func(), error) {
	if journal == nil {
		return nil, fmt.Errorf("conversation journal is nil")
	}
	releaseLease, err := localfs.AcquireLease(ctx, journal.path+".domain.lock")
	if err != nil {
		return nil, err
	}
	journal.mu.Lock()
	release := func() {
		journal.mu.Unlock()
		releaseLease()
	}
	if journal.closed {
		release()
		return nil, fmt.Errorf("conversation journal is closed")
	}
	if err := journal.refreshLocked(ctx, false); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

// readFromAnchorLocked starts at the nearest indexed anchor at or before target and
// returns up to limit selected transactions, stopping after through.
func (journal *Journal) readFromAnchorLocked(ctx context.Context, target Cursor, selected func(Cursor) bool, through Cursor, limit int, visit func(Record) error) ([]Record, int64, error) {
	anchor := Location{}
	for _, candidate := range journal.sparse {
		if candidate.Cursor > target {
			break
		}
		anchor = candidate
	}
	for _, candidate := range journal.recent {
		if candidate.Cursor > target {
			break
		}
		if candidate.Cursor > anchor.Cursor {
			anchor = candidate
		}
	}
	startOffset := int64(0)
	previousCursor := Cursor(0)
	previousSHA := ""
	if anchor.Cursor > 0 {
		startOffset = anchor.Offset
		previousCursor = anchor.Cursor - 1
		previousSHA = anchor.PreviousRecordSHA256
	}
	return journal.readSelectedLocked(ctx, startOffset, previousCursor, previousSHA, selected, through, limit, visit)
}

func (journal *Journal) readSelectedLocked(
	ctx context.Context,
	startOffset int64,
	previousCursor Cursor,
	previousSHA string,
	selected func(Cursor) bool,
	through Cursor,
	limit int,
	visit func(Record) error,
) ([]Record, int64, error) {
	file, err := os.Open(journal.path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	if _, err := file.Seek(startOffset, io.SeekStart); err != nil {
		return nil, 0, err
	}
	reader := bufio.NewReaderSize(file, 64*1024)
	readOffset := startOffset
	transactions := 0
	var bytesRead int64
	result := make([]Record, 0)
	for {
		if err := ctx.Err(); err != nil {
			return nil, bytesRead, err
		}
		lineStart := readOffset
		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		readOffset += int64(len(line))
		bytesRead += int64(len(line))
		trimmed := trimRecord(line)
		if len(bytes.TrimSpace(trimmed)) == 0 {
			if previousCursor > 0 {
				continue
			}
			return nil, bytesRead, fmt.Errorf("conversation journal contains an empty range record")
		}
		cursor := previousCursor + 1
		// Committed transactions that are not selected are only counted.
		// Decoding them made every read of a long session parse up to
		// SparseEvery whole transactions. A skipped transaction right before a
		// selected one is hashed, so the selected one still verifies its chain.
		if !selected(cursor) {
			if cursor < through && selected(cursor+1) {
				previousSHA = recordSHA256(trimmed)
			}
			previousCursor = cursor
			if cursor >= through || errors.Is(readErr, io.EOF) {
				break
			}
			continue
		}
		if !json.Valid(trimmed) {
			if errors.Is(readErr, io.EOF) && (len(line) == 0 || line[len(line)-1] != '\n') {
				break
			}
			return nil, bytesRead, fmt.Errorf("conversation journal range contains invalid JSON")
		}
		body, common, err := decodeTransaction(trimmed)
		if err != nil {
			return nil, bytesRead, err
		}
		payloads := []json.RawMessage{trimmed}
		legacy := true
		if common {
			if body.Identity != journal.identity || body.Cursor != cursor || body.PreviousRecordSHA256 != previousSHA {
				return nil, bytesRead, fmt.Errorf("conversation journal range chain is invalid at cursor %d", cursor)
			}
			payloads = body.Records
			legacy = false
		}
		location := Location{Cursor: cursor, Offset: lineStart, Length: len(trimmed), PreviousRecordSHA256: previousSHA}
		for index, payload := range payloads {
			record := Record{Location: location, Payload: payload, Legacy: legacy}
			record.Location.RecordIndex = index
			if visit != nil {
				if err := visit(record); err != nil {
					return nil, bytesRead, err
				}
			} else {
				record.Payload = append(json.RawMessage(nil), payload...)
				result = append(result, record)
			}
		}
		transactions++
		previousCursor = cursor
		previousSHA = recordSHA256(trimmed)
		if transactions >= limit || cursor >= through || errors.Is(readErr, io.EOF) {
			break
		}
	}
	return result, bytesRead, nil
}
