package interactive

import (
	"encoding/json"
	"fmt"
	"strings"

	"denova/internal/agents/conversationjournal"
)

const (
	// Version 12 projects creator-authored branch-plan events and revision tokens.
	storyProjectionVersion      = 12
	storyRecentTransactionLimit = 200
	storyRecentCommitLimit      = 200
	storyTurnAnchorEvery        = 256
)

type storyTurnAnchor struct {
	Before int                        `json:"before"`
	Cursor conversationjournal.Cursor `json:"cursor"`
}

type storyCommitLocator struct {
	CommandID          string                     `json:"command_id"`
	OperationID        string                     `json:"operation_id"`
	Cycle              int                        `json:"cycle"`
	BranchID           string                     `json:"branch_id"`
	Hash               string                     `json:"hash"`
	AgentCanonicalHash string                     `json:"agent_canonical_hash,omitempty"`
	EventID            string                     `json:"event_id"`
	EventType          string                     `json:"event_type"`
	Cursor             conversationjournal.Cursor `json:"cursor"`
}

type storyBranchProjection struct {
	Head                  string                     `json:"head"`
	ContextRevision       uint64                     `json:"context_revision"`
	LatestTurnID          string                     `json:"latest_turn_id,omitempty"`
	LatestTurnParentID    string                     `json:"latest_turn_parent_id,omitempty"`
	Depth                 int                        `json:"depth"`
	State                 map[string]any             `json:"state"`
	StateBeforeLatest     map[string]any             `json:"state_before_latest,omitempty"`
	Plan                  *BranchPlan                `json:"plan,omitempty"`
	PlanBeforeLatest      *BranchPlan                `json:"plan_before_latest,omitempty"`
	PendingPlayerInputIDs []string                   `json:"pending_player_input_ids,omitempty"`
	PendingInterruption   *TurnInterruptedEvent      `json:"pending_interruption,omitempty"`
	TailCursor            conversationjournal.Cursor `json:"tail_cursor,omitempty"`
}

// storyJournalProjection is the bounded game reducer checkpoint. It stores
// current branch state and sparse locators, never historical narrative,
// thinking, rich tool results, maintenance state, or prior state snapshots.
type storyJournalProjection struct {
	Version       int                               `json:"version"`
	StoryID       string                            `json:"story_id"`
	Generation    string                            `json:"generation"`
	Meta          StoryMeta                         `json:"meta"`
	EventCount    int                               `json:"event_count"`
	TurnCount     int                               `json:"turn_count"`
	RecentCursors []conversationjournal.Cursor      `json:"recent_cursors,omitempty"`
	TurnAnchors   []storyTurnAnchor                 `json:"turn_anchors,omitempty"`
	RecentCommits []storyCommitLocator              `json:"recent_commits,omitempty"`
	Branches      map[string]*storyBranchProjection `json:"branches"`

	expectedID         string
	expectedGeneration string
	lastCursor         conversationjournal.Cursor
}

func newStoryJournalProjection(storyID, generation string) *storyJournalProjection {
	projection := &storyJournalProjection{expectedID: strings.TrimSpace(storyID), expectedGeneration: strings.TrimSpace(generation)}
	_ = projection.Reset()
	return projection
}

func (projection *storyJournalProjection) Reset() error {
	expectedID := projection.expectedID
	expectedGeneration := projection.expectedGeneration
	*projection = storyJournalProjection{
		Version: storyProjectionVersion, StoryID: expectedID, Generation: expectedGeneration,
		Branches: make(map[string]*storyBranchProjection), expectedID: expectedID, expectedGeneration: expectedGeneration,
	}
	return nil
}

func (projection *storyJournalProjection) Restore(data json.RawMessage) error {
	expectedID := projection.expectedID
	expectedGeneration := projection.expectedGeneration
	var restored storyJournalProjection
	if err := json.Unmarshal(data, &restored); err != nil {
		return err
	}
	if restored.Version != storyProjectionVersion {
		return fmt.Errorf("%w: unsupported story projection version %d", conversationjournal.ErrProjectionCheckpointIncompatible, restored.Version)
	}
	if restored.StoryID != expectedID || restored.Generation != expectedGeneration {
		return fmt.Errorf("story projection identity mismatch")
	}
	if restored.Branches == nil {
		restored.Branches = make(map[string]*storyBranchProjection)
	}
	restored.expectedID = expectedID
	restored.expectedGeneration = expectedGeneration
	if len(restored.RecentCursors) > 0 {
		restored.lastCursor = restored.RecentCursors[len(restored.RecentCursors)-1]
	}
	*projection = restored
	return nil
}

func (projection *storyJournalProjection) Checkpoint() (json.RawMessage, error) {
	return json.Marshal(projection)
}

