package runtime

import (
	"context"
	"encoding/json"
	"fmt"
)

// EngineCheckpointSnapshot is the private, bounded continuation state needed
// by the Agent facade to prepare a structural operation. It is never a display
// projection and must not be exposed by transports.
type EngineCheckpointSnapshot struct {
	Cursor                Cursor
	State                 json.RawMessage
	StateDescriptor       PayloadDescriptor
	Capabilities          map[string]json.RawMessage
	CapabilityDescriptors map[string]PayloadDescriptor
}

// CapabilityDescriptor returns the exact CAS descriptor for a slot, including
// the canonical descriptor of an absent slot.
func (snapshot EngineCheckpointSnapshot) CapabilityDescriptor(name string) PayloadDescriptor {
	if descriptor, ok := snapshot.CapabilityDescriptors[name]; ok {
		return descriptor
	}
	return describePayload(nil)
}

type engineCheckpointRequest struct{ response chan EngineCheckpointSnapshot }

type idleEngineCheckpointResponse struct {
	snapshot EngineCheckpointSnapshot
	err      error
}

type idleEngineCheckpointRequest struct {
	response chan idleEngineCheckpointResponse
}

// EngineCheckpointUpdate is an idle-only structural replacement of the
// Engine continuation state and selected capability slots. Guards participate
// in the same CAS without emitting an event. An empty State performs only the
// idle/guard verification, which lets an exact product-history replay remain a
// true no-op without racing a newly admitted Run. This seam is intentionally
// below the public Agent API; products must use Session structural operations.
type EngineCheckpointUpdate struct {
	ExpectedState     PayloadDescriptor
	State             json.RawMessage
	CapabilityGuards  map[string]PayloadDescriptor
	CapabilityChanges []EngineCapabilityState
}

type replaceEngineCheckpointRequest struct {
	ctx      context.Context
	update   EngineCheckpointUpdate
	response chan error
}

func (h *Harness) EngineCheckpoint(ctx context.Context) (EngineCheckpointSnapshot, error) {
	if h == nil {
		return EngineCheckpointSnapshot{}, ErrHarnessClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	request := engineCheckpointRequest{response: make(chan EngineCheckpointSnapshot, 1)}
	select {
	case h.requests <- request:
	case <-h.done:
		return EngineCheckpointSnapshot{}, h.terminalError()
	case <-ctx.Done():
		return EngineCheckpointSnapshot{}, ctx.Err()
	}
	select {
	case snapshot := <-request.response:
		return snapshot, nil
	case <-h.done:
		return EngineCheckpointSnapshot{}, h.terminalError()
	case <-ctx.Done():
		return EngineCheckpointSnapshot{}, ctx.Err()
	}
}

// IdleEngineCheckpoint returns one exact private Engine checkpoint only when
// the binding has no active, queued, recovering, structural, or interactive
// work. Agent uses it for read-only inspection that must never observe a
// half-settled lifecycle boundary.
func (h *Harness) IdleEngineCheckpoint(ctx context.Context) (EngineCheckpointSnapshot, error) {
	if h == nil {
		return EngineCheckpointSnapshot{}, ErrHarnessClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	request := idleEngineCheckpointRequest{response: make(chan idleEngineCheckpointResponse, 1)}
	select {
	case h.requests <- request:
	case <-h.done:
		return EngineCheckpointSnapshot{}, h.terminalError()
	case <-ctx.Done():
		return EngineCheckpointSnapshot{}, ctx.Err()
	}
	select {
	case response := <-request.response:
		return response.snapshot, response.err
	case <-h.done:
		return EngineCheckpointSnapshot{}, h.terminalError()
	case <-ctx.Done():
		return EngineCheckpointSnapshot{}, ctx.Err()
	}
}

// ReplaceEngineCheckpoint atomically commits an opaque Engine checkpoint and
// capability changes when the binding is idle and every descriptor still
// matches. It cannot race an admitted Run, Clear, compaction, or another
// structural operation into a partially rebuilt transcript.
func (h *Harness) ReplaceEngineCheckpoint(ctx context.Context, update EngineCheckpointUpdate) error {
	if h == nil {
		return ErrHarnessClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cloned := EngineCheckpointUpdate{
		ExpectedState: update.ExpectedState,
		State:         cloneRawMessage(update.State),
	}
	if len(update.CapabilityGuards) > 0 {
		cloned.CapabilityGuards = make(map[string]PayloadDescriptor, len(update.CapabilityGuards))
		for name, descriptor := range update.CapabilityGuards {
			cloned.CapabilityGuards[name] = descriptor
		}
	}
	if len(update.CapabilityChanges) > 0 {
		cloned.CapabilityChanges = make([]EngineCapabilityState, len(update.CapabilityChanges))
		copy(cloned.CapabilityChanges, update.CapabilityChanges)
		for index := range cloned.CapabilityChanges {
			cloned.CapabilityChanges[index].State = cloneRawMessage(cloned.CapabilityChanges[index].State)
		}
	}
	request := replaceEngineCheckpointRequest{ctx: ctx, update: cloned, response: make(chan error, 1)}
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

func (s *harnessState) replaceEngineCheckpointEvents(update EngineCheckpointUpdate) ([]EventPayload, error) {
	// ExpectedState is also a guard for capability-only/no-op replacements.
	// Otherwise a product provenance advance could validate one transcript and
	// commit after a newly completed Run changed it.
	if describePayload(s.engineState) != update.ExpectedState {
		return nil, fmt.Errorf("%w: Engine checkpoint changed", ErrInvalidCommand)
	}
	if len(update.State) > 0 {
		if err := s.validateEngineState(update.State); err != nil {
			return nil, err
		}
	}
	for name, expected := range update.CapabilityGuards {
		if err := validateCapabilityName(name); err != nil {
			return nil, err
		}
		if s.capabilityState(name).Descriptor != expected {
			return nil, fmt.Errorf("%w: capability %q state changed", ErrInvalidCommand, name)
		}
	}
	capabilityEvents, err := s.capabilityStateEvents(update.CapabilityChanges)
	if err != nil {
		return nil, err
	}
	payloads := make([]EventPayload, 0, 1+len(capabilityEvents))
	if len(update.State) > 0 {
		payloads = append(payloads, EngineStateCommittedEvent{
			State: cloneRawMessage(update.State), Descriptor: describePayload(update.State),
		})
	}
	payloads = append(payloads, capabilityEvents...)
	return payloads, nil
}
