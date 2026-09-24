package interactive

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"denova/internal/agents/conversationjournal"
	"denova/internal/localfs"
)

var (
	ErrStorySceneNotFound         = errors.New("recorded scene version is unavailable")
	ErrStorySceneRevisionConflict = errors.New("recorded scene narrative revision changed")
)

// TurnSceneProjection contains only native data needed for scene inspection.
// A discarded regenerated version can be inspected without selecting it.
type TurnSceneProjection struct {
	Turn         TurnEvent
	PreviousTurn *TurnEvent
	State        TurnStateProjection
}

// ReadSceneAtTurn projects an exact recorded version's original parent chain.
// It uses native journal decoding and checkpoint reduction, with no migrations,
// tail repair, branch commands or canonical writes. Rebuildable indexes may be
// refreshed by the ordinary journal reader.
func (s *Store) ReadSceneAtTurn(ctx context.Context, storyID, branchID, turnID, sourceRevision string) (TurnSceneProjection, error) {
	if err := ctx.Err(); err != nil {
		return TurnSceneProjection{}, err
	}
	for _, value := range []string{branchID, turnID, sourceRevision} {
		if value == "" || strings.TrimSpace(value) != value {
			return TurnSceneProjection{}, fmt.Errorf("%w: exact branch, turn and revision are required", ErrStorySceneNotFound)
		}
	}
	// This independent handle touches no Store caches. Use the native lease
	// directly so cancellation also interrupts contention with a writer, without
	// first waiting for the Store mutex or blocking reads of other stories.
	storyPath := s.storyPath(storyID)
	release, err := localfs.AcquireLease(ctx, storyPath+".mutation.lock")
	if err != nil {
		return TurnSceneProjection{}, err
	}
	defer func() {
		if err := release(); err != nil {
			slog.ErrorContext(ctx, "[interactive-story] release scene read lease failed", "story_id", storyID, "error", err)
		}
	}()
	// Opening through Store's normal loader can migrate released data. This
	// inspection handle intentionally uses the existing journal reader directly.
	meta, err := readStoryJournalHeader(s.storyPath(storyID))
	if err != nil {
		return TurnSceneProjection{}, err
	}
	generation := storyJournalGeneration(meta)
	projection := newStoryJournalProjection(storyID, generation)
	journal, err := conversationjournal.Open(ctx, s.storyPath(storyID), conversationjournal.Identity{ID: storyID, Generation: generation}, projection, conversationjournal.Options{})
	if err != nil {
		return TurnSceneProjection{}, err
	}
	defer journal.Close()
	info, err := os.Stat(s.storyPath(storyID))
	if err != nil {
		return TurnSceneProjection{}, err
	}
	if info.Size() > journal.Head().VerifiedBytes {
		return TurnSceneProjection{}, fmt.Errorf("scene inspection requires a complete canonical journal tail")
	}
	meta, err = cloneStoryMeta(projection.Meta)
	if err != nil {
		return TurnSceneProjection{}, err
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return TurnSceneProjection{}, fmt.Errorf("%w: branch %q", ErrStorySceneNotFound, branchID)
	}
	handle := &storyJournalHandle{journal: journal}
	var links []StoryEventRecord
	if err := scanStoryEventsContextLocked(ctx, handle, func(record StoryEventRecord) error {
		if record.Envelope.ID != "" {
			// Keep only ancestry and version identity. Private provider/tool/model
			// payloads and unrelated prose are discarded after each scan batch.
			links = append(links, StoryEventRecord{Envelope: record.Envelope, Raw: map[string]any{"parent_id": parentIDFromRaw(record.Raw)}})
		}
		return nil
	}); err != nil {
		return TurnSceneProjection{}, err
	}
	events := eventsByID(links)
	target, ok := events[turnID]
	if !ok || target.Envelope.Type != StoryEventTypeTurn {
		return TurnSceneProjection{}, fmt.Errorf("%w: turn %q", ErrStorySceneNotFound, turnID)
	}
	if target.Envelope.BranchID != branchID {
		_, inherited := eventPath(branch.FromEvent, events)
		if !inherited[turnID] {
			return TurnSceneProjection{}, fmt.Errorf("%w: turn is not owned or inherited by branch %q", ErrStorySceneNotFound, branchID)
		}
	}
	ancestry, ancestorIDs := eventPath(turnID, events)
	if len(ancestry) == 0 || parentIDFromRaw(ancestry[0].Raw) != "" {
		return TurnSceneProjection{}, fmt.Errorf("scene inspection found an incomplete native parent chain")
	}
	previousID := ""
	for index := len(ancestry) - 2; index >= 0; index-- {
		if ancestry[index].Envelope.Type == StoryEventTypeTurn {
			previousID = ancestry[index].Envelope.ID
			break
		}
	}
	turns, err := sceneTurnsFromJournal(ctx, handle, branchID, turnID, previousID)
	if err != nil {
		return TurnSceneProjection{}, err
	}
	turn, ok := turns[turnID]
	if !ok {
		return TurnSceneProjection{}, fmt.Errorf("%w: turn projection is unavailable", ErrStorySceneNotFound)
	}
	if TurnNarrativeRevision(turn) != sourceRevision {
		return TurnSceneProjection{}, ErrStorySceneRevisionConflict
	}
	state, err := s.stateForStoryAncestorsContextLocked(ctx, handle, ancestorIDs)
	if err != nil {
		return TurnSceneProjection{}, err
	}
	if err := applyFrozenMissingInitialActors(state, meta.ActorStateSchema); err != nil {
		return TurnSceneProjection{}, err
	}
	applyLegacyActorStateAliases(state, meta.ActorStateSchema)
	versions := buildTurnVersionIndex(links)
	for id, value := range turns {
		if alternatives := versions[turnVersionKey(value.BranchID, parentIDFromRaw(events[id].Raw))]; len(alternatives) > 1 {
			value.Versions = append([]TurnVersion(nil), alternatives...)
			for index := range value.Versions {
				if value.Versions[index].TurnID == id {
					value.VersionIdx = index
					value.Versions[index].Current = true
				}
			}
			turns[id] = value
		}
	}
	if err := ctx.Err(); err != nil {
		return TurnSceneProjection{}, err
	}
	result := TurnSceneProjection{Turn: turns[turnID], State: TurnStateProjection{BranchID: branchID, SourceRevision: sourceRevision, State: state, Schema: meta.ActorStateSchema}}
	if previous, ok := turns[previousID]; ok {
		result.PreviousTurn = &previous
	}
	return result, nil
}

