package session

import (
	"context"
	"fmt"

	"denova/internal/agents/conversationjournal"

	agentschema "github.com/alfredxw/denova/agent/schema"
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
	temporary := &Session{
		ID: s.ID, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		title: s.title, journalIncarnation: s.journalIncarnation,
		partialMaterialization: true,
		messages:               make([]*agentschema.Message, 0), records: make([]historyRecord, 0),
	}
	projection := historyPageProjection{
		rows: temporary, index: newSessionJournalProjection(s.ID, s.journalIncarnation),
		start: requestedStart, end: end, maxRows: max(limit, 2*sessionHistoryAnchorEvery),
	}
	projection.index.HistoryCount = anchor.Before
	if err := s.journal.VisitRange(ctx, conversationjournal.Range{After: anchor.Cursor - 1, Through: through}, projection.apply); err != nil {
		return HistoryPage{}, fmt.Errorf("project session history range: %w", err)
	}
	pageEntries := temporary.History()
	start := end
	if len(projection.positions) > 0 {
		start = projection.positions[0]
	}
	if err := applyJournalAskAnswers(pageEntries, &s.projection.AgentSessions, s.journal); err != nil {
		return HistoryPage{}, err
	}
	if err := s.applyExternalHistoryOutcomesLocked(ctx, pageEntries); err != nil {
		return HistoryPage{}, err
	}
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
