package app

import (
	"context"
	"testing"

	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/interactive"
)

func TestDefaultChatServiceProvidesDurableRuntimeProjection(t *testing.T) {
	service := agents.NewEphemeralChatService()
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	application := &App{
		workspace:   "/book",
		session:     &session.Session{ID: "session-1"},
		chatService: service,
	}
	projection, ok := application.WritingAgentRuntimeProjection(context.Background())
	productBinding, err := agents.ParseRuntimeBinding(projection.Binding)
	if !ok || err != nil || productBinding.AgentKind != agents.AgentKindIDE {
		t.Fatalf("default durable projection = %#v available=%t", projection, ok)
	}
}

func TestAgentRuntimeProjectionUsesExplicitWritingAndGameIdentities(t *testing.T) {
	workspace := t.TempDir()
	service, err := agents.NewDurableChatService(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Projection", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	application := &App{
		workspace:   workspace,
		session:     &session.Session{ID: "session-1"},
		chatService: service,
		interactive: store,
	}

	writing, ok := application.WritingAgentRuntimeProjection(context.Background())
	if !ok {
		t.Fatal("writing projection unavailable")
	}
	writingBinding, err := agents.ParseRuntimeBinding(writing.Binding)
	if err != nil || writingBinding.AgentKind != agents.AgentKindIDE || writingBinding.Workspace != workspace || writingBinding.SessionID != "session-1" {
		t.Fatalf("writing binding = %#v", writing.Binding)
	}

	game, ok := application.InteractiveAgentRuntimeProjection(context.Background(), story.ID, "main")
	if !ok {
		t.Fatal("game projection unavailable")
	}
	gameBinding, err := agents.ParseRuntimeBinding(game.Binding)
	if err != nil || gameBinding.AgentKind != agents.AgentKindInteractiveStory || gameBinding.Workspace != workspace || gameBinding.StoryID != story.ID || gameBinding.BranchID != "main" {
		t.Fatalf("game binding = %#v", game.Binding)
	}
}
