package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

type taskModel struct {
	mu        sync.Mutex
	responses []*agent.Message
}

type blockingTaskModel struct {
	release  <-chan struct{}
	response string
}

type notifyingTaskExecutor struct {
	*LocalTasks
	waiting chan struct{}
	once    sync.Once
}

func (executor *notifyingTaskExecutor) Wait(ctx context.Context, refs []TaskRef) ([]TaskWaitOutcome, error) {
	executor.once.Do(func() { close(executor.waiting) })
	return executor.LocalTasks.Wait(ctx, refs)
}

func (model *blockingTaskModel) Generate(ctx context.Context, _ []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	return model.next(ctx)
}

func (model *blockingTaskModel) Stream(ctx context.Context, _ []*agent.Message, _ ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.next(ctx)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}

func (model *blockingTaskModel) next(ctx context.Context) (*agent.Message, error) {
	select {
	case <-model.release:
		return agent.AssistantMessage(model.response, nil), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (model *taskModel) Generate(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.Message, error) {
	return model.next()
}

func (model *taskModel) Stream(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.next()
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}

func (model *taskModel) next() (*agent.Message, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	if len(model.responses) == 0 {
		return nil, fmt.Errorf("task model exhausted")
	}
	message := model.responses[0].Clone()
	model.responses = model.responses[1:]
	return message, nil
}

func newTaskAgent(t *testing.T, store agentsession.Store, model agent.BaseChatModel) *agent.Agent {
	t.Helper()
	owner, err := agent.New(context.Background(), agent.Definition{
		Name: "researcher", Model: model,
		ModelIdentity: agent.CapabilityIdentity{Kind: "test.task.model", Version: 1},
	}, agent.WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func newTaskExecutor(t *testing.T, owner *agent.Agent) *LocalTasks {
	t.Helper()
	executor, err := NewLocalTasks(LocalTaskOptions{Parallelism: 4}, LocalTaskAgent{
		Name: "researcher", Description: "Researches one bounded question", Opener: owner,
		Identity: agent.CapabilityIdentity{Kind: "test.task.researcher", Version: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func TestLocalTasksObserveFinalOutputFromColdAgent(t *testing.T) {
	store := agentsession.Memory()
	first := newTaskAgent(t, store, &taskModel{responses: []*agent.Message{agent.AssistantMessage("cold durable answer", nil)}})
	executor := newTaskExecutor(t, first)
	task, err := executor.Start(context.Background(), TaskRequest{
		Agent: "researcher", Prompt: "research", IdempotencyKey: "stable-task",
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := first.Session(context.Background(), agent.SessionKey{
		Namespace: "task.researcher", ID: task.Ref.Session, Attributes: map[string]string{"agent": "researcher"},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, found, err := session.AttachRun(context.Background(), task.Ref.Run)
	if err != nil || !found {
		t.Fatalf("attach running task found=%t err=%v", found, err)
	}
	if _, err := run.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	cold := newTaskAgent(t, store, &taskModel{})
	observation, err := newTaskExecutor(t, cold).Observe(context.Background(), task.Ref, "0")
	if err != nil {
		t.Fatal(err)
	}
	if observation.Task.Status != string(agent.ResultCompleted) || observation.Output != "cold durable answer" {
		t.Fatalf("cold observation = %#v", observation)
	}
	if len(observation.Events) != 0 || observation.Cursor == "" || observation.Incomplete {
		t.Fatalf("cold transcript observation events=%#v cursor=%q incomplete=%t", observation.Events, observation.Cursor, observation.Incomplete)
	}
	if err := cold.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	// A reconnecting caller commonly persists the head cursor before losing
	// its process. The final result must remain recoverable even though replay
	// has no events after that cursor.
	reopened := newTaskAgent(t, store, &taskModel{})
	defer reopened.Close(context.Background())
	atHead, err := newTaskExecutor(t, reopened).Observe(context.Background(), task.Ref, observation.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(atHead.Events) != 0 || atHead.Output != "cold durable answer" ||
		atHead.Task.Status != string(agent.ResultCompleted) {
		t.Fatalf("cold head observation = %#v", atHead)
	}
}

func TestLocalTasksReopensPriorRouteAfterParentCycleChanges(t *testing.T) {
	store := agentsession.Memory()
	first := newTaskAgent(t, store, &taskModel{responses: []*agent.Message{agent.AssistantMessage("route-stable", nil)}})
	firstExecutor, err := NewLocalTasks(LocalTaskOptions{Parallelism: 4}, LocalTaskAgent{
		Name: "researcher", Description: "Research", Opener: first,
		Identity:         agent.CapabilityIdentity{Kind: "test.task.route", Version: 1},
		Attributes:       map[string]string{"parent": "stable", "route": "cycle-one"},
		LookupAttributes: map[string]string{"parent": "stable"},
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := firstExecutor.Start(context.Background(), TaskRequest{
		Agent: "researcher", Prompt: "research", IdempotencyKey: "stable-route",
	})
	if err != nil {
		t.Fatal(err)
	}
	firstSession, err := first.Session(context.Background(), agent.SessionKey{
		Namespace: "task.researcher", ID: task.Ref.Session,
		Attributes: map[string]string{"agent": "researcher", "parent": "stable", "route": "cycle-one"},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, found, err := firstSession.AttachRun(context.Background(), task.Ref.Run)
	if err != nil || !found {
		t.Fatalf("attach first route found=%t err=%v", found, err)
	}
	if _, err := run.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	cold := newTaskAgent(t, store, &taskModel{})
	defer cold.Close(context.Background())
	secondExecutor, err := NewLocalTasks(LocalTaskOptions{Parallelism: 4}, LocalTaskAgent{
		Name: "researcher", Description: "Research", Opener: cold,
		Identity:         agent.CapabilityIdentity{Kind: "test.task.route", Version: 1},
		Attributes:       map[string]string{"parent": "stable", "route": "cycle-two"},
		LookupAttributes: map[string]string{"parent": "stable"},
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := secondExecutor.Observe(context.Background(), task.Ref, "0")
	if err != nil {
		t.Fatal(err)
	}
	if observation.Output != "route-stable" || observation.Task.Status != string(agent.ResultCompleted) {
		t.Fatalf("prior route observation = %#v", observation)
	}
	keys, err := cold.ListSessions(context.Background(), agent.SessionSelector{
		Namespace: "task.researcher", ID: task.Ref.Session,
	})
	if err != nil || len(keys) != 1 || keys[0].Attributes["route"] != "cycle-one" {
		t.Fatalf("task route keys=%#v err=%v", keys, err)
	}
}

func TestLocalTasksWaitReturnsFirstReadyAndLeavesOtherRunning(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	fast := newTaskAgent(t, agentsession.Memory(), &taskModel{responses: []*agent.Message{agent.AssistantMessage("fast result", nil)}})
	defer fast.Close(context.Background())
	release := make(chan struct{})
	slow := newTaskAgent(t, agentsession.Memory(), &blockingTaskModel{release: release, response: "slow result"})
	defer slow.Close(context.Background())
	executor, err := NewLocalTasks(LocalTaskOptions{Parallelism: 2},
		LocalTaskAgent{Name: "fast", Description: "Fast", Opener: fast, Identity: agent.CapabilityIdentity{Kind: "test.task.fast", Version: 1}},
		LocalTaskAgent{Name: "slow", Description: "Slow", Opener: slow, Identity: agent.CapabilityIdentity{Kind: "test.task.slow", Version: 1}},
	)
	if err != nil {
		t.Fatal(err)
	}
	fastTask, err := executor.Start(ctx, TaskRequest{Agent: "fast", Prompt: "fast", IdempotencyKey: "fast"})
	if err != nil {
		t.Fatal(err)
	}
	slowTask, err := executor.Start(ctx, TaskRequest{Agent: "slow", Prompt: "slow", IdempotencyKey: "slow"})
	if err != nil {
		t.Fatal(err)
	}
	outcomes, err := executor.Wait(ctx, []TaskRef{fastTask.Ref, slowTask.Ref})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 2 || outcomes[0].Task == nil || !outcomes[0].Ready || outcomes[0].Task.Status != string(agent.ResultCompleted) || outcomes[0].Task.Output != "fast result" {
		t.Fatalf("fast wait outcome = %#v", outcomes)
	}
	if outcomes[1].Task == nil || outcomes[1].Ready || outcomes[1].Task.Status != "running" {
		t.Fatalf("slow wait outcome = %#v", outcomes[1])
	}
	close(release)
	outcomes, err = executor.Wait(ctx, []TaskRef{slowTask.Ref})
	if err != nil || len(outcomes) != 1 || outcomes[0].Task == nil || !outcomes[0].Ready || outcomes[0].Task.Output != "slow result" {
		t.Fatalf("settled slow outcome=%#v err=%v", outcomes, err)
	}
}

func TestLocalTasksWaitForwardsStableTypedChildIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	release := make(chan struct{})
	owner := newTaskAgent(t, agentsession.Memory(), &blockingTaskModel{release: release, response: "child stream"})
	defer owner.Close(context.Background())
	executor := &notifyingTaskExecutor{LocalTasks: newTaskExecutor(t, owner), waiting: make(chan struct{})}
	started, err := executor.Start(ctx, TaskRequest{Agent: "researcher", Prompt: "research", IdempotencyKey: "forward-task"})
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := json.Marshal(taskWaitInput{Targets: []taskWaitTarget{{Ref: started.Ref}}})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := agent.New(ctx, agent.Definition{
		Name: "root", Model: &taskModel{responses: []*agent.Message{
			agent.AssistantMessage("", []agent.ToolCall{{
				ID: "wait", Type: "function", Function: agent.FunctionCall{Name: "task_wait", Arguments: string(arguments)},
			}}),
			agent.AssistantMessage("parent final", nil),
		}},
		ModelIdentity: agent.CapabilityIdentity{Kind: "test.task.parent-model", Version: 1},
		Tools:         Tasks(executor),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close(context.Background())
	run, err := parent.Run(ctx, agent.Text("delegate"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.waiting:
		close(release)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var forwarded []agent.NestedEvent
	for event := range run.Events() {
		if nested, ok := event.Payload.(agent.NestedEvent); ok {
			forwarded = append(forwarded, nested)
		}
	}
	if result, waitErr := run.Wait(ctx); waitErr != nil || result.Status != agent.ResultCompleted {
		t.Fatalf("parent result=%#v err=%v", result, waitErr)
	}
	if len(forwarded) == 0 {
		t.Fatal("task_wait did not publish typed child events")
	}
	event := forwarded[0]
	if event.Source.Name != "researcher" || event.Source.InvocationType != "task" ||
		event.Source.InvocationID == "" || event.SessionID != started.Ref.Session || event.Child.RunID != started.Ref.Run {
		t.Fatalf("forwarded identity = %#v", event)
	}
}

func TestLocalTasksEnforcesCapacityWithoutBreakingIdempotentStart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	release := make(chan struct{})
	owner := newTaskAgent(t, agentsession.Memory(), &blockingTaskModel{release: release, response: "done"})
	defer owner.Close(context.Background())
	executor, err := NewLocalTasks(LocalTaskOptions{Parallelism: 1}, LocalTaskAgent{
		Name: "researcher", Description: "Research", Opener: owner,
		Identity: agent.CapabilityIdentity{Kind: "test.task.capacity", Version: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := executor.Start(ctx, TaskRequest{Agent: "researcher", Prompt: "first", IdempotencyKey: "first"})
	if err != nil {
		t.Fatal(err)
	}
	retried, err := executor.Start(ctx, TaskRequest{Agent: "researcher", Prompt: "first", IdempotencyKey: "first"})
	if err != nil || retried.Ref != first.Ref {
		t.Fatalf("idempotent retry=%#v err=%v", retried, err)
	}
	if _, err := executor.Start(ctx, TaskRequest{Agent: "researcher", Prompt: "second", IdempotencyKey: "second"}); !errors.Is(err, ErrTaskCapacityExceeded) {
		t.Fatalf("capacity error = %v", err)
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, err := executor.Wait(cancelled, []TaskRef{first.Ref}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait error = %v", err)
	}
	observation, err := executor.Observe(ctx, first.Ref, "0")
	if err != nil || observation.Task.Status != "running" {
		t.Fatalf("task after interrupted wait=%#v err=%v", observation.Task, err)
	}
	close(release)
	if outcomes, waitErr := executor.Wait(ctx, []TaskRef{first.Ref}); waitErr != nil || outcomes[0].Task == nil || outcomes[0].Task.Status != string(agent.ResultCompleted) {
		t.Fatalf("final wait=%#v err=%v", outcomes, waitErr)
	}
}

func TestTaskStatusDistinguishesWaitingAndAborting(t *testing.T) {
	const runID = "run"
	if got := taskStatus(agent.SessionSnapshot{ActiveRunID: runID, PendingInteractions: []agent.InteractionRequest{{ID: "ask"}}}, runID); got != "waiting_input" {
		t.Fatalf("waiting status = %q", got)
	}
	if got := taskStatus(agent.SessionSnapshot{ActiveRunID: runID, ActiveAbortPending: true}, runID); got != "aborting" {
		t.Fatalf("aborting status = %q", got)
	}
}
