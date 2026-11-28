package session

import (
	"context"
	"errors"
	"fmt"

	"denova/internal/agents/conversationjournal"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

var errHistoryWindowComplete = errors.New("history window complete")

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
	return s.readHistoryPageLocked(ctx, before, limit)
}

// The caller owns the projection. Search opens a read snapshot without
// materializing an execution Session or replaying its model context.
func (s *Session) readHistoryPageLocked(ctx context.Context, before, limit int) (HistoryPage, error) {
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
	anchors := s.projection.HistoryAnchors
	anchorIndex := requestedStart / sessionHistoryAnchorEvery
	anchor := anchors[anchorIndex]
	temporary := &Session{
		ID: s.ID, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		title: s.title, journalIncarnation: s.journalIncarnation,
		partialMaterialization: true,
		messages:               make([]*agentschema.Message, 0), records: make([]historyRecord, 0),
	}
	projection := historyPageProjection{
		rows: temporary, position: anchorIndex * sessionHistoryAnchorEvery,
		start: requestedStart, end: end, maxRows: max(limit, 2*sessionHistoryAnchorEvery),
	}
	err := s.journal.VisitFrom(ctx, anchor.Location, s.journal.Head().Cursor, func(record conversationjournal.Record) error {
		if err := projection.apply(record); err != nil {
			return err
		}
		if projection.position >= end {
			return errHistoryWindowComplete
		}
		return nil
	})
	if err != nil && !errors.Is(err, errHistoryWindowComplete) {
		return HistoryPage{}, fmt.Errorf("project session history range: %w", err)
	}
	if err := s.patchHistoryPage(ctx, &projection); err != nil {
		return HistoryPage{}, fmt.Errorf("patch session history page: %w", err)
	}
	pageEntries := temporary.History()
	start := end
	if len(projection.positions) > 0 {
		start = projection.positions[0]
	}
	if err := applyJournalAskAnswers(pageEntries, &s.projection.AgentSessions, s.journal); err != nil {
		return HistoryPage{}, err
	}
	s.hydrateExternalAskCommandIDsLocked(pageEntries)
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
