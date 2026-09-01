package interactive

import (
	"fmt"
	"sort"
	"strings"
)

func snapshotFromLines(storyID, branchID string, meta StoryMeta, lines []StoryEventRecord) (Snapshot, error) {
	projectedLines, err := projectStoryEventOverlays(lines)
	if err != nil {
		return Snapshot{}, err
	}
	lines = projectedLines
	providerContinuations, err := providerContinuationsByTurn(lines)
	if err != nil {
		return Snapshot{}, err
	}
	modelContextContinuations, err := modelContextProviderContinuationsByOwner(lines)
	if err != nil {
		return Snapshot{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return Snapshot{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	state := initialStoryState()
	snapshot := Snapshot{
		StoryID: storyID, BranchID: branchID, Turns: make([]TurnEvent, 0), State: state,
		ActorStateSchema: meta.ActorStateSchema, StateSchemaInitialization: meta.StateSchemaInitialization,
	}
	eventsByID := eventsByID(lines)
	path, pathSet := eventPath(branch.Head, eventsByID)
	turnVersions := buildTurnVersionIndex(lines)
	for _, record := range path {
		switch record.Envelope.Type {
		case StoryEventTypeTurn:
			var turn TurnEvent
			if err := mapToStruct(record.Raw, &turn); err != nil {
				return Snapshot{}, err
			}
			turn.DisplayEvents = sanitizeDisplayEvents(turn.DisplayEvents)
			turn.ModelContextMessages, err = hydrateModelContextProviderContinuations(
				turn.ModelContextMessages, turn.ID, modelContextContinuations,
			)
			if err != nil {
				return Snapshot{}, err
			}
			turn.ProviderContinuation = cloneProviderContinuation(providerContinuations[turn.ID])
			turn.ResolvedPlayerInputContexts, err = normalizeResolvedPlayerInputContexts(
				turn.ResolvedPlayerInputContexts, turn.BranchID, turn.PlayerInputID, turn.ConsumedPlayerInputIDs,
			)
			if err != nil {
				return Snapshot{}, err
			}
			versions := turnVersions[turnVersionKey(turn.BranchID, parentIDFromRaw(record.Raw))]
			if len(versions) > 1 {
				turn.Versions = versions
				for index, version := range versions {
					if version.TurnID == turn.ID {
						turn.VersionIdx = index
						turn.Versions[index].Current = true
						break
					}
				}
			}
			snapshot.Turns = append(snapshot.Turns, turn)
			currentTurn := turn
			snapshot.CurrentTurn = &currentTurn
			if turn.StateDelta != nil {
				for _, op := range turn.StateDelta.Ops {
					applyStateOp(state, op)
				}
				for _, op := range turn.StateDelta.ActorOps {
					applyActorStateOp(state, op)
				}
			}
		case StoryEventTypeStateDelta:
			var delta StateDeltaEvent
			if err := mapToStruct(record.Raw, &delta); err != nil {
				return Snapshot{}, err
			}
			for _, op := range delta.Ops {
				applyStateOp(state, op)
			}
			for _, op := range delta.ActorOps {
				applyActorStateOp(state, op)
			}
		case StoryEventTypePlayerInput, StoryEventTypeTurnInterrupted, StoryEventTypeModelContextBatch, StoryEventTypeModelContextProviderContinuation, StoryEventTypeProviderContinuation, StoryEventTypeBranch, StoryEventTypeHotChoices, StoryEventTypeTurnVersionSelected, StoryEventTypeBranchPlanUpdated, StoryEventTypeBranchPlanRevised:
			// These records are projected separately or are intentionally absent
			// from model-visible turn/state history.
		}
	}
	initializeActors := true
	if storyStateSchemaPolicyRequiresOpeningDraft(meta.StateSchemaPolicy) && meta.StateSchemaInitialization != nil && meta.StateSchemaInitialization.Status == StateSchemaInitializationWaitingOpening {
		initializeActors = false
	}
	if initializeActors {
		if err := applyFrozenMissingInitialActors(state, meta.ActorStateSchema); err != nil {
			return Snapshot{}, fmt.Errorf("补全冻结初始 Actor 失败: %w", err)
		}
	}
	applyLegacyActorStateAliases(state, meta.ActorStateSchema)
	if snapshot.CurrentTurn != nil && (snapshot.CurrentTurn.TurnResult == nil || len(snapshot.CurrentTurn.TurnResult.Choices) == 0) && snapshot.CurrentTurn.HotState == nil {
		if legacy, ok := latestHotChoicesForHead(lines, branchID, snapshot.CurrentTurn.ID); ok {
			hotState := normalizeHotState(&HotState{Choices: legacy.Choices})
			snapshot.CurrentTurn.HotState = hotState
			if len(snapshot.Turns) > 0 {
				snapshot.Turns[len(snapshot.Turns)-1].HotState = hotState
			}
		}
	}
	snapshot.Graph = buildStoryGraph(meta, lines, eventsByID, pathSet)
	pendingInputs, err := pendingPlayerInputsForBranch(lines, branchID, pathSet)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.PendingPlayerInputs = pendingInputs
	pendingBatches, err := pendingModelContextBatchesForBranch(
		lines, branchID, pathSet, pendingInputs, modelContextContinuations,
	)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.PendingModelContextBatches = pendingBatches
	snapshot.TurnCount = len(snapshot.Turns)
	return snapshot, nil
}

func pendingModelContextBatchesForBranch(
	lines []StoryEventRecord,
	branchID string,
	activeAncestry map[string]bool,
	pendingInputs []PlayerInputAcceptedEvent,
	continuations map[string]map[int]map[string]any,
) ([]ModelContextBatchEvent, error) {
	inputOrder := make(map[string]int, len(pendingInputs))
	inputs := make(map[string]PlayerInputAcceptedEvent, len(pendingInputs))
	for index, input := range pendingInputs {
		inputOrder[input.ID] = index
		inputs[input.ID] = input
	}
	result := make([]ModelContextBatchEvent, 0)
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeModelContextBatch || record.Envelope.BranchID != branchID {
			continue
		}
		var event ModelContextBatchEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return nil, err
		}
		normalized, err := normalizeModelContextBatchEvent(event)
		if err != nil {
			return nil, err
		}
		normalized.Messages, err = hydrateModelContextProviderContinuations(normalized.Messages, normalized.ID, continuations)
		if err != nil {
			return nil, err
		}
		input, pending := inputs[normalized.PlayerInputID]
		if !pending {
			continue
		}
		if normalized.AgentCommandID != input.AgentCommandID || normalized.AgentOperationID != input.AgentOperationID ||
			normalized.AgentCycle != input.AgentCycle {
			return nil, fmt.Errorf("%w: pending batch does not match accepted player input", ErrModelContextBatchIdentityConflict)
		}
		if parentID := strings.TrimSpace(normalized.ParentID); parentID != "" && !activeAncestry[parentID] {
			continue
		}
		result = append(result, normalized)
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, right := inputOrder[result[i].PlayerInputID], inputOrder[result[j].PlayerInputID]
		if left != right {
			return left < right
		}
		return result[i].BatchOrdinal < result[j].BatchOrdinal
	})
	lastOrdinal := make(map[string]int, len(pendingInputs))
	for _, event := range result {
		expected := lastOrdinal[event.PlayerInputID]
		if event.BatchOrdinal != expected {
			return nil, fmt.Errorf("%w: player input %s has a missing or duplicate pending batch ordinal", ErrModelContextBatchIdentityConflict, event.PlayerInputID)
		}
		lastOrdinal[event.PlayerInputID] = expected + 1
	}
	return result, nil
}

