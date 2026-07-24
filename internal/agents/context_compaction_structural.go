package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"denova/internal/agents/session"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

// ContextStructuralAction is closed because structural context mutations must
// be understood by the durable actor, journal reducer, and canonical store.
type ContextStructuralAction string

const (
	ContextStructuralCompact ContextStructuralAction = "compact_context"
	ContextStructuralRemove  ContextStructuralAction = "remove_compaction"
)

type ContextStructuralIdentity struct {
	CommandID   runstate.CommandID
	OperationID runstate.OperationID
	Cycle       int
}

// ContextStructuralIntent is prepared without canonical writes. Commit=false
// represents a policy skip and settles successfully without opening a domain
// commit barrier.
type ContextStructuralIntent struct {
	Hash   string
	Commit bool
	Result ContextStructuralResult
}

type ContextStructuralReceipt struct {
	Revision string
}

type ContextStructuralResult struct {
	Compaction ContextCompactionResult
	Removed    bool
}

// ContextStructuralOperation splits expensive/model preparation from the
// actor-authorized CAS write. Reconcile must recognize an exact deterministic
// identity after an ambiguous write or a transport retry.
type ContextStructuralOperation interface {
	Prepare(context.Context, ContextStructuralIdentity, func(Event)) (ContextStructuralIntent, error)
	Commit(context.Context, ContextStructuralIdentity, ContextStructuralIntent) (ContextStructuralReceipt, error)
	Reconcile(context.Context) (ContextStructuralResult, ContextStructuralReceipt, bool, error)
}

type ContextStructuralSpec struct {
	CommandID string
	Action    ContextStructuralAction
	Ref       runstate.ContextCompactionRef
	Options   RunOptions
	Emit      func(Event)
	Operation ContextStructuralOperation
	// RestorePlan is the exact bounded mutation used to rebuild Operation after
	// a process restart. It is encoded into Ref before durable admission.
	RestorePlan *ContextStructuralRestorePlan
}

type contextStructuralConversation struct {
	action    ContextStructuralAction
	operation ContextStructuralOperation
	emit      func(Event)

	mu      sync.Mutex
	result  ContextStructuralResult
	receipt ContextStructuralReceipt
	err     error
	done    bool
}

func (c *contextStructuralConversation) AssembleModelContext(context.Context, string, ModelContextInput) (ModelContextResult, error) {
	return ModelContextResult{}, errors.New("structural context operation cannot assemble chat messages")
}

func (c *contextStructuralConversation) AppendAssistant(string) error {
	return errors.New("structural context operation cannot append assistant output")
}

func (c *contextStructuralConversation) MarkInterrupted(string, string, string) error { return nil }
func (c *contextStructuralConversation) PendingInterruption() *session.Interruption   { return nil }
func (c *contextStructuralConversation) ResolveInterruption(string) error             { return nil }

func (c *contextStructuralConversation) finish(result ContextStructuralResult, receipt ContextStructuralReceipt, err error) {
	c.mu.Lock()
	c.result = result
	c.receipt = receipt
	c.err = err
	c.done = true
	c.mu.Unlock()
}

func (c *contextStructuralConversation) outcome() (ContextStructuralResult, ContextStructuralReceipt, error, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.result, c.receipt, c.err, c.done
}

// RunStructural extends the per-binding guard without weakening the normal
// harnessEngine.Run validation path.
func (e *bindingHarnessEngine) RunStructural(
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
			ErrHarnessBindingMismatch, e.binding, request.Binding, request.Snapshot.Binding,
		)
	}
	return e.owner.runStructural(ctx, request, emit, e.binding)
}

func (e *bindingHarnessEngine) RestoreStructuralOperation(
	ctx context.Context,
	snapshot runstate.StructuralOperationSnapshot,
) error {
	if e == nil || e.owner == nil {
		return fmt.Errorf("restore structural context operation: engine is nil")
	}
	if !snapshot.Binding.Equal(e.binding) {
		return fmt.Errorf(
			"%w: factory=%+v snapshot=%+v",
			ErrHarnessBindingMismatch, e.binding, snapshot.Binding,
		)
	}
	return e.owner.restoreStructuralOperation(ctx, snapshot)
}

