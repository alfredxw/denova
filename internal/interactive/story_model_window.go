package interactive

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"denova/internal/agents/conversationjournal"
	"denova/internal/agents/toolresult"

	agenthistory "github.com/alfredxw/denova/agent/context/history"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agentcanonical "github.com/alfredxw/denova/agent/session/canonical"
)

type storyResolvedModelSpan struct {
	Boundary int `json:"boundary"`
	Count    int `json:"count"`
}
type storyModelNode struct {
	Parent        string
	Cursor        conversationjournal.Cursor
	Count         int
	TurnAt        string // Original Turn timestamp for activity queries without reading history bodies.
	Resolved      []storyResolvedModelSpan
	Narratives    map[string]conversationjournal.Cursor
	Continuations []conversationjournal.Cursor
}

// Compact tuples keep per-event locators small even for short legacy Turns.
// Field names stay explicit in memory; this versioned cache is rebuildable.
func (node storyModelNode) MarshalJSON() ([]byte, error) {
	fields := []any{node.Parent, node.Cursor, node.Count, node.TurnAt}
	if len(node.Resolved)+len(node.Narratives)+len(node.Continuations) > 0 {
		fields = append(fields, node.Resolved, node.Narratives, node.Continuations)
	}
	return json.Marshal(fields)
}
func (node *storyModelNode) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields) != 4 && len(fields) != 7 {
		return fmt.Errorf("invalid Game model locator")
	}
	targets := []any{&node.Parent, &node.Cursor, &node.Count, &node.TurnAt, &node.Resolved, &node.Narratives, &node.Continuations}
	for index, field := range fields {
		if err := json.Unmarshal(field, targets[index]); err != nil {
			return err
		}
	}
	return nil
}

func (projection *storyJournalProjection) indexModelEvent(cursor conversationjournal.Cursor, record StoryEventRecord) error {
	if projection.ModelNodes == nil {
		projection.ModelNodes = make(map[string]storyModelNode)
	}
	node := projection.ModelNodes[record.Envelope.ID]
	node.Parent, node.Cursor = parentIDFromRaw(record.Raw), cursor
	policy := toolresult.ContextPolicy{Enabled: true}
	switch record.Envelope.Type {
	case StoryEventTypeTurn:
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			return err
		}
		resolved, err := normalizeResolvedPlayerInputContexts(turn.ResolvedPlayerInputContexts, turn.BranchID, turn.PlayerInputID, turn.ConsumedPlayerInputIDs)
		if err != nil {
			return err
		}
		node.Count = len(ModelTurnMessages(modelTurnFromEvent(turn), policy))
		node.TurnAt = turn.Ts
		for _, context := range resolved {
			messages := PlayerInputContextMessages(context.ModelContextBatches, agentschema.UserMessageWithAttachments(context.Input.Text, context.Input.Attachments))
			node.Resolved = append(node.Resolved, storyResolvedModelSpan{Boundary: context.Input.AcceptedTurnCount, Count: len(toolresult.ApplyContextPolicy(messages, policy))})
		}
	case StoryEventTypeTurnNarrativeRevised:
		var revision TurnNarrativeRevisedEvent
		if err := mapToStruct(record.Raw, &revision); err != nil {
			return err
		}
		owner := projection.ModelNodes[revision.TurnID]
		if owner.Narratives == nil {
			owner.Narratives = make(map[string]conversationjournal.Cursor)
		}
		owner.Narratives[record.Envelope.BranchID] = cursor
		projection.ModelNodes[revision.TurnID] = owner
	case StoryEventTypeProviderContinuation:
		var continuation providerContinuationEvent
		if err := mapToStruct(record.Raw, &continuation); err != nil {
			return err
		}
		owner := projection.ModelNodes[continuation.TurnID]
		owner.Continuations = append(owner.Continuations, cursor)
		projection.ModelNodes[continuation.TurnID] = owner
	case StoryEventTypeModelContextProviderContinuation:
		var continuation modelContextProviderContinuationEvent
		if err := mapToStruct(record.Raw, &continuation); err != nil {
			return err
		}
		owner := projection.ModelNodes[continuation.OwnerID]
		if len(owner.Continuations) == 0 || owner.Continuations[len(owner.Continuations)-1] != cursor {
			owner.Continuations = append(owner.Continuations, cursor)
		}
		projection.ModelNodes[continuation.OwnerID] = owner
	}
	projection.ModelNodes[record.Envelope.ID] = node
	return nil
}

func modelTurnFromEvent(turn TurnEvent) StoryModelTurn {
	return StoryModelTurn{ID: turn.ID, BranchID: turn.BranchID, PlayerInputID: turn.PlayerInputID, Ts: turn.Ts, User: turn.User, Attachments: turn.Attachments, Narrative: turn.Narrative,
		ProviderContinuation: turn.ProviderContinuation, ResolvedPlayerInputContexts: turn.ResolvedPlayerInputContexts, ModelContextMessages: turn.ModelContextMessages}
}

