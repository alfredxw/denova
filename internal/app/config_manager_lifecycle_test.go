package app

import (
	"context"
	"testing"

	"denova/config"
	configmanagerapp "denova/internal/app/configmanager"
)

func TestConfigManagerDoesNotCreateSessionBeforeWorkspaceAdmission(t *testing.T) {
	root := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		OpenAIModel: "test-model", NovaDir: root, Workspace: root, ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	projectID := application.ProjectID()
	runtime, err := application.AgentChat().ProjectRuntime(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := runtime.SessionStore.List("")
	if err != nil {
		t.Fatal(err)
	}
	application.mu.Lock()
	application.projectTransitions[projectID] = struct{}{}
	application.mu.Unlock()

	task := application.ConfigManager().StartTask(context.Background(), configmanagerapp.Request{
		ProjectID:   projectID,
		CommandID:   "config-manager-admission",
		Instruction: "update configuration",
		Origin:      "test",
	})
	if task != nil {
		t.Fatal("config manager task was admitted while workspace generation was fenced")
	}
	sessions, err := runtime.SessionStore.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != len(before) {
		t.Fatalf("rejected Config Manager request changed session count: before=%+v after=%+v", before, sessions)
	}
	for index := range before {
		if sessions[index].ID != before[index].ID {
			t.Fatalf("rejected Config Manager request changed sessions: before=%+v after=%+v", before, sessions)
		}
	}
}
