package interactive

import (
	"errors"
	"testing"

	"denova/config"
	"denova/internal/agents/conversationconfig"
)

func TestStoryBranchRuntimeConfigCopiesPersistsAndRejectsStaleWrites(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	seed := conversationconfig.Config{
		AgentKind: config.AgentKindInteractiveStory, ProfileID: "default", ThinkingLevel: "off",
		ApprovalMode: config.AgentApprovalWrite,
	}
	story, err := store.CreateStory(CreateStoryRequest{Title: "Config story", RuntimeConfig: &seed})
	if err != nil {
		t.Fatal(err)
	}
	initial, ok, err := store.BranchRuntimeConfig(story.ID, "main")
	if err != nil || !ok || initial.Revision != 1 || initial.Config != seed {
		t.Fatalf("initial branch runtime config = %#v, present=%v, err=%v", initial, ok, err)
	}

	configured := seed
	configured.ThinkingLevel = "high"
	configured.ApprovalMode = config.AgentApprovalFullAccess
	saved, err := store.SetBranchRuntimeConfig(story.ID, "main", configured, initial.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetBranchRuntimeConfig(story.ID, "main", seed, initial.Revision); !errors.Is(err, conversationconfig.ErrRevisionConflict) {
		t.Fatalf("stale branch writer should conflict, got %v", err)
	}

	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{BranchID: "main", User: "Go", Narrative: "Done"})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{ParentEventID: turn.ID, Title: "Fork"})
	if err != nil {
		t.Fatal(err)
	}
	inherited, ok, err := store.BranchRuntimeConfig(story.ID, branch.ID)
	if err != nil || !ok {
		t.Fatalf("forked branch runtime config missing: %#v, present=%v, err=%v", inherited, ok, err)
	}
	if inherited.Revision != 1 || inherited.Config != saved.Config {
		t.Fatalf("forked branch did not copy its source selection: %#v", inherited)
	}

	reopened := NewStore(root)
	restored, ok, err := reopened.BranchRuntimeConfig(story.ID, branch.ID)
	if err != nil || !ok || restored != inherited {
		t.Fatalf("restored branch runtime config = %#v, present=%v, err=%v", restored, ok, err)
	}
}

func TestRecentRuntimeConfigUsesMostRecentlyUpdatedStory(t *testing.T) {
	store := NewStore(t.TempDir())
	first := conversationconfig.Config{
		AgentKind: config.AgentKindInteractiveStory, ProfileID: "default", ThinkingLevel: "low",
		ApprovalMode: config.AgentApprovalWrite,
	}
	firstStory, err := store.CreateStory(CreateStoryRequest{Title: "First", RuntimeConfig: &first})
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.ThinkingLevel = "max"
	second.ApprovalMode = config.AgentApprovalAsk
	secondStory, err := store.CreateStory(CreateStoryRequest{Title: "Second", RuntimeConfig: &second})
	if err != nil {
		t.Fatal(err)
	}

	recent, ok, err := store.RecentRuntimeConfig("")
	if err != nil || !ok || recent != second {
		t.Fatalf("recent Game config = %#v, present=%v, err=%v", recent, ok, err)
	}
	previous, ok, err := store.RecentRuntimeConfig(secondStory.ID)
	if err != nil || !ok || previous != first {
		t.Fatalf("excluded recent Game config = %#v, present=%v, err=%v", previous, ok, err)
	}
	if firstStory.ID == secondStory.ID {
		t.Fatal("story IDs should be distinct")
	}
}
