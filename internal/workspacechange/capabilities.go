package workspacechange

// liveHistoryCapabilities validates the ledger's history position against the
// current workspace head. The booleans are presentation hints only; Undo and
// Redo repeat the same revision checks before mutating files.
func (s *Service) liveHistoryCapabilities(group *ChangeGroup) (canUndo, canRedo bool) {
	if group == nil {
		return false, false
	}
	if s.undone[group.ID] {
		if s.redoInvalid[group.ID] {
			return false, false
		}
		return false, s.historyHeadMatches(group, false)
	}
	return s.historyHeadMatches(group, true), false
}

func (s *Service) historyHeadMatches(group *ChangeGroup, undo bool) bool {
	byPath := map[string][]*ChangeSet{}
	for _, change := range group.ChangeSets {
		if change.Origin == OriginUndo || change.Origin == OriginRedo {
			continue
		}
		if change.ApplyState == ApplyStateConflicted || change.ApplyState == ApplyStatePrepared {
			return false
		}
		copy := change
		byPath[change.Path] = append(byPath[change.Path], &copy)
	}
	if validateHistoryContinuity(group.ID, byPath) != nil {
		return false
	}
	hasNetChange := false
	for path, changes := range byPath {
		first := changes[0]
		last := changes[len(changes)-1]
		if first.BeforeExists == last.AfterExists && (!first.BeforeExists || first.BaseRevision == last.Revision) {
			continue
		}
		hasNetChange = true
		expectedExists := last.AfterExists
		expectedRevision := last.Revision
		if !undo {
			expectedExists = first.BeforeExists
			expectedRevision = first.BaseRevision
		}
		current, currentExists, err := s.readVisibleState(path)
		if err != nil || currentExists != expectedExists || stateRevision(current, currentExists) != expectedRevision {
			return false
		}
	}
	return hasNetChange
}
