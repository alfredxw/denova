package tools

import (
	"context"
	"fmt"
	"sync"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

type taskModel struct {
	mu        sync.Mutex
	responses []*agent.Message
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
	executor, err := NewLocalTasks(LocalTaskAgent{
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
		Agent: "researcher", Prompt: "research", IdempotencyKey: "stable-task", Detached: true,
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
	if len(observation.Events) == 0 || observation.Cursor == "" {
		t.Fatalf("cold replay events=%#v cursor=%q", observation.Events, observation.Cursor)
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
	firstExecutor, err := NewLocalTasks(LocalTaskAgent{
		Name: "researcher", Description: "Research", Opener: first,
		Identity:         agent.CapabilityIdentity{Kind: "test.task.route", Version: 1},
		Attributes:       map[string]string{"parent": "stable", "route": "cycle-one"},
		LookupAttributes: map[string]string{"parent": "stable"},
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := firstExecutor.Start(context.Background(), TaskRequest{
		Agent: "researcher", Prompt: "research", IdempotencyKey: "stable-route", Detached: true,
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
	secondExecutor, err := NewLocalTasks(LocalTaskAgent{
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

func TestLocalTasksForwardsStableTypedChildIdentity(t *testing.T) {
	owner := newTaskAgent(t, agentsession.Memory(), &taskModel{responses: []*agent.Message{agent.AssistantMessage("child stream", nil)}})
	defer owner.Close(context.Background())
	executor := newTaskExecutor(t, owner)
	toolset, err := Tasks(executor)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := agent.New(context.Background(), agent.Definition{
		Name: "root", Model: &taskModel{responses: []*agent.Message{
			agent.AssistantMessage("", []agent.ToolCall{{
				ID: "task-call", Type: "function", Function: agent.FunctionCall{
					Name: "task", Arguments: `{"action":"start","starts":[{"agent":"researcher","prompt":"research","idempotency_key":"forward-task"}]}`,
				},
			}}),
			agent.AssistantMessage("parent final", nil),
		}},
		ModelIdentity: agent.CapabilityIdentity{Kind: "test.task.parent-model", Version: 1},
		Tools:         toolset,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close(context.Background())
	run, err := parent.Run(context.Background(), agent.Text("delegate"))
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != agent.ResultCompleted {
		t.Fatalf("parent result=%#v err=%v", result, waitErr)
	}
	var forwarded []agent.NestedEvent
	interactions := 0
	for event := range run.Events() {
		if nested, ok := event.Payload.(agent.NestedEvent); ok {
			forwarded = append(forwarded, nested)
		}
		if _, ok := event.Payload.(agent.InteractionRequested); ok {
			interactions++
		}
	}
	if len(forwarded) == 0 {
		t.Fatal("parent lifecycle did not publish typed child events")
	}
	event := forwarded[0]
	if event.Source.Name != "researcher" || event.Source.InvocationType != "task" ||
		event.Source.InvocationID == "" || len(event.Source.Path) < 2 ||
		event.SessionID == "" || event.Child.RunID == "" {
		t.Fatalf("forwarded identity = %#v", event)
	}
	if interactions != 0 {
		t.Fatalf("parent task descriptor requested duplicate permission: interactions=%d", interactions)
	}
}
