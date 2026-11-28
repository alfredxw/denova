package session

import (
	"context"

	"denova/internal/agents/conversationjournal"
)

type historyAnchor struct {
	Location conversationjournal.Location `json:"location"`
}

func (projection *sessionJournalProjection) rememberHistoryRow(location conversationjournal.Location) {
	// The array position determines the visible row: index * anchor interval.
	if projection.HistoryCount%sessionHistoryAnchorEvery == 0 {
		projection.HistoryAnchors = append(projection.HistoryAnchors, historyAnchor{Location: location})
	}
	projection.HistoryCount++
}

func (projection *sessionJournalProjection) rememberDisplayPatch(target string, location conversationjournal.Location) {
	if projection.DisplayPatches == nil {
		projection.DisplayPatches = make(map[string][]conversationjournal.Location)
	}
	projection.DisplayPatches[target] = append(projection.DisplayPatches[target], location)
}

// Patches can arrive after a display window, including after a later user turn.
// Fetch only patches belonging to retained rows and apply each exactly once.
func (s *Session) patchHistoryPage(ctx context.Context, page *historyPageProjection) error {
	var locations []conversationjournal.Location
	for _, row := range page.rows.records {
		if row.display != nil {
			locations = append(locations, s.projection.DisplayPatches[row.journalID]...)
		}
	}
	if len(locations) == 0 {
		return nil
	}
	records, err := s.journal.ReadLocations(ctx, locations)
	if err != nil {
		return err
	}
	for _, record := range records {
		if err := appendConversationRecord(page.rows, record); err != nil {
			return err
		}
	}
	page.trim()
	return nil
}
