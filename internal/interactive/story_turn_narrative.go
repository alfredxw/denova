package interactive

import (
	"fmt"
	"strings"
	"time"
)

const maxEditableTurnNarrativeBytes = 512 * 1024

// UpdateTurnNarrative corrects a turn's prose without regenerating the turn or
// changing its stable ID, state settlement, choices, images, or descendants.
func (s *Store) UpdateTurnNarrative(storyID string, req UpdateTurnNarrativeRequest) (UpdateTurnNarrativeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	turnID := strings.TrimSpace(req.TurnID)
	if turnID == "" {
		return UpdateTurnNarrativeResult{}, fmt.Errorf("回合 ID 不能为空 / Turn ID is required")
	}
	narrative := normalizeEditableTurnNarrative(req.Narrative)
	if narrative == "" {
		return UpdateTurnNarrativeResult{}, fmt.Errorf("AI 回复不能为空 / AI reply cannot be empty")
	}
	if len([]byte(narrative)) > maxEditableTurnNarrativeBytes {
		return UpdateTurnNarrativeResult{}, fmt.Errorf("AI 回复超过 %d bytes / AI reply exceeds %d bytes", maxEditableTurnNarrativeBytes, maxEditableTurnNarrativeBytes)
	}

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return UpdateTurnNarrativeResult{}, err
	}
	branchID := strings.TrimSpace(req.BranchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return UpdateTurnNarrativeResult{}, fmt.Errorf("分支不存在 / Branch does not exist: %s", branchID)
	}

	snapshot, err := snapshotFromLines(storyID, branchID, meta, lines)
	if err != nil {
		return UpdateTurnNarrativeResult{}, err
	}
	turnIndex := -1
	for index := range snapshot.Turns {
		if snapshot.Turns[index].ID == turnID {
			turnIndex = index
			break
		}
	}
	if turnIndex < 0 {
		return UpdateTurnNarrativeResult{}, fmt.Errorf("只能编辑当前剧情路径上的 AI 回复 / Only AI replies on the current story path can be edited: %s", turnID)
	}

	var turn TurnEvent
	var turnRaw map[string]any
	for index := range lines {
		if lines[index].Envelope.Type != StoryEventTypeTurn || lines[index].Envelope.ID != turnID {
			continue
		}
		if err := mapToStruct(lines[index].Raw, &turn); err != nil {
			return UpdateTurnNarrativeResult{}, err
		}
		turnRaw = lines[index].Raw
		break
	}
	if turnRaw == nil {
		return UpdateTurnNarrativeResult{}, fmt.Errorf("回合不存在 / Turn does not exist: %s", turnID)
	}
	if req.ExpectedNarrative != nil && turn.Narrative != *req.ExpectedNarrative {
		return UpdateTurnNarrativeResult{}, fmt.Errorf("AI 回复已变化，请重新加载后再编辑 / AI reply changed; reload before editing")
	}
	if turn.Narrative == narrative {
		return UpdateTurnNarrativeResult{Turn: turn}, nil
	}

	turn.Narrative = narrative
	turnRaw["narrative"] = narrative
	if turn.TerminalOutcome != nil && turn.TerminalOutcome.Terminal && turn.TerminalOutcome.CausedByTurnID == turn.ID {
		outcome := *turn.TerminalOutcome
		outcome.FinalNarrativeSummary = trimBytes(narrative, maxInteractiveTextBytes)
		turn.TerminalOutcome = &outcome
		turnRaw["terminal_outcome"] = outcome
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	result := UpdateTurnNarrativeResult{Turn: turn}
	newEvents := []any(nil)
	if compaction := snapshot.ContextCompaction; compaction != nil && turnIndex < compaction.SourceTurnCount {
		removal := ContextCompactionRemovalEvent{
			V:               schemaVersion,
			Type:            StoryEventTypeCompactionRemoved,
			ID:              newID("ccr"),
			ParentID:        branch.Head,
			BranchID:        branchID,
			Ts:              now,
			AgentKind:       compaction.AgentKind,
			CompactionID:    compaction.ID,
			SourceTurnCount: compaction.SourceTurnCount,
			Reason:          "turn_narrative_edited",
		}
		branch.Head = removal.ID
		meta.Branches[branchID] = branch
		newEvents = append(newEvents, removal)
		result.ContextCompactionInvalidated = true
	}

	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines, newEvents...); err != nil {
		return UpdateTurnNarrativeResult{}, err
	}
	if err := s.touchIndexLocked(storyID, now, len(newEvents)); err != nil {
		return UpdateTurnNarrativeResult{}, err
	}
	return result, nil
}

func normalizeEditableTurnNarrative(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.TrimSpace(value)
}
