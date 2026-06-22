package interactive

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const schemaVersion = 1
const maxStoryLineBytes = 16 * 1024 * 1024
const defaultFirstStoryTitle = "新的开始"

// DefaultStoryReplyTargetChars is the default target length for one interactive story turn.
const DefaultStoryReplyTargetChars = 2000
const maxStoryOpeningTextRunes = 4000

const (
	StoryOpeningModeAI     = "ai"
	StoryOpeningModePreset = "preset"
	StoryOpeningModeCustom = "custom"
)

// Store manages interactive story data inside a workspace.
type Store struct {
	root string
	mu   sync.Mutex
}

// NewStore creates an interactive store rooted at the workspace directory.
func NewStore(root string) *Store {
	return &Store{root: root}
}

// Root returns the workspace root.
func (s *Store) Root() string {
	return s.root
}

func (s *Store) Index() (Index, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readIndexLocked()
}

func (s *Store) CreateStory(req CreateStoryRequest) (StorySummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.storyDir(), 0o755); err != nil {
		return StorySummary{}, fmt.Errorf("创建互动故事目录失败: %w", err)
	}
	index, err := s.readIndexLocked()
	if err != nil {
		return StorySummary{}, err
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = defaultStoryTitle(index.Stories)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	story := StorySummary{
		ID:               newID("st"),
		Title:            title,
		Origin:           strings.TrimSpace(req.Origin),
		StoryTellerID:    strings.TrimSpace(req.StoryTellerID),
		ReplyTargetChars: normalizeStoryReplyTargetChars(req.ReplyTargetChars),
		Opening:          normalizeStoryOpeningConfig(req.Opening),
		CreatedAt:        now,
		UpdatedAt:        now,
		Branches:         1,
	}
	if story.StoryTellerID == "" {
		story.StoryTellerID = "classic"
	}

	meta := StoryMeta{
		V:                schemaVersion,
		Type:             StoryEventTypeMeta,
		StoryID:          story.ID,
		Title:            story.Title,
		Origin:           story.Origin,
		StoryTellerID:    story.StoryTellerID,
		ReplyTargetChars: story.ReplyTargetChars,
		Opening:          story.Opening,
		CurrentBranch:    "main",
		Branches: map[string]BranchMeta{
			"main": {CreatedAt: now},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := validateStoryMeta(meta); err != nil {
		return StorySummary{}, err
	}
	if err := writeJSONL(s.storyPath(story.ID), []any{meta}); err != nil {
		return StorySummary{}, err
	}

	index.CurrentStoryID = story.ID
	index.Stories = append(index.Stories, story)
	if err := s.writeIndexLocked(index); err != nil {
		return StorySummary{}, err
	}
	return story, nil
}

func (s *Store) UpdateStory(storyID string, req UpdateStoryRequest) (StorySummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StorySummary{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if title := strings.TrimSpace(req.Title); title != "" {
		meta.Title = title
	}
	if tellerID := strings.TrimSpace(req.StoryTellerID); tellerID != "" {
		meta.StoryTellerID = tellerID
	}
	if req.ReplyTargetChars != nil {
		if *req.ReplyTargetChars <= 0 {
			return StorySummary{}, fmt.Errorf("互动故事单轮目标字数必须大于 0")
		}
		meta.ReplyTargetChars = *req.ReplyTargetChars
	}
	if req.Opening != nil {
		meta.Opening = normalizeStoryOpeningConfig(*req.Opening)
	}
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return StorySummary{}, err
	}
	index, err := s.readIndexLocked()
	if err != nil {
		return StorySummary{}, err
	}
	for i := range index.Stories {
		if index.Stories[i].ID == storyID {
			index.Stories[i].Title = meta.Title
			index.Stories[i].StoryTellerID = meta.StoryTellerID
			index.Stories[i].ReplyTargetChars = meta.ReplyTargetChars
			index.Stories[i].Opening = meta.Opening
			index.Stories[i].UpdatedAt = now
			if err := s.writeIndexLocked(index); err != nil {
				return StorySummary{}, err
			}
			return index.Stories[i], nil
		}
	}
	return StorySummary{}, fmt.Errorf("故事不存在: %s", storyID)
}

func (s *Store) DeleteStory(storyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.readIndexLocked()
	if err != nil {
		return err
	}
	next := index.Stories[:0]
	removed := false
	for _, story := range index.Stories {
		if story.ID == storyID {
			removed = true
			continue
		}
		next = append(next, story)
	}
	if !removed {
		return fmt.Errorf("故事不存在: %s", storyID)
	}
	index.Stories = next
	if index.CurrentStoryID == storyID {
		index.CurrentStoryID = ""
		if len(index.Stories) > 0 {
			index.CurrentStoryID = index.Stories[0].ID
		}
	}
	if err := os.Remove(s.storyPath(storyID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(s.usagePath(storyID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return s.writeIndexLocked(index)
}

func (s *Store) StoryContext(storyID, branchID string) (StoryContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StoryContext{}, err
	}
	snapshot, err := snapshotFromLines(storyID, branchID, meta, lines)
	if err != nil {
		return StoryContext{}, err
	}
	usageEvents, err := s.readTokenUsageEventsLocked(storyID, snapshot.BranchID)
	if err != nil {
		return StoryContext{}, err
	}
	snapshot.TokenUsageEvents = usageEvents
	return StoryContext{Meta: meta, Snapshot: snapshot}, nil
}

func (s *Store) HotChoices(storyID, branchID string) (HotChoicesEvent, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return HotChoicesEvent{}, false, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return HotChoicesEvent{}, false, err
	}
	event, ok := latestHotChoicesForHead(lines, branchID, branch.Head)
	return event, ok, nil
}

func (s *Store) SaveHotChoices(storyID, branchID string, choices []string) (HotChoicesEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	choices = normalizeChoiceListLimit(choices, 10)
	if len(choices) == 0 {
		return HotChoicesEvent{}, fmt.Errorf("快捷选择不能为空")
	}
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return HotChoicesEvent{}, err
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return HotChoicesEvent{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := HotChoicesEvent{
		V:        schemaVersion,
		Type:     StoryEventTypeHotChoices,
		ID:       newID("hc"),
		ParentID: branch.Head,
		BranchID: branchID,
		Ts:       now,
		Choices:  choices,
	}
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines, event); err != nil {
		return HotChoicesEvent{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 1); err != nil {
		return HotChoicesEvent{}, err
	}
	return event, nil
}

func (s *Store) AppendContextCompaction(storyID, branchID string, event ContextCompactionEvent) (ContextCompactionEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return ContextCompactionEvent{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return ContextCompactionEvent{}, fmt.Errorf("分支不存在: %s", branchID)
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
	if err := s.rewriteStoryLocked(storyID, meta, lines, event); err != nil {
		return ContextCompactionEvent{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 1); err != nil {
		return ContextCompactionEvent{}, err
	}
	return event, nil
}

func (s *Store) AppendContextCompactionRemoval(storyID, branchID string, event ContextCompactionRemovalEvent) (ContextCompactionRemovalEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return ContextCompactionRemovalEvent{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return ContextCompactionRemovalEvent{}, fmt.Errorf("分支不存在: %s", branchID)
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
	if err := s.rewriteStoryLocked(storyID, meta, lines, event); err != nil {
		return ContextCompactionRemovalEvent{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 1); err != nil {
		return ContextCompactionRemovalEvent{}, err
	}
	return event, nil
}

func (s *Store) AppendTurn(storyID string, req AppendTurnRequest) (TurnEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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
	parentID := any(nil)
	if branch.Head != "" {
		parentID = branch.Head
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := TurnEvent{
		V:             schemaVersion,
		Type:          StoryEventTypeTurn,
		ID:            newID("ev"),
		ParentID:      parentID,
		BranchID:      branchID,
		Ts:            now,
		User:          req.User,
		Narrative:     req.Narrative,
		Thinking:      strings.TrimSpace(req.Thinking),
		DisplayEvents: sanitizeDisplayEvents(req.DisplayEvents),
		Flags:         map[string]bool{"pinned": false, "locked": false},
	}
	branch.Head = event.ID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines, event); err != nil {
		return TurnEvent{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 1); err != nil {
		return TurnEvent{}, err
	}
	return event, nil
}

func (s *Store) AppendTurnWithState(storyID string, req AppendTurnWithStateRequest) (TurnEvent, *StateDeltaEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return TurnEvent{}, nil, fmt.Errorf("分支不存在: %s", branchID)
	}
	parentID := any(nil)
	if branch.Head != "" {
		parentID = branch.Head
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	turn := TurnEvent{
		V:             schemaVersion,
		Type:          StoryEventTypeTurn,
		ID:            newID("ev"),
		ParentID:      parentID,
		BranchID:      branchID,
		Ts:            now,
		User:          req.User,
		Narrative:     req.Narrative,
		Thinking:      strings.TrimSpace(req.Thinking),
		DisplayEvents: sanitizeDisplayEvents(req.DisplayEvents),
		HotState:      normalizeHotState(req.HotState),
		MemoryStatus:  "pending",
		Flags:         map[string]bool{"pinned": false, "locked": false},
	}
	branch.Head = turn.ID

	var delta *StateDeltaEvent
	if len(req.Ops) > 0 {
		stateDelta := newStateDelta(req.Ops)
		turn.StateDelta = &stateDelta
		turn.StateStatus = "ready"
		stateDeltaEvent := newStateDeltaEvent(turn.ID, parentIDString(parentID), branchID, now, req.Ops)
		delta = &stateDeltaEvent
	} else {
		turn.StateStatus = "pending"
	}

	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines, turn); err != nil {
		return TurnEvent{}, nil, err
	}
	if err := s.touchIndexLocked(storyID, now, 1); err != nil {
		return TurnEvent{}, nil, err
	}
	return turn, delta, nil
}

// AppendTurnDisplayEvent appends a display-only event to an existing turn.
// The event is kept out of future model context and does not move branch head.
func (s *Store) AppendTurnDisplayEvent(storyID, branchID, turnID string, event DisplayEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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
	if _, ok := meta.Branches[branchID]; !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return fmt.Errorf("展示事件缺少所属回合")
	}
	updated := false
	for i := range lines {
		raw := lines[i].Raw
		if lines[i].Envelope.ID != turnID || lines[i].Envelope.Type != StoryEventTypeTurn {
			continue
		}
		if lines[i].Envelope.BranchID != branchID {
			return fmt.Errorf("展示事件回合不属于当前分支: %s", turnID)
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

func (s *Store) SwitchTurnVersion(storyID string, req SwitchTurnVersionRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	turnID := strings.TrimSpace(req.TurnID)
	versionTurnID := strings.TrimSpace(req.VersionTurnID)
	if turnID == "" || versionTurnID == "" {
		return fmt.Errorf("回合版本参数不能为空")
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
		return fmt.Errorf("只能切换当前剧情路径上的回合版本: %s", turnID)
	}
	currentIndex := -1
	var current *StoryEventRecord
	for i := range path {
		if path[i].Envelope.ID == turnID && path[i].Envelope.Type == StoryEventTypeTurn {
			current = &path[i]
			currentIndex = i
			break
		}
	}
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

	nextHead := versionTurnID
	if currentIndex >= 0 && currentIndex < len(path)-1 {
		next := path[currentIndex+1]
		if err := reparentStoryEvent(lines, next, turnID, versionTurnID); err != nil {
			return err
		}
		nextHead = branch.Head
	}
	branch.Head = nextHead
	meta.Branches[branchID] = branch
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return err
	}
	return s.touchIndexLocked(storyID, now, 0)
}

func reparentStoryEvent(lines []StoryEventRecord, child StoryEventRecord, oldParentID, newParentID string) error {
	if parentIDFromRaw(child.Raw) != oldParentID {
		return fmt.Errorf("当前剧情路径不连续，无法切换版本: %s", child.Envelope.ID)
	}
	for i := range lines {
		if lines[i].Envelope.ID != child.Envelope.ID || lines[i].Envelope.Type != child.Envelope.Type {
			continue
		}
		if parentIDFromRaw(lines[i].Raw) != oldParentID {
			continue
		}
		lines[i].Raw["parent_id"] = newParentID
		lines[i].Envelope.ParentID = newParentID
		return nil
	}
	return fmt.Errorf("剧情后续节点不存在，无法切换版本: %s", child.Envelope.ID)
}

func (s *Store) AppendStateDelta(storyID string, req AppendStateDeltaRequest) (StateDeltaEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(req.Ops) == 0 {
		return StateDeltaEvent{}, fmt.Errorf("状态变化不能为空")
	}

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StateDeltaEvent{}, err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return StateDeltaEvent{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	parentID := strings.TrimSpace(req.ParentID)
	if parentID == "" {
		parentID = branch.Head
	}
	if parentID == "" {
		return StateDeltaEvent{}, fmt.Errorf("状态变化缺少所属回合")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := newStateDeltaEvent(parentID, parentID, branchID, now, req.Ops)
	updated := false
	for i := range lines {
		raw := lines[i].Raw
		if lines[i].Envelope.ID != parentID || lines[i].Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(raw, &turn); err != nil {
			return StateDeltaEvent{}, err
		}
		ops := append([]StateOp(nil), req.Ops...)
		if turn.StateDelta != nil && len(turn.StateDelta.Ops) > 0 {
			ops = append(append([]StateOp(nil), turn.StateDelta.Ops...), req.Ops...)
		}
		raw["state_delta"] = newStateDelta(ops)
		raw["state_status"] = "ready"
		delete(raw, "state_error")
		updated = true
		break
	}
	if !updated {
		return StateDeltaEvent{}, fmt.Errorf("状态变化所属回合不存在: %s", parentID)
	}
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return StateDeltaEvent{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 0); err != nil {
		return StateDeltaEvent{}, err
	}
	return event, nil
}

func (s *Store) MarkStateFailed(storyID string, req MarkStateFailedRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	if _, ok := meta.Branches[branchID]; !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	parentID := strings.TrimSpace(req.ParentID)
	if parentID == "" {
		return fmt.Errorf("状态失败标记缺少所属回合")
	}
	errText := strings.TrimSpace(req.Error)
	if errText == "" {
		errText = "状态生成失败"
	}
	updated := false
	for _, record := range lines {
		raw := record.Raw
		if record.Envelope.ID != parentID || record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		raw["state_status"] = "failed"
		raw["state_error"] = errText
		raw["memory_status"] = "failed"
		raw["memory_error"] = errText
		updated = true
		break
	}
	if !updated {
		return fmt.Errorf("状态失败标记所属回合不存在: %s", parentID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return err
	}
	return s.touchIndexLocked(storyID, now, 0)
}

func (s *Store) CreateBranch(storyID string, req CreateBranchRequest) (BranchSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return BranchSummary{}, err
	}
	parentID := strings.TrimSpace(req.ParentEventID)
	if parentID == "" {
		return BranchSummary{}, fmt.Errorf("父事件不能为空")
	}
	fromBranch, ok := findEventBranch(lines, parentID)
	if !ok {
		return BranchSummary{}, fmt.Errorf("父事件不存在: %s", parentID)
	}
	branchID := "br_" + strings.TrimPrefix(newID(""), "_")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "新分支"
	}
	meta.CurrentBranch = branchID
	meta.Branches[branchID] = BranchMeta{
		Head:      parentID,
		CreatedAt: now,
		From:      fromBranch,
		FromEvent: parentID,
		Title:     title,
	}
	meta.UpdatedAt = now
	event := BranchEvent{
		V:        schemaVersion,
		Type:     StoryEventTypeBranch,
		ID:       newID("ev"),
		ParentID: parentID,
		BranchID: branchID,
		From:     fromBranch,
		Ts:       now,
		Title:    title,
	}
	if err := s.rewriteStoryLocked(storyID, meta, lines, event); err != nil {
		return BranchSummary{}, err
	}
	if err := s.updateIndexBranchesLocked(storyID, len(meta.Branches), now, 1); err != nil {
		return BranchSummary{}, err
	}
	return BranchSummary{ID: branchID, Head: parentID, From: fromBranch, FromEvent: parentID, Title: title, CreatedAt: now, Current: true}, nil
}

func (s *Store) SwitchBranch(storyID, branchID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	if _, ok := meta.Branches[branchID]; !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	meta.CurrentBranch = branchID
	meta.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return s.rewriteStoryLocked(storyID, meta, lines)
}

func (s *Store) DeleteBranch(storyID, branchID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	branchID = strings.TrimSpace(branchID)
	if branchID == "" {
		return fmt.Errorf("分支不能为空")
	}
	if branchID == "main" {
		return fmt.Errorf("主线不能删除")
	}
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	if branch.Head != branch.FromEvent {
		return fmt.Errorf("只能删除尚未产生独立剧情的空分支")
	}
	for id, candidate := range meta.Branches {
		if id != branchID && candidate.From == branchID {
			return fmt.Errorf("该分支已有子分支，不能删除")
		}
	}
	nextLines := make([]StoryEventRecord, 0, len(lines))
	removedEvents := 0
	for _, record := range lines {
		if record.Envelope.Type == StoryEventTypeBranch && record.Envelope.BranchID == branchID {
			removedEvents++
			continue
		}
		nextLines = append(nextLines, record)
	}
	if removedEvents == 0 {
		return fmt.Errorf("分支记录不存在: %s", branchID)
	}
	delete(meta.Branches, branchID)
	if meta.CurrentBranch == branchID {
		if branch.From != "" {
			meta.CurrentBranch = branch.From
		} else {
			meta.CurrentBranch = "main"
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, nextLines); err != nil {
		return err
	}
	return s.updateIndexBranchesLocked(storyID, len(meta.Branches), now, -removedEvents)
}

func (s *Store) Branches(storyID string) ([]BranchSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return nil, err
	}
	return branchSummaries(meta), nil
}

func (s *Store) Snapshot(storyID, branchID string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot, err := snapshotFromLines(storyID, branchID, meta, lines)
	if err != nil {
		return Snapshot{}, err
	}
	usageEvents, err := s.readTokenUsageEventsLocked(storyID, snapshot.BranchID)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.TokenUsageEvents = usageEvents
	return snapshot, nil
}

func findEventBranch(lines []StoryEventRecord, eventID string) (string, bool) {
	for _, record := range lines {
		if record.Envelope.ID != eventID {
			continue
		}
		return record.Envelope.BranchID, record.Envelope.BranchID != ""
	}
	return "", false
}

func defaultStoryTitle(stories []StorySummary) string {
	if len(stories) == 0 {
		return defaultFirstStoryTitle
	}
	next := len(stories) + 1
	for _, story := range stories {
		title := strings.TrimSpace(story.Title)
		if !strings.HasPrefix(title, "故事线") {
			continue
		}
		rawNumber := strings.TrimSpace(strings.TrimPrefix(title, "故事线"))
		if rawNumber == "" {
			continue
		}
		number, err := strconv.Atoi(rawNumber)
		if err == nil && number >= next {
			next = number + 1
		}
	}
	if next < 2 {
		next = 2
	}
	return fmt.Sprintf("故事线 %d", next)
}

func normalizeStoryReplyTargetChars(value int) int {
	if value <= 0 {
		return DefaultStoryReplyTargetChars
	}
	return value
}

func normalizeStorySummary(story StorySummary) StorySummary {
	story.ReplyTargetChars = normalizeStoryReplyTargetChars(story.ReplyTargetChars)
	story.Opening = normalizeStoryOpeningConfig(story.Opening)
	return story
}

func normalizeStoryMeta(meta StoryMeta) StoryMeta {
	meta.ReplyTargetChars = normalizeStoryReplyTargetChars(meta.ReplyTargetChars)
	meta.Opening = normalizeStoryOpeningConfig(meta.Opening)
	return meta
}

func normalizeStoryOpeningConfig(config StoryOpeningConfig) StoryOpeningConfig {
	mode := strings.TrimSpace(config.Mode)
	switch mode {
	case StoryOpeningModePreset, StoryOpeningModeCustom:
	default:
		mode = StoryOpeningModeAI
	}
	normalized := StoryOpeningConfig{
		Mode:       mode,
		PresetID:   strings.TrimSpace(config.PresetID),
		PresetText: truncateStoryOpeningText(config.PresetText),
		CustomText: truncateStoryOpeningText(config.CustomText),
	}
	if mode != StoryOpeningModePreset {
		normalized.PresetID = ""
		normalized.PresetText = ""
	}
	if mode != StoryOpeningModeCustom {
		normalized.CustomText = ""
	}
	return normalized
}

func truncateStoryOpeningText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxStoryOpeningTextRunes {
		return text
	}
	return string(runes[:maxStoryOpeningTextRunes])
}

func newID(prefix string) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + strconv.FormatInt(time.Now().UnixNano(), 36) + hex.EncodeToString(b[:])
}