func pendingPlayerInputsForBranch(lines []StoryEventRecord, branchID string, activeAncestry map[string]bool) ([]PlayerInputAcceptedEvent, error) {
	consumed := make(map[string]bool)
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			return nil, err
		}
		if strings.TrimSpace(turn.PlayerInputID) != "" {
			consumed[turn.PlayerInputID] = true
		}
		for _, playerInputID := range turn.ConsumedPlayerInputIDs {
			if playerInputID = strings.TrimSpace(playerInputID); playerInputID != "" {
				consumed[playerInputID] = true
			}
		}
	}
	pending := make([]PlayerInputAcceptedEvent, 0)
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypePlayerInput || record.Envelope.BranchID != branchID || consumed[record.Envelope.ID] {
			continue
		}
		var input PlayerInputAcceptedEvent
		if err := mapToStruct(record.Raw, &input); err != nil {
			return nil, err
		}
		// Accepted input is an audit side event and does not advance branch.Head.
		// Only an input attached to the current ancestry can participate in a
		// future model call; rewound or version-replaced futures remain durable
		// but are deliberately absent from this model-facing projection.
		if parentID := strings.TrimSpace(input.ParentID); parentID != "" && !activeAncestry[parentID] {
			continue
		}
		pending = append(pending, input)
	}
	return pending, nil
}

