package conversation

import (
	"testing"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

func TestModelOnlyContinuationIsPersistedOutsideVisibleHistory(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("hidden-continuation")
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversation(sess).WithInputVisibility(agentrun.InputModelOnly)
	messages, err := assembleAndCommitModelContextForTest(conversation, "continue active goal", "continue active goal")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Content != "continue active goal" {
		t.Fatalf("model-only input missing from assembled context: %#v", messages)
	}
	if history := sess.History(); len(history) != 0 {
		t.Fatalf("model-only input leaked into visible history: %#v", history)
	}
	effective := sess.GetEffectiveMessages()
	if len(effective) != 1 || effective[0].Content != "continue active goal" {
		t.Fatalf("model-only input missing from canonical context: %#v", effective)
	}
}
