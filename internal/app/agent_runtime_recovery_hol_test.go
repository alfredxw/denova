package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"denova/config"
	"denova/internal/agent"
	runstate "denova/internal/agent/runtime"
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
			actions := agent.RuntimeRecoveryActions(status)
			if status.Phase != runstate.PhaseRunning || !status.RecoveryPaused || len(actions) != 3 ||
				actions[2].Kind != agent.RuntimeRecoverySteer || actions[2].CommandID != "hol-steer" {
				t.Fatalf("initial HOL actions = %#v status=%#v", actions, status)
			}
			first, err := recoverConsecutiveAction(reopened, mode, storyID, actions[2])
			if err != nil {
				t.Fatal(err)
			}
			waitForTaskEventType(t, first.Task, agent.RuntimeRecoveryRequiredEventType)
			if first.Task.Finished() {
				t.Fatal("first recovered item closed the display Task at the next recovery boundary")
			}

			status, projected = consecutiveRecoveryProjection(reopened, mode, storyID)
			actions = agent.RuntimeRecoveryActions(status)
			if !projected || status.InputRecovery == nil || len(actions) != 3 ||
				actions[2].Kind != agent.RuntimeRecoveryFollowUp || actions[2].CommandID != "hol-follow-up" {
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
			if countInteractiveTaskEvents(events, agent.RuntimeRecoveryRequiredEventType) != 1 || countInteractiveTaskEvents(events, "done") != 1 {
				t.Fatalf("consecutive recovery task events = %#v", events)
			}
			status, projected = consecutiveRecoveryProjection(reopened, mode, storyID)
			if !projected || status.Phase != runstate.PhaseIdle || status.LastOperation == nil ||
				status.LastOperation.Status != runstate.OperationSucceeded || len(agent.RuntimeRecoveryActions(status)) != 0 {
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
	materializer := &failOnceHarnessInputMaterializer{
		delegate: reopened, target: "hol-follow-up",
	}
	installConsecutiveRecoveryTestChat(t, reopened, root, &allowFollowUp, materializer)
	status, projected := reopened.WritingAgentRuntimeProjection(context.Background())
	actions := agent.RuntimeRecoveryActions(status)
	if !projected || len(actions) != 3 || actions[2].Kind != agent.RuntimeRecoverySteer || actions[2].CommandID != "hol-steer" {
		t.Fatalf("initial abort recovery actions = %#v status=%#v projected=%t", actions, status, projected)
	}
	first, err := reopened.RecoverWritingAgent(context.Background(), AgentRuntimeRecoveryRequest{Action: actions[2]})
	if err != nil {
		t.Fatal(err)
	}
	waitForTaskEventType(t, first.Task, agent.RuntimeRecoveryRequiredEventType)
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
	if first.Task.Status() != TaskAborted {
		t.Fatalf("aborted recovery Task status = %s", first.Task.Status())
	}
	if got := materializer.targetCalls.Load(); got != 2 {
		t.Fatalf("Task.Abort input materialization attempts = %d, want fail plus exact replay", got)
	}
	status, projected = reopened.WritingAgentRuntimeProjection(context.Background())
	if !projected || status.Phase != runstate.PhaseIdle || status.InputRecovery != nil ||
		status.LastOperation == nil || status.LastOperation.Status != runstate.OperationAborted {
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
	options := agent.RunOptions{AgentKind: agent.AgentKindIDE, Workspace: workspace, SessionID: sessionID, Mode: "ide"}
	if mode == "game" {
		options = agent.RunOptions{
			AgentKind: agent.AgentKindInteractiveStory, Workspace: workspace,
			StoryID: os.Getenv("DENOVA_HOL_RECOVERY_STORY"), BranchID: "main", Mode: "interactive",
		}
	}
	vanished := make(chan struct{})
	accepted, err := application.chatService.StartWithOptions(
		context.Background(), newInteractiveReplayRunner(t, &interactiveReplayModel{message: agent.AssistantMessage("must not run", nil)}),
		&interactiveCrashConversation{vanished: vanished}, application.bookService,
		agent.ChatRequest{CommandID: "hol-start", Message: "parent before crash"}, options, nil,
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
	prepareMustNotRun := func(context.Context) (agent.HarnessTurnExecution, error) {
		return agent.HarnessTurnExecution{}, errors.New("seed deferred preparation must not run")
	}
	for _, command := range []agent.AgentCommandSpec{
		{
			Kind: agent.AgentCommandSteer, CommandID: "hol-steer", OperationID: operationID,
			Request: agent.ChatRequest{Message: "recover first"}, Options: options, Prepare: prepareMustNotRun,
		},
		{
			Kind: agent.AgentCommandFollowUp, CommandID: "hol-follow-up", OperationID: operationID,
			Request: agent.ChatRequest{Message: "recover second"}, Options: options, Prepare: prepareMustNotRun,
		},
	} {
		if _, err := application.chatService.SubmitCommand(context.Background(), command); err != nil {
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
	materializers ...agent.HarnessInputMaterializer,
) {
	t.Helper()
	application.mu.RLock()
	previous := application.chatService
	application.mu.RUnlock()
	if previous != nil {
		if err := previous.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	runner := newInteractiveReplayRunner(t, &interactiveReplayModel{message: agent.AssistantMessage("recovered cycle", nil)})
	restored := func(_ context.Context, request agent.HarnessTurnRestoreRequest) (agent.HarnessTurnSpec, error) {
		if request.CommandID == "hol-follow-up" && !allowFollowUp.Load() {
			return agent.HarnessTurnSpec{}, errors.New("follow-up dependency temporarily unavailable")
		}
		return agent.HarnessTurnSpec{Runner: runner, Conversation: &interactiveReplayConversation{}}, nil
	}
	var materializer agent.HarnessInputMaterializer = application
	if len(materializers) > 0 && materializers[0] != nil {
		materializer = materializers[0]
	}
	service, err := agent.NewDurableChatService(
		context.Background(), root,
		agent.WithHarnessDomainCommitReconciler(application.reconcileHarnessDomainCommit),
		agent.WithHarnessInputMaterializer(materializer),
		agent.WithHarnessTurnRestorer(restored),
		agent.WithHarnessStructuralRestorer(application.restoreContextStructuralOperation),
	)
	if err != nil {
		t.Fatal(err)
	}
	application.mu.Lock()
	application.chatService = service
	application.mu.Unlock()
}

type failOnceHarnessInputMaterializer struct {
	delegate agent.HarnessInputMaterializer
	target   runstate.CommandID

	targetCalls atomic.Int32
}

func (m *failOnceHarnessInputMaterializer) PlanHarnessInputMaterialization(
	ctx context.Context,
	request agent.HarnessInputMaterializationRequest,
) (runstate.InputMaterializationPlan, error) {
	return m.delegate.PlanHarnessInputMaterialization(ctx, request)
}

func (m *failOnceHarnessInputMaterializer) MaterializeHarnessInput(
	ctx context.Context,
	request agent.HarnessInputMaterializationRequest,
	plan runstate.InputMaterializationPlan,
) (runstate.InputMaterializationReceipt, error) {
	if request.Identity.CommandID == m.target && m.targetCalls.Add(1) == 1 {
		return runstate.InputMaterializationReceipt{}, errors.New("accepted input store failed once during Task.Abort")
	}
	return m.delegate.MaterializeHarnessInput(ctx, request, plan)
}

func consecutiveRecoveryProjection(application *App, mode, storyID string) (runstate.StatusSnapshot, bool) {
	if mode == "game" {
		return application.InteractiveAgentRuntimeProjection(context.Background(), storyID, "main")
	}
	return application.WritingAgentRuntimeProjection(context.Background())
}

func recoverConsecutiveAction(application *App, mode, storyID string, action agent.RuntimeRecoveryAction) (AgentRuntimeRecoveryResult, error) {
	request := AgentRuntimeRecoveryRequest{Action: action}
	if mode == "game" {
		request.StoryID = storyID
		request.BranchID = "main"
		return application.RecoverInteractiveAgent(context.Background(), request)
	}
	return application.RecoverWritingAgent(context.Background(), request)
}

func waitForTaskEventType(t *testing.T, task *Task, eventType string) {
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
