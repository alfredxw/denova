package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"errors"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/interactive"
)

func TestColdRecoveryKeepsOneTaskAcrossConsecutiveQueuedPauses(t *testing.T) {
	if mode := os.Getenv("DENOVA_HOL_RECOVERY_SEED"); mode != "" {
		runConsecutiveRecoveryCrashSeed(t, mode)
		return
	}
	for _, mode := range []string{"writing", "game"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			workspace := t.TempDir()
			cfg := &config.Config{
				OpenAIAPIKey: "unused", OpenAIModel: "test-model",
				NovaDir: root, Workspace: workspace, ResumeLastWorkspace: false,
			}
			seed, err := New(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			storyID := ""
			if mode == "game" {
				story, createErr := seed.CreateInteractiveStory(interactive.CreateStoryRequest{Title: "queued recovery", StoryTellerID: "classic"})
				if createErr != nil {
					seed.Close()
					t.Fatal(createErr)
				}
				storyID = story.ID
			}
			seed.Close()

			command := exec.Command(os.Args[0], "-test.run=^TestColdRecoveryKeepsOneTaskAcrossConsecutiveQueuedPauses$/^"+mode+"$")
			command.Env = append(os.Environ(),
				"DENOVA_HOL_RECOVERY_SEED="+mode,
				"DENOVA_HOL_RECOVERY_ROOT="+root,
				"DENOVA_HOL_RECOVERY_WORKSPACE="+workspace,
				"DENOVA_HOL_RECOVERY_STORY="+storyID,
			)
			if output, runErr := command.CombinedOutput(); runErr != nil {
				t.Fatalf("%s crash seed failed: %v\n%s", mode, runErr, output)
			}

			reopened, err := New(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			var allowFollowUp atomic.Bool
			installConsecutiveRecoveryTestChat(t, reopened, root, &allowFollowUp)

			status, projected := consecutiveRecoveryProjection(reopened, mode, storyID)
			if !projected {
				t.Fatal("cold runtime projection unavailable")
			}
			actions := agentexecution.RuntimeRecoveryActions(status)
			if status.Phase != agentrun.RunPhaseRunning || !status.RecoveryPaused || len(actions) != 3 ||
				actions[2].Kind != agentexecution.RuntimeRecoverySteer || actions[2].CommandID != "hol-steer" {
				t.Fatalf("initial HOL actions = %#v status=%#v", actions, status)
			}
			first, err := recoverConsecutiveAction(reopened, mode, storyID, actions[2])
			if err != nil {
				t.Fatal(err)
			}
			waitForTaskEventType(t, first.Task, agentexecution.RuntimeRecoveryRequiredEventType)
			if first.Task.Finished() {
				t.Fatal("first recovered item closed the display Task at the next recovery boundary")
			}

			status, projected = consecutiveRecoveryProjection(reopened, mode, storyID)
			actions = agentexecution.RuntimeRecoveryActions(status)
			if !projected || status.InputRecovery == nil || len(actions) != 3 ||
				actions[2].Kind != agentexecution.RuntimeRecoveryFollowUp || actions[2].CommandID != "hol-follow-up" {
				t.Fatalf("second HOL actions = %#v status=%#v projected=%t", actions, status, projected)
			}
			allowFollowUp.Store(true)
			second, err := recoverConsecutiveAction(reopened, mode, storyID, actions[2])
			if err != nil {
				t.Fatal(err)
			}
			if second.Task != first.Task {
				t.Fatalf("second HOL item replaced display Task %p with %p", first.Task, second.Task)
			}
			waitInteractiveTask(t, first.Task)
			events, subscription := first.Task.Subscribe()
			defer first.Task.Unsubscribe(subscription)
			if countInteractiveTaskEvents(events, agentexecution.RuntimeRecoveryRequiredEventType) != 1 || countInteractiveTaskEvents(events, "done") != 1 {
				t.Fatalf("consecutive recovery task events = %#v", events)
			}
			status, projected = consecutiveRecoveryProjection(reopened, mode, storyID)
			if !projected || status.Phase != agentrun.RunPhaseIdle || status.LastOperation == nil ||
				status.LastOperation.Status != agentrun.OperationSucceeded || len(agentexecution.RuntimeRecoveryActions(status)) != 0 {
				t.Fatalf("consecutive recovery terminal status = %#v projected=%t", status, projected)
			}
		})
	}
}

