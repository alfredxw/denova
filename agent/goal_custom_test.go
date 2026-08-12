package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	agentsession "github.com/alfredxw/denova/agent/session"
)

type customGoalData struct {
	Step  int    `json:"step"`
	Label string `json:"label"`
}

type customGoalManager struct{}

func (customGoalManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "goal.custom-data-test", Version: 1}
}

func (customGoalManager) Apply(_ context.Context, request GoalApplyRequest) (GoalState, error) {
	if request.Mutation.Kind != GoalMutationKind("advance") {
		return GoalState{}, fmt.Errorf("unsupported custom Goal mutation %q", request.Mutation.Kind)
	}
	var data customGoalData
	if err := json.Unmarshal(request.Mutation.Data, &data); err != nil {
		return GoalState{}, err
	}
	revision := uint64(1)
	if request.Present {
		revision = request.Current.Revision + 1
	}
	return GoalState{
		ID: "custom-goal", Status: GoalStatus("tracking"), Revision: revision,
		Data: append(json.RawMessage(nil), request.Mutation.Data...),
	}, nil
}

func (customGoalManager) Prepare(_ context.Context, request GoalPrepareRequest) (GoalPreparation, error) {
	if !request.Present {
		return GoalPreparation{}, nil
	}
	var data customGoalData
	if err := json.Unmarshal(request.State.Data, &data); err != nil {
		return GoalPreparation{}, err
	}
	return GoalPreparation{Context: []ContextFragment{{
		Source: "goal.custom", Purpose: "custom Goal state", Resource: request.State.ID,
		Revision: fmt.Sprintf("%d", request.State.Revision), Placement: ContextFinalUserPrefix,
		Content: fmt.Sprintf("Custom step %d: %s", data.Step, data.Label), HardLimit: 64 << 10,
	}}}, nil
}

func (customGoalManager) AfterRun(context.Context, GoalAfterRunRequest) (GoalContinuation, error) {
	return GoalContinuation{}, nil
}

func TestCustomGoalManagerPersistsOpaqueStateAndPreparesItsOwnContext(t *testing.T) {
	store := &persistentMemoryStore{Store: agentsession.Memory()}
	definition := Definition{
		Model:         &lifecycleModel{responses: []*Message{AssistantMessage("unused", nil)}},
		ModelIdentity: CapabilityIdentity{Kind: "model.custom-goal-test", Version: 1},
		Goal:          customGoalManager{},
	}
	owner, err := New(context.Background(), definition, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	key := NamedSession("custom-goal-data")
	session, err := owner.Session(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	payload := json.RawMessage(`{"step":3,"label":"review ending"}`)
	created, err := session.UpdateGoal(context.Background(), GoalMutation{
		Kind: "advance", MutationID: "custom-mutation-1", Data: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != GoalStatus("tracking") || string(created.Data) != string(payload) {
		t.Fatalf("custom Goal state = %#v", created)
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	definition.Model = &lifecycleModel{responses: []*Message{AssistantMessage("unused", nil)}}
	owner, err = New(context.Background(), definition, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err = owner.Session(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	reopened, present, err := session.Goal(context.Background())
	if err != nil || !present || string(reopened.Data) != string(payload) {
		t.Fatalf("reopened custom Goal = %#v present=%t error=%v", reopened, present, err)
	}
	inspection, err := session.Inspect(context.Background(), Text("continue"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, message := range inspection.ModelRequest.Messages {
		if message != nil && message.Role == User && message.Content != "continue" &&
			containsAll(message.Content, "Custom step 3", "review ending") {
			found = true
		}
	}
	if !found {
		t.Fatalf("custom Goal context missing from inspection: %#v", inspection.ModelRequest.Messages)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
