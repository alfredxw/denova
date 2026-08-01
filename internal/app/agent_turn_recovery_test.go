package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentharness "denova/internal/agents/harness"
	"errors"
	"testing"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"
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
	writingRequest := agentharness.TurnRestoreRequest{
		Binding: agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Workspace: workspace, SessionID: sessionID},
		Kind:    agentharness.CommandFollowUp, CommandID: "restore-writing-follow-up",
		OperationID: "writing-operation", Request: agentchat.ChatRequest{Message: "continue", Locale: "en-US"},
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindIDE, Workspace: workspace, SessionID: sessionID, Mode: "ide",
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
	gameRequest := agentharness.TurnRestoreRequest{
		Binding: agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace, StoryID: story.ID, BranchID: "main"},
		Kind:    agentharness.CommandSteer, CommandID: "restore-game-steer",
		OperationID: "game-operation", Request: agentchat.ChatRequest{Message: "open the door", Locale: "zh-CN"},
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace,
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
	_, err := application.restoreHarnessTurn(context.Background(), agentharness.TurnRestoreRequest{
		Binding: agentrun.RuntimeBinding{AgentKind: "unsupported"},
	})
	if !errors.Is(err, agentharness.ErrTurnRestoreUnavailable) {
		t.Fatalf("unsupported profile restore error = %v", err)
	}
}

func TestAppRoutesGeneralQueuedTurnRecoveryToAgentChat(t *testing.T) {
	application := &App{}
	spec, err := application.restoreHarnessTurn(context.Background(), agentharness.TurnRestoreRequest{
		Binding: agentrun.RuntimeBinding{
			AgentKind: agentrun.AgentKindGeneral, ProjectID: "project-1",
			Mode: "agent_chat", SessionID: "session-1",
		},
	})
	if err != nil || spec.Prepare == nil {
		t.Fatalf("General queued turn restore spec = %#v err=%v", spec, err)
	}
}
