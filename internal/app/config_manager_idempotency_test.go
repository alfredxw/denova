package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/session"
	runstate "github.com/alfredxw/denova/agent/runtime"
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

	request := ConfigManagerRequest{
		CommandID:   "config-manager-same-start",
		Instruction: "update the selected resource",
		Origin:      "settings", ResourceID: "resource-1",
		Context: map[string]string{"kind": "teller"},
	}
	first, err := application.StartConfigManagerTaskWithError(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := application.StartConfigManagerTaskWithError(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if replayed != first || replayed.ID() != first.ID() {
		t.Fatalf("same Config Manager command did not return the exact Task: first=%p/%s replay=%p/%s", first, first.ID(), replayed, replayed.ID())
	}

	conflict := request
	conflict.Instruction = "delete the selected resource"
	if task, err := application.StartConfigManagerTaskWithError(context.Background(), conflict); task != nil || !errors.Is(err, ErrAgentCommandConflict) {
		t.Fatalf("different Config Manager payload reuse = task=%v err=%v", task, err)
	}
}

func TestConfigManagerInitialStartRequiresCallerCommandID(t *testing.T) {
	service := &ConfigManagerAppService{}
	if task, err := service.StartTaskWithError(context.Background(), ConfigManagerRequest{Instruction: "update"}); task != nil || !errors.Is(err, ErrAgentCommandIDRequired) {
		t.Fatalf("missing command_id = task=%v err=%v", task, err)
	}
}

func TestConfigManagerInitialStartRejectsOversizedCommandIDBeforeWorkspaceAccess(t *testing.T) {
	service := &ConfigManagerAppService{}
	request := ConfigManagerRequest{CommandID: strings.Repeat("x", 4097), Instruction: "update"}
	if task, err := service.StartTaskWithError(context.Background(), request); task != nil || !errors.Is(err, runstate.ErrInvalidCommand) {
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

	blocker, err := NewDeferredRegisteredTask(nil)
	if err != nil {
		t.Fatal(err)
	}
	application.activeTaskReplay.byteLimit = blocker.displayReplayRegistryCharge()
	reservation, err := application.activeTaskReplay.reserve(blocker)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		reservation.release()
		blocker.failBeforeStart(errors.New("test cleanup"))
	})

	request := ConfigManagerRequest{
		CommandID: "config-capacity-pre-admission", Instruction: "update settings",
		Origin: "settings", ResourceID: "resource-capacity",
	}
	if task, err := application.StartConfigManagerTaskWithError(context.Background(), request); task != nil || !errors.Is(err, ErrAgentReplayCapacity) {
		t.Fatalf("capacity start = task=%v err=%v, want pre-admission capacity rejection", task, err)
	}

	sessionID, err := configManagerSessionID(request)
	if err != nil {
		t.Fatal(err)
	}
	application.mu.RLock()
	workspace := application.workspace
	chatService := application.chatService
	application.mu.RUnlock()
	status, err := chatService.RuntimeStatusProjection(context.Background(), agents.RunOptions{
		AgentKind: agents.AgentKindConfigManager, SessionID: sessionID,
		Workspace: workspace, Mode: "config_manager",
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != runstate.PhaseIdle || status.ActiveOperation != "" || status.LastOperation != nil || len(status.Queue) != 0 {
		t.Fatalf("Runtime was mutated before capacity admission: %#v", status)
	}
}

func TestConfigManagerOlderSettledStartColdReplayWithoutModel(t *testing.T) {
	root := t.TempDir()
	workspace, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	service, err := agents.NewDurableChatService(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	requests := []ConfigManagerRequest{
		{CommandID: "config-older-settled", Instruction: "first update"},
		{CommandID: "config-newer-settled", Instruction: "second update"},
	}
	answers := []string{"first config answer", "second config answer"}
	sessionID, err := configManagerSessionID(requests[0])
	if err != nil {
		t.Fatal(err)
	}
	options := agents.RunOptions{
		AgentKind: agents.AgentKindConfigManager, Workspace: workspace,
		SessionID: sessionID, Mode: "config_manager",
	}
	for index := range requests {
		chatRequest := agents.ChatRequest{
			CommandID: requests[index].CommandID,
			Message:   buildConfigManagerMessage(requests[index]),
		}
		outcome := service.RunWithOptions(
			context.Background(), newConfigManagerColdReplayRunner(t, answers[index]),
			configManagerColdReplayConversation{}, nil, chatRequest, options, nil,
		)
		if outcome.Status != agents.RunOutcomeCompleted || outcome.Content != answers[index] {
			_ = service.Close(context.Background())
			t.Fatalf("seed run %d outcome = %#v", index, outcome)
		}
	}
	if err := service.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	application, err := New(context.Background(), &config.Config{
		OpenAIModel: "test-model", NovaDir: root, Workspace: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	application.mu.Lock()
	application.bookState = nil
	application.sessionStore = nil
	application.agentRunner = nil
	application.mu.Unlock()

	task, err := application.StartConfigManagerTaskWithError(context.Background(), requests[0])
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-task.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("older settled Config Manager replay waited for future runtime events")
	}
	events, subscription := task.Subscribe()
	defer task.Unsubscribe(subscription)
	var content string
	for _, event := range events {
		if event.Event.Type == "chunk" {
			if data, ok := event.Event.Data.(map[string]string); ok {
				content += data["content"]
			}
		}
	}
	if task.Status() != TaskDone || content != answers[0] {
		t.Fatalf("cold replay status=%s content=%q events=%#v", task.Status(), content, events)
	}
}

func newConfigManagerColdReplayRunner(t *testing.T, answer string) *agent.Runner {
	t.Helper()
	built, err := agent.NewAgent(context.Background(), agent.AgentConfig{
		Name: "DenovaConfigManagerAgent", Description: "Config replay test",
		Instruction: "Return the fixed answer.", Model: configManagerColdReplayModel{answer: answer},
	})
	if err != nil {
		t.Fatal(err)
	}
	return agent.NewRunner(agent.RunnerConfig{Agent: built, EnableStreaming: true})
}

type configManagerColdReplayModel struct{ answer string }

func (m configManagerColdReplayModel) Generate(context.Context, []*agents.Message, ...agent.ModelOption) (*agents.Message, error) {
	return agents.AssistantMessage(m.answer, nil), nil
}

func (m configManagerColdReplayModel) Stream(context.Context, []*agents.Message, ...agent.ModelOption) (*agents.StreamReader[*agents.Message], error) {
	return agents.StreamReaderFromArray([]*agents.Message{agents.AssistantMessage(m.answer, nil)}), nil
}

type configManagerColdReplayConversation struct{}

func (configManagerColdReplayConversation) AssembleModelContext(ctx context.Context, _ string, input agents.ModelContextInput) (agents.ModelContextResult, error) {
	return agents.AssembleSingleUserModelContext(ctx, input)
}
func (configManagerColdReplayConversation) AppendAssistant(string) error { return nil }
func (configManagerColdReplayConversation) MarkInterrupted(string, string, string) error {
	return nil
}
func (configManagerColdReplayConversation) PendingInterruption() *session.Interruption { return nil }
func (configManagerColdReplayConversation) ResolveInterruption(string) error           { return nil }
