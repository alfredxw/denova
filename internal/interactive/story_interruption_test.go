package interactive

import "testing"

func TestStoryTurnInterruptionPersistsForPendingPlayerInput(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	story, err := store.CreateStory(CreateStoryRequest{Title: "Paused story"})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := NewPlayerInputIntent(DomainCommitIdentity{
		CommandID: "pause-command", OperationID: "pause-operation", Cycle: 1,
	}, "main", "Open the door")
	if err != nil {
		t.Fatal(err)
	}
	input, err := store.CommitPlayerInput(story.ID, intent)
	if err != nil {
		t.Fatal(err)
	}
	interruption, err := store.MarkTurnInterrupted(
		story.ID,
		"main",
		input.Event.ID,
		"Open the door",
		"The hinges groaned",
		"user_requested",
	)
	if err != nil {
		t.Fatal(err)
	}
	if interruption.PlayerInputID != input.Event.ID {
		t.Fatalf("interruption = %#v", interruption)
	}

	cold := NewStore(dir)
	pending, err := cold.PendingTurnInterruption(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil || pending.ID != interruption.ID || pending.AssistantContent != "The hinges groaned" {
		t.Fatalf("pending interruption after reload = %#v", pending)
	}
	if _, _, err := cold.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: intent.Text, Narrative: "The door opened.",
		AgentCommandID: intent.Identity.CommandID, AgentOperationID: intent.Identity.OperationID, AgentCycle: intent.Identity.Cycle,
	}); err != nil {
		t.Fatal(err)
	}
	pending, err = cold.PendingTurnInterruption(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if pending != nil {
		t.Fatalf("completed turn retained interruption = %#v", pending)
	}
}
