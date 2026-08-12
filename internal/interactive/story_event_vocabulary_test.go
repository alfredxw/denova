package interactive

import (
	"strings"
	"testing"

	"denova/internal/agents/conversationjournal"
)

func TestPersistedStoryEventVocabularyClassifiesEveryEvent(t *testing.T) {
	expected := map[string]bool{
		StoryEventTypePlayerInput:          true,
		StoryEventTypeModelContextBatch:    true,
		StoryEventTypeTurn:                 true,
		StoryEventTypeStateDelta:           true,
		StoryEventTypeBranch:               true,
		StoryEventTypeHotChoices:           false,
		StoryEventTypeTurnVersionSelected:  true,
		StoryEventTypeTurnNarrativeRevised: true,
		StoryEventTypeTurnDisplayAppended:  false,
		StoryEventTypeTurnStateRevised:     true,
		StoryEventTypeStoryConfigUpdated:   true,
		StoryEventTypeBranchSwitched:       false,
		StoryEventTypeBranchArchived:       false,
		StoryEventTypeBranchHeadMoved:      true,
	}
	if len(persistedStoryEventModelContextChanges) != len(expected) {
		t.Fatalf("persisted event vocabulary size = %d, want %d", len(persistedStoryEventModelContextChanges), len(expected))
	}
	for eventType, changesModelContext := range expected {
		actual, err := storyEventChangesModelContext(eventType)
		if err != nil {
			t.Fatalf("classification %q: %v", eventType, err)
		}
		if actual != changesModelContext {
			t.Fatalf("event %q changes_model_context = %t, want %t", eventType, actual, changesModelContext)
		}
		if err := validateStoryEventEnvelope(StoryEventEnvelope{
			V: schemaVersion, Type: eventType, ID: "event", BranchID: "main", Ts: "2026-07-30T00:00:00Z",
		}); err != nil {
			t.Fatalf("envelope rejected registered event %q: %v", eventType, err)
		}
	}
}

func TestUnknownStoryEventFailsValidationAndProjectionWithoutMutation(t *testing.T) {
	const unknown = "future_unclassified_event"
	err := validateStoryEventEnvelope(StoryEventEnvelope{
		V: schemaVersion, Type: unknown, ID: "unknown", BranchID: "main", Ts: "2026-07-30T00:00:00Z",
	})
	if err == nil || !strings.Contains(err.Error(), "未知故事事件类型") {
		t.Fatalf("unknown envelope error = %v", err)
	}

	projection := newStoryJournalProjection("story", "generation")
	err = projection.applyEvent(1, StoryEventRecord{
		Envelope: StoryEventEnvelope{V: schemaVersion, Type: unknown, ID: "unknown", BranchID: "main", Ts: "2026-07-30T00:00:00Z"},
	})
	if err == nil || !strings.Contains(err.Error(), "未知故事事件类型") {
		t.Fatalf("unknown projection error = %v", err)
	}
	if projection.EventCount != 0 || len(projection.Branches) != 0 || projection.lastCursor != conversationjournal.Cursor(0) {
		t.Fatalf("unknown event mutated projection: %#v", projection)
	}
}
