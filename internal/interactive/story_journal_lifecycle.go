package interactive

import (
	"errors"
	"strings"
)

// Close flushes all rebuildable story indexes. A Store can be used again
// after Close; the next operation reopens only the requested story.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var result error
	for storyID := range s.storyJournals {
		result = errors.Join(result, s.evictStoryJournalLocked(storyID))
	}
	return result
}

func (s *Store) evictStoryJournalLocked(storyID string) error {
	if s.storyJournals == nil {
		return nil
	}
	storyID = strings.TrimSpace(storyID)
	handle := s.storyJournals[storyID]
	delete(s.storyJournals, storyID)
	if handle == nil || handle.journal == nil {
		return nil
	}
	return handle.journal.Close()
}