// pendingPlayerInputsFromProjection intersects a bounded recent-page result
// with the journal reducer's complete pending-ID checkpoint. The page retains
// the rich input payload; the reducer supplies the authoritative lifecycle.
func pendingPlayerInputsFromProjection(inputs []PlayerInputAcceptedEvent, pendingPlayerInputIDs []string) []PlayerInputAcceptedEvent {
	if len(inputs) == 0 || len(pendingPlayerInputIDs) == 0 {
		return []PlayerInputAcceptedEvent{}
	}
	pending := make(map[string]bool, len(pendingPlayerInputIDs))
	for _, playerInputID := range pendingPlayerInputIDs {
		if playerInputID = strings.TrimSpace(playerInputID); playerInputID != "" {
			pending[playerInputID] = true
		}
	}
	result := make([]PlayerInputAcceptedEvent, 0, len(inputs))
	for _, input := range inputs {
		if pending[input.ID] {
			result = append(result, input)
		}
	}
	return result
}

func buildTurnVersionIndex(lines []StoryEventRecord) map[string][]TurnVersion {
	result := map[string][]TurnVersion{}
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		id := record.Envelope.ID
		branchID := record.Envelope.BranchID
		ts := record.Envelope.Ts
		if id == "" || branchID == "" {
			continue
		}
		key := turnVersionKey(branchID, parentIDFromRaw(record.Raw))
		result[key] = append(result[key], TurnVersion{TurnID: id, Ts: ts})
	}
	for key := range result {
		sort.Slice(result[key], func(i, j int) bool {
			return result[key][i].Ts < result[key][j].Ts
		})
	}
	return result
}

func turnVersionKey(branchID, parentID string) string {
	return branchID + "\x00" + parentID
}

func initialStoryState() map[string]any {
	return map[string]any{
		"on_stage":       []any{},
		"actors":         map[string]any{},
		"actor_archives": map[string]any{},
		"characters":     map[string]any{},
		"events":         []any{},
		"scene":          map[string]any{},
		"inventory":      map[string]any{},
		"resources":      map[string]any{},
		"world_flags":    []any{},
		"rules":          []any{},
		"threads":        []any{},
	}
}

func normalizeHotState(hot *HotState) *HotState {
	if hot == nil {
		return nil
	}
	choices := normalizeChoiceListLimit(hot.Choices, 5)
	if len(choices) == 0 {
		return nil
	}
	return &HotState{Choices: choices}
}

func normalizeChoiceListLimit(input []string, limit int) []string {
	if limit <= 0 {
		limit = 5
	}
	choices := make([]string, 0, len(input))
	seen := map[string]bool{}
	for _, choice := range input {
		choice = strings.TrimSpace(choice)
		key := normalizedChoiceKey(choice)
		if key == "" || seen[key] {
			continue
		}
		choices = append(choices, choice)
		seen[key] = true
		if len(choices) >= limit {
			break
		}
	}
	return choices
}

