package agentruntime

import (
	"context"
	"encoding/json"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	agent "github.com/alfredxw/denova/agent"
)

// Plan is a recovery projection of the selected runtime's plan. Only provider
// observations replace it; Denova does not execute a second Todo state machine.
func (store ProductState) Plan(ctx context.Context) ([]agent.TodoItem, error) {
	raw, present, err := store.Read(ctx, agent.TodoCapability)
	var state agent.TodoState
	if err == nil && present {
		err = json.Unmarshal(raw, &state)
	}
	return state.Items, err
}

func (store ProductState) ObservePlan(ctx context.Context, event agentrun.Event) error {
	if event.Type != "todo_updated" {
		return nil
	}
	items, err := external.PlanItems(event)
	if err != nil {
		return err
	}
	return store.Update(ctx, agent.TodoCapability, func(raw json.RawMessage, present bool) (json.RawMessage, error) {
		var state agent.TodoState
		if present {
			if err := json.Unmarshal(raw, &state); err != nil {
				return nil, err
			}
		}
		state.Revision++
		state.Items = items
		return json.Marshal(state)
	})
}