func (projection *storyJournalProjection) Apply(record conversationjournal.Record) error {
	projection.rememberCursor(record.Location.Cursor)
	meta, events, err := decodeStoryProjectionPayload(record.Payload)
	if err != nil {
		return err
	}
	if meta.StoryID != "" {
		return projection.applyMeta(record.Location.Cursor, meta)
	}
	if len(events) != 1 {
		return fmt.Errorf("story projection payload is empty")
	}
	return projection.applyEvent(record.Location.Cursor, events[0])
}

func decodeStoryProjectionPayload(payload json.RawMessage) (StoryMeta, []StoryEventRecord, error) {
	var meta StoryMeta
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &typed); err != nil {
		return StoryMeta{}, nil, err
	}
	if typed.Type == StoryEventTypeMeta {
		if err := json.Unmarshal(payload, &meta); err != nil {
			return StoryMeta{}, nil, err
		}
		meta = normalizeStoryMeta(meta)
		if err := validateStoryMeta(meta); err != nil {
			return StoryMeta{}, nil, err
		}
		return meta, nil, nil
	}
	event, err := decodeStoryEventRecord(payload)
	if err != nil {
		return StoryMeta{}, nil, err
	}
	return StoryMeta{}, []StoryEventRecord{event}, nil
}

func (projection *storyJournalProjection) applyMeta(cursor conversationjournal.Cursor, meta StoryMeta) error {
	meta = normalizeStoryMeta(meta)
	if err := validateStoryMeta(meta); err != nil {
		return err
	}
	if meta.StoryID != projection.expectedID {
		return fmt.Errorf("story projection id mismatch: have=%q want=%q", meta.StoryID, projection.expectedID)
	}
	projection.Meta = meta
	projection.StoryID = meta.StoryID
	projection.Generation = projection.expectedGeneration
	for branchID, branchMeta := range meta.Branches {
		branch := projection.branch(branchID)
		// The immutable first metadata row describes the final legacy head even
		// though its flat events follow. Rebuild that path from the root.
		if cursor != 1 || projection.EventCount > 0 {
			branch.Head = branchMeta.Head
		}
	}
	for branchID := range projection.Branches {
		if _, ok := meta.Branches[branchID]; !ok {
			delete(projection.Branches, branchID)
		}
	}
	return nil
}

