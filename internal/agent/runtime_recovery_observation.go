package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"denova/internal/agentruntime"
)

type recoveryDisplayRouteContextKey struct{}

type recoveryDisplayRoute struct {
	TaskID string
	Emit   func(Event)
}

func withRecoveryDisplayRoute(ctx context.Context, route recoveryDisplayRoute) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if route.Emit == nil && strings.TrimSpace(route.TaskID) == "" {
		return ctx
	}
	route.TaskID = strings.TrimSpace(route.TaskID)
	return context.WithValue(ctx, recoveryDisplayRouteContextKey{}, route)
}

func recoveryDisplayRouteFromContext(ctx context.Context) recoveryDisplayRoute {
	if ctx == nil {
		return recoveryDisplayRoute{}
	}
	route, _ := ctx.Value(recoveryDisplayRouteContextKey{}).(recoveryDisplayRoute)
	return route

}

func recoveryEventEmitter(ctx context.Context) func(Event) {
	return recoveryDisplayRouteFromContext(ctx).Emit
}

// RecoveryObservation owns the one restart-scoped display observation for an
// exact binding. Resume may be called more than once as ordered recovery
// actions become current; Wait follows successor operations until the binding
// reaches a terminal idle state with no accepted queue.
type RecoveryObservation struct {
	owner       *chatHarness
	harness     *agentruntime.Harness
	observation agentruntime.Observation
	binding     agentruntime.BindingRef
	cancel      context.CancelFunc

	mu         sync.Mutex
	initial    agentruntime.StatusSnapshot
	boundRoute recoveryDisplayRoute
}

func (s *ChatService) OpenRecoveryObservation(
	ctx context.Context,
	options RunOptions,
) (*RecoveryObservation, error) {
	harness, binding, err := s.openRecoveryHarness(ctx, options)
	if err != nil {
		return nil, err
	}
	observeCtx, cancel := context.WithCancel(s.harness.lifecycle)
	observation, err := harness.ObserveFromNow(observeCtx)
	if err != nil {
		cancel()
		return nil, err
	}
	status, err := harness.Status(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	return &RecoveryObservation{
		owner: s.harness, harness: harness, observation: observation,
		binding: binding, initial: status, cancel: cancel,
	}, nil
}

func (r *RecoveryObservation) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	cancel := r.cancel
	r.cancel = nil
	route := r.boundRoute
	r.boundRoute = recoveryDisplayRoute{}
	r.mu.Unlock()
	if r.harness != nil && route.TaskID != "" {
		r.harness.UnbindRecoveryContext(withRecoveryDisplayRoute(context.Background(), route))
	}
	if cancel != nil {
		cancel()
	}
}

func (s *ChatService) openRecoveryHarness(
	ctx context.Context,
	options RunOptions,
) (*agentruntime.Harness, agentruntime.BindingRef, error) {
	if s == nil || s.harness == nil || s.harness.runtime == nil {
		return nil, agentruntime.BindingRef{}, ErrRuntimeProjectionUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	binding, err := harnessBindingForOptions(options)
	if err != nil {
		return nil, agentruntime.BindingRef{}, err
	}
	ref, err := agentruntime.BindingReference(binding)
	if err != nil {
		return nil, agentruntime.BindingRef{}, err
	}
	harness, err := s.harness.runtime.Open(ctx, binding)
	if err != nil {
		return nil, agentruntime.BindingRef{}, err
	}
	return harness, ref, nil
}

func (r *RecoveryObservation) InitialStatus() agentruntime.StatusSnapshot {
	if r == nil {
		return agentruntime.StatusSnapshot{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.initial
}

// CurrentStatus re-reads the actor-owned projection. It remains available after
// Close because closing a display observer does not close the durable binding.
func (r *RecoveryObservation) CurrentStatus(ctx context.Context) (agentruntime.StatusSnapshot, error) {
	if r == nil || r.harness == nil {
		return agentruntime.StatusSnapshot{}, ErrRuntimeProjectionUnavailable
	}
	return r.harness.Status(ctx)
}

// DisplayMetadata resolves bounded display identity from the exact accepted
// input named by action. Corrupt or legacy private descriptors degrade to the
// durable user text; they must not prevent an attach or abort recovery action.
func (r *RecoveryObservation) DisplayMetadata(
	ctx context.Context,
	action RuntimeRecoveryAction,
) (RuntimeRecoveryDisplayMetadata, error) {
	if r == nil || r.harness == nil {
		return RuntimeRecoveryDisplayMetadata{}, ErrRuntimeProjectionUnavailable
	}
	if action.Kind == RuntimeRecoveryCompactContext || action.Kind == RuntimeRecoveryRemoveCompaction {
		return RuntimeRecoveryDisplayMetadata{}, nil
	}
	commandID := action.CommandID
	operationID := action.OperationID
	if action.Kind == RuntimeRecoveryAbort {
		status, statusErr := r.harness.Status(ctx)
		if statusErr != nil {
			return RuntimeRecoveryDisplayMetadata{}, statusErr
		}
		commandID = status.ActiveCommandID
		operationID = status.ActiveOperation
		if status.InputRecovery != nil {
			commandID = status.InputRecovery.CommandID
			operationID = status.InputRecovery.OperationID
		}
	}
	input, found, err := r.harness.RecoveryInput(ctx, commandID, operationID)
	if err != nil {
		return RuntimeRecoveryDisplayMetadata{}, err
	}
	if !found {
		// Display metadata is optional for Abort and legacy journals. Durable
		// action identity was already validated independently above.
		return RuntimeRecoveryDisplayMetadata{}, nil
	}
	metadata := RuntimeRecoveryDisplayMetadata{Message: input.Text}
	request, ok := recoveryDisplayRequest(r.binding, input)
	if !ok {
		return metadata, nil
	}
	metadata.Message = request.Request.Message
	metadata.RegenerateFromTurnID = request.Options.TurnID
	return metadata, nil
}

func recoveryDisplayRequest(binding agentruntime.BindingRef, input agentruntime.UserInput) (HarnessTurnRestoreRequest, bool) {
	if len(input.RestoreDescriptor) == 0 {
		return HarnessTurnRestoreRequest{}, false
	}
	var descriptor harnessTurnRestoreDescriptor
	if err := json.Unmarshal(input.RestoreDescriptor, &descriptor); err != nil || descriptor.Version != harnessTurnRestoreDescriptorVersion {
		return HarnessTurnRestoreRequest{}, false
	}
	request := descriptor.Request.chatRequest()
	if request.Message != input.Text {
		return HarnessTurnRestoreRequest{}, false
	}
	options := descriptor.Options.runOptions().normalized(descriptor.Options.Workspace)
	resolvedBinding, err := harnessBindingForOptions(options)
	if err != nil {
		return HarnessTurnRestoreRequest{}, false
	}
	resolvedRef, err := agentruntime.BindingReference(resolvedBinding)
	if err != nil || resolvedRef != binding {
		return HarnessTurnRestoreRequest{}, false
	}
	return HarnessTurnRestoreRequest{Request: request, Options: options}, true
}
