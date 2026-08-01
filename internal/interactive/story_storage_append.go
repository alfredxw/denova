package interactive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/agents/conversationjournal"
	"denova/internal/localfs"
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

// UnmarshalJSON keeps focused storage tests and maintenance tooling able to
// inspect a shared v2 physical transaction through the legacy v1 view. Runtime
// validation still happens in conversationjournal before domain decoding.
func (transaction *storyAppendTransaction) UnmarshalJSON(data []byte) error {
	type alias storyAppendTransaction
	var discriminator struct {
		Journal string `json:"journal"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return err
	}
	if discriminator.Journal != "denova.conversation.append" {
		var decoded alias
		if err := json.Unmarshal(data, &decoded); err != nil {
			return err
		}
		*transaction = storyAppendTransaction(decoded)
		return nil
	}
	var outer struct {
		Records  []json.RawMessage `json:"records"`
		Checksum string            `json:"checksum"`
	}
	if err := json.Unmarshal(data, &outer); err != nil {
		return err
	}
	meta := StoryMeta{}
	events := make([]map[string]any, 0, len(outer.Records))
	for _, payload := range outer.Records {
		decodedMeta, decodedEvents, _, err := decodeStoryProjectionPayload(payload)
		if err != nil {
			return err
		}
		if decodedMeta.StoryID != "" {
			meta = decodedMeta
		}
		for _, event := range decodedEvents {
			events = append(events, event.Raw)
		}
	}
	*transaction = storyAppendTransaction{
		storyAppendTransactionBody: storyAppendTransactionBody{
			Journal: storyAppendTransactionKind, Version: storyAppendTransactionVersion, Meta: meta, Events: events,
		},
		Checksum: outer.Checksum,
	}
	return nil
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

// appendStoryTransactionLocked publishes domain events plus the resulting
// metadata through the shared checksummed conversation journal. The old v1
// encoder remains only behind the injected writer used by compatibility tests.
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
	eventRecords := make([]StoryEventRecord, 0, len(newEvents))
	payloads := make([]json.RawMessage, 0, len(newEvents)+1)
	for _, event := range newEvents {
		record, err := storyEventRecordForWrite(event)
		if err != nil {
			return err
		}
		events = append(events, record.Raw)
		eventRecords = append(eventRecords, record)
		payload, err := json.Marshal(record.Raw)
		if err != nil {
			return err
		}
		payloads = append(payloads, payload)
	}
	metaPayload, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	payloads = append(payloads, metaPayload)
	if s.appendStoryRecord == nil {
		handle, openErr := s.openStoryJournalLocked(storyID)
		if openErr != nil {
			return openErr
		}
		if _, refreshErr := handle.journal.ReadRange(context.Background(), conversationjournal.Range{After: handle.journal.Head().Cursor}); refreshErr != nil {
			return refreshErr
		}
		head := handle.journal.Head()
		commit, appendErr := handle.journal.Append(context.Background(), conversationjournal.Guard{Cursor: head.Cursor, RecordSHA256: head.RecordSHA256}, payloads...)
		if appendErr == nil {
			advanceStoryRecentCaches(handle, commit.Head.Cursor, meta, eventRecords)
			return nil
		}
		committed, reconcileErr := s.reconcileStoryAppendLocked(storyID, meta, events)
		if reconcileErr != nil {
			return errors.Join(appendErr, fmt.Errorf("reconcile ambiguous story append: %w", reconcileErr))
		}
		if committed {
			handle.recent = make(map[string]storyRecentCache)
			slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-story] reconciled shared journal append story_id=%s events=%d original_error=%v", storyID, len(events), appendErr))
			return nil
		}
		return appendErr
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
	appendRecord := s.appendStoryRecord
	appendErr := appendRecord(s.storyPath(storyID), line)
	if appendErr == nil {
		if handle := s.storyJournals[storyID]; handle != nil {
			handle.recent = make(map[string]storyRecentCache)
		}
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
	if handle := s.storyJournals[storyID]; handle != nil {
		handle.recent = make(map[string]storyRecentCache)
	}
	slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-story] reconciled ambiguous append transaction story_id=%s events=%d original_error=%v", storyID, len(events), appendErr))
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
	if discriminator.Journal == "denova.conversation.append" {
		var outer struct {
			Records []json.RawMessage `json:"records"`
		}
		if err := json.Unmarshal(line, &outer); err != nil {
			return StoryMeta{}, nil, true, err
		}
		if len(outer.Records) == 0 {
			return StoryMeta{}, nil, true, fmt.Errorf("shared story transaction has no records")
		}
		var meta StoryMeta
		events := make([]StoryEventRecord, 0, len(outer.Records))
		for index, payload := range outer.Records {
			decodedMeta, decodedEvents, _, err := decodeStoryProjectionPayload(payload)
			if err != nil {
				return StoryMeta{}, nil, true, fmt.Errorf("decode shared story record %d: %w", index+1, err)
			}
			if decodedMeta.StoryID != "" {
				meta = decodedMeta
			}
			events = append(events, decodedEvents...)
		}
		if meta.StoryID == "" {
			return StoryMeta{}, nil, true, fmt.Errorf("shared story transaction has no metadata")
		}
		return meta, events, true, nil
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
	if err := localfs.SyncDirectory(filepath.Dir(path)); err != nil {
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
	return localfs.SyncDirectory(filepath.Dir(path))
}
