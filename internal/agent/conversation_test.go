package agent

import (
	"testing"

	"github.com/cloudwego/eino/schema"

	"nova/internal/session"
)

func TestSessionConversationLimitsRecentUserTurns(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("default")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		if err := sess.Append(schema.UserMessage("user " + string(rune('0'+i)))); err != nil {
			t.Fatal(err)
		}
		if err := sess.Append(schema.AssistantMessage("assistant "+string(rune('0'+i)), nil)); err != nil {
			t.Fatal(err)
		}
	}
	conversation := NewSessionConversation(sess, WithSessionRecentTurns(2))
	history, err := conversation.PrepareMessages("user 5", "agent user 5")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Fatalf("history length = %d, want 3", len(history))
	}
	if history[0].Content != "user 4" || history[1].Content != "assistant 4" || history[2].Content != "agent user 5" {
		t.Fatalf("unexpected limited history: %#v", history)
	}
}
