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

	"denova/internal/filelease"
)

// ReadRange reads a stable physical range without touching the domain
// projection. Domain adapters use their own logical locators to choose cursors.
func (journal *Journal) ReadRange(ctx context.Context, selected Range) ([]Record, error) {
	if journal == nil {
		return nil, fmt.Errorf("conversation journal is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := filelease.Acquire(ctx, journal.path+".domain.lock")
	if err != nil {
		return nil, err
	}
	defer release()
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.closed {
		return nil, fmt.Errorf("conversation journal is closed")
	}
	if err := journal.refreshLocked(ctx, false); err != nil {
		return nil, err
	}
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
	anchor := Location{}
	target := selected.After + 1
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
	result, bytesRead, err := journal.readRangeFromLocked(ctx, startOffset, previousCursor, previousSHA, selected.After, through, limit)
	journal.stats.LastRangeBytesRead = bytesRead
	return result, err
}

func (journal *Journal) readRangeFromLocked(
	ctx context.Context,
	startOffset int64,
	previousCursor Cursor,
	previousSHA string,
	after Cursor,
	through Cursor,
	limit int,
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
		cursor := previousCursor + 1
		payloads := []json.RawMessage{append(json.RawMessage(nil), trimmed...)}
		legacy := true
		if common {
			if body.Identity != journal.identity || body.Cursor != cursor || body.PreviousRecordSHA256 != previousSHA {
				return nil, bytesRead, fmt.Errorf("conversation journal range chain is invalid at cursor %d", cursor)
			}
			payloads = body.Records
			legacy = false
		}
		lineSHA := recordSHA256(trimmed)
		if cursor > after && cursor <= through {
			location := Location{Cursor: cursor, Offset: lineStart, Length: len(trimmed), PreviousRecordSHA256: previousSHA}
			for index, payload := range payloads {
				record := Record{Location: location, Payload: append(json.RawMessage(nil), payload...), Legacy: legacy}
				record.Location.RecordIndex = index
				result = append(result, record)
			}
			transactions++
			if transactions >= limit {
				break
			}
		}
		previousCursor = cursor
		previousSHA = lineSHA
		if cursor >= through || errors.Is(readErr, io.EOF) {
			break
		}
	}
	return result, bytesRead, nil
}
