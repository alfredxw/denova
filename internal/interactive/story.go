package interactive

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const schemaVersion = 1
const maxStoryLineBytes = 16 * 1024 * 1024

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

type CreateStoryRequest struct {
	Title         string `json:"title"`
	Origin        string `json:"origin"`
	StoryTellerID string `json:"story_teller_id"`
}

type AppendTurnRequest struct {
	BranchID  string `json:"branch_id"`
	User      string `json:"user"`
	Narrative string `json:"narrative"`
	Thinking  string `json:"thinking,omitempty"`
}

type AppendTurnWithStateRequest struct {
	BranchID  string    `json:"branch_id"`
	User      string    `json:"user"`
	Narrative string    `json:"narrative"`
	Thinking  string    `json:"thinking,omitempty"`
	Ops       []StateOp `json:"ops,omitempty"`
}

type AppendStateDeltaRequest struct {
	ParentID string    `json:"parent_id"`
	BranchID string    `json:"branch_id"`
	Ops      []StateOp `json:"ops"`
}

type UpdateStoryRequest struct {
	Title         string `json:"title"`
	StoryTellerID string `json:"story_teller_id"`
}

type CreateBranchRequest struct {
	ParentEventID string `json:"parent_event_id"`
	Title         string `json:"title"`
}

type Index struct {
	CurrentStoryID string         `json:"current_story_id"`
	Stories        []StorySummary `json:"stories"`
}

type StorySummary struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Origin        string `json:"origin"`
	StoryTellerID string `json:"story_teller_id"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	Branches      int    `json:"branches"`
	Events        int    `json:"events"`
}

type BranchMeta struct {
	Head      string `json:"head"`
	CreatedAt string `json:"created_at"`
	From      string `json:"from,omitempty"`
	FromEvent string `json:"from_event,omitempty"`
	Title     string `json:"title,omitempty"`
}

type BranchSummary struct {
	ID        string `json:"id"`
	Head      string `json:"head"`
	From      string `json:"from,omitempty"`
	FromEvent string `json:"from_event,omitempty"`
	Title     string `json:"title,omitempty"`
	CreatedAt string `json:"created_at"`
	Current   bool   `json:"current"`
}

type StoryMeta struct {
	V             int                   `json:"v"`
	Type          string                `json:"type"`
	StoryID       string                `json:"story_id"`
	Title         string                `json:"title"`
	Origin        string                `json:"origin"`
	StoryTellerID string                `json:"story_teller_id"`
	CurrentBranch string                `json:"current_branch"`
	Branches      map[string]BranchMeta `json:"branches"`
	CreatedAt     string                `json:"created_at"`
	UpdatedAt     string                `json:"updated_at"`
}

type TurnEvent struct {
	V          int             `json:"v"`
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	ParentID   any             `json:"parent_id"`
	BranchID   string          `json:"branch_id"`
	Ts         string          `json:"ts"`
	User       string          `json:"user"`
	Narrative  string          `json:"narrative"`
	Thinking   string          `json:"thinking,omitempty"`
	StateDelta *StateDelta     `json:"state_delta,omitempty"`
	Alts       []TurnAlt       `json:"alts,omitempty"`
	AltIdx     int             `json:"alt_idx,omitempty"`
	Flags      map[string]bool `json:"flags,omitempty"`
}

type TurnAlt struct {
	Narrative string `json:"narrative"`
	Ts        string `json:"ts"`
}

type StateDelta struct {
	Ops []StateOp `json:"ops"`
}

type StateDeltaEvent struct {
	V        int       `json:"v"`
	Type     string    `json:"type"`
	ID       string    `json:"id"`
	ParentID string    `json:"parent_id"`
	BranchID string    `json:"branch_id"`
	Ts       string    `json:"ts"`
	Ops      []StateOp `json:"ops"`
}

type BranchEvent struct {
	V        int    `json:"v"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	BranchID string `json:"branch_id"`
	From     string `json:"from"`
	Ts       string `json:"ts"`
	Title    string `json:"title"`
}

type StateOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

type Snapshot struct {
	StoryID     string         `json:"story_id"`
	BranchID    string         `json:"branch_id"`
	Turns       []TurnEvent    `json:"turns"`
	CurrentTurn *TurnEvent     `json:"current_turn,omitempty"`
	State       map[string]any `json:"state"`
	Graph       StoryGraph     `json:"graph"`
}

type StoryGraph struct {
	Nodes    []PlotNode      `json:"nodes"`
	Branches []BranchSummary `json:"branches"`
}

type PlotNode struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
	BranchID string `json:"branch_id"`
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	Ts       string `json:"ts"`
	Current  bool   `json:"current"`
	Head     bool   `json:"head"`
}