func TestRecoveryTaskAbortRetriesFailOnceInputMaterialization(t *testing.T) {
	if os.Getenv("DENOVA_ABORT_RECOVERY_SEED") == "1" {
		runConsecutiveRecoveryCrashSeed(t, "writing")
		return
	}
	root := t.TempDir()
	workspace := t.TempDir()
	cfg := &config.Config{
		OpenAIAPIKey: "unused", OpenAIModel: "test-model",
		NovaDir: root, Workspace: workspace, ResumeLastWorkspace: false,
	}
	seed, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	seed.Close()
	command := exec.Command(os.Args[0], "-test.run=^TestRecoveryTaskAbortRetriesFailOnceInputMaterialization$")
	command.Env = append(os.Environ(),
		"DENOVA_ABORT_RECOVERY_SEED=1",
		"DENOVA_HOL_RECOVERY_ROOT="+root,
		"DENOVA_HOL_RECOVERY_WORKSPACE="+workspace,
	)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		t.Fatalf("abort crash seed failed: %v\n%s", runErr, output)
	}

	reopened, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var allowFollowUp atomic.Bool
	materializer := &failOnceInputMaterializer{
		delegate: profileInputMaterializerForTest{application: reopened}, target: "hol-follow-up",
	}
	installConsecutiveRecoveryTestChat(t, reopened, root, &allowFollowUp, materializer)
	status, projected := reopened.WritingAgentRuntimeProjection(context.Background())
	actions := agentexecution.RuntimeRecoveryActions(status)
	if !projected || len(actions) != 3 || actions[2].Kind != agentexecution.RuntimeRecoverySteer || actions[2].CommandID != "hol-steer" {
		t.Fatalf("initial abort recovery actions = %#v status=%#v projected=%t", actions, status, projected)
	}
	first, err := reopened.RecoverWritingAgent(context.Background(), AgentRuntimeRecoveryRequest{Action: actions[2]})
	if err != nil {
		t.Fatal(err)
	}
	waitForTaskEventType(t, first.Task, agentexecution.RuntimeRecoveryRequiredEventType)
	status, projected = reopened.WritingAgentRuntimeProjection(context.Background())
	if !projected || status.InputRecovery == nil || status.InputRecovery.CommandID != "hol-follow-up" {
		t.Fatalf("abort input-recovery boundary = %#v projected=%t", status, projected)
	}

	first.Task.Abort()
	select {
	case <-first.Task.Done():
	case <-time.After(time.Second):
		t.Fatal("Task.Abort did not durably settle fail-once input recovery")
	}
	if first.Task.Status() != apptask.Aborted {
		t.Fatalf("aborted recovery Task status = %s", first.Task.Status())
	}
	if got := materializer.targetCalls.Load(); got != 2 {
		t.Fatalf("Task.Abort input materialization attempts = %d, want fail plus exact replay", got)
	}
	status, projected = reopened.WritingAgentRuntimeProjection(context.Background())
	if !projected || status.Phase != agentrun.RunPhaseIdle || status.InputRecovery != nil ||
		status.LastOperation == nil || status.LastOperation.Status != agentrun.OperationAborted {
		t.Fatalf("Task.Abort terminal runtime = %#v projected=%t", status, projected)
	}
}

