package interactive

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"
)

// CanonicalHistoryHead identifies an append-only branch lane without loading
// historical turns. Epoch changes are rebuilt from creator edits/replacements.
func (s *Store) CanonicalHistoryHead(ctx context.Context, storyID, branchID string) (agent.CanonicalHistoryHead, error) {
	if err := ctx.Err(); err != nil {
		return agent.CanonicalHistoryHead{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return agent.CanonicalHistoryHead{}, err
	}
	defer release()
	handle, err := s.refreshStoryJournalLocked(storyID, false)
	if err != nil {
		return agent.CanonicalHistoryHead{}, err
	}
	if branchID == "" {
		branchID = handle.projection.Meta.CurrentBranch
	}
	branch, ok := handle.projection.Branches[branchID]
	if !ok {
		return agent.CanonicalHistoryHead{}, fmt.Errorf("story branch %q is unavailable", branchID)
	}
	revision := branch.HistoryRevision
	if revision == "" {
		revision = "empty"
	}
	return agent.CanonicalHistoryHead{
		Identity: fmt.Sprintf("%s/%s/%s/%s", handle.projection.StoryID, handle.projection.Generation, branchID, branch.HistoryEpoch),
		Revision: revision,
	}, nil
}
