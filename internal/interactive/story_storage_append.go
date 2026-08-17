package interactive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"denova/internal/agents/conversationjournal"
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

// appendStoryTransactionLocked publishes domain events plus the resulting
// metadata through the shared checksummed conversation journal.
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
	handle, err := s.openStoryJournalLocked(storyID)
	if err != nil {
		return err
	}
	if _, err := handle.journal.ReadRange(context.Background(), conversationjournal.Range{After: handle.journal.Head().Cursor}); err != nil {
		return err
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
	if !committed {
		return appendErr
	}
	handle.recent = make(map[string]storyRecentCache)
	slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-story] reconciled shared journal append story_id=%s events=%d original_error=%v", storyID, len(events), appendErr))
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
