package interactive

import (
	"fmt"
	"os"
	"strings"
	"time"

	agentcontext "denova/internal/agents/context"
)

// ToolResultCleanupByID reads one stable cleanup event from the canonical
// story journal, including events no longer on the active branch ancestry.
func (s *Store) ToolResultCleanupByID(storyID, id string) (ToolResultCleanupEvent, bool, error) {
	if s == nil {
		return ToolResultCleanupEvent{}, false, fmt.Errorf("interactive store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, lines, err := s.readStoryJournalLocked(strings.TrimSpace(storyID))
	if os.IsNotExist(err) {
		return ToolResultCleanupEvent{}, false, nil
	}
	if err != nil {
		return ToolResultCleanupEvent{}, false, err
	}
	return toolResultCleanupEventByID(lines, id)
}

// AppendToolResultCleanup publishes one frozen cleanup projection at an exact
// branch head. Exact retries reconcile by stable event ID before evaluating the
// now-stale parent guard.
func (s *Store) AppendToolResultCleanup(
	storyID string,
	branchID string,
	event ToolResultCleanupEvent,
) (ToolResultCleanupEvent, error) {
	if s == nil {
		return ToolResultCleanupEvent{}, fmt.Errorf("interactive store is nil")
	}
	normalized, err := normalizeToolResultCleanupEvent(event)
	if err != nil {
		return ToolResultCleanupEvent{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return ToolResultCleanupEvent{}, err
	}
	defer releaseStory()

	meta, lines, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return ToolResultCleanupEvent{}, err
	}
	branchID = strings.TrimSpace(branchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	if existing, ok, findErr := toolResultCleanupEventByID(lines, normalized.ID); findErr != nil {
		return ToolResultCleanupEvent{}, findErr
	} else if ok {
		if !sameToolResultCleanupEventIntent(existing, normalized, branchID) {
			return ToolResultCleanupEvent{}, fmt.Errorf("%w: tool result cleanup id %q has different content", ErrStoryContextRevisionConflict, normalized.ID)
		}
		s.syncStoryIndexProjectionLocked(storyID, meta, len(lines))
		return cloneToolResultCleanupEvent(existing), nil
	}
	// The branch recent cache is deliberately bounded. A stable retry may be
	// older than that window, so canonical identity must be checked before the
	// parent CAS or a new append.
	_, all, err := s.readStoryJournalLocked(storyID)
	if err != nil {
		return ToolResultCleanupEvent{}, err
	}
	if existing, ok, findErr := toolResultCleanupEventByID(all, normalized.ID); findErr != nil {
		return ToolResultCleanupEvent{}, findErr
	} else if ok {
		if !sameToolResultCleanupEventIntent(existing, normalized, branchID) {
			return ToolResultCleanupEvent{}, fmt.Errorf("%w: tool result cleanup id %q has different content", ErrStoryContextRevisionConflict, normalized.ID)
		}
		s.syncStoryIndexProjectionLocked(storyID, meta, len(all))
		return cloneToolResultCleanupEvent(existing), nil
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return ToolResultCleanupEvent{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	if normalized.ExpectedParentID != nil && branch.Head != strings.TrimSpace(*normalized.ExpectedParentID) {
		return ToolResultCleanupEvent{}, fmt.Errorf(
			"%w: 当前分支已前进，拒绝提交基于旧版本的工具结果清理: expected_parent=%s current_head=%s",
			ErrStoryContextRevisionConflict, strings.TrimSpace(*normalized.ExpectedParentID), branch.Head,
		)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	normalized.V = schemaVersion
	normalized.Type = StoryEventTypeToolResultCleanup
	normalized.ParentID = branch.Head
	normalized.BranchID = branchID
	normalized.ExpectedParentID = nil
	if normalized.Ts == "" {
		normalized.Ts = now
	}
	branch.Head = normalized.ID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, normalized); err != nil {
		return ToolResultCleanupEvent{}, err
	}
	s.syncStoryIndexProjectionLocked(storyID, meta, len(all)+1)
	return cloneToolResultCleanupEvent(normalized), nil
}

func toolResultCleanupEventByID(lines []StoryEventRecord, id string) (ToolResultCleanupEvent, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ToolResultCleanupEvent{}, false, nil
	}
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeToolResultCleanup || record.Envelope.ID != id {
			continue
		}
		var event ToolResultCleanupEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return ToolResultCleanupEvent{}, false, err
		}
		normalized, err := normalizeToolResultCleanupEvent(event)
		if err != nil {
			return ToolResultCleanupEvent{}, false, err
		}
		return normalized, true, nil
	}
	return ToolResultCleanupEvent{}, false, nil
}

func normalizeToolResultCleanupEvent(event ToolResultCleanupEvent) (ToolResultCleanupEvent, error) {
	normalized, err := agentcontext.NormalizeToolResultCleanup(
		toolResultCleanupEventValue(event),
		func() string { return newID("trc") },
	)
	if err != nil {
		return ToolResultCleanupEvent{}, err
	}
	event.Type = StoryEventTypeToolResultCleanup
	return applyToolResultCleanupEventValue(event, normalized), nil
}

func sameToolResultCleanupEventIntent(existing, requested ToolResultCleanupEvent, branchID string) bool {
	return existing.BranchID == strings.TrimSpace(branchID) &&
		agentcontext.SameToolResultCleanupIntent(toolResultCleanupEventValue(existing), toolResultCleanupEventValue(requested))
}

func cloneToolResultCleanupEvent(event ToolResultCleanupEvent) ToolResultCleanupEvent {
	return applyToolResultCleanupEventValue(event, agentcontext.CloneToolResultCleanup(toolResultCleanupEventValue(event)))
}

func toolResultCleanupEventValue(event ToolResultCleanupEvent) agentcontext.ToolResultCleanup {
	return agentcontext.ToolResultCleanup{
		ID: event.ID, AgentKind: event.AgentKind, SourceStart: event.SourceStart, SourceEnd: event.SourceEnd,
		Replacements: event.Replacements, ReclaimedTokens: event.ReclaimedTokens, TriggeredAtUsage: event.TriggeredAtUsage,
		EarliestChanged: event.EarliestChanged, WarmSuffixTokens: event.WarmSuffixTokens, RendererVersion: event.RendererVersion,
	}
}

func applyToolResultCleanupEventValue(event ToolResultCleanupEvent, value agentcontext.ToolResultCleanup) ToolResultCleanupEvent {
	event.ID = value.ID
	event.AgentKind = value.AgentKind
	event.SourceStart = value.SourceStart
	event.SourceEnd = value.SourceEnd
	event.Replacements = value.Replacements
	event.ReclaimedTokens = value.ReclaimedTokens
	event.TriggeredAtUsage = value.TriggeredAtUsage
	event.EarliestChanged = value.EarliestChanged
	event.WarmSuffixTokens = value.WarmSuffixTokens
	event.RendererVersion = value.RendererVersion
	return event
}
