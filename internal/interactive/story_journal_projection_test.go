package interactive

import (
	"errors"
	"testing"

	"denova/internal/agents/conversationjournal"
)

func TestStoryProjectionMarksVersionMismatchAsRebuildable(t *testing.T) {
	projection := newStoryJournalProjection("story-id", "generation-id")
	err := projection.Restore([]byte(`{"version":8}`))
	if !errors.Is(err, conversationjournal.ErrProjectionCheckpointIncompatible) {
		t.Fatalf("projection version error = %v", err)
	}
}
