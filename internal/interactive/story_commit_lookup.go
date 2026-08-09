package interactive

import (
	"context"
	"fmt"
	"strings"

	"denova/internal/agents/conversationjournal"
)

// recentStoryCommitLocked resolves the only commit window that can still
// belong to an active runtime operation. Historical audit APIs may still scan
// the canonical journal, but normal crash reconciliation must never replay the
// complete game log merely to prove that a newly generated command ID is not
// present.
func (s *Store) recentStoryCommitLocked(
	storyID string,
	eventType string,
	identity DomainCommitIdentity,
) (StoryEventRecord, storyCommitLocator, bool, error) {
	handle, err := s.refreshStoryJournalLocked(strings.TrimSpace(storyID), false)
	if err != nil {
		return StoryEventRecord{}, storyCommitLocator{}, false, err
	}
	commandID := strings.TrimSpace(identity.CommandID)
	for index := len(handle.projection.RecentCommits) - 1; index >= 0; index-- {
		locator := handle.projection.RecentCommits[index]
		if locator.CommandID != commandID || locator.EventType != eventType {
			continue
		}
		if locator.OperationID != strings.TrimSpace(identity.OperationID) || locator.Cycle != identity.Cycle {
			return StoryEventRecord{}, locator, false, nil
		}
		after := conversationjournal.Cursor(0)
		if locator.Cursor > 1 {
			after = locator.Cursor - 1
		}
		physical, readErr := handle.journal.ReadRange(context.Background(), conversationjournal.Range{
			After: after, Through: locator.Cursor, Limit: 1,
		})
		if readErr != nil {
			return StoryEventRecord{}, locator, false, readErr
		}
		stats := StoryJournalReplayStats{BytesRead: handle.journal.ReplayStats().LastRangeBytesRead}
		seen := make(map[conversationjournal.Cursor]bool)
		for _, item := range physical {
			if !seen[item.Location.Cursor] {
				seen[item.Location.Cursor] = true
				stats.RecordsRead++
				stats.TransactionsRead++
			}
			_, events, decodeErr := decodeStoryProjectionPayload(item.Payload)
			if decodeErr != nil {
				return StoryEventRecord{}, locator, false, decodeErr
			}
			stats.EventsRead += int64(len(events))
			for _, event := range events {
				if event.Envelope.Type == locator.EventType && event.Envelope.ID == locator.EventID {
					s.rememberStoryReplayStats(storyID, stats)
					return event, locator, true, nil
				}
			}
		}
		return StoryEventRecord{}, locator, false, fmt.Errorf(
			"story commit locator is stale: type=%s event_id=%s cursor=%d",
			locator.EventType, locator.EventID, locator.Cursor,
		)
	}
	s.rememberStoryReplayStats(storyID, StoryJournalReplayStats{})
	return StoryEventRecord{}, storyCommitLocator{}, false, nil
}
