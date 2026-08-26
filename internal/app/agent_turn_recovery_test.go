package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	"errors"
	"testing"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"
)

func TestAppRestoresWritingAndGameQueuedTurnDependencies(t *testing.T) {
	application := newExecutionProfileTestApp(t)

	application.mu.RLock()
	workspace := application.workspace
	sessionID := application.session.ID
	application.mu.RUnlock()
	writingRequest := agentexecution.CycleRestoreRequest{
		Binding: agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Workspace: workspace, SessionID: sessionID},
		Kind:    agentexecution.CommandFollowUp, CommandID: "restore-writing-follow-up",
		OperationID: "writing-operation", Request: agentchat.ChatRequest{Message: "continue", Locale: "en-US"},
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindIDE, Workspace: workspace, SessionID: sessionID, Mode: "ide",
		},
		Deferred: true,
	}
	writing, err := prepareProfileCycleForTest(application, context.Background(), writingRequest)
	if err != nil {
		t.Fatal(err)
	}
	if writing.Definition.Model == nil || writing.Conversation == nil || writing.Options.Workspace != workspace || writing.Options.SessionID != sessionID {
		t.Fatalf("restored writing execution = %#v", writing)
	}

	story, err := application.CreateInteractiveStory(interactive.CreateStoryRequest{
		Title: "Recovery", StoryTellerID: "classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	gameRequest := agentexecution.CycleRestoreRequest{
		Binding: agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace, StoryID: story.ID, BranchID: "main"},
		Kind:    agentexecution.CommandSteer, CommandID: "restore-game-steer",
		OperationID: "game-operation", Request: agentchat.ChatRequest{Message: "open the door", Locale: "zh-CN"},
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace,
			StoryID: story.ID, BranchID: "main", Mode: "interactive",
		},
		Deferred: true,
	}
	game, err := prepareProfileCycleForTest(application, context.Background(), gameRequest)
	if err != nil {
		t.Fatal(err)
	}
	if game.Definition.Model == nil || game.Conversation == nil || game.Options.Workspace != workspace || game.Options.StoryID != story.ID || game.Options.BranchID != "main" {
		t.Fatalf("restored game execution = %#v", game)
	}
}

func TestExecutionProfilesExposeOnlySupportedCapabilities(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	want := map[agentexecution.ProfileID]bool{
		agentexecution.ProfileWriting:       true,
		agentexecution.ProfileAgentChat:     true,
		agentexecution.ProfileGame:          true,
		agentexecution.ProfileConfigManager: true,
		agentexecution.ProfileImage:         true,
	}
	profiles := application.executionProfiles()
	if len(profiles) != len(want) {
		t.Fatalf("profile count = %d, want %d", len(profiles), len(want))
	}
	for _, profile := range profiles {
		if !want[profile.ID()] {
			t.Fatalf("unexpected profile %q", profile.ID())
		}
		if _, ok := profile.(agentexecution.QueuedCycleProfile); !ok {
			t.Fatalf("profile %q cannot restore a public Agent cycle", profile.ID())
		}
	}
}

func newExecutionProfileTestApp(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		OpenAIModel: "test-model", NovaDir: root, Workspace: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	return application
}

func TestAppRejectsQueuedTurnRecoveryForUnsupportedProfile(t *testing.T) {
	application := &App{}
	_, err := prepareProfileCycleForTest(application, context.Background(), agentexecution.CycleRestoreRequest{
		Binding: agentrun.RuntimeBinding{AgentKind: "unsupported"},
	})
	if !errors.Is(err, agentrun.ErrInvalidBinding) {
		t.Fatalf("unsupported profile restore error = %v", err)
	}
}

func TestAppRoutesGeneralQueuedTurnRecoveryToAgentChat(t *testing.T) {
	application := &App{}
	profile, err := executionProfileForTest(application, agentrun.RuntimeBinding{
		AgentKind: agentrun.AgentKindGeneral, ProjectID: "project-1",
		Mode: "agent_chat", SessionID: "session-1",
	})
	if err != nil || profile.ID() != agentexecution.ProfileAgentChat {
		t.Fatalf("General queued turn profile = %#v err=%v", profile, err)
	}
}
