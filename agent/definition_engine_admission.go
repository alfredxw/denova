package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
)

func (engine *definitionEngine) canonicalInputRequest(
	request runstate.TurnSnapshot,
) (PrepareRequest, Input, error) {
	input, err := decodeInput(request.Input)
	if err != nil {
		return PrepareRequest{}, Input{}, err
	}
	input.IdempotencyKey = string(request.CommandID)
	reason, err := turnReasonForSnapshot(request)
	if err != nil {
		return PrepareRequest{}, Input{}, err
	}
	return PrepareRequest{
		Session:  SessionView{Key: engine.key, Revision: uint64(request.ContextCursor)},
		Run:      runViewForTurn(request),
		Input:    input,
		Reason:   reason,
		HostData: cloneHostData(input.HostData),
	}, input, nil
}

// PlanInputMaterialization resolves the product canonical-input adapter before
// the Definition is fully prepared.
func (engine *definitionEngine) PlanInputMaterialization(
	ctx context.Context,
	request runstate.InputMaterializationRequest,
) (runstate.InputMaterializationPlan, error) {
	prepare, input, err := engine.canonicalInputRequest(request.Snapshot)
	if err != nil {
		return runstate.InputMaterializationPlan{}, err
	}
	adapter, err := engine.source.CanonicalInput(ctx, prepare)
	if err != nil {
		return runstate.InputMaterializationPlan{}, fmt.Errorf("resolve canonical Agent input: %w", err)
	}
	if adapter == nil {
		return runstate.InputMaterializationPlan{}, nil
	}
	hash, err := canonicalInputHash(input, adapter.Identity())
	if err != nil {
		return runstate.InputMaterializationPlan{}, err
	}
	return runstate.InputMaterializationPlan{Required: true, Hash: hash}, nil
}

func (engine *definitionEngine) MaterializeInput(
	ctx context.Context,
	request runstate.InputMaterializationRequest,
	plan runstate.InputMaterializationPlan,
) (runstate.InputMaterializationReceipt, error) {
	prepare, input, err := engine.canonicalInputRequest(request.Snapshot)
	if err != nil {
		return runstate.InputMaterializationReceipt{}, err
	}
	adapter, err := engine.source.CanonicalInput(ctx, prepare)
	if err != nil {
		return runstate.InputMaterializationReceipt{}, fmt.Errorf("resolve canonical Agent input: %w", err)
	}
	if adapter == nil || !plan.Required {
		return runstate.InputMaterializationReceipt{}, errors.New("canonical Agent input materializer is unavailable")
	}
	want, err := canonicalInputHash(input, adapter.Identity())
	if err != nil {
		return runstate.InputMaterializationReceipt{}, err
	}
	if plan.Hash != want {
		return runstate.InputMaterializationReceipt{}, fmt.Errorf("%w: canonical Agent input changed after planning", runstate.ErrDomainCommitRejected)
	}
	identity := canonicalCommitIdentity(engine.key, request.Snapshot, CommitInput)
	receipt, err := adapter.MaterializeInput(ctx, InputCommitRequest{Identity: identity, Hash: want, Input: input})
	if err != nil {
		return runstate.InputMaterializationReceipt{}, fmt.Errorf("materialize canonical Agent input: %w", err)
	}
	revision := strings.TrimSpace(receipt.Revision)
	if revision == "" {
		return runstate.InputMaterializationReceipt{}, errors.New("materialize canonical Agent input returned an empty revision")
	}
	return runstate.InputMaterializationReceipt{Revision: revision}, nil
}

func canonicalInputHash(input Input, adapter CapabilityIdentity) (string, error) {
	if err := adapter.validate("Canonical"); err != nil {
		return "", err
	}
	return hashCanonical(struct {
		Version uint16
		Adapter CapabilityIdentity
		Input   Input
	}{Version: 2, Adapter: adapter, Input: input})
}

