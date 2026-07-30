package session

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	contextCompactionHealthFailure     = "failure"
	contextCompactionHealthSuccess     = "success"
	contextCompactionHealthManualRetry = "manual_retry"
)

// CommitContextCompactionHealthAtContext appends a model-invisible health
// transition against one exact model-context revision. The health row does not
// advance ContextRevision; otherwise it would invalidate itself immediately.
func (s *Session) CommitContextCompactionHealthAtContext(
	ctx context.Context,
	expected ContextCursor,
	record ContextCompactionHealth,
) (ContextCompactionHealth, error) {
	result := record
	err := s.withCanonicalMutation(ctx, "commit context compaction health", func() error {
		normalized, normalizeErr := normalizeContextCompactionHealth(record)
		if normalizeErr != nil {
			return normalizeErr
		}
		if existing, ok := s.contextCompactionHealthByIDLocked(normalized.ID); ok {
			if !sameContextCompactionHealthIntent(existing, normalized) {
				return fmt.Errorf("%w: context compaction health id %q has different content", ErrDomainCommitIdentityConflict, normalized.ID)
			}
			result = existing
			return nil
		}
		if current := s.contextCursorLocked(); current.Revision != expected.Revision {
			return fmt.Errorf("%w: expected=%d current=%d", ErrContextRevisionConflict, expected.Revision, current.Revision)
		}
		normalized.BasisRevision = expected.Revision
		previous, active := s.latestContextCompactionHealthLocked(normalized.AgentKind)
		switch normalized.Outcome {
		case contextCompactionHealthFailure:
			normalized.ConsecutiveFailures = 1
			if active && previous.StructureFingerprint == normalized.StructureFingerprint {
				normalized.ConsecutiveFailures = previous.ConsecutiveFailures + 1
			}
		case contextCompactionHealthSuccess, contextCompactionHealthManualRetry:
			normalized.ConsecutiveFailures = 0
		}
		if appendErr := s.appendJournalRecordLocked(normalized); appendErr != nil {
			return appendErr
		}
		s.records = append(s.records, historyRecord{kind: historyTypeCompactionHealth, compactionHealth: &normalized, createdAt: normalized.CreatedAt})
		advanceUpdatedAt(s, normalized.CreatedAt)
		result = normalized
		return nil
	})
	return result, err
}

// LatestContextCompactionHealth carries the failure state across ordinary
// transcript growth, but releases it when a newer rewind selects a different
// model branch. Health rows remain in the append-only audit journal.
func (s *Session) LatestContextCompactionHealth(agentKind string) (ContextCompactionHealth, bool) {
	if s == nil {
		return ContextCompactionHealth{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latestContextCompactionHealthLocked(agentKind)
}

func (s *Session) latestContextCompactionHealthLocked(agentKind string) (ContextCompactionHealth, bool) {
	var health ContextCompactionHealth
	var found bool
	if s.projection != nil {
		for index := len(s.projection.Structural) - 1; index >= 0; index-- {
			candidate := s.projection.Structural[index].Health
			if candidate != nil && contextRecordMatchesAgent(candidate.AgentKind, agentKind) {
				health, found = *candidate, true
				break
			}
		}
	} else {
		for index := len(s.records) - 1; index >= 0; index-- {
			candidate := s.records[index].compactionHealth
			if candidate != nil && contextRecordMatchesAgent(candidate.AgentKind, agentKind) {
				health, found = *candidate, true
				break
			}
		}
	}
	if !found {
		return ContextCompactionHealth{}, false
	}
	if rewind, active, err := s.latestContextWindowProjectionLocked(agentKind); err == nil && active && rewind.ContextRevision > health.BasisRevision {
		// A rewind selects a different model branch without deleting the prior
		// health audit row. That structural change releases the old fuse while
		// ordinary transcript growth intentionally does not.
		return ContextCompactionHealth{}, false
	}
	return health, true
}

func normalizeContextCompactionHealth(record ContextCompactionHealth) (ContextCompactionHealth, error) {
	record.Type = historyTypeCompactionHealth
	record.ID = strings.TrimSpace(record.ID)
	record.AgentKind = strings.TrimSpace(record.AgentKind)
	record.StructureFingerprint = strings.TrimSpace(record.StructureFingerprint)
	record.Outcome = strings.TrimSpace(record.Outcome)
	record.FailureCode = strings.TrimSpace(record.FailureCode)
	if record.ID == "" || record.StructureFingerprint == "" {
		return ContextCompactionHealth{}, fmt.Errorf("context compaction health requires id and structure fingerprint")
	}
	switch record.Outcome {
	case contextCompactionHealthFailure:
		if record.FailureCode == "" {
			return ContextCompactionHealth{}, fmt.Errorf("failed context compaction health requires failure code")
		}
	case contextCompactionHealthSuccess, contextCompactionHealthManualRetry:
		record.FailureCode = ""
	default:
		return ContextCompactionHealth{}, fmt.Errorf("unsupported context compaction health outcome %q", record.Outcome)
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	return record, nil
}

func (s *Session) contextCompactionHealthByIDLocked(id string) (ContextCompactionHealth, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ContextCompactionHealth{}, false
	}
	if s.projection != nil {
		for index := len(s.projection.Structural) - 1; index >= 0; index-- {
			if health := s.projection.Structural[index].Health; health != nil && health.ID == id {
				return *health, true
			}
		}
	}
	for index := len(s.records) - 1; index >= 0; index-- {
		if health := s.records[index].compactionHealth; health != nil && health.ID == id {
			return *health, true
		}
	}
	return ContextCompactionHealth{}, false
}

func sameContextCompactionHealthIntent(existing, requested ContextCompactionHealth) bool {
	return existing.ID == requested.ID && existing.AgentKind == requested.AgentKind &&
		existing.StructureFingerprint == requested.StructureFingerprint && existing.Outcome == requested.Outcome &&
		existing.FailureCode == requested.FailureCode
}
