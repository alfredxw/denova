package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestTaskWaitRoutesChildInteractionThroughHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ask := Ask()
	owner, err := agent.New(ctx, agent.Definition{
		Name: "researcher",
		Model: &taskModel{responses: []*agent.Message{
			agent.AssistantMessage("", []agent.ToolCall{{
				ID: "ask-scope", Type: "function",
				Function: agent.FunctionCall{Name: "ask", Arguments: `{"questions":[{"id":"scope","prompt":"What scope should be inspected?","allow_free_text":true}]}`},
			}}),
			agent.AssistantMessage("interaction resumed", nil),
		}},
		ModelIdentity: agent.CapabilityIdentity{Kind: "test.task.interaction.model", Version: 1},
		Tools:         ask,
	}, agent.WithSessionStore(agentsession.Memory()))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())

	started, err := newTaskExecutor(t, owner).Start(ctx, TaskRequest{
		Agent: "researcher", Prompt: "inspect", IdempotencyKey: "interaction",
	})
	if err != nil {
		t.Fatal(err)
	}

	reopened := newTaskExecutor(t, owner)
	childSession, err := owner.Session(ctx, agent.SessionKey{
		Namespace: "task.researcher", ID: started.Ref.Session, Attributes: map[string]string{"agent": "researcher"},
	})
	if err != nil {
		t.Fatal(err)
	}
	childEvents, err := childSession.Observe(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	waitingForInput := false
	for !waitingForInput {
		select {
		case event, ok := <-childEvents.Events:
			if !ok {
				t.Fatal("child event stream closed before the interaction request")
			}
			_, waitingForInput = event.Payload.(agent.InteractionRequested)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	waiting, err := reopened.Observe(ctx, started.Ref, "0")
	if err != nil {
		t.Fatal(err)
	}
	if waiting.Task.Status != "waiting_input" || waiting.Incomplete {
		t.Fatalf("waiting observation = %#v", waiting)
	}

	arguments, err := json.Marshal(taskWaitInput{Targets: []taskWaitTarget{{Ref: started.Ref}}})
	if err != nil {
		t.Fatal(err)
	}
	waitCall := func(id string) *agent.Message {
		return agent.AssistantMessage("", []agent.ToolCall{{
			ID: id, Type: "function", Function: agent.FunctionCall{Name: "task_wait", Arguments: string(arguments)},
		}})
	}
	parent, err := agent.New(ctx, agent.Definition{
		Name: "root", Model: &taskModel{responses: []*agent.Message{
			waitCall("wait-interaction"), waitCall("wait-final"), agent.AssistantMessage("parent resumed", nil),
		}},
		ModelIdentity: agent.CapabilityIdentity{Kind: "test.task.interaction.parent", Version: 1},
		Tools:         Tasks(reopened),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close(context.Background())
	run, err := parent.Run(ctx, agent.Text("wait for the delegated task"))
	if err != nil {
		t.Fatal(err)
	}
	interactions := 0
	events := run.Events()
	for events != nil {
		select {
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			requested, ok := event.Payload.(agent.InteractionRequested)
			if !ok {
				continue
			}
			interactions++
			if requested.Request.Kind != agent.InteractionAsk {
				t.Fatalf("interaction kind = %q", requested.Request.Kind)
			}
			if err := run.Respond(ctx, requested.Request.ID, agent.InteractionResponse{
				Answers: []agent.InteractionAnswer{{QuestionID: "scope", Text: "the whole repository"}},
			}); err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	result, err := run.Wait(ctx)
	if err != nil || result.Status != agent.ResultCompleted || interactions != 1 {
		t.Fatalf("parent result=%#v interactions=%d err=%v", result, interactions, err)
	}
	observation, err := reopened.Observe(ctx, started.Ref, "0")
	if err != nil {
		t.Fatal(err)
	}
	if observation.Task.Status != string(agent.ResultCompleted) || observation.Output != "interaction resumed" {
		t.Fatalf("completed observation = %#v", observation)
	}
}