func (projection *storyJournalProjection) applyEvent(cursor conversationjournal.Cursor, record StoryEventRecord) error {
	changesModelContext, err := storyEventChangesModelContext(record.Envelope.Type)
	if err != nil {
		return err
	}
	projection.EventCount++
	branch := projection.branch(record.Envelope.BranchID)
	branch.TailCursor = cursor
	parentID := parentIDFromRaw(record.Raw)
	switch record.Envelope.Type {
	case StoryEventTypeTurn:
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			return err
		}
		branch.consumePlayerInputs(turn.PlayerInputID, turn.ConsumedPlayerInputIDs)
		if branch.PendingInterruption != nil && !branch.hasPendingPlayerInput(branch.PendingInterruption.PlayerInputID) {
			branch.PendingInterruption = nil
		}
		if projection.TurnCount%storyTurnAnchorEvery == 0 {
			projection.TurnAnchors = append(projection.TurnAnchors, storyTurnAnchor{Before: projection.TurnCount, Cursor: cursor})
		}
		projection.TurnCount++
		if parentID == branch.Head || (branch.Head == "" && parentID == "") {
			before := cloneStoryState(branch.State)
			branch.StateBeforeLatest = cloneStoryState(before)
			branch.PlanBeforeLatest = cloneBranchPlan(branch.Plan)
			branch.State = cloneStoryState(before)
			applyTurnState(branch.State, turn)
			branch.Head = turn.ID
			branch.LatestTurnID = turn.ID
			branch.LatestTurnParentID = parentID
			branch.Depth++
		} else if parentID == branch.LatestTurnParentID && branch.LatestTurnID != "" {
			branch.State = cloneStoryState(branch.StateBeforeLatest)
			branch.Plan = cloneBranchPlan(branch.PlanBeforeLatest)
			applyTurnState(branch.State, turn)
			branch.Head = turn.ID
			branch.LatestTurnID = turn.ID
		}
		projection.rememberCommit(cursor, StoryEventTypeTurn, turn.ID, turn.BranchID, turn.AgentCommandID, turn.AgentOperationID, turn.AgentCycle, turn.AgentCommitHash, turn.AgentCanonicalHash)
	case StoryEventTypeStateDelta:
		var delta StateDeltaEvent
		if err := mapToStruct(record.Raw, &delta); err != nil {
			return err
		}
		if parentID == branch.Head || record.Envelope.ID == branch.Head {
			applyStateDeltaToProjection(branch.State, StateDelta{SchemaVersion: delta.SchemaVersion, Ops: delta.Ops, ActorOps: delta.ActorOps})
			branch.Head = record.Envelope.ID
		}
	case StoryEventTypePlayerInput:
		var event PlayerInputAcceptedEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		branch.rememberPendingPlayerInput(event.ID)
		projection.rememberCommit(cursor, StoryEventTypePlayerInput, event.ID, event.BranchID, event.AgentCommandID, event.AgentOperationID, event.AgentCycle, event.AgentCommitHash, event.AgentCanonicalHash)
	case StoryEventTypeTurnInterrupted:
		var event TurnInterruptedEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		branch.PendingInterruption = &event
	case StoryEventTypeModelContextBatch:
		var event ModelContextBatchEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		normalized, err := normalizeModelContextBatchEvent(event)
		if err != nil {
			return err
		}
		projection.rememberCommit(cursor, StoryEventTypeModelContextBatch, normalized.ID, normalized.BranchID, normalized.AgentCommandID, normalized.AgentOperationID, normalized.AgentCycle, normalized.BatchHash, "")
	case StoryEventTypeProviderContinuation, StoryEventTypeModelContextProviderContinuation:
		// Opaque state is joined to its owner only by bounded model-history reads.
		// It never changes branch state or public projections by itself.
	case StoryEventTypeBranchPlanUpdated:
		var event BranchPlanUpdatedEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		if event.TurnID == branch.LatestTurnID && event.ParentID == event.TurnID {
			branch.Plan = &BranchPlan{Markdown: event.Markdown, UpdatedTurnID: event.TurnID, UpdatedAt: event.Ts, Revision: event.ID}
		}
	case StoryEventTypeBranchPlanRevised:
		var event BranchPlanUpdatedEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		if event.TurnID == branch.LatestTurnID && event.ParentID == branch.Head {
			branch.Plan = &BranchPlan{Markdown: event.Markdown, UpdatedTurnID: event.TurnID, UpdatedAt: event.Ts, Revision: event.ID}
			branch.Head = event.ID
		}
	case StoryEventTypeTurnStateRevised:
		var revision TurnStateRevisedEvent
		if err := mapToStruct(record.Raw, &revision); err != nil {
			return err
		}
		if revision.TurnID == branch.LatestTurnID && (revision.ClearStateDelta || revision.StateDelta != nil) {
			branch.State = cloneStoryState(branch.StateBeforeLatest)
			if !revision.ClearStateDelta && revision.StateDelta != nil {
				applyStateDeltaToProjection(branch.State, *revision.StateDelta)
			}
		}
	case StoryEventTypeBranch:
		var event BranchEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		branch.Head = event.ParentID
		branch.LatestTurnID = event.LatestTurnID
		branch.Depth = event.Depth
		// A fork inherits the complete model-visible prefix at its parent. Its
		// revision therefore starts at that canonical depth rather than at the
		// number of branch-creation records observed for the new branch.
		branch.ContextRevision = uint64(event.Depth)
		if event.StateCheckpoint != nil {
			branch.State = cloneStoryState(event.StateCheckpoint)
		}
		branch.Plan = cloneBranchPlan(event.PlanCheckpoint)
		branch.PlanBeforeLatest = nil
	case StoryEventTypeBranchHeadMoved:
		var event BranchHeadMovedEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		branch.Head = event.NextHead
		branch.LatestTurnID = event.NextLatestTurnID
		branch.Depth = event.NextDepth
		branch.State = cloneStoryState(event.StateCheckpoint)
		branch.StateBeforeLatest = nil
		branch.Plan = cloneBranchPlan(event.PlanCheckpoint)
		branch.PlanBeforeLatest = nil
	case StoryEventTypeTurnVersionSelected:
		var event TurnVersionSelectionEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		branch.Head = event.ProjectedHeadID
		branch.LatestTurnID = event.CurrentTurnID
		branch.Depth = event.CurrentDepth
		if event.CurrentState != nil {
			branch.State = cloneStoryState(event.CurrentState)
		}
		branch.StateBeforeLatest = nil
		branch.Plan = cloneBranchPlan(event.CurrentPlan)
		branch.PlanBeforeLatest = nil
	case StoryEventTypeHotChoices,
		StoryEventTypeTurnNarrativeRevised, StoryEventTypeTurnDisplayAppended,
		StoryEventTypeStoryConfigUpdated, StoryEventTypeBranchSwitched, StoryEventTypeBranchArchived:
		// Side/audit records do not independently advance branch state.
	default:
		return fmt.Errorf("story projection does not handle persisted event type %q", record.Envelope.Type)
	}
	if changesModelContext && record.Envelope.Type != StoryEventTypeBranch {
		branch.ContextRevision++
	}
	return nil
}

