package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type partialTaskExecutor struct {
	schemaTaskExecutor
	observed []TaskRef
	starts   []TaskRequest
}

func (executor *partialTaskExecutor) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "test.task.partial", Version: 1}
}

func (executor *partialTaskExecutor) Start(_ context.Context, request TaskRequest) (Task, error) {
	executor.starts = append(executor.starts, request)
	if request.Agent == "full" {
		return Task{}, ErrTaskCapacityExceeded
	}
	return Task{Ref: TaskRef{Agent: request.Agent, Session: "session", Run: "run"}, Status: "running", Receipt: &agent.CommandReceipt{CommandID: "accepted", Cursor: 1}}, nil
}

type smallResultTaskExecutor struct{ partialTaskExecutor }

func (*smallResultTaskExecutor) ResultLimit() int { return 512 }

func TestSendRejectsUnreturnableReceiptsBeforeAdmission(t *testing.T) {
	executor := &smallResultTaskExecutor{}
	result, err := taskDefinition(t, executor, "send").Tool.Run(t.Context(), `{"items":[{"action":"delegate","message":"inspect"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(executor.starts) != 0 || !strings.Contains(result.ModelContent, `"code":"result_too_large"`) {
		t.Fatalf("starts=%v result=%s", executor.starts, result.ModelContent)
	}
}

func (executor *partialTaskExecutor) Observe(_ context.Context, ref TaskRef, cursor string) (TaskObservation, error) {
	executor.observed = append(executor.observed, ref)
	return TaskObservation{Task: Task{Ref: ref, Status: "running"}, Cursor: cursor}, nil
}

func (executor *partialTaskExecutor) Wait(_ context.Context, refs []TaskRef) ([]TaskWaitOutcome, error) {
	outcomes := make([]TaskWaitOutcome, len(refs))
	for index, ref := range refs {
		if ref.Run == "missing" {
			outcomes[index].Err = ErrTaskNotFound
			continue
		}
		outcomes[index] = TaskWaitOutcome{
			Task:  &Task{Ref: ref, Status: string(agent.ResultCompleted), Output: "done"},
			Ready: true,
		}
	}
	return outcomes, nil
}

func TestSendPreservesPartialSuccess(t *testing.T) {
	executor := &partialTaskExecutor{}
	result, err := taskDefinition(t, executor, "send").Tool.Run(context.Background(), `{"items":[
 {"action":"delegate","agent":"researcher","message":"inspect"},
 {"action":"delegate","agent":"full","message":"inspect"},
 {"action":"delegate"},
 {"action":"delegate","message":22},
 {"action":"unknown"},
 {"action":"delegate","message":"inspect with default"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Results []sendResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.ModelContent), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 6 {
		t.Fatal(result.ModelContent)
	}
	for _, index := range []int{0, 5} {
		if response.Results[index].Outcome != "accepted" || response.Results[index].Receipt == nil {
			t.Fatal(result.ModelContent)
		}
	}
	for _, index := range []int{2, 3, 4} {
		if response.Results[index].Error == nil || response.Results[index].Error.Code != "invalid_input" {
			t.Fatal(result.ModelContent)
		}
	}
	if response.Results[1].Error.Code != "capacity_exceeded" {
		t.Fatal(result.ModelContent)
	}
}
func TestAwaitPreservesValidTargetsAndDoesNotReturnWaitOutput(t *testing.T) {
	executor := &partialTaskExecutor{}
	result, err := taskDefinition(t, executor, "await").Tool.Run(context.Background(), `{"targets":[
 {"ref":{"agent":"researcher","session":"one","run":"done"}},
 {"ref":{"agent":"researcher","session":"two","run":"missing"}},
 {"ref":{"agent":"researcher","session":"three"}}, {"ref":42}]}`)
	if err != nil {
		t.Fatal(err)
	}
	var response awaitReport
	if err := json.Unmarshal([]byte(result.ModelContent), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 4 || response.Reason != "ready" || response.Results[0].Run == nil || !*response.Results[0].Ready || response.Results[0].Output != nil || response.Results[1].Error.Code != "not_found" || response.Results[2].Error.Code != "invalid_input" || response.Results[3].Error.Code != "invalid_input" {
		t.Fatal(result.ModelContent)
	}
}
func TestAwaitZeroReadsOutputWithoutWaiting(t *testing.T) {
	executor := &partialTaskExecutor{}
	result, err := taskDefinition(t, executor, "await").Tool.Run(context.Background(), `{"timeout_ms":0,"targets":[{"ref":{"agent":"researcher","session":"one","run":"running"}}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(executor.observed) != 1 || !strings.Contains(result.ModelContent, `"output"`) || strings.Contains(result.ModelContent, `"events"`) {
		t.Fatal(result.ModelContent)
	}
}

func taskDefinition(t *testing.T, executor TaskExecutor, name string) agent.ToolDefinition {
	t.Helper()
	definitions, err := Tasks(executor).PrepareTools(context.Background(), agent.ToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range definitions {
		info, infoErr := definition.Tool.Info(context.Background())
		if infoErr != nil {
			t.Fatal(infoErr)
		}
		if info != nil && info.Name == name {
			return definition
		}
	}
	t.Fatalf("tool %q was not prepared", name)
	return agent.ToolDefinition{}
}
