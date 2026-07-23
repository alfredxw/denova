package interactive

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func (s *Store) CreateBranch(storyID string, req CreateBranchRequest) (BranchSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return BranchSummary{}, err
	}
	defer releaseStory()

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
	if err := s.cloneDirectorPlanForBranchLocked(storyID, fromBranch, branchID, title); err != nil {
		return BranchSummary{}, err
	}
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		_ = os.RemoveAll(s.directorPlanBranchDir(storyID, branchID))
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
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return err
	}
	defer releaseStory()

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
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return err
	}
	defer releaseStory()

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
	_ = os.RemoveAll(s.directorPlanBranchDir(storyID, branchID))
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
