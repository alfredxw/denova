package agent

import (
	"context"
	"testing"

	runstate "denova/internal/agent/runtime"
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
	if snapshot.Binding.Profile != runstate.ProfileWriting || snapshot.Phase != runstate.PhaseIdle {
		t.Fatalf("default durable projection = %#v", snapshot)
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
		want    runstate.BindingRef
	}{
		{
			name:    "writing",
			options: RunOptions{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "session-1"},
			want: runstate.BindingRef{
				Kind: runstate.BindingWriting, Profile: runstate.ProfileWriting,
				Workspace: "/book", SessionID: "session-1",
			},
		},
		{
			name:    "game",
			options: RunOptions{AgentKind: AgentKindInteractiveStory, Workspace: "/book", StoryID: "story-1", BranchID: "main"},
			want: runstate.BindingRef{
				Kind: runstate.BindingGame, Profile: runstate.ProfileGame,
				Workspace: "/book", StoryID: "story-1", BranchID: "main",
			},
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
			if snapshot.Cursor != 0 || snapshot.Phase != runstate.PhaseIdle {
				t.Fatalf("new projection = %#v, want cursor=0 phase=idle", snapshot)
			}
		})
	}
}
