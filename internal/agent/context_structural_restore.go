package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"denova/config"
	"denova/internal/agentruntime"
)

var ErrHarnessStructuralRestoreUnavailable = errors.New("agent harness structural restore dependency is unavailable")

// HarnessStructuralRestoreRequest is immutable snapshot identity plus a
// strictly decoded deterministic mutation. The host may resolve canonical
// stores and callbacks, but must not perform model, tool, or canonical writes.
type HarnessStructuralRestoreRequest struct {
	Binding  agentruntime.BindingRef
	Snapshot agentruntime.StructuralOperationSnapshot
	Options  RunOptions
	Plan     ContextStructuralRestorePlan
}

// HarnessStructuralRestorer rebuilds process-local commit/reconcile code for an
// accepted structural command. Implementations must be effect-free and
// idempotent for Snapshot.CommandID/OperationID.
type HarnessStructuralRestorer func(context.Context, HarnessStructuralRestoreRequest) (ContextStructuralSpec, error)

func (e *harnessEngine) restoreStructuralOperation(
	ctx context.Context,
	snapshot agentruntime.StructuralOperationSnapshot,
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
	log.Printf(
		"[agent-harness] rebuilding cold structural operation binding=%+v command_id=%s operation_id=%s kind=%s spec_ref=%s",
		snapshot.Binding,
		snapshot.CommandID,
		snapshot.OperationID,
		snapshot.Kind,
		ref,
	)
	plan, err := DecodeContextStructuralRestorePlan(
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
	request := HarnessStructuralRestoreRequest{
		Binding: snapshot.Binding, Snapshot: cloneContextStructuralSnapshot(snapshot),
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
	restored.Ref = cloneContextCompactionRef(snapshot.Ref)
	restored.Options, err = contextStructuralBindingOptions(snapshot.Binding, restored.Options)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHarnessStructuralRestoreUnavailable, err)
	}
	restored.RestorePlan = &plan
	command := contextStructuralCommand(snapshot.CommandID, action, restored.Ref)
	if command == nil {
		return fmt.Errorf("%w: unsupported structural snapshot kind %q", ErrHarnessStructuralRestoreUnavailable, snapshot.Kind)
	}
	conversation := &contextStructuralConversation{action: action, operation: restored.Operation, emit: restored.Emit}
	registration, err := e.register(ref, command, HarnessTurnSpec{
		CommandID: snapshot.CommandID, CommandKind: AgentCommandKind(action),
		Conversation: conversation, Options: restored.Options, Emit: restored.Emit,
	})
	if err != nil {
		return err
	}
	registration.accept()
	log.Printf(
		"[agent-harness] cold structural operation registered and pinned binding=%+v command_id=%s operation_id=%s action=%s",
		snapshot.Binding,
		snapshot.CommandID,
		snapshot.OperationID,
		action,
	)
	return nil
}

func contextStructuralCommand(
	commandID agentruntime.CommandID,
	action ContextStructuralAction,
	ref agentruntime.ContextCompactionRef,
) agentruntime.Command {
	switch action {
	case ContextStructuralCompact:
		return agentruntime.CompactIfNeeded{ID: commandID, Ref: cloneContextCompactionRef(ref)}
	case ContextStructuralRemove:
		return agentruntime.RemoveCompaction{ID: commandID, Ref: cloneContextCompactionRef(ref)}
	default:
		return nil
	}
}

func contextStructuralActionForKind(kind agentruntime.StructuralOperationKind) ContextStructuralAction {
	switch kind {
	case agentruntime.StructuralCompactContext:
		return ContextStructuralCompact
	case agentruntime.StructuralRemoveCompaction:
		return ContextStructuralRemove
	default:
		return ""
	}
}

