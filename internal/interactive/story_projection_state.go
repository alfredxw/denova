package interactive

import "strings"

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
	if plan, planErr := s.readDirectorPlanLocked(storyID, snapshot.BranchID); planErr == nil {
		snapshot.DirectorPlan = &plan
		status := DirectorPlanStatusFromPlan(plan, len(snapshot.Turns) > 0)
		snapshot.DirectorPlanStatus = &status
	}
	usageEvents, err := s.readTokenUsageEventsLocked(storyID, snapshot.BranchID)
	if err != nil {
		return StoryContext{}, err
	}
	snapshot.TokenUsageEvents = usageEvents
	return StoryContext{Meta: meta, Snapshot: snapshot}, nil
}

func (s *Store) Snapshot(storyID, branchID string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot, err := snapshotFromLines(storyID, branchID, meta, lines)
	if err != nil {
		return Snapshot{}, err
	}
	if plan, planErr := s.readDirectorPlanLocked(storyID, snapshot.BranchID); planErr == nil {
		snapshot.DirectorPlan = &plan
		status := DirectorPlanStatusFromPlan(plan, len(snapshot.Turns) > 0)
		snapshot.DirectorPlanStatus = &status
	}
	usageEvents, err := s.readTokenUsageEventsLocked(storyID, snapshot.BranchID)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.TokenUsageEvents = usageEvents
	return snapshot, nil
}

func findEventBranch(lines []StoryEventRecord, eventID string) (string, bool) {
	for _, record := range lines {
		if record.Envelope.ID != eventID {
			continue
		}
		return record.Envelope.BranchID, record.Envelope.BranchID != ""
	}
	return "", false
}

func branchIsTerminal(lines []StoryEventRecord, head string) bool {
	turn := latestTurnForBranchHead(lines, head)
	return turn != nil && turn.TerminalOutcome != nil && turn.TerminalOutcome.Terminal
}

func stateFromPath(path []StoryEventRecord) map[string]any {
	state := initialStoryState()
	for _, record := range path {
		switch record.Envelope.Type {
		case StoryEventTypeStateDelta:
			var delta StateDeltaEvent
			if err := mapToStruct(record.Raw, &delta); err == nil {
				for _, op := range delta.Ops {
					applyStateOp(state, op)
				}
				for _, op := range delta.ActorOps {
					applyActorStateOp(state, op)
				}
			}
		case StoryEventTypeTurn:
			var turn TurnEvent
			if err := mapToStruct(record.Raw, &turn); err == nil && turn.StateDelta != nil {
				for _, op := range turn.StateDelta.Ops {
					applyStateOp(state, op)
				}
				for _, op := range turn.StateDelta.ActorOps {
					applyActorStateOp(state, op)
				}
			}
		}
	}
	return state
}

func stateBeforeTurn(path []StoryEventRecord, turnID string) map[string]any {
	state := initialStoryState()
	for _, record := range path {
		if record.Envelope.ID == turnID {
			break
		}
		switch record.Envelope.Type {
		case StoryEventTypeStateDelta:
			var delta StateDeltaEvent
			if err := mapToStruct(record.Raw, &delta); err == nil {
				for _, op := range delta.Ops {
					applyStateOp(state, op)
				}
				for _, op := range delta.ActorOps {
					applyActorStateOp(state, op)
				}
			}
		case StoryEventTypeTurn:
			var turn TurnEvent
			if err := mapToStruct(record.Raw, &turn); err == nil && turn.StateDelta != nil {
				for _, op := range turn.StateDelta.Ops {
					applyStateOp(state, op)
				}
				for _, op := range turn.StateDelta.ActorOps {
					applyActorStateOp(state, op)
				}
			}
		}
	}
	return state
}

func (s *Store) storyDirectorForMeta(meta StoryMeta) StoryDirector {
	if strings.TrimSpace(s.novaDir) == "" {
		return DefaultStoryDirector()
	}
	directorID := normalizedStoryDirectorID(meta.StoryDirectorID)
	director, err := NewStoryDirectorLibrary(s.novaDir).Get(directorID)
	if err == nil {
		return director
	}
	fallback, fallbackErr := NewStoryDirectorLibrary(s.novaDir).Get(DefaultStoryDirectorID)
	if fallbackErr == nil {
		return fallback
	}
	return DefaultStoryDirector()
}

func terminalOutcomeFromRuleResolution(resolution RuleResolution, turnID, narrative string) *TerminalOutcome {
	if resolution.TerminalCandidate == nil {
		return nil
	}
	candidate := resolution.TerminalCandidate
	return normalizeTerminalOutcomePointer(&TerminalOutcome{
		Terminal:              true,
		Type:                  firstNonEmptyString(candidate.Type, "bad_end"),
		Reason:                candidate.Reason,
		FinalNarrativeSummary: trimBytes(narrative, maxInteractiveTextBytes),
		CausedByTurnID:        turnID,
		RuleResolutionID:      resolution.ID,
	})
}
