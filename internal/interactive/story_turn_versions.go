package interactive

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"denova/internal/agents/conversationjournal"
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
	parentID := parentIDFromRaw(target.Raw)
	if parentID != parentIDFromRaw(current.Raw) {
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
	handle, err := s.openStoryJournalLocked(storyID)
	if err != nil {
		return err
	}
	projection := handle.projection.Branches[branchID]
	if projection == nil || projection.LatestTurnID != turnID {
		return fmt.Errorf("%w: turn version selection does not match the branch projection", ErrStoryContextRevisionConflict)
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
		CurrentTurnID:   versionTurnID,
		CurrentDepth:    projection.Depth,
	}
	parentState := projection.StateBeforeLatest
	parentPlan := projection.PlanBeforeLatest
	if parentState == nil || projection.LatestTurnParentID != parentID {
		checkpoint, err := s.checkpointAtStoryEventLocked(handle, parentID)
		if err != nil {
			return err
		}
		parentState, parentPlan = checkpoint.State, checkpoint.Plan
		selection.CurrentDepth = checkpoint.Depth + 1
		selection.ParentState, selection.ParentPlan = parentState, parentPlan
	}
	selection.CurrentState, selection.CurrentPlan, err = projectSelectedTurnVersion(handle, target, parentState, parentPlan, projectedEvents)
	if err != nil {
		return err
	}
	newEvents = append(newEvents, selection)

	branch.Head = projectedHead
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, newEvents...); err != nil {
		return err
	}
	return s.syncStorySummaryLocked(storyID)
}

// projectSelectedTurnVersion applies only the selected turn and its suffix to
// the common parent. Inactive versions can have state and plan edits outside
// the recent view, so read their side records from the journal.
func projectSelectedTurnVersion(handle *storyJournalHandle, selected StoryEventRecord, parentState map[string]any, parentPlan *BranchPlan, suffix []any) (map[string]any, *BranchPlan, error) {
	var turn TurnEvent
	if err := mapToStruct(selected.Raw, &turn); err != nil {
		return nil, nil, err
	}
	node := handle.projection.ModelNodes[turn.ID]
	if node.Cursor == 0 {
		return nil, nil, fmt.Errorf("missing journal locator for turn version %s", turn.ID)
	}
	plan := cloneBranchPlan(parentPlan)
	err := handle.journal.VisitRange(context.Background(), conversationjournal.Range{After: node.Cursor - 1}, func(record conversationjournal.Record) error {
		var header struct {
			Type     string `json:"type"`
			BranchID string `json:"branch_id"`
			TurnID   string `json:"turn_id"`
		}
		if err := json.Unmarshal(record.Payload, &header); err != nil {
			return err
		}
		if header.BranchID != turn.BranchID || header.TurnID != turn.ID {
			return nil
		}
		switch header.Type {
		case StoryEventTypeTurnStateRevised:
			var revision TurnStateRevisedEvent
			if err := json.Unmarshal(record.Payload, &revision); err != nil {
				return err
			}
			if revision.ClearStateDelta {
				turn.StateDelta = nil
			} else if revision.StateDelta != nil {
				turn.StateDelta = revision.StateDelta
			}
		case StoryEventTypeBranchPlanUpdated:
			var update BranchPlanUpdatedEvent
			if err := json.Unmarshal(record.Payload, &update); err != nil {
				return err
			}
			plan = &BranchPlan{Markdown: update.Markdown, UpdatedTurnID: update.TurnID, UpdatedAt: update.Ts, Revision: update.ID}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	state := cloneStoryState(parentState)
	applyTurnState(state, turn)
	for _, raw := range suffix {
		record, err := storyEventRecordForWrite(raw)
		if err != nil {
			return nil, nil, err
		}
		switch record.Envelope.Type {
		case StoryEventTypeStateDelta:
			var delta StateDeltaEvent
			if err := mapToStruct(record.Raw, &delta); err != nil {
				return nil, nil, err
			}
			applyStateDeltaToProjection(state, StateDelta{SchemaVersion: delta.SchemaVersion, Ops: delta.Ops, ActorOps: delta.ActorOps})
		case StoryEventTypeBranchPlanRevised:
			var update BranchPlanUpdatedEvent
			if err := mapToStruct(record.Raw, &update); err != nil {
				return nil, nil, err
			}
			plan = &BranchPlan{Markdown: update.Markdown, UpdatedTurnID: turn.ID, UpdatedAt: update.Ts, Revision: update.ID}
		}
	}
	return state, plan, nil
}

func turnRecordOnPath(path []StoryEventRecord, turnID string) (int, *StoryEventRecord) {
	for index := range path {
		if path[index].Envelope.ID == turnID && path[index].Envelope.Type == StoryEventTypeTurn {
			return index, &path[index]
		}
	}
	return -1, nil
}

// projectTurnVersionSuffix creates new envelopes for the active suffix. Agent
// maintenance is not part of the Story journal, so every canonical event is
// projected exactly once onto the selected version path.
func projectTurnVersionSuffix(
	suffix []StoryEventRecord,
	selectedTurnID, selectionID string,
) (string, []any, []TurnVersionProjection, error) {
	parentID := selectedTurnID
	projected := make([]any, 0, len(suffix))
	projections := make([]TurnVersionProjection, 0, len(suffix))
	for _, record := range suffix {
		clonedRaw, projection, err := cloneTurnVersionPathEvent(record, parentID, selectionID)
		if err != nil {
			return "", nil, nil, err
		}
		if record.Envelope.Type == StoryEventTypeBranchPlanRevised {
			clonedRaw["turn_id"] = selectedTurnID
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