// verifyCanonicalInputCommit proves that admission and the fully prepared
// Definition describe the same canonical boundary.
func (engine *definitionEngine) verifyCanonicalInputCommit(
	snapshot runstate.TurnSnapshot,
	input Input,
	adapter CanonicalAdapter,
) error {
	commit := snapshot.InputCommit
	if adapter == nil {
		if commit != nil {
			return fmt.Errorf("%w: canonical Agent input was committed but the prepared Definition has no Canonical Adapter", ErrDefinitionMismatch)
		}
		return nil
	}
	if commit == nil {
		return fmt.Errorf("%w: prepared Definition requires a canonical Agent input receipt", runstate.ErrDomainCommitRejected)
	}
	wantIdentity := engineCommitIdentity(canonicalCommitIdentity(engine.key, snapshot, CommitInput))
	if commit.Identity != wantIdentity {
		return fmt.Errorf("%w: canonical Agent input receipt identity does not match the active cycle", runstate.ErrDomainCommitRejected)
	}
	wantHash, err := canonicalInputHash(input, adapter.Identity())
	if err != nil {
		return err
	}
	if commit.Hash != wantHash || strings.TrimSpace(commit.Revision) == "" {
		return fmt.Errorf("%w: canonical Agent input receipt does not match the prepared Definition", ErrDefinitionMismatch)
	}
	return nil
}

func (engine *definitionEngine) commitCanonicalOutput(
	ctx context.Context,
	request runstate.EngineRequest,
	message *Message,
	adapter CanonicalAdapter,
) (*Message, error) {
	if adapter == nil {
		return CloneMessage(message), nil
	}
	identity := canonicalCommitIdentity(engine.key, request.Snapshot, CommitOutput)
	hash, err := hashCanonical(struct {
		Version uint16
		Message Message
	}{Version: 1, Message: *CloneMessage(message)})
	if err != nil {
		return nil, err
	}
	receipt, err := adapter.CommitOutput(ctx, OutputCommitRequest{
		Identity: identity, Hash: hash, Message: *CloneMessage(message),
	})
	if err != nil {
		return nil, fmt.Errorf("commit canonical Agent output: %w", err)
	}
	receipt.Revision = strings.TrimSpace(receipt.Revision)
	if receipt.Revision == "" {
		return nil, errors.New("commit canonical Agent output returned an empty revision")
	}
	effective := CloneMessage(message)
	if receipt.Transcript != nil {
		effective.Content = receipt.Transcript.Content
		effective.ReasoningContent = receipt.Transcript.Thinking
	}
	return effective, nil
}

func canonicalCommitIdentity(key SessionKey, snapshot runstate.TurnSnapshot, stage CommitStage) CommitIdentity {
	return CommitIdentity{
		Session: key, CommandID: string(snapshot.CommandID), RunID: string(snapshot.OperationID),
		Cycle: snapshot.Cycle, Stage: stage,
	}
}

func engineCommitIdentity(identity CommitIdentity) runstate.DomainCommitIdentity {
	stage := runstate.DomainCommitInput
	if identity.Stage == CommitOutput {
		stage = runstate.DomainCommitOutput
	}
	return runstate.DomainCommitIdentity{
		CommandID: runstate.CommandID(identity.CommandID), OperationID: runstate.OperationID(identity.RunID),
		Cycle: identity.Cycle, Stage: stage,
	}
}

