package interactive

import (
	"fmt"
	"strings"
	"time"
)

// UpdateBranchPlan replaces the existing future blueprint without creating a
// story Turn or changing state, choices, images, or committed narrative. The
// revision becomes a canonical branch-head event because it changes the model
// context seen by the next Game Agent run.
func (s *Store) UpdateBranchPlan(storyID string, req UpdateBranchPlanRequest) (UpdateBranchPlanResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return UpdateBranchPlanResult{}, err
	}
	defer releaseStory()

	markdown := normalizeBranchPlanMarkdown(req.Markdown)
	if err := validateEditableBranchPlanMarkdown(markdown); err != nil {
		return UpdateBranchPlanResult{}, fmt.Errorf("规划文档必须非空、大小合规，并至少保留一个唯一的二级标题 / Branch plan must be non-empty, within limits, and keep at least one unique H2 section: %w", err)
	}

	meta, _, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return UpdateBranchPlanResult{}, err
	}
	branchID := strings.TrimSpace(req.BranchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return UpdateBranchPlanResult{}, fmt.Errorf("分支不存在 / Branch does not exist: %s", branchID)
	}

	projection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return UpdateBranchPlanResult{}, err
	}
	if projection.LatestTurnID == "" || projection.Plan == nil {
		return UpdateBranchPlanResult{}, fmt.Errorf("当前分支还没有可编辑的规划 / The current branch does not have an editable plan yet")
	}
	current := *projection.Plan
	if strings.TrimSpace(req.BaseRevision) != current.Revision {
		return UpdateBranchPlanResult{}, fmt.Errorf("分支规划已在其他位置更新，请重新加载后再保存 / %w", ErrBranchPlanRevisionConflict)
	}
	if current.Markdown == markdown {
		return UpdateBranchPlanResult{BranchPlan: current, ContextRevision: projection.ContextRevision}, nil
	}

	previousContextRevision := projection.ContextRevision
	now := time.Now().UTC().Format(time.RFC3339Nano)
	eventID := newID("bpr")
	event := BranchPlanUpdatedEvent{
		V: schemaVersion, Type: StoryEventTypeBranchPlanRevised, ID: eventID,
		ParentID: branch.Head, BranchID: branchID, Ts: now,
		TurnID: projection.LatestTurnID, Markdown: markdown,
	}
	branch.Head = eventID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return UpdateBranchPlanResult{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return UpdateBranchPlanResult{}, err
	}
	return UpdateBranchPlanResult{
		BranchPlan: BranchPlan{
			Markdown: markdown, UpdatedTurnID: projection.LatestTurnID,
			UpdatedAt: now, Revision: eventID,
		},
		ContextRevision: previousContextRevision + 1,
	}, nil
}
