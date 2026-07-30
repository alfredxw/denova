package session

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// AppendToolResultCleanup persists a frozen cleanup projection at the latest
// context revision. Raw messages and display history are intentionally left
// untouched.
func (s *Session) AppendToolResultCleanup(record ToolResultCleanupRecord) (ToolResultCleanupRecord, error) {
	result := record
	err := s.withCanonicalMutation(context.Background(), "append tool result cleanup", func() error {
		var appendErr error
		result, appendErr = s.appendToolResultCleanupLocked(record)
		return appendErr
	})
	return result, err
}

// AppendToolResultCleanupAt publishes a cleanup projection against the exact
// model-visible revision used by its planner.
func (s *Session) AppendToolResultCleanupAt(
	expected ContextCursor,
	record ToolResultCleanupRecord,
) (ToolResultCleanupRecord, error) {
	return s.AppendToolResultCleanupAtContext(context.Background(), expected, record)
}

// AppendToolResultCleanupAtContext uses the canonical file lease and refreshes
// the tail before CAS. An exact retry returns the already committed record even
// when the caller's revision is now stale.
func (s *Session) AppendToolResultCleanupAtContext(
	ctx context.Context,
	expected ContextCursor,
	record ToolResultCleanupRecord,
) (ToolResultCleanupRecord, error) {
	result := record
	err := s.withCanonicalMutation(ctx, "append tool result cleanup with revision", func() error {
		normalized, err := normalizeToolResultCleanupRecord(record)
		if err != nil {
			return err
		}
		if existing, ok := s.toolResultCleanupByIDLocked(normalized.ID); ok {
			if !sameToolResultCleanupIntent(existing, normalized) {
				return fmt.Errorf("%w: cleanup id %q has different content", ErrDomainCommitIdentityConflict, normalized.ID)
			}
			result = existing
			return nil
		}
		if current := s.contextCursorLocked(); current.Revision != expected.Revision {
			return fmt.Errorf("%w: expected=%d current=%d", ErrContextRevisionConflict, expected.Revision, current.Revision)
		}
		result, err = s.appendNormalizedToolResultCleanupLocked(normalized)
		return err
	})
	return result, err
}

func (s *Session) appendToolResultCleanupLocked(record ToolResultCleanupRecord) (ToolResultCleanupRecord, error) {
	normalized, err := normalizeToolResultCleanupRecord(record)
	if err != nil {
		return ToolResultCleanupRecord{}, err
	}
	if existing, ok := s.toolResultCleanupByIDLocked(normalized.ID); ok {
		if !sameToolResultCleanupIntent(existing, normalized) {
			return ToolResultCleanupRecord{}, fmt.Errorf("%w: cleanup id %q has different content", ErrDomainCommitIdentityConflict, normalized.ID)
		}
		return existing, nil
	}
	return s.appendNormalizedToolResultCleanupLocked(normalized)
}

