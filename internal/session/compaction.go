package session

import (
	"context"
	"strings"
	"time"
)

// AppendContextCompaction persists a compaction epoch. It intentionally does
// not append to messages, so user-visible history stays uncompressed.
func (s *Session) AppendContextCompaction(record ContextCompaction) (ContextCompaction, error) {
	result := record
	err := s.withCanonicalMutation(context.Background(), "append context compaction", func() error {
		var appendErr error
		result, appendErr = s.appendContextCompactionLocked(record)
		return appendErr
	})
	return result, err
}

func (s *Session) appendContextCompactionLocked(record ContextCompaction) (ContextCompaction, error) {
	now := time.Now().UTC()
	record.Type = historyTypeCompaction
	if strings.TrimSpace(record.ID) == "" {
		record.ID = newContextCompactionID()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.Epoch <= 0 {
		record.Epoch = s.nextCompactionEpochLocked(record.AgentKind)
	}
	if record.SourceEndIndex <= 0 || record.SourceEndIndex > len(s.messages) {
		record.SourceEndIndex = len(s.messages)
	}
	if record.SourceStartIndex < s.clearAfterIndex {
		record.SourceStartIndex = s.clearAfterIndex
	}
	if record.SourceStartIndex > record.SourceEndIndex {
		record.SourceStartIndex = record.SourceEndIndex
	}
	if record.SourceMessageCount <= 0 {
		record.SourceMessageCount = record.SourceEndIndex - record.SourceStartIndex
	}
	record.ContextRevision = s.contextRevision + 1
	if err := s.appendJournalRecordLocked(record); err != nil {
		return record, err
	}
	s.contextRevision = record.ContextRevision
	s.records = append(s.records, historyRecord{kind: historyTypeCompaction, compaction: &record, createdAt: record.CreatedAt})
	advanceUpdatedAt(s, record.CreatedAt)
	return record, nil
}

// RemoveLatestContextCompaction soft-disables the latest active compaction for
// an agent. Raw messages remain untouched so context can reconnect to history.
func (s *Session) RemoveLatestContextCompaction(agentKind, reason string) (ContextCompactionRemoval, bool, error) {
	var result ContextCompactionRemoval
	var removed bool
	err := s.withCanonicalMutation(context.Background(), "remove latest context compaction", func() error {
		var removeErr error
		result, removed, removeErr = s.removeLatestContextCompactionLocked(agentKind, reason)
		return removeErr
	})
	return result, removed, err
}

func (s *Session) removeLatestContextCompactionLocked(agentKind, reason string) (ContextCompactionRemoval, bool, error) {
	compaction, ok := s.latestActiveContextCompactionLocked(agentKind)
	if !ok {
		return ContextCompactionRemoval{}, false, nil
	}
	now := time.Now().UTC()
	record := ContextCompactionRemoval{
		Type:             historyTypeCompactionRemoved,
		ID:               newContextCompactionRemovalID(),
		AgentKind:        compaction.AgentKind,
		CompactionID:     compaction.ID,
		SourceStartIndex: compaction.SourceStartIndex,
		SourceEndIndex:   compaction.SourceEndIndex,
		Reason:           strings.TrimSpace(reason),
		CreatedAt:        now,
		ContextRevision:  s.contextRevision + 1,
	}
	if strings.TrimSpace(record.AgentKind) == "" {
		record.AgentKind = strings.TrimSpace(agentKind)
	}
	if err := s.appendJournalRecordLocked(record); err != nil {
		return record, false, err
	}
	s.contextRevision = record.ContextRevision
	s.records = append(s.records, historyRecord{kind: historyTypeCompactionRemoved, compactionRemoval: &record, createdAt: record.CreatedAt})
	advanceUpdatedAt(s, record.CreatedAt)
	return record, true, nil
}

// LatestContextCompaction returns the newest compaction epoch after the latest
// clear marker for the given agent kind.
func (s *Session) LatestContextCompaction(agentKind string) (ContextCompaction, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.latestActiveContextCompactionLocked(agentKind)
}

// LatestContextCompactionRemoval returns the newest removal marker after the
// latest clear marker for the given agent kind.
func (s *Session) LatestContextCompactionRemoval(agentKind string) (ContextCompactionRemoval, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := len(s.records) - 1; i >= 0; i-- {
		record := s.records[i]
		if record.kind != historyTypeCompactionRemoved || record.compactionRemoval == nil {
			continue
		}
		removal := *record.compactionRemoval
		if removal.SourceEndIndex <= s.clearAfterIndex {
			continue
		}
		if strings.TrimSpace(agentKind) != "" && strings.TrimSpace(removal.AgentKind) != "" && removal.AgentKind != agentKind {
			continue
		}
		return removal, true
	}
	return ContextCompactionRemoval{}, false
}

func (s *Session) NextContextCompactionEpoch(agentKind string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextCompactionEpochLocked(agentKind)
}

func (s *Session) latestActiveContextCompactionLocked(agentKind string) (ContextCompaction, bool) {
	for i := len(s.records) - 1; i >= 0; i-- {
		record := s.records[i]
		if record.kind == historyTypeCompactionRemoved && record.compactionRemoval != nil {
			removal := *record.compactionRemoval
			if removal.SourceEndIndex <= s.clearAfterIndex {
				continue
			}
			if strings.TrimSpace(agentKind) == "" || strings.TrimSpace(removal.AgentKind) == "" || removal.AgentKind == agentKind {
				return ContextCompaction{}, false
			}
			continue
		}
		if record.kind != historyTypeCompaction || record.compaction == nil {
			continue
		}
		compaction := *record.compaction
		if compaction.SourceEndIndex <= s.clearAfterIndex {
			continue
		}
		if strings.TrimSpace(agentKind) != "" && strings.TrimSpace(compaction.AgentKind) != "" && compaction.AgentKind != agentKind {
			continue
		}
		return compaction, true
	}
	return ContextCompaction{}, false
}

func (s *Session) nextCompactionEpochLocked(agentKind string) int {
	epoch := 0
	for _, record := range s.records {
		if record.kind != historyTypeCompaction || record.compaction == nil {
			continue
		}
		if strings.TrimSpace(agentKind) != "" && strings.TrimSpace(record.compaction.AgentKind) != "" && record.compaction.AgentKind != agentKind {
			continue
		}
		if record.compaction.Epoch > epoch {
			epoch = record.compaction.Epoch
		}
	}
	return epoch + 1
}
