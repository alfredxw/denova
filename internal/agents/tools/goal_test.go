package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/goal"
)

type goalToolStore struct{ current goal.State }

func (store *goalToolStore) Goal(context.Context) (goal.State, bool, error) {
	return store.current, store.current.Visible(), nil
}

func (store *goalToolStore) FinishGoal(_ context.Context, id string, revision uint64, outcome goal.Status, report string) (goal.State, error) {
	next, err := goal.Finish(store.current, id, revision, outcome, report, time.Now().UTC())
	if err == nil {
		store.current = next
	}
	return next, err
}

func TestGoalFinishToolIsProtectedRootOnlySessionMutation(t *testing.T) {
	current, err := goal.New("Complete everything", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	store := &goalToolStore{current: current}
	definition, err := NewGoalFinish(store, current)
	if err != nil {
		t.Fatal(err)
	}
	info, err := definition.Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "goal_finish" ||
		definition.Descriptor.Capability != config.AgentToolGoal ||
		definition.Descriptor.Execution != agent.ToolExecutionSessionExclusive ||
		definition.Descriptor.MutationScope != agent.ToolMutationSession ||
		definition.Descriptor.PostCheck != agent.ToolPostCheckSessionState ||
		definition.Descriptor.ResultRetention != agent.ToolResultProtected ||
		definition.Descriptor.Steering != agent.SteeringFinishCurrent {
		t.Fatalf("goal_finish descriptor = %#v", definition.Descriptor)
	}
	childCtx, finishChild, err := agent.BeginChildInvocation(context.Background(), "worker")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := finishChild(); err != nil {
			t.Fatal(err)
		}
	}()
	if _, err := definition.Tool.Run(childCtx, `{"outcome":"completed","report":"done"}`); err == nil || !strings.Contains(err.Error(), "root Agent invocation") {
		t.Fatalf("child goal_finish error = %v", err)
	}
	if store.current.Status != goal.StatusActive {
		t.Fatalf("child call mutated goal: %#v", store.current)
	}
	result, err := definition.Tool.Run(context.Background(), `{"outcome":"completed","report":"done"}`)
	if err != nil {
		t.Fatal(err)
	}
	if store.current.Status != goal.StatusCompleted || !strings.Contains(result.ModelContent, `"status":"completed"`) {
		t.Fatalf("root completion = goal:%#v result:%#v", store.current, result)
	}
}