func (branch *storyBranchProjection) rememberPendingPlayerInput(playerInputID string) {
	playerInputID = strings.TrimSpace(playerInputID)
	if playerInputID == "" {
		return
	}
	for _, existing := range branch.PendingPlayerInputIDs {
		if existing == playerInputID {
			return
		}
	}
	branch.PendingPlayerInputIDs = append(branch.PendingPlayerInputIDs, playerInputID)
}

func (branch *storyBranchProjection) hasPendingPlayerInput(playerInputID string) bool {
	if branch == nil {
		return false
	}
	playerInputID = strings.TrimSpace(playerInputID)
	for _, pendingID := range branch.PendingPlayerInputIDs {
		if pendingID == playerInputID {
			return true
		}
	}
	return false
}

func (branch *storyBranchProjection) consumePlayerInputs(currentPlayerInputID string, consumedPlayerInputIDs []string) {
	if branch == nil || len(branch.PendingPlayerInputIDs) == 0 {
		return
	}
	consumed := make(map[string]bool, len(consumedPlayerInputIDs)+1)
	if currentPlayerInputID = strings.TrimSpace(currentPlayerInputID); currentPlayerInputID != "" {
		consumed[currentPlayerInputID] = true
	}
	for _, playerInputID := range consumedPlayerInputIDs {
		if playerInputID = strings.TrimSpace(playerInputID); playerInputID != "" {
			consumed[playerInputID] = true
		}
	}
	if len(consumed) == 0 {
		return
	}
	pending := branch.PendingPlayerInputIDs[:0]
	for _, playerInputID := range branch.PendingPlayerInputIDs {
		if !consumed[playerInputID] {
			pending = append(pending, playerInputID)
		}
	}
	if len(pending) == 0 {
		branch.PendingPlayerInputIDs = nil
		return
	}
	branch.PendingPlayerInputIDs = pending
}

func (projection *storyJournalProjection) branch(branchID string) *storyBranchProjection {
	branchID = strings.TrimSpace(branchID)
	branch := projection.Branches[branchID]
	if branch == nil {
		branch = &storyBranchProjection{State: initialStoryState()}
		projection.Branches[branchID] = branch
	}
	if branch.State == nil {
		branch.State = initialStoryState()
	}
	return branch
}

func (projection *storyJournalProjection) rememberCursor(cursor conversationjournal.Cursor) {
	if cursor == 0 || cursor == projection.lastCursor {
		return
	}
	projection.lastCursor = cursor
	projection.RecentCursors = append(projection.RecentCursors, cursor)
	if overflow := len(projection.RecentCursors) - storyRecentTransactionLimit; overflow > 0 {
		projection.RecentCursors = append([]conversationjournal.Cursor(nil), projection.RecentCursors[overflow:]...)
	}
}

func (projection *storyJournalProjection) rememberCommit(cursor conversationjournal.Cursor, eventType, eventID, branchID, commandID, operationID string, cycle int, hash, agentCanonicalHash string) {
	if strings.TrimSpace(commandID) == "" {
		return
	}
	projection.RecentCommits = append(projection.RecentCommits, storyCommitLocator{
		CommandID: strings.TrimSpace(commandID), OperationID: strings.TrimSpace(operationID), Cycle: cycle,
		BranchID: branchID, Hash: strings.TrimSpace(hash), AgentCanonicalHash: strings.TrimSpace(agentCanonicalHash),
		EventID: eventID, EventType: eventType, Cursor: cursor,
	})
	if overflow := len(projection.RecentCommits) - storyRecentCommitLimit; overflow > 0 {
		projection.RecentCommits = append([]storyCommitLocator(nil), projection.RecentCommits[overflow:]...)
	}
}

func applyTurnState(state map[string]any, turn TurnEvent) {
	if turn.StateDelta != nil {
		applyStateDeltaToProjection(state, *turn.StateDelta)
	}
}

func applyStateDeltaToProjection(state map[string]any, delta StateDelta) {
	for _, op := range delta.Ops {
		applyStateOp(state, op)
	}
	for _, op := range delta.ActorOps {
		applyActorStateOp(state, op)
	}
}

func cloneStoryState(state map[string]any) map[string]any {
	if state == nil {
		return initialStoryState()
	}
	data, err := json.Marshal(state)
	if err != nil {
		return initialStoryState()
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil || cloned == nil {
		return initialStoryState()
	}
	return cloned
}

func storyJournalGeneration(meta StoryMeta) string {
	return "story:" + strings.TrimSpace(meta.StoryID) + ":" + strings.TrimSpace(meta.CreatedAt)
}
