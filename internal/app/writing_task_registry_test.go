package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/session"
)

func TestWritingOlderSettledStartColdReplayThroughApp(t *testing.T) {
	root := t.TempDir()
	workspace, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	chatService, err := agent.NewDurableChatService(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}

	requests := []agent.ChatRequest{
		{CommandID: "writing-older-settled", Message: "write the first answer"},
		{CommandID: "writing-newer-settled", Message: "write the second answer"},
	}
	answers := []string{"first durable answer", "second durable answer"}
	options := agent.RunOptions{
		AgentKind: agent.AgentKindIDE,
		Workspace: workspace,
		SessionID: "default",
		Mode:      "ide",
	}
	for index := range requests {
		outcome := chatService.RunWithOptions(
			context.Background(),
			newWritingColdReplayRunner(t, answers[index]),
			writingColdReplayConversation{},
			nil,
			requests[index],
			options,
			nil,
		)
		if outcome.Status != agent.RunOutcomeCompleted || outcome.Content != answers[index] {
			_ = chatService.Close(context.Background())
			t.Fatalf("seed run %d outcome = %#v", index, outcome)
		}
	}
	if err := chatService.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

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
	reopened.agentRunner = nil
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
	if task.Status() != TaskDone {
		t.Fatalf("cold replay task status = %q, want %q", task.Status(), TaskDone)
	}

	events, subscription := task.Subscribe()
	defer task.Unsubscribe(subscription)
	var replayedContent string
	var doneEvents int
	for _, event := range events {
		switch event.Event.Type {
		case "chunk":
			if data, ok := event.Event.Data.(map[string]string); ok {
				replayedContent += data["content"]
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

func newWritingColdReplayRunner(t *testing.T, answer string) *adk.Runner {
	t.Helper()
	built, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        "DenovaAgent",
		Description: "Writing cold replay test",
		Instruction: "Return the fixed test answer.",
		Model:       writingColdReplayModel{answer: answer},
	})
	if err != nil {
		t.Fatal(err)
	}
	return adk.NewRunner(context.Background(), adk.RunnerConfig{Agent: built, EnableStreaming: true})
}

type writingColdReplayModel struct {
	answer string
}

// writingColdReplayConversation is the producer-side adapter only. The App
// replay deliberately receives neither this Conversation nor the test Runner.
type writingColdReplayConversation struct{}

func (writingColdReplayConversation) AssembleModelContext(
	ctx context.Context,
	_ string,
	input agent.ModelContextInput,
) (agent.ModelContextResult, error) {
	return agent.AssembleSingleUserModelContext(ctx, input)
}

func (writingColdReplayConversation) AppendAssistant(string) error { return nil }

func (writingColdReplayConversation) MarkInterrupted(string, string, string) error { return nil }

func (writingColdReplayConversation) PendingInterruption() *session.Interruption { return nil }

func (writingColdReplayConversation) ResolveInterruption(string) error { return nil }

func (m writingColdReplayModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage(m.answer, nil), nil
}

func (m writingColdReplayModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{schema.AssistantMessage(m.answer, nil)}), nil
}
