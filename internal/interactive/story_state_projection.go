package interactive

import "fmt"

// TurnStateProjection reuses the native checkpoint used by branch creation.
// It is read-only and contains no private plans or model recovery messages.
type TurnStateProjection struct {
	BranchID       string
	SourceRevision string
	State          map[string]any
	Schema         *ActorStateSchemaSnapshot
}

// ReadStateAtTurn returns committed state at a reachable historical turn. The
// branch is checked before resolving the checkpoint so a future or sibling
// turn cannot accidentally supply state for the page currently being read.
func (s *Store) ReadStateAtTurn(storyID, branchID, turnID string) (TurnStateProjection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cursor := ""
	for {
		loaded, err := s.readStoryHistoryPageLocked(storyID, branchID, cursor, maxStoryHistoryPageTurns, true)
		if err != nil {
			return TurnStateProjection{}, err
		}
		branchID = loaded.page.BranchID
		for _, turn := range loaded.page.Turns {
			if turn.ID != turnID {
				continue
			}
			checkpoint, err := s.checkpointAtTurnLocked(storyID, turnID)
			if err != nil {
				return TurnStateProjection{}, err
			}
			state := checkpoint.State
			if err := applyFrozenMissingInitialActors(state, loaded.meta.ActorStateSchema); err != nil {
				return TurnStateProjection{}, err
			}
			applyLegacyActorStateAliases(state, loaded.meta.ActorStateSchema)
			return TurnStateProjection{BranchID: branchID, SourceRevision: TurnNarrativeRevision(turn), State: state, Schema: loaded.meta.ActorStateSchema}, nil
		}
		if !loaded.page.HasMore || loaded.page.BeforeCursor == "" {
			return TurnStateProjection{}, fmt.Errorf("Turn %q is not on branch %q", turnID, branchID)
		}
		cursor = loaded.page.BeforeCursor
	}
}
