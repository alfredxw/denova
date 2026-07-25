package interactive

import (
	"fmt"
	"strings"
)

func (s *Store) StoryContext(storyID, branchID string) (StoryContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, snapshot, err := s.boundedStorySnapshotLocked(storyID, branchID)
	if err != nil {
		return StoryContext{}, err
	}
	if plan, planErr := s.readDirectorPlanLocked(storyID, snapshot.BranchID); planErr == nil {
		snapshot.DirectorPlan = &plan
		status := DirectorPlanStatusFromPlan(plan, snapshot.TurnCount > 0)
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

	_, snapshot, err := s.boundedStorySnapshotLocked(storyID, branchID)
	if err != nil {
		return Snapshot{}, err
	}
	if plan, planErr := s.readDirectorPlanLocked(storyID, snapshot.BranchID); planErr == nil {
		snapshot.DirectorPlan = &plan
		status := DirectorPlanStatusFromPlan(plan, snapshot.TurnCount > 0)
		snapshot.DirectorPlanStatus = &status
	}
	usageEvents, err := s.readTokenUsageEventsLocked(storyID, snapshot.BranchID)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.TokenUsageEvents = usageEvents
	return snapshot, nil
}

func (s *Store) boundedStorySnapshotLocked(storyID, branchID string) (StoryMeta, Snapshot, error) {
	return s.boundedStorySnapshotWithLimitLocked(storyID, branchID, defaultStoryHistoryPageTurns)
}

// boundedStorySnapshotWithLimitLocked is the shared hot-path projection. UI
// snapshots use the default page, while Director reconciliation may retain its
// full bounded decision horizon without scanning the canonical prefix.
func (s *Store) boundedStorySnapshotWithLimitLocked(storyID, branchID string, limit int) (StoryMeta, Snapshot, error) {
	loaded, err := s.readStoryHistoryPageLocked(storyID, branchID, "", limit, true)
	if err != nil {
		return StoryMeta{}, Snapshot{}, err
	}
	snapshot := loaded.snapshot
	if err := s.freezeLegacyActorStateSchemaFromStateLocked(storyID, &loaded.meta, loaded.projection.State); err != nil {
		return StoryMeta{}, Snapshot{}, err
	}
	if handle := s.storyJournals[strings.TrimSpace(storyID)]; handle != nil {
		handle.projection.Meta.ActorStateSchema = loaded.meta.ActorStateSchema
		handle.projection.Meta.StateSchemaInitialization = loaded.meta.StateSchemaInitialization
		if err := cacheStoryRecentLoaded(handle, snapshot.BranchID, loaded.meta, loaded.records); err != nil {
			return StoryMeta{}, Snapshot{}, err
		}
	}
	snapshot.ActorStateSchema = loaded.meta.ActorStateSchema
	snapshot.StateSchemaInitialization = loaded.meta.StateSchemaInitialization
	snapshot.State = cloneStoryState(loaded.projection.State)
	initializeActors := true
	if storyStateSchemaPolicyRequiresOpeningDraft(loaded.meta.StateSchemaPolicy) && loaded.meta.StateSchemaInitialization != nil && loaded.meta.StateSchemaInitialization.Status == StateSchemaInitializationWaitingOpening {
		initializeActors = false
	}
	if initializeActors {
		if err := applyFrozenMissingInitialActors(snapshot.State, loaded.meta.ActorStateSchema); err != nil {
			return StoryMeta{}, Snapshot{}, fmt.Errorf("补全冻结初始 Actor 失败: %w", err)
		}
	}
	applyLegacyActorStateAliases(snapshot.State, loaded.meta.ActorStateSchema)
	if loaded.projection.Compaction != nil {
		compaction := *loaded.projection.Compaction
		snapshot.ContextCompaction = &compaction
	} else {
		snapshot.ContextCompaction = nil
	}
	if loaded.projection.CompactionRemoval != nil {
		removal := *loaded.projection.CompactionRemoval
		snapshot.ContextCompactionRemoval = &removal
	} else {
		snapshot.ContextCompactionRemoval = nil
	}
	snapshot.TurnCount = loaded.totalTurns
	snapshot.TurnStart = loaded.turnStart
	snapshot.HistoryBeforeCursor = loaded.page.BeforeCursor
	snapshot.HasEarlierTurns = loaded.page.HasMore
	return loaded.meta, snapshot, nil
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
