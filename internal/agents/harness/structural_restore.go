package harness

import (
	"context"
	agentstructural "denova/internal/agents/context/structural"
	agentrun "denova/internal/agents/run"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

var ErrStructuralRestoreUnavailable = errors.New("agent harness structural restore dependency is unavailable")

// StructuralRestoreRequest is immutable snapshot identity plus a
// strictly decoded deterministic mutation. The host may resolve canonical
// stores and callbacks, but must not perform model, tool, or canonical writes.
type StructuralRestoreRequest struct {
	Binding  agentrun.RuntimeBinding
	Snapshot agentrun.StructuralOperation
	Options  agentrun.Options
	Plan     agentstructural.RestorePlan
}

// StructuralRestorer rebuilds process-local commit/reconcile code for an
// accepted structural command. Implementations must be effect-free and
// idempotent for Snapshot.CommandID/agentrun.OperationID.
type StructuralRestorer func(context.Context, StructuralRestoreRequest) (agentstructural.Spec, error)

func (e *harnessEngine) restoreStructuralOperation(
	ctx context.Context,
	snapshot runstate.StructuralOperationSnapshot,
) error {
	if e == nil {
		return fmt.Errorf("%w: engine is nil", ErrStructuralRestoreUnavailable)
	}
	ref := strings.TrimSpace(snapshot.Ref.SpecRef)
	if ref == "" {
		return ErrHarnessTurnSpecRefRequired
	}
	// A same-process replay keeps the original operation closure and remains the
	// cheapest path. Cold recovery consults the host only when this pin misses.
	if err := e.pinAccepted(ref); err == nil {
		return nil
	} else if !errors.Is(err, ErrHarnessTurnSpecNotFound) {
		return err
	}

	e.mu.Lock()
	restorer := e.structuralRestorer
	e.mu.Unlock()
	if restorer == nil {
		return ErrStructuralRestoreUnavailable
	}
	slog.InfoContext(ctx, fmt.Sprintf(
		"[agent-harness] rebuilding cold structural operation binding=%+v command_id=%s operation_id=%s kind=%s spec_ref=%s",
		snapshot.Binding,
		snapshot.CommandID,
		snapshot.OperationID,
		snapshot.Kind,
		ref,
	))
	plan, err := agentstructural.DecodeRuntimeRestorePlan(
		cloneJSONRawMessage(snapshot.Ref.RestoreDescriptor),
		snapshot.Binding,
		snapshot.Ref.ExpectedRevision,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStructuralRestoreUnavailable, err)
	}
	action := agentstructural.ActionFromRuntimeKind(snapshot.Kind)
	if action == "" || plan.Action != action {
		return fmt.Errorf("%w: restore plan action %q does not match snapshot kind %q", ErrStructuralRestoreUnavailable, plan.Action, snapshot.Kind)
	}
	options, err := contextStructuralBindingOptions(snapshot.Binding, agentrun.Options{})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStructuralRestoreUnavailable, err)
	}
	productSnapshot, err := agentrun.StructuralOperationFromRuntime(cloneContextStructuralSnapshot(snapshot))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStructuralRestoreUnavailable, err)
	}
	request := StructuralRestoreRequest{
		Binding: productSnapshot.Binding, Snapshot: productSnapshot,
		Options: options, Plan: cloneContextStructuralRestorePlan(plan),
	}
	restored, err := restorer(ctx, request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStructuralRestoreUnavailable, err)
	}
	if restored.Operation == nil {
		return fmt.Errorf("%w: restored structural operation is required", ErrStructuralRestoreUnavailable)
	}
	// Durable snapshot identity always wins over callback output. Only host-owned
	// operation code and event delivery are accepted from the callback.
	restored.CommandID = string(snapshot.CommandID)
	restored.Action = action
	restored.Ref = agentrun.ContextCompactionRefFromRuntime(cloneContextCompactionRef(snapshot.Ref))
	restored.Options, err = contextStructuralBindingOptions(snapshot.Binding, restored.Options)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStructuralRestoreUnavailable, err)
	}
	restored.RestorePlan = &plan
	command := contextStructuralCommand(snapshot.CommandID, action, agentrun.ContextCompactionRefToRuntime(restored.Ref))
	if command == nil {
		return fmt.Errorf("%w: unsupported structural snapshot kind %q", ErrStructuralRestoreUnavailable, snapshot.Kind)
	}
	conversation := &contextStructuralConversation{action: action, operation: restored.Operation, emit: restored.Emit}
	registration, err := e.register(ref, command, TurnSpec{
		CommandID: agentrun.CommandID(snapshot.CommandID), CommandKind: CommandKind(action),
		Conversation: conversation, Options: restored.Options, Emit: restored.Emit,
	})
	if err != nil {
		return err
	}
	registration.accept()
	slog.InfoContext(ctx, fmt.Sprintf(
		"[agent-harness] cold structural operation registered and pinned binding=%+v command_id=%s operation_id=%s action=%s",
		snapshot.Binding,
		snapshot.CommandID,
		snapshot.OperationID,
		action,
	))
	return nil
}

