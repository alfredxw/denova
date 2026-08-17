package tools

import (
	"context"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestLocalTasksRespondsToDetachedInteractionWithoutExecutorProcessState(t *testing.T) {
	ask := Ask()
	owner, err := agent.New(context.Background(), agent.Definition{
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

	started, err := newTaskExecutor(t, owner).Start(context.Background(), TaskRequest{
		Agent: "researcher", Prompt: "inspect", IdempotencyKey: "detached-interaction", Detached: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var pending agent.InteractionRequest
	for pending.ID == "" && time.Now().Before(deadline) {
		observation, observeErr := newTaskExecutor(t, owner).Observe(context.Background(), started.Ref, "0")
		if observeErr != nil {
			t.Fatal(observeErr)
		}
		if len(observation.Interactions) != 0 {
			pending = observation.Interactions[0]
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if pending.ID == "" || pending.Kind != agent.InteractionAsk {
		t.Fatalf("pending interaction = %#v", pending)
	}

	// A fresh executor has no process-local waiter or task registry. TaskRef and
	// the durable Session snapshot are the complete interaction-resume identity.
	reopened := newTaskExecutor(t, owner)
	if err := reopened.Respond(context.Background(), started.Ref, pending.ID, agent.InteractionResponse{
		Answers: []agent.InteractionAnswer{{QuestionID: "scope", Text: "the whole repository"}},
	}); err != nil {
		t.Fatal(err)
	}

	for time.Now().Before(deadline) {
		observation, observeErr := reopened.Observe(context.Background(), started.Ref, "0")
		if observeErr != nil {
			t.Fatal(observeErr)
		}
		if observation.Task.Status == string(agent.ResultCompleted) {
			if observation.Output != "interaction resumed" {
				t.Fatalf("completed observation = %#v", observation)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("detached interaction did not resume and settle")
}
