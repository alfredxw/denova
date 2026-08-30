package interactive

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"denova/internal/agents/conversationjournal"
)

const storyCheckpointScanTransactions = 1024

type storyEventCheckpoint struct {
	SourceBranchID string
	LatestTurnID   string
	Depth          int
	State          map[string]any
	Plan           *BranchPlan
}

// checkpointAtTurnLocked resolves one historical turn without materializing
// the story. The backwards pass retains only ancestor IDs; two forward
// streaming passes then account for append-only state revisions and build the
// exact branch checkpoint in chronological order.
func (s *Store) checkpointAtTurnLocked(storyID, turnID string) (storyEventCheckpoint, error) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return storyEventCheckpoint{}, fmt.Errorf("父回合不能为空 / Parent turn is required")
	}
	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return storyEventCheckpoint{}, err
	}
	branchIDs := make([]string, 0, len(handle.projection.Meta.Branches))
	for branchID := range handle.projection.Meta.Branches {
		branchIDs = append(branchIDs, branchID)
	}
	sort.Strings(branchIDs)

	for _, pathBranchID := range branchIDs {
		ancestorIDs := make(map[string]bool)
		cursor := ""
		found := false
		sourceBranchID := ""
		depth := 0
		for {
			loaded, readErr := s.readStoryHistoryPageLocked(storyID, pathBranchID, cursor, maxStoryHistoryPageTurns, true)
			if readErr != nil {
				return storyEventCheckpoint{}, readErr
			}
			path, _ := eventPath(loaded.pageHeadID, eventsByID(loaded.records))
			through := len(path)
			if !found {
				through = 0
				for index, record := range path {
					if record.Envelope.Type != StoryEventTypeTurn || record.Envelope.ID != turnID {
						continue
					}
					found = true
					through = index + 1
					sourceBranchID = record.Envelope.BranchID
					break
				}
			}
			if found {
				for _, record := range path[:through] {
					if ancestorIDs[record.Envelope.ID] {
						continue
					}
					ancestorIDs[record.Envelope.ID] = true
					if record.Envelope.Type == StoryEventTypeTurn {
						depth++
					}
				}
			}
			if !loaded.page.HasMore || strings.TrimSpace(loaded.page.BeforeCursor) == "" {
				break
			}
			cursor = loaded.page.BeforeCursor
		}
		if !found {
			continue
		}
		state, stateErr := s.stateForStoryAncestorsLocked(handle, ancestorIDs)
		if stateErr != nil {
			return storyEventCheckpoint{}, stateErr
		}
		plan, planErr := s.planForStoryAncestorsLocked(handle, ancestorIDs)
		if planErr != nil {
			return storyEventCheckpoint{}, planErr
		}
		return storyEventCheckpoint{
			SourceBranchID: sourceBranchID,
			LatestTurnID:   turnID,
			Depth:          depth,
			State:          state,
			Plan:           plan,
		}, nil
	}
	return storyEventCheckpoint{}, fmt.Errorf("父回合不存在 / Parent turn not found: %s", turnID)
}

func (s *Store) planForStoryAncestorsLocked(handle *storyJournalHandle, ancestorIDs map[string]bool) (*BranchPlan, error) {
	var plan *BranchPlan
	if err := scanStoryEventsLocked(handle, func(record StoryEventRecord) error {
		if record.Envelope.Type != StoryEventTypeBranchPlanUpdated {
			return nil
		}
		var event BranchPlanUpdatedEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		if ancestorIDs[event.TurnID] {
			plan = &BranchPlan{Markdown: event.Markdown, UpdatedTurnID: event.TurnID, UpdatedAt: event.Ts}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return plan, nil
}

func (s *Store) stateForStoryAncestorsLocked(handle *storyJournalHandle, ancestorIDs map[string]bool) (map[string]any, error) {
	revisions := make(map[string]TurnStateRevisedEvent)
	if err := scanStoryEventsLocked(handle, func(record StoryEventRecord) error {
		if record.Envelope.Type != StoryEventTypeTurnStateRevised {
			return nil
		}
		var revision TurnStateRevisedEvent
		if err := mapToStruct(record.Raw, &revision); err != nil {
			return err
		}
		if ancestorIDs[revision.TurnID] {
			revisions[revision.TurnID] = revision
		}
		return nil
	}); err != nil {
		return nil, err
	}

	state := initialStoryState()
	if err := scanStoryEventsLocked(handle, func(record StoryEventRecord) error {
		if !ancestorIDs[record.Envelope.ID] {
			return nil
		}
		switch record.Envelope.Type {
		case StoryEventTypeStateDelta:
			var delta StateDeltaEvent
			if err := mapToStruct(record.Raw, &delta); err != nil {
				return err
			}
			applyStateDeltaToProjection(state, StateDelta{SchemaVersion: delta.SchemaVersion, Ops: delta.Ops, ActorOps: delta.ActorOps})
		case StoryEventTypeTurn:
			var turn TurnEvent
			if err := mapToStruct(record.Raw, &turn); err != nil {
				return err
			}
			if revision, ok := revisions[turn.ID]; ok {
				if revision.ClearStateDelta {
					turn.StateDelta = nil
				} else if revision.StateDelta != nil {
					value := *revision.StateDelta
					turn.StateDelta = &value
				}
			}
			applyTurnState(state, turn)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return state, nil
}

func scanStoryEventsLocked(handle *storyJournalHandle, visit func(StoryEventRecord) error) error {
	if handle == nil || handle.journal == nil {
		return fmt.Errorf("story journal is unavailable")
	}
	head := handle.journal.Head().Cursor
	for after := conversationjournal.Cursor(0); after < head; {
		records, err := handle.journal.ReadRange(context.Background(), conversationjournal.Range{
			After: after, Through: head, Limit: storyCheckpointScanTransactions,
		})
		if err != nil {
			return err
		}
		next := after
		for _, physical := range records {
			if physical.Location.Cursor > next {
				next = physical.Location.Cursor
			}
			_, events, decodeErr := decodeStoryProjectionPayload(physical.Payload)
			if decodeErr != nil {
				return decodeErr
			}
			for _, record := range events {
				if err := visit(record); err != nil {
					return err
				}
			}
		}
		if next <= after {
			return fmt.Errorf("story journal scan made no progress after cursor %d", after)
		}
		after = next
	}
	return nil
}
