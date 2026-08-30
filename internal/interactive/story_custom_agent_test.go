package interactive

import (
	"testing"

	"denova/config"
	"denova/internal/agents/conversationconfig"
)

func TestCreateBranchPersistsSelectedCustomAgent(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "Agent branch", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{
		BranchID: "main", User: "Take the left path.", Narrative: "The corridor bends into darkness.",
	})
	if err != nil {
		t.Fatal(err)
	}
	selection := conversationconfig.Config{
		AgentKind: config.AgentKindInteractiveStory, CustomAgentID: "dramatic-narrator",
		ProfileID: "default", ThinkingLevel: "medium", ApprovalMode: config.AgentApprovalAsk,
	}
	branch, err := store.CreateBranch(story.ID, CreateBranchRequest{
		ParentEventID: turn.ID, Title: "Dramatic route", RuntimeConfig: &selection,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, ok, err := store.BranchRuntimeConfig(story.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || snapshot.Revision != 1 || snapshot.Config != selection {
		t.Fatalf("branch runtime config = %#v, present=%v", snapshot, ok)
	}
}
