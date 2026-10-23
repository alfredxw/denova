package app

import (
	"errors"
	"reflect"
	"testing"

	"denova/config"
	"denova/internal/interactive"
)

func TestDetachedGameDraftCanBeAbandonedOnlyByMatchingInterruption(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{Title: "Detached Game draft", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	binding := ConversationConfigBinding{Mode: ConversationModeInteractive, ProjectID: application.ProjectID(), StoryID: story.ID, BranchID: "main"}
	initial, err := application.ConversationConfig(t.Context(), binding)
	if err != nil {
		t.Fatal(err)
	}
	cli := initial.Config
	cli.Runtime = &config.RuntimeSelection{Kind: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{Model: "test"}}
	if _, err := application.interactive.SetBranchRuntimeConfig(story.ID, "main", cli, initial.Revision); err != nil {
		t.Fatal(err)
	}
	before, err := application.interactive.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	intent, err := interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID: "detached-turn", OperationID: "external-game-detached-turn", Cycle: 1,
	}, "main", "Open the gate")
	if err != nil {
		t.Fatal(err)
	}
	input, err := application.interactive.CommitPlayerInput(story.ID, intent)
	if err != nil {
		t.Fatal(err)
	}
	interrupted, err := application.interactive.MarkTurnInterrupted(story.ID, "main", input.Event.ID, intent.Text, "The gate creaks.", "connection_lost")
	if err != nil {
		t.Fatal(err)
	}
	if err := application.AbandonInteractiveInterruption(t.Context(), story.ID, "main", "wrong-interruption"); !errors.Is(err, ErrAgentContextChanged) {
		t.Fatalf("wrong interruption was accepted: %v", err)
	}
	if err := application.AbandonInteractiveInterruption(t.Context(), story.ID, "main", interrupted.ID); err != nil {
		t.Fatal(err)
	}
	if err := application.AbandonInteractiveInterruption(t.Context(), story.ID, "main", interrupted.ID); err != nil {
		t.Fatalf("repeated abandonment should be idempotent: %v", err)
	}
	after, err := application.interactive.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.TurnCount != before.TurnCount || !reflect.DeepEqual(after.State, before.State) || len(after.PendingPlayerInputs) != 0 {
		t.Fatalf("abandonment affected committed Game state: %+v", after)
	}
}
