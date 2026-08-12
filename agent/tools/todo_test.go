package tools_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/tools"
)

type todoModel struct {
	mu        sync.Mutex
	responses []*agent.Message
}

func (model *todoModel) Generate(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.Message, error) {
	return model.next()
}

func (model *todoModel) Stream(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.next()
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}

func (model *todoModel) next() (*agent.Message, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	if len(model.responses) == 0 {
		return nil, errors.New("Todo model exhausted")
	}
	message := model.responses[0]
	model.responses = model.responses[1:]
	return message.Clone(), nil
}

func TestTodoUsesDurableSessionStateAndReportsPartialMutationFailures(t *testing.T) {
	toolset, err := tools.Todo()
	if err != nil {
		t.Fatal(err)
	}
	model := &todoModel{responses: []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "todo-update", Type: "function", Function: agent.FunctionCall{
				Name: "todo", Arguments: `{"action":"update","expected_revision":0,"mutations":[{"id":"one","text":"First task","status":"in_progress"},{"id":"bad","status":"completed"}]}`,
			},
		}}),
		agent.AssistantMessage("updated", nil),
	}}
	owner, err := agent.New(context.Background(), agent.Definition{Model: model, Tools: toolset})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), agent.NamedSession("todo"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), agent.Text("update todo"))
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != agent.ResultCompleted {
		t.Fatalf("result = %#v error = %v", result, waitErr)
	}
	observation, err := session.Observe(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Snapshot.Todo == nil || observation.Snapshot.Todo.Revision != 1 ||
		len(observation.Snapshot.Todo.Items) != 1 || observation.Snapshot.Todo.Items[0].ID != "one" ||
		observation.Snapshot.Todo.Items[0].Status != agent.TodoInProgress {
		t.Fatalf("Todo snapshot = %#v", observation.Snapshot.Todo)
	}
}

func TestTodoValidatesBatchFinalStateInsteadOfMutationOrder(t *testing.T) {
	toolset, err := tools.Todo()
	if err != nil {
		t.Fatal(err)
	}
	model := &todoModel{responses: []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "todo-first", Type: "function", Function: agent.FunctionCall{
				Name: "todo", Arguments: `{"action":"update","expected_revision":0,"mutations":[{"id":"one","text":"First","status":"in_progress"}]}`,
			},
		}}),
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "todo-second", Type: "function", Function: agent.FunctionCall{
				Name: "todo", Arguments: `{"action":"update","expected_revision":1,"mutations":[{"id":"two","text":"Second","status":"in_progress"},{"id":"one","status":"completed"}]}`,
			},
		}}),
		agent.AssistantMessage("updated", nil),
	}}
	owner, err := agent.New(context.Background(), agent.Definition{Model: model, Tools: toolset})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), agent.NamedSession("todo-single-progress"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), agent.Text("update todo"))
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != agent.ResultCompleted {
		t.Fatalf("result = %#v error = %v", result, waitErr)
	}
	snapshot, err := session.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Todo == nil || snapshot.Todo.Revision != 2 || len(snapshot.Todo.Items) != 2 ||
		snapshot.Todo.Items[0].ID != "one" || snapshot.Todo.Items[0].Status != agent.TodoCompleted ||
		snapshot.Todo.Items[1].ID != "two" || snapshot.Todo.Items[1].Status != agent.TodoInProgress {
		t.Fatalf("Todo snapshot = %#v", snapshot.Todo)
	}
}

func TestTodoReplaceAndClearPublishCompleteDurableState(t *testing.T) {
	toolset, err := tools.Todo()
	if err != nil {
		t.Fatal(err)
	}
	model := &todoModel{responses: []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "todo-replace", Type: "function", Function: agent.FunctionCall{
				Name: "todo", Arguments: `{"action":"replace","expected_revision":0,"items":[{"id":"one","text":"First","status":"pending"},{"id":"two","text":"Second","status":"in_progress"}]}`,
			},
		}}),
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "todo-clear", Type: "function", Function: agent.FunctionCall{
				Name: "todo", Arguments: `{"action":"clear","expected_revision":1}`,
			},
		}}),
		agent.AssistantMessage("cleared", nil),
	}}
	owner, err := agent.New(context.Background(), agent.Definition{Model: model, Tools: toolset})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), agent.NamedSession("todo-replace-clear"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), agent.Text("replace then clear"))
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != agent.ResultCompleted {
		t.Fatalf("result = %#v error = %v", result, waitErr)
	}
	snapshot, err := session.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Todo == nil || snapshot.Todo.Revision != 2 || len(snapshot.Todo.Items) != 0 {
		t.Fatalf("Todo snapshot = %#v", snapshot.Todo)
	}
}
