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
	event    CapabilityStateCommittedEvent
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
	if h == nil {
		return ErrHarnessClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	event := CapabilityStateCommittedEvent{
		Capability: name, Expected: expected, State: cloneRawMessage(state), Deleted: deleteState,
	}
	request := setCapabilityStateRequest{ctx: ctx, event: event, response: make(chan error, 1)}
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
