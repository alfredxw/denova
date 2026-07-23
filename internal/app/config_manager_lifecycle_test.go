package app

import (
	"context"
	"testing"

	"denova/config"
	"denova/internal/agent/session"
	"denova/internal/book"
)

func TestConfigManagerDoesNotCreateSessionBeforeWorkspaceAdmission(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	application := &App{
		cfg:                 &config.Config{Workspace: workspace},
		workspace:           workspace,
		bookState:           book.NewState(workspace),
		sessionStore:        store,
		workspaceTransition: true,
		workspaceTransitionTargets: map[string]struct{}{
			lifecycleWorkspaceKey(workspace): {},
		},
	}

	task := (&ConfigManagerAppService{app: application}).StartTask(context.Background(), ConfigManagerRequest{
		CommandID:   "config-manager-admission",
		Instruction: "update configuration",
		Origin:      "test",
	})
	if task != nil {
		t.Fatal("config manager task was admitted while workspace generation was fenced")
	}
	sessions, err := store.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("rejected config manager request created session side effects: %+v", sessions)
	}
}
