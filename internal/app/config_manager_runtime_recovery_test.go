package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"denova/config"
	agents "denova/internal/agents"
)

type configManagerProjectionProbe struct {
	stored        agentrun.RuntimeStatus
	recovered     agentrun.RuntimeStatus
	storedCalls   int
	recoveryCalls int
}

func (p *configManagerProjectionProbe) RuntimeStatusProjection(context.Context, agentrun.Options) (agentrun.RuntimeStatus, error) {
	p.storedCalls++
	return p.stored, nil
}

func (p *configManagerProjectionProbe) RuntimeRecoveryStatusProjection(context.Context, agentrun.Options) (agentrun.RuntimeStatus, error) {
	p.recoveryCalls++
	return p.recovered, nil
}

func TestConfigManagerIdleActiveProjectionNeverOpensRecoveryActor(t *testing.T) {
	probe := &configManagerProjectionProbe{stored: agentrun.RuntimeStatus{Phase: agentrun.RunPhaseIdle}}
	for index := 0; index < 256; index++ {
		status, ok := projectConfigManagerRuntime(context.Background(), probe, agentrun.Options{
			AgentKind: agentrun.AgentKindConfigManager,
			Workspace: "/book",
			SessionID: fmt.Sprintf("config-scope-%d", index),
		})
		if !ok || status.Phase != agentrun.RunPhaseIdle {
			t.Fatalf("idle projection %d = %#v projected=%t", index, status, ok)
		}
	}
	if probe.storedCalls != 256 || probe.recoveryCalls != 0 {
		t.Fatalf("projection calls stored=%d recovery=%d", probe.storedCalls, probe.recoveryCalls)
	}
}

func TestConfigManagerColdUnfinishedProjectionOpensCanonicalRecovery(t *testing.T) {
	probe := &configManagerProjectionProbe{
		stored: agentrun.RuntimeStatus{
			Phase: agentrun.RunPhaseIdle, RecoveryPending: true,
			LastOperation: &agentrun.OperationSummary{Status: agentrun.OperationInterrupted},
		},
		recovered: agentrun.RuntimeStatus{
			Phase: agentrun.RunPhaseRunning, RecoveryPaused: true,
			ActiveCommandID: "accepted-start", ActiveOperation: "operation-1",
		},
	}
	status, ok := projectConfigManagerRuntime(context.Background(), probe, agentrun.Options{
		AgentKind: agentrun.AgentKindConfigManager, Workspace: "/book", SessionID: "config-scope",
	})
	if !ok || !status.RecoveryPaused || status.ActiveCommandID != "accepted-start" {
		t.Fatalf("canonical recovery projection = %#v projected=%t", status, ok)
	}
	if probe.storedCalls != 1 || probe.recoveryCalls != 1 {
		t.Fatalf("projection calls stored=%d recovery=%d", probe.storedCalls, probe.recoveryCalls)
	}
}

func TestConfigManagerNewNormalTaskSupersedesSettledRecoveryDisplay(t *testing.T) {
	recoveryTask, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	recoveryTask.RejectStart(errors.New("settled recovery"))
	startTask, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	startTask.RejectStart(errors.New("settled normal run"))

	selected, recoverySelected := selectConfigManagerDisplayRecord(
		configManagerTaskRecord{CommandID: "new-normal", Task: startTask},
		&configManagerRecoveryRun{task: recoveryTask},
	)
	if recoverySelected || selected.Task != startTask || selected.CommandID != "new-normal" {
		t.Fatalf("selected display = %#v recovery_selected=%t", selected, recoverySelected)
	}
}

func TestConfigManagerSettledOrMismatchedDisplayIsNotStreamAttached(t *testing.T) {
	settled, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	settled.RejectStart(errors.New("settled display"))
	runtime := agentrun.RuntimeStatus{
		Phase: agentrun.RunPhaseRunning, RecoveryPaused: true,
		ActiveCommandID: "accepted-start", ActiveOperation: "operation-1",
	}
	if configManagerDisplayOwnsRuntime(configManagerTaskRecord{CommandID: "accepted-start", Task: settled}, runtime) {
		t.Fatal("settled Config Manager display was reported as stream attached")
	}

	active, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { active.RejectStart(errors.New("test cleanup")) })
	if configManagerDisplayOwnsRuntime(configManagerTaskRecord{CommandID: "older-command", Task: active}, runtime) {
		t.Fatal("display task from another Runtime command was reported as attached")
	}
	if !configManagerDisplayOwnsRuntime(configManagerTaskRecord{CommandID: "accepted-start", Task: active}, runtime) {
		t.Fatal("active display task for the exact Runtime command was not attached")
	}
}

