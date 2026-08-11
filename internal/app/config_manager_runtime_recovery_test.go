package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"denova/config"
	configmanagerapp "denova/internal/app/configmanager"
)

func TestConfigManagerColdRecoveryAttachesAndAbortsSameDisplayTask(t *testing.T) {
	if os.Getenv("DENOVA_CONFIG_MANAGER_RECOVERY_SEED") == "1" {
		runConfigManagerRecoveryCrashSeed(t)
		return
	}
	dataDir := t.TempDir()
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestConfigManagerColdRecoveryAttachesAndAbortsSameDisplayTask$")
	command.Env = append(os.Environ(),
		"DENOVA_CONFIG_MANAGER_RECOVERY_SEED=1",
		"DENOVA_CONFIG_MANAGER_RECOVERY_DATA_DIR="+dataDir,
		"DENOVA_CONFIG_MANAGER_RECOVERY_WORKSPACE="+workspace,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Config Manager recovery crash seed failed: %v\n%s", err, output)
	}

	application, err := New(context.Background(), &config.Config{
		OpenAIAPIKey: "unused", OpenAIModel: "test-model",
		NovaDir: dataDir, Workspace: workspace, ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	projectID := application.ProjectID()
	scope := configmanagerapp.Request{ProjectID: projectID, Origin: "settings", ResourceID: "resource-cold-recovery"}

	view := application.ConfigManager().ActiveView(context.Background(), scope)
	actions := agentexecution.RuntimeRecoveryActions(view.Runtime)
	if !view.RuntimeProjectionOK || !view.Runtime.RecoveryPaused || view.StreamAttached || len(actions) != 2 ||
		actions[0].Kind != agentexecution.RuntimeRecoveryAttach || actions[0].CommandID != "config-manager-recovery-start" ||
		actions[1].Kind != agentexecution.RuntimeRecoveryAbort {
		t.Fatalf("cold Config Manager projection view=%#v actions=%#v", view, actions)
	}
	if view.Task != nil {
		t.Fatalf("cold runtime unexpectedly exposed a process-local display: %#v", view.Task)
	}

	attached, err := application.ConfigManager().Recover(context.Background(), scope, AgentRuntimeRecoveryRequest{Action: actions[0]})
	if err != nil {
		t.Fatal(err)
	}
	if attached.Task == nil || attached.Task.Finished() {
		t.Fatalf("attach result = %#v", attached)
	}
	attachedView := application.ConfigManager().ActiveView(context.Background(), scope)
	if attachedView.Task == nil || attachedView.Task.ID != attached.Task.ID() || !attachedView.StreamAttached {
		t.Fatalf("attached active view = %#v task=%s", attachedView, attached.Task.ID())
	}
	if exact := application.ConfigManager().DisplayTask(context.Background(), scope, attached.Task.ID()); exact != attached.Task {
		t.Fatalf("exact live display lookup = %p, want %p", exact, attached.Task)
	}
	aborted, err := application.ConfigManager().Recover(context.Background(), scope, AgentRuntimeRecoveryRequest{Action: actions[1]})
	if err != nil {
		t.Fatal(err)
	}
	if aborted.Task != attached.Task {
		t.Fatalf("abort changed display task: attach=%p/%s abort=%p/%s", attached.Task, attached.Task.ID(), aborted.Task, aborted.Task.ID())
	}
	select {
	case <-attached.Task.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Config Manager recovery display did not settle after abort")
	}
	status := application.ConfigManager().ActiveView(context.Background(), scope).Runtime
	if status.Phase != agentrun.RunPhaseIdle || status.LastOperation == nil || status.LastOperation.Status != agentrun.OperationAborted {
		t.Fatalf("aborted Config Manager recovery status = %#v", status)
	}

	clearScope := configmanagerapp.Request{ProjectID: projectID, Origin: "settings", ResourceID: "resource-cold-clear"}
	clearView := application.ConfigManager().ActiveView(context.Background(), clearScope)
	clearActions := agentexecution.RuntimeRecoveryActions(clearView.Runtime)
	if !clearView.Runtime.RecoveryPaused || clearView.StreamAttached || len(clearActions) < 1 || clearActions[0].Kind != agentexecution.RuntimeRecoveryAttach {
		t.Fatalf("cold clear projection view=%#v actions=%#v", clearView, clearActions)
	}
	clearRecovery, err := application.ConfigManager().Recover(context.Background(), clearScope, AgentRuntimeRecoveryRequest{Action: clearActions[0]})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.ConfigManager().ClearContext(context.Background(), clearScope); err != nil {
		t.Fatal(err)
	}
	select {
	case <-clearRecovery.Task.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Config Manager /clear did not drain the exact display task")
	}
	cleared := application.ConfigManager().ActiveView(context.Background(), clearScope)
	if cleared.Task != nil || cleared.StreamAttached || cleared.Runtime.Phase != agentrun.RunPhaseIdle || cleared.Runtime.RecoveryPaused || cleared.Runtime.RecoveryPending {
		t.Fatalf("Config Manager /clear left active state: %#v", cleared)
	}
	unchanged := application.ConfigManager().ActiveView(context.Background(), scope).Runtime
	if unchanged.LastOperation == nil || unchanged.LastOperation.Status != agentrun.OperationAborted {
		t.Fatalf("clearing another Config Manager scope changed the recovered scope: %#v", unchanged)
	}
}

func runConfigManagerRecoveryCrashSeed(t *testing.T) {
	t.Helper()
	application, err := New(context.Background(), &config.Config{
		OpenAIAPIKey: "unused", OpenAIModel: "test-model",
		NovaDir:             os.Getenv("DENOVA_CONFIG_MANAGER_RECOVERY_DATA_DIR"),
		Workspace:           os.Getenv("DENOVA_CONFIG_MANAGER_RECOVERY_WORKSPACE"),
		ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	application.mu.RLock()
	workspace := application.workspace
	projectID := application.cfg.ProjectID
	stateRoot := application.cfg.ProjectStateDir
	application.mu.RUnlock()
	seeds := []struct {
		scope     configmanagerapp.Request
		commandID string
	}{
		{scope: configmanagerapp.Request{ProjectID: projectID, Origin: "settings", ResourceID: "resource-cold-recovery"}, commandID: "config-manager-recovery-start"},
		{scope: configmanagerapp.Request{ProjectID: projectID, Origin: "settings", ResourceID: "resource-cold-clear"}, commandID: "config-manager-clear-start"},
	}
	vanished := make([]chan struct{}, 0, len(seeds))
	for _, seed := range seeds {
		sessionID, err := configmanagerapp.SessionID(seed.scope)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := application.sessionStore.GetOrCreate(sessionID); err != nil {
			t.Fatal(err)
		}
		restoreData, err := configmanagerapp.RestoreDataForRequest(seed.scope)
		if err != nil {
			t.Fatal(err)
		}
		reachedContext := make(chan struct{})
		vanished = append(vanished, reachedContext)
		chatRequest := agentchat.ChatRequest{CommandID: seed.commandID, Message: "persist Config Manager work before crash"}
		options := agentrun.Options{
			AgentKind: agentrun.AgentKindConfigManager, ProjectID: projectID, StateRoot: stateRoot, Workspace: workspace,
			SessionID: sessionID, Mode: configmanagerapp.RuntimeMode, RestoreData: restoreData,
		}
		binding := agentrun.RuntimeBinding{
			AgentKind: agentrun.AgentKindConfigManager, ProjectID: projectID, Workspace: workspace,
			SessionID: sessionID, Mode: configmanagerapp.RuntimeMode,
		}
		cycle, err := application.ConfigManager().PrepareCycle(context.Background(), agentexecution.CycleRestoreRequest{
			Request: chatRequest, Options: options,
		}, binding)
		if err != nil {
			t.Fatal(err)
		}
		cycle.Conversation = &interactiveCrashConversation{vanished: reachedContext}
		if _, err := application.executionRuntime.Start(context.Background(), agentexecution.StartRequest{Cycle: cycle}); err != nil {
			t.Fatal(err)
		}
	}
	for _, reachedContext := range vanished {
		select {
		case <-reachedContext:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Config Manager recovery seed did not reach model context assembly")
		}
	}
	os.Exit(0)
}
