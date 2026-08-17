package agent

import (
	"context"
	"strings"
	"testing"
)

type rootOnlyGoalManager struct{}

func (rootOnlyGoalManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "goal.root-only-test", Version: 1}
}
func (rootOnlyGoalManager) Apply(context.Context, GoalApplyRequest) (GoalState, error) {
	return GoalState{}, nil
}
func (rootOnlyGoalManager) Prepare(context.Context, GoalPrepareRequest) (GoalPreparation, error) {
	return GoalPreparation{}, nil
}
func (rootOnlyGoalManager) AfterRun(context.Context, GoalAfterRunRequest) (GoalContinuation, error) {
	return GoalContinuation{}, nil
}

func TestStandardGoalToolRejectsChildInvocationBeforeStateAccess(t *testing.T) {
	definition, err := standardGoalTool(rootOnlyGoalManager{}, SessionView{}, RunView{})
	if err != nil {
		t.Fatal(err)
	}
	child, finish, err := BeginChildInvocation(context.Background(), "researcher")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := finish(); err != nil {
			t.Error(err)
		}
	}()
	_, err = definition.Tool.Run(child, `{"action":"complete","expected_id":"goal","expected_revision":1}`)
	if err == nil || !strings.Contains(err.Error(), "root Agent invocation") {
		t.Fatalf("child Goal tool error=%v", err)
	}
}
