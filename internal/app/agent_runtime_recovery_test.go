package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"denova/config"
)

func TestConcurrentColdWritingRecoveryCreatesOneDisplayTask(t *testing.T) {
	if os.Getenv("DENOVA_WRITING_RECOVERY_SEED") == "1" {
		runWritingRecoveryCrashSeed(t)
		return
	}
	root := t.TempDir()
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		OpenAIAPIKey: "unused", OpenAIModel: "test-model",
		NovaDir: root, Workspace: workspace, ResumeLastWorkspace: false,
	}
	seed, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	seed.Close()

	command := exec.Command(os.Args[0], "-test.run=^TestConcurrentColdWritingRecoveryCreatesOneDisplayTask$")
	command.Env = append(os.Environ(),
		"DENOVA_WRITING_RECOVERY_SEED=1",
		"DENOVA_WRITING_RECOVERY_ROOT="+root,
		"DENOVA_WRITING_RECOVERY_WORKSPACE="+workspace,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("crash seed failed: %v\n%s", err, output)
	}

	reopened, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	status, ok := reopened.WritingAgentRuntimeProjection(context.Background())
	if !ok {
		t.Fatal("writing recovery projection unavailable")
	}
	actions := agentexecution.RuntimeRecoveryActions(status)
	if status.Phase != agentrun.RunPhaseRunning || !status.RecoveryPaused || len(actions) != 2 || actions[0].Kind != agentexecution.RuntimeRecoveryAttach || actions[0].CommandID != "writing-recovery-start" || actions[1].Kind != agentexecution.RuntimeRecoveryAbort {
		t.Fatalf("cold recovery actions = %#v status=%#v", actions, status)
	}
	abortAction := actions[1]

	const callers = 16
	results := make(chan AgentRuntimeRecoveryResult, callers)
	errorsCh := make(chan error, callers)
	var start sync.WaitGroup
	start.Add(1)
	for index := 0; index < callers; index++ {
		runAppErrorTestGoroutine(errorsCh, "concurrent writing recovery", func() error {
			start.Wait()
			result, recoverErr := reopened.RecoverWritingAgent(context.Background(), AgentRuntimeRecoveryRequest{Action: abortAction})
			if recoverErr != nil {
				return recoverErr
			}
			results <- result
			return nil
		})
	}
	start.Done()
	// The helper publishes its terminal error only after the closure (and its
	// result send) returns. Receiving exactly one terminal value per caller is
	// therefore the join barrier; closing the channels after a closure-owned
	// WaitGroup can race with the helper's final send.
	for range callers {
		recoverErr := <-errorsCh
		if recoverErr != nil {
			t.Fatal(recoverErr)
		}
	}
	close(results)
	var task *apptask.Task
	for result := range results {
		if task == nil {
			task = result.Task
		}
		if result.Task != task {
			t.Fatalf("concurrent recovery created task %p, want %p", result.Task, task)
		}
	}
	if task == nil {
		t.Fatal("concurrent recovery returned no task")
	}
	replayed, err := reopened.RecoverWritingAgent(context.Background(), AgentRuntimeRecoveryRequest{Action: abortAction})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Task != task || replayed.Receipt.CommandID != abortAction.CommandID {
		t.Fatalf("repeated abort recovery = %#v task=%p", replayed, task)
	}
	select {
	case <-task.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("rehydrated interrupted Start did not settle after explicit abort")
	}
	status, ok = reopened.WritingAgentRuntimeProjection(context.Background())
	if !ok || status.Phase != agentrun.RunPhaseIdle || status.LastOperation == nil || status.LastOperation.Status != agentrun.OperationAborted {
		t.Fatalf("aborted cold recovery status = %#v projected=%t", status, ok)
	}
}

func runWritingRecoveryCrashSeed(t *testing.T) {
	t.Helper()
	application, err := New(context.Background(), &config.Config{
		OpenAIAPIKey: "unused", OpenAIModel: "test-model",
		NovaDir: os.Getenv("DENOVA_WRITING_RECOVERY_ROOT"), Workspace: os.Getenv("DENOVA_WRITING_RECOVERY_WORKSPACE"),
		ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	application.mu.RLock()
	sessionID := application.session.ID
	workspace := application.workspace
	application.mu.RUnlock()
	vanished := make(chan struct{})
	request := agentchat.ChatRequest{CommandID: "writing-recovery-start", Message: "persist before crash"}
	cycle, _, err := application.chat().prepareWritingCycle(context.Background(), request, "")
	if err != nil {
		t.Fatal(err)
	}
	cycle.Conversation = &interactiveCrashConversation{vanished: vanished}
	cycle.Options = agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: workspace, SessionID: sessionID, Mode: "ide"}
	cycle.Request = request
	if _, err := application.executionRuntime.Start(context.Background(), agentexecution.StartRequest{Cycle: cycle}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-vanished:
		os.Exit(0)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("writing recovery seed did not reach model context assembly")
	}
}