func resolveBranch(meta StoryMeta, branchID string) (string, BranchMeta, error) {
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return "", BranchMeta{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	return branchID, branch, nil
}

func latestHotChoicesForHead(lines []StoryEventRecord, branchID, parentID string) (HotChoicesEvent, bool) {
	var latest HotChoicesEvent
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeHotChoices {
			continue
		}
		if record.Envelope.BranchID != branchID {
			continue
		}
		if parentIDFromRaw(record.Raw) != parentID {
			continue
		}
		var event HotChoicesEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			continue
		}
		event.Choices = normalizeChoiceListLimit(event.Choices, 10)
		if len(event.Choices) == 0 {
			continue
		}
		if latest.ID == "" || event.Ts >= latest.Ts {
			latest = event
		}
	}
	return latest, latest.ID != ""
}

func eventsByID(lines []StoryEventRecord) map[string]StoryEventRecord {
	events := make(map[string]StoryEventRecord, len(lines))
	for _, record := range lines {
		if record.Envelope.ID != "" {
			events[record.Envelope.ID] = record
		}
	}
	return events
}

func eventPath(head string, events map[string]StoryEventRecord) ([]StoryEventRecord, map[string]bool) {
	reversed := make([]StoryEventRecord, 0)
	inPath := map[string]bool{}
	for id := head; id != ""; {
		record, ok := events[id]
		if !ok || inPath[id] {
			break
		}
		reversed = append(reversed, record)
		inPath[id] = true
		id = parentIDFromRaw(record.Raw)
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed, inPath
}

func buildStoryGraph(meta StoryMeta, lines []StoryEventRecord, events map[string]StoryEventRecord, currentPath map[string]bool) StoryGraph {
	headTurns := map[string]bool{}
	for _, branch := range meta.Branches {
		if headTurn := nearestTurnAncestor(branch.Head, events); headTurn != "" {
			headTurns[headTurn] = true
		}
	}
	nodes := make([]PlotNode, 0)
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			continue
		}
		parentID := parentIDFromRaw(record.Raw)
		if parentID != "" {
			parentID = nearestTurnAncestor(parentID, events)
		}
		terminal := turn.TerminalOutcome != nil && turn.TerminalOutcome.Terminal
		terminalType := ""
		if turn.TerminalOutcome != nil {
			terminalType = turn.TerminalOutcome.Type
		}
		title := turn.User
		if turn.UserContextOnly {
			title = turn.Narrative
		}
		nodes = append(nodes, PlotNode{
			ID:           turn.ID,
			ParentID:     parentID,
			BranchID:     turn.BranchID,
			Title:        compactText(title, 24),
			Summary:      compactText(turn.Narrative, 72),
			Ts:           turn.Ts,
			Current:      currentPath[turn.ID],
			Head:         headTurns[turn.ID],
			Terminal:     terminal,
			TerminalType: terminalType,
		})
	}
	return StoryGraph{Nodes: nodes, Branches: branchSummaries(meta)}
}

func nearestTurnAncestor(head string, events map[string]StoryEventRecord) string {
	for id := head; id != ""; {
		record, ok := events[id]
		if !ok {
			return ""
		}
		if record.Envelope.Type == StoryEventTypeTurn {
			return id
		}
		id = parentIDFromRaw(record.Raw)
	}
	return ""
}

func branchSummaries(meta StoryMeta) []BranchSummary {
	result := make([]BranchSummary, 0, len(meta.Branches))
	for id, branch := range meta.Branches {
		result = append(result, BranchSummary{
			ID:        id,
			Head:      branch.Head,
			From:      branch.From,
			FromEvent: branch.FromEvent,
			Title:     branch.Title,
			CreatedAt: branch.CreatedAt,
			Current:   id == meta.CurrentBranch,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID == "main" {
			return true
		}
		if result[j].ID == "main" {
			return false
		}
		return result[i].CreatedAt < result[j].CreatedAt
	})
	return result
}

func parentIDFromRaw(raw map[string]any) string {
	switch value := raw["parent_id"].(type) {
	case string:
		return value
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func parentIDString(parentID any) string {
	switch value := parentID.(type) {
	case string:
		return value
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func compactText(text string, limit int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" {
		return "未命名节点"
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}
