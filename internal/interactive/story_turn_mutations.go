package interactive

import (
	"fmt"
	"strings"
	"time"
)

func (s *Store) AppendTurn(storyID string, req AppendTurnRequest) (TurnEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return TurnEvent{}, err
	}
	defer releaseStory()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return TurnEvent{}, err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return TurnEvent{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	if branchIsTerminal(lines, branch.Head) {
		return TurnEvent{}, fmt.Errorf("当前分支已终局，请从历史回合创建新分支后继续")
	}
	parentID := any(nil)
	if branch.Head != "" {
		parentID = branch.Head
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := TurnEvent{
		V:                    schemaVersion,
		Type:                 StoryEventTypeTurn,
		ID:                   newID("ev"),
		ParentID:             parentID,
		BranchID:             branchID,
		Ts:                   now,
		User:                 req.User,
		Narrative:            req.Narrative,
		Thinking:             strings.TrimSpace(req.Thinking),
		DisplayEvents:        sanitizeDisplayEvents(req.DisplayEvents),
		ModelContextMessages: sanitizeModelContextMessages(req.ModelContextMessages),
		Flags:                map[string]bool{"pinned": false, "locked": false},
	}
	branch.Head = event.ID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return TurnEvent{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 1); err != nil {
		return TurnEvent{}, err
	}
	return event, nil
}

// AppendTurnDisplayEvent appends a display-only event to an existing turn.
// The event is kept out of future model context and does not move branch head.
func (s *Store) AppendTurnDisplayEvent(storyID, branchID, turnID string, event DisplayEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return err
	}
	defer releaseStory()

	events := sanitizeDisplayEvents([]DisplayEvent{event})
	if len(events) == 0 {
		return nil
	}
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return fmt.Errorf("展示事件缺少所属回合")
	}
	_, pathSet := eventPath(branch.Head, eventsByID(lines))
	if !pathSet[turnID] {
		return fmt.Errorf("展示事件回合不属于当前分支路径: %s", turnID)
	}
	updated := false
	for i := range lines {
		raw := lines[i].Raw
		if lines[i].Envelope.ID != turnID || lines[i].Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(raw, &turn); err != nil {
			return err
		}
		raw["display_events"] = appendDisplayEvent(turn.DisplayEvents, events[0])
		updated = true
		break
	}
	if !updated {
		return fmt.Errorf("展示事件所属回合不存在: %s", turnID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return err
	}
	return s.touchIndexLocked(storyID, now, 0)
}

func (s *Store) RewindToTurnParent(storyID string, req RewindTurnRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return err
	}
	defer releaseStory()

	turnID := strings.TrimSpace(req.TurnID)
	if turnID == "" {
		return fmt.Errorf("回合 ID 不能为空")
	}
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	events := eventsByID(lines)
	path, pathSet := eventPath(branch.Head, events)
	if !pathSet[turnID] {
		return fmt.Errorf("只能编辑当前剧情路径上的回合: %s", turnID)
	}
	var target *StoryEventRecord
	for i := range path {
		if path[i].Envelope.ID == turnID && path[i].Envelope.Type == StoryEventTypeTurn {
			target = &path[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("回合不存在: %s", turnID)
	}
	branch.Head = parentIDFromRaw(target.Raw)
	meta.Branches[branchID] = branch
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return err
	}
	return s.touchIndexLocked(storyID, now, 0)
}
