package tools

import (
	"context"
	"encoding/json"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type recordingTodoStore struct {
	loads    int
	requests []TodoApplyRequest
}

func (*recordingTodoStore) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "test.todo.schema", Version: 1}
}
func (store *recordingTodoStore) Load(context.Context) ([]TodoItem, uint64, error) {
	store.loads++
	return nil, 0, nil
}
func (store *recordingTodoStore) Apply(_ context.Context, request TodoApplyRequest) (TodoApplyResult, error) {
	store.requests = append(store.requests, request)
	return TodoApplyResult{Revision: request.ExpectedRevision + 1}, nil
}

func TestTodoRejectsMissingAndMixedMutationFieldsBeforeStoreAccess(t *testing.T) {
	store := &recordingTodoStore{}
	definitions, err := Todo(store).PrepareTools(context.Background(), agent.ToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	tool := definitions[0].Tool
	for _, arguments := range []string{
		`{"action":"replace","items":[]}`,
		`{"action":"replace","expected_revision":0}`,
		`{"action":"replace","expected_revision":0,"items":null}`,
		`{"action":"replace","expected_revision":null,"items":[]}`,
		`{"action":"clear"}`,
		`{"action":"update","expected_revision":0,"mutations":[]}`,
		`{"action":"update","expected_revision":0,"mutations":[{"id":"one","text":"One"}],"items":[]}`,
		`{"action":"replace","expected_revision":0,"items":[],"mutations":[]}`,
		`{"action":"clear","expected_revision":0,"items":[]}`,
		`{"action":"read","items":[]}`,
	} {
		if _, err := tool.Run(context.Background(), arguments); err == nil {
			t.Errorf("invalid action reached the store: %s", arguments)
		}
	}
	if len(store.requests) != 0 || store.loads != 0 {
		t.Fatalf("invalid input touched store: %#v", store)
	}
	// Explicit zero and an explicit empty list mean an intentional replacement.
	if _, err := tool.Run(context.Background(), `{"action":"replace","expected_revision":0,"items":[]}`); err != nil {
		t.Fatal(err)
	}
	if len(store.requests) != 1 || store.requests[0].ExpectedRevision != 0 || store.requests[0].Mode != TodoApplyReplace || len(store.requests[0].Items) != 0 {
		t.Fatalf("replacement = %#v", store.requests)
	}
}

type recordingTaskExecutor struct {
	schemaTaskExecutor
	calls int
}

func (executor *recordingTaskExecutor) Start(context.Context, TaskRequest) (Task, error) {
	executor.calls++
	return Task{}, nil
}
func (executor *recordingTaskExecutor) Observe(context.Context, TaskRef, string) (TaskObservation, error) {
	executor.calls++
	return TaskObservation{}, nil
}

func (executor *recordingTaskExecutor) Steer(context.Context, TaskRef, agent.Input) (agent.CommandReceipt, error) {
	executor.calls++
	return agent.CommandReceipt{CommandID: "steer", Cursor: 1}, nil
}
func (executor *recordingTaskExecutor) Abort(context.Context, TaskRef, agent.AbortRequest) (agent.CommandReceipt, error) {
	executor.calls++
	return agent.CommandReceipt{CommandID: "abort", Cursor: 1}, nil
}
func TestSendRejectsMixedActionFieldsBeforeExecution(t *testing.T) {
	executor := &recordingTaskExecutor{}
	tool := taskDefinition(t, executor, "send").Tool
	result, err := tool.Run(context.Background(), `{"items":[
 {"action":"delegate","message":"inspect","to":{"agent":"a","session":"s"}},
 {"action":"steer","to":{"agent":"a","session":"s","run":"r"}},
 {"action":"steer","to":{"agent":"a","session":"s","run":"r"},"message":"continue","reason":"extra"},
 {"action":"abort","to":{"agent":"a","session":"s","run":"r"},"reason":" "},
 {"action":"message","to":{"agent":"a","session":"s","run":"r"},"message":"context"},
 {"action":"resume","to":{"agent":"a","session":"s","run":"r"},"message":"extra"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Results []sendResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.ModelContent), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 6 || executor.calls != 0 {
		t.Fatal(result.ModelContent)
	}
	for _, item := range response.Results {
		if item.Error == nil || item.Error.Code != "invalid_input" {
			t.Fatal(result.ModelContent)
		}
	}
}