func contextStructuralBindingOptions(binding agentruntime.BindingRef, options RunOptions) (RunOptions, error) {
	options.Workspace = binding.Workspace
	options.SessionID = binding.SessionID
	options.StoryID = binding.StoryID
	options.BranchID = binding.BranchID
	options.AutomationTaskID = ""
	switch binding.Profile {
	case agentruntime.ProfileWriting:
		options.AgentKind = AgentKindIDE
		options.Mode = "ide"
	case agentruntime.ProfileGame:
		options.AgentKind = AgentKindInteractiveStory
		options.Mode = "interactive"
	case agentruntime.ProfileConfigManager:
		options.AgentKind = AgentKindConfigManager
	case agentruntime.ProfileImage:
		options.AgentKind = AgentKindImage
	case agentruntime.ProfileDirector:
		options.AgentKind = config.AgentKindInteractiveDirector
		options.Mode = "interactive"
	default:
		return RunOptions{}, fmt.Errorf("unsupported structural binding profile %q", binding.Profile)
	}
	options = options.normalized(binding.Workspace)
	resolved, err := harnessBindingForOptions(options)
	if err != nil {
		return RunOptions{}, err
	}
	resolvedRef, err := agentruntime.BindingReference(resolved)
	if err != nil {
		return RunOptions{}, err
	}
	if resolvedRef != binding {
		return RunOptions{}, fmt.Errorf("%w: restored options do not match structural snapshot", ErrHarnessBindingMismatch)
	}
	return options, nil
}

func cloneContextStructuralSnapshot(snapshot agentruntime.StructuralOperationSnapshot) agentruntime.StructuralOperationSnapshot {
	snapshot.Ref = cloneContextCompactionRef(snapshot.Ref)
	return snapshot
}

func cloneContextCompactionRef(ref agentruntime.ContextCompactionRef) agentruntime.ContextCompactionRef {
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
	harness *agentruntime.Harness,
	expectedAction ContextStructuralAction,
) (ContextStructuralResult, bool, error) {
	if h == nil || harness == nil {
		return ContextStructuralResult{}, false, fmt.Errorf("agent durable runtime is unavailable")
	}
	status, err := harness.Status(ctx)
	if err != nil {
		return ContextStructuralResult{}, false, err
	}
	if !status.RecoveryPaused || status.Phase != agentruntime.PhaseCompacting || status.ActiveStructural == nil {
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
	plan, err := DecodeContextStructuralRestorePlan(snapshot.Ref.RestoreDescriptor, snapshot.Binding, snapshot.Ref.ExpectedRevision)
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
	log.Printf(
		"[agent-harness] explicitly resuming recovery-paused structural operation binding=%+v command_id=%s operation_id=%s action=%s",
		snapshot.Binding,
		snapshot.CommandID,
		snapshot.OperationID,
		action,
	)
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
	log.Printf(
		"[agent-harness] recovered structural operation settled binding=%+v command_id=%s operation_id=%s action=%s",
		snapshot.Binding,
		snapshot.CommandID,
		snapshot.OperationID,
		action,
	)
	return cloneContextStructuralRestorePlan(plan).Result, true, nil
}

// Caller cancellation durably requests Abort for the exact recovered
// operation, then keeps observing until the actor publishes a terminal event.
// This prevents a caller from returning while a canonical commit can still
// become authoritative behind the selected in-memory projection.
func (h *chatHarness) waitForRecoveredStructuralSettlement(
	caller context.Context,
	harness *agentruntime.Harness,
	observation agentruntime.Observation,
	receipt agentruntime.Receipt,
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
			case agentruntime.OperationSettledEvent:
				if payload.OperationID == receipt.OperationID {
					return structuralSettlementError(&agentruntime.OperationSummary{
						OperationID: payload.OperationID, Status: payload.Status, Reason: payload.Reason,
					})
				}
			case agentruntime.OperationInterruptedEvent:
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
			_, err := harness.Submit(h.lifecycle, agentruntime.Abort{
				ID: agentruntime.CommandID(newHarnessIdentity("command")), OperationID: receipt.OperationID, Reason: reason,
			})
			if err != nil && !errors.Is(err, agentruntime.ErrInvalidCommand) &&
				!errors.Is(err, agentruntime.ErrStaleOperation) && !errors.Is(err, agentruntime.ErrDomainCommitRejected) {
				return err
			}
		case <-h.lifecycle.Done():
			return h.lifecycle.Err()
		}
	}
}
