package interactive

import (
	"fmt"
	"strings"
	"time"
)

// SwitchTurnVersion selects an alternative turn without rewriting any
// historical event. Descendants are appended as a fresh projection whose
// parent chain starts at the selected version; the original suffix remains an
// immutable, inspectable history.
func (s *Store) SwitchTurnVersion(storyID string, req SwitchTurnVersionRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return err
	}
	defer releaseStory()

	turnID := strings.TrimSpace(req.TurnID)
	versionTurnID := strings.TrimSpace(req.VersionTurnID)
	if turnID == "" || versionTurnID == "" {
		return fmt.Errorf("回合版本参数不能为空")
	}
	meta, lines, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return err
	}
	branchID := strings.TrimSpace(req.BranchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	if err := requireLatestLogicalTurn(meta, lines, branchID, turnID); err != nil {
		return err
	}
	lines, err = projectStoryEventOverlays(lines)
	if err != nil {
		return err
	}
	events := eventsByID(lines)
	path, pathSet := eventPath(branch.Head, events)
	if !pathSet[turnID] {
		return fmt.Errorf("只能切换当前剧情路径上的回合版本: %s", turnID)
	}
	currentIndex, current := turnRecordOnPath(path, turnID)
	if current == nil {
		return fmt.Errorf("回合不存在: %s", turnID)
	}
	target, ok := events[versionTurnID]
	if !ok {
		return fmt.Errorf("目标版本不存在: %s", versionTurnID)
	}
	if target.Envelope.Type != StoryEventTypeTurn {
		return fmt.Errorf("目标版本不是互动回合: %s", versionTurnID)
	}
	if target.Envelope.BranchID != branchID {
		return fmt.Errorf("目标版本不属于当前分支: %s", versionTurnID)
	}
	if parentIDFromRaw(target.Raw) != parentIDFromRaw(current.Raw) {
		return fmt.Errorf("只能在同一剧情位置切换版本")
	}
	if turnID == versionTurnID {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	selectionID := newID("vs")
	projectedHead, projectedEvents, projections, err := projectTurnVersionSuffix(path[currentIndex+1:], versionTurnID, selectionID)
	if err != nil {
		return err
	}
	newEvents := append([]any(nil), projectedEvents...)
	invalidated := activeDescendantCompaction(path, currentIndex)
	if invalidated != nil {
		removal := ContextCompactionRemovalEvent{
			V:               schemaVersion,
			Type:            StoryEventTypeCompactionRemoved,
			ID:              newID("ccr"),
			ParentID:        projectedHead,
			BranchID:        branchID,
			Ts:              now,
			AgentKind:       invalidated.AgentKind,
			CompactionID:    invalidated.ID,
			SourceTurnCount: invalidated.SourceTurnCount,
			Reason:          "turn_version_switched",
		}
		projectedHead = removal.ID
		newEvents = append(newEvents, removal)
	}

	selection := TurnVersionSelectionEvent{
		V:               schemaVersion,
		Type:            StoryEventTypeTurnVersionSelected,
		ID:              selectionID,
		ParentID:        projectedHead,
		BranchID:        branchID,
		Ts:              now,
		ReplacedTurnID:  turnID,
		SelectedTurnID:  versionTurnID,
		PreviousHeadID:  branch.Head,
		ProjectedHeadID: projectedHead,
		ProjectedEvents: projections,
	}
	projectedRecords := make([]StoryEventRecord, 0, len(projectedEvents))
	for _, projectedEvent := range projectedEvents {
		record, recordErr := storyEventRecordForWrite(projectedEvent)
		if recordErr != nil {
			return recordErr
		}
		projectedRecords = append(projectedRecords, record)
	}
	selectionEvents := eventsByID(append(append([]StoryEventRecord(nil), lines...), projectedRecords...))
	selectionPath, _ := eventPath(projectedHead, selectionEvents)
	selection.CurrentState = stateFromPath(selectionPath)
	selection.CurrentTurnID = nearestTurnAncestor(projectedHead, selectionEvents)
	for _, record := range selectionPath {
		if record.Envelope.Type == StoryEventTypeTurn {
			selection.CurrentDepth++
		}
	}
	if invalidated != nil {
		selection.InvalidatedCompactionID = invalidated.ID
	}
	newEvents = append(newEvents, selection)

	branch.Head = projectedHead
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, newEvents...); err != nil {
		return err
	}
	return s.touchIndexLocked(storyID, now, len(newEvents))
}

