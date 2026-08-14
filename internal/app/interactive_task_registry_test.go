package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	apptask "denova/internal/app/task"
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
		if err := <-errorsCh; err != nil {
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

func newInteractiveStartRegistryFixture(t *testing.T) (*InteractiveAppService, InteractiveAgentStartRequest) {
	t.Helper()
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "registry", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	application := &App{workspace: workspace, interactive: store}
	return &InteractiveAppService{app: application}, InteractiveAgentStartRequest{
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

type interactiveReplayConversation struct {
	store    *interactive.Store
	storyID  string
	branchID string
	message  string
}

func (*interactiveReplayConversation) AssembleModelContext(ctx context.Context, _ string, input agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	return agentcontext.AssembleSingleUserModelContext(ctx, input)
}

func (*interactiveReplayConversation) AppendAssistant(string) error { return nil }

func (*interactiveReplayConversation) MarkInterrupted(string, string, string) error { return nil }

func (*interactiveReplayConversation) PendingInterruption() *session.Interruption { return nil }

func (*interactiveReplayConversation) ResolveInterruption(string) error { return nil }
