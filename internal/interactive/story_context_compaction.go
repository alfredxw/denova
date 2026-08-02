package interactive

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ContextCompactionByID reads a stable structural record from the canonical
// story log. It includes inactive historical checkpoints for crash recovery.
func (s *Store) ContextCompactionByID(storyID, id string) (ContextCompactionEvent, bool, error) {
	if s == nil {
		return ContextCompactionEvent{}, false, fmt.Errorf("interactive store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, lines, err := s.readStoryJournalLocked(strings.TrimSpace(storyID))
	if os.IsNotExist(err) {
		return ContextCompactionEvent{}, false, nil
	}
	if err != nil {
		return ContextCompactionEvent{}, false, err
	}
	return contextCompactionEventByID(lines, id)
}

// ContextCompactionRemovalByID is the removal counterpart to
// ContextCompactionByID and is likewise query-only.
func (s *Store) ContextCompactionRemovalByID(storyID, id string) (ContextCompactionRemovalEvent, bool, error) {
	if s == nil {
		return ContextCompactionRemovalEvent{}, false, fmt.Errorf("interactive store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, lines, err := s.readStoryJournalLocked(strings.TrimSpace(storyID))
	if os.IsNotExist(err) {
		return ContextCompactionRemovalEvent{}, false, nil
	}
	if err != nil {
		return ContextCompactionRemovalEvent{}, false, err
	}
	return contextCompactionRemovalEventByID(lines, id)
}

// AppendContextCompaction publishes an append-only model-history checkpoint at
// an exact branch head. Replaying the same stable event ID reconciles the
// original write; reusing it for different content is rejected.
func (s *Store) AppendContextCompaction(storyID, branchID string, event ContextCompactionEvent) (ContextCompactionEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return ContextCompactionEvent{}, err
	}
	defer releaseStory()

	meta, lines, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return ContextCompactionEvent{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	if existing, ok, findErr := contextCompactionEventByID(lines, event.ID); findErr != nil {
		return ContextCompactionEvent{}, findErr
	} else if ok {
		if !sameContextCompactionEventIntent(existing, event, branchID) {
			return ContextCompactionEvent{}, fmt.Errorf("%w: context compaction id %q has different content", ErrStoryContextRevisionConflict, event.ID)
		}
		return existing, nil
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return ContextCompactionEvent{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	if event.ExpectedParentID != nil && branch.Head != strings.TrimSpace(*event.ExpectedParentID) {
		return ContextCompactionEvent{}, fmt.Errorf("%w: 当前分支已前进，拒绝提交基于旧版本的上下文压缩: expected_parent=%s current_head=%s", ErrStoryContextRevisionConflict, strings.TrimSpace(*event.ExpectedParentID), branch.Head)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if event.ID == "" {
		event.ID = newID("cc")
	}
	event.V = schemaVersion
	event.Type = StoryEventTypeCompaction
	event.ParentID = branch.Head
	event.BranchID = branchID
	if event.Ts == "" {
		event.Ts = now
	}
	if event.Epoch <= 0 {
		event.Epoch = nextContextCompactionEpoch(lines, branch.Head)
	}
	branch.Head = event.ID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return ContextCompactionEvent{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return ContextCompactionEvent{}, err
	}
	return event, nil
}

// AppendContextCompactionRemoval soft-removes exactly one checkpoint without
// deleting raw story events. It follows the same stable-ID and branch-head CAS
// rules as checkpoint publication.
func (s *Store) AppendContextCompactionRemoval(storyID, branchID string, event ContextCompactionRemovalEvent) (ContextCompactionRemovalEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return ContextCompactionRemovalEvent{}, err
	}
	defer releaseStory()

	meta, lines, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return ContextCompactionRemovalEvent{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	if existing, ok, findErr := contextCompactionRemovalEventByID(lines, event.ID); findErr != nil {
		return ContextCompactionRemovalEvent{}, findErr
	} else if ok {
		if !sameContextCompactionRemovalEventIntent(existing, event, branchID) {
			return ContextCompactionRemovalEvent{}, fmt.Errorf("%w: context compaction removal id %q has different content", ErrStoryContextRevisionConflict, event.ID)
		}
		return existing, nil
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return ContextCompactionRemovalEvent{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	if event.ExpectedParentID != nil && branch.Head != strings.TrimSpace(*event.ExpectedParentID) {
		return ContextCompactionRemovalEvent{}, fmt.Errorf("%w: 当前分支已前进，拒绝提交基于旧版本的压缩撤销: expected_parent=%s current_head=%s", ErrStoryContextRevisionConflict, strings.TrimSpace(*event.ExpectedParentID), branch.Head)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if event.ID == "" {
		event.ID = newID("ccr")
	}
	event.V = schemaVersion
	event.Type = StoryEventTypeCompactionRemoved
	event.ParentID = branch.Head
	event.BranchID = branchID
	if event.Ts == "" {
		event.Ts = now
	}
	branch.Head = event.ID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return ContextCompactionRemovalEvent{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return ContextCompactionRemovalEvent{}, err
	}
	return event, nil
}

func contextCompactionEventByID(lines []StoryEventRecord, id string) (ContextCompactionEvent, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ContextCompactionEvent{}, false, nil
	}
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeCompaction || record.Envelope.ID != id {
			continue
		}
		var event ContextCompactionEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return ContextCompactionEvent{}, false, err
		}
		return event, true, nil
	}
	return ContextCompactionEvent{}, false, nil
}

func contextCompactionRemovalEventByID(lines []StoryEventRecord, id string) (ContextCompactionRemovalEvent, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ContextCompactionRemovalEvent{}, false, nil
	}
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeCompactionRemoved || record.Envelope.ID != id {
			continue
		}
		var event ContextCompactionRemovalEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return ContextCompactionRemovalEvent{}, false, err
		}
		return event, true, nil
	}
	return ContextCompactionRemovalEvent{}, false, nil
}

func sameContextCompactionEventIntent(existing, requested ContextCompactionEvent, branchID string) bool {
	return existing.ID == requested.ID && existing.BranchID == strings.TrimSpace(branchID) &&
		existing.CompactionCheckpoint == requested.CompactionCheckpoint &&
		existing.SourceTurnCount == requested.SourceTurnCount
}

func sameContextCompactionRemovalEventIntent(existing, requested ContextCompactionRemovalEvent, branchID string) bool {
	return existing.ID == requested.ID && existing.BranchID == strings.TrimSpace(branchID) &&
		existing.AgentKind == requested.AgentKind && existing.CompactionID == requested.CompactionID &&
		existing.SourceTurnCount == requested.SourceTurnCount && existing.Reason == requested.Reason
}
