package interactive

import (
	"fmt"
	"strings"
	"time"
)

const (
	ContextCompactionHealthOutcomeFailure     = "failure"
	ContextCompactionHealthOutcomeSuccess     = "success"
	ContextCompactionHealthOutcomeManualRetry = "manual_retry"
)

// ContextCompactionHealthState returns the branch's model-context revision and
// the latest health transition when it is still active for that exact revision.
func (s *Store) ContextCompactionHealthState(
	storyID, branchID, agentKind string,
) (uint64, ContextCompactionHealthEvent, bool, error) {
	if s == nil {
		return 0, ContextCompactionHealthEvent{}, false, fmt.Errorf("interactive store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, _, err := s.readStoryRecentLocked(strings.TrimSpace(storyID), strings.TrimSpace(branchID))
	if err != nil {
		return 0, ContextCompactionHealthEvent{}, false, err
	}
	branchID = strings.TrimSpace(branchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return 0, ContextCompactionHealthEvent{}, false, err
	}
	revision := projection.ContextRevision
	health := projection.CompactionHealth
	if health == nil || !contextCompactionHealthAgentMatches(health.AgentKind, agentKind) {
		return revision, ContextCompactionHealthEvent{}, false, nil
	}
	return revision, *health, true, nil
}

// AppendContextCompactionHealth appends one model-invisible transition against
// an exact branch context revision. It deliberately leaves the branch head and
// context revision unchanged, so the row does not invalidate itself.
func (s *Store) AppendContextCompactionHealth(
	storyID, branchID string,
	event ContextCompactionHealthEvent,
) (ContextCompactionHealthEvent, error) {
	if s == nil {
		return ContextCompactionHealthEvent{}, fmt.Errorf("interactive store is nil")
	}
	normalized, err := normalizeContextCompactionHealthEvent(event)
	if err != nil {
		return ContextCompactionHealthEvent{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return ContextCompactionHealthEvent{}, err
	}
	defer releaseStory()

	meta, recent, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return ContextCompactionHealthEvent{}, err
	}
	branchID = strings.TrimSpace(branchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branchMeta, ok := meta.Branches[branchID]
	if !ok {
		return ContextCompactionHealthEvent{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return ContextCompactionHealthEvent{}, err
	}
	if existing, ok, findErr := contextCompactionHealthEventByID(recent, normalized.ID); findErr != nil {
		return ContextCompactionHealthEvent{}, findErr
	} else if ok {
		reconciled, reconcileErr := reconcileContextCompactionHealthEvent(existing, normalized, branchID)
		if reconcileErr != nil {
			return ContextCompactionHealthEvent{}, reconcileErr
		}
		s.syncStoryIndexProjectionLocked(storyID, meta, len(recent))
		return reconciled, nil
	}
	if projection.CompactionHealth != nil && projection.CompactionHealth.ID == normalized.ID {
		reconciled, reconcileErr := reconcileContextCompactionHealthEvent(*projection.CompactionHealth, normalized, branchID)
		if reconcileErr != nil {
			return ContextCompactionHealthEvent{}, reconcileErr
		}
		s.syncStoryIndexProjectionLocked(storyID, meta, len(recent))
		return reconciled, nil
	}
	// Health rows do not advance the branch revision, so an exact retry can be
	// older than both the bounded recent cache and the latest projected health.
	// Reconcile the stable ID from canonical JSONL before evaluating the CAS.
	_, all, err := s.readStoryJournalLocked(storyID)
	if err != nil {
		return ContextCompactionHealthEvent{}, err
	}
	if existing, found, findErr := contextCompactionHealthEventByID(all, normalized.ID); findErr != nil {
		return ContextCompactionHealthEvent{}, findErr
	} else if found {
		reconciled, reconcileErr := reconcileContextCompactionHealthEvent(existing, normalized, branchID)
		if reconcileErr != nil {
			return ContextCompactionHealthEvent{}, reconcileErr
		}
		s.syncStoryIndexProjectionLocked(storyID, meta, len(all))
		return reconciled, nil
	}
	if projection.ContextRevision != normalized.ExpectedContextRevision {
		return ContextCompactionHealthEvent{}, fmt.Errorf(
			"%w: context compaction health expected_revision=%d current_revision=%d",
			ErrStoryContextRevisionConflict, normalized.ExpectedContextRevision, projection.ContextRevision,
		)
	}

	normalized.BasisRevision = projection.ContextRevision
	previous := projection.CompactionHealth
	switch normalized.Outcome {
	case ContextCompactionHealthOutcomeFailure:
		normalized.ConsecutiveFailures = 1
		if previous != nil && previous.StructureFingerprint == normalized.StructureFingerprint &&
			contextCompactionHealthAgentMatches(previous.AgentKind, normalized.AgentKind) {
			normalized.ConsecutiveFailures = previous.ConsecutiveFailures + 1
		}
	case ContextCompactionHealthOutcomeSuccess, ContextCompactionHealthOutcomeManualRetry:
		normalized.ConsecutiveFailures = 0
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	normalized.V = schemaVersion
	normalized.Type = StoryEventTypeCompactionHealth
	normalized.ParentID = branchMeta.Head
	normalized.BranchID = branchID
	normalized.ExpectedContextRevision = 0
	if normalized.Ts == "" {
		normalized.Ts = now
	}
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, normalized); err != nil {
		return ContextCompactionHealthEvent{}, err
	}
	s.syncStoryIndexProjectionLocked(storyID, meta, len(all)+1)
	return normalized, nil
}

func normalizeContextCompactionHealthEvent(event ContextCompactionHealthEvent) (ContextCompactionHealthEvent, error) {
	event.Type = StoryEventTypeCompactionHealth
	event.ID = strings.TrimSpace(event.ID)
	event.AgentKind = strings.TrimSpace(event.AgentKind)
	event.StructureFingerprint = strings.TrimSpace(event.StructureFingerprint)
	event.Outcome = strings.TrimSpace(event.Outcome)
	event.FailureCode = strings.TrimSpace(event.FailureCode)
	if event.ID == "" || event.StructureFingerprint == "" {
		return ContextCompactionHealthEvent{}, fmt.Errorf("context compaction health requires id and structure fingerprint")
	}
	switch event.Outcome {
	case ContextCompactionHealthOutcomeFailure:
		if event.FailureCode == "" {
			return ContextCompactionHealthEvent{}, fmt.Errorf("failed context compaction health requires failure code")
		}
	case ContextCompactionHealthOutcomeSuccess, ContextCompactionHealthOutcomeManualRetry:
		event.FailureCode = ""
	default:
		return ContextCompactionHealthEvent{}, fmt.Errorf("unsupported context compaction health outcome %q", event.Outcome)
	}
	return event, nil
}

func contextCompactionHealthEventByID(lines []StoryEventRecord, id string) (ContextCompactionHealthEvent, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ContextCompactionHealthEvent{}, false, nil
	}
	for index := len(lines) - 1; index >= 0; index-- {
		record := lines[index]
		if record.Envelope.Type != StoryEventTypeCompactionHealth || record.Envelope.ID != id {
			continue
		}
		var event ContextCompactionHealthEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return ContextCompactionHealthEvent{}, false, err
		}
		normalized, err := normalizeContextCompactionHealthEvent(event)
		return normalized, true, err
	}
	return ContextCompactionHealthEvent{}, false, nil
}

func reconcileContextCompactionHealthEvent(
	existing, requested ContextCompactionHealthEvent,
	branchID string,
) (ContextCompactionHealthEvent, error) {
	if !sameContextCompactionHealthEventIntent(existing, requested, branchID) {
		return ContextCompactionHealthEvent{}, fmt.Errorf(
			"%w: context compaction health id %q has different content",
			ErrStoryContextRevisionConflict, requested.ID,
		)
	}
	return existing, nil
}

func sameContextCompactionHealthEventIntent(existing, requested ContextCompactionHealthEvent, branchID string) bool {
	return existing.ID == requested.ID && existing.BranchID == strings.TrimSpace(branchID) &&
		contextCompactionHealthAgentMatches(existing.AgentKind, requested.AgentKind) &&
		existing.StructureFingerprint == requested.StructureFingerprint && existing.Outcome == requested.Outcome &&
		existing.FailureCode == requested.FailureCode
}

func contextCompactionHealthAgentMatches(left, right string) bool {
	return strings.TrimSpace(left) == strings.TrimSpace(right)
}
