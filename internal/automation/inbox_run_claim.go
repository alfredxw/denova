package automation

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type InboxRunClaimDisposition string

const (
	InboxRunClaimed   InboxRunClaimDisposition = "claimed"
	InboxRunResumed   InboxRunClaimDisposition = "resumed"
	InboxRunCompleted InboxRunClaimDisposition = "completed"
)

// InboxConfirmationRunID derives the only run identity that may execute one
// inbox action. Immutable inbox semantics, not request timing, own the ID.
func InboxConfirmationRunID(item TriggerInboxItem) (string, error) {
	if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.TaskID) == "" || strings.TrimSpace(item.Fingerprint) == "" {
		return "", fmt.Errorf("complete inbox action identity is required")
	}
	return deterministicTriggerID("run", "inbox-confirmation", item.ID, triggerInboxIntentHash(item), strings.TrimSpace(item.SourceRunID)), nil
}

// ClaimInboxRun atomically binds a pending inbox action to its deterministic
// run before Agent admission. Replays may resume only the exact same run.
func (s *Store) ClaimInboxRun(ctx context.Context, id, runID string) (TriggerInboxItem, InboxRunClaimDisposition, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return TriggerInboxItem{}, "", fmt.Errorf("deterministic inbox run id is required")
	}
	disposition := InboxRunClaimed
	item, err := s.mutateInboxItem(ctx, id, func(item TriggerInboxItem, _ time.Time) (TriggerInboxItem, bool, error) {
		switch item.Status {
		case InboxStatusPending:
			if item.RunID == "" {
				item.RunID = runID
				return item, true, nil
			}
			if item.RunID != runID {
				return TriggerInboxItem{}, false, fmt.Errorf("%w: inbox_id=%s run identity differs", ErrTriggerActionConflict, item.ID)
			}
			disposition = InboxRunResumed
			return item, false, nil
		case InboxStatusConfirmed:
			if item.RunID != runID {
				return TriggerInboxItem{}, false, fmt.Errorf("%w: inbox_id=%s confirmed run differs", ErrTriggerActionConflict, item.ID)
			}
			disposition = InboxRunCompleted
			return item, false, nil
		default:
			return TriggerInboxItem{}, false, fmt.Errorf("automation inbox item %s cannot claim a run from status %s", item.ID, item.Status)
		}
	})
	return item, disposition, err
}

// CompleteInboxRun advances only the exact claimed run to confirmed. It is an
// exact replay when the same receipt was already committed.
func (s *Store) CompleteInboxRun(ctx context.Context, id, runID string) (TriggerInboxItem, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return TriggerInboxItem{}, fmt.Errorf("deterministic inbox run id is required")
	}
	return s.mutateInboxItem(ctx, id, func(item TriggerInboxItem, now time.Time) (TriggerInboxItem, bool, error) {
		if item.RunID != runID {
			return TriggerInboxItem{}, false, fmt.Errorf("%w: inbox_id=%s completion run differs", ErrTriggerActionConflict, item.ID)
		}
		switch item.Status {
		case InboxStatusConfirmed:
			return item, false, nil
		case InboxStatusPending:
			item.Status = InboxStatusConfirmed
			item.HandledAt = &now
			if item.ReadAt == nil {
				item.ReadAt = &now
			}
			return item, true, nil
		default:
			return TriggerInboxItem{}, false, fmt.Errorf("automation inbox item %s cannot complete a run from status %s", item.ID, item.Status)
		}
	})
}