func (s *Session) appendNormalizedToolResultCleanupLocked(record ToolResultCleanupRecord) (ToolResultCleanupRecord, error) {
	if record.SourceStart < int64(s.clearAfterIndex) || record.SourceEnd > int64(s.messageCount) {
		return ToolResultCleanupRecord{}, fmt.Errorf(
			"tool result cleanup source range [%d,%d) is outside effective Session range [%d,%d)",
			record.SourceStart, record.SourceEnd, s.clearAfterIndex, s.messageCount,
		)
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	record.ContextRevision = s.contextRevision + 1
	if err := s.appendJournalRecordLocked(record); err != nil {
		return ToolResultCleanupRecord{}, err
	}
	record = cloneToolResultCleanupRecord(record)
	s.contextRevision = record.ContextRevision
	s.records = append(s.records, historyRecord{kind: historyTypeToolResultCleanup, toolResultCleanup: &record, createdAt: record.CreatedAt})
	advanceUpdatedAt(s, record.CreatedAt)
	return cloneToolResultCleanupRecord(record), nil
}

// LatestToolResultCleanup returns the latest effective cleanup after the most
// recent compaction/removal boundary for this Agent.
func (s *Session) LatestToolResultCleanup(agentKind string) (ToolResultCleanupRecord, bool) {
	if s == nil {
		return ToolResultCleanupRecord{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latestActiveToolResultCleanupLocked(agentKind)
}

// ToolResultCleanupByID returns one recent canonical cleanup record. The
// rebuildable projection keeps the same bounded structural lookup window used
// by compaction idempotency.
func (s *Session) ToolResultCleanupByID(id string) (ToolResultCleanupRecord, bool) {
	if s == nil {
		return ToolResultCleanupRecord{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.toolResultCleanupByIDLocked(id)
}

func (s *Session) latestActiveToolResultCleanupLocked(agentKind string) (ToolResultCleanupRecord, bool) {
	if s.projection != nil {
		for index := len(s.projection.Structural) - 1; index >= 0; index-- {
			record := s.projection.Structural[index]
			if record.Cleanup != nil {
				cleanup := *record.Cleanup
				if contextRecordMatchesAgent(cleanup.AgentKind, agentKind) {
					return cloneToolResultCleanupRecord(cleanup), true
				}
				continue
			}
			if record.Compaction != nil && contextRecordMatchesAgent(record.Compaction.AgentKind, agentKind) {
				return ToolResultCleanupRecord{}, false
			}
			if record.Removal != nil && contextRecordMatchesAgent(record.Removal.AgentKind, agentKind) {
				return ToolResultCleanupRecord{}, false
			}
		}
		return ToolResultCleanupRecord{}, false
	}
	for index := len(s.records) - 1; index >= 0; index-- {
		record := s.records[index]
		if record.kind == historyTypeToolResultCleanup && record.toolResultCleanup != nil {
			cleanup := *record.toolResultCleanup
			if cleanup.SourceEnd <= int64(s.clearAfterIndex) || !contextRecordMatchesAgent(cleanup.AgentKind, agentKind) {
				continue
			}
			return cloneToolResultCleanupRecord(cleanup), true
		}
		if record.kind == historyTypeCompaction && record.compaction != nil && contextRecordMatchesAgent(record.compaction.AgentKind, agentKind) {
			return ToolResultCleanupRecord{}, false
		}
		if record.kind == historyTypeCompactionRemoved && record.compactionRemoval != nil && contextRecordMatchesAgent(record.compactionRemoval.AgentKind, agentKind) {
			return ToolResultCleanupRecord{}, false
		}
	}
	return ToolResultCleanupRecord{}, false
}

func (s *Session) toolResultCleanupByIDLocked(id string) (ToolResultCleanupRecord, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ToolResultCleanupRecord{}, false
	}
	if s.projection != nil {
		for _, record := range s.projection.Structural {
			if record.Cleanup != nil && record.Cleanup.ID == id {
				return cloneToolResultCleanupRecord(*record.Cleanup), true
			}
		}
		return ToolResultCleanupRecord{}, false
	}
	for _, record := range s.records {
		if record.kind == historyTypeToolResultCleanup && record.toolResultCleanup != nil && record.toolResultCleanup.ID == id {
			return cloneToolResultCleanupRecord(*record.toolResultCleanup), true
		}
	}
	return ToolResultCleanupRecord{}, false
}

func normalizeToolResultCleanupRecord(record ToolResultCleanupRecord) (ToolResultCleanupRecord, error) {
	record.Type = historyTypeToolResultCleanup
	record.ID = strings.TrimSpace(record.ID)
	if record.ID == "" {
		record.ID = newToolResultCleanupID()
	}
	record.AgentKind = strings.TrimSpace(record.AgentKind)
	record.RendererVersion = strings.TrimSpace(record.RendererVersion)
	if record.SourceStart < 0 || record.SourceEnd <= record.SourceStart {
		return ToolResultCleanupRecord{}, fmt.Errorf("tool result cleanup source range [%d,%d) is invalid", record.SourceStart, record.SourceEnd)
	}
	if record.RendererVersion == "" {
		return ToolResultCleanupRecord{}, fmt.Errorf("tool result cleanup renderer version is required")
	}
	if record.ReclaimedTokens <= 0 {
		return ToolResultCleanupRecord{}, fmt.Errorf("tool result cleanup reclaimed tokens must be positive")
	}
	if record.TriggeredAtUsage < 0 || record.WarmSuffixTokens < 0 {
		return ToolResultCleanupRecord{}, fmt.Errorf("tool result cleanup usage metrics cannot be negative")
	}
	if len(record.Replacements) == 0 {
		return ToolResultCleanupRecord{}, fmt.Errorf("tool result cleanup replacements are required")
	}
	record.Replacements = append([]ToolResultReplacement(nil), record.Replacements...)
	for index := range record.Replacements {
		replacement := &record.Replacements[index]
		replacement.ToolCallID = strings.TrimSpace(replacement.ToolCallID)
		if replacement.MessageIndex < record.SourceStart || replacement.MessageIndex >= record.SourceEnd {
			return ToolResultCleanupRecord{}, fmt.Errorf("tool result cleanup replacement index %d is outside source range", replacement.MessageIndex)
		}
		if replacement.ToolCallID == "" || replacement.Placeholder == "" {
			return ToolResultCleanupRecord{}, fmt.Errorf("tool result cleanup replacement %d requires tool_call_id and placeholder", index)
		}
	}
	sort.SliceStable(record.Replacements, func(left, right int) bool {
		if record.Replacements[left].MessageIndex == record.Replacements[right].MessageIndex {
			return record.Replacements[left].ToolCallID < record.Replacements[right].ToolCallID
		}
		return record.Replacements[left].MessageIndex < record.Replacements[right].MessageIndex
	})
	messageIndexes := make(map[int64]struct{}, len(record.Replacements))
	for _, replacement := range record.Replacements {
		_, duplicateIndex := messageIndexes[replacement.MessageIndex]
		if duplicateIndex {
			return ToolResultCleanupRecord{}, fmt.Errorf("tool result cleanup contains a duplicate replacement target")
		}
		messageIndexes[replacement.MessageIndex] = struct{}{}
	}
	record.EarliestChanged = record.Replacements[0].MessageIndex
	return record, nil
}

func sameToolResultCleanupIntent(existing, requested ToolResultCleanupRecord) bool {
	return existing.ID == requested.ID && existing.AgentKind == requested.AgentKind &&
		existing.SourceStart == requested.SourceStart && existing.SourceEnd == requested.SourceEnd &&
		slices.Equal(existing.Replacements, requested.Replacements) &&
		existing.ReclaimedTokens == requested.ReclaimedTokens && existing.TriggeredAtUsage == requested.TriggeredAtUsage &&
		existing.EarliestChanged == requested.EarliestChanged && existing.WarmSuffixTokens == requested.WarmSuffixTokens &&
		existing.RendererVersion == requested.RendererVersion
}

func cloneToolResultCleanupRecord(record ToolResultCleanupRecord) ToolResultCleanupRecord {
	record.Replacements = append([]ToolResultReplacement(nil), record.Replacements...)
	return record
}

func contextRecordMatchesAgent(recordAgentKind, requestedAgentKind string) bool {
	recordAgentKind = strings.TrimSpace(recordAgentKind)
	requestedAgentKind = strings.TrimSpace(requestedAgentKind)
	return requestedAgentKind == "" || recordAgentKind == "" || recordAgentKind == requestedAgentKind
}