// sceneTurnsFromJournal retains two public turn payloads and reduces only their
// native side revisions in journal order. It never hydrates private context or
// constructs the full Story graph, including on discarded regeneration paths.
func sceneTurnsFromJournal(ctx context.Context, handle *storyJournalHandle, branchID, turnID, previousID string) (map[string]TurnEvent, error) {
	selected := make(map[string]StoryEventRecord, 2)
	var legacyChoices HotChoicesEvent
	if err := scanStoryEventsContextLocked(ctx, handle, func(record StoryEventRecord) error {
		switch record.Envelope.Type {
		case StoryEventTypeTurn:
			if record.Envelope.ID != turnID && record.Envelope.ID != previousID {
				return nil
			}
			raw := make(map[string]any)
			for _, key := range []string{"v", "type", "id", "parent_id", "branch_id", "ts", "user", "user_context_only", "narrative", "narrative_revision", "state_delta", "state_status", "hot_state", "turn_result"} {
				if value, ok := record.Raw[key]; ok {
					raw[key] = value
				}
			}
			selected[record.Envelope.ID] = StoryEventRecord{Envelope: record.Envelope, Raw: raw}
		case StoryEventTypeTurnNarrativeRevised, StoryEventTypeTurnStateRevised:
			targetID, _ := record.Raw["turn_id"].(string)
			base, ok := selected[targetID]
			if !ok {
				return nil
			}
			projected, err := projectStoryEventOverlays([]StoryEventRecord{base, record})
			if err != nil {
				return err
			}
			selected[targetID] = projected[0]
		case StoryEventTypeHotChoices:
			if choices, ok := latestHotChoicesForHead([]StoryEventRecord{record}, branchID, turnID); ok && (legacyChoices.ID == "" || choices.Ts >= legacyChoices.Ts) {
				legacyChoices = choices
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	turns := make(map[string]TurnEvent, len(selected))
	for id, record := range selected {
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			return nil, err
		}
		if id == turnID && (turn.TurnResult == nil || len(turn.TurnResult.Choices) == 0) && turn.HotState == nil {
			turn.HotState = normalizeHotState(&HotState{Choices: legacyChoices.Choices})
		}
		turns[id] = turn
	}
	return turns, nil
}
