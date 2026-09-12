package app

import (
	"context"
	"testing"

	"denova/internal/interactive"
)

func TestManualContextRemovalPreparesFreshWritingAndGameSessions(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	ctx := context.Background()
	// The application drains and closes the previous Agent binding before each
	// maintenance operation, including an operation with no checkpoint to remove.
	removed, err := application.RemoveContextCompactionCommand(ctx, "remove-empty-writing-checkpoint")
	if err != nil || removed {
		t.Fatalf("fresh writing maintenance: removed=%t error=%v", removed, err)
	}
	story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{Title: "Fresh game maintenance", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	removed, err = application.RemoveInteractiveContextCompactionCommand(ctx, story.ID, "main", "remove-empty-game-checkpoint")
	if err != nil || removed {
		t.Fatalf("fresh game maintenance: removed=%t error=%v", removed, err)
	}
	snapshot, err := application.interactive.Snapshot(story.ID, "main")
	if err != nil || len(snapshot.Turns) != 0 {
		t.Fatalf("maintenance created an unexpected game turn: turns=%d error=%v", len(snapshot.Turns), err)
	}
}
