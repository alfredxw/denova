package app

import (
	"context"
	"testing"
)

// TestConversationProjectionWarningGuards pins the non-fatal warning contract:
// only an interactive binding with a story ID can consume a projection failure,
// and an absent signal always yields an empty diagnostic. The failure-injection
// path itself is covered by the interactive store regression test because the
// unexported store hook is unreachable from this package.
func TestConversationProjectionWarningGuards(t *testing.T) {
	if got := (*App)(nil).ConversationProjectionWarning(context.Background(), ConversationConfigBinding{
		Mode: ConversationModeInteractive, StoryID: "story-1",
	}); got != "" {
		t.Fatalf("nil app must return empty warning, got %q", got)
	}

	app := newExecutionProfileTestApp(t)

	// Writing and Agent Chat never surface a Game catalog projection signal.
	if got := app.ConversationProjectionWarning(context.Background(), ConversationConfigBinding{Mode: ConversationModeWriting, StoryID: "story-1"}); got != "" {
		t.Fatalf("writing mode must return empty warning, got %q", got)
	}

	// An interactive binding requires a story ID to scope consumption.
	if got := app.ConversationProjectionWarning(context.Background(), ConversationConfigBinding{Mode: ConversationModeInteractive}); got != "" {
		t.Fatalf("empty story id must return empty warning, got %q", got)
	}

	// A story without a recorded projection failure yields no warning, and a
	// nil request context must not panic the diagnostic path.
	interactive := ConversationConfigBinding{Mode: ConversationModeInteractive, StoryID: "story-without-failure"}
	if got := app.ConversationProjectionWarning(context.Background(), interactive); got != "" {
		t.Fatalf("story without failure must return empty warning, got %q", got)
	}
	if got := app.ConversationProjectionWarning(nil, interactive); got != "" {
		t.Fatalf("nil context must still return empty warning, got %q", got)
	}
}