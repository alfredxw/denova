package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type partialTaskExecutor struct {
	observed []TaskObserveTarget
}

func (executor *partialTaskExecutor) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "test.task.partial", Version: 1}
}

func (executor *partialTaskExecutor) Start(_ context.Context, request TaskRequest) (Task, error) {
	if request.Agent == "full" {
		return Task{}, ErrTaskCapacityExceeded
	}
	return Task{Ref: TaskRef{Agent: request.Agent, Session: "session", Run: "run"}, Status: "running"}, nil
}

func (executor *partialTaskExecutor) Observe(_ context.Context, ref TaskRef, cursor string) (TaskObservation, error) {
	executor.observed = append(executor.observed, TaskObserveTarget{Ref: ref, Cursor: cursor})
	return TaskObservation{Task: Task{Ref: ref, Status: "running"}, Cursor: cursor}, nil
}

func (executor *partialTaskExecutor) Wait(_ context.Context, refs []TaskRef) ([]TaskWaitOutcome, error) {
	outcomes := make([]TaskWaitOutcome, len(refs))
	for index, ref := range refs {
		if ref.Run == "missing" {
			outcomes[index].Err = errors.New("task Run was not found")
			continue
		}
		outcomes[index] = TaskWaitOutcome{
			Task:  &Task{Ref: ref, Status: string(agent.ResultCompleted), Output: "done"},
			Ready: true,
		}
	}
	return outcomes, nil
}

func (*partialTaskExecutor) Steer(context.Context, TaskRef, agent.Input) error { return nil }
func (*partialTaskExecutor) Respond(context.Context, TaskRef, string, agent.InteractionResponse) error {
	return nil
}
func (*partialTaskExecutor) Abort(context.Context, TaskRef, agent.AbortRequest) error { return nil }

func TestTaskBatchPreservesPartialSuccessAndPerTargetCursors(t *testing.T) {
	executor := &partialTaskExecutor{}
	start := taskDefinition(t, executor, "task")
	result, err := start.Tool.Run(context.Background(), `{
		"action":"start",
		"starts":[
			{"agent":"researcher","prompt":"inspect"},
			{"agent":"full","prompt":"inspect"},
			{"agent":"researcher"},
			{"prompt":"inspect with the default"}
		]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	var started struct {
		Results []taskItemResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.ModelContent), &started); err != nil {
		t.Fatal(err)
	}
	if len(started.Results) != 4 || started.Results[0].Task == nil ||
		started.Results[1].ErrorCode != "capacity_exceeded" || started.Results[2].ErrorCode != "invalid_input" ||
		started.Results[3].Task == nil {
		t.Fatalf("start results = %#v", started.Results)
	}

	result, err = start.Tool.Run(context.Background(), `{
		"action":"observe",
		"targets":[
			{"ref":{"agent":"researcher","session":"one","run":"one"},"cursor":"7"},
			{"ref":{"agent":"researcher","session":"two","run":"two"},"cursor":"19"}
		]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(executor.observed) != 2 || executor.observed[0].Cursor != "7" || executor.observed[1].Cursor != "19" {
		t.Fatalf("observed targets = %#v", executor.observed)
	}
}

func TestTaskWaitReturnsValidTargetsBesideInvalidTargets(t *testing.T) {
	executor := &partialTaskExecutor{}
	wait := taskDefinition(t, executor, "task_wait")
	result, err := wait.Tool.Run(context.Background(), `{
		"targets":[
			{"ref":{"agent":"researcher","session":"one","run":"done"}},
			{"ref":{"agent":"researcher","session":"two","run":"missing"}},
			{"ref":{"agent":"researcher","session":"three"}}
		]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Results []taskItemResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.ModelContent), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 3 || response.Results[0].Task == nil || !response.Results[0].Ready ||
		response.Results[0].Task.Output != "" ||
		response.Results[1].ErrorCode != "task_error" || response.Results[2].ErrorCode != "invalid_input" {
		t.Fatalf("wait results = %#v", response.Results)
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
