package interactive

import (
	"encoding/json"
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
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return StoryMeta{}, Snapshot{}, err
	}
	defer release()

	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return StoryMeta{}, Snapshot{}, err
	}
	if branchID == "" {
		branchID = handle.projection.Meta.CurrentBranch
	}
	limit = normalizeStoryHistoryPageLimit(limit)
	cacheKey := storySnapshotCacheKey{branchID: branchID, limit: limit}
	head := handle.journal.Head().Cursor
	if cached, ok := handle.snapshots[cacheKey]; ok && cached.cursor == head {
		meta, snapshot, cloneErr := cloneStorySnapshotCache(cached.meta, cached.snapshot)
		if cloneErr != nil {
			return StoryMeta{}, Snapshot{}, cloneErr
		}
		s.rememberStoryReplayStats(storyID, StoryJournalReplayStats{})
		return meta, snapshot, nil
	}

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
	snapshot.ContextRevision = loaded.projection.ContextRevision
	snapshot.State = cloneStoryState(loaded.projection.State)
	snapshot.BranchPlan = cloneBranchPlan(loaded.projection.Plan)
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
	snapshot.TurnCount = loaded.totalTurns
	snapshot.TurnStart = loaded.turnStart
	snapshot.HistoryBeforeCursor = loaded.page.BeforeCursor
	snapshot.HasEarlierTurns = loaded.page.HasMore
	if cachedMeta, cachedSnapshot, cloneErr := cloneStorySnapshotCache(loaded.meta, snapshot); cloneErr != nil {
		return StoryMeta{}, Snapshot{}, cloneErr
	} else {
		if handle.snapshots == nil {
			handle.snapshots = make(map[storySnapshotCacheKey]storySnapshotCache)
		}
		handle.snapshots[cacheKey] = storySnapshotCache{
			cursor: handle.journal.Head().Cursor, meta: cachedMeta, snapshot: cachedSnapshot,
		}
	}
	return loaded.meta, snapshot, nil
}

// cloneStorySnapshotCache isolates callers from the hot projection cache. The
// bounded core intentionally excludes Director-plan and token-usage overlays;
// Snapshot and StoryContext attach those independently after this cache lookup.
func cloneStorySnapshotCache(meta StoryMeta, snapshot Snapshot) (StoryMeta, Snapshot, error) {
	payload, err := json.Marshal(struct {
		Meta     StoryMeta `json:"meta"`
		Snapshot Snapshot  `json:"snapshot"`
	}{Meta: meta, Snapshot: snapshot})
	if err != nil {
		return StoryMeta{}, Snapshot{}, err
	}
	var cloned struct {
		Meta     StoryMeta `json:"meta"`
		Snapshot Snapshot  `json:"snapshot"`
	}
	if err := json.Unmarshal(payload, &cloned); err != nil {
		return StoryMeta{}, Snapshot{}, err
	}
	// Preserve the established API distinction between an initialized empty
	// pending collection and an absent collection across the JSON deep copy.
	if snapshot.PendingPlayerInputs != nil && cloned.Snapshot.PendingPlayerInputs == nil {
		cloned.Snapshot.PendingPlayerInputs = []PlayerInputAcceptedEvent{}
	}
	if snapshot.PendingModelContextBatches != nil && cloned.Snapshot.PendingModelContextBatches == nil {
		cloned.Snapshot.PendingModelContextBatches = []ModelContextBatchEvent{}
	}
	return cloned.Meta, cloned.Snapshot, nil
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
