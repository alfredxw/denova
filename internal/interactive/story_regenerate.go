package interactive

import (
	"errors"
	"fmt"
	"strings"
)

var ErrStoryContextRevisionConflict = errors.New("interactive story context revision conflict")

// StoryContextAtTurnParent builds a read-only model projection for
// regeneration. Canonical branch metadata and head are never modified.
func (s *Store) StoryContextAtTurnParent(storyID, branchID, turnID string) (StoryContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return StoryContext{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return StoryContext{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	if err := requireLatestLogicalTurn(meta, lines, branchID, strings.TrimSpace(turnID)); err != nil {
		return StoryContext{}, err
	}
	parentID, err := regenerationParentOnCurrentPath(lines, branch.Head, strings.TrimSpace(turnID))
	if err != nil {
		return StoryContext{}, err
	}
	branch.Head = parentID
	meta.Branches[branchID] = branch
	snapshot, err := snapshotFromLines(storyID, branchID, meta, lines)
	if err != nil {
		return StoryContext{}, err
	}
	if projection, projectionErr := s.storyBranchProjectionLocked(storyID, branchID); projectionErr == nil {
		snapshot.State = cloneStoryState(projection.StateBeforeLatest)
		if projection.Depth > 0 {
			snapshot.TurnCount = projection.Depth - 1
		}
		snapshot.BranchPlan = cloneBranchPlan(projection.PlanBeforeLatest)
	}
	// This historical parent is a distinct canonical model-history revision.
	// Reusing the live branch's monotonically accumulated counter would let
	// Canonical replay must not mistake different regenerate content for an exact
	// retry. The projected prefix depth is stable for the same target and always
	// advances once the replacement turn becomes the live branch.
	snapshot.ContextRevision = uint64(snapshot.TurnCount)
	// Token-usage telemetry is mutable runtime data and is unavailable in this
	// historical projection. BranchPlan is restored from its event checkpoint
	// above so regeneration cannot see intent learned after the replaced turn.
	snapshot.TokenUsageEvents = nil
	return StoryContext{Meta: meta, Snapshot: snapshot}, nil
}

// regenerationParentOnCurrentPath validates that target is still a turn on
// the current branch path and returns its parent revision.
func regenerationParentOnCurrentPath(lines []StoryEventRecord, currentHead, turnID string) (string, error) {
	if turnID == "" {
		return "", fmt.Errorf("回合 ID 不能为空")
	}
	path, pathSet := eventPath(currentHead, eventsByID(lines))
	if !pathSet[turnID] {
		return "", fmt.Errorf("%w: regenerate target %s is no longer on the current branch path", ErrStoryContextRevisionConflict, turnID)
	}
	for i := range path {
		if path[i].Envelope.ID == turnID && path[i].Envelope.Type == StoryEventTypeTurn {
			return parentIDFromRaw(path[i].Raw), nil
		}
	}
	return "", fmt.Errorf("回合不存在: %s", turnID)
}
