package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentcontext "denova/internal/agents/context"
	agentexecution "denova/internal/agents/execution"
	apptask "denova/internal/app/task"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agents "denova/internal/agents"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	configmanagerapp "denova/internal/app/configmanager"
	projectdomain "denova/internal/project"

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

func TestConfigManagerOlderSettledStartColdReplayWithoutModel(t *testing.T) {
	root := t.TempDir()
	workspace, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	service, err := agentexecution.NewDurableRuntime(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	registry := projectdomain.NewRegistry(root)
	record, err := registry.EnsureBook(workspace)
	if err != nil {
		t.Fatal(err)
	}
	requests := []configmanagerapp.Request{
		{ProjectID: record.ID, CommandID: "config-older-settled", Instruction: "first update"},
		{ProjectID: record.ID, CommandID: "config-newer-settled", Instruction: "second update"},
	}
	answers := []string{"first config answer", "second config answer"}
	sessionID, err := configmanagerapp.SessionID(requests[0])
	if err != nil {
		t.Fatal(err)
	}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindConfigManager, ProjectID: record.ID, Workspace: workspace,
		SessionID: sessionID, Mode: "config_manager",
	}
	for index := range requests {
		chatRequest := agentchat.ChatRequest{
			CommandID: requests[index].CommandID,
			Message:   configmanagerapp.BuildMessage(requests[index]),
		}
		outcome := runExecutionCycle(service,
			context.Background(), newConfigManagerColdReplayRunner(t, answers[index]),
			configManagerColdReplayConversation{}, nil, chatRequest, options, nil,
		)
		if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != answers[index] {
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

	task, err := application.ConfigManager().StartTaskWithError(context.Background(), requests[0])
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
	if task.Status() != apptask.Done || content != answers[0] {
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

func (configManagerColdReplayConversation) AssembleModelContext(ctx context.Context, _ string, input agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	return agentcontext.AssembleSingleUserModelContext(ctx, input)
}
func (configManagerColdReplayConversation) AppendAssistant(string) error { return nil }
func (configManagerColdReplayConversation) MarkInterrupted(string, string, string) error {
	return nil
}
func (configManagerColdReplayConversation) PendingInterruption() *session.Interruption { return nil }
func (configManagerColdReplayConversation) ResolveInterruption(string) error           { return nil }
