package app

import (
	"context"
	agentharness "denova/internal/agents/harness"
	"testing"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/interactive"
)

func TestDefaultChatServiceProvidesDurableRuntimeProjection(t *testing.T) {
	service := agentharness.NewEphemeralService()
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	application := &App{
		workspace:   "/book",
		session:     &session.Session{ID: "session-1"},
		chatService: service,
	}
	projection, ok := application.WritingAgentRuntimeProjection(context.Background())
	if !ok || projection.Binding.AgentKind != agentrun.AgentKindIDE {
		t.Fatalf("default durable projection = %#v available=%t", projection, ok)
	}
}

func TestAgentRuntimeProjectionUsesExplicitWritingAndGameIdentities(t *testing.T) {
	workspace := t.TempDir()
	service, err := agentharness.NewDurableService(context.Background(), t.TempDir())
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
	if writing.Binding.AgentKind != agentrun.AgentKindIDE || writing.Binding.Workspace != workspace || writing.Binding.SessionID != "session-1" {
		t.Fatalf("writing binding = %#v", writing.Binding)
	}

	game, ok := application.InteractiveAgentRuntimeProjection(context.Background(), story.ID, "main")
	if !ok {
		t.Fatal("game projection unavailable")
	}
	if game.Binding.AgentKind != agentrun.AgentKindInteractiveStory || game.Binding.Workspace != workspace || game.Binding.StoryID != story.ID || game.Binding.BranchID != "main" {
		t.Fatalf("game binding = %#v", game.Binding)
	}
}
