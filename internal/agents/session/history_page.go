package session

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/conversationjournal"
)

// HistoryPage is a bounded chronological slice of the UI transcript. Before
// is a stable logical position: the next request supplies NextBefore.
type HistoryPage struct {
	Entries    []HistoryEntry
	NextBefore int
	HasMore    bool
	Total      int
}

// ReadHistoryPage reads only the sparse interval needed for one page. It does
// not materialize the complete session even when the canonical journal has
// hundreds of thousands of transactions.
func (s *Session) ReadHistoryPage(ctx context.Context, before, limit int) (HistoryPage, error) {
	if s == nil {
		return HistoryPage{}, fmt.Errorf("session is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 {
		return HistoryPage{}, fmt.Errorf("history page limit must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshCanonicalTailLocked(); err != nil {
		return HistoryPage{}, fmt.Errorf("refresh session history page: %w", err)
	}
	if s.journal == nil || s.projection == nil {
		return HistoryPage{}, fmt.Errorf("session history index is unavailable")
	}
	total := s.projection.HistoryCount
	end := total
	if before >= 0 {
		end = min(before, total)
	}
	requestedStart := max(0, end-limit)
	if requestedStart == end {
		return HistoryPage{Entries: []HistoryEntry{}, NextBefore: requestedStart, HasMore: requestedStart > 0, Total: total}, nil
	}
	anchor := historyAnchor{Before: 0, Cursor: 1}
	for _, candidate := range s.projection.HistoryAnchors {
		if candidate.Before > requestedStart {
			break
		}
		anchor = candidate
	}
	through := s.journal.Head().Cursor
	lookahead := end + sessionHistoryAnchorEvery
	for _, candidate := range s.projection.HistoryAnchors {
		if candidate.Before >= lookahead && candidate.Cursor > anchor.Cursor {
			through = candidate.Cursor - 1
			break
		}
	}
	records, err := s.journal.ReadRange(ctx, conversationjournal.Range{After: anchor.Cursor - 1, Through: through})
	if err != nil {
		return HistoryPage{}, fmt.Errorf("read session history range: %w", err)
	}
	temporary := &Session{
		ID: s.ID, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		title: s.title, journalIncarnation: s.journalIncarnation,
		partialMaterialization: true,
		messages:               make([]*agent.Message, 0), records: make([]historyRecord, 0),
	}
	for _, record := range records {
		if err := appendConversationRecord(temporary, record); err != nil {
			return HistoryPage{}, fmt.Errorf("project session history cursor %d: %w", record.Location.Cursor, err)
		}
	}
	entries := temporary.History()
	// The row limit is a paging target, not permission to split one Agent
	// turn. Display progress and tool events can make a single turn much larger
	// than the target, so align the page to the latest user/clear boundary at
	// or before it. The sparse anchor bounds the extra scan for normal turns
	// while still restoring an unusually large turn as one coherent unit.
	start := anchor.Before
	boundaryLimit := min(len(entries)-1, requestedStart-anchor.Before)
	for index := 0; index <= boundaryLimit; index++ {
		entry := entries[index]
		if entry.Role == string(agent.User) || entry.Type == historyTypeClear {
			start = anchor.Before + index
		}
	}
	from := max(0, start-anchor.Before)
	to := min(len(entries), end-anchor.Before)
	if from > to {
		from = to
	}
	pageEntries := append([]HistoryEntry(nil), entries[from:to]...)
	return HistoryPage{Entries: pageEntries, NextBefore: start, HasMore: start > 0, Total: total}, nil
}

// JournalReplayStats exposes complexity counters to tests and offline
// benchmarks without exposing the shared journal's physical index format.
func (s *Session) JournalReplayStats() conversationjournal.ReplayStats {
	if s == nil || s.journal == nil {
		return conversationjournal.ReplayStats{}
	}
	return s.journal.ReplayStats()
}
