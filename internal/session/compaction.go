package session

import (
	"strings"
	"time"
)

// AppendContextCompaction persists a compaction epoch. It intentionally does
// not append to messages, so user-visible history stays uncompressed.
func (s *Session) AppendContextCompaction(record ContextCompaction) (ContextCompaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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
	s.records = append(s.records, historyRecord{kind: historyTypeCompaction, compaction: &record, createdAt: record.CreatedAt})
	s.UpdatedAt = record.CreatedAt
	return record, s.persistLocked()
}

// RemoveLatestContextCompaction soft-disables the latest active compaction for
// an agent. Raw messages remain untouched so context can reconnect to history.
func (s *Session) RemoveLatestContextCompaction(agentKind, reason string) (ContextCompactionRemoval, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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
	}
	if strings.TrimSpace(record.AgentKind) == "" {
		record.AgentKind = strings.TrimSpace(agentKind)
	}
	s.records = append(s.records, historyRecord{kind: historyTypeCompactionRemoved, compactionRemoval: &record, createdAt: record.CreatedAt})
	s.UpdatedAt = record.CreatedAt
	return record, true, s.persistLocked()
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
