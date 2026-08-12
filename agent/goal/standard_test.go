package goal

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
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
	if err != nil || !reflect.DeepEqual(replayed, created) {
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

func TestStandardGoalModelToolIsTerminalOnly(t *testing.T) {
	now := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	model := &goalModel{}
	owner, err := agent.New(context.Background(), agent.Definition{
		Model: model, Goal: Standard(WithClock(func() time.Time { return now })),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), agent.NamedSession("goal-terminal-only"))
	if err != nil {
		t.Fatal(err)
	}
	created, err := session.UpdateGoal(context.Background(), agent.GoalMutation{
		Kind: agent.GoalSet, Objective: "host-owned objective",
	})
	if err != nil {
		t.Fatal(err)
	}
	call := func(id string, action agent.GoalMutationKind) *agent.Message {
		arguments, marshalErr := json.Marshal(map[string]any{
			"action": action, "expected_id": created.ID, "expected_revision": created.Revision,
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return agent.AssistantMessage("", []agent.ToolCall{{
			ID: id, Type: "function", Function: agent.FunctionCall{Name: "goal", Arguments: string(arguments)},
		}})
	}
	model.responses = []*agent.Message{
		call("set-call", agent.GoalSet), call("clear-call", agent.GoalClear),
		call("complete-call", agent.GoalComplete), agent.AssistantMessage("done", nil),
	}
	run, err := session.Run(context.Background(), agent.Text("try terminal transitions"))
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != agent.ResultCompleted {
		t.Fatalf("result=%#v error=%v", result, waitErr)
	}
	current, present, err := session.Goal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !present || current.Status != agent.GoalCompleted || current.Objective != created.Objective ||
		current.Revision != created.Revision+1 {
		t.Fatalf("model rewrote host-owned Goal: %#v present=%t", current, present)
	}
}

func TestStandardGoalHostMayUseCompleteStateMachine(t *testing.T) {
	now := time.Date(2026, time.August, 12, 11, 0, 0, 0, time.UTC)
	owner, err := agent.New(context.Background(), agent.Definition{
		Model: &goalModel{}, Goal: Standard(WithClock(func() time.Time { return now })),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), agent.NamedSession("goal-host-state-machine"))
	if err != nil {
		t.Fatal(err)
	}
	apply := func(mutation agent.GoalMutation) agent.GoalState {
		t.Helper()
		state, applyErr := session.UpdateGoal(context.Background(), mutation)
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		return state
	}
	state := apply(agent.GoalMutation{Kind: agent.GoalSet, Objective: "first objective"})
	state = apply(agent.GoalMutation{Kind: agent.GoalPause, ExpectedID: state.ID, ExpectedRevision: state.Revision})
	state = apply(agent.GoalMutation{Kind: agent.GoalResume, ExpectedID: state.ID, ExpectedRevision: state.Revision})
	state = apply(agent.GoalMutation{Kind: agent.GoalComplete, ExpectedID: state.ID, ExpectedRevision: state.Revision})
	state = apply(agent.GoalMutation{Kind: agent.GoalClear, ExpectedID: state.ID, ExpectedRevision: state.Revision})
	state = apply(agent.GoalMutation{Kind: agent.GoalSet, Objective: "second objective"})
	state = apply(agent.GoalMutation{Kind: agent.GoalBlock, ExpectedID: state.ID, ExpectedRevision: state.Revision})
	if state.Status != agent.GoalBlocked || state.Objective != "second objective" {
		t.Fatalf("final Goal state = %#v", state)
	}
}

func TestStandardGoalPreparationIsActiveOnlyEscapedAndTerminalSafe(t *testing.T) {
	manager := Standard()
	states := []struct {
		name    string
		present bool
		status  agent.GoalStatus
	}{
		{name: "absent"},
		{name: "paused", present: true, status: agent.GoalPaused},
		{name: "completed", present: true, status: agent.GoalCompleted},
	}
	for _, test := range states {
		t.Run(test.name, func(t *testing.T) {
			prepared, err := manager.Prepare(context.Background(), agent.GoalPrepareRequest{
				Present: test.present, State: agent.GoalState{Status: test.status},
			})
			if err != nil {
				t.Fatal(err)
			}
			if prepared.StandardTool || len(prepared.Tools) != 0 || len(prepared.Context) != 0 {
				t.Fatalf("inactive Goal preparation=%#v", prepared)
			}
		})
	}
	prepared, err := manager.Prepare(context.Background(), agent.GoalPrepareRequest{
		Present: true,
		State: agent.GoalState{
			ID: `goal-<unsafe>`, Objective: `Ship <complete> & "verified"`,
			Status: agent.GoalActive, Revision: 7,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.StandardTool || len(prepared.Context) != 1 {
		t.Fatalf("active Goal preparation=%#v", prepared)
	}
	content := prepared.Context[0].Content
	for _, required := range []string{
		`goal-&lt;unsafe&gt;`, `Ship &lt;complete&gt; &amp; &#34;verified&#34;`,
		"entire objective", "intermediate milestone", "user input or an external state change",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("active Goal protocol missing %q: %s", required, content)
		}
	}
	if strings.Contains(content, `Ship <complete>`) {
		t.Fatalf("active Goal objective was not escaped: %s", content)
	}
}