func (engine *definitionEngine) ResolveInteraction(
	ctx context.Context,
	request runstate.InteractionResolveRequest,
) (json.RawMessage, error) {
	input, err := decodeInput(request.Snapshot.Input)
	if err != nil {
		return nil, err
	}
	input.IdempotencyKey = string(request.Snapshot.CommandID)
	transcript, err := decodeEngineTranscript(request.Snapshot.State)
	if err != nil {
		return nil, err
	}
	currentCompaction, currentCompactionPresent, _, err := compactionStateFrom(request.Snapshot.Capabilities)
	if err != nil {
		return nil, err
	}
	clearState, clearPresent, err := clearStateFrom(request.Snapshot.Capabilities)
	if err != nil {
		return nil, err
	}
	currentCompaction, currentCompactionPresent = clearCompaction(
		currentCompaction, currentCompactionPresent, clearState, clearPresent,
	)
	compaction := compactionStatePointer(currentCompaction, currentCompactionPresent)
	prepared, err := prepareDefinitionBase(ctx, engine.source, PrepareRequest{
		Session: SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		Run:     runViewForTurn(request.Snapshot),
		Input:   input, Reason: TurnReasonInteraction,
		DefinitionKey: transcript.DefinitionKey, BehaviorKey: transcript.BehaviorKey,
		HostData:   cloneHostData(input.HostData),
		Compaction: compaction,
	})
	if err != nil {
		return nil, err
	}
	if transcript.DefinitionKey != "" && transcript.DefinitionKey != prepared.definitionKey {
		return nil, fmt.Errorf("%w: interaction Definition changed", ErrDefinitionMismatch)
	}
	if transcript.BehaviorKey != "" && transcript.BehaviorKey != prepared.behaviorKey {
		return nil, fmt.Errorf("%w: interaction behavior identity changed", ErrDefinitionMismatch)
	}
	prepareRequest := PrepareRequest{
		Session: SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		Run:     runViewForTurn(request.Snapshot), Input: input, Reason: TurnReasonInteraction,
		DefinitionKey: transcript.DefinitionKey, BehaviorKey: transcript.BehaviorKey,
		HostData:   cloneHostData(input.HostData),
		Compaction: compaction,
	}
	if err := materializeDefinitionCapabilities(ctx, prepareRequest, &prepared); err != nil {
		return nil, err
	}
	if err := engine.applyGoalPreparation(ctx, runstate.EngineRequest{Snapshot: request.Snapshot}, &prepared); err != nil {
		return nil, err
	}
	materialized, err := materializedDefinitionFingerprint(prepared)
	if err != nil {
		return nil, err
	}
	if transcript.PreparationStage == enginePreparationMaterialized &&
		transcript.MaterializedFingerprint != materialized {
		return nil, fmt.Errorf(
			"%w: interaction materialized Definition changed (previous=%s current=%s)",
			ErrDefinitionMismatch, transcript.MaterializedFingerprint, materialized,
		)
	}
	var interactionRequest InteractionRequest
	if err := json.Unmarshal(request.Interaction.Request, &interactionRequest); err != nil {
		return nil, fmt.Errorf("decode Interaction request: %w", err)
	}
	if interactionRequest.ID != request.Interaction.ID {
		return nil, ErrInteractionStale
	}
	var response InteractionResponse
	if err := json.Unmarshal(request.Response, &response); err != nil {
		return nil, fmt.Errorf("decode Interaction response: %w", err)
	}
	policy := effectiveInteractionPolicy(prepared.definition.Interaction)
	resolution, err := policy.Resolve(ctx, interactionRequest, response)
	if err != nil {
		return nil, err
	}
	if interactionRequest.Kind == InteractionPermission {
		presentation := interactionRequest.Permission
		if presentation == nil {
			return nil, errors.New("Permission Interaction has no presentation")
		}
		if resolution.Cancelled {
			// Cancellation is never authorization and has no policy-owned work to
			// persist. In particular, do not call a custom policy with an empty
			// choice: a cancelled UI response must not accidentally enter its
			// remember path.
			resolution.Permission = PermissionDeny
			encoded, encodeErr := json.Marshal(resolution)
			if encodeErr != nil {
				return nil, fmt.Errorf("encode cancelled Permission resolution: %w", encodeErr)
			}
			return encoded, nil
		}
		if resolution.Permission == PermissionRemember && !presentation.CanRemember {
			return nil, errors.New("Permission Interaction cannot remember this request")
		}
		var descriptor ToolDescriptor
		found := false
		for _, tool := range prepared.tools {
			info, infoErr := tool.Tool.Info(ctx)
			if infoErr != nil {
				return nil, infoErr
			}
			if info != nil && info.Name == presentation.Tool {
				descriptor, found = tool.Descriptor, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("%w: permission tool %q is unavailable", ErrDefinitionMismatch, presentation.Tool)
		}
		resolved, resolveErr := effectivePermissionPolicy(prepared.definition.Permission).Resolve(ctx, PermissionResolveRequest{
			Request: PermissionRequest{
				Session: SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
				Run:     runViewForTurn(request.Snapshot),
				CallID:  presentation.CallID, Tool: presentation.Tool,
				Arguments: append(json.RawMessage(nil), presentation.Arguments...), Descriptor: descriptor,
			},
			Resolution: resolution,
		})
		if resolveErr != nil {
			return nil, resolveErr
		}
		switch resolution.Permission {
		case PermissionAllowOnce:
			if !resolved.Allowed || resolved.Remembered {
				return nil, errors.New("Permission Policy returned an inconsistent allow-once decision")
			}
		case PermissionRemember:
			if !resolved.Allowed || !resolved.Remembered {
				return nil, errors.New("Permission Policy returned success before the remembered rule was durable")
			}
		case PermissionDeny:
			if resolved.Allowed || resolved.Remembered {
				return nil, errors.New("Permission Policy returned an inconsistent deny decision")
			}
		default:
			return nil, errors.New("Permission Policy received an invalid resolution")
		}
	}
	encoded, err := json.Marshal(resolution)
	if err != nil {
		return nil, fmt.Errorf("encode Interaction resolution: %w", err)
	}
	return encoded, nil
}

func (engine *definitionEngine) PrepareAdmission(
	ctx context.Context,
	request runstate.TurnAdmissionRequest,
) ([]runstate.EngineCapabilityState, error) {
	input, err := decodeInput(request.Snapshot.Input)
	if err != nil || input.Goal == nil {
		return nil, err
	}
	input.IdempotencyKey = string(request.Snapshot.CommandID)
	transcript, err := decodeEngineTranscript(request.Snapshot.State)
	if err != nil {
		return nil, err
	}
	reason, err := turnReasonForSnapshot(request.Snapshot)
	if err != nil {
		return nil, err
	}
	prepared, err := prepareDefinitionBase(ctx, engine.source, PrepareRequest{
		Session: SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		Run:     runViewForTurn(request.Snapshot),
		Input:   input, Reason: reason,
		DefinitionKey: transcript.DefinitionKey, BehaviorKey: transcript.BehaviorKey,
		HostData: cloneHostData(input.HostData),
	})
	if err != nil {
		return nil, err
	}
	if prepared.definition.Goal == nil {
		return nil, ErrCapabilityUnsupported
	}
	raw, present := request.Snapshot.Capabilities[goalCapability]
	var current GoalState
	if present {
		current, err = decodeGoalState(raw)
		if err != nil {
			return nil, err
		}
	}
	next, err := applyGoalMutation(ctx, prepared.definition.Goal, GoalApplyRequest{
		Session: SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		Run:     runViewForTurn(request.Snapshot),
		Current: current, Present: present, Mutation: *input.Goal,
	})
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	if present && string(encoded) == string(raw) {
		return nil, nil
	}
	return []runstate.EngineCapabilityState{{
		Capability: goalCapability, State: encoded,
	}}, nil
}

type engineControlState struct {
	mu      sync.RWMutex
	control runstate.EngineControlKind
	failure error
}

func (state *engineControlState) set(kind runstate.EngineControlKind) {
	state.mu.Lock()
	if kind == runstate.EngineControlAbort || state.control == "" {
		state.control = kind
	}
	state.mu.Unlock()
}

func (state *engineControlState) kind() runstate.EngineControlKind {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.control
}

func (state *engineControlState) fail(err error) {
	state.mu.Lock()
	state.failure = err
	state.mu.Unlock()
}

func (state *engineControlState) err() error {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.failure
}

var _ runstate.EngineFactory = (*definitionEngineFactory)(nil)
var _ runstate.Engine = (*definitionEngine)(nil)
var _ runstate.EngineInteractionResolver = (*definitionEngine)(nil)
var _ runstate.EngineAdmissionPreparer = (*definitionEngine)(nil)
var _ runstate.EngineInputMaterializer = (*definitionEngine)(nil)