func (e *harnessEngine) runStructural(
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
	if err := validateHarnessTurnBinding(spec.Options, expectedBinding); err != nil {
		return runstate.EngineResult{}, err
	}
	conversation, ok := spec.Conversation.(*contextStructuralConversation)
	if !ok || conversation == nil || conversation.operation == nil {
		return runstate.EngineResult{}, fmt.Errorf("%w: structural context operation is required", ErrHarnessTurnSpecInvalid)
	}
	if spec.CommandID != request.Snapshot.CommandID || structuralKindForAction(conversation.action) != request.Snapshot.Kind {
		return runstate.EngineResult{}, fmt.Errorf("%w: structural command identity changed before execution", ErrHarnessTurnSpecConflict)
	}
	identity := ContextStructuralIdentity{
		CommandID: request.Snapshot.CommandID, OperationID: request.Snapshot.OperationID, Cycle: request.Snapshot.Cycle,
	}
	intent, err := conversation.operation.Prepare(ctx, identity, conversation.emit)
	if err != nil {
		conversation.finish(intent.Result, ContextStructuralReceipt{}, err)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return runstate.EngineResult{Status: runstate.EngineAborted}, nil
		}
		return runstate.EngineResult{}, err
	}
	if !intent.Commit {
		conversation.finish(intent.Result, ContextStructuralReceipt{}, nil)
		return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
	}
	if strings.TrimSpace(intent.Hash) == "" {
		err := fmt.Errorf("structural context intent hash is required")
		conversation.finish(intent.Result, ContextStructuralReceipt{}, err)
		return runstate.EngineResult{}, err
	}
	domainIdentity := runstate.DomainCommitIdentity{
		CommandID: identity.CommandID, OperationID: identity.OperationID,
		Cycle: identity.Cycle, Stage: runstate.DomainCommitOutput,
	}
	if err := emit(runstate.EngineDomainCommitIntent{Identity: domainIdentity, Hash: intent.Hash}); err != nil {
		conversation.finish(intent.Result, ContextStructuralReceipt{}, err)
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
		conversation.finish(intent.Result, ContextStructuralReceipt{}, err)
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

func reconcileContextStructuralOperation(ctx context.Context, operation ContextStructuralOperation) (ContextStructuralResult, ContextStructuralReceipt, bool, error) {
	if operation == nil {
		return ContextStructuralResult{}, ContextStructuralReceipt{}, false, nil
	}
	return operation.Reconcile(ctx)
}

func structuralKindForAction(action ContextStructuralAction) runstate.StructuralOperationKind {
	switch action {
	case ContextStructuralCompact:
		return runstate.StructuralCompactContext
	case ContextStructuralRemove:
		return runstate.StructuralRemoveCompaction
	default:
		return ""
	}
}

// ExecuteContextStructuralOperation durably accepts, runs, and observes one
// compaction mutation on the same exact binding used by normal Agent turns.
func (s *ChatService) ExecuteContextStructuralOperation(ctx context.Context, spec ContextStructuralSpec) (ContextStructuralResult, error) {
	if s == nil || s.harness == nil || s.harness.runtime == nil || s.harness.engine == nil {
		return ContextStructuralResult{}, fmt.Errorf("agent durable runtime is unavailable")
	}
	return s.harness.executeContextStructuralOperation(ctx, spec)
}

func (h *chatHarness) executeContextStructuralOperation(ctx context.Context, spec ContextStructuralSpec) (ContextStructuralResult, error) {
	if h == nil || h.runtime == nil || h.engine == nil {
		return ContextStructuralResult{}, fmt.Errorf("agent durable runtime is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ContextStructuralResult{}, err
	}
	if strings.TrimSpace(spec.CommandID) == "" || spec.Operation == nil || structuralKindForAction(spec.Action) == "" {
		return ContextStructuralResult{}, fmt.Errorf("%w: complete structural context spec is required", runstate.ErrInvalidCommand)
	}
	spec.Options = spec.Options.normalized(spec.Options.Workspace)
	binding, err := harnessBindingForOptions(spec.Options)
	if err != nil {
		return ContextStructuralResult{}, err
	}
	bindingRef, err := runstate.BindingReference(binding)
	if err != nil {
		return ContextStructuralResult{}, err
	}
	if spec.RestorePlan == nil && len(spec.Ref.RestoreDescriptor) == 0 {
		return ContextStructuralResult{}, fmt.Errorf(
			"%w: structural context operations require an exact restore plan",
			runstate.ErrInvalidCommand,
		)
	}
	if spec.RestorePlan != nil {
		if spec.RestorePlan.Action != spec.Action {
			return ContextStructuralResult{}, fmt.Errorf("%w: structural restore plan action does not match spec", runstate.ErrInvalidCommand)
		}
		descriptor, encodeErr := EncodeContextStructuralRestorePlan(*spec.RestorePlan, bindingRef, spec.Ref.ExpectedRevision)
		if encodeErr != nil {
			return ContextStructuralResult{}, fmt.Errorf("%w: %v", runstate.ErrInvalidCommand, encodeErr)
		}
		spec.Ref.RestoreDescriptor = descriptor
	} else if len(spec.Ref.RestoreDescriptor) > 0 {
		plan, decodeErr := DecodeContextStructuralRestorePlan(spec.Ref.RestoreDescriptor, bindingRef, spec.Ref.ExpectedRevision)
		if decodeErr != nil {
			return ContextStructuralResult{}, fmt.Errorf("%w: %v", runstate.ErrInvalidCommand, decodeErr)
		}
		if plan.Action != spec.Action {
			return ContextStructuralResult{}, fmt.Errorf("%w: structural restore descriptor action does not match spec", runstate.ErrInvalidCommand)
		}
	}
	harness, err := h.runtime.Open(ctx, binding)
	if err != nil {
		return ContextStructuralResult{}, err
	}
	refSemantics := semanticJSONFingerprint("agent-structural-reference.v1", struct {
		Action ContextStructuralAction       `json:"action"`
		Ref    runstate.ContextCompactionRef `json:"ref"`
	}{Action: spec.Action, Ref: spec.Ref})
	spec.Ref.SpecRef = harnessCommandTurnRef(binding, spec.CommandID, refSemantics)
	var command runstate.Command
	switch spec.Action {
	case ContextStructuralCompact:
		command = runstate.CompactIfNeeded{ID: runstate.CommandID(spec.CommandID), Ref: spec.Ref}
	case ContextStructuralRemove:
		command = runstate.RemoveCompaction{ID: runstate.CommandID(spec.CommandID), Ref: spec.Ref}
	}
	conversation := &contextStructuralConversation{action: spec.Action, operation: spec.Operation, emit: spec.Emit}
	registration, err := h.engine.register(spec.Ref.SpecRef, command, HarnessTurnSpec{
		CommandID: runstate.CommandID(spec.CommandID), CommandKind: AgentCommandKind(spec.Action),
		Conversation: conversation, Options: spec.Options, Emit: spec.Emit,
	})
	if err != nil {
		return ContextStructuralResult{}, err
	}
	defer registration.release()
	observeCtx, stopObserving := context.WithCancel(h.lifecycle)
	defer stopObserving()
	observation, err := harness.ObserveFromNow(observeCtx)
	if err != nil {
		return ContextStructuralResult{}, err
	}
	receipt, err := harness.Submit(ctx, command)
	if err != nil {
		return ContextStructuralResult{}, err
	}
	if !receipt.Replayed {
		registration.accept()
	}
	if err := h.waitForStructuralSettlement(ctx, harness, observation, receipt); err != nil {
		return ContextStructuralResult{}, err
	}
	if result, _, runErr, done := conversation.outcome(); done {
		return result, runErr
	}
	result, _, found, err := spec.Operation.Reconcile(context.WithoutCancel(ctx))
	if err != nil {
		return ContextStructuralResult{}, err
	}
	if !found {
		return ContextStructuralResult{}, fmt.Errorf("structural context operation %s settled without a canonical receipt", receipt.OperationID)
	}
	return result, nil
}

func (h *chatHarness) waitForStructuralSettlement(
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
				ID: runstate.CommandID(newHarnessIdentity("command")), OperationID: receipt.OperationID, Reason: reason,
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
