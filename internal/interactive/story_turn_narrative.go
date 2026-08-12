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
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return UpdateTurnNarrativeResult{}, err
	}
	defer releaseStory()

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

	meta, lines, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return UpdateTurnNarrativeResult{}, err
	}
	branchID := strings.TrimSpace(req.BranchID)
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	_, ok := meta.Branches[branchID]
	if !ok {
		return UpdateTurnNarrativeResult{}, fmt.Errorf("分支不存在 / Branch does not exist: %s", branchID)
	}
	if err := requireLatestLogicalTurn(meta, lines, branchID, turnID); err != nil {
		return UpdateTurnNarrativeResult{}, err
	}

	snapshot, err := snapshotFromLines(storyID, branchID, meta, lines)
	if err != nil {
		return UpdateTurnNarrativeResult{}, err
	}
	turnOnPath := false
	for index := range snapshot.Turns {
		if snapshot.Turns[index].ID == turnID {
			turnOnPath = true
			break
		}
	}
	if !turnOnPath {
		return UpdateTurnNarrativeResult{}, fmt.Errorf("只能编辑当前剧情路径上的 AI 回复 / Only AI replies on the current story path can be edited: %s", turnID)
	}

	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != turnID {
		return UpdateTurnNarrativeResult{}, fmt.Errorf("回合不存在 / Turn does not exist: %s", turnID)
	}
	turn := *snapshot.CurrentTurn
	if req.ExpectedNarrative != nil && turn.Narrative != *req.ExpectedNarrative {
		return UpdateTurnNarrativeResult{}, fmt.Errorf("AI 回复已变化，请重新加载后再编辑 / AI reply changed; reload before editing")
	}
	if turn.Narrative == narrative {
		return UpdateTurnNarrativeResult{Turn: turn}, nil
	}

	turn.Narrative = narrative
	if turn.TerminalOutcome != nil && turn.TerminalOutcome.Terminal && turn.TerminalOutcome.CausedByTurnID == turn.ID {
		outcome := *turn.TerminalOutcome
		outcome.FinalNarrativeSummary = trimBytes(narrative, maxInteractiveTextBytes)
		turn.TerminalOutcome = &outcome
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	result := UpdateTurnNarrativeResult{Turn: turn}
	newEvents := []any{TurnNarrativeRevisedEvent{
		V: schemaVersion, Type: StoryEventTypeTurnNarrativeRevised, ID: newID("tnr"),
		ParentID: turnID, BranchID: branchID, Ts: now, TurnID: turnID, Narrative: narrative,
	}}

	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, newEvents...); err != nil {
		return UpdateTurnNarrativeResult{}, err
	}
	if err := s.syncStorySummaryLocked(storyID); err != nil {
		return UpdateTurnNarrativeResult{}, err
	}
	return result, nil
}

func normalizeEditableTurnNarrative(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.TrimSpace(value)
}