func contextStructuralCommand(
	commandID runstate.CommandID,
	action agentstructural.Action,
	ref runstate.ContextCompactionRef,
) runstate.Command {
	switch action {
	case agentstructural.Compact:
		return runstate.CompactIfNeeded{ID: commandID, Ref: cloneContextCompactionRef(ref)}
	case agentstructural.Remove:
		return runstate.RemoveCompaction{ID: commandID, Ref: cloneContextCompactionRef(ref)}
	default:
		return nil
	}
}

func contextStructuralBindingOptions(binding runstate.BindingRef, options agentrun.Options) (agentrun.Options, error) {
	productBinding, err := agentrun.ParseRuntimeBinding(binding)
	if err != nil {
		return agentrun.Options{}, err
	}
	options.ProjectID = productBinding.ProjectID
	if productBinding.ProjectID == "" {
		options.Workspace = productBinding.Workspace
	}
	options.SessionID = productBinding.SessionID
	options.StoryID = productBinding.StoryID
	options.BranchID = productBinding.BranchID
	options.AutomationTaskID = ""
	switch productBinding.AgentKind {
	case agentrun.AgentKindGeneral:
		options.AgentKind = agentrun.AgentKindGeneral
		options.Mode = agentrun.ModeAgentChat
	case agentrun.AgentKindIDE:
		options.AgentKind = agentrun.AgentKindIDE
		if productBinding.ProjectID != "" {
			options.Mode = agentrun.ModeAgentChat
		} else {
			options.Mode = "ide"
		}
	case agentrun.AgentKindInteractiveStory:
		options.AgentKind = agentrun.AgentKindInteractiveStory
		options.Mode = "interactive"
	case agentrun.AgentKindConfigManager:
		options.AgentKind = agentrun.AgentKindConfigManager
	case agentrun.AgentKindImage:
		options.AgentKind = agentrun.AgentKindImage
	case config.AgentKindInteractiveDirector:
		options.AgentKind = config.AgentKindInteractiveDirector
		options.Mode = "interactive"
	default:
		return agentrun.Options{}, fmt.Errorf("unsupported structural binding agent kind %q", productBinding.AgentKind)
	}
	options = options.Normalize(productBinding.Workspace)
	resolved, err := agentrun.BindingForOptions(options)
	if err != nil {
		return agentrun.Options{}, err
	}
	resolvedRef, err := runstate.BindingReference(resolved)
	if err != nil {
		return agentrun.Options{}, err
	}
	if !resolvedRef.Equal(binding) {
		return agentrun.Options{}, fmt.Errorf("%w: restored options do not match structural snapshot", ErrBindingMismatch)
	}
	return options, nil
}

