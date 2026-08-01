package interactive

import (
	"fmt"
	"log"
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

	meta, _, err := s.boundedStorySnapshotWithLimitLocked(storyID, "", 1)
	if err != nil {
		return BranchSummary{}, err
	}
	parentID := strings.TrimSpace(req.ParentEventID)
	if parentID == "" {
		return BranchSummary{}, fmt.Errorf("父事件不能为空")
	}
	checkpoint, err := s.checkpointAtTurnLocked(storyID, parentID)
	if err != nil {
		return BranchSummary{}, err
	}
	fromBranch := checkpoint.SourceBranchID
	branchID := "br_" + strings.TrimPrefix(newID(""), "_")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "新分支"
	}
	previousBranch := meta.CurrentBranch
	meta.CurrentBranch = branchID
	meta.Branches[branchID] = BranchMeta{
		Head:      parentID,
		CreatedAt: now,
		From:      fromBranch,
		FromEvent: parentID,
		Title:     title,
	}
	if source := meta.Branches[fromBranch]; source.RuntimeConfig != nil {
		branch := meta.Branches[branchID]
		value := *source.RuntimeConfig
		branch.RuntimeConfig = &value
		branch.RuntimeConfigRevision = 1
		meta.Branches[branchID] = branch
	}
	meta.UpdatedAt = now
	event := BranchEvent{
		V: schemaVersion, Type: StoryEventTypeBranch, ID: newID("ev"), ParentID: parentID,
		BranchID: branchID, From: fromBranch, Ts: now, Title: title,
		StateCheckpoint: cloneStoryState(checkpoint.State), LatestTurnID: checkpoint.LatestTurnID, Depth: checkpoint.Depth,
	}
	switched := BranchSwitchedEvent{
		V: schemaVersion, Type: StoryEventTypeBranchSwitched, ID: newID("bsw"),
		ParentID: parentID, BranchID: branchID, Ts: now,
		FromBranch: previousBranch, ToBranch: branchID,
	}
	if err := s.cloneDirectorPlanForBranchLocked(storyID, fromBranch, branchID, title); err != nil {
		return BranchSummary{}, err
	}
	if err := s.appendStoryTransactionLocked(storyID, meta, event, switched); err != nil {
		_ = os.RemoveAll(s.directorPlanBranchDir(storyID, branchID))
		return BranchSummary{}, err
	}
	if err := s.updateIndexBranchesLocked(storyID, len(meta.Branches), now, 2); err != nil {
		return BranchSummary{}, err
	}
	if closeErr := s.evictStoryJournalLocked(storyID); closeErr != nil {
		log.Printf("[interactive-story] flush index on branch creation failed story_id=%s branch_id=%s error=%v", storyID, branchID, closeErr)
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

	meta, _, err := s.boundedStorySnapshotWithLimitLocked(storyID, branchID, 1)
	if err != nil {
		return err
	}
	if _, ok := meta.Branches[branchID]; !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	fromBranch := meta.CurrentBranch
	meta.CurrentBranch = branchID
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	event := BranchSwitchedEvent{
		V: schemaVersion, Type: StoryEventTypeBranchSwitched, ID: newID("bsw"),
		ParentID: meta.Branches[branchID].Head, BranchID: branchID, Ts: now,
		FromBranch: fromBranch, ToBranch: branchID,
	}
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return err
	}
	if err := s.touchIndexLocked(storyID, now, 1); err != nil {
		return err
	}
	if closeErr := s.evictStoryJournalLocked(storyID); closeErr != nil {
		log.Printf("[interactive-story] flush index on branch switch failed story_id=%s branch_id=%s error=%v", storyID, branchID, closeErr)
	}
	return nil
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
	meta, _, err := s.boundedStorySnapshotWithLimitLocked(storyID, branchID, 1)
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
	previousCurrent := meta.CurrentBranch
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
	event := BranchArchivedEvent{
		V: schemaVersion, Type: StoryEventTypeBranchArchived, ID: newID("bar"),
		ParentID: branch.Head, BranchID: branchID, Ts: now,
		PreviousCurrentBranch: previousCurrent, NextCurrentBranch: meta.CurrentBranch,
	}
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return err
	}
	if err := s.updateIndexBranchesLocked(storyID, len(meta.Branches), now, 1); err != nil {
		return err
	}
	// Archive and index publication own the user-visible transaction. Artifacts
	// are collected only after branch references are durably unreachable; a GC
	// failure is retryable maintenance and cannot roll the archive back.
	if err := s.removeBranchToolArtifacts(storyID, branchID); err != nil {
		log.Printf("[interactive-story] remove archived branch tool artifacts failed story_id=%s branch_id=%s error=%v", storyID, branchID, err)
	}
	return nil
}

func (s *Store) Branches(storyID string) ([]BranchSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, _, err := s.boundedStorySnapshotWithLimitLocked(storyID, "", 1)
	if err != nil {
		return nil, err
	}
	return branchSummaries(meta), nil
}
