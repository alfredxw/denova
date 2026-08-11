package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func (engine *definitionEngine) commitCanonicalInput(
	ctx context.Context,
	request runstate.EngineRequest,
	input Input,
	adapter CanonicalAdapter,
	emit runstate.EngineEventSink,
) error {
	if adapter == nil {
		return nil
	}
	identity := canonicalCommitIdentity(engine.key, request.Snapshot, CommitInput)
	hash, err := hashCanonical(struct {
		Version uint16
		Input   Input
	}{Version: 1, Input: input})
	if err != nil {
		return err
	}
	runtimeIdentity := runtimeCommitIdentity(identity)
	if err := emit(runstate.EngineDomainCommitIntent{Identity: runtimeIdentity, Hash: hash}); err != nil {
		return err
	}
	receipt, err := adapter.MaterializeInput(ctx, InputCommitRequest{Identity: identity, Hash: hash, Input: input})
	if err != nil {
		return fmt.Errorf("materialize canonical Agent input: %w", err)
	}
	receipt.Revision = strings.TrimSpace(receipt.Revision)
	if receipt.Revision == "" {
		return errors.New("materialize canonical Agent input returned an empty revision")
	}
	return emit(runstate.EngineDomainCommitReceipt{Identity: runtimeIdentity, Hash: hash, Revision: receipt.Revision})
}

