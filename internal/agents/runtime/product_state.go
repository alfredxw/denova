package agentruntime

import (
	"context"
	"encoding/json"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/interactive"
	agent "github.com/alfredxw/denova/agent"
	publicgoal "github.com/alfredxw/denova/agent/goal"
)

// ProductState gives a selected peer runtime access to the same journal and
// capability schemas as Native, without constructing a Native Agent Session.
// Mutation callbacks are pure and run under the product's canonical CAS fence.
type ProductState struct {
	Read   func(context.Context, string) (json.RawMessage, bool, error)
	Update func(context.Context, string, func(json.RawMessage, bool) (json.RawMessage, error)) error
}

func SessionState(options agentrun.Options, sess *session.Session) (ProductState, error) {
	key, err := agentrun.AgentSessionKeyForOptions(options)
	if err != nil {
		return ProductState{}, err
	}
	return ProductState{
		Read: func(ctx context.Context, capability string) (json.RawMessage, bool, error) {
			return sess.LoadCapability(ctx, key, capability)
		},
		Update: func(ctx context.Context, capability string, update func(json.RawMessage, bool) (json.RawMessage, error)) error {
			return sess.UpdateCapability(ctx, key, capability, update)
		},
	}, nil
}

func GameState(options agentrun.Options, store *interactive.Store) (ProductState, error) {
	key, err := agentrun.AgentSessionKeyForOptions(options)
	if err != nil {
		return ProductState{}, err
	}
	return ProductState{
		Read: func(ctx context.Context, capability string) (json.RawMessage, bool, error) {
			return store.LoadCapability(ctx, options.StoryID, key, capability)
		},
		Update: func(ctx context.Context, capability string, update func(json.RawMessage, bool) (json.RawMessage, error)) error {
			return store.UpdateCapability(ctx, options.StoryID, key, capability, update)
		},
	}, nil
}

// Goal uses the published Goal state schema, shared at the product boundary.
// Its state machine is independent of which executor supplies the evaluator.
func (store ProductState) Goal(ctx context.Context) (agent.GoalState, bool, error) {
	raw, present, err := store.Read(ctx, "agent.goal")
	var state agent.GoalState
	if err == nil && present {
		err = json.Unmarshal(raw, &state)
	}
	return state, present, err
}

func (store ProductState) UpdateGoal(ctx context.Context, mutation agent.GoalMutation) (agent.GoalState, error) {
	var result agent.GoalState
	err := store.Update(ctx, "agent.goal", func(raw json.RawMessage, present bool) (json.RawMessage, error) {
		var current agent.GoalState
		if present {
			if err := json.Unmarshal(raw, &current); err != nil {
				return nil, err
			}
		}
		var err error
		result, err = publicgoal.Standard().Apply(ctx, agent.GoalApplyRequest{Current: current, Present: present, Mutation: mutation})
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	})
	return result, err
}

func (store ProductState) GoalContext(ctx context.Context) (string, error) {
	state, present, err := store.Goal(ctx)
	if err != nil {
		return "", err
	}
	preparation, err := publicgoal.Standard().Prepare(ctx, agent.GoalPrepareRequest{State: state, Present: present})
	if err != nil {
		return "", err
	}
	text := ""
	for _, fragment := range preparation.Context {
		text += fragment.Content + "\n\n"
	}
	return text, nil
}
