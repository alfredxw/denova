package interactive

import (
	"fmt"
	"sort"
	"strings"
)

func snapshotFromLines(storyID, branchID string, meta StoryMeta, lines []StoryEventRecord) (Snapshot, error) {
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return Snapshot{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	state := initialStoryState()
	snapshot := Snapshot{StoryID: storyID, BranchID: branchID, State: state}
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
			}
		case StoryEventTypeStateDelta:
			var delta StateDeltaEvent
			if err := mapToStruct(record.Raw, &delta); err != nil {
				return Snapshot{}, err
			}
			for _, op := range delta.Ops {
				applyStateOp(state, op)
			}
		case StoryEventTypeCompaction:
			var compaction ContextCompactionEvent
			if err := mapToStruct(record.Raw, &compaction); err != nil {
				return Snapshot{}, err
			}
			snapshot.ContextCompaction = &compaction
		case StoryEventTypeCompactionRemoved:
			var removal ContextCompactionRemovalEvent
			if err := mapToStruct(record.Raw, &removal); err != nil {
				return Snapshot{}, err
			}
			snapshot.ContextCompaction = nil
			snapshot.ContextCompactionRemoval = &removal
		}
	}
	snapshot.Graph = buildStoryGraph(meta, lines, eventsByID, pathSet)
	return snapshot, nil
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
		"on_stage":    []any{},
		"characters":  map[string]any{},
		"events":      []any{},
		"scene":       map[string]any{},
		"inventory":   map[string]any{},
		"resources":   map[string]any{},
		"world_flags": []any{},
		"rules":       []any{},
		"threads":     []any{},
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
		if choice == "" || seen[choice] {
			continue
		}
		choices = append(choices, choice)
		seen[choice] = true
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
		nodes = append(nodes, PlotNode{
			ID:       turn.ID,
			ParentID: parentID,
			BranchID: turn.BranchID,
			Title:    compactText(turn.User, 24),
			Summary:  compactText(turn.Narrative, 72),
			Ts:       turn.Ts,
			Current:  currentPath[turn.ID],
			Head:     headTurns[turn.ID],
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

func nextContextCompactionEpoch(lines []StoryEventRecord, head string) int {
	events := eventsByID(lines)
	path, _ := eventPath(head, events)
	epoch := 0
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeCompaction {
			continue
		}
		var compaction ContextCompactionEvent
		if err := mapToStruct(record.Raw, &compaction); err != nil {
			continue
		}
		if compaction.Epoch > epoch {
			epoch = compaction.Epoch
		}
	}
	return epoch + 1
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
