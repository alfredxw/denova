package agents

import (
	"context"
	"errors"
	"testing"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestDefaultChatServiceUsesDurableMemoryHarness(t *testing.T) {
	service := NewEphemeralChatService()
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	snapshot, err := service.RuntimeStatusProjection(context.Background(), RunOptions{
		AgentKind: AgentKindIDE,
		Workspace: "/book",
		SessionID: "session-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Binding.AgentKind != AgentKindIDE || snapshot.Phase != RunPhaseIdle {
		t.Fatalf("default durable projection = %#v", snapshot)
	}
}

func TestForegroundWorkspaceClosePreservesAgentChatBindings(t *testing.T) {
	service, err := newHarnessChatService(context.Background(), DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close(context.Background()) })

	writingRef, err := (RuntimeBinding{
		AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "writing-session",
	}).Ref()
	if err != nil {
		t.Fatal(err)
	}
	agentChatRef, err := (RuntimeBinding{
		AgentKind: AgentKindIDE, Mode: runtimeBindingProfileAgentChat,
		Workspace: "/book", SessionID: "agent-chat-session",
	}).Ref()
	if err != nil {
		t.Fatal(err)
	}
	writingHarness, err := service.harness.runtime.Open(context.Background(), writingRef)
	if err != nil {
		t.Fatal(err)
	}
	agentChatHarness, err := service.harness.runtime.Open(context.Background(), agentChatRef)
	if err != nil {
		t.Fatal(err)
	}

	if err := service.CloseForegroundWorkspaceBindings(context.Background(), "/book"); err != nil {
		t.Fatal(err)
	}
	if _, err := writingHarness.Status(context.Background()); !errors.Is(err, runstate.ErrHarnessClosed) {
		t.Fatalf("foreground Writing binding status error = %v, want ErrHarnessClosed", err)
	}
	stillAgentChat, err := service.harness.runtime.Open(context.Background(), agentChatRef)
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
	service, err := newHarnessChatService(context.Background(), DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
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
		options RunOptions
		want    RuntimeBinding
	}{
		{
			name:    "writing",
			options: RunOptions{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "session-1"},
			want:    RuntimeBinding{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "session-1"},
		},
		{
			name:    "agent_chat",
			options: RunOptions{AgentKind: AgentKindIDE, Mode: "agent_chat", Workspace: "/book", SessionID: "session-1"},
			want:    RuntimeBinding{AgentKind: AgentKindIDE, Mode: "agent_chat", Workspace: "/book", SessionID: "session-1"},
		},
		{
			name:    "game",
			options: RunOptions{AgentKind: AgentKindInteractiveStory, Workspace: "/book", StoryID: "story-1", BranchID: "main"},
			want:    RuntimeBinding{AgentKind: AgentKindInteractiveStory, Workspace: "/book", StoryID: "story-1", BranchID: "main"},
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
			if snapshot.Cursor != 0 || snapshot.Phase != RunPhaseIdle {
				t.Fatalf("new projection = %#v, want cursor=0 phase=idle", snapshot)
			}
		})
	}
}
