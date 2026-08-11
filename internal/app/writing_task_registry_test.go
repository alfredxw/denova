package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentcontext "denova/internal/agents/context"
	apptask "denova/internal/app/task"
	"path/filepath"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agents "denova/internal/agents"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
)

func TestWritingOlderSettledStartColdReplayThroughApp(t *testing.T) {
	root := t.TempDir()
	workspace, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := New(context.Background(), writingColdReplayConfig(root))
	if err != nil {
		t.Fatal(err)
	}

	requests := []agentchat.ChatRequest{
		{CommandID: "writing-older-settled", Message: "write the first answer"},
		{CommandID: "writing-newer-settled", Message: "write the second answer"},
	}
	answers := []string{"first durable answer", "second durable answer"}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE,
		Workspace: workspace,
		SessionID: "default",
		Mode:      "ide",
	}
	for index := range requests {
		operation, startErr := startPublicExecutionCycle(
			seed.executionRuntime, context.Background(), writingColdReplayModel{answer: answers[index]},
			&interactiveReplayConversation{}, seed.bookService, requests[index], options, nil,
		)
		if startErr != nil {
			seed.Close()
			t.Fatal(startErr)
		}
		outcome := operation.Wait(context.Background())
		if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != answers[index] {
			seed.Close()
			t.Fatalf("seed run %d outcome = %#v", index, outcome)
		}
	}
	seed.Close()

	reopened, err := New(context.Background(), writingColdReplayConfig(root))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	// A settled replay is admitted before request preparation. Removing the
	// process-local model state makes the test fail if App accidentally tries to
	// reconstruct a Runner or Conversation instead of using the durable result.
	reopened.mu.Lock()
	reopened.bookState = nil
	reopened.mu.Unlock()

	task, err := reopened.StartTaskWithError(context.Background(), requests[0])
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-task.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("older settled Writing replay waited for future runtime events")
	}
	if task.Status() != apptask.Done {
		t.Fatalf("cold replay task status = %q, want %q", task.Status(), apptask.Done)
	}

	events, subscription := task.Subscribe()
	defer task.Unsubscribe(subscription)
	var replayedContent string
	var doneEvents int
	for _, event := range events {
		switch event.Event.Type {
		case "chunk":
			switch data := event.Event.Data.(type) {
			case map[string]string:
				replayedContent += data["content"]
			case map[string]any:
				content, _ := data["content"].(string)
				replayedContent += content
			}
		case "done":
			doneEvents++
		}
	}
	if replayedContent != answers[0] || doneEvents != 1 {
		t.Fatalf("older cold replay events = %#v, content=%q done=%d", events, replayedContent, doneEvents)
	}
}

func writingColdReplayConfig(root string) *config.Config {
	return &config.Config{
		OpenAIModel:         "test-model",
		NovaDir:             root,
		Workspace:           root,
		ResumeLastWorkspace: false,
	}
}

type writingColdReplayModel struct {
	answer string
}

// writingColdReplayConversation is the producer-side adapter only. The App
// replay deliberately receives neither this Conversation nor the test model.
type writingColdReplayConversation struct{}

func (writingColdReplayConversation) AssembleModelContext(
	ctx context.Context,
	_ string,
	input agentcontext.ModelContextInput,
) (agentcontext.ModelContextResult, error) {
	return agentcontext.AssembleSingleUserModelContext(ctx, input)
}

func (writingColdReplayConversation) AppendAssistant(string) error { return nil }

func (writingColdReplayConversation) MarkInterrupted(string, string, string) error { return nil }

func (writingColdReplayConversation) PendingInterruption() *session.Interruption { return nil }

func (writingColdReplayConversation) ResolveInterruption(string) error { return nil }

func (m writingColdReplayModel) Generate(context.Context, []*agents.Message, ...agent.ModelOption) (*agents.Message, error) {
	return agents.AssistantMessage(m.answer, nil), nil
}

func (m writingColdReplayModel) Stream(context.Context, []*agents.Message, ...agent.ModelOption) (*agents.StreamReader[*agents.Message], error) {
	return agents.StreamReaderFromArray([]*agents.Message{agents.AssistantMessage(m.answer, nil)}), nil
}
