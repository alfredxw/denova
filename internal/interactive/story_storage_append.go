package interactive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/fsdurability"
)

const (
	storyAppendTransactionKind    = "denova.story.append"
	storyAppendTransactionVersion = 1
)

// StoryJournalReplayStats exposes physical replay cost separately from the
// logical story projection. It is diagnostic state only and never enters model
// context or canonical story data.
type StoryJournalReplayStats struct {
	BytesRead        int64
	RecordsRead      int64
	TransactionsRead int64
	EventsRead       int64
}

// LastStoryJournalReplayStats returns the latest complete read statistics for
// one story in this Store process.
func (s *Store) LastStoryJournalReplayStats(storyID string) StoryJournalReplayStats {
	if s == nil {
		return StoryJournalReplayStats{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastStoryReplayByStory[strings.TrimSpace(storyID)]
}

type storyAppendTransactionBody struct {
	Journal string           `json:"journal"`
	Version int              `json:"version"`
	Meta    StoryMeta        `json:"meta"`
	Events  []map[string]any `json:"events"`
}

type storyAppendTransaction struct {
	storyAppendTransactionBody
	Checksum string `json:"checksum"`
}

type storyAppendRecordError struct {
	writeErr     error
	syncErr      error
	closeErr     error
	directoryErr error
}

func (e *storyAppendRecordError) Error() string {
	return errors.Join(e.writeErr, e.syncErr, e.closeErr, e.directoryErr).Error()
}

func (e *storyAppendRecordError) Unwrap() error {
	return errors.Join(e.writeErr, e.syncErr, e.closeErr, e.directoryErr)
}

// Page-cache visibility cannot prove durability after fsync fails. Write and
// close result loss can be reconciled only when Sync itself succeeded; a
// directory-sync failure is likewise safe to inspect but is still reported if
// the exact transaction is absent.
func (e *storyAppendRecordError) canReconcileByReadback() bool {
	return e != nil && e.syncErr == nil
}

// appendStoryTransactionLocked publishes one metadata revision and all new
// events through one checksummed JSON record. Existing history is not copied;
// low-frequency edits continue to use rewriteStoryLocked as an explicit
// compaction/rewrite boundary.
func (s *Store) appendStoryTransactionLocked(storyID string, meta StoryMeta, newEvents ...any) error {
	storyID = strings.TrimSpace(storyID)
	if s.heldStoryLeases == nil || s.heldStoryLeases[storyID] <= 0 {
		return fmt.Errorf("story append transaction requires the cross-store mutation lease")
	}
	if len(newEvents) == 0 {
		return fmt.Errorf("story append transaction requires at least one event")
	}
	meta = normalizeStoryMeta(meta)
	if err := validateStoryMeta(meta); err != nil {
		return err
	}
	if meta.StoryID != storyID {
		return fmt.Errorf("story append metadata identity mismatch: have %q want %q", meta.StoryID, storyID)
	}
	events := make([]map[string]any, 0, len(newEvents))
	for _, event := range newEvents {
		record, err := storyEventRecordForWrite(event)
		if err != nil {
			return err
		}
		events = append(events, record.Raw)
	}
	body := storyAppendTransactionBody{
		Journal: storyAppendTransactionKind, Version: storyAppendTransactionVersion,
		Meta: meta, Events: events,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode story append checksum body: %w", err)
	}
	digest := sha256.Sum256(bodyJSON)
	line, err := json.Marshal(storyAppendTransaction{
		storyAppendTransactionBody: body,
		Checksum:                   hex.EncodeToString(digest[:]),
	})
	if err != nil {
		return fmt.Errorf("encode story append transaction: %w", err)
	}
	appendRecord := appendStoryRecord
	if s.appendStoryRecord != nil {
		appendRecord = s.appendStoryRecord
	}
	appendErr := appendRecord(s.storyPath(storyID), line)
	if appendErr == nil {
		return nil
	}
	var durabilityErr *storyAppendRecordError
	if !errors.As(appendErr, &durabilityErr) || !durabilityErr.canReconcileByReadback() {
		return appendErr
	}
	committed, reconcileErr := s.reconcileStoryAppendLocked(storyID, meta, events)
	if reconcileErr != nil {
		return errors.Join(appendErr, fmt.Errorf("reconcile ambiguous story append: %w", reconcileErr))
	}
	if !committed {
		return appendErr
	}
	log.Printf("[interactive-story] reconciled ambiguous append transaction story_id=%s events=%d original_error=%v", storyID, len(events), appendErr)
	return nil
}

func (s *Store) reconcileStoryAppendLocked(storyID string, expectedMeta StoryMeta, expectedEvents []map[string]any) (bool, error) {
	meta, events, err := s.readStoryJournalLocked(storyID)
	if err != nil {
		return false, err
	}
	if !sameCanonicalJSON(meta, expectedMeta) {
		return false, nil
	}
	byID := make(map[string]StoryEventRecord, len(events))
	for _, event := range events {
		byID[event.Envelope.ID] = event
	}
	for _, expected := range expectedEvents {
		record, err := mapToStoryEventRecord(expected)
		if err != nil {
			return false, err
		}
		actual, ok := byID[record.Envelope.ID]
		if !ok || actual.Envelope.Type != record.Envelope.Type || !sameCanonicalJSON(actual.Raw, expected) {
			return false, nil
		}
	}
	return true, nil
}

func sameCanonicalJSON(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func decodeStoryAppendTransaction(line []byte) (StoryMeta, []StoryEventRecord, bool, error) {
	var discriminator struct {
		Journal string `json:"journal"`
	}
	if err := json.Unmarshal(line, &discriminator); err != nil {
		return StoryMeta{}, nil, false, err
	}
	if discriminator.Journal != storyAppendTransactionKind {
		return StoryMeta{}, nil, false, nil
	}
	var transaction storyAppendTransaction
	if err := json.Unmarshal(line, &transaction); err != nil {
		return StoryMeta{}, nil, true, err
	}
	body := transaction.storyAppendTransactionBody
	if body.Version != storyAppendTransactionVersion {
		return StoryMeta{}, nil, true, fmt.Errorf("unsupported story append transaction version %d", body.Version)
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return StoryMeta{}, nil, true, err
	}
	digest := sha256.Sum256(bodyJSON)
	if transaction.Checksum != hex.EncodeToString(digest[:]) {
		return StoryMeta{}, nil, true, fmt.Errorf("story append transaction checksum mismatch")
	}
	meta := normalizeStoryMeta(body.Meta)
	if err := validateStoryMeta(meta); err != nil {
		return StoryMeta{}, nil, true, fmt.Errorf("validate story append metadata: %w", err)
	}
	events := make([]StoryEventRecord, 0, len(body.Events))
	for index, raw := range body.Events {
		record, err := mapToStoryEventRecord(raw)
		if err != nil {
			return StoryMeta{}, nil, true, fmt.Errorf("validate story append event %d: %w", index+1, err)
		}
		events = append(events, record)
	}
	return meta, events, true, nil
}

func appendStoryRecord(path string, line []byte) error {
	if len(line) == 0 {
		return fmt.Errorf("story append record is empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat story before append: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("story metadata is missing before append")
	}
	last := make([]byte, 1)
	existing, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open story before append: %w", err)
	}
	_, readErr := existing.ReadAt(last, info.Size()-1)
	closeErr := existing.Close()
	if readErr != nil || closeErr != nil {
		return errors.Join(readErr, closeErr)
	}
	record := make([]byte, 0, len(line)+2)
	if last[0] != '\n' {
		record = append(record, '\n')
	}
	record = append(record, line...)
	record = append(record, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open story for append: %w", err)
	}
	writeErr := writeAllStoryBytes(file, record)
	syncErr := file.Sync()
	closeErr = file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return &storyAppendRecordError{writeErr: writeErr, syncErr: syncErr, closeErr: closeErr}
	}
	if err := fsdurability.SyncDirectory(filepath.Dir(path)); err != nil {
		return &storyAppendRecordError{directoryErr: err}
	}
	return nil
}

func writeAllStoryBytes(file *os.File, data []byte) error {
	for len(data) > 0 {
		written, err := file.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

func repairTornStoryTail(path string, validBytes int64) error {
	backupPath := fmt.Sprintf("%s.recovery.%d.%d.bak", path, time.Now().UTC().UnixNano(), os.Getpid())
	source, err := os.Open(path)
	if err != nil {
		return err
	}
	backup, err := os.OpenFile(backupPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		_ = source.Close()
		return err
	}
	_, copyErr := io.Copy(backup, source)
	sourceCloseErr := source.Close()
	if copyErr == nil {
		copyErr = backup.Sync()
	}
	backupCloseErr := backup.Close()
	if copyErr != nil || sourceCloseErr != nil || backupCloseErr != nil {
		return errors.Join(copyErr, sourceCloseErr, backupCloseErr)
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open story torn tail for repair (backup %s): %w", backupPath, err)
	}
	repairErr := file.Truncate(validBytes)
	if repairErr == nil {
		repairErr = file.Sync()
	}
	closeErr := file.Close()
	if repairErr != nil || closeErr != nil {
		return errors.Join(repairErr, closeErr)
	}
	return fsdurability.SyncDirectory(filepath.Dir(path))
}
