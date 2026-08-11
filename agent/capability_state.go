package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// LoadSessionState reads one Agent-owned durable capability slot from the
// currently executing tool. It is unavailable outside a concrete Tool.Run.
func LoadSessionState(ctx context.Context, capability string, target any) (bool, error) {
	client := capabilityStateFromContext(ctx)
	if client == nil {
		return false, ErrCapabilityUnsupported
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	raw, present := client.states[capability]
	if !present {
		return false, nil
	}
	if target == nil {
		return true, nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return false, fmt.Errorf("decode Session state %q: %w", capability, err)
	}
	return true, nil
}

// UpdateSessionState atomically derives and commits one Agent-owned durable
// capability slot from the currently executing tool. The callback executes
// under the cycle-local state lock and must not call Session-state functions.
func UpdateSessionState(
	ctx context.Context,
	capability string,
	update func(current json.RawMessage, present bool) (next json.RawMessage, delete bool, err error),
) error {
	client := capabilityStateFromContext(ctx)
	if client == nil || client.emit == nil || update == nil {
		return ErrCapabilityUnsupported
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	current, present := client.states[capability]
	next, remove, err := update(append(json.RawMessage(nil), current...), present)
	if err != nil {
		return err
	}
	if !remove && next == nil {
		return nil
	}
	if !remove && (len(next) == 0 || !json.Valid(next)) {
		return errors.New("Session state update requires valid non-empty JSON")
	}
	if !remove && present && bytes.Equal(current, next) {
		return nil
	}
	if err := client.emit(runstate.EngineCapabilityState{
		Capability: capability, Expected: describeCapabilityState(current),
		State: append(json.RawMessage(nil), next...), Delete: remove,
	}); err != nil {
		return err
	}
	if remove {
		delete(client.states, capability)
	} else {
		client.states[capability] = append(json.RawMessage(nil), next...)
	}
	return nil
}

type capabilityStateClient struct {
	mu     sync.Mutex
	states map[string]json.RawMessage
	emit   runstate.EngineEventSink
}

const TodoCapability = "agent.todo"

type capabilityStateContextKey struct{}

func newCapabilityStateClient(states map[string]json.RawMessage, emit runstate.EngineEventSink) *capabilityStateClient {
	return &capabilityStateClient{states: cloneRawStateMap(states), emit: emit}
}

func contextWithCapabilityState(ctx context.Context, client *capabilityStateClient) context.Context {
	return context.WithValue(ctx, capabilityStateContextKey{}, client)
}

func capabilityStateFromContext(ctx context.Context) *capabilityStateClient {
	if ctx == nil {
		return nil
	}
	client, _ := ctx.Value(capabilityStateContextKey{}).(*capabilityStateClient)
	return client
}

func (client *capabilityStateClient) updateGoal(
	ctx context.Context,
	manager GoalManager,
	session SessionView,
	run RunView,
	mutation GoalMutation,
) (GoalState, error) {
	if client == nil || client.emit == nil || manager == nil {
		return GoalState{}, ErrCapabilityUnsupported
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	currentRaw, present := client.states[goalCapability]
	var current GoalState
	var err error
	if present {
		current, err = decodeGoalState(currentRaw)
		if err != nil {
			return GoalState{}, err
		}
	}
	if mutation.MutationID == "" {
		mutation.MutationID = CurrentToolExecutionID(ctx)
	}
	if mutation.MutationID == "" {
		return GoalState{}, errors.New("Goal mutation requires a stable mutation identity")
	}
	next, err := manager.Apply(ctx, GoalApplyRequest{
		Session: session, Run: run, Current: current, Present: present, Mutation: mutation,
	})
	if err != nil {
		return GoalState{}, err
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return GoalState{}, err
	}
	if err := client.emit(runstate.EngineCapabilityState{
		Capability: goalCapability, Expected: describeCapabilityState(currentRaw), State: encoded,
	}); err != nil {
		return GoalState{}, err
	}
	client.states[goalCapability] = append(json.RawMessage(nil), encoded...)
	return next, nil
}

func describeCapabilityState(state []byte) runstate.PayloadDescriptor {
	digest := sha256.Sum256(state)
	return runstate.PayloadDescriptor{Bytes: len(state), SHA256: hex.EncodeToString(digest[:])}
}

func cloneRawStateMap(states map[string]json.RawMessage) map[string]json.RawMessage {
	cloned := make(map[string]json.RawMessage, len(states))
	for name, state := range states {
		cloned[name] = append(json.RawMessage(nil), state...)
	}
	return cloned
}

func (client *capabilityStateClient) goal() (GoalState, bool, error) {
	if client == nil {
		return GoalState{}, false, nil
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	raw, present := client.states[goalCapability]
	if !present {
		return GoalState{}, false, nil
	}
	state, err := decodeGoalState(raw)
	return state, present, err
}
