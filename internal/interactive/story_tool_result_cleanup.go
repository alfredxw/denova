package interactive

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"time"
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
	event.Type = StoryEventTypeToolResultCleanup
	event.ID = strings.TrimSpace(event.ID)
	if event.ID == "" {
		event.ID = newID("trc")
	}
	event.AgentKind = strings.TrimSpace(event.AgentKind)
	event.RendererVersion = strings.TrimSpace(event.RendererVersion)
	if event.SourceStart < 0 || event.SourceEnd <= event.SourceStart {
		return ToolResultCleanupEvent{}, fmt.Errorf("tool result cleanup source range [%d,%d) is invalid", event.SourceStart, event.SourceEnd)
	}
	if event.RendererVersion == "" {
		return ToolResultCleanupEvent{}, fmt.Errorf("tool result cleanup renderer version is required")
	}
	if event.ReclaimedTokens <= 0 {
		return ToolResultCleanupEvent{}, fmt.Errorf("tool result cleanup reclaimed tokens must be positive")
	}
	if event.TriggeredAtUsage < 0 || event.WarmSuffixTokens < 0 {
		return ToolResultCleanupEvent{}, fmt.Errorf("tool result cleanup usage metrics cannot be negative")
	}
	if len(event.Replacements) == 0 {
		return ToolResultCleanupEvent{}, fmt.Errorf("tool result cleanup replacements are required")
	}
	event.Replacements = append([]ToolResultReplacement(nil), event.Replacements...)
	for index := range event.Replacements {
		replacement := &event.Replacements[index]
		replacement.ToolCallID = strings.TrimSpace(replacement.ToolCallID)
		if replacement.MessageIndex < event.SourceStart || replacement.MessageIndex >= event.SourceEnd {
			return ToolResultCleanupEvent{}, fmt.Errorf("tool result cleanup replacement index %d is outside source range", replacement.MessageIndex)
		}
		if replacement.ToolCallID == "" || replacement.Placeholder == "" {
			return ToolResultCleanupEvent{}, fmt.Errorf("tool result cleanup replacement %d requires tool_call_id and placeholder", index)
		}
	}
	sort.SliceStable(event.Replacements, func(left, right int) bool {
		if event.Replacements[left].MessageIndex == event.Replacements[right].MessageIndex {
			return event.Replacements[left].ToolCallID < event.Replacements[right].ToolCallID
		}
		return event.Replacements[left].MessageIndex < event.Replacements[right].MessageIndex
	})
	messageIndexes := make(map[int64]struct{}, len(event.Replacements))
	for _, replacement := range event.Replacements {
		_, duplicateIndex := messageIndexes[replacement.MessageIndex]
		if duplicateIndex {
			return ToolResultCleanupEvent{}, fmt.Errorf("tool result cleanup contains a duplicate replacement target")
		}
		messageIndexes[replacement.MessageIndex] = struct{}{}
	}
	event.EarliestChanged = event.Replacements[0].MessageIndex
	return event, nil
}

func sameToolResultCleanupEventIntent(existing, requested ToolResultCleanupEvent, branchID string) bool {
	return existing.ID == requested.ID && existing.BranchID == strings.TrimSpace(branchID) &&
		existing.AgentKind == requested.AgentKind && existing.SourceStart == requested.SourceStart && existing.SourceEnd == requested.SourceEnd &&
		slices.Equal(existing.Replacements, requested.Replacements) && existing.ReclaimedTokens == requested.ReclaimedTokens &&
		existing.TriggeredAtUsage == requested.TriggeredAtUsage && existing.EarliestChanged == requested.EarliestChanged &&
		existing.WarmSuffixTokens == requested.WarmSuffixTokens && existing.RendererVersion == requested.RendererVersion
}

func cloneToolResultCleanupEvent(event ToolResultCleanupEvent) ToolResultCleanupEvent {
	event.Replacements = append([]ToolResultReplacement(nil), event.Replacements...)
	return event
}
