package harness

import (
	"context"
	"denova/internal/agents/run"
	"errors"
	"testing"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestDefaultChatServiceUsesDurableMemoryHarness(t *testing.T) {
	service := NewEphemeralService()
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	snapshot, err := service.RuntimeStatusProjection(context.Background(), agentrun.Options{
		AgentKind: agentrun.AgentKindIDE,
		Workspace: "/book",
		SessionID: "session-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Binding.AgentKind != agentrun.AgentKindIDE || snapshot.Phase != agentrun.RunPhaseIdle {
		t.Fatalf("default durable projection = %#v", snapshot)
	}
}

func TestProjectRuntimeBindingIdentitySurvivesRelink(t *testing.T) {
	t.Parallel()

	for _, agentKind := range []string{agentrun.AgentKindIDE, agentrun.AgentKindGeneral} {
		agentKind := agentKind
		t.Run(agentKind, func(t *testing.T) {
			t.Parallel()
			before, err := (agentrun.RuntimeBinding{
				AgentKind: agentKind, ProjectID: "project-1", Mode: agentrun.ModeAgentChat,
				Workspace: "/old/location", SessionID: "session-1",
			}).Ref()
			if err != nil {
				t.Fatal(err)
			}
			after, err := (agentrun.RuntimeBinding{
				AgentKind: agentKind, ProjectID: "project-1", Mode: agentrun.ModeAgentChat,
				Workspace: "/new/location", SessionID: "session-1",
			}).Ref()
			if err != nil {
				t.Fatal(err)
			}
			if !before.Equal(after) || before.Label("workspace") != "" {
				t.Fatalf("Project binding changed after relink: before=%#v after=%#v", before, after)
			}
			decoded, err := agentrun.ParseRuntimeBinding(before)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.AgentKind != agentKind || decoded.ProjectID != "project-1" || decoded.SessionID != "session-1" || decoded.Workspace != "" {
				t.Fatalf("decoded stable Project binding = %#v", decoded)
			}
		})
	}
}

func TestForegroundWorkspaceClosePreservesAgentChatBindings(t *testing.T) {
	service, err := newService(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close(context.Background()) })

	writingRef, err := (agentrun.RuntimeBinding{
		AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "writing-session",
	}).Ref()
	if err != nil {
		t.Fatal(err)
	}
	agentChatRef, err := (agentrun.RuntimeBinding{
		AgentKind: agentrun.AgentKindIDE, Mode: agentrun.ModeAgentChat,
		Workspace: "/book", SessionID: "agent-chat-session",
	}).Ref()
	if err != nil {
		t.Fatal(err)
	}
	writingHarness, err := service.coordinator.runtime.Open(context.Background(), writingRef)
	if err != nil {
		t.Fatal(err)
	}
	agentChatHarness, err := service.coordinator.runtime.Open(context.Background(), agentChatRef)
	if err != nil {
		t.Fatal(err)
	}

	if err := service.CloseForegroundWorkspaceBindings(context.Background(), "/book"); err != nil {
		t.Fatal(err)
	}
	if _, err := writingHarness.Status(context.Background()); !errors.Is(err, runstate.ErrHarnessClosed) {
		t.Fatalf("foreground Writing binding status error = %v, want ErrHarnessClosed", err)
	}
	stillAgentChat, err := service.coordinator.runtime.Open(context.Background(), agentChatRef)
	if err != nil || stillAgentChat != agentChatHarness {
		t.Fatalf("AgentChat binding changed during foreground close: same=%t err=%v", stillAgentChat == agentChatHarness, err)
	}

	if err := service.CloseAgentChatSessionBindings(context.Background(), "/book", "agent-chat-session"); err != nil {
		t.Fatal(err)
	}
	if _, err := agentChatHarness.Status(context.Background()); !errors.Is(err, runstate.ErrHarnessClosed) {
		t.Fatalf("scoped AgentChat binding status error = %v, want ErrHarnessClosed", err)
	}
}

func TestRuntimeStatusProjectionDerivesProfileBindings(t *testing.T) {
	service, err := newService(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	tests := []struct {
		name    string
		options agentrun.Options
		want    agentrun.RuntimeBinding
	}{
		{
			name:    "writing",
			options: agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "session-1"},
			want:    agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Workspace: "/book", SessionID: "session-1"},
		},
		{
			name:    "agent_chat",
			options: agentrun.Options{AgentKind: agentrun.AgentKindIDE, Mode: "agent_chat", Workspace: "/book", SessionID: "session-1"},
			want:    agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Mode: "agent_chat", Workspace: "/book", SessionID: "session-1"},
		},
		{
			name:    "game",
			options: agentrun.Options{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: "/book", StoryID: "story-1", BranchID: "main"},
			want:    agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: "/book", StoryID: "story-1", BranchID: "main"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, err := service.RuntimeStatusProjection(context.Background(), test.options)
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Binding != test.want {
				t.Fatalf("binding = %#v, want %#v", snapshot.Binding, test.want)
			}
			if snapshot.Cursor != 0 || snapshot.Phase != agentrun.RunPhaseIdle {
				t.Fatalf("new projection = %#v, want cursor=0 phase=idle", snapshot)
			}
		})
	}
}
