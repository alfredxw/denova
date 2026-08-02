package interactive

import (
	"fmt"
	"strings"
	"time"

	agentcontext "denova/internal/agents/context"
)

const (
	ContextCompactionHealthOutcomeFailure     = agentcontext.CompactionHealthFailure
	ContextCompactionHealthOutcomeSuccess     = agentcontext.CompactionHealthSuccess
	ContextCompactionHealthOutcomeManualRetry = agentcontext.CompactionHealthManualRetry
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
		s.syncStoryIndexProjectionLocked(storyID)
		return reconciled, nil
	}
	if projection.CompactionHealth != nil && projection.CompactionHealth.ID == normalized.ID {
		reconciled, reconcileErr := reconcileContextCompactionHealthEvent(*projection.CompactionHealth, normalized, branchID)
		if reconcileErr != nil {
			return ContextCompactionHealthEvent{}, reconcileErr
		}
		s.syncStoryIndexProjectionLocked(storyID)
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
		s.syncStoryIndexProjectionLocked(storyID)
		return reconciled, nil
	}
	if projection.ContextRevision != normalized.ExpectedContextRevision {
		return ContextCompactionHealthEvent{}, fmt.Errorf(
			"%w: context compaction health expected_revision=%d current_revision=%d",
			ErrStoryContextRevisionConflict, normalized.ExpectedContextRevision, projection.ContextRevision,
		)
	}

	normalized.BasisRevision = projection.ContextRevision
	var previousValue *agentcontext.CompactionHealth
	if projection.CompactionHealth != nil {
		value := contextCompactionHealthEventValue(*projection.CompactionHealth)
		previousValue = &value
	}
	normalized = applyContextCompactionHealthEventValue(
		normalized,
		agentcontext.AdvanceCompactionHealth(previousValue, contextCompactionHealthEventValue(normalized)),
	)
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
	s.syncStoryIndexProjectionLocked(storyID)
	return normalized, nil
}

func normalizeContextCompactionHealthEvent(event ContextCompactionHealthEvent) (ContextCompactionHealthEvent, error) {
	normalized, err := agentcontext.NormalizeCompactionHealth(contextCompactionHealthEventValue(event))
	if err != nil {
		return ContextCompactionHealthEvent{}, err
	}
	event.Type = StoryEventTypeCompactionHealth
	return applyContextCompactionHealthEventValue(event, normalized), nil
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
		agentcontext.SameCompactionHealthIntent(
			contextCompactionHealthEventValue(existing),
			contextCompactionHealthEventValue(requested),
		)
}

func contextCompactionHealthAgentMatches(left, right string) bool {
	return strings.TrimSpace(left) == strings.TrimSpace(right)
}

func contextCompactionHealthEventValue(event ContextCompactionHealthEvent) agentcontext.CompactionHealth {
	return agentcontext.CompactionHealth{
		ID: event.ID, AgentKind: event.AgentKind, StructureFingerprint: event.StructureFingerprint,
		Outcome: event.Outcome, FailureCode: event.FailureCode, ConsecutiveFailures: event.ConsecutiveFailures,
	}
}

func applyContextCompactionHealthEventValue(event ContextCompactionHealthEvent, value agentcontext.CompactionHealth) ContextCompactionHealthEvent {
	event.ID = value.ID
	event.AgentKind = value.AgentKind
	event.StructureFingerprint = value.StructureFingerprint
	event.Outcome = value.Outcome
	event.FailureCode = value.FailureCode
	event.ConsecutiveFailures = value.ConsecutiveFailures
	return event
}
