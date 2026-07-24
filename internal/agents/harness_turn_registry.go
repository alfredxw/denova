package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

var (
	ErrHarnessTurnSpecRefRequired = errors.New("agent harness turn spec reference is required")
	ErrHarnessTurnSpecConflict    = errors.New("agent harness turn spec conflicts with an existing command")
	ErrHarnessTurnSpecNotFound    = errors.New("agent harness turn spec was not found")
	ErrHarnessTurnSpecInvalid     = errors.New("agent harness turn spec is incomplete")
)

// harnessTurnSpecRegistration separates request ownership from durable command
// ownership. Each concurrent Submit attempt owns one lease. A fresh durable
// acceptance pins the entry independently of those leases until Engine.Run
// consumes it or the coordinator durably cancels its queued input.
type harnessTurnSpecRegistration struct {
	fingerprint string
	spec        HarnessTurnSpec
	leases      int
	accepted    bool
}

type harnessTurnSpecLease struct {
	engine *harnessEngine
	ref    string
	entry  *harnessTurnSpecRegistration
	once   sync.Once
}

// register associates ref with one semantic command and returns ownership of
// this Submit attempt. Equal retries share the first adapter state; a command ID
// reused with different semantics is rejected before either payload can be
// paired with the other's process-local state.
func (e *harnessEngine) register(
	ref string,
	command runstate.Command,
	spec HarnessTurnSpec,
) (*harnessTurnSpecLease, error) {
	if e == nil {
		return nil, fmt.Errorf("register agent harness turn spec: engine is nil")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, ErrHarnessTurnSpecRefRequired
	}
	if command == nil {
		return nil, fmt.Errorf("%w: command is required", runstate.ErrInvalidCommand)
	}
	fingerprint := harnessCommandSemanticFingerprint(command) + ":" + harnessTurnRuntimeSemanticFingerprint(spec)

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.specs == nil {
		e.specs = make(map[string]*harnessTurnSpecRegistration)
	}
	entry, exists := e.specs[ref]
	if exists {
		if entry.fingerprint != fingerprint {
			return nil, fmt.Errorf(
				"%w: %w for reference %q",
				runstate.ErrInvalidCommand,
				ErrHarnessTurnSpecConflict,
				ref,
			)
		}
		entry.leases++
		return &harnessTurnSpecLease{engine: e, ref: ref, entry: entry}, nil
	}
	entry = &harnessTurnSpecRegistration{fingerprint: fingerprint, spec: spec, leases: 1}
	e.specs[ref] = entry
	return &harnessTurnSpecLease{engine: e, ref: ref, entry: entry}, nil
}

// accept transfers this attempt's lease to durable command ownership. It is
// called only for a fresh Receipt; replay attempts merely release their lease.
func (l *harnessTurnSpecLease) accept() {
	if l == nil {
		return
	}
	l.once.Do(func() { l.engine.releaseLease(l.ref, l.entry, true) })
}

func (l *harnessTurnSpecLease) release() {
	if l == nil {
		return
	}
	l.once.Do(func() { l.engine.releaseLease(l.ref, l.entry, false) })
}

func (e *harnessEngine) releaseLease(ref string, entry *harnessTurnSpecRegistration, accepted bool) {
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

func (e *harnessEngine) take(ref string) (HarnessTurnSpec, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return HarnessTurnSpec{}, ErrHarnessTurnSpecRefRequired
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	entry, exists := e.specs[ref]
	if !exists {
		return HarnessTurnSpec{}, fmt.Errorf("%w: %q", ErrHarnessTurnSpecNotFound, ref)
	}
	delete(e.specs, ref)
	return entry.spec, nil
}

func (e *harnessEngine) pinAccepted(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ErrHarnessTurnSpecRefRequired
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	entry := e.specs[ref]
	if entry == nil {
		return fmt.Errorf("%w: %q", ErrHarnessTurnSpecNotFound, ref)
	}
	entry.accepted = true
	return nil
}

func (e *harnessEngine) discard(ref string) {
	if e == nil {
		return
	}
	e.mu.Lock()
	delete(e.specs, strings.TrimSpace(ref))
	e.mu.Unlock()
}

func (e *harnessEngine) restorePendingInput(
	ctx context.Context,
	binding runstate.BindingRef,
	input runstate.QueuedInput,
) error {
	if e == nil {
		return fmt.Errorf("%w: engine is nil", ErrHarnessTurnRestoreUnavailable)
	}
	ref := strings.TrimSpace(input.Input.TurnSpecRef)
	if ref == "" {
		return ErrHarnessTurnSpecRefRequired
	}
	e.mu.Lock()
	if entry := e.specs[ref]; entry != nil {
		entry.accepted = true
		e.mu.Unlock()
		return nil
	}
	restorer := e.turnRestorer
	e.mu.Unlock()
	if restorer == nil {
		return ErrHarnessTurnRestoreUnavailable
	}
	request, err := decodeHarnessTurnRestoreRequest(binding, input)
	if err != nil {
		return err
	}
	route := recoveryDisplayRouteFromContext(ctx)
	request.Emit = route.Emit
	request.Options.TaskID = route.TaskID
	restored, err := restorer(ctx, request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHarnessTurnRestoreUnavailable, err)
	}
	restored = mergeRestoredHarnessTurn(request, restored)
	if err := validateHarnessTurnBinding(restored.Options, binding); err != nil {
		return err
	}
	if restored.Prepare == nil {
		if err := validateHarnessTurnExecutable(restored); err != nil {
			return err
		}
	}
	command, err := restoredQueuedCommand(request, input.Input)
	if err != nil {
		return err
	}
	fingerprint := harnessCommandSemanticFingerprint(command) + ":" + harnessTurnRuntimeSemanticFingerprint(restored)
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.specs == nil {
		e.specs = make(map[string]*harnessTurnSpecRegistration)
	}
	if entry := e.specs[ref]; entry != nil {
		entry.accepted = true
		return nil
	}
	e.specs[ref] = &harnessTurnSpecRegistration{
		fingerprint: fingerprint,
		spec:        restored,
		accepted:    true,
	}
	return nil
}

func restoredQueuedCommand(request HarnessTurnRestoreRequest, input runstate.UserInput) (runstate.Command, error) {
	switch request.Kind {
	case AgentCommandSteer:
		return runstate.Steer{ID: runstate.CommandID(request.CommandID), OperationID: runstate.OperationID(request.OperationID), Input: input}, nil
	case AgentCommandFollowUp:
		return runstate.FollowUp{ID: runstate.CommandID(request.CommandID), OperationID: runstate.OperationID(request.OperationID), Input: input}, nil
	case AgentCommandNextTurn:
		return runstate.NextTurn{ID: runstate.CommandID(request.CommandID), AfterOperationID: runstate.OperationID(request.AfterOperationID), Input: input}, nil
	default:
		return nil, fmt.Errorf("%w: queued command kind %q is not restorable", ErrHarnessTurnRestoreUnavailable, request.Kind)
	}
}

// ReleasePendingInput implements runstate.EnginePendingInputReleaser. The
// coordinator calls it only after QueueCancelled is durable and reduced, so a
// queued adapter spec remains available until the command can no longer run.
func (e *harnessEngine) ReleasePendingInput(_ context.Context, input runstate.UserInput) {
	if e == nil {
		return
	}
	e.discard(input.TurnSpecRef)
}

func (e *harnessEngine) clear() {
	if e == nil {
		return
	}
	e.mu.Lock()
	clear(e.specs)
	e.mu.Unlock()
}
