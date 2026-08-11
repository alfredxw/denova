package goal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"
)

type goalModel struct {
	mu        sync.Mutex
	responses []*agent.Message
	inputs    [][]*agent.Message
}

func (model *goalModel) Generate(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	return model.next(input)
}

func (model *goalModel) Stream(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.next(input)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}

func (model *goalModel) next(input []*agent.Message) (*agent.Message, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.inputs = append(model.inputs, input)
	if len(model.responses) == 0 {
		return nil, errors.New("Goal model exhausted")
	}
	message := model.responses[0]
	model.responses = model.responses[1:]
	return message, nil
}

func TestStandardGoalSessionContextAndModelToolShareDurableState(t *testing.T) {
	now := time.Date(2026, time.August, 10, 10, 0, 0, 0, time.UTC)
	model := &goalModel{}
	owner, err := agent.New(context.Background(), agent.Definition{
		Model: model, Goal: Standard(WithClock(func() time.Time { return now })),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), agent.NamedSession("goal"))
	if err != nil {
		t.Fatal(err)
	}
	created, err := session.UpdateGoal(context.Background(), agent.GoalMutation{
		Kind: agent.GoalSet, Objective: "finish the durable goal",
	})
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := json.Marshal(map[string]any{
		"action": "complete", "expected_id": created.ID,
		"expected_revision": created.Revision, "report": "verified",
	})
	if err != nil {
		t.Fatal(err)
	}
	model.responses = []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "goal-call", Type: "function",
			Function: agent.FunctionCall{Name: "goal", Arguments: string(arguments)},
		}}),
		agent.AssistantMessage("done", nil),
	}
	run, err := session.Run(context.Background(), agent.Text("continue"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(context.Background()); err != nil || result.Status != agent.ResultCompleted {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	current, present, err := session.Goal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !present || current.Status != agent.GoalCompleted || current.Report != "verified" || current.Revision != created.Revision+1 {
		t.Fatalf("Goal=%#v present=%v", current, present)
	}
	model.mu.Lock()
	firstInput := model.inputs[0]
	model.mu.Unlock()
	if len(firstInput) == 0 || !strings.Contains(firstInput[len(firstInput)-1].Content, "finish the durable goal") {
		t.Fatalf("active Goal missing from model input: %#v", firstInput)
	}
}

func TestStandardGoalUsesRevisionAndMutationIdempotencyFences(t *testing.T) {
	now := time.Date(2026, time.August, 10, 10, 0, 0, 0, time.UTC)
	manager := Standard(WithClock(func() time.Time { return now }))
	created, err := manager.Apply(context.Background(), agent.GoalApplyRequest{Mutation: agent.GoalMutation{
		Kind: agent.GoalSet, Objective: "ship", MutationID: "mutation-1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := manager.Apply(context.Background(), agent.GoalApplyRequest{
		Present: true, Current: created,
		Mutation: agent.GoalMutation{Kind: agent.GoalSet, Objective: "ignored replay", MutationID: "mutation-1"},
	})
	if err != nil || replayed != created {
		t.Fatalf("idempotent replay=%#v err=%v", replayed, err)
	}
	_, err = manager.Apply(context.Background(), agent.GoalApplyRequest{
		Present: true, Current: created,
		Mutation: agent.GoalMutation{Kind: agent.GoalPause, ExpectedRevision: created.Revision + 1, MutationID: "mutation-2"},
	})
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale mutation error=%v", err)
	}
}