func (engine *definitionEngine) commitCanonicalOutput(
	ctx context.Context,
	request runstate.EngineRequest,
	message *Message,
	adapter CanonicalAdapter,
	emit runstate.EngineEventSink,
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
	runtimeIdentity := runtimeCommitIdentity(identity)
	if err := emit(runstate.EngineDomainCommitIntent{Identity: runtimeIdentity, Hash: hash}); err != nil {
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
	if err := emit(runstate.EngineDomainCommitReceipt{Identity: runtimeIdentity, Hash: hash, Revision: receipt.Revision}); err != nil {
		return nil, err
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

func runtimeCommitIdentity(identity CommitIdentity) runstate.DomainCommitIdentity {
	stage := runstate.DomainCommitInput
	if identity.Stage == CommitOutput {
		stage = runstate.DomainCommitOutput
	}
	return runstate.DomainCommitIdentity{
		CommandID: runstate.CommandID(identity.CommandID), OperationID: runstate.OperationID(identity.RunID),
		Cycle: identity.Cycle, Stage: stage,
	}
}

func publicCommitIdentity(key SessionKey, identity runstate.DomainCommitIdentity) (CommitIdentity, error) {
	var stage CommitStage
	switch identity.Stage {
	case runstate.DomainCommitInput:
		stage = CommitInput
	case runstate.DomainCommitOutput:
		stage = CommitOutput
	default:
		return CommitIdentity{}, fmt.Errorf("unsupported canonical commit stage %q", identity.Stage)
	}
	return CommitIdentity{
		Session: key, CommandID: string(identity.CommandID), RunID: string(identity.OperationID),
		Cycle: identity.Cycle, Stage: stage,
	}, nil
}

func (engine *definitionEngine) recoveryCanonical(
	ctx context.Context,
	commandID string,
	runID string,
	cycle int,
	encoded json.RawMessage,
) (CanonicalAdapter, error) {
	transcript, err := decodeEngineTranscript(encoded)
	if err != nil {
		return nil, err
	}
	prepared, err := prepareDefinitionBase(ctx, engine.source, PrepareRequest{
		Session: SessionView{Key: engine.key}, Run: RunView{ID: runID, CommandID: commandID, Cycle: cycle}, Reason: TurnReasonRecovery,
		DefinitionKey: transcript.DefinitionKey, RestoreKey: transcript.RestoreKey,
		HostData: cloneHostData(transcript.HostData),
	})
	if err != nil {
		return nil, err
	}
	if engine.persistent {
		if err := validatePersistentDefinition(prepared.definition); err != nil {
			return nil, err
		}
	}
	if prepared.definition.Canonical == nil {
		return nil, ErrCapabilityUnsupported
	}
	if transcript.DefinitionKey != "" && prepared.definitionKey != transcript.DefinitionKey ||
		transcript.RestoreKey != "" && prepared.restoreKey != transcript.RestoreKey {
		return nil, ErrDefinitionMismatch
	}
	return prepared.definition.Canonical, nil
}

func (engine *definitionEngine) ReconcileDomainCommit(
	ctx context.Context,
	request runstate.DomainCommitReconcileRequest,
) (runstate.DomainCommitReconcileResult, error) {
	adapter, err := engine.recoveryCanonical(ctx, string(request.Commit.Identity.CommandID), string(request.Commit.Identity.OperationID), request.Commit.Identity.Cycle, request.State)
	if err != nil {
		return runstate.DomainCommitReconcileResult{}, err
	}
	identity, err := publicCommitIdentity(engine.key, request.Commit.Identity)
	if err != nil {
		return runstate.DomainCommitReconcileResult{}, err
	}
	result, err := adapter.Reconcile(ctx, ReconcileRequest{Identity: identity, Hash: request.Commit.Hash})
	if err != nil {
		return runstate.DomainCommitReconcileResult{}, err
	}
	return runstate.DomainCommitReconcileResult{Found: result.Found, Revision: result.Revision}, nil
}

func (engine *definitionEngine) ReconcileHostEffect(ctx context.Context, recovered runstate.HostEffectReconcileRequest) error {
	effect := recovered.Effect
	adapter, err := engine.recoveryCanonical(ctx, "", string(effect.OperationID), effect.Cycle, recovered.State)
	if err != nil {
		return err
	}
	var value Effect
	if err := json.Unmarshal(effect.Payload, &value); err != nil {
		return fmt.Errorf("decode canonical Tool effect %q: %w", effect.ID, err)
	}
	request := EffectRequest{
		ID: string(effect.ID),
		Identity: CommitIdentity{
			Session: engine.key, RunID: string(effect.OperationID), Cycle: effect.Cycle, Stage: CommitOutput,
		},
		CallID: effect.CallID, Index: effect.Index, Effect: value,
	}
	results, err := adapter.ApplyEffects(ctx, []EffectRequest{request})
	if err != nil {
		return err
	}
	if len(results) != 1 || results[0].ID != request.ID {
		return errors.New("Canonical Adapter returned an incomplete Tool effect result")
	}
	if results[0].Error != "" {
		return errors.New(results[0].Error)
	}
	if strings.TrimSpace(results[0].Revision) == "" {
		return errors.New("Canonical Adapter Tool effect result has no revision")
	}
	return nil
}

func (engine *definitionEngine) ResolveInteraction(
	ctx context.Context,
	request runstate.InteractionResolveRequest,
) (json.RawMessage, error) {
	input, err := decodeInput(request.Snapshot.Input)
	if err != nil {
		return nil, err
	}
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
		Input:   input, Reason: TurnReasonRecovery,
		DefinitionKey: transcript.DefinitionKey, RestoreKey: transcript.RestoreKey,
		HostData:   cloneHostData(input.HostData),
		Compaction: compaction,
	})
	if err != nil {
		return nil, err
	}
	if engine.persistent {
		if err := validatePersistentDefinition(prepared.definition); err != nil {
			return nil, err
		}
	}
	if transcript.DefinitionKey != "" && transcript.DefinitionKey != prepared.definitionKey {
		return nil, fmt.Errorf("%w: interaction Definition changed", ErrDefinitionMismatch)
	}
	if transcript.RestoreKey != "" && transcript.RestoreKey != prepared.restoreKey {
		return nil, fmt.Errorf("%w: interaction restore identity changed", ErrDefinitionMismatch)
	}
	if err := materializeDefinitionCapabilities(ctx, PrepareRequest{
		Session: SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		Run:     runViewForTurn(request.Snapshot), Input: input, Reason: TurnReasonRecovery,
		DefinitionKey: transcript.DefinitionKey, RestoreKey: transcript.RestoreKey,
		HostData:   cloneHostData(input.HostData),
		Compaction: compaction,
	}, &prepared); err != nil {
		return nil, err
	}
	var interactionRequest InteractionRequest
	if err := json.Unmarshal(request.Interaction.Request, &interactionRequest); err != nil {
		return nil, fmt.Errorf("decode durable Interaction request: %w", err)
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
			return nil, errors.New("durable Permission Interaction has no presentation")
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
		if resolved.Allowed {
			if resolution.Permission != PermissionAllowOnce && resolution.Permission != PermissionRemember {
				return nil, errors.New("Permission Policy allowed a denied resolution")
			}
		} else {
			resolution.Permission = PermissionDeny
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
	transcript, err := decodeEngineTranscript(request.Snapshot.State)
	if err != nil {
		return nil, err
	}
	prepared, err := prepareDefinitionBase(ctx, engine.source, PrepareRequest{
		Session: SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		Run:     runViewForTurn(request.Snapshot),
		Input:   input, Reason: TurnReasonStart,
		DefinitionKey: transcript.DefinitionKey, RestoreKey: transcript.RestoreKey,
		HostData: cloneHostData(input.HostData),
	})
	if err != nil {
		return nil, err
	}
	if engine.persistent {
		if err := validatePersistentDefinition(prepared.definition); err != nil {
			return nil, err
		}
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
	next, err := prepared.definition.Goal.Apply(ctx, GoalApplyRequest{
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
	return []runstate.EngineCapabilityState{{
		Capability: goalCapability, Expected: describeCapabilityState(raw), State: encoded,
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

func watchEngineControls(
	ctx context.Context,
	controls <-chan runstate.EngineControl,
	done <-chan struct{},
	state *engineControlState,
	cancel AgentCancelFunc,
	interactions *engineInteractionClient,
) {
	for {
		select {
		case control, ok := <-controls:
			if !ok {
				return
			}
			switch control.Kind {
			case runstate.EngineControlPreempt:
				state.set(control.Kind)
				_, _ = cancel(WithAgentCancelMode(CancelAfterChatModel | CancelAfterToolCalls))
			case runstate.EngineControlAbort:
				state.set(control.Kind)
				_, _ = cancel(WithAgentCancelMode(CancelImmediate))
			case runstate.EngineControlInteractionResolved:
				if interactions != nil {
					interactions.deliver(control.InteractionID, control.Response)
				}
			}
		case <-done:
			return
		case <-ctx.Done():
			return
		}
	}
}

var _ runstate.EngineFactory = (*definitionEngineFactory)(nil)
var _ runstate.Engine = (*definitionEngine)(nil)
var _ runstate.EngineDomainCommitReconciler = (*definitionEngine)(nil)
var _ runstate.EngineHostEffectReconciler = (*definitionEngine)(nil)
var _ runstate.EngineInteractionResolver = (*definitionEngine)(nil)
var _ runstate.EngineAdmissionPreparer = (*definitionEngine)(nil)