func TestConfigManagerRecoveryRegistryRejectsUnboundedActiveRuns(t *testing.T) {
	registry := configManagerRecoveryRegistry{replayByteLimit: (maxRememberedConfigManagerRecoveries + 1) * (64 << 20)}
	tasks := make([]*apptask.Task, 0, maxRememberedConfigManagerRecoveries+1)
	t.Cleanup(func() {
		for _, task := range tasks {
			task.RejectStart(errors.New("test cleanup"))
		}
	})
	for index := 0; index < maxRememberedConfigManagerRecoveries; index++ {
		task, err := apptask.NewDeferred(nil)
		if err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, task)
		if err := registry.install(&configManagerRecoveryRun{
			workspace: "/book", sessionID: fmt.Sprintf("scope-%d", index), task: task,
		}); err != nil {
			t.Fatalf("install recovery %d: %v", index, err)
		}
	}
	overflow, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	tasks = append(tasks, overflow)
	err = registry.install(&configManagerRecoveryRun{workspace: "/book", sessionID: "overflow", task: overflow})
	if !errors.Is(err, ErrAgentReplayCapacity) || len(registry.runs) != maxRememberedConfigManagerRecoveries {
		t.Fatalf("overflow err=%v records=%d", err, len(registry.runs))
	}
}

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
	scope := ConfigManagerRequest{Origin: "settings", ResourceID: "resource-cold-recovery"}
	sessionID, err := configManagerSessionID(scope)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	stale.RejectStart(errors.New("stale display from before recovery"))
	application.mu.RLock()
	currentWorkspace := application.workspace
	application.mu.RUnlock()
	if err := application.configManager().starts.remember(writingStartRecord{
		commandID: "stale-config-display", workspace: currentWorkspace,
		sessionID: sessionID, fingerprint: "stale", task: stale,
	}); err != nil {
		t.Fatal(err)
	}

	view := application.ConfigManagerAgentActiveView(context.Background(), scope)
	actions := agentharness.RuntimeRecoveryActions(view.Runtime)
	if !view.RuntimeProjectionOK || !view.Runtime.RecoveryPaused || view.StreamAttached || len(actions) != 2 ||
		actions[0].Kind != agentharness.RuntimeRecoveryAttach || actions[0].CommandID != "config-manager-recovery-start" ||
		actions[1].Kind != agentharness.RuntimeRecoveryAbort {
		t.Fatalf("cold Config Manager projection view=%#v actions=%#v", view, actions)
	}
	if view.Task == nil || application.ConfigManagerDisplayTask(context.Background(), scope, view.Task.ID) != nil {
		t.Fatalf("settled stale display was attachable: %#v", view.Task)
	}

	attached, err := application.RecoverConfigManagerAgent(context.Background(), scope, AgentRuntimeRecoveryRequest{Action: actions[0]})
	if err != nil {
		t.Fatal(err)
	}
	if attached.Task == nil || attached.Task.Finished() {
		t.Fatalf("attach result = %#v", attached)
	}
	attachedView := application.ConfigManagerAgentActiveView(context.Background(), scope)
	if attachedView.Task == nil || attachedView.Task.ID != attached.Task.ID() || !attachedView.StreamAttached {
		t.Fatalf("attached active view = %#v task=%s", attachedView, attached.Task.ID())
	}
	if exact := application.ConfigManagerDisplayTask(context.Background(), scope, attached.Task.ID()); exact != attached.Task {
		t.Fatalf("exact live display lookup = %p, want %p", exact, attached.Task)
	}
	aborted, err := application.RecoverConfigManagerAgent(context.Background(), scope, AgentRuntimeRecoveryRequest{Action: actions[1]})
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
	status := application.ConfigManagerAgentActiveView(context.Background(), scope).Runtime
	if status.Phase != agentrun.RunPhaseIdle || status.LastOperation == nil || status.LastOperation.Status != agentrun.OperationAborted {
		t.Fatalf("aborted Config Manager recovery status = %#v", status)
	}

	clearScope := ConfigManagerRequest{Origin: "settings", ResourceID: "resource-cold-clear"}
	clearView := application.ConfigManagerAgentActiveView(context.Background(), clearScope)
	clearActions := agentharness.RuntimeRecoveryActions(clearView.Runtime)
	if !clearView.Runtime.RecoveryPaused || clearView.StreamAttached || len(clearActions) < 1 || clearActions[0].Kind != agentharness.RuntimeRecoveryAttach {
		t.Fatalf("cold clear projection view=%#v actions=%#v", clearView, clearActions)
	}
	clearRecovery, err := application.RecoverConfigManagerAgent(context.Background(), clearScope, AgentRuntimeRecoveryRequest{Action: clearActions[0]})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.ClearConfigManagerSessionContext(context.Background(), clearScope); err != nil {
		t.Fatal(err)
	}
	select {
	case <-clearRecovery.Task.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Config Manager /clear did not drain the exact display task")
	}
	cleared := application.ConfigManagerAgentActiveView(context.Background(), clearScope)
	if cleared.Task != nil || cleared.StreamAttached || cleared.Runtime.Phase != agentrun.RunPhaseIdle || cleared.Runtime.RecoveryPaused || cleared.Runtime.RecoveryPending {
		t.Fatalf("Config Manager /clear left active state: %#v", cleared)
	}
	unchanged := application.ConfigManagerAgentActiveView(context.Background(), scope).Runtime
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
	application.mu.RUnlock()
	seeds := []struct {
		scope     ConfigManagerRequest
		commandID string
	}{
		{scope: ConfigManagerRequest{Origin: "settings", ResourceID: "resource-cold-recovery"}, commandID: "config-manager-recovery-start"},
		{scope: ConfigManagerRequest{Origin: "settings", ResourceID: "resource-cold-clear"}, commandID: "config-manager-clear-start"},
	}
	vanished := make([]chan struct{}, 0, len(seeds))
	for _, seed := range seeds {
		sessionID, err := configManagerSessionID(seed.scope)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := application.sessionStore.GetOrCreate(sessionID); err != nil {
			t.Fatal(err)
		}
		reachedContext := make(chan struct{})
		vanished = append(vanished, reachedContext)
		if _, err := application.chatService.StartWithOptions(
			context.Background(),
			newInteractiveReplayRunner(t, &interactiveReplayModel{message: agents.AssistantMessage("must not run", nil)}),
			&interactiveCrashConversation{vanished: reachedContext}, application.bookService,
			agentchat.ChatRequest{CommandID: seed.commandID, Message: "persist Config Manager work before crash"},
			configManagerRunOptions(workspace, "", sessionID), nil,
		); err != nil {
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
