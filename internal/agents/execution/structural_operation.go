package execution

import (
	"context"
	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"errors"
	"fmt"
	"strings"
	"sync"

	agentstructural "denova/internal/agents/context/structural"
	"denova/internal/agents/session"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

type contextStructuralConversation struct {
	action    agentstructural.Action
	operation agentstructural.Operation
	emit      func(agentrun.Event)

	mu      sync.Mutex
	result  agentstructural.Result
	receipt agentstructural.Receipt
	err     error
	done    bool
}

func (c *contextStructuralConversation) AssembleModelContext(context.Context, string, agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	return agentcontext.ModelContextResult{}, errors.New("structural context operation cannot assemble chat messages")
}

func (c *contextStructuralConversation) AppendAssistant(string) error {
	return errors.New("structural context operation cannot append assistant output")
}

func (c *contextStructuralConversation) MarkInterrupted(string, string, string) error { return nil }
func (c *contextStructuralConversation) PendingInterruption() *session.Interruption   { return nil }
func (c *contextStructuralConversation) ResolveInterruption(string) error             { return nil }

func (c *contextStructuralConversation) finish(result agentstructural.Result, receipt agentstructural.Receipt, err error) {
	c.mu.Lock()
	c.result = result
	c.receipt = receipt
	c.err = err
	c.done = true
	c.mu.Unlock()
}

func (c *contextStructuralConversation) outcome() (agentstructural.Result, agentstructural.Receipt, error, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.result, c.receipt, c.err, c.done
}

// RunStructural extends the per-binding guard without weakening the normal
// durableEngine.Run validation path.
func (e *bindingEngine) RunStructural(
	ctx context.Context,
	request runstate.StructuralEngineRequest,
	emit runstate.EngineEventSink,
) (runstate.EngineResult, error) {
	if e == nil || e.owner == nil {
		return runstate.EngineResult{}, fmt.Errorf("run structural context operation: engine is nil")
	}
	if !request.Binding.Equal(e.binding) || !request.Snapshot.Binding.Equal(e.binding) {
		return runstate.EngineResult{}, fmt.Errorf(
			"%w: factory=%+v request=%+v snapshot=%+v",
			ErrBindingMismatch, e.binding, request.Binding, request.Snapshot.Binding,
		)
	}
	return e.owner.runStructural(ctx, request, emit, e.binding)
}

func (e *bindingEngine) RestoreStructuralOperation(
	ctx context.Context,
	snapshot runstate.StructuralOperationSnapshot,
) error {
	if e == nil || e.owner == nil {
		return fmt.Errorf("restore structural context operation: engine is nil")
	}
	if !snapshot.Binding.Equal(e.binding) {
		return fmt.Errorf(
			"%w: factory=%+v snapshot=%+v",
			ErrBindingMismatch, e.binding, snapshot.Binding,
		)
	}
	return e.owner.restoreStructuralOperation(ctx, snapshot)
}

func (e *durableEngine) runStructural(
	ctx context.Context,
	request runstate.StructuralEngineRequest,
	emit runstate.EngineEventSink,
	expectedBinding runstate.BindingRef,
) (runstate.EngineResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if emit == nil {
		return runstate.EngineResult{}, fmt.Errorf("run structural context operation: event sink is required")
	}
	spec, err := e.take(request.Snapshot.Ref.SpecRef)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	if err := validateCycleBinding(spec.Options, expectedBinding); err != nil {
		return runstate.EngineResult{}, err
	}
	conversation, ok := spec.Conversation.(*contextStructuralConversation)
	if !ok || conversation == nil || conversation.operation == nil {
		return runstate.EngineResult{}, fmt.Errorf("%w: structural context operation is required", ErrCycleSpecInvalid)
	}
	if runstate.CommandID(spec.CommandID) != request.Snapshot.CommandID || agentstructural.RuntimeKind(conversation.action) != request.Snapshot.Kind {
		return runstate.EngineResult{}, fmt.Errorf("%w: structural command identity changed before execution", ErrCycleSpecConflict)
	}
	identity := agentstructural.Identity{
		CommandID: agentrun.CommandID(request.Snapshot.CommandID), OperationID: agentrun.OperationID(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle,
	}
	intent, err := conversation.operation.Prepare(ctx, identity, conversation.emit)
	if err != nil {
		conversation.finish(intent.Result, agentstructural.Receipt{}, err)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return runstate.EngineResult{Status: runstate.EngineAborted}, nil
		}
		return runstate.EngineResult{}, err
	}
	if !intent.Commit {
		conversation.finish(intent.Result, agentstructural.Receipt{}, nil)
		return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
	}
	if strings.TrimSpace(intent.Hash) == "" {
		err := fmt.Errorf("structural context intent hash is required")
		conversation.finish(intent.Result, agentstructural.Receipt{}, err)
		return runstate.EngineResult{}, err
	}
	domainIdentity := runstate.DomainCommitIdentity{
		CommandID: runstate.CommandID(identity.CommandID), OperationID: runstate.OperationID(identity.OperationID),
		Cycle: identity.Cycle, Stage: runstate.DomainCommitOutput,
	}
	if err := emit(runstate.EngineDomainCommitIntent{Identity: domainIdentity, Hash: intent.Hash}); err != nil {
		conversation.finish(intent.Result, agentstructural.Receipt{}, err)
		return runstate.EngineResult{}, err
	}
	receipt, err := conversation.operation.Commit(ctx, identity, intent)
	if err != nil {
		// Commit may have reached canonical storage before returning an error.
		// Reconcile exact deterministic identity before leaving an authorized
		// intent without its durable runtime receipt.
		result, reconciled, found, reconcileErr := reconcileContextStructuralOperation(ctx, conversation.operation)
		if reconcileErr != nil {
			err = errors.Join(err, reconcileErr)
		} else if found {
			intent.Result = result
			receipt = reconciled
			err = nil
		}
	}
	if err != nil {
		conversation.finish(intent.Result, agentstructural.Receipt{}, err)
		return runstate.EngineResult{}, err
	}
	if strings.TrimSpace(receipt.Revision) == "" {
		err := fmt.Errorf("structural context commit revision is required")
		conversation.finish(intent.Result, receipt, err)
		return runstate.EngineResult{}, err
	}
	if err := emit(runstate.EngineDomainCommitReceipt{Identity: domainIdentity, Hash: intent.Hash, Revision: receipt.Revision}); err != nil {
		conversation.finish(intent.Result, receipt, err)
		return runstate.EngineResult{}, err
	}
	conversation.finish(intent.Result, receipt, nil)
	return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
}

func reconcileContextStructuralOperation(ctx context.Context, operation agentstructural.Operation) (agentstructural.Result, agentstructural.Receipt, bool, error) {
	if operation == nil {
		return agentstructural.Result{}, agentstructural.Receipt{}, false, nil
	}
	return operation.Reconcile(ctx)
}

// ExecuteStructuralOperation durably accepts, runs, and observes one
// compaction mutation on the same exact binding used by normal Agent turns.
func (s *Runtime) ExecuteStructuralOperation(ctx context.Context, spec agentstructural.Spec) (agentstructural.Result, error) {
	if s == nil || s.coordinator == nil || s.coordinator.runtime == nil || s.coordinator.engine == nil {
		return agentstructural.Result{}, fmt.Errorf("agent durable runtime is unavailable")
	}
	return s.coordinator.executeContextStructuralOperation(ctx, spec)
}

func (h *coordinator) executeContextStructuralOperation(ctx context.Context, spec agentstructural.Spec) (agentstructural.Result, error) {
	if h == nil || h.runtime == nil || h.engine == nil {
		return agentstructural.Result{}, fmt.Errorf("agent durable runtime is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return agentstructural.Result{}, err
	}
	if strings.TrimSpace(spec.CommandID) == "" || spec.Operation == nil || agentstructural.RuntimeKind(spec.Action) == "" {
		return agentstructural.Result{}, fmt.Errorf("%w: complete structural context spec is required", runstate.ErrInvalidCommand)
	}
	spec.Options = spec.Options.Normalize(spec.Options.Workspace)
	binding, err := agentrun.BindingForOptions(spec.Options)
	if err != nil {
		return agentstructural.Result{}, err
	}
	bindingRef, err := runstate.BindingReference(binding)
	if err != nil {
		return agentstructural.Result{}, err
	}
	if spec.RestorePlan == nil && len(spec.Ref.RestoreDescriptor) == 0 {
		return agentstructural.Result{}, fmt.Errorf(
			"%w: structural context operations require an exact restore plan",
			runstate.ErrInvalidCommand,
		)
	}
	if spec.RestorePlan != nil {
		if spec.RestorePlan.Action != spec.Action {
			return agentstructural.Result{}, fmt.Errorf("%w: structural restore plan action does not match spec", runstate.ErrInvalidCommand)
		}
		productBinding, projectErr := agentrun.ParseRuntimeBinding(bindingRef)
		if projectErr != nil {
			return agentstructural.Result{}, projectErr
		}
		descriptor, encodeErr := agentstructural.EncodeRestorePlan(*spec.RestorePlan, productBinding, spec.Ref.ExpectedRevision)
		if encodeErr != nil {
			return agentstructural.Result{}, fmt.Errorf("%w: %v", runstate.ErrInvalidCommand, encodeErr)
		}
		spec.Ref.RestoreDescriptor = descriptor
	} else if len(spec.Ref.RestoreDescriptor) > 0 {
		productBinding, projectErr := agentrun.ParseRuntimeBinding(bindingRef)
		if projectErr != nil {
			return agentstructural.Result{}, projectErr
		}
		plan, decodeErr := agentstructural.DecodeRestorePlan(spec.Ref.RestoreDescriptor, productBinding, spec.Ref.ExpectedRevision)
		if decodeErr != nil {
			return agentstructural.Result{}, fmt.Errorf("%w: %v", runstate.ErrInvalidCommand, decodeErr)
		}
		if plan.Action != spec.Action {
			return agentstructural.Result{}, fmt.Errorf("%w: structural restore descriptor action does not match spec", runstate.ErrInvalidCommand)
		}
	}
	harness, err := h.runtime.Open(ctx, binding)
	if err != nil {
		return agentstructural.Result{}, err
	}
	refSemantics := semanticJSONFingerprint("agent-structural-reference.v1", struct {
		Action agentstructural.Action        `json:"action"`
		Ref    agentrun.ContextCompactionRef `json:"ref"`
	}{Action: spec.Action, Ref: spec.Ref})
	spec.Ref.SpecRef = commandCycleRef(binding, spec.CommandID, refSemantics)
	runtimeRef := agentrun.ContextCompactionRefToRuntime(spec.Ref)
	var command runstate.Command
	switch spec.Action {
	case agentstructural.Compact:
		command = runstate.CompactIfNeeded{ID: runstate.CommandID(spec.CommandID), Ref: runtimeRef}
	case agentstructural.Remove:
		command = runstate.RemoveCompaction{ID: runstate.CommandID(spec.CommandID), Ref: runtimeRef}
	}
	conversation := &contextStructuralConversation{action: spec.Action, operation: spec.Operation, emit: spec.Emit}
	registration, err := h.engine.register(spec.Ref.SpecRef, command, cycleSpec{
		CommandID: agentrun.CommandID(spec.CommandID), CommandKind: CommandKind(spec.Action),
		Conversation: conversation, Options: spec.Options, Emit: spec.Emit,
	})
	if err != nil {
		return agentstructural.Result{}, err
	}
	defer registration.release()
	observeCtx, stopObserving := context.WithCancel(h.lifecycle)
	defer stopObserving()
	observation, err := harness.ObserveFromNow(observeCtx)
	if err != nil {
		return agentstructural.Result{}, err
	}
	receipt, err := harness.Submit(ctx, command)
	if err != nil {
		return agentstructural.Result{}, err
	}
	if !receipt.Replayed {
		registration.accept()
	}
	if err := h.waitForStructuralSettlement(ctx, harness, observation, receipt); err != nil {
		return agentstructural.Result{}, err
	}
	if result, _, runErr, done := conversation.outcome(); done {
		return result, runErr
	}
	result, _, found, err := spec.Operation.Reconcile(context.WithoutCancel(ctx))
	if err != nil {
		return agentstructural.Result{}, err
	}
	if !found {
		return agentstructural.Result{}, fmt.Errorf("structural context operation %s settled without a canonical receipt", receipt.OperationID)
	}
	return result, nil
}

func (h *coordinator) waitForStructuralSettlement(
	caller context.Context,
	harness *runstate.Harness,
	observation runstate.Observation,
	receipt runstate.Receipt,
) error {
	if status, err := harness.Status(h.lifecycle); err == nil && operationAlreadySettled(status, receipt) {
		return structuralSettlementError(status.LastOperation)
	}
	callerDone := caller.Done()
	abortSent := false
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
					return structuralSettlementError(&runstate.OperationSummary{OperationID: payload.OperationID, Status: payload.Status, Reason: payload.Reason})
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
			if abortSent {
				continue
			}
			abortSent = true
			reason := "structural context caller canceled"
			if err := caller.Err(); err != nil {
				reason = err.Error()
			}
			_, err := harness.Submit(h.lifecycle, runstate.Abort{
				ID: runstate.CommandID(newOperationIdentity("command")), OperationID: receipt.OperationID, Reason: reason,
			})
			if err != nil && !errors.Is(err, runstate.ErrInvalidCommand) && !errors.Is(err, runstate.ErrStaleOperation) && !errors.Is(err, runstate.ErrDomainCommitRejected) {
				return err
			}
		case <-h.lifecycle.Done():
			return h.lifecycle.Err()
		}
	}
}

func operationAlreadySettled(status runstate.StatusSnapshot, receipt runstate.Receipt) bool {
	return status.LastOperation != nil && status.LastOperation.OperationID == receipt.OperationID
}

func structuralSettlementError(summary *runstate.OperationSummary) error {
	if summary == nil || summary.Status == runstate.OperationSucceeded {
		return nil
	}
	reason := strings.TrimSpace(summary.Reason)
	if reason == "" {
		reason = fmt.Sprintf("structural context operation %s", summary.Status)
	}
	return errors.New(reason)
}
