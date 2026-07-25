package app

import (
	"fmt"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents"
	"denova/internal/interactive"
)

func TestInteractiveConversationModelHistoryIsIndependentFromDisplayPage(t *testing.T) {
	workspace := t.TempDir()
	novaDir := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{
		Title: "模型历史投影", StoryTellerID: "classic", ReplyTargetChars: 700,
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 105; index++ {
		if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
			User: fmt.Sprintf("第%d次行动", index), Narrative: fmt.Sprintf("第%d段剧情", index),
		}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 100 || snapshot.Turns[0].User != "第6次行动" {
		t.Fatalf("expected bounded display page, got first=%q turns=%d", snapshot.Turns[0].User, len(snapshot.Turns))
	}

	conversation := newInteractiveConversation(store, novaDir, workspace, story.ID, "main", "继续", story.ReplyTargetChars, &config.Config{})
	history, err := assembleAndCommitInteractiveContextForTest(conversation, "继续", "继续")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 211 {
		t.Fatalf("model messages = %d, want 105 user/assistant pairs plus current input", len(history))
	}
	if history[0].Content != "第1次行动" || history[208].Content != "第105次行动" || history[209].Content != "第105段剧情" {
		t.Fatalf("model context did not preserve the canonical history outside the display page: first=%q tail=%q/%q", history[0].Content, history[208].Content, history[209].Content)
	}
}

func TestInteractiveMessageListSummaryBoundsLongHistory(t *testing.T) {
	messages := make([]*agents.Message, 20)
	for index := range messages {
		messages[index] = agents.UserMessage(fmt.Sprintf("message-%02d", index))
	}

	summary := interactiveMessageListSummary(messages)
	if !strings.Contains(summary, "count=20") || !strings.Contains(summary, "omitted=12") {
		t.Fatalf("long message summary did not report its bounded shape: %s", summary)
	}
	if strings.Contains(summary, "message-10") || !strings.Contains(summary, "message-00") || !strings.Contains(summary, "message-19") {
		t.Fatalf("long message summary did not retain only useful edges: %s", summary)
	}
}