func runConsecutiveRecoveryCrashSeed(t *testing.T, mode string) {
	t.Helper()
	application, err := New(context.Background(), &config.Config{
		OpenAIAPIKey: "unused", OpenAIModel: "test-model",
		NovaDir: os.Getenv("DENOVA_HOL_RECOVERY_ROOT"), Workspace: os.Getenv("DENOVA_HOL_RECOVERY_WORKSPACE"),
		ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	application.mu.RLock()
	workspace := application.workspace
	sessionID := application.session.ID
	application.mu.RUnlock()
	options := agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: workspace, SessionID: sessionID, Mode: "ide"}
	if mode == "game" {
		options = agentrun.Options{
			AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace,
			StoryID: os.Getenv("DENOVA_HOL_RECOVERY_STORY"), BranchID: "main", Mode: "interactive",
		}
	}
	vanished := make(chan struct{})
	accepted, err := startExecutionCycle(application.executionRuntime,
		context.Background(), newInteractiveReplayRunner(t, &interactiveReplayModel{message: agents.AssistantMessage("must not run", nil)}),
		&interactiveCrashConversation{vanished: vanished}, application.bookService,
		agentchat.ChatRequest{CommandID: "hol-start", Message: "parent before crash"}, options, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-vanished:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("HOL crash seed did not reach model context")
	}
	operationID := accepted.Receipt().OperationID
	for _, command := range []agentexecution.CommandRequest{
		{
			Kind: agentexecution.CommandSteer, CommandID: "hol-steer", OperationID: operationID,
			Request: agentchat.ChatRequest{Message: "recover first"}, Options: options,
		},
		{
			Kind: agentexecution.CommandFollowUp, CommandID: "hol-follow-up", OperationID: operationID,
			Request: agentchat.ChatRequest{Message: "recover second"}, Options: options,
		},
	} {
		if _, err := application.executionRuntime.SubmitCommand(context.Background(), command); err != nil {
			t.Fatal(err)
		}
	}
	os.Exit(0)
}

func installConsecutiveRecoveryTestChat(
	t *testing.T,
	application *App,
	root string,
	allowFollowUp *atomic.Bool,
	materializers ...agentexecution.InputMaterializer,
) {
	t.Helper()
	application.mu.RLock()
	previous := application.executionRuntime
	application.mu.RUnlock()
	if previous != nil {
		if err := previous.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	runner := newInteractiveReplayRunner(t, &interactiveReplayModel{message: agents.AssistantMessage("recovered cycle", nil)})
	restored := func(_ context.Context, request agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error) {
		if request.CommandID == "hol-follow-up" && !allowFollowUp.Load() {
			return agentexecution.Cycle{}, errors.New("follow-up dependency temporarily unavailable")
		}
		return agentexecution.Cycle{Runner: runner, Conversation: &interactiveReplayConversation{}}, nil
	}
	var materializer agentexecution.InputMaterializer = profileInputMaterializerForTest{application: application}
	if len(materializers) > 0 && materializers[0] != nil {
		materializer = materializers[0]
	}
	profiles := application.executionProfiles()
	for index, profile := range profiles {
		domain, ok := profile.(agentexecution.DomainCommitProfile)
		if !ok {
			t.Fatalf("execution profile %q has no domain commit capability", profile.ID())
		}
		profiles[index] = &consecutiveRecoveryTestProfile{
			Profile: profile, domain: domain, prepare: restored, materializer: materializer,
		}
	}
	service, err := agentexecution.NewDurableRuntime(
		context.Background(), root,
		agentexecution.WithProfiles(profiles...),
	)
	if err != nil {
		t.Fatal(err)
	}
	application.mu.Lock()
	application.executionRuntime = service
	application.mu.Unlock()
}

type consecutiveRecoveryTestProfile struct {
	agentexecution.Profile
	domain       agentexecution.DomainCommitProfile
	prepare      func(context.Context, agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error)
	materializer agentexecution.InputMaterializer
}

func (profile *consecutiveRecoveryTestProfile) ReconcileDomainCommit(
	ctx context.Context,
	request agentrun.DomainCommitReconcileRequest,
) (agentrun.DomainCommitReconcileResult, error) {
	return profile.domain.ReconcileDomainCommit(ctx, request)
}

func (profile *consecutiveRecoveryTestProfile) PrepareCycle(
	ctx context.Context,
	request agentexecution.CycleRestoreRequest,
) (agentexecution.Cycle, error) {
	return profile.prepare(ctx, request)
}

func (profile *consecutiveRecoveryTestProfile) PlanInput(
	ctx context.Context,
	request agentexecution.InputMaterializationRequest,
) (agentrun.InputMaterializationPlan, error) {
	return profile.materializer.PlanInput(ctx, request)
}

func (profile *consecutiveRecoveryTestProfile) MaterializeInput(
	ctx context.Context,
	request agentexecution.InputMaterializationRequest,
	plan agentrun.InputMaterializationPlan,
) (agentrun.InputMaterializationReceipt, error) {
	return profile.materializer.MaterializeInput(ctx, request, plan)
}

type failOnceInputMaterializer struct {
	delegate agentexecution.InputMaterializer
	target   agentrun.CommandID

	targetCalls atomic.Int32
}

func (m *failOnceInputMaterializer) PlanInput(
	ctx context.Context,
	request agentexecution.InputMaterializationRequest,
) (agentrun.InputMaterializationPlan, error) {
	return m.delegate.PlanInput(ctx, request)
}

func (m *failOnceInputMaterializer) MaterializeInput(
	ctx context.Context,
	request agentexecution.InputMaterializationRequest,
	plan agentrun.InputMaterializationPlan,
) (agentrun.InputMaterializationReceipt, error) {
	if request.Identity.CommandID == m.target && m.targetCalls.Add(1) == 1 {
		return agentrun.InputMaterializationReceipt{}, errors.New("accepted input store failed once during Task.Abort")
	}
	return m.delegate.MaterializeInput(ctx, request, plan)
}

func consecutiveRecoveryProjection(application *App, mode, storyID string) (agentrun.RuntimeStatus, bool) {
	if mode == "game" {
		return application.InteractiveAgentRuntimeProjection(context.Background(), storyID, "main")
	}
	return application.WritingAgentRuntimeProjection(context.Background())
}

func recoverConsecutiveAction(application *App, mode, storyID string, action agentexecution.RuntimeRecoveryAction) (AgentRuntimeRecoveryResult, error) {
	request := AgentRuntimeRecoveryRequest{Action: action}
	if mode == "game" {
		request.StoryID = storyID
		request.BranchID = "main"
		return application.RecoverInteractiveAgent(context.Background(), request)
	}
	return application.RecoverWritingAgent(context.Background(), request)
}

func waitForTaskEventType(t *testing.T, task *apptask.Task, eventType string) {
	t.Helper()
	events, subscription := task.Subscribe()
	for _, event := range events {
		if event.Event.Type == eventType {
			task.Unsubscribe(subscription)
			return
		}
	}
	defer task.Unsubscribe(subscription)
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		select {
		case event, ok := <-subscription.Events():
			if !ok {
				t.Fatalf("task %s closed before emitting %s", task.ID(), eventType)
			}
			if event.Event.Type == eventType {
				return
			}
		case <-deadline.C:
			t.Fatalf("task %s did not emit %s", task.ID(), eventType)
		}
	}
}
