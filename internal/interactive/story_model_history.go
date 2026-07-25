package interactive

import (
	"fmt"
	"strings"
)

// StoryModelHistoryQuery selects an exact logical turn interval on the active
// branch path. StartTurn is zero-based and EndTurn is exclusive. Callers must
// derive this interval from their own context budget; UI paging policy is not
// part of this interface.
type StoryModelHistoryQuery struct {
	BranchID  string
	StartTurn int
	EndTurn   int
}

// StoryModelTurn is the model-visible projection of one canonical turn. It
// deliberately excludes thinking, display events, state snapshots, versions,
// and other UI-only or runtime-only fields.
type StoryModelTurn struct {
	ID                   string
	BranchID             string
	User                 string
	Narrative            string
	ModelContextMessages []ModelContextMessage
}

// StoryModelHistory is an exact, ordered projection of Query's logical range.
// TotalTurns describes the current canonical branch and makes stale range
// assumptions observable to callers.
type StoryModelHistory struct {
	StoryID    string
	BranchID   string
	StartTurn  int
	EndTurn    int
	TotalTurns int
	Turns      []StoryModelTurn
}

// ReadModelHistory reads model-visible turns independently from the bounded UI
// snapshot. The implementation pages backward through the journal, so its hot
// cost is proportional to the requested active context tail rather than the
// complete story length.
func (s *Store) ReadModelHistory(storyID string, query StoryModelHistoryQuery) (StoryModelHistory, error) {
	if s == nil {
		return StoryModelHistory{}, fmt.Errorf("interactive store is nil")
	}
	storyID = strings.TrimSpace(storyID)
	if storyID == "" {
		return StoryModelHistory{}, fmt.Errorf("故事 ID 不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return StoryModelHistory{}, err
	}
	branchID := strings.TrimSpace(query.BranchID)
	if branchID == "" {
		branchID = handle.projection.Meta.CurrentBranch
	}
	branch := handle.projection.Branches[branchID]
	if branch == nil {
		return StoryModelHistory{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	totalTurns := branch.Depth
	if query.StartTurn < 0 || query.EndTurn < query.StartTurn || query.EndTurn > totalTurns {
		return StoryModelHistory{}, fmt.Errorf(
			"模型历史范围无效: start=%d end=%d total=%d",
			query.StartTurn, query.EndTurn, totalTurns,
		)
	}
	result := StoryModelHistory{
		StoryID: storyID, BranchID: branchID,
		StartTurn: query.StartTurn, EndTurn: query.EndTurn, TotalTurns: totalTurns,
	}
	if query.StartTurn == query.EndTurn {
		return result, nil
	}

	expectedHead := branch.Head
	beforeCursor := ""
	turnPagesNewestFirst := make([][]TurnEvent, 0, (totalTurns-query.StartTurn+maxStoryHistoryPageTurns-1)/maxStoryHistoryPageTurns)
	loadedCount := 0
	loadedStart := totalTurns
	for loadedStart > query.StartTurn {
		page, pageErr := s.readStoryHistoryPageLocked(storyID, branchID, beforeCursor, maxStoryHistoryPageTurns, true)
		if pageErr != nil {
			return StoryModelHistory{}, pageErr
		}
		if len(page.page.Turns) == 0 {
			return StoryModelHistory{}, fmt.Errorf(
				"模型历史范围不完整: start=%d loaded_start=%d total=%d",
				query.StartTurn, loadedStart, totalTurns,
			)
		}
		turnPagesNewestFirst = append(turnPagesNewestFirst, page.page.Turns)
		loadedCount += len(page.page.Turns)
		loadedStart = totalTurns - loadedCount
		if loadedStart <= query.StartTurn {
			break
		}
		if !page.page.HasMore || strings.TrimSpace(page.page.BeforeCursor) == "" {
			return StoryModelHistory{}, fmt.Errorf(
				"模型历史父链不完整: start=%d loaded_start=%d total=%d",
				query.StartTurn, loadedStart, totalTurns,
			)
		}
		beforeCursor = page.page.BeforeCursor
	}
	loadedTurns := make([]TurnEvent, 0, loadedCount)
	for index := len(turnPagesNewestFirst) - 1; index >= 0; index-- {
		loadedTurns = append(loadedTurns, turnPagesNewestFirst[index]...)
	}

	// Each page refreshes the shared handle. Reject an externally advanced path
	// instead of combining two branch revisions into one model request.
	currentHandle, err := s.openStoryJournalLocked(storyID)
	if err != nil {
		return StoryModelHistory{}, err
	}
	currentBranch := currentHandle.projection.Branches[branchID]
	if currentBranch == nil || currentBranch.Head != expectedHead || currentBranch.Depth != totalTurns {
		return StoryModelHistory{}, fmt.Errorf(
			"%w: model history changed while reading branch %s",
			ErrStoryContextRevisionConflict, branchID,
		)
	}
	from := query.StartTurn - loadedStart
	through := query.EndTurn - loadedStart
	if from < 0 || through < from || through > len(loadedTurns) {
		return StoryModelHistory{}, fmt.Errorf(
			"模型历史投影范围不完整: loaded_start=%d from=%d through=%d loaded=%d",
			loadedStart, from, through, len(loadedTurns),
		)
	}
	result.Turns = make([]StoryModelTurn, 0, through-from)
	for _, turn := range loadedTurns[from:through] {
		result.Turns = append(result.Turns, StoryModelTurn{
			ID: turn.ID, BranchID: turn.BranchID, User: turn.User, Narrative: turn.Narrative,
			ModelContextMessages: sanitizeModelContextMessages(turn.ModelContextMessages),
		})
	}
	return result, nil
}
