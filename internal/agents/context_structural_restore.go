package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

var ErrHarnessStructuralRestoreUnavailable = errors.New("agent harness structural restore dependency is unavailable")

// HarnessStructuralRestoreRequest is immutable snapshot identity plus a
// strictly decoded deterministic mutation. The host may resolve canonical
// stores and callbacks, but must not perform model, tool, or canonical writes.
type HarnessStructuralRestoreRequest struct {
	Binding  RuntimeBinding
	Snapshot StructuralOperation
	Options  RunOptions
	Plan     ContextStructuralRestorePlan
}

// HarnessStructuralRestorer rebuilds process-local commit/reconcile code for an
// accepted structural command. Implementations must be effect-free and
// idempotent for Snapshot.CommandID/OperationID.
type HarnessStructuralRestorer func(context.Context, HarnessStructuralRestoreRequest) (ContextStructuralSpec, error)

func (e *harnessEngine) restoreStructuralOperation(
	ctx context.Context,
	snapshot runstate.StructuralOperationSnapshot,
) error {
	if e == nil {
		return fmt.Errorf("%w: engine is nil", ErrHarnessStructuralRestoreUnavailable)
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
		return ErrHarnessStructuralRestoreUnavailable
	}
	slog.InfoContext(ctx, fmt.Sprintf(
		"[agent-harness] rebuilding cold structural operation binding=%+v command_id=%s operation_id=%s kind=%s spec_ref=%s",
		snapshot.Binding,
		snapshot.CommandID,
		snapshot.OperationID,
		snapshot.Kind,
		ref,
	))
	plan, err := decodeContextStructuralRestorePlan(
		cloneJSONRawMessage(snapshot.Ref.RestoreDescriptor),
		snapshot.Binding,
		snapshot.Ref.ExpectedRevision,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHarnessStructuralRestoreUnavailable, err)
	}
	action := contextStructuralActionForKind(snapshot.Kind)
	if action == "" || plan.Action != action {
		return fmt.Errorf("%w: restore plan action %q does not match snapshot kind %q", ErrHarnessStructuralRestoreUnavailable, plan.Action, snapshot.Kind)
	}
	options, err := contextStructuralBindingOptions(snapshot.Binding, RunOptions{})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHarnessStructuralRestoreUnavailable, err)
	}
	productSnapshot, err := structuralOperationFromRuntime(cloneContextStructuralSnapshot(snapshot))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHarnessStructuralRestoreUnavailable, err)
	}
	request := HarnessStructuralRestoreRequest{
		Binding: productSnapshot.Binding, Snapshot: productSnapshot,
		Options: options, Plan: cloneContextStructuralRestorePlan(plan),
	}
	restored, err := restorer(ctx, request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHarnessStructuralRestoreUnavailable, err)
	}
	if restored.Operation == nil {
		return fmt.Errorf("%w: restored structural operation is required", ErrHarnessStructuralRestoreUnavailable)
	}
	// Durable snapshot identity always wins over callback output. Only host-owned
	// operation code and event delivery are accepted from the callback.
	restored.CommandID = string(snapshot.CommandID)
	restored.Action = action
	restored.Ref = contextCompactionRefFromRuntime(cloneContextCompactionRef(snapshot.Ref))
	restored.Options, err = contextStructuralBindingOptions(snapshot.Binding, restored.Options)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHarnessStructuralRestoreUnavailable, err)
	}
	restored.RestorePlan = &plan
	command := contextStructuralCommand(snapshot.CommandID, action, contextCompactionRefToRuntime(restored.Ref))
	if command == nil {
		return fmt.Errorf("%w: unsupported structural snapshot kind %q", ErrHarnessStructuralRestoreUnavailable, snapshot.Kind)
	}
	conversation := &contextStructuralConversation{action: action, operation: restored.Operation, emit: restored.Emit}
	registration, err := e.register(ref, command, HarnessTurnSpec{
		CommandID: CommandID(snapshot.CommandID), CommandKind: AgentCommandKind(action),
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
	action ContextStructuralAction,
	ref runstate.ContextCompactionRef,
) runstate.Command {
	switch action {
	case ContextStructuralCompact:
		return runstate.CompactIfNeeded{ID: commandID, Ref: cloneContextCompactionRef(ref)}
	case ContextStructuralRemove:
		return runstate.RemoveCompaction{ID: commandID, Ref: cloneContextCompactionRef(ref)}
	default:
		return nil
	}
}

func contextStructuralActionForKind(kind runstate.StructuralOperationKind) ContextStructuralAction {
	switch kind {
	case runstate.StructuralCompactContext:
		return ContextStructuralCompact
	case runstate.StructuralRemoveCompaction:
		return ContextStructuralRemove
	default:
		return ""
	}
}

func contextStructuralBindingOptions(binding runstate.BindingRef, options RunOptions) (RunOptions, error) {
	productBinding, err := ParseRuntimeBinding(binding)
	if err != nil {
		return RunOptions{}, err
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
	case AgentKindGeneral:
		options.AgentKind = AgentKindGeneral
		options.Mode = runtimeBindingProfileAgentChat
	case AgentKindIDE:
		options.AgentKind = AgentKindIDE
		if productBinding.ProjectID != "" {
			options.Mode = runtimeBindingProfileAgentChat
		} else {
			options.Mode = "ide"
		}
	case AgentKindInteractiveStory:
		options.AgentKind = AgentKindInteractiveStory
		options.Mode = "interactive"
	case AgentKindConfigManager:
		options.AgentKind = AgentKindConfigManager
	case AgentKindImage:
		options.AgentKind = AgentKindImage
	case config.AgentKindInteractiveDirector:
		options.AgentKind = config.AgentKindInteractiveDirector
		options.Mode = "interactive"
	default:
		return RunOptions{}, fmt.Errorf("unsupported structural binding agent kind %q", productBinding.AgentKind)
	}
	options = options.normalized(productBinding.Workspace)
	resolved, err := harnessBindingForOptions(options)
	if err != nil {
		return RunOptions{}, err
	}
	resolvedRef, err := runstate.BindingReference(resolved)
	if err != nil {
		return RunOptions{}, err
	}
	if !resolvedRef.Equal(binding) {
		return RunOptions{}, fmt.Errorf("%w: restored options do not match structural snapshot", ErrHarnessBindingMismatch)
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

func cloneContextStructuralRestorePlan(plan ContextStructuralRestorePlan) ContextStructuralRestorePlan {
	plan.Mutation = cloneJSONRawMessage(plan.Mutation)
	return plan
}

func cloneJSONRawMessage(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}

// ResumeRecoveredContextStructuralOperation explicitly resumes the one exact
// structural command left recovery-paused on options' binding. Open itself
// remains reconciliation-only and never invokes this method implicitly.
func (s *ChatService) ResumeRecoveredContextStructuralOperation(
	ctx context.Context,
	options RunOptions,
	expectedAction ...ContextStructuralAction,
) (ContextStructuralResult, bool, error) {
	if s == nil || s.harness == nil || s.harness.runtime == nil {
		return ContextStructuralResult{}, false, fmt.Errorf("agent durable runtime is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ContextStructuralResult{}, false, err
	}
	if len(expectedAction) > 1 {
		return ContextStructuralResult{}, false, fmt.Errorf("at most one expected structural action is allowed")
	}
	options = options.normalized(options.Workspace)
	binding, err := harnessBindingForOptions(options)
	if err != nil {
		return ContextStructuralResult{}, false, err
	}
	harness, err := s.harness.runtime.Open(ctx, binding)
	if err != nil {
		return ContextStructuralResult{}, false, err
	}
	var expected ContextStructuralAction
	if len(expectedAction) == 1 {
		expected = expectedAction[0]
		if structuralKindForAction(expected) == "" {
			return ContextStructuralResult{}, false, fmt.Errorf("unsupported expected structural action %q", expected)
		}
	}
	return s.harness.resumeRecoveredContextStructuralOperation(ctx, harness, expected)
}

func (h *chatHarness) resumeRecoveredContextStructuralOperation(
	ctx context.Context,
	harness *runstate.Harness,
	expectedAction ContextStructuralAction,
) (ContextStructuralResult, bool, error) {
	if h == nil || harness == nil {
		return ContextStructuralResult{}, false, fmt.Errorf("agent durable runtime is unavailable")
	}
	status, err := harness.Status(ctx)
	if err != nil {
		return ContextStructuralResult{}, false, err
	}
	if !status.RecoveryPaused || status.Phase != runstate.PhaseCompacting || status.ActiveStructural == nil {
		return ContextStructuralResult{}, false, nil
	}
	snapshot := cloneContextStructuralSnapshot(*status.ActiveStructural)
	action := contextStructuralActionForKind(snapshot.Kind)
	if action == "" {
		return ContextStructuralResult{}, false, fmt.Errorf("unsupported recovered structural kind %q", snapshot.Kind)
	}
	if expectedAction != "" && expectedAction != action {
		return ContextStructuralResult{}, false, fmt.Errorf(
			"recovered structural action %q does not match expected %q",
			action,
			expectedAction,
		)
	}
	plan, err := decodeContextStructuralRestorePlan(snapshot.Ref.RestoreDescriptor, snapshot.Binding, snapshot.Ref.ExpectedRevision)
	if err != nil {
		return ContextStructuralResult{}, false, fmt.Errorf("decode recovered structural operation: %w", err)
	}
	if plan.Action != action {
		return ContextStructuralResult{}, false, fmt.Errorf("recovered structural plan action %q does not match snapshot %q", plan.Action, action)
	}
	observeCtx, stopObserving := context.WithCancel(h.lifecycle)
	defer stopObserving()
	observation, err := harness.ObserveFromNow(observeCtx)
	if err != nil {
		return ContextStructuralResult{}, false, err
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
		return ContextStructuralResult{}, false, err
	}
	if !receipt.Replayed || receipt.CommandID != snapshot.CommandID || receipt.OperationID != snapshot.OperationID {
		return ContextStructuralResult{}, false, fmt.Errorf("recovered structural replay changed durable identity")
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
func (h *chatHarness) waitForRecoveredStructuralSettlement(
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
