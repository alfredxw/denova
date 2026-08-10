package app

import (
	"context"
	agentcontext "denova/internal/agents/context"
	agentexecution "denova/internal/agents/execution"
	apptask "denova/internal/app/task"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agents "denova/internal/agents"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/interactive"
)

func TestInteractiveStartRegistryRequiresIdentityAndReplaysExactSettledTask(t *testing.T) {
	service, request := newInteractiveStartRegistryFixture(t)
	if task, err := service.StartInteractiveTaskWithError(context.Background(), InteractiveAgentStartRequest{
		StoryID: request.StoryID, BranchID: request.BranchID, Message: request.Message,
	}); !errors.Is(err, ErrAgentCommandIDRequired) || task != nil {
		t.Fatalf("missing command id = task=%v err=%v", task, err)
	}

	identity, err := service.resolveInteractiveStart(request)
	if err != nil {
		t.Fatal(err)
	}
	original := apptask.New(func(context.Context, *apptask.Task, func(agentrun.Event)) {})
	waitInteractiveTask(t, original)
	if err := service.starts.remember(identity, original); err != nil {
		t.Fatal(err)
	}

	replayed, err := service.StartInteractiveTaskWithError(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if replayed != original {
		t.Fatalf("same command returned task %p, want exact original %p", replayed, original)
	}

	changed := request
	changed.Message = "走向另一条路"
	if task, err := service.StartInteractiveTaskWithError(context.Background(), changed); !errors.Is(err, ErrAgentCommandConflict) || task != nil {
		t.Fatalf("changed command payload = task=%v err=%v", task, err)
	}
}

func TestInteractiveStartRegistrySerializesConcurrentExactReplay(t *testing.T) {
	service, request := newInteractiveStartRegistryFixture(t)
	identity, err := service.resolveInteractiveStart(request)
	if err != nil {
		t.Fatal(err)
	}
	original := apptask.New(func(context.Context, *apptask.Task, func(agentrun.Event)) {})
	waitInteractiveTask(t, original)
	if err := service.starts.remember(identity, original); err != nil {
		t.Fatal(err)
	}

	const callers = 16
	results := make(chan *apptask.Task, callers)
	errorsCh := make(chan error, callers)
	var start sync.WaitGroup
	start.Add(1)
	for range callers {
		runAppErrorTestGoroutine(errorsCh, "concurrent interactive task replay", func() error {
			start.Wait()
			task, err := service.StartInteractiveTaskWithError(context.Background(), request)
			results <- task
			return err
		})
	}
	start.Done()
	for range callers {
		err := <-errorsCh
		if err != nil {
			t.Fatal(err)
		}
	}
	close(results)
	for task := range results {
		if task != original {
			t.Fatalf("concurrent retry returned task %p, want %p", task, original)
		}
	}
}

func TestInteractiveInitialStartColdReplayBuildsBoundedTaskWithoutGameCycle(t *testing.T) {
	root := t.TempDir()
	workspace := t.TempDir()
	cfg := config.Config{
		OpenAIAPIKey: "unused", OpenAIModel: "test-model",
		NovaDir: root, Workspace: workspace, ResumeLastWorkspace: false,
	}
	first, err := New(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	story, err := first.CreateInteractiveStory(interactive.CreateStoryRequest{Title: "cold replay", StoryTellerID: "classic"})
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	request := InteractiveAgentStartRequest{
		CommandID: "game-cold-success", StoryID: story.ID, BranchID: "main",
		Message: "推开石门", StyleScenes: []string{"雨夜"}, Locale: "zh-CN",
	}
	identity, err := first.interactiveService().resolveInteractiveStart(request)
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	chatModel := &interactiveReplayModel{message: agents.AssistantMessage("持久化回答", nil)}
	accepted, err := startExecutionCycle(first.executionRuntime,
		context.Background(), newInteractiveReplayRunner(t, chatModel), &interactiveReplayConversation{},
		first.bookService, identity.chatRequest, identity.options("seed-display-task"), nil,
	)
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	outcome := accepted.Wait(context.Background())
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "持久化回答" {
		first.Close()
		t.Fatalf("seed outcome = %#v", outcome)
	}
	newerRequest := request
	newerRequest.CommandID = "game-newer-success"
	newerRequest.Message = "查看火把"
	newerIdentity, err := first.interactiveService().resolveInteractiveStart(newerRequest)
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	newer, err := startExecutionCycle(first.executionRuntime,
		context.Background(), newInteractiveReplayRunner(t, chatModel), &interactiveReplayConversation{},
		first.bookService, newerIdentity.chatRequest, newerIdentity.options("newer-display-task"), nil,
	)
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	if newerOutcome := newer.Wait(context.Background()); newerOutcome.Status != agentrun.OutcomeCompleted {
		first.Close()
		t.Fatalf("newer seed outcome = %#v", newerOutcome)
	}
	if got := chatModel.calls.Load(); got != 2 {
		first.Close()
		t.Fatalf("seed model calls = %d, want 2", got)
	}
	first.Close()

	reopenCfg := cfg
	second, err := New(context.Background(), &reopenCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	runnerBefore := second.interactiveStoryRunner
	task, err := second.StartInteractiveTaskWithError(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	waitInteractiveTask(t, task)
	if second.interactiveStoryRunner != runnerBefore {
		t.Fatal("durable replay prepared a new Game cycle")
	}
	if got := chatModel.calls.Load(); got != 2 {
		t.Fatalf("cold replay invoked the seed provider: calls=%d", got)
	}
	events, subscription := task.Subscribe()
	defer task.Unsubscribe(subscription)
	if countInteractiveTaskEvents(events, "chunk") != 1 || countInteractiveTaskEvents(events, "done") != 1 {
		t.Fatalf("cold replay events = %#v", events)
	}
	snapshot, err := second.InteractiveSnapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 0 {
		t.Fatalf("display replay created a fake Story turn: %#v", snapshot.Turns)
	}
	if len(snapshot.PendingPlayerInputs) != 2 {
		t.Fatalf("cold replay duplicated or lost accepted player input: %#v", snapshot.PendingPlayerInputs)
	}

	changed := request
	changed.Message = "改变请求"
	if duplicate, err := second.StartInteractiveTaskWithError(context.Background(), changed); !errors.Is(err, ErrAgentCommandConflict) || duplicate != nil {
		t.Fatalf("cold changed payload = task=%v err=%v", duplicate, err)
	}
}

func TestInteractiveStatusOwnsInterruptedCommand(t *testing.T) {
	status := agentrun.RuntimeStatus{
		LastOperation: &agentrun.OperationSummary{CommandID: "newer", Status: agentrun.OperationSucceeded},
		RecentOperations: []agentrun.OperationSummary{{
			CommandID: "game-cold-interrupted", Status: agentrun.OperationInterrupted,
		}},
	}
	if !interactiveStatusOwnsCommand(status, "game-cold-interrupted") {
		t.Fatal("interrupted durable start was not recognized as replayable")
	}
}

func TestInteractiveInitialStartColdInterruptedReplayDoesNotRunGameCycle(t *testing.T) {
	if os.Getenv("DENOVA_GAME_CRASH_SEED") == "1" {
		runInteractiveCrashSeed(t)
		return
	}

	root := t.TempDir()
	workspace := t.TempDir()
	cfg := config.Config{
		OpenAIAPIKey: "unused", OpenAIModel: "test-model",
		NovaDir: root, Workspace: workspace, ResumeLastWorkspace: false,
	}
	seed, err := New(context.Background(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	story, err := seed.CreateInteractiveStory(interactive.CreateStoryRequest{Title: "cold interrupted", StoryTellerID: "classic"})
	if err != nil {
		seed.Close()
		t.Fatal(err)
	}
	seed.Close()

	command := exec.Command(os.Args[0], "-test.run=^TestInteractiveInitialStartColdInterruptedReplayDoesNotRunGameCycle$")
	command.Env = append(os.Environ(),
		"DENOVA_GAME_CRASH_SEED=1",
		"DENOVA_GAME_CRASH_ROOT="+root,
		"DENOVA_GAME_CRASH_WORKSPACE="+workspace,
		"DENOVA_GAME_CRASH_STORY="+story.ID,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("crash seed failed: %v\n%s", err, output)
	}

	reopenCfg := cfg
	reopened, err := New(context.Background(), &reopenCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	runnerBefore := reopened.interactiveStoryRunner
	status, projected := reopened.InteractiveAgentRuntimeProjection(context.Background(), story.ID, "main")
	if !projected {
		t.Fatal("cold Game runtime projection unavailable")
	}
	actions := agentexecution.RuntimeRecoveryActions(status)
	if status.Phase != agentrun.RunPhaseRunning || !status.RecoveryPaused || len(actions) != 2 ||
		actions[0].Kind != agentexecution.RuntimeRecoveryAttach || actions[0].CommandID != "game-cold-interrupted" ||
		actions[1].Kind != agentexecution.RuntimeRecoveryAbort {
		t.Fatalf("cold Game recovery actions = %#v status=%#v", actions, status)
	}
	result, err := reopened.RecoverInteractiveAgent(context.Background(), AgentRuntimeRecoveryRequest{
		Action: actions[1], StoryID: story.ID, BranchID: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := reopened.RecoverInteractiveAgent(context.Background(), AgentRuntimeRecoveryRequest{
		Action: actions[1], StoryID: story.ID, BranchID: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Task != result.Task || replayed.Receipt.CommandID != actions[1].CommandID {
		t.Fatalf("repeated cold Game abort = %#v first_task=%p", replayed, result.Task)
	}
	activeTask, info := reopened.ActiveInteractiveTaskFor(story.ID, "main")
	if activeTask != result.Task || info.StoryID != story.ID || info.BranchID != "main" ||
		info.Message != "记住这次行动" || info.CommandID != string(actions[1].CommandID) {
		t.Fatalf("server-derived cold Game display identity task=%p info=%#v", activeTask, info)
	}
	waitInteractiveTask(t, result.Task)
	if reopened.interactiveStoryRunner != runnerBefore {
		t.Fatal("cold recovery abort prepared a new Game cycle")
	}
	events, subscription := result.Task.Subscribe()
	defer result.Task.Unsubscribe(subscription)
	if countInteractiveTaskEvents(events, "aborted") != 1 {
		t.Fatalf("cold Game abort events = %#v", events)
	}
	snapshot, err := reopened.InteractiveSnapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Turns) != 0 {
		t.Fatalf("interrupted replay created a fake Story turn: %#v", snapshot.Turns)
	}
	if len(snapshot.PendingPlayerInputs) != 1 || snapshot.PendingPlayerInputs[0].Text != "记住这次行动" {
		t.Fatalf("accepted player input was not independently durable: %#v", snapshot.PendingPlayerInputs)
	}
	status, projected = reopened.InteractiveAgentRuntimeProjection(context.Background(), story.ID, "main")
	if !projected || status.Phase != agentrun.RunPhaseIdle || status.LastOperation == nil || status.LastOperation.Status != agentrun.OperationAborted || len(agentexecution.RuntimeRecoveryActions(status)) != 0 {
		t.Fatalf("cold Game abort terminal projection = %#v projected=%t", status, projected)
	}
}

func runInteractiveCrashSeed(t *testing.T) {
	t.Helper()
	application, err := New(context.Background(), &config.Config{
		OpenAIAPIKey: "unused", OpenAIModel: "test-model",
		NovaDir:   os.Getenv("DENOVA_GAME_CRASH_ROOT"),
		Workspace: os.Getenv("DENOVA_GAME_CRASH_WORKSPACE"), ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := InteractiveAgentStartRequest{
		CommandID: "game-cold-interrupted", StoryID: os.Getenv("DENOVA_GAME_CRASH_STORY"),
		BranchID: "main", Message: "记住这次行动", Locale: "zh-CN",
	}
	identity, err := application.interactiveService().resolveInteractiveStart(request)
	if err != nil {
		t.Fatal(err)
	}
	vanished := make(chan struct{})
	conversation := &interactiveCrashConversation{vanished: vanished}
	if _, err := startExecutionCycle(application.executionRuntime,
		context.Background(), newInteractiveReplayRunner(t, &interactiveReplayModel{message: agents.AssistantMessage("must not run", nil)}),
		conversation, application.bookService, identity.chatRequest, identity.options("crashed-display-task"), nil,
	); err != nil {
		t.Fatal(err)
	}
	select {
	case <-vanished:
		os.Exit(0)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("crash seed engine did not reach model-context assembly")
	}
}

func newInteractiveStartRegistryFixture(t *testing.T) (*InteractiveAppService, InteractiveAgentStartRequest) {
	t.Helper()
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "registry", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	application := &App{workspace: workspace, interactive: store}
	service := &InteractiveAppService{app: application}
	return service, InteractiveAgentStartRequest{
		CommandID: "game-registry-command", StoryID: story.ID, BranchID: "main",
		Message: "向前走", StyleScenes: []string{"雨夜"}, Locale: "zh-CN",
	}
}

func waitInteractiveTask(t *testing.T, task *apptask.Task) {
	t.Helper()
	select {
	case <-task.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("task %s did not finish", task.ID())
	}
}

func countInteractiveTaskEvents(events []apptask.Event, eventType string) int {
	count := 0
	for _, event := range events {
		if event.Event.Type == eventType {
			count++
		}
	}
	return count
}

type interactiveReplayConversation struct{}

func (*interactiveReplayConversation) AssembleModelContext(ctx context.Context, _ string, input agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	return agentcontext.AssembleSingleUserModelContext(ctx, input)
}

func (*interactiveReplayConversation) AppendAssistant(string) error { return nil }

func (*interactiveReplayConversation) MarkInterrupted(string, string, string) error { return nil }

func (*interactiveReplayConversation) PendingInterruption() *session.Interruption { return nil }

func (*interactiveReplayConversation) ResolveInterruption(string) error { return nil }

type interactiveCrashConversation struct {
	vanished chan struct{}
}

func (c *interactiveCrashConversation) AssembleModelContext(context.Context, string, agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	close(c.vanished)
	runtime.Goexit()
	return agentcontext.ModelContextResult{}, nil
}

func (*interactiveCrashConversation) AppendAssistant(string) error { return nil }

func (*interactiveCrashConversation) MarkInterrupted(string, string, string) error { return nil }

func (*interactiveCrashConversation) PendingInterruption() *session.Interruption { return nil }

func (*interactiveCrashConversation) ResolveInterruption(string) error { return nil }

type interactiveReplayModel struct {
	message *agents.Message
	calls   atomic.Int32
}

func (m *interactiveReplayModel) Generate(context.Context, []*agents.Message, ...agent.ModelOption) (*agents.Message, error) {
	m.calls.Add(1)
	return m.message, nil
}

func (m *interactiveReplayModel) Stream(context.Context, []*agents.Message, ...agent.ModelOption) (*agents.StreamReader[*agents.Message], error) {
	m.calls.Add(1)
	return agents.StreamReaderFromArray([]*agents.Message{m.message}), nil
}

func newInteractiveReplayRunner(t *testing.T, chatModel agent.BaseChatModel) *agent.Runner {
	t.Helper()
	built, err := agent.NewAgent(context.Background(), agent.AgentConfig{
		Name: "DenovaInteractiveStoryAgent", Description: "game replay test", Instruction: "test", Model: chatModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	return agent.NewRunner(agent.RunnerConfig{Agent: built, EnableStreaming: true})
}

var _ agent.BaseChatModel = (*interactiveReplayModel)(nil)
