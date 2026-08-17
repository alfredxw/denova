package interactive

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrHistoricalTurnRequiresBranch is returned when a mutation targets an
// older logical turn. Callers must explicitly fork at that turn before
// changing it so the canonical journal remains append-only and unambiguous.
var ErrHistoricalTurnRequiresBranch = errors.New("只能编辑当前分支最新一轮；请先从历史回合创建新分支 / Only the latest turn can be edited; create a branch from history first")

// projectStoryEventOverlays applies append-only side revisions to immutable
// base turns. The returned records are an in-memory view; canonical raw maps
// are never modified.
func projectStoryEventOverlays(lines []StoryEventRecord) ([]StoryEventRecord, error) {
	projected := make([]StoryEventRecord, len(lines))
	turnIndex := make(map[string]int)
	for index, record := range lines {
		clonedRaw := make(map[string]any, len(record.Raw))
		for key, value := range record.Raw {
			clonedRaw[key] = value
		}
		projected[index] = StoryEventRecord{Envelope: record.Envelope, Raw: clonedRaw}
		if record.Envelope.Type == StoryEventTypeTurn {
			turnIndex[record.Envelope.ID] = index
		}
	}
	for _, record := range lines {
		var targetID string
		switch record.Envelope.Type {
		case StoryEventTypeTurnNarrativeRevised:
			var revision TurnNarrativeRevisedEvent
			if err := mapToStruct(record.Raw, &revision); err != nil {
				return nil, err
			}
			targetID = strings.TrimSpace(revision.TurnID)
			index, ok := turnIndex[targetID]
			if !ok {
				return nil, fmt.Errorf("正文修订目标回合不存在: %s", targetID)
			}
			var turn TurnEvent
			if err := mapToStruct(projected[index].Raw, &turn); err != nil {
				return nil, err
			}
			turn.Narrative = revision.Narrative
			if turn.TerminalOutcome != nil && turn.TerminalOutcome.Terminal && turn.TerminalOutcome.CausedByTurnID == turn.ID {
				outcome := *turn.TerminalOutcome
				outcome.FinalNarrativeSummary = trimBytes(revision.Narrative, maxInteractiveTextBytes)
				turn.TerminalOutcome = &outcome
			}
			projected[index].Raw = storyTurnRaw(turn)
		case StoryEventTypeTurnDisplayAppended:
			var revision TurnDisplayAppendedEvent
			if err := mapToStruct(record.Raw, &revision); err != nil {
				return nil, err
			}
			targetID = strings.TrimSpace(revision.TurnID)
			index, ok := turnIndex[targetID]
			if !ok {
				return nil, fmt.Errorf("展示修订目标回合不存在: %s", targetID)
			}
			var turn TurnEvent
			if err := mapToStruct(projected[index].Raw, &turn); err != nil {
				return nil, err
			}
			turn.DisplayEvents = appendDisplayEvent(turn.DisplayEvents, revision.Display)
			projected[index].Raw = storyTurnRaw(turn)
		case StoryEventTypeTurnStateRevised:
			var revision TurnStateRevisedEvent
			if err := mapToStruct(record.Raw, &revision); err != nil {
				return nil, err
			}
			targetID = strings.TrimSpace(revision.TurnID)
			index, ok := turnIndex[targetID]
			if !ok {
				return nil, fmt.Errorf("状态修订目标回合不存在: %s", targetID)
			}
			var turn TurnEvent
			if err := mapToStruct(projected[index].Raw, &turn); err != nil {
				return nil, err
			}
			if revision.ClearStateDelta {
				turn.StateDelta = nil
			} else if revision.StateDelta != nil {
				value := *revision.StateDelta
				turn.StateDelta = &value
			}
			turn.StateStatus = revision.StateStatus
			turn.StateError = revision.StateError
			if revision.ClearRuleResolution {
				turn.RuleResolution = nil
			} else if revision.RuleResolution != nil {
				turn.RuleResolution = normalizeRuleResolutionPointer(revision.RuleResolution)
			}
			if revision.ClearTerminalOutcome {
				turn.TerminalOutcome = nil
			} else if revision.TerminalOutcome != nil {
				turn.TerminalOutcome = normalizeTerminalOutcomePointer(revision.TerminalOutcome)
			}
			projected[index].Raw = storyTurnRaw(turn)
		}
	}
	return projected, nil
}

func storyTurnRaw(turn TurnEvent) map[string]any {
	data, err := json.Marshal(turn)
	if err != nil {
		return map[string]any{}
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return map[string]any{}
	}
	return raw
}

func latestLogicalTurnID(meta StoryMeta, lines []StoryEventRecord, branchID string) string {
	branch, ok := meta.Branches[branchID]
	if !ok {
		return ""
	}
	if projected, err := projectStoryEventOverlays(lines); err == nil {
		lines = projected
	}
	events := eventsByID(lines)
	return nearestTurnAncestor(branch.Head, events)
}

func requireLatestLogicalTurn(meta StoryMeta, lines []StoryEventRecord, branchID, turnID string) error {
	latest := latestLogicalTurnID(meta, lines, branchID)
	if latest == "" || strings.TrimSpace(turnID) != latest {
		return ErrHistoricalTurnRequiresBranch
	}
	return nil
}
