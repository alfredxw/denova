package app

import (
	"context"
	"errors"
	"testing"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/interactive"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestAppRestoresWritingAndGameQueuedTurnDependencies(t *testing.T) {
	root := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		OpenAIModel: "test-model", NovaDir: root, Workspace: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)

	application.mu.RLock()
	workspace := application.workspace
	sessionID := application.session.ID
	application.mu.RUnlock()
	writingRequest := agents.HarnessTurnRestoreRequest{
		Binding: writingRuntimeBindingForTest(workspace, sessionID),
		Kind:    agents.AgentCommandFollowUp, CommandID: "restore-writing-follow-up",
		OperationID: "writing-operation", Request: agents.ChatRequest{Message: "continue", Locale: "en-US"},
		Options: agents.RunOptions{
			AgentKind: agents.AgentKindIDE, Workspace: workspace, SessionID: sessionID, Mode: "ide",
		},
		Deferred: true,
	}
	writingSpec, err := application.restoreHarnessTurn(context.Background(), writingRequest)
	if err != nil {
		t.Fatal(err)
	}
	writing, err := writingSpec.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if writing.Runner == nil || writing.Conversation == nil || writing.Options.Workspace != workspace || writing.Options.SessionID != sessionID {
		t.Fatalf("restored writing execution = %#v", writing)
	}

	story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{
		Title: "Recovery", StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	gameRequest := agents.HarnessTurnRestoreRequest{
		Binding: gameRuntimeBindingForTest(workspace, story.ID, "main"),
		Kind:    agents.AgentCommandSteer, CommandID: "restore-game-steer",
		OperationID: "game-operation", Request: agents.ChatRequest{Message: "open the door", Locale: "zh-CN"},
		Options: agents.RunOptions{
			AgentKind: agents.AgentKindInteractiveStory, Workspace: workspace,
			StoryID: story.ID, BranchID: "main", Mode: "interactive",
		},
		Deferred: true,
	}
	gameSpec, err := application.restoreHarnessTurn(context.Background(), gameRequest)
	if err != nil {
		t.Fatal(err)
	}
	game, err := gameSpec.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if game.Runner == nil || game.Conversation == nil || game.Options.Workspace != workspace || game.Options.StoryID != story.ID || game.Options.BranchID != "main" {
		t.Fatalf("restored game execution = %#v", game)
	}
}

func TestAppRejectsQueuedTurnRecoveryForUnsupportedProfile(t *testing.T) {
	application := &App{}
	_, err := application.restoreHarnessTurn(context.Background(), agents.HarnessTurnRestoreRequest{
		Binding: runstate.BindingRef{Kind: "unsupported", Key: "unsupported"},
	})
	if !errors.Is(err, agents.ErrHarnessTurnRestoreUnavailable) {
		t.Fatalf("unsupported profile restore error = %v", err)
	}
}
