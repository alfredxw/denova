package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const maxCapabilityNameBytes = 4 << 10

// CapabilityStateSnapshot is an exact CAS value. State is opaque to Runtime.
type CapabilityStateSnapshot struct {
	State      json.RawMessage
	Descriptor PayloadDescriptor
	Exists     bool
}

type capabilityStateRequest struct {
	name     string
	response chan capabilityStateResponse
}

type capabilityStateResponse struct {
	snapshot CapabilityStateSnapshot
	err      error
}

type setCapabilityStateRequest struct {
	ctx      context.Context
	changes  []EngineCapabilityState
	response chan error
}

// CapabilityState reads one exact durable slot through the actor lane.
func (h *Harness) CapabilityState(ctx context.Context, name string) (CapabilityStateSnapshot, error) {
	if h == nil {
		return CapabilityStateSnapshot{}, ErrHarnessClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateCapabilityName(name); err != nil {
		return CapabilityStateSnapshot{}, err
	}
	request := capabilityStateRequest{name: name, response: make(chan capabilityStateResponse, 1)}
	select {
	case h.requests <- request:
	case <-h.done:
		return CapabilityStateSnapshot{}, h.terminalError()
	case <-ctx.Done():
		return CapabilityStateSnapshot{}, ctx.Err()
	}
	select {
	case response := <-request.response:
		return response.snapshot, response.err
	case <-h.done:
		return CapabilityStateSnapshot{}, h.terminalError()
	case <-ctx.Done():
		return CapabilityStateSnapshot{}, ctx.Err()
	}
}

// SetCapabilityState performs an idle-only durable CAS. Engine-owned updates
// use EngineCapabilityState on the same reducer path.
func (h *Harness) SetCapabilityState(
	ctx context.Context,
	name string,
	expected PayloadDescriptor,
	state json.RawMessage,
	deleteState bool,
) error {
	return h.SetCapabilityStates(ctx, EngineCapabilityState{
		Capability: name, Expected: expected, State: state, Delete: deleteState,
	})
}

// SetCapabilityStates performs one idle-only durable CAS transaction across
// all supplied slots. Either every expected descriptor matches and the whole
// batch is appended, or no capability changes. This is the structural seam for
// operations such as Session.Clear that must never expose a half-reset state.
func (h *Harness) SetCapabilityStates(ctx context.Context, changes ...EngineCapabilityState) error {
	if h == nil {
		return ErrHarnessClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if len(changes) == 0 {
		return nil
	}
	cloned := make([]EngineCapabilityState, len(changes))
	copy(cloned, changes)
	for index := range cloned {
		cloned[index].State = cloneRawMessage(cloned[index].State)
	}
	request := setCapabilityStateRequest{ctx: ctx, changes: cloned, response: make(chan error, 1)}
	select {
	case h.requests <- request:
	case <-h.done:
		return h.terminalError()
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-request.response:
		return err
	case <-h.done:
		return h.terminalError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *harnessState) capabilityStateEvents(changes []EngineCapabilityState) ([]EventPayload, error) {
	if len(changes) > 64 {
		return nil, fmt.Errorf("%w: too many capability state changes", ErrInvalidCommand)
	}
	validation := *s
	validation.capabilityStates = cloneCapabilityStates(s.capabilityStates)
	if validation.capabilityStates == nil {
		validation.capabilityStates = make(map[string]json.RawMessage)
	}
	seen := make(map[string]struct{}, len(changes))
	payloads := make([]EventPayload, 0, len(changes))
	for _, change := range changes {
		if _, duplicate := seen[change.Capability]; duplicate {
			return nil, fmt.Errorf("%w: duplicate capability %q", ErrInvalidCommand, change.Capability)
		}
		seen[change.Capability] = struct{}{}
		payload := CapabilityStateCommittedEvent{
			Capability: change.Capability, Expected: change.Expected,
			State: cloneRawMessage(change.State), Deleted: change.Delete,
		}
		if err := validation.validateCapabilityStateEvent(payload); err != nil {
			return nil, err
		}
		if payload.Deleted {
			delete(validation.capabilityStates, payload.Capability)
		} else {
			validation.capabilityStates[payload.Capability] = cloneRawMessage(payload.State)
		}
		payloads = append(payloads, payload)
	}
	return payloads, nil
}

func validateCapabilityName(name string) error {
	if strings.TrimSpace(name) != name || name == "" || len(name) > maxCapabilityNameBytes {
		return fmt.Errorf("%w: invalid capability name", ErrInvalidCommand)
	}
	return nil
}

func (s *harnessState) capabilityState(name string) CapabilityStateSnapshot {
	state, exists := s.capabilityStates[name]
	return CapabilityStateSnapshot{
		State: cloneRawMessage(state), Descriptor: describePayload(state), Exists: exists,
	}
}

func (s *harnessState) validateCapabilityStateEvent(event CapabilityStateCommittedEvent) error {
	if err := validateCapabilityName(event.Capability); err != nil {
		return err
	}
	current := s.capabilityState(event.Capability)
	if current.Descriptor != event.Expected {
		return fmt.Errorf("%w: capability %q state changed", ErrInvalidCommand, event.Capability)
	}
	if event.Deleted {
		if len(event.State) != 0 {
			return errors.New("deleted capability state cannot contain data")
		}
		return nil
	}
	if len(event.State) == 0 || !json.Valid(event.State) {
		return errors.New("capability state must be valid non-empty JSON")
	}
	var total int64
	for name, state := range s.capabilityStates {
		if name != event.Capability {
			total += int64(len(name) + len(state))
		}
	}
	total += int64(len(event.Capability) + len(event.State))
	limit := s.memoryLimits.normalized().MaxEngineStateBytes
	if total > limit {
		return &ByteBudgetError{Scope: ByteBudgetEngineState, Current: total - int64(len(event.State)), Incoming: int64(len(event.State)), Limit: limit}
	}
	return nil
}
