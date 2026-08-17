package app

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/interactive"
	"denova/internal/session"
)

func TestInteractiveConversationPersistsTokenUsageContextWindow(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title:         "上下文用量",
		StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "继续", 800, nil)
	if err := conversation.AppendDisplayEvent(session.DisplayEvent{
		ID:                  "run-context-window",
		Role:                "token_usage",
		RunID:               "run-context-window",
		AgentKind:           "interactive_story",
		ContextWindowTokens: 400_000,
		ContextPromptTokens: 1200,
		PromptTokens:        1200,
		TotalTokens:         1280,
		ModelCalls:          1,
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.TokenUsageEvents) != 1 || snapshot.TokenUsageEvents[0].ContextWindowTokens != 400_000 || snapshot.TokenUsageEvents[0].ContextPromptTokens != 1200 {
		t.Fatalf("context window was not preserved in token usage sidecar: %#v", snapshot.TokenUsageEvents)
	}
}

func TestInteractiveConversationKeepsContextWindowWhenRemovedCompactionSkips(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "上下文用量", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{BranchID: "main", User: "继续", Narrative: "剧情继续。"}); err != nil {
		t.Fatal(err)
	}
	compaction, err := store.AppendContextCompaction(story.ID, "main", interactive.ContextCompactionEvent{
		AgentKind:       config.AgentKindInteractiveStory,
		Summary:         "较早剧情摘要",
		SourceTurnCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendContextCompactionRemoval(story.ID, "main", interactive.ContextCompactionRemovalEvent{
		AgentKind:       config.AgentKindInteractiveStory,
		CompactionID:    compaction.ID,
		SourceTurnCount: 1,
		Reason:          "user_removed",
	}); err != nil {
		t.Fatal(err)
	}

	const contextWindowTokens = 123_456
	conversation := newInteractiveConversation(store, t.TempDir(), workspace, story.ID, "main", "继续", 800, &config.Config{
		OpenAIContextWindowTokens: contextWindowTokens,
	})
	messages := []*schema.Message{schema.UserMessage("继续")}
	compacted, result, err := conversation.CompactContextIfNeeded(context.Background(), agent.ContextCompactionInput{Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	if result.SkippedReason != "removed_same_source" || result.ContextWindowTokens != contextWindowTokens {
		t.Fatalf("skip result must retain the model context window: %#v", result)
	}
	if len(compacted) != len(messages) || compacted[0] != messages[0] {
		t.Fatalf("skipped compaction should preserve input messages: %#v", compacted)
	}
}