type storyModelBlock struct {
	turnID   string
	resolved int // -1 selects the settled Turn; non-negative selects its resolved input.
	count    int
	messages []*agentschema.Message // Only unfinished inputs already present in the bounded snapshot.
}

// ReadCanonicalWindow reads only selected model-message blocks on the exact
// branch ancestry. The index contains counts and locators; selected bodies,
// narrative revisions and provider continuations come from Story JSONL.
func (s *Store) ReadCanonicalWindow(ctx context.Context, storyID string, snapshot Snapshot, count int, archive *agenthistory.HistoryArchive) ([]*agentschema.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if archive != nil {
		if archive.From < 0 || archive.To <= archive.From || archive.To > count {
			return nil, fmt.Errorf("%w: invalid Game archive", agentcanonical.ErrInvalidCanonicalMessages)
		}
		previous := archive.From - 1
		for _, index := range archive.Retained {
			if index <= previous || index >= archive.To {
				return nil, fmt.Errorf("%w: invalid retained Game coordinate", agentcanonical.ErrInvalidCanonicalMessages)
			}
			previous = index
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return nil, err
	}
	defer release()
	handle, err := s.refreshStoryJournalLocked(storyID, false)
	if err != nil {
		return nil, err
	}
	branch := handle.projection.Branches[snapshot.BranchID]
	if branch == nil || snapshot.TurnCount > branch.Depth {
		return nil, fmt.Errorf("%w: Game branch changed", agentcanonical.ErrInvalidCanonicalMessages)
	}
	if snapshot.TurnCount == branch.Depth && snapshot.ContextRevision != branch.ContextRevision {
		return nil, fmt.Errorf("%w: Game context revision changed", ErrStoryContextRevisionConflict)
	}
	var path []string
	seen := make(map[string]bool)
	for id := branch.Head; id != ""; {
		node, found := handle.projection.ModelNodes[id]
		if !found || seen[id] {
			return nil, fmt.Errorf("%w: Game ancestry is incomplete at %s", agentcanonical.ErrInvalidCanonicalMessages, id)
		}
		seen[id] = true
		if node.Count > 0 {
			path = append(path, id)
		}
		id = node.Parent
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	if len(path) != branch.Depth {
		return nil, fmt.Errorf("%w: Game ancestry depth changed", agentcanonical.ErrInvalidCanonicalMessages)
	}
	path = path[:snapshot.TurnCount] // Regeneration reads the target's parent prefix.
	resolvedAt := make(map[int][]storyModelBlock)
	for owner, id := range path {
		for index, resolved := range handle.projection.ModelNodes[id].Resolved {
			boundary := resolved.Boundary
			if boundary == owner+1 {
				boundary = owner
			}
			if boundary < 0 || boundary > owner {
				return nil, fmt.Errorf("%w: invalid resolved Game input boundary", agentcanonical.ErrInvalidCanonicalMessages)
			}
			resolvedAt[boundary] = append(resolvedAt[boundary], storyModelBlock{turnID: id, resolved: index, count: resolved.Count})
		}
	}
	policy := toolresult.ContextPolicy{Enabled: true}
	batches := make(map[string][]ModelContextBatchEvent)
	for _, batch := range snapshot.PendingModelContextBatches {
		batches[batch.PlayerInputID] = append(batches[batch.PlayerInputID], batch)
	}
	pendingAt := make(map[int][]storyModelBlock)
	for _, input := range snapshot.PendingPlayerInputs {
		if input.AcceptedTurnCount < 0 || input.AcceptedTurnCount > branch.Depth {
			return nil, fmt.Errorf("%w: invalid pending Game input boundary", agentcanonical.ErrInvalidCanonicalMessages)
		}
		messages := toolresult.ApplyContextPolicy(PlayerInputContextMessages(batches[input.ID], agentschema.UserMessageWithAttachments(input.Text, input.Attachments)), policy)
		pendingAt[input.AcceptedTurnCount] = append(pendingAt[input.AcceptedTurnCount], storyModelBlock{count: len(messages), messages: messages})
	}
	var blocks []storyModelBlock
	actual := 0
	for boundary := 0; boundary <= len(path); boundary++ {
		blocks = append(blocks, resolvedAt[boundary]...)
		blocks = append(blocks, pendingAt[boundary]...)
		if boundary < len(path) {
			id := path[boundary]
			blocks = append(blocks, storyModelBlock{turnID: id, resolved: -1, count: handle.projection.ModelNodes[id].Count})
		}
	}
	for _, block := range blocks {
		actual += block.count
	}
	if actual != count {
		return nil, fmt.Errorf("%w: aligned Game message count changed (%d != %d)", agentcanonical.ErrInvalidCanonicalMessages, actual, count)
	}
	loaded := make(map[string]StoryModelTurn)
	var stats StoryJournalReplayStats
	var result []*agentschema.Message
	ordinal := 0
	for _, block := range blocks {
		end := ordinal + block.count
		selected := archive == nil || ordinal < archive.From || end > archive.To
		if !selected {
			index := sort.SearchInts(archive.Retained, ordinal)
			selected = index < len(archive.Retained) && archive.Retained[index] < end
		}
		if selected {
			messages := block.messages
			if block.turnID != "" {
				turn, found := loaded[block.turnID]
				if !found {
					turn, err = readIndexedModelTurn(ctx, handle, block.turnID, snapshot.BranchID, branch.TailCursor, &stats)
					if err != nil {
						return nil, err
					}
					loaded[block.turnID] = turn
				}
				if block.resolved < 0 {
					messages = ModelTurnMessages(turn, policy)
				} else {
					resolved := turn.ResolvedPlayerInputContexts[block.resolved]
					messages = toolresult.ApplyContextPolicy(PlayerInputContextMessages(resolved.ModelContextBatches, agentschema.UserMessageWithAttachments(resolved.Input.Text, resolved.Input.Attachments)), policy)
				}
			}
			if len(messages) != block.count {
				return nil, fmt.Errorf("%w: Game model block changed", agentcanonical.ErrInvalidCanonicalMessages)
			}
			for index, message := range agentcanonical.CanonicalContextStateOrder(messages) {
				if archive.Contains(ordinal + index) {
					result = append(result, message)
				}
			}
		}
		ordinal = end
	}
	s.rememberStoryReplayStats(storyID, stats)
	return result, nil
}

func readIndexedModelTurn(ctx context.Context, handle *storyJournalHandle, id, branchID string, through conversationjournal.Cursor, stats *StoryJournalReplayStats) (StoryModelTurn, error) {
	node := handle.projection.ModelNodes[id]
	cursors := append([]conversationjournal.Cursor{node.Cursor}, node.Continuations...)
	if cursor := node.Narratives[branchID]; cursor != 0 && cursor <= through {
		cursors = append(cursors, cursor)
	}
	sort.Slice(cursors, func(i, j int) bool { return cursors[i] < cursors[j] })
	var lines []StoryEventRecord
	for index, cursor := range cursors {
		if cursor > through || (index > 0 && cursor == cursors[index-1]) {
			continue
		}
		records, err := handle.journal.ReadRange(ctx, conversationjournal.Range{After: cursor - 1, Through: cursor})
		if err != nil {
			return StoryModelTurn{}, err
		}
		stats.BytesRead += handle.journal.ReplayStats().LastRangeBytesRead
		stats.RecordsRead += int64(len(records))
		located, err := decodeLocatedStoryRecords(records)
		if err != nil {
			return StoryModelTurn{}, err
		}
		for _, item := range located {
			record := item.record
			if record.Envelope.ID != id && record.Envelope.BranchID != branchID {
				continue
			}
			include := record.Envelope.ID == id || (record.Envelope.Type == StoryEventTypeTurnNarrativeRevised && storyRevisionTurnID(record) == id)
			if record.Envelope.Type == StoryEventTypeProviderContinuation {
				var event providerContinuationEvent
				if err := mapToStruct(record.Raw, &event); err != nil {
					return StoryModelTurn{}, err
				}
				include = event.TurnID == id
			}
			if record.Envelope.Type == StoryEventTypeModelContextProviderContinuation {
				var event modelContextProviderContinuationEvent
				if err := mapToStruct(record.Raw, &event); err != nil {
					return StoryModelTurn{}, err
				}
				include = event.OwnerID == id
			}
			if include {
				lines = append(lines, record)
			}
		}
	}
	lines, err := projectStoryEventOverlays(lines)
	if err != nil {
		return StoryModelTurn{}, err
	}
	continuations, err := providerContinuationsByTurn(lines)
	if err != nil {
		return StoryModelTurn{}, err
	}
	contextContinuations, err := modelContextProviderContinuationsByOwner(lines)
	if err != nil {
		return StoryModelTurn{}, err
	}
	for _, record := range lines {
		if record.Envelope.ID != id || record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			return StoryModelTurn{}, err
		}
		turn.ModelContextMessages, err = hydrateModelContextProviderContinuations(turn.ModelContextMessages, id, contextContinuations)
		if err != nil {
			return StoryModelTurn{}, err
		}
		turn.ProviderContinuation = continuations[id]
		turn.ResolvedPlayerInputContexts, err = normalizeResolvedPlayerInputContexts(turn.ResolvedPlayerInputContexts, turn.BranchID, turn.PlayerInputID, turn.ConsumedPlayerInputIDs)
		if err != nil {
			return StoryModelTurn{}, err
		}
		return modelTurnFromEvent(turn), nil
	}
	return StoryModelTurn{}, fmt.Errorf("%w: Game turn %s is missing", agentcanonical.ErrInvalidCanonicalMessages, id)
}
