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

	meta, lines, err := s.readStoryLocked(storyID)
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
	// Director plan documents and token-usage telemetry are mutable branch
	// sidecars, not events on the parent path. Attaching the latest sidecar here
	// would leak facts learned after the regenerated turn into its replacement
	// model call. Leave both unavailable in this historical projection; after
	// the replacement commits, Director maintenance rebuilds from the new head.
	snapshot.DirectorPlan = nil
	snapshot.DirectorPlanStatus = nil
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
