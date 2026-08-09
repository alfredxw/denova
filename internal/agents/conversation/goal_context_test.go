package conversation

import (
	"context"
	"strings"
	"testing"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

func TestActiveGoalContextIsEscapedAndOmittedWhenPaused(t *testing.T) {
	ctx := context.Background()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("goal-context")
	if err != nil {
		t.Fatal(err)
	}
	current, err := sess.SetGoal(ctx, `Ship <complete> & "verified"`, 0)
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversation(sess)
	fragment, err := conversation.goalContextFragment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fragment == nil || fragment.Source != "session.goal" || fragment.Purpose == "" {
		t.Fatalf("active goal fragment = %#v", fragment)
	}
	if !strings.Contains(fragment.Content, "Ship &lt;complete&gt; &amp; &#34;verified&#34;") || strings.Contains(fragment.Content, "Ship <complete>") {
		t.Fatalf("goal objective was not XML-escaped: %q", fragment.Content)
	}
	if _, err := sess.PauseGoal(ctx, current.Revision); err != nil {
		t.Fatal(err)
	}
	fragment, err = conversation.goalContextFragment(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fragment != nil {
		t.Fatalf("paused goal must not enter model context: %#v", fragment)
	}
}

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
