package agents

import (
	"encoding/json"
	"fmt"
	"strings"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// CommandID is the caller-owned idempotency identity of one Agent command.
// Callers must reuse it while command acceptance is uncertain.
type CommandID string

// OperationID identifies the durable operation selected for an accepted command.
type OperationID string

// Cursor is a durable observation position. It is display/recovery state and
// never participates in model context construction.
type Cursor uint64

// HostEffectID is the idempotency identity of one reconciled tool side effect.
type HostEffectID string

const HostEffectToolMutationCommitted = "tool_mutation_committed"

// CommandReceipt is the stable result of durable command admission.
type CommandReceipt struct {
	CommandID   CommandID
	OperationID OperationID
	Cursor      Cursor
	Replayed    bool
}

// Receipt and StatusSnapshot keep the control vocabulary compact for callers.
// Both names refer to Denova-owned projections, never agent/runtime types.
type Receipt = CommandReceipt

// RunPhase is the closed product projection of durable Agent execution.
type RunPhase string

const (
	RunPhaseIdle       RunPhase = "idle"
	RunPhaseRunning    RunPhase = "running"
	RunPhaseCompacting RunPhase = "compacting"
)

const (
	PhaseIdle       = RunPhaseIdle
	PhaseRunning    = RunPhaseRunning
	PhaseCompacting = RunPhaseCompacting
)

// OperationStatus is the terminal state of one accepted Agent operation.
type OperationStatus string

const (
	OperationSucceeded   OperationStatus = "succeeded"
	OperationFailed      OperationStatus = "failed"
	OperationAborted     OperationStatus = "aborted"
	OperationInterrupted OperationStatus = "interrupted"
	OperationIncomplete  OperationStatus = "incomplete"
)

// DeliveryKind describes how queued input follows an active Agent operation.
type DeliveryKind string

const (
	DeliverySteer    DeliveryKind = "steer"
	DeliveryFollowUp DeliveryKind = "follow_up"
	DeliveryNextTurn DeliveryKind = "next_turn"
)

// ContextCompactionRef identifies one bounded structural context mutation.
// RestoreDescriptor is private recovery input and is never projected to HTTP.
type ContextCompactionRef struct {
	SpecRef           string
	Source            string
	Purpose           string
	Resource          string
	ExpectedRevision  string
	CompactionID      string
	Force             bool
	RestoreDescriptor json.RawMessage
}

// StructuralOperationKind is the closed durable context operation vocabulary.
type StructuralOperationKind string

const (
	StructuralCompactContext   StructuralOperationKind = "compact_context"
	StructuralRemoveCompaction StructuralOperationKind = "remove_compaction"
)

// StructuralOperation is the safe identity of an active context mutation.
type StructuralOperation struct {
	Binding       RuntimeBinding
	CommandID     CommandID
	OperationID   OperationID
	Cycle         int
	Kind          StructuralOperationKind
	Ref           ContextCompactionRef
	ContextCursor Cursor
}

// InputRecovery identifies accepted input whose canonical materialization is
// awaiting an explicit recovery action. The input itself remains private.
type InputRecovery struct {
	CommandID   CommandID
	OperationID OperationID
	Cycle       int
	Delivery    DeliveryKind
}

// QueuedCommand is the bounded display projection of one accepted successor.
type QueuedCommand struct {
	CommandID        CommandID
	OperationID      OperationID
	Delivery         DeliveryKind
	Message          string
	MessageTruncated bool
}

// OpenToolCall is display-only evidence for a tool that has not settled yet.
type OpenToolCall struct {
	CallID      string
	Name        string
	OperationID OperationID
	Cycle       int
}

// ActiveOutput is the bounded in-process stream projection used for reconnects.
type ActiveOutput struct {
	OperationID       OperationID
	Cycle             int
	Content           string
	Thinking          string
	ContentTruncated  bool
	ThinkingTruncated bool
	RehydrateRequired bool
}

// OperationSummary is the bounded terminal evidence retained for retries.
type OperationSummary struct {
	OperationID        OperationID
	CommandID          CommandID
	CommandFingerprint string
	ReceiptCursor      Cursor
	Status             OperationStatus
	Reason             string
	ReasonTruncated    bool
}

// DomainCommitStage names the canonical input/output write protected by the
// durable Agent coordinator.
type DomainCommitStage string

const (
	DomainCommitInput  DomainCommitStage = "input"
	DomainCommitOutput DomainCommitStage = "output"
)

// DomainCommitIdentity correlates one canonical write with an Agent cycle.
type DomainCommitIdentity struct {
	CommandID   CommandID
	OperationID OperationID
	Cycle       int
	Stage       DomainCommitStage
}

// DomainCommitState is the safe recovery evidence for a canonical write.
type DomainCommitState struct {
	Identity  DomainCommitIdentity
	Hash      string
	Revision  string
	Abandoned bool
	Reason    string
}

// RuntimeStatus is Denova's bounded Agent control projection. It deliberately
// omits journal payloads, restore descriptors, tool arguments, host effects,
// and runtime memory accounting so app callers cannot depend on coordinator
// implementation details.
type RuntimeStatus struct {
	Binding                  RuntimeBinding
	Cursor                   Cursor
	Phase                    RunPhase
	ActiveCommandID          CommandID
	ActiveCommandFingerprint string
	ActiveReceiptCursor      Cursor
	ActiveOperation          OperationID
	ActiveCycle              int
	RecoveryPaused           bool
	RecoveryPending          bool
	InputRecovery            *InputRecovery
	ActiveStructural         *StructuralOperation
	ActiveOutput             ActiveOutput
	Queue                    []QueuedCommand
	OpenToolCalls            []OpenToolCall
	LastOperation            *OperationSummary
	RecentOperations         []OperationSummary
	LastDomainCommit         *DomainCommitState
	DomainCommits            []DomainCommitState
}

type StatusSnapshot = RuntimeStatus

// DomainCommitReconcileRequest asks the host to query one exact canonical
// write. Reconciliation must never replay the write.
type DomainCommitReconcileRequest struct {
	Binding    RuntimeBinding
	Commit     DomainCommitState
	Structural *StructuralOperation
}

type DomainCommitReconcileResult struct {
	Found    bool
	Revision string
}

type InputMaterializationPlan struct {
	Required bool
	Hash     string
}

type InputMaterializationReceipt struct {
	Revision string
}

var (
	ErrInvalidCommand       = runstate.ErrInvalidCommand
	ErrInvalidBinding       = runstate.ErrInvalidBinding
	ErrStaleOperation       = runstate.ErrStaleOperation
	ErrQueueConflict        = runstate.ErrQueueConflict
	ErrBusy                 = runstate.ErrBusy
	ErrDomainCommitRejected = runstate.ErrDomainCommitRejected
)

// ValidateCommandID applies the exact durable command envelope without
// exposing runtime configuration to app or transport packages.
func ValidateCommandID(commandID string) error {
	return runstate.ValidateCommandID(commandID, runstate.DefaultInputLimits())
}

// ValidateRecoveryIdentity validates the caller-owned identities required by
// an explicit recovery action.
func ValidateRecoveryIdentity(commandID, operationID string) error {
	if err := ValidateCommandID(commandID); err != nil {
		return err
	}
	limits := runstate.DefaultInputLimits()
	operationID = strings.TrimSpace(operationID)
	if operationID == "" || len(operationID) > limits.MaxOperationIDBytes {
		return ErrInvalidCommand
	}
	return nil
}

func commandReceiptFromRuntime(receipt runstate.Receipt) CommandReceipt {
	return CommandReceipt{
		CommandID: CommandID(receipt.CommandID), OperationID: OperationID(receipt.OperationID),
		Cursor: Cursor(receipt.Cursor), Replayed: receipt.Replayed,
	}
}

func runtimeStatusFromSnapshot(snapshot runstate.StatusSnapshot) (RuntimeStatus, error) {
	binding, err := ParseRuntimeBinding(snapshot.Binding)
	if err != nil {
		return RuntimeStatus{}, fmt.Errorf("project Agent binding: %w", err)
	}
	status := RuntimeStatus{
		Binding: binding, Cursor: Cursor(snapshot.Cursor), Phase: RunPhase(snapshot.Phase),
		ActiveCommandID:          CommandID(snapshot.ActiveCommandID),
		ActiveCommandFingerprint: snapshot.ActiveCommandFingerprint,
		ActiveReceiptCursor:      Cursor(snapshot.ActiveReceiptCursor),
		ActiveOperation:          OperationID(snapshot.ActiveOperation), ActiveCycle: snapshot.ActiveCycle,
		RecoveryPaused: snapshot.RecoveryPaused, RecoveryPending: snapshot.RecoveryPending,
		ActiveOutput: activeOutputFromRuntime(snapshot.ActiveOutput),
	}
	if snapshot.InputRecovery != nil {
		status.InputRecovery = &InputRecovery{
			CommandID: CommandID(snapshot.InputRecovery.CommandID), OperationID: OperationID(snapshot.InputRecovery.OperationID),
			Cycle: snapshot.InputRecovery.Cycle, Delivery: DeliveryKind(snapshot.InputRecovery.Delivery),
		}
	}
	if snapshot.ActiveStructural != nil {
		projected, projectErr := structuralOperationFromRuntime(*snapshot.ActiveStructural)
		if projectErr != nil {
			return RuntimeStatus{}, projectErr
		}
		status.ActiveStructural = &projected
	}
	status.Queue = make([]QueuedCommand, 0, len(snapshot.Queue))
	for _, item := range snapshot.Queue {
		status.Queue = append(status.Queue, QueuedCommand{
			CommandID: CommandID(item.CommandID), OperationID: OperationID(item.OperationID),
			Delivery: DeliveryKind(item.Delivery), Message: item.Input.Text,
			MessageTruncated: item.InputTextTruncated,
		})
	}
	status.OpenToolCalls = make([]OpenToolCall, 0, len(snapshot.OpenToolCalls))
	for _, call := range snapshot.OpenToolCalls {
		status.OpenToolCalls = append(status.OpenToolCalls, OpenToolCall{
			CallID: call.CallID, Name: call.Name, OperationID: OperationID(call.OperationID), Cycle: call.Cycle,
		})
	}
	status.LastOperation = operationSummaryPointerFromRuntime(snapshot.LastOperation)
	status.RecentOperations = make([]OperationSummary, 0, len(snapshot.RecentOperations))
	for _, operation := range snapshot.RecentOperations {
		status.RecentOperations = append(status.RecentOperations, operationSummaryFromRuntime(operation))
	}
	status.LastDomainCommit = domainCommitPointerFromRuntime(snapshot.LastDomainCommit)
	status.DomainCommits = make([]DomainCommitState, 0, len(snapshot.DomainCommits))
	for _, commit := range snapshot.DomainCommits {
		status.DomainCommits = append(status.DomainCommits, domainCommitFromRuntime(commit))
	}
	return status, nil
}

func activeOutputFromRuntime(output runstate.ActiveOutputSnapshot) ActiveOutput {
	return ActiveOutput{
		OperationID: OperationID(output.OperationID), Cycle: output.Cycle,
		Content: output.Content, Thinking: output.Thinking,
		ContentTruncated: output.ContentTruncated, ThinkingTruncated: output.ThinkingTruncated,
		RehydrateRequired: output.RehydrateRequired,
	}
}

func operationSummaryFromRuntime(summary runstate.OperationSummary) OperationSummary {
	return OperationSummary{
		OperationID: OperationID(summary.OperationID), CommandID: CommandID(summary.CommandID),
		CommandFingerprint: summary.CommandFingerprint, ReceiptCursor: Cursor(summary.ReceiptCursor),
		Status: OperationStatus(summary.Status), Reason: summary.Reason, ReasonTruncated: summary.ReasonTruncated,
	}
}

func operationSummaryPointerFromRuntime(summary *runstate.OperationSummary) *OperationSummary {
	if summary == nil {
		return nil
	}
	projected := operationSummaryFromRuntime(*summary)
	return &projected
}

func domainCommitFromRuntime(commit runstate.DomainCommitState) DomainCommitState {
	return DomainCommitState{
		Identity: DomainCommitIdentity{
			CommandID: CommandID(commit.Identity.CommandID), OperationID: OperationID(commit.Identity.OperationID),
			Cycle: commit.Identity.Cycle, Stage: DomainCommitStage(commit.Identity.Stage),
		},
		Hash: commit.Hash, Revision: commit.Revision, Abandoned: commit.Abandoned, Reason: commit.Reason,
	}
}

func domainCommitPointerFromRuntime(commit *runstate.DomainCommitState) *DomainCommitState {
	if commit == nil {
		return nil
	}
	projected := domainCommitFromRuntime(*commit)
	return &projected
}

func domainCommitReconcileRequestFromRuntime(request runstate.DomainCommitReconcileRequest) (DomainCommitReconcileRequest, error) {
	binding, err := ParseRuntimeBinding(request.Binding)
	if err != nil {
		return DomainCommitReconcileRequest{}, err
	}
	projected := DomainCommitReconcileRequest{Binding: binding, Commit: domainCommitFromRuntime(request.Commit)}
	if request.Structural != nil {
		structural, projectErr := structuralOperationFromRuntime(*request.Structural)
		if projectErr != nil {
			return DomainCommitReconcileRequest{}, projectErr
		}
		projected.Structural = &structural
	}
	return projected, nil
}

func structuralOperationFromRuntime(snapshot runstate.StructuralOperationSnapshot) (StructuralOperation, error) {
	binding, err := ParseRuntimeBinding(snapshot.Binding)
	if err != nil {
		return StructuralOperation{}, fmt.Errorf("project structural Agent binding: %w", err)
	}
	return StructuralOperation{
		Binding: binding, CommandID: CommandID(snapshot.CommandID), OperationID: OperationID(snapshot.OperationID),
		Cycle: snapshot.Cycle, Kind: StructuralOperationKind(snapshot.Kind),
		Ref: contextCompactionRefFromRuntime(snapshot.Ref), ContextCursor: Cursor(snapshot.ContextCursor),
	}, nil
}

func structuralOperationToRuntime(snapshot StructuralOperation) (runstate.StructuralOperationSnapshot, error) {
	binding, err := snapshot.Binding.Ref()
	if err != nil {
		return runstate.StructuralOperationSnapshot{}, fmt.Errorf("encode structural Agent binding: %w", err)
	}
	return runstate.StructuralOperationSnapshot{
		Binding: binding, CommandID: runstate.CommandID(snapshot.CommandID), OperationID: runstate.OperationID(snapshot.OperationID),
		Cycle: snapshot.Cycle, Kind: runstate.StructuralOperationKind(snapshot.Kind),
		Ref: contextCompactionRefToRuntime(snapshot.Ref), ContextCursor: runstate.Cursor(snapshot.ContextCursor),
	}, nil
}

func contextCompactionRefFromRuntime(ref runstate.ContextCompactionRef) ContextCompactionRef {
	return ContextCompactionRef{
		SpecRef: ref.SpecRef, Source: ref.Source, Purpose: ref.Purpose, Resource: ref.Resource,
		ExpectedRevision: ref.ExpectedRevision, CompactionID: ref.CompactionID, Force: ref.Force,
		RestoreDescriptor: append(json.RawMessage(nil), ref.RestoreDescriptor...),
	}
}

func contextCompactionRefToRuntime(ref ContextCompactionRef) runstate.ContextCompactionRef {
	return runstate.ContextCompactionRef{
		SpecRef: ref.SpecRef, Source: ref.Source, Purpose: ref.Purpose, Resource: ref.Resource,
		ExpectedRevision: ref.ExpectedRevision, CompactionID: ref.CompactionID, Force: ref.Force,
		RestoreDescriptor: append(json.RawMessage(nil), ref.RestoreDescriptor...),
	}
}

func domainCommitReconcileResultToRuntime(result DomainCommitReconcileResult) runstate.DomainCommitReconcileResult {
	return runstate.DomainCommitReconcileResult{Found: result.Found, Revision: result.Revision}
}

func inputMaterializationPlanToRuntime(plan InputMaterializationPlan) runstate.InputMaterializationPlan {
	return runstate.InputMaterializationPlan{Required: plan.Required, Hash: plan.Hash}
}

func inputMaterializationPlanFromRuntime(plan runstate.InputMaterializationPlan) InputMaterializationPlan {
	return InputMaterializationPlan{Required: plan.Required, Hash: plan.Hash}
}

func inputMaterializationReceiptToRuntime(receipt InputMaterializationReceipt) runstate.InputMaterializationReceipt {
	return runstate.InputMaterializationReceipt{Revision: receipt.Revision}
}
