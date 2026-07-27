package conversationjournal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"denova/internal/filelease"
)

// Journal is a concurrency-safe handle to one canonical conversation file.
// Independent handles rendezvous through the same cross-process file lease.
type Journal struct {
	path      string
	indexPath string
	identity  Identity
	reducer   Reducer
	options   Options

	mu                  sync.Mutex
	head                Head
	journalSize         int64
	validOffset         int64
	needsNewline        bool
	tornTail            []byte
	initialRecordSHA256 string
	sparse              []Location
	recent              []Location
	dirtyTransactions   int
	indexDirty          bool
	stats               ReplayStats
	closed              bool
	projectionInvalid   bool
}

// Open restores a reducer from the sidecar and replays only the unindexed
// canonical tail. A missing or invalid sidecar triggers one streaming rebuild.
func Open(ctx context.Context, path string, identity Identity, reducer Reducer, options Options) (*Journal, error) {
	if err := validateIdentity(identity); err != nil {
		return nil, err
	}
	if reducer == nil {
		return nil, fmt.Errorf("conversation journal reducer is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := filelease.Acquire(ctx, path+".domain.lock")
	if err != nil {
		return nil, fmt.Errorf("acquire conversation journal open lease: %w", err)
	}
	defer release()

	journal := &Journal{
		path: path, indexPath: sidecarPath(path), identity: identity,
		reducer: reducer, options: options.normalized(),
	}
	journal.mu.Lock()
	err = journal.loadLocked(ctx)
	journal.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return journal, nil
}

func (journal *Journal) Path() string {
	if journal == nil {
		return ""
	}
	return journal.path
}

func (journal *Journal) Head() Head {
	if journal == nil {
		return Head{}
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return journal.head
}

func (journal *Journal) ReplayStats() ReplayStats {
	if journal == nil {
		return ReplayStats{}
	}
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return journal.stats
}

func (journal *Journal) loadLocked(ctx context.Context) error {
	info, err := os.Stat(journal.path)
	if err != nil {
		return fmt.Errorf("stat conversation journal: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("conversation journal is not a regular file: %s", journal.path)
	}
	journal.journalSize = info.Size()

	loaded, indexErr := journal.restoreIndexLocked()
	if loaded {
		journal.stats.IndexLoaded = true
		beforeBytes := journal.stats.BytesRead
		beforeRecords := journal.stats.TransactionsRead
		if err := journal.scanLocked(ctx, journal.validOffset); err == nil {
			journal.stats.TailBytesRead = journal.stats.BytesRead - beforeBytes
			journal.stats.TailRecordsRead = journal.stats.TransactionsRead - beforeRecords
			return nil
		}
		// A valid physical checkpoint with an incompatible domain projection is
		// never partially trusted. Rebuild from the canonical source.
		_ = journal.reducer.Reset()
	}
	if indexErr != nil && !errors.Is(indexErr, os.ErrNotExist) {
		log.Printf("[conversation-journal] rebuild invalid index path=%s error=%v", journal.indexPath, indexErr)
	}
	if err := journal.resetForReplayLocked(); err != nil {
		return err
	}
	if err := journal.scanLocked(ctx, 0); err != nil {
		return err
	}
	journal.stats.IndexRebuilt = true
	if err := journal.persistIndexLocked(); err != nil {
		log.Printf("[conversation-journal] initial index write deferred path=%s error=%v", journal.indexPath, err)
	}
	return nil
}

func (journal *Journal) resetForReplayLocked() error {
	if err := journal.reducer.Reset(); err != nil {
		return fmt.Errorf("reset conversation projection: %w", err)
	}
	journal.head = Head{Identity: journal.identity}
	journal.validOffset = 0
	journal.needsNewline = false
	journal.tornTail = nil
	journal.initialRecordSHA256 = ""
	journal.sparse = nil
	journal.recent = nil
	journal.stats.BytesRead = 0
	journal.stats.TransactionsRead = 0
	journal.stats.DomainRecordsRead = 0
	return nil
}

func (journal *Journal) scanLocked(ctx context.Context, startOffset int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.Open(journal.path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() < startOffset {
		return fmt.Errorf("conversation journal shrank: indexed=%d actual=%d", startOffset, info.Size())
	}
	if _, err := file.Seek(startOffset, io.SeekStart); err != nil {
		return err
	}
	reader := bufio.NewReaderSize(file, 64*1024)
	readOffset := startOffset
	firstSegment := true
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		lineStart := readOffset
		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		readOffset += int64(len(line))
		journal.stats.BytesRead += int64(len(line))
		newlineTerminated := len(line) > 0 && line[len(line)-1] == '\n'
		trimmed := trimRecord(line)

		// A valid unterminated prior record is separated by a newline before
		// the next append. It is not an empty domain transaction.
		if firstSegment && startOffset > 0 && journal.needsNewline && newlineTerminated && len(trimmed) == 0 {
			journal.validOffset = readOffset
			journal.needsNewline = false
			firstSegment = false
			continue
		}
		firstSegment = false
		if len(bytes.TrimSpace(trimmed)) == 0 {
			return fmt.Errorf("conversation journal contains an empty record at line %d offset %d", journal.head.Cursor+1, lineStart)
		}
		if !json.Valid(trimmed) {
			if errors.Is(readErr, io.EOF) && !newlineTerminated {
				journal.tornTail = append([]byte(nil), line...)
				journal.journalSize = info.Size()
				break
			}
			return fmt.Errorf("conversation journal contains invalid JSON at line %d offset %d", journal.head.Cursor+1, lineStart)
		}
		if err := journal.applyLineLocked(trimmed, lineStart, len(trimmed), readOffset, newlineTerminated); err != nil {
			return err
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	finalInfo, err := file.Stat()
	if err != nil {
		return err
	}
	if finalInfo.Size() != info.Size() || readOffset != info.Size() {
		return fmt.Errorf("conversation journal changed during replay: %s", journal.path)
	}
	journal.journalSize = info.Size()
	journal.head.VerifiedBytes = journal.validOffset
	return nil
}

func (journal *Journal) applyLineLocked(line []byte, offset int64, length int, nextOffset int64, newlineTerminated bool) error {
	body, common, err := decodeTransaction(line)
	if err != nil {
		return fmt.Errorf("decode conversation journal cursor %d: %w", journal.head.Cursor+1, err)
	}
	previousSHA := journal.head.RecordSHA256
	nextCursor := journal.head.Cursor + 1
	payloads := []json.RawMessage{append(json.RawMessage(nil), line...)}
	legacy := true
	if common {
		if body.Identity != journal.identity {
			return fmt.Errorf("conversation journal identity changed: have=%+v want=%+v", body.Identity, journal.identity)
		}
		if body.Cursor != nextCursor {
			return fmt.Errorf("conversation journal cursor gap: have=%d want=%d", body.Cursor, nextCursor)
		}
		if body.PreviousRecordSHA256 != previousSHA {
			return fmt.Errorf("conversation journal checksum chain mismatch at cursor %d", body.Cursor)
		}
		payloads = body.Records
		legacy = false
	}
	location := Location{
		Cursor: nextCursor, Offset: offset, Length: length,
		PreviousRecordSHA256: previousSHA,
	}
	for index, payload := range payloads {
		record := Record{
			Location: location, Payload: append(json.RawMessage(nil), payload...), Legacy: legacy,
		}
		record.Location.RecordIndex = index
		if err := journal.reducer.Apply(record); err != nil {
			return fmt.Errorf("reduce conversation journal line %d cursor %d record %d: %w", nextCursor, nextCursor, index, err)
		}
		journal.stats.DomainRecordsRead++
	}
	lineSHA := recordSHA256(line)
	if nextCursor == 1 {
		journal.initialRecordSHA256 = lineSHA
	}
	journal.recordLocationLocked(location)
	journal.head.Cursor = nextCursor
	journal.head.RecordSHA256 = lineSHA
	journal.head.TransactionCount++
	journal.validOffset = nextOffset
	journal.head.VerifiedBytes = nextOffset
	journal.needsNewline = !newlineTerminated
	journal.tornTail = nil
	journal.stats.TransactionsRead++
	journal.indexDirty = true
	return nil
}

func (journal *Journal) recordLocationLocked(location Location) {
	if location.Cursor == 1 || (uint64(location.Cursor)-1)%uint64(journal.options.SparseEvery) == 0 {
		journal.sparse = append(journal.sparse, location)
	}
	journal.recent = append(journal.recent, location)
	if overflow := len(journal.recent) - journal.options.RecentRecords; overflow > 0 {
		journal.recent = append([]Location(nil), journal.recent[overflow:]...)
	}
}
