package interactiveapp

import (
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/session"
	"denova/internal/interactive"
)

func TestInteractiveConversationPersistsStreamedDisplayEventsAtStableBoundaries(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Display streaming"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{User: "Continue", Narrative: "Started"})
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, t.TempDir(), workspace, story.ID, "main", "", 800, nil)
	conversation.lastTurn = &turn

	if err := conversation.AppendDisplayEvent(session.DisplayEvent{
		ID: "progress-1", Role: "tool_call", Name: "submit_director_plan_update", Args: "raw-part-1", Status: "running",
	}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayEvent(session.DisplayEvent{
		ID: "progress-1", Role: "tool_call", Name: "submit_director_plan_update", Args: "raw-part-1|raw-part-2", Status: "running",
	}); err != nil {
		t.Fatal(err)
	}
	if got := displayEventByID(t, conversation.DisplayEventsSnapshot(), "progress-1").Args; got != "raw-part-1|raw-part-2" {
		t.Fatalf("live progress args = %q, want exact raw input", got)
	}
	if got := persistedDisplayEventByID(t, store, story.ID, "progress-1").Args; got != "raw-part-1" {
		t.Fatalf("transient progress was persisted as %q, want initial snapshot", got)
	}
	if err := conversation.AppendDisplayEvent(session.DisplayEvent{
		ID: "progress-1", Role: "tool_call", Name: "submit_director_plan_update", Args: "raw-part-1|raw-part-2", Status: "success",
	}); err != nil {
		t.Fatal(err)
	}
	if got := persistedDisplayEventByID(t, store, story.ID, "progress-1"); got.Status != "success" || got.Args != "raw-part-1|raw-part-2" {
		t.Fatalf("terminal progress event mismatch: %#v", got)
	}

	if err := conversation.AppendDisplayEvent(session.DisplayEvent{
		ID: "tool-1", Role: "tool_call", Name: "read", Status: "running",
	}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayToolArgs("tool-1", "read", `{"path":"chapters/ch01.md"}`); err != nil {
		t.Fatal(err)
	}
	if got := persistedDisplayEventByID(t, store, story.ID, "tool-1").Args; got != "" {
		t.Fatalf("streamed tool args were persisted before completion: %q", got)
	}
	presentation := agent.ToolPresentation{Call: agent.ToolPresentationImage, Result: agent.ToolPresentationInteractiveMedia}
	if err := conversation.UpdateDisplayToolResult("tool-1", "read", "success", "ok", &presentation); err != nil {
		t.Fatal(err)
	}
	if got := persistedDisplayEventByID(t, store, story.ID, "tool-1"); got.Args != `{"path":"chapters/ch01.md"}` || got.Result != "ok" || got.ToolPresentation == nil || got.ToolPresentation.Result != agent.ToolPresentationInteractiveMedia {
		t.Fatalf("terminal tool event mismatch: %#v", got)
	}

	if err := conversation.AppendDisplayEvent(session.DisplayEvent{
		ID: "assistant-1", Role: "assistant", Content: "First", SubAgent: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendDisplayEventContent("assistant-1", "assistant", " second"); err != nil {
		t.Fatal(err)
	}
	if got := persistedDisplayEventByID(t, store, story.ID, "assistant-1").Content; got != "First" {
		t.Fatalf("streamed assistant content was persisted before flush: %q", got)
	}
	if err := conversation.FlushDisplayEventContent("assistant-1", "assistant"); err != nil {
		t.Fatal(err)
	}
	if got := persistedDisplayEventByID(t, store, story.ID, "assistant-1").Content; got != "First second" {
		t.Fatalf("flushed assistant content = %q, want %q", got, "First second")
	}
}

func persistedDisplayEventByID(t *testing.T, store *interactive.Store, storyID, eventID string) interactive.DisplayEvent {
	t.Helper()
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) == 0 {
		t.Fatal("story snapshot has no turns")
	}
	return displayEventByID(t, snapshot.Turns[len(snapshot.Turns)-1].DisplayEvents, eventID)
}

func displayEventByID(t *testing.T, events []interactive.DisplayEvent, eventID string) interactive.DisplayEvent {
	t.Helper()
	for _, event := range events {
		if event.ID == eventID {
			return event
		}
	}
	t.Fatalf("display event %q not found: %#v", eventID, events)
	return interactive.DisplayEvent{}
}