func turnRecordOnPath(path []StoryEventRecord, turnID string) (int, *StoryEventRecord) {
	for index := range path {
		if path[index].Envelope.ID == turnID && path[index].Envelope.Type == StoryEventTypeTurn {
			return index, &path[index]
		}
	}
	return -1, nil
}

// projectTurnVersionSuffix creates new envelopes for the active suffix. A
// compaction summaries and cleanup projections are never copied onto a changed
// ancestor. Existing removal markers are copied so a checkpoint active before
// the selected turn cannot accidentally reappear after projecting the suffix.
func projectTurnVersionSuffix(
	suffix []StoryEventRecord,
	selectedTurnID, selectionID string,
) (string, []any, []TurnVersionProjection, error) {
	parentID := selectedTurnID
	projected := make([]any, 0, len(suffix))
	projections := make([]TurnVersionProjection, 0, len(suffix))
	for _, record := range suffix {
		if record.Envelope.Type == StoryEventTypeCompaction || record.Envelope.Type == StoryEventTypeCompactionHealth || record.Envelope.Type == StoryEventTypeToolResultCleanup {
			continue
		}
		clonedRaw, projection, err := cloneTurnVersionPathEvent(record, parentID, selectionID)
		if err != nil {
			return "", nil, nil, err
		}
		parentID = projection.ProjectedID
		projected = append(projected, clonedRaw)
		projections = append(projections, projection)
	}
	return parentID, projected, projections, nil
}

func cloneTurnVersionPathEvent(record StoryEventRecord, parentID, selectionID string) (map[string]any, TurnVersionProjection, error) {
	if record.Envelope.ID == "" {
		return nil, TurnVersionProjection{}, fmt.Errorf("剧情后续节点缺少 ID，无法切换版本")
	}
	clonedRaw := make(map[string]any, len(record.Raw)+1)
	for key, value := range record.Raw {
		clonedRaw[key] = value
	}
	projectedID := newID("ev")
	clonedRaw["id"] = projectedID
	if parentID == "" {
		clonedRaw["parent_id"] = nil
	} else {
		clonedRaw["parent_id"] = parentID
	}
	clonedRaw["version_projection"] = map[string]any{
		"selection_id":    selectionID,
		"source_event_id": record.Envelope.ID,
	}
	if _, err := mapToStoryEventRecord(clonedRaw); err != nil {
		return nil, TurnVersionProjection{}, fmt.Errorf("投影剧情后续节点 %s 失败: %w", record.Envelope.ID, err)
	}
	return clonedRaw, TurnVersionProjection{
		SourceID: record.Envelope.ID, ProjectedID: projectedID, EventType: record.Envelope.Type,
	}, nil
}

func activeDescendantCompaction(path []StoryEventRecord, selectedIndex int) *ContextCompactionEvent {
	activeIndex := -1
	var active *ContextCompactionEvent
	for index, record := range path {
		switch record.Envelope.Type {
		case StoryEventTypeCompaction:
			var event ContextCompactionEvent
			if err := mapToStruct(record.Raw, &event); err != nil {
				continue
			}
			active = &event
			activeIndex = index
		case StoryEventTypeCompactionRemoved:
			active = nil
			activeIndex = -1
		}
	}
	if active == nil || activeIndex <= selectedIndex {
		return nil
	}
	copy := *active
	return &copy
}
