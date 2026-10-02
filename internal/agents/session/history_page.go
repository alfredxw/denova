package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

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
	page, err := s.readHistoryWindowLocked(ctx, end, requestedStart, total, historyAnchorsAny)
	if errors.Is(err, errTurnAnchorReplayDiverged) {
		slog.Warn("History turn anchor replay diverged; replaying from the turn boundary",
			"session_id", s.ID, "before", end, "error", err)
		page, err = s.readHistoryWindowLocked(ctx, end, requestedStart, total, historyAnchorsTurnBoundary)
	}
	return page, err
}

// historyAnchorSelection names which sparse anchors a history window may
// replay from.
type historyAnchorSelection int

const (
	// historyAnchorsAny also uses Turn anchors, bounding the replay of a long
	// Agent turn to roughly one anchor interval plus the page.
	historyAnchorsAny historyAnchorSelection = iota
	// historyAnchorsTurnBoundary replays from complete turn state only; it is
	// the verified fallback when a Turn anchor replay diverges from the index.
	historyAnchorsTurnBoundary
)

var errTurnAnchorReplayDiverged = errors.New("history turn anchor replay diverged from the index")

func (s *Session) readHistoryWindowLocked(ctx context.Context, end, requestedStart, total int, selection historyAnchorSelection) (HistoryPage, error) {
	anchor := historyAnchor{Before: 0, Cursor: 1}
	for _, candidate := range s.projection.HistoryAnchors {
		if candidate.Before > requestedStart {
			break
		}
		if candidate.Turn && selection == historyAnchorsTurnBoundary {
			continue
		}
		anchor = candidate
	}
	through := s.journal.Head().Cursor
	expectedRows := total - anchor.Before
	lookahead := end + sessionHistoryAnchorEvery
	for _, candidate := range s.projection.HistoryAnchors {
		if candidate.Before >= lookahead && candidate.Cursor > anchor.Cursor {
			through = candidate.Cursor - 1
			expectedRows = candidate.Before - anchor.Before
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
		partialMaterialization: true, replayStartsInsideTurn: anchor.Turn,
		messages: make([]*agent.Message, 0), records: make([]historyRecord, 0),
	}
	for _, record := range records {
		if err := appendConversationRecord(temporary, record); err != nil {
			if anchor.Turn {
				return HistoryPage{}, fmt.Errorf("%w: project cursor %d: %v", errTurnAnchorReplayDiverged, record.Location.Cursor, err)
			}
			return HistoryPage{}, fmt.Errorf("project session history cursor %d: %w", record.Location.Cursor, err)
		}
	}
	entries := temporary.History()
	// Row positions come from the index; a Turn anchor replay must reproduce
	// them exactly or later pages would skip or repeat rows.
	if anchor.Turn && len(entries) != expectedRows {
		return HistoryPage{}, fmt.Errorf("%w: replayed %d rows from position %d, index expects %d",
			errTurnAnchorReplayDiverged, len(entries), anchor.Before, expectedRows)
	}
	// The row limit is a paging target, not permission to split an ordinary
	// Agent turn. Display progress and tool events can make a single turn much
	// larger than the target, so align the page to the latest user/clear
	// boundary at or before it. A turn too long to have one within the anchor
	// interval is split at the requested row instead: restoring it whole would
	// make every page as expensive as the turn itself.
	start := anchor.Before
	if anchor.Turn {
		start = requestedStart
	}
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
