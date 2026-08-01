package session

import (
	"context"
	"fmt"
	"os"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/conversationjournal"
)

// refreshCanonicalTailLocked materializes only physical transactions appended
// since this Session last observed the file. The shared journal validates the
// checksum chain and incarnation while holding the cross-process file lease.
func (s *Session) refreshCanonicalTailLocked() error {
	if s.journal == nil {
		return fmt.Errorf("会话 journal 未打开: %s", s.filePath)
	}
	before := s.materializedCursor
	beforeBytes := s.journalOffset
	records, err := s.journal.ReadRange(context.Background(), conversationjournal.Range{After: before})
	if err != nil {
		return err
	}
	if len(records) == 0 {
		head := s.journal.Head()
		s.journalOffset = head.VerifiedBytes
		if stat, statErr := os.Stat(s.filePath); statErr == nil {
			s.journalSize = stat.Size()
		}
		s.lastReplayBytes = 0
		s.lastReplayRecords = 0
		return nil
	}
	candidate := cloneSessionForTailReplay(s)
	candidate.partialMaterialization = s.messageBaseIndex > 0
	lastCursor := before
	for _, record := range records {
		if err := appendConversationRecord(candidate, record); err != nil {
			return fmt.Errorf("解析会话 journal 增量 cursor %d: %w", record.Location.Cursor, err)
		}
		lastCursor = record.Location.Cursor
	}
	candidate.partialMaterialization = false
	candidate.materializedCursor = lastCursor
	head := s.journal.Head()
	candidate.journalOffset = head.VerifiedBytes
	candidate.journalLineCount = int(head.Cursor)
	if stat, statErr := os.Stat(s.filePath); statErr == nil {
		candidate.journalSize = stat.Size()
	} else {
		return statErr
	}
	candidate.lastReplayBytes = max(0, candidate.journalOffset-beforeBytes)
	candidate.lastReplayRecords = int(lastCursor - before)
	if candidate.projection != nil {
		candidate.messageCount = candidate.projection.MessageCount
		candidate.clearAfterIndex = candidate.projection.ClearAfter
		candidate.contextRevision = candidate.projection.ContextRevision
		candidate.title = candidate.projection.Title
		candidate.CreatedAt = candidate.projection.CreatedAt
		candidate.UpdatedAt = candidate.projection.UpdatedAt
	}
	candidate.trimMaterializedWindowLocked()
	s.replaceCanonicalStateLocked(candidate)
	return nil
}

func (s *Session) reloadCanonicalLocked() error {
	recovered, err := loadSession(s.filePath)
	if err != nil {
		return err
	}
	if recovered.journalIncarnation != s.journalIncarnation {
		return fmt.Errorf("session journal incarnation changed during reload: expected=%q actual=%q path=%s", s.journalIncarnation, recovered.journalIncarnation, s.filePath)
	}
	s.replaceCanonicalStateLocked(recovered)
	return nil
}

func cloneSessionForTailReplay(source *Session) *Session {
	result := &Session{
		ID: source.ID, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
		filePath: source.filePath, title: source.title,
		clearAfterIndex: source.clearAfterIndex, contextRevision: source.contextRevision,
		journalSize: source.journalSize, journalOffset: source.journalOffset,
		journalIncarnation: source.journalIncarnation,
		journalNeedsLF:     source.journalNeedsLF, journalLineCount: source.journalLineCount,
		lastReplayBytes: source.lastReplayBytes, lastReplayRecords: source.lastReplayRecords,
		journal: source.journal, projection: source.projection,
		materializedCursor: source.materializedCursor,
		messageBaseIndex:   source.messageBaseIndex, messageCount: source.messageCount,
		historyBaseIndex:       source.historyBaseIndex,
		partialMaterialization: source.partialMaterialization,
		messages:               append([]*agent.Message(nil), source.messages...),
		records:                make([]historyRecord, len(source.records)),
	}
	result.runtimeConfigRevision = source.runtimeConfigRevision
	if source.runtimeConfig != nil {
		value := *source.runtimeConfig
		result.runtimeConfig = &value
	}
	for index, record := range source.records {
		result.records[index] = cloneHistoryRecordForTailReplay(record)
	}
	return result
}

func cloneHistoryRecordForTailReplay(record historyRecord) historyRecord {
	clone := record
	clone.messageMetadata.RunPath = append([]string(nil), record.messageMetadata.RunPath...)
	clone.messageMetadata.UserReferences = append([]UserMessageReference(nil), record.messageMetadata.UserReferences...)
	if record.display != nil {
		display := *record.display
		display.Illustration = cloneChapterIllustration(record.display.Illustration)
		display.RunPath = append([]string(nil), record.display.RunPath...)
		display.UsageCalls = append([]TokenUsageCall(nil), record.display.UsageCalls...)
		for index := range display.UsageCalls {
			display.UsageCalls[index].RequestedTools = append([]string(nil), record.display.UsageCalls[index].RequestedTools...)
			display.UsageCalls[index].AfterTools = append([]string(nil), record.display.UsageCalls[index].AfterTools...)
		}
		display.SSEHiddenFields = append([]string(nil), record.display.SSEHiddenFields...)
		clone.display = &display
	}
	if record.interruption != nil {
		interruption := *record.interruption
		if record.interruption.ResolvedAt != nil {
			resolvedAt := *record.interruption.ResolvedAt
			interruption.ResolvedAt = &resolvedAt
		}
		clone.interruption = &interruption
	}
	if record.ask != nil {
		interaction := cloneAskInteraction(*record.ask)
		clone.ask = &interaction
	}
	if record.compaction != nil {
		compaction := *record.compaction
		clone.compaction = &compaction
	}
	if record.compactionRemoval != nil {
		removal := *record.compactionRemoval
		clone.compactionRemoval = &removal
	}
	if record.compactionHealth != nil {
		health := *record.compactionHealth
		clone.compactionHealth = &health
	}
	if record.toolResultCleanup != nil {
		cleanup := cloneToolResultCleanupRecord(*record.toolResultCleanup)
		clone.toolResultCleanup = &cleanup
	}
	return clone
}

// trimMaterializedWindowLocked bounds resident transcript and display data.
// It only drops a physical prefix already represented by the rebuildable
// projection; raw JSONL remains available to paged readers and exporters.
func (s *Session) trimMaterializedWindowLocked() {
	if len(s.messages) <= sessionRecentTransactionLimit || s.projection == nil {
		return
	}
	drop := len(s.messages) - sessionRecentTransactionLimit
	s.messages = append([]*agent.Message(nil), s.messages[drop:]...)
	s.messageBaseIndex += drop
	// Keep record mutations addressable for active streams while avoiding a
	// second unbounded resident copy. This is deliberately a larger mixed-event
	// window than the 200-row UI page cache.
	if len(s.records) > sessionRecentTransactionLimit*2 {
		overflow := len(s.records) - sessionRecentTransactionLimit*2
		s.records = append([]historyRecord(nil), s.records[overflow:]...)
	}
}
