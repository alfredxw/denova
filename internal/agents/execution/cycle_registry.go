package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

var (
	ErrCycleSpecRefRequired = errors.New("agent execution turn spec reference is required")
	ErrCycleSpecConflict    = errors.New("agent execution turn spec conflicts with an existing command")
	ErrCycleSpecNotFound    = errors.New("agent execution turn spec was not found")
	ErrCycleSpecInvalid     = errors.New("agent execution turn spec is incomplete")
)

// cycleSpecRegistration separates request ownership from durable command
// ownership. Each concurrent Submit attempt owns one lease. A fresh durable
// acceptance pins the entry independently of those leases until Engine.Run
// consumes it or the coordinator durably cancels its queued input.
type cycleSpecRegistration struct {
	fingerprint string
	spec        cycleSpec
	leases      int
	accepted    bool
}

type cycleSpecLease struct {
	engine *durableEngine
	ref    string
	entry  *cycleSpecRegistration
	once   sync.Once
}

// register associates ref with one semantic command and returns ownership of
// this Submit attempt. Equal retries share the first adapter state; a command ID
// reused with different semantics is rejected before either payload can be
// paired with the other's process-local state.
func (e *durableEngine) register(
	ref string,
	command runstate.Command,
	spec cycleSpec,
) (*cycleSpecLease, error) {
	if e == nil {
		return nil, fmt.Errorf("register agent execution turn spec: engine is nil")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, ErrCycleSpecRefRequired
	}
	if command == nil {
		return nil, fmt.Errorf("%w: command is required", runstate.ErrInvalidCommand)
	}
	fingerprint := commandSemanticFingerprint(command) + ":" + cycleRuntimeSemanticFingerprint(spec)

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.specs == nil {
		e.specs = make(map[string]*cycleSpecRegistration)
	}
	entry, exists := e.specs[ref]
	if exists {
		if entry.fingerprint != fingerprint {
			return nil, fmt.Errorf(
				"%w: %w for reference %q",
				runstate.ErrInvalidCommand,
				ErrCycleSpecConflict,
				ref,
			)
		}
		entry.leases++
		return &cycleSpecLease{engine: e, ref: ref, entry: entry}, nil
	}
	entry = &cycleSpecRegistration{fingerprint: fingerprint, spec: spec, leases: 1}
	e.specs[ref] = entry
	return &cycleSpecLease{engine: e, ref: ref, entry: entry}, nil
}

// accept transfers this attempt's lease to durable command ownership. It is
// called only for a fresh Receipt; replay attempts merely release their lease.
func (l *cycleSpecLease) accept() {
	if l == nil {
		return
	}
	l.once.Do(func() { l.engine.releaseLease(l.ref, l.entry, true) })
}

func (l *cycleSpecLease) release() {
	if l == nil {
		return
	}
	l.once.Do(func() { l.engine.releaseLease(l.ref, l.entry, false) })
}

func (e *durableEngine) releaseLease(ref string, entry *cycleSpecRegistration, accepted bool) {
	if e == nil || entry == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	current, exists := e.specs[ref]
	if !exists || current != entry {
		return
	}
	if accepted {
		entry.accepted = true
	}
	if entry.leases > 0 {
		entry.leases--
	}
	if entry.leases == 0 && !entry.accepted {
		delete(e.specs, ref)
	}
}

func (e *durableEngine) take(ref string) (cycleSpec, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return cycleSpec{}, ErrCycleSpecRefRequired
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	entry, exists := e.specs[ref]
	if !exists {
		return cycleSpec{}, fmt.Errorf("%w: %q", ErrCycleSpecNotFound, ref)
	}
	delete(e.specs, ref)
	return entry.spec, nil
}

func (e *durableEngine) pinAccepted(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ErrCycleSpecRefRequired
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	entry := e.specs[ref]
	if entry == nil {
		return fmt.Errorf("%w: %q", ErrCycleSpecNotFound, ref)
	}
	entry.accepted = true
	return nil
}

func (e *durableEngine) discard(ref string) {
	if e == nil {
		return
	}
	e.mu.Lock()
	delete(e.specs, strings.TrimSpace(ref))
	e.mu.Unlock()
}

func (e *durableEngine) restorePendingInput(
	ctx context.Context,
	binding runstate.BindingRef,
	input runstate.QueuedInput,
) error {
	if e == nil {
		return fmt.Errorf("%w: engine is nil", ErrCyclePreparationUnavailable)
	}
	ref := strings.TrimSpace(input.Input.TurnSpecRef)
	if ref == "" {
		return ErrCycleSpecRefRequired
	}
	e.mu.Lock()
	if entry := e.specs[ref]; entry != nil {
		entry.accepted = true
		e.mu.Unlock()
		return nil
	}
	e.mu.Unlock()
	request, err := decodeCycleRestoreRequest(binding, input)
	if err != nil {
		return err
	}
	profile, err := e.profiles.profile(binding.Profile)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCyclePreparationUnavailable, err)
	}
	queued, ok := profile.(QueuedCycleProfile)
	if !ok {
		return fmt.Errorf("%w: profile %q does not accept queued cycles", ErrCyclePreparationUnavailable, profile.ID())
	}
	route := recoveryDisplayRouteFromContext(ctx)
	request.Emit = route.Emit
	request.Options.TaskID = route.TaskID
	cycle, err := queued.PrepareCycle(ctx, request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCyclePreparationUnavailable, err)
	}
	restored := restoredCycleSpec(request, cycle)
	if err := validateCycleBinding(restored.Options, binding); err != nil {
		return err
	}
	if err := validateCycleExecutable(restored); err != nil {
		return err
	}
	command, err := restoredQueuedCommand(request, input.Input)
	if err != nil {
		return err
	}
	fingerprint := commandSemanticFingerprint(command) + ":" + cycleRuntimeSemanticFingerprint(restored)
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.specs == nil {
		e.specs = make(map[string]*cycleSpecRegistration)
	}
	if entry := e.specs[ref]; entry != nil {
		entry.accepted = true
		return nil
	}
	e.specs[ref] = &cycleSpecRegistration{
		fingerprint: fingerprint,
		spec:        restored,
		accepted:    true,
	}
	return nil
}

func restoredQueuedCommand(request CycleRestoreRequest, input runstate.UserInput) (runstate.Command, error) {
	switch request.Kind {
	case CommandSteer:
		return runstate.Steer{ID: runstate.CommandID(request.CommandID), OperationID: runstate.OperationID(request.OperationID), Input: input}, nil
	case CommandFollowUp:
		return runstate.FollowUp{ID: runstate.CommandID(request.CommandID), OperationID: runstate.OperationID(request.OperationID), Input: input}, nil
	case CommandNextTurn:
		return runstate.NextTurn{ID: runstate.CommandID(request.CommandID), AfterOperationID: runstate.OperationID(request.AfterOperationID), Input: input}, nil
	default:
		return nil, fmt.Errorf("%w: queued command kind %q is not restorable", ErrCyclePreparationUnavailable, request.Kind)
	}
}

// ReleasePendingInput implements runstate.EnginePendingInputReleaser. The
// coordinator calls it only after QueueCancelled is durable and reduced, so a
// queued adapter spec remains available until the command can no longer run.
func (e *durableEngine) ReleasePendingInput(_ context.Context, input runstate.UserInput) {
	if e == nil {
		return
	}
	e.discard(input.TurnSpecRef)
}

func (e *durableEngine) clear() {
	if e == nil {
		return
	}
	e.mu.Lock()
	clear(e.specs)
	e.mu.Unlock()
}
