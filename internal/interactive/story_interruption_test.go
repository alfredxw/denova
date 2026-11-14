package interactive

import (
	"reflect"
	"testing"
)

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

func TestAbandonedGameInputKeepsStoryAndModelHistoryIntact(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	story, err := store.CreateStory(CreateStoryRequest{Title: "Aborted story"})
	if err != nil { t.Fatal(err) }
	baseline, err := store.Snapshot(story.ID, "main")
	if err != nil { t.Fatal(err) }
	intent, err := NewPlayerInputIntent(DomainCommitIdentity{
		CommandID: "abort-command", OperationID: "external-game-abort-operation", Cycle: 1,
	}, "main", "Open the door")
	if err != nil { t.Fatal(err) }
	input, err := store.CommitPlayerInput(story.ID, intent)
	if err != nil { t.Fatal(err) }
	if _, err := store.MarkTurnInterrupted(story.ID, "main", input.Event.ID, intent.Text, "Half a scene", "connection_lost"); err != nil { t.Fatal(err) }
	if err := store.AbandonPendingTurn(story.ID, "main", input.Event.ID); err != nil { t.Fatal(err) }
	if err := store.AbandonPendingTurn(story.ID, "main", input.Event.ID); err != nil { t.Fatalf("abort must be idempotent: %v", err) }
	cold := NewStore(dir)
	snapshot, err := cold.Snapshot(story.ID, "main")
	if err != nil { t.Fatal(err) }
	if snapshot.TurnCount != baseline.TurnCount || !reflect.DeepEqual(snapshot.State, baseline.State) {
		t.Fatalf("aborting input modified story state: before=%+v after=%+v", baseline, snapshot)
	}
	if len(snapshot.PendingPlayerInputs) != 0 || len(snapshot.PendingModelContextBatches) != 0 {
		t.Fatalf("abandoned input leaked into future model context: %+v", snapshot.PendingPlayerInputs)
	}
	pending, err := cold.PendingTurnInterruption(story.ID, "main")
	if err != nil || pending != nil { t.Fatalf("interruption persisted after abort: %+v %v", pending, err) }
	history, err := cold.ReadModelHistory(story.ID, StoryModelHistoryQuery{BranchID: "main", EndTurn: baseline.TurnCount})
	if err != nil || len(history.Turns) != baseline.TurnCount { t.Fatalf("history changed: %+v %v", history, err) }
	next, err := NewPlayerInputIntent(DomainCommitIdentity{
		CommandID: "next-command", OperationID: "external-game-next-operation", Cycle: 1,
	}, "main", "Try a different route")
	if err != nil { t.Fatal(err) }
	if _, err := cold.CommitPlayerInput(story.ID, next); err != nil { t.Fatalf("new player input blocked after abort: %v", err) }
	snapshot, err = cold.Snapshot(story.ID, "main")
	if err != nil || len(snapshot.PendingPlayerInputs) != 1 || snapshot.PendingPlayerInputs[0].Text != next.Text {
		t.Fatalf("new turn does not have an independent pending input: %+v %v", snapshot.PendingPlayerInputs, err)
	}
}
