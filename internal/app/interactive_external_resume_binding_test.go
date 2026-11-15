package app

import (
	"strings"
	"testing"

	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"
)

// TestResolveGameResumeBinding covers the application-side binding resolution
// used before StartExternalTurn resumes an interrupted Game input.
func TestResolveGameResumeBinding(t *testing.T) {
	t.Run("empty resume id returns no binding", func(t *testing.T) {
		root := t.TempDir()
		store := interactive.NewStore(root)
		defer store.Close()
		story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Binding empty"})
		if err != nil {
			t.Fatal(err)
		}
		commitExternalGameInput(t, store, story.ID, "round-1")
		bound, err := resolveGameResumeBinding(store, story.ID, "main", "")
		if err != nil {
			t.Fatal(err)
		}
		if bound != agentrun.CommandID("") {
			t.Fatalf("bound = %q, want empty", bound)
		}
	})

	t.Run("interruption mismatch fails like StartExternalTurn", func(t *testing.T) {
		root := t.TempDir()
		store := interactive.NewStore(root)
		defer store.Close()
		story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Binding mismatch"})
		if err != nil {
			t.Fatal(err)
		}
		commitExternalGameInput(t, store, story.ID, "round-1")
		if _, err := resolveGameResumeBinding(store, story.ID, "main", "dangling"); err == nil || !strings.Contains(err.Error(), "interruption changed") {
			t.Fatalf("err = %v, want interruption changed", err)
		}
	})

	t.Run("resolves the continued input command", func(t *testing.T) {
		root := t.TempDir()
		store := interactive.NewStore(root)
		defer store.Close()
		story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Binding resume"})
		if err != nil {
			t.Fatal(err)
		}
		inputID := commitExternalGameInput(t, store, story.ID, "round-1")
		bound, err := resolveGameResumeBinding(store, story.ID, "main", inputID+"-interrupted")
		if err != nil {
			t.Fatal(err)
		}
		if bound != agentrun.CommandID("round-1") {
			t.Fatalf("bound = %q, want round-1", bound)
		}
	})

	t.Run("settled continued input fails like StartExternalTurn", func(t *testing.T) {
		root := t.TempDir()
		store := interactive.NewStore(root)
		defer store.Close()
		story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Binding missing input"})
		if err != nil {
			t.Fatal(err)
		}
		inputID := commitExternalGameInput(t, store, story.ID, "round-1")
		if err := store.AbandonPendingTurn(story.ID, "main", inputID); err != nil {
			t.Fatal(err)
		}
		// Settling the input removes its interruption, so the resume request
		// no longer matches the journal and must fail like StartExternalTurn.
		if _, err := resolveGameResumeBinding(store, story.ID, "main", inputID+"-interrupted"); err == nil || !strings.Contains(err.Error(), "interruption changed") {
			t.Fatalf("err = %v, want interruption changed", err)
		}
	})
}
