package agentruntime

import (
	"context"
	"encoding/json"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/agents/sessionjournal"
	"denova/internal/interactive"

	publicgoal "github.com/alfredxw/denova/agent/engine/goal"
	agentschema "github.com/alfredxw/denova/agent/schema"
)

// ProductState gives a selected peer runtime access to the same journal and
// capability schemas as Native, without constructing a Native Agent Session.
// Mutation callbacks are pure and run under the product's canonical CAS fence.
type ProductState struct {
	// engine is an admission snapshot; changing engines requires an idle controller.
	engine config.RuntimeID
	Read   func(context.Context, string) (json.RawMessage, bool, error)
	// Transact atomically replaces only the returned states, preserving a backup
	// named by the format upgrade before the first append. Its borrowed reader
	// observes one canonical snapshot; callbacks must tolerate CAS retries.
	Transact func(context.Context, string, func(sessionjournal.CapabilityReader) (map[string]json.RawMessage, error)) error
}

func SessionState(options agentrun.Options, sess *session.Session) (ProductState, error) {
	key, err := agentrun.AgentSessionKeyForOptions(options)
	if err != nil {
		return ProductState{}, err
	}
	selection, _ := sess.RuntimeConfig()
	return ProductState{
		engine: selection.Engine().Kind,
		Read: func(ctx context.Context, capability string) (json.RawMessage, bool, error) {
			return sess.LoadCapability(ctx, key, capability)
		},
		Transact: func(ctx context.Context, upgrade string, update func(sessionjournal.CapabilityReader) (map[string]json.RawMessage, error)) error {
			return sess.UpdateCapabilities(ctx, key, upgrade, update)
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
		Transact: func(ctx context.Context, upgrade string, update func(sessionjournal.CapabilityReader) (map[string]json.RawMessage, error)) error {
			return store.UpdateCapabilities(ctx, options.StoryID, key, upgrade, update)
		},
	}, nil
}

// Update replaces one state using the same atomic seam as multi-state changes.
func (store ProductState) Update(ctx context.Context, capability string, update func(json.RawMessage, bool) (json.RawMessage, error)) error {
	return store.Transact(ctx, "runtime-controls-v1", func(read sessionjournal.CapabilityReader) (map[string]json.RawMessage, error) {
		raw, present, err := read(capability)
		if err != nil {
			return nil, err
		}
		next, err := update(raw, present)
		if err != nil {
			return nil, err
		}
		return map[string]json.RawMessage{capability: next}, nil
	})
}

// Goal uses the published Goal state schema, shared at the product boundary.
// Its state machine is independent of which executor supplies the evaluator.
func (store ProductState) Goal(ctx context.Context) (publicgoal.GoalState, bool, error) {
	raw, present, err := store.Read(ctx, "agent.goal")
	var state publicgoal.GoalState
	if err == nil && present {
		err = json.Unmarshal(raw, &state)
	}
	return state, present, err
}

func (store ProductState) UpdateGoal(ctx context.Context, mutation agentschema.GoalMutation) (publicgoal.GoalState, error) {
	var result publicgoal.GoalState
	err := store.Update(ctx, "agent.goal", func(raw json.RawMessage, present bool) (json.RawMessage, error) {
		var current publicgoal.GoalState
		if present {
			if err := json.Unmarshal(raw, &current); err != nil {
				return nil, err
			}
		}
		var err error
		result, err = publicgoal.Standard().Apply(ctx, publicgoal.GoalApplyRequest{Current: current, Present: present, Mutation: mutation})
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	})
	return result, err
}

func (store ProductState) GoalContext(ctx context.Context) (string, error) {
	if !store.supportsGoal() {
		return "", nil
	}
	state, present, err := store.Goal(ctx)
	if err != nil {
		return "", err
	}
	preparation, err := publicgoal.Standard().Prepare(ctx, publicgoal.GoalPrepareRequest{State: state, Present: present})
	if err != nil {
		return "", err
	}
	text := ""
	for _, fragment := range preparation.Context {
		text += fragment.Content + "\n\n"
	}
	return text, nil
}

func (store ProductState) supportsGoal() bool {
	return store.engine != config.RuntimeGrok && store.engine != config.RuntimeAntigravity
}
