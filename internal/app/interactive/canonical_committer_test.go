package interactiveapp

import (
	"context"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"
)

func TestCanonicalCommitterKeepsGameAttachmentsInStoryAndModelInput(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Attachment story", Origin: "Test"})
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, t.TempDir(), workspace, story.ID, "main", "Inspect the map", story.ReplyTargetChars, nil)
	conversation.BindAgentCycleIdentity(agentrun.CycleIdentity{CommandID: "attachment-command", OperationID: "attachment-operation", Cycle: 1})
	committer, err := NewCanonicalCommitter(CanonicalCommitterConfig{Conversation: conversation})
	if err != nil {
		t.Fatal(err)
	}
	attachment := agent.Attachment{ID: "att_0123456789abcdef0123456789abcdef", Name: "map.png", MediaType: "image/png", Size: 8, Path: "/state/map.png", SHA256: "digest"}
	if _, err := committer.MaterializeInput(context.Background(), agent.InputCommitRequest{
		Identity: agent.CommitIdentity{CommandID: "attachment-command", RunID: "attachment-operation", Cycle: 1, Stage: agent.CommitInput},
		Hash:     "public-input-hash",
		Input:    agent.Input{Text: "Inspect the map", Attachments: []agent.Attachment{attachment}},
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(story.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.PendingPlayerInputs) != 1 || len(snapshot.PendingPlayerInputs[0].Attachments) != 1 || snapshot.PendingPlayerInputs[0].Attachments[0] != attachment {
		t.Fatalf("canonical game input lost attachments: %#v", snapshot.PendingPlayerInputs)
	}
	assembled, err := conversation.AssembleModelContext(context.Background(), "Inspect the map", agentcontext.ModelContextInput{
		UserMessage: "Inspect the map", Attachments: []agent.Attachment{attachment}, Budget: conversation.ModelContextBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}
	last := assembled.Messages[len(assembled.Messages)-1]
	if len(last.Attachments) != 1 || last.Attachments[0] != attachment {
		t.Fatalf("game model input lost attachments: %#v", last.Attachments)
	}
}

func TestGameInterruptionSurvivesRepeatedBindingOfSameCycle(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Paused story", Origin: "Test"})
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, t.TempDir(), workspace, story.ID, "main", "Wait by the door", story.ReplyTargetChars, nil)
	identity := agentrun.CycleIdentity{CommandID: "pause-command", OperationID: "pause-operation", Cycle: 1}
	conversation.BindAgentCycleIdentity(identity)
	committer, err := NewCanonicalCommitter(CanonicalCommitterConfig{Conversation: conversation})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := committer.MaterializeInput(context.Background(), agent.InputCommitRequest{
		Identity: agent.CommitIdentity{CommandID: "pause-command", RunID: "pause-operation", Cycle: 1, Stage: agent.CommitInput},
		Hash:     "pause-input-hash",
		Input:    agent.Input{Text: "Wait by the door"},
	}); err != nil {
		t.Fatal(err)
	}

	// Context materialization binds the same accepted cycle again before the
	// stream can be interrupted. That idempotent bind must retain its input.
	conversation.BindAgentCycleIdentity(identity)
	if err := conversation.MarkInterrupted("Wait by the door", "The hallway", agentrun.AbortReasonUserRequested); err != nil {
		t.Fatal(err)
	}
	pending := conversation.PendingInterruption()
	if pending == nil || pending.UserMessage != "Wait by the door" || pending.AssistantContent != "The hallway" {
		t.Fatalf("pending interruption = %#v", pending)
	}
}