type StoryContext struct {
	Meta     StoryMeta `json:"meta"`
	Snapshot Snapshot  `json:"snapshot"`
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
	now := time.Now().UTC().Format(time.RFC3339Nano)
	story := StorySummary{
		ID:            newID("st"),
		Title:         strings.TrimSpace(req.Title),
		Origin:        strings.TrimSpace(req.Origin),
		StoryTellerID: strings.TrimSpace(req.StoryTellerID),
		CreatedAt:     now,
		UpdatedAt:     now,
		Branches:      1,
	}
	if story.Title == "" {
		return StorySummary{}, fmt.Errorf("故事标题不能为空")
	}
	if story.StoryTellerID == "" {
		story.StoryTellerID = "classic"
	}

	meta := StoryMeta{
		V:             schemaVersion,
		Type:          "meta",
		StoryID:       story.ID,
		Title:         story.Title,
		Origin:        story.Origin,
		StoryTellerID: story.StoryTellerID,
		CurrentBranch: "main",
		Branches: map[string]BranchMeta{
			"main": {CreatedAt: now},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := writeJSONL(s.storyPath(story.ID), []any{meta}); err != nil {
		return StorySummary{}, err
	}

	index, err := s.readIndexLocked()
	if err != nil {
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
	return StoryContext{Meta: meta, Snapshot: snapshot}, nil
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
		V:         schemaVersion,
		Type:      "turn",
		ID:        newID("ev"),
		ParentID:  parentID,
		BranchID:  branchID,
		Ts:        now,
		User:      req.User,
		Narrative: req.Narrative,
		Thinking:  strings.TrimSpace(req.Thinking),
		Flags:     map[string]bool{"pinned": false, "locked": false},
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
		V:         schemaVersion,
		Type:      "turn",
		ID:        newID("ev"),
		ParentID:  parentID,
		BranchID:  branchID,
		Ts:        now,
		User:      req.User,
		Narrative: req.Narrative,
		Thinking:  strings.TrimSpace(req.Thinking),
		Flags:     map[string]bool{"pinned": false, "locked": false},
	}
	branch.Head = turn.ID

	var delta *StateDeltaEvent
	if len(req.Ops) > 0 {
		turn.StateDelta = &StateDelta{Ops: req.Ops}
		stateDelta := StateDeltaEvent{
			V:        schemaVersion,
			Type:     "state_delta",
			ID:       turn.ID,
			ParentID: parentIDString(parentID),
			BranchID: branchID,
			Ts:       now,
			Ops:      req.Ops,
		}
		delta = &stateDelta
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

func (s *Store) AppendStateDelta(storyID string, req AppendStateDeltaRequest) (StateDeltaEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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
	event := StateDeltaEvent{
		V:        schemaVersion,
		Type:     "state_delta",
		ID:       parentID,
		ParentID: parentID,
		BranchID: branchID,
		Ts:       now,
		Ops:      req.Ops,
	}
	updated := false
	for _, raw := range lines {
		id, _ := raw["id"].(string)
		eventType, _ := raw["type"].(string)
		if id != parentID || eventType != "turn" {
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
		raw["state_delta"] = StateDelta{Ops: ops}
		updated = true
		break
	}
	if !updated {
		return StateDeltaEvent{}, fmt.Errorf("状态变化所属回合不存在: %s", parentID)
	}
	branch.Head = parentID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return StateDeltaEvent{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 0); err != nil {
		return StateDeltaEvent{}, err
	}
	return event, nil
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
		Type:     "branch",
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
	nextLines := make([]map[string]any, 0, len(lines))
	removedEvents := 0
	for _, raw := range lines {
		eventType, _ := raw["type"].(string)
		rawBranchID, _ := raw["branch_id"].(string)
		if eventType == "branch" && rawBranchID == branchID {
			removedEvents++
			continue
		}
		nextLines = append(nextLines, raw)
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
	return snapshotFromLines(storyID, branchID, meta, lines)
}

func snapshotFromLines(storyID, branchID string, meta StoryMeta, lines []map[string]any) (Snapshot, error) {
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return Snapshot{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	state := initialStoryState()
	snapshot := Snapshot{StoryID: storyID, BranchID: branchID, State: state}
	eventsByID := eventsByID(lines)
	path, pathSet := eventPath(branch.Head, eventsByID)
	for _, raw := range path {
		eventType, _ := raw["type"].(string)
		switch eventType {
		case "turn":
			var turn TurnEvent
			if err := mapToStruct(raw, &turn); err != nil {
				return Snapshot{}, err
			}
			snapshot.Turns = append(snapshot.Turns, turn)
			currentTurn := turn
			snapshot.CurrentTurn = &currentTurn
			if turn.StateDelta != nil {
				for _, op := range turn.StateDelta.Ops {
					applyStateOp(state, op)
				}
			}
		case "state_delta":
			var delta StateDeltaEvent
			if err := mapToStruct(raw, &delta); err != nil {
				return Snapshot{}, err
			}
			for _, op := range delta.Ops {
				applyStateOp(state, op)
			}
		}
	}
	snapshot.Graph = buildStoryGraph(meta, lines, eventsByID, pathSet)
	return snapshot, nil
}

func initialStoryState() map[string]any {
	return map[string]any{
		"on_stage":     []any{},
		"characters":   map[string]any{},
		"events":       []any{},
		"scene":        map[string]any{},
		"inventory":    map[string]any{},
		"resources":    map[string]any{},
		"world_flags":  []any{},
		"rules":        []any{},
		"threads":      []any{},
		"action_space": []any{},
	}
}

func eventsByID(lines []map[string]any) map[string]map[string]any {
	events := make(map[string]map[string]any, len(lines))
	for _, raw := range lines {
		id, _ := raw["id"].(string)
		if id != "" {
			events[id] = raw
		}
	}
	return events
}

func eventPath(head string, events map[string]map[string]any) ([]map[string]any, map[string]bool) {
	reversed := make([]map[string]any, 0)
	inPath := map[string]bool{}
	for id := head; id != ""; {
		raw, ok := events[id]
		if !ok || inPath[id] {
			break
		}
		reversed = append(reversed, raw)
		inPath[id] = true
		id = parentIDFromRaw(raw)
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed, inPath
}

func buildStoryGraph(meta StoryMeta, lines []map[string]any, events map[string]map[string]any, currentPath map[string]bool) StoryGraph {
	headTurns := map[string]bool{}
	for _, branch := range meta.Branches {
		if headTurn := nearestTurnAncestor(branch.Head, events); headTurn != "" {
			headTurns[headTurn] = true
		}
	}
	nodes := make([]PlotNode, 0)
	for _, raw := range lines {
		if eventType, _ := raw["type"].(string); eventType != "turn" {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(raw, &turn); err != nil {
			continue
		}
		parentID := parentIDFromRaw(raw)
		if parentID != "" {
			parentID = nearestTurnAncestor(parentID, events)
		}
		nodes = append(nodes, PlotNode{
			ID:       turn.ID,
			ParentID: parentID,
			BranchID: turn.BranchID,
			Title:    compactText(turn.User, 24),
			Summary:  compactText(turn.Narrative, 72),
			Ts:       turn.Ts,
			Current:  currentPath[turn.ID],
			Head:     headTurns[turn.ID],
		})
	}
	return StoryGraph{Nodes: nodes, Branches: branchSummaries(meta)}
}

func nearestTurnAncestor(head string, events map[string]map[string]any) string {
	for id := head; id != ""; {
		raw, ok := events[id]
		if !ok {
			return ""
		}
		if eventType, _ := raw["type"].(string); eventType == "turn" {
			return id
		}
		id = parentIDFromRaw(raw)
	}
	return ""
}

func branchSummaries(meta StoryMeta) []BranchSummary {
	result := make([]BranchSummary, 0, len(meta.Branches))
	for id, branch := range meta.Branches {
		result = append(result, BranchSummary{
			ID:        id,
			Head:      branch.Head,
			From:      branch.From,
			FromEvent: branch.FromEvent,
			Title:     branch.Title,
			CreatedAt: branch.CreatedAt,
			Current:   id == meta.CurrentBranch,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID == "main" {
			return true
		}
		if result[j].ID == "main" {
			return false
		}
		return result[i].CreatedAt < result[j].CreatedAt
	})
	return result
}

func parentIDFromRaw(raw map[string]any) string {
	switch value := raw["parent_id"].(type) {
	case string:
		return value
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func parentIDString(parentID any) string {
	switch value := parentID.(type) {
	case string:
		return value
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func compactText(text string, limit int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" {
		return "未命名节点"
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

func (s *Store) storyDir() string {
	return filepath.Join(s.root, "interactive", "story")
}

func (s *Store) indexPath() string {
	return filepath.Join(s.storyDir(), "index.json")
}

func (s *Store) storyPath(storyID string) string {
	return filepath.Join(s.storyDir(), "story-"+storyID+".jsonl")
}

func (s *Store) readIndexLocked() (Index, error) {
	data, err := os.ReadFile(s.indexPath())
	if os.IsNotExist(err) {
		return Index{Stories: []StorySummary{}}, nil
	}
	if err != nil {
		return Index{}, err
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return Index{}, fmt.Errorf("解析互动故事索引失败: %w", err)
	}
	return index, nil
}

func (s *Store) writeIndexLocked(index Index) error {
	if err := os.MkdirAll(s.storyDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath(), data, 0o644)
}

func (s *Store) touchIndexLocked(storyID, updatedAt string, eventDelta int) error {
	index, err := s.readIndexLocked()
	if err != nil {
		return err
	}
	for i := range index.Stories {
		if index.Stories[i].ID == storyID {
			index.Stories[i].UpdatedAt = updatedAt
			index.Stories[i].Events += eventDelta
			return s.writeIndexLocked(index)
		}
	}
	return fmt.Errorf("故事不存在: %s", storyID)
}

func (s *Store) updateIndexBranchesLocked(storyID string, branches int, updatedAt string, eventDelta int) error {
	index, err := s.readIndexLocked()
	if err != nil {
		return err
	}
	for i := range index.Stories {
		if index.Stories[i].ID == storyID {
			index.Stories[i].Branches = branches
			index.Stories[i].UpdatedAt = updatedAt
			index.Stories[i].Events += eventDelta
			return s.writeIndexLocked(index)
		}
	}
	return fmt.Errorf("故事不存在: %s", storyID)
}

func findEventBranch(lines []map[string]any, eventID string) (string, bool) {
	for _, raw := range lines {
		id, _ := raw["id"].(string)
		if id != eventID {
			continue
		}
		branchID, _ := raw["branch_id"].(string)
		return branchID, branchID != ""
	}
	return "", false
}

func (s *Store) readStoryLocked(storyID string) (StoryMeta, []map[string]any, error) {
	file, err := os.Open(s.storyPath(storyID))
	if err != nil {
		return StoryMeta{}, nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxStoryLineBytes)
	if !scanner.Scan() {
		return StoryMeta{}, nil, fmt.Errorf("故事文件为空: %s", storyID)
	}
	var meta StoryMeta
	if err := json.Unmarshal(scanner.Bytes(), &meta); err != nil {
		return StoryMeta{}, nil, fmt.Errorf("解析故事元信息失败: %w", err)
	}
	var lines []map[string]any
	for scanner.Scan() {
		var raw map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			return StoryMeta{}, nil, fmt.Errorf("解析故事事件失败: %w", err)
		}
		lines = append(lines, raw)
	}
	if err := scanner.Err(); err != nil {
		return StoryMeta{}, nil, err
	}
	return meta, lines, nil
}

func (s *Store) rewriteStoryLocked(storyID string, meta StoryMeta, events []map[string]any, newEvents ...any) error {
	lines := make([]any, 0, len(events)+len(newEvents)+1)
	lines = append(lines, meta)
	for _, event := range events {
		lines = append(lines, event)
	}
	lines = append(lines, newEvents...)
	return writeJSONL(s.storyPath(storyID), lines)
}

func writeJSONL(path string, lines []any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(file)
	enc.SetEscapeHTML(false)
	for _, line := range lines {
		if err := enc.Encode(line); err != nil {
			_ = file.Close()
			return err
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func mapToStruct(raw map[string]any, out any) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func applyStateOp(state map[string]any, op StateOp) {
	switch op.Op {
	case "set":
		setPath(state, op.Path, op.Value)
	case "merge":
		current, _ := getPath(state, op.Path).(map[string]any)
		if current == nil {
			current = map[string]any{}
		}
		if value, ok := op.Value.(map[string]any); ok {
			for k, v := range value {
				current[k] = v
			}
		}
		setPath(state, op.Path, current)
	case "push":
		current, _ := getPath(state, op.Path).([]any)
		setPath(state, op.Path, append(current, op.Value))
	case "pull":
		current, _ := getPath(state, op.Path).([]any)
		next := current[:0]
		for _, item := range current {
			if fmt.Sprint(item) != fmt.Sprint(op.Value) {
				next = append(next, item)
			}
		}
		setPath(state, op.Path, next)
	case "inc":
		current, _ := getPath(state, op.Path).(float64)
		by := 1.0
		if value, ok := op.Value.(float64); ok {
			by = value
		}
		setPath(state, op.Path, current+by)
	case "unset":
		unsetPath(state, op.Path)
	}
}

func getPath(root map[string]any, path string) any {
	parts := strings.Split(path, ".")
	var current any = root
	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = obj[part]
	}
	return current
}

func setPath(root map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	current := root
	for _, part := range parts[:len(parts)-1] {
		next, _ := current[part].(map[string]any)
		if next == nil {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = value
}

func unsetPath(root map[string]any, path string) {
	parts := strings.Split(path, ".")
	current := root
	for _, part := range parts[:len(parts)-1] {
		next, _ := current[part].(map[string]any)
		if next == nil {
			return
		}
		current = next
	}
	delete(current, parts[len(parts)-1])
}

func newID(prefix string) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + strconv.FormatInt(time.Now().UnixNano(), 36) + hex.EncodeToString(b[:])
}
