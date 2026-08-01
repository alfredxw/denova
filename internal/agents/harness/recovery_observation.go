package harness

import (
	"context"
	"denova/internal/agents/run"
	"encoding/json"
	"strings"
	"sync"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

type recoveryDisplayRouteContextKey struct{}

type recoveryDisplayRoute struct {
	TaskID string
	Emit   func(agentrun.Event)
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

func recoveryEventEmitter(ctx context.Context) func(agentrun.Event) {
	return recoveryDisplayRouteFromContext(ctx).Emit
}

// RecoveryObservation owns the one restart-scoped display observation for an
// exact binding. Resume may be called more than once as ordered recovery
// actions become current; Wait follows successor operations until the binding
// reaches a terminal idle state with no accepted queue.
type RecoveryObservation struct {
	owner       *coordinator
	harness     *runstate.Harness
	observation runstate.Observation
	binding     runstate.BindingRef
	cancel      context.CancelFunc

	mu         sync.Mutex
	initial    runstate.StatusSnapshot
	boundRoute recoveryDisplayRoute
}

func (s *Service) OpenRecoveryObservation(
	ctx context.Context,
	options agentrun.Options,
) (*RecoveryObservation, error) {
	harness, binding, err := s.openRecoveryHarness(ctx, options)
	if err != nil {
		return nil, err
	}
	observeCtx, cancel := context.WithCancel(s.coordinator.lifecycle)
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
		owner: s.coordinator, harness: harness, observation: observation,
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

func (s *Service) openRecoveryHarness(
	ctx context.Context,
	options agentrun.Options,
) (*runstate.Harness, runstate.BindingRef, error) {
	if s == nil || s.coordinator == nil || s.coordinator.runtime == nil {
		return nil, runstate.BindingRef{}, ErrRuntimeProjectionUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	binding, err := agentrun.BindingForOptions(options)
	if err != nil {
		return nil, runstate.BindingRef{}, err
	}
	ref, err := runstate.BindingReference(binding)
	if err != nil {
		return nil, runstate.BindingRef{}, err
	}
	harness, err := s.coordinator.runtime.Open(ctx, binding)
	if err != nil {
		return nil, runstate.BindingRef{}, err
	}
	return harness, ref, nil
}

func (r *RecoveryObservation) InitialStatus() agentrun.RuntimeStatus {
	if r == nil {
		return agentrun.RuntimeStatus{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	projected, _ := agentrun.RuntimeStatusFromSnapshot(r.initial)
	return projected
}

// CurrentStatus re-reads the actor-owned projection. It remains available after
// Close because closing a display observer does not close the durable binding.
func (r *RecoveryObservation) CurrentStatus(ctx context.Context) (agentrun.RuntimeStatus, error) {
	if r == nil || r.harness == nil {
		return agentrun.RuntimeStatus{}, ErrRuntimeProjectionUnavailable
	}
	status, err := r.harness.Status(ctx)
	if err != nil {
		return agentrun.RuntimeStatus{}, err
	}
	return agentrun.RuntimeStatusFromSnapshot(status)
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
		commandID = agentrun.CommandID(status.ActiveCommandID)
		operationID = agentrun.OperationID(status.ActiveOperation)
		if status.InputRecovery != nil {
			commandID = agentrun.CommandID(status.InputRecovery.CommandID)
			operationID = agentrun.OperationID(status.InputRecovery.OperationID)
		}
	}
	input, found, err := r.harness.RecoveryInput(ctx, runstate.CommandID(commandID), runstate.OperationID(operationID))
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

func recoveryDisplayRequest(binding runstate.BindingRef, input runstate.UserInput) (TurnRestoreRequest, bool) {
	if len(input.RestoreDescriptor) == 0 {
		return TurnRestoreRequest{}, false
	}
	var descriptor harnessTurnRestoreDescriptor
	if err := json.Unmarshal(input.RestoreDescriptor, &descriptor); err != nil || descriptor.Version != harnessTurnRestoreDescriptorVersion {
		return TurnRestoreRequest{}, false
	}
	request := descriptor.Request.chatRequest()
	if request.Message != input.Text {
		return TurnRestoreRequest{}, false
	}
	options := descriptor.Options.runOptions().Normalize(descriptor.Options.Workspace)
	resolvedBinding, err := agentrun.BindingForOptions(options)
	if err != nil {
		return TurnRestoreRequest{}, false
	}
	resolvedRef, err := runstate.BindingReference(resolvedBinding)
	if err != nil || !resolvedRef.Equal(binding) {
		return TurnRestoreRequest{}, false
	}
	return TurnRestoreRequest{Request: request, Options: options}, true
}
