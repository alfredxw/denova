package runtime

import (
	"context"
	"fmt"
	"strings"
)

// InputMaterializationRequest is the immutable accepted cycle projected to a
// provider-free adapter boundary. Implementations must derive canonical input
// exclusively from Binding and Snapshot; no model, tool, or mutable registry
// lookup is permitted at this boundary.
type InputMaterializationRequest struct {
	Binding  BindingRef
	Snapshot TurnSnapshot
}

// InputMaterializationPlan describes whether this binding has canonical input
// state and, when it does, the stable semantic hash authorized by the journal.
type InputMaterializationPlan struct {
	Required bool
	Hash     string
}

// InputMaterializationReceipt identifies the canonical append acknowledged by
// the domain store. Revision must be stable for idempotent retries.
type InputMaterializationReceipt struct {
	Revision string
}

// EngineInputMaterializer is an optional adapter extension executed after a
// cycle is durably accepted and before Engine.Run. Planning must be pure and
// provider-free. MaterializeInput must be idempotent for the exact binding,
// command, operation, cycle, and semantic hash because recovery may retry it.
type EngineInputMaterializer interface {
	PlanInputMaterialization(context.Context, InputMaterializationRequest) (InputMaterializationPlan, error)
	MaterializeInput(context.Context, InputMaterializationRequest, InputMaterializationPlan) (InputMaterializationReceipt, error)
}

// ensureInputMaterialized closes the accepted-input outbox before Engine.Run.
// It is safe both on the normal actor path and during provider-free recovery.
func (h *Harness) ensureInputMaterialized(ctx context.Context, state *harnessState) error {
	if state.phase != PhaseRunning {
		return nil
	}
	materializer, ok := h.engine.(EngineInputMaterializer)
	if !ok {
		return nil
	}
	if commit := state.domainCommit(DomainCommitInput); commit != nil && strings.TrimSpace(commit.Revision) != "" {
		return nil
	}

	request := InputMaterializationRequest{
		Binding:  state.binding.Clone(),
		Snapshot: state.turnSnapshot(state.activeSnapshotID),
	}
	plan, err := planInputMaterialization(ctx, materializer, request)
	if err != nil {
		return fmt.Errorf("plan accepted input materialization: %w", err)
	}
	plan.Hash = strings.TrimSpace(plan.Hash)
	existing := state.domainCommit(DomainCommitInput)
	if !plan.Required {
		if plan.Hash != "" {
			return fmt.Errorf("%w: optional input materialization plan contains a hash", ErrDomainCommitRejected)
		}
		if existing != nil {
			return fmt.Errorf("%w: accepted input intent no longer has a materialization plan", ErrDomainCommitRejected)
		}
		return nil
	}
	if plan.Hash == "" || len(plan.Hash) > 256 {
		return fmt.Errorf("%w: input materialization hash is invalid", ErrDomainCommitRejected)
	}

	identity := DomainCommitIdentity{
		CommandID:   state.activeCycleCommandID,
		OperationID: state.activeOperation,
		Cycle:       state.activeCycle,
		Stage:       DomainCommitInput,
	}
	intentWasPending := existing != nil
	if existing == nil {
		if _, err := h.commit(ctx, state, []EventPayload{DomainCommitIntentAcceptedEvent{
			Identity: identity,
			Hash:     plan.Hash,
		}}); err != nil {
			return fmt.Errorf("persist accepted input materialization intent: %w", err)
		}
		existing = state.domainCommit(DomainCommitInput)
	} else if existing.Identity != identity || existing.Hash != plan.Hash {
		return fmt.Errorf("%w: accepted input materialization identity or hash changed", ErrDomainCommitRejected)
	}

	// An intent observed before this attempt may have crossed the canonical
	// write boundary before its receipt was appended. Query it before invoking
	// the idempotent writer so recovery never blindly replays that effect.
	if intentWasPending {
		found, revision, err := h.queryInputMaterialization(ctx, state, *existing)
		if err != nil {
			return err
		}
		if found {
			return h.recordInputMaterializationReceipt(ctx, state, identity, plan.Hash, revision)
		}
	}

	receipt, err := materializeInput(ctx, materializer, request, plan)
	if err != nil {
		return fmt.Errorf("materialize accepted input: %w", err)
	}
	return h.recordInputMaterializationReceipt(ctx, state, identity, plan.Hash, receipt.Revision)
}

func (h *Harness) queryInputMaterialization(
	ctx context.Context,
	state *harnessState,
	commit DomainCommitState,
) (bool, string, error) {
	reconciler, ok := h.engine.(EngineDomainCommitReconciler)
	if !ok {
		return false, "", nil
	}
	result, err := queryDomainCommit(ctx, reconciler, DomainCommitReconcileRequest{
		Binding: state.binding.Clone(), Commit: commit,
		Snapshot: state.turnSnapshot(state.activeSnapshotID),
		State:    cloneRawMessage(state.engineState), Capabilities: cloneCapabilityStates(state.capabilityStates),
	})
	if err != nil {
		return false, "", fmt.Errorf("reconcile accepted input materialization: %w", err)
	}
	result.Revision = strings.TrimSpace(result.Revision)
	if !result.Found {
		if result.Revision != "" {
			return false, "", fmt.Errorf("reconcile accepted input materialization: not-found result contains revision")
		}
		return false, "", nil
	}
	if result.Revision == "" {
		return false, "", fmt.Errorf("reconcile accepted input materialization: found result requires revision")
	}
	return true, result.Revision, nil
}

func (h *Harness) recordInputMaterializationReceipt(
	ctx context.Context,
	state *harnessState,
	identity DomainCommitIdentity,
	hash string,
	revision string,
) error {
	revision = strings.TrimSpace(revision)
	if revision == "" || len(revision) > 4096 {
		return fmt.Errorf("%w: input materialization revision is invalid", ErrDomainCommitRejected)
	}
	commit := state.domainCommit(DomainCommitInput)
	if commit == nil || commit.Identity != identity || commit.Hash != hash {
		return fmt.Errorf("%w: input materialization receipt does not match its intent", ErrDomainCommitRejected)
	}
	if commit.Revision != "" {
		if commit.Revision == revision {
			return nil
		}
		return fmt.Errorf("%w: input materialization already has a different revision", ErrDomainCommitRejected)
	}
	if _, err := h.commit(ctx, state, []EventPayload{DomainCommitReceiptEvent{
		Identity: identity,
		Hash:     hash,
		Revision: revision,
	}}); err != nil {
		return fmt.Errorf("persist accepted input materialization receipt: %w", err)
	}
	return nil
}

func planInputMaterialization(
	ctx context.Context,
	materializer EngineInputMaterializer,
	request InputMaterializationRequest,
) (result InputMaterializationPlan, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			resultErr = fmt.Errorf("input materialization planner panic: %v", recovered)
		}
	}()
	return materializer.PlanInputMaterialization(ctx, request)
}

func materializeInput(
	ctx context.Context,
	materializer EngineInputMaterializer,
	request InputMaterializationRequest,
	plan InputMaterializationPlan,
) (result InputMaterializationReceipt, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			resultErr = fmt.Errorf("input materializer panic: %v", recovered)
		}
	}()
	return materializer.MaterializeInput(ctx, request, plan)
}
