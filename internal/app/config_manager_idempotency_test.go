package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	apptask "denova/internal/app/task"

	"denova/config"
	agentrun "denova/internal/agents/run"
	configmanagerapp "denova/internal/app/configmanager"
)

func TestConfigManagerInitialStartReusesExactTaskAndRejectsConflict(t *testing.T) {
	root := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		OpenAIModel: "test-model", NovaDir: root, Workspace: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)

	request := configmanagerapp.Request{
		ProjectID:   application.ProjectID(),
		CommandID:   "config-manager-same-start",
		Instruction: "update the selected resource",
		Origin:      "settings", ResourceID: "resource-1",
		Context: map[string]string{"kind": "teller"},
	}
	first, err := application.ConfigManager().StartTaskWithError(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := application.ConfigManager().StartTaskWithError(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if replayed != first || replayed.ID() != first.ID() {
		t.Fatalf("same Config Manager command did not return the exact Task: first=%p/%s replay=%p/%s", first, first.ID(), replayed, replayed.ID())
	}

	conflict := request
	conflict.Instruction = "delete the selected resource"
	if task, err := application.ConfigManager().StartTaskWithError(context.Background(), conflict); task != nil || !errors.Is(err, ErrAgentCommandConflict) {
		t.Fatalf("different Config Manager payload reuse = task=%v err=%v", task, err)
	}
}

func TestConfigManagerInitialStartRequiresCallerCommandID(t *testing.T) {
	service := configmanagerapp.NewService(nil)
	if task, err := service.StartTaskWithError(context.Background(), configmanagerapp.Request{Instruction: "update"}); task != nil || !errors.Is(err, ErrAgentCommandIDRequired) {
		t.Fatalf("missing command_id = task=%v err=%v", task, err)
	}
}

func TestConfigManagerInitialStartRejectsOversizedCommandIDBeforeWorkspaceAccess(t *testing.T) {
	service := configmanagerapp.NewService(nil)
	request := configmanagerapp.Request{CommandID: strings.Repeat("x", 4097), Instruction: "update"}
	if task, err := service.StartTaskWithError(context.Background(), request); task != nil || !errors.Is(err, agentrun.ErrInvalidCommand) {
		t.Fatalf("oversized command_id = task=%v err=%v", task, err)
	}
}

func TestConfigManagerReplayCapacityRejectsBeforeRuntimeAdmission(t *testing.T) {
	root := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		OpenAIModel: "test-model", NovaDir: root, Workspace: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)

	blocker, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	application.activeTaskReplay.Configure(apptask.ReplayAdmissionLimits{MaxBytes: blocker.DisplayReplayCharge()})
	reservation, err := application.activeTaskReplay.Reserve(blocker)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		reservation.Release()
		blocker.RejectStart(errors.New("test cleanup"))
	})

	request := configmanagerapp.Request{
		ProjectID: application.ProjectID(),
		CommandID: "config-capacity-pre-admission", Instruction: "update settings",
		Origin: "settings", ResourceID: "resource-capacity",
	}
	if task, err := application.ConfigManager().StartTaskWithError(context.Background(), request); task != nil || !errors.Is(err, ErrAgentReplayCapacity) {
		t.Fatalf("capacity start = task=%v err=%v, want pre-admission capacity rejection", task, err)
	}

	sessionID, err := configmanagerapp.SessionID(request)
	if err != nil {
		t.Fatal(err)
	}
	application.mu.RLock()
	workspace := application.workspace
	executionRuntime := application.executionRuntime
	application.mu.RUnlock()
	status, err := executionRuntime.RuntimeStatusProjection(context.Background(), agentrun.Options{
		AgentKind: agentrun.AgentKindConfigManager, SessionID: sessionID,
		Workspace: workspace, Mode: "config_manager",
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentrun.RunPhaseIdle || status.ActiveOperation != "" || status.LastOperation != nil || len(status.Queue) != 0 {
		t.Fatalf("Runtime was mutated before capacity admission: %#v", status)
	}
}