func cloneContextStructuralSnapshot(snapshot runstate.StructuralOperationSnapshot) runstate.StructuralOperationSnapshot {
	snapshot.Binding = snapshot.Binding.Clone()
	snapshot.Ref = cloneContextCompactionRef(snapshot.Ref)
	return snapshot
}

func cloneContextCompactionRef(ref runstate.ContextCompactionRef) runstate.ContextCompactionRef {
	ref.RestoreDescriptor = cloneJSONRawMessage(ref.RestoreDescriptor)
	return ref
}

func cloneContextStructuralRestorePlan(plan agentstructural.RestorePlan) agentstructural.RestorePlan {
	plan.Mutation = cloneJSONRawMessage(plan.Mutation)
	return plan
}

func cloneJSONRawMessage(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}

// ResumeRecoveredStructuralOperation explicitly resumes the one exact
// structural command left recovery-paused on options' binding. Open itself
// remains reconciliation-only and never invokes this method implicitly.
func (s *Service) ResumeRecoveredStructuralOperation(
	ctx context.Context,
	options agentrun.Options,
	expectedAction ...agentstructural.Action,
) (agentstructural.Result, bool, error) {
	if s == nil || s.coordinator == nil || s.coordinator.runtime == nil {
		return agentstructural.Result{}, false, fmt.Errorf("agent durable runtime is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return agentstructural.Result{}, false, err
	}
	if len(expectedAction) > 1 {
		return agentstructural.Result{}, false, fmt.Errorf("at most one expected structural action is allowed")
	}
	options = options.Normalize(options.Workspace)
	binding, err := agentrun.BindingForOptions(options)
	if err != nil {
		return agentstructural.Result{}, false, err
	}
	harness, err := s.coordinator.runtime.Open(ctx, binding)
	if err != nil {
		return agentstructural.Result{}, false, err
	}
	var expected agentstructural.Action
	if len(expectedAction) == 1 {
		expected = expectedAction[0]
		if agentstructural.RuntimeKind(expected) == "" {
			return agentstructural.Result{}, false, fmt.Errorf("unsupported expected structural action %q", expected)
		}
	}
	return s.coordinator.resumeRecoveredContextStructuralOperation(ctx, harness, expected)
}

func (h *coordinator) resumeRecoveredContextStructuralOperation(
	ctx context.Context,
	harness *runstate.Harness,
	expectedAction agentstructural.Action,
) (agentstructural.Result, bool, error) {
	if h == nil || harness == nil {
		return agentstructural.Result{}, false, fmt.Errorf("agent durable runtime is unavailable")
	}
	status, err := harness.Status(ctx)
	if err != nil {
		return agentstructural.Result{}, false, err
	}
	if !status.RecoveryPaused || status.Phase != runstate.PhaseCompacting || status.ActiveStructural == nil {
		return agentstructural.Result{}, false, nil
	}
	snapshot := cloneContextStructuralSnapshot(*status.ActiveStructural)
	action := agentstructural.ActionFromRuntimeKind(snapshot.Kind)
	if action == "" {
		return agentstructural.Result{}, false, fmt.Errorf("unsupported recovered structural kind %q", snapshot.Kind)
	}
	if expectedAction != "" && expectedAction != action {
		return agentstructural.Result{}, false, fmt.Errorf(
			"recovered structural action %q does not match expected %q",
			action,
			expectedAction,
		)
	}
	plan, err := agentstructural.DecodeRuntimeRestorePlan(snapshot.Ref.RestoreDescriptor, snapshot.Binding, snapshot.Ref.ExpectedRevision)
	if err != nil {
		return agentstructural.Result{}, false, fmt.Errorf("decode recovered structural operation: %w", err)
	}
	if plan.Action != action {
		return agentstructural.Result{}, false, fmt.Errorf("recovered structural plan action %q does not match snapshot %q", plan.Action, action)
	}
	observeCtx, stopObserving := context.WithCancel(h.lifecycle)
	defer stopObserving()
	observation, err := harness.ObserveFromNow(observeCtx)
	if err != nil {
		return agentstructural.Result{}, false, err
	}
	command := contextStructuralCommand(snapshot.CommandID, action, snapshot.Ref)
	slog.InfoContext(ctx, fmt.Sprintf(
		"[agent-harness] explicitly resuming recovery-paused structural operation binding=%+v command_id=%s operation_id=%s action=%s",
		snapshot.Binding,
		snapshot.CommandID,
		snapshot.OperationID,
		action,
	))
	receipt, err := harness.Submit(ctx, command)
	if err != nil {
		return agentstructural.Result{}, false, err
	}
	if !receipt.Replayed || receipt.CommandID != snapshot.CommandID || receipt.OperationID != snapshot.OperationID {
		return agentstructural.Result{}, false, fmt.Errorf("recovered structural replay changed durable identity")
	}
	if err := h.waitForRecoveredStructuralSettlement(ctx, harness, observation, receipt); err != nil {
		return cloneContextStructuralRestorePlan(plan).Result, true, err
	}
	slog.InfoContext(ctx, fmt.Sprintf(
		"[agent-harness] recovered structural operation settled binding=%+v command_id=%s operation_id=%s action=%s",
		snapshot.Binding,
		snapshot.CommandID,
		snapshot.OperationID,
		action,
	))
	return cloneContextStructuralRestorePlan(plan).Result, true, nil
}

// Caller cancellation durably requests Abort for the exact recovered
// operation, then keeps observing until the actor publishes a terminal event.
// This prevents a caller from returning while a canonical commit can still
// become authoritative behind the selected in-memory projection.
func (h *coordinator) waitForRecoveredStructuralSettlement(
	caller context.Context,
	harness *runstate.Harness,
	observation runstate.Observation,
	receipt runstate.Receipt,
) error {
	if status, err := harness.Status(h.lifecycle); err == nil && operationAlreadySettled(status, receipt) {
		return structuralSettlementError(status.LastOperation)
	}
	callerDone := caller.Done()
	events := observation.Events
	errorsCh := observation.Errors
	for {
		select {
		case event, ok := <-events:
			if !ok {
				events = nil
				if errorsCh == nil {
					return fmt.Errorf("structural context observation closed before operation %s settled", receipt.OperationID)
				}
				continue
			}
			switch payload := event.Payload.(type) {
			case runstate.OperationSettledEvent:
				if payload.OperationID == receipt.OperationID {
					return structuralSettlementError(&runstate.OperationSummary{
						OperationID: payload.OperationID, Status: payload.Status, Reason: payload.Reason,
					})
				}
			case runstate.OperationInterruptedEvent:
				if payload.OperationID == receipt.OperationID {
					return fmt.Errorf("structural context operation interrupted: %s", payload.Reason)
				}
			}
		case observationErr, ok := <-errorsCh:
			if !ok {
				errorsCh = nil
				if events == nil {
					return fmt.Errorf("structural context observation closed before operation %s settled", receipt.OperationID)
				}
				continue
			}
			if observationErr != nil {
				return observationErr
			}
		case <-callerDone:
			callerDone = nil
			reason := "recovered structural context caller canceled"
			if err := caller.Err(); err != nil {
				reason = err.Error()
			}
			_, err := harness.Submit(h.lifecycle, runstate.Abort{
				ID: runstate.CommandID(newHarnessIdentity("command")), OperationID: receipt.OperationID, Reason: reason,
			})
			if err != nil && !errors.Is(err, runstate.ErrInvalidCommand) &&
				!errors.Is(err, runstate.ErrStaleOperation) && !errors.Is(err, runstate.ErrDomainCommitRejected) {
				return err
			}
		case <-h.lifecycle.Done():
			return h.lifecycle.Err()
		}
	}
}
