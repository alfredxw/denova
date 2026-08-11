package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	runstate "github.com/alfredxw/denova/agent/runtime"
	agentsession "github.com/alfredxw/denova/agent/session"
)

const engineTranscriptVersion = 2

type engineTranscript struct {
	Version           uint16     `json:"version"`
	DefinitionKey     string     `json:"definition_key"`
	RestoreKey        string     `json:"restore_key"`
	PrefixFingerprint string     `json:"prefix_fingerprint"`
	Messages          []*Message `json:"messages,omitempty"`
	HostData          *HostData  `json:"host_data,omitempty"`
	ClearRevision     uint64     `json:"clear_revision,omitempty"`
}

type definitionEngineFactory struct {
	source     Source
	persistent bool
	trace      TraceSink
}

func (factory *definitionEngineFactory) NewEngine(_ context.Context, binding runstate.BindingRef) (runstate.Engine, error) {
	if factory == nil || factory.source == nil {
		return nil, ErrDefinitionUnavailable
	}
	key, err := sessionKeyFromBinding(binding)
	if err != nil {
		return nil, err
	}
	return &definitionEngine{source: factory.source, key: key, persistent: factory.persistent, trace: factory.trace}, nil
}

type definitionEngine struct {
	source     Source
	key        SessionKey
	persistent bool
	trace      TraceSink
}

func (engine *definitionEngine) Run(
	ctx context.Context,
	request runstate.EngineRequest,
	emit runstate.EngineEventSink,
) (runstate.EngineResult, error) {
	if engine == nil || engine.source == nil {
		return runstate.EngineResult{}, ErrDefinitionUnavailable
	}
	if emit == nil {
		return runstate.EngineResult{}, errors.New("Agent Engine Event sink is required")
	}
	input, err := decodeInput(request.Snapshot.Input)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	state, err := decodeEngineTranscript(request.Snapshot.State)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	clearState, clearPresent, err := applyClearToTranscript(&state, request.Snapshot.Capabilities)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	currentCompaction, currentCompactionPresent, _, err := compactionStateFrom(request.Snapshot.Capabilities)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	currentCompaction, currentCompactionPresent = clearCompaction(
		currentCompaction, currentCompactionPresent, clearState, clearPresent,
	)
	reason := TurnReasonStart
	if request.Snapshot.Cycle > 1 {
		reason = TurnReasonSteer
	}
	prepareRequest := PrepareRequest{
		Session: SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		Run:     runViewForTurn(request.Snapshot),
		Input:   input, Reason: reason,
		DefinitionKey: state.DefinitionKey, RestoreKey: state.RestoreKey,
		HostData:   cloneHostData(input.HostData),
		Compaction: compactionStatePointer(currentCompaction, currentCompactionPresent),
	}
	prepared, err := prepareDefinitionBase(ctx, engine.source, prepareRequest)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	if engine.persistent {
		if err := validatePersistentDefinition(prepared.definition); err != nil {
			return runstate.EngineResult{}, err
		}
	}
	prepared.hostData = cloneHostData(input.HostData)
	prepared.clearRevision = state.ClearRevision
	if state.DefinitionKey != "" && state.DefinitionKey != prepared.definitionKey {
		return runstate.EngineResult{}, fmt.Errorf("%w: definition_key have=%q want=%q", ErrDefinitionMismatch, prepared.definitionKey, state.DefinitionKey)
	}
	if state.RestoreKey != "" && state.RestoreKey != prepared.restoreKey {
		return runstate.EngineResult{}, fmt.Errorf("%w: restore_key changed", ErrDefinitionMismatch)
	}
	// Canonical input reconciliation may run after the product adapter exits at
	// any instruction, including while assembling context. Persist the exact
	// Definition and HostData before opening the input commit barrier so cold
	// recovery never needs to infer product identity from process-local state.
	preparedCheckpoint, err := encodeEngineTranscript(prepared, state.Messages)
	if err != nil {
		return runstate.EngineResult{}, fmt.Errorf("encode pre-commit Agent transcript: %w", err)
	}
	if err := emit(runstate.EngineStateCheckpoint{State: preparedCheckpoint}); err != nil {
		return runstate.EngineResult{}, err
	}
	if err := engine.commitCanonicalInput(ctx, request, input, prepared.definition.Canonical, emit); err != nil {
		return runstate.EngineResult{}, err
	}
	if err := materializeDefinitionCapabilities(ctx, prepareRequest, &prepared); err != nil {
		return runstate.EngineResult{}, err
	}
	if err := engine.applyGoalPreparation(ctx, request, &prepared); err != nil {
		return runstate.EngineResult{}, err
	}
	compaction, compactionPresent, compactionChanged, err := engine.applyAutomaticCompaction(ctx, request, prepared, state.Messages, emit)
	if err != nil {
		return runstate.EngineResult{}, fmt.Errorf("automatic Compaction: %w", err)
	}
	compaction, compactionPresent = clearCompaction(compaction, compactionPresent, clearState, clearPresent)
	if compactionChanged {
		prepareRequest.Compaction = compactionStatePointer(compaction, compactionPresent)
		if err := rematerializeDefinitionContext(ctx, prepareRequest, &prepared); err != nil {
			return runstate.EngineResult{}, err
		}
	}

	effectiveTranscript := effectiveCompactionMessages(state.Messages, compaction, compactionPresent)
	resumeTools := recoverableInteractionBoundary(state.Messages, request.Snapshot.Interactions)
	var modelMessages []*Message
	var persistedUser *Message
	if resumeTools {
		modelMessages = cloneMessages(state.Messages)
	} else {
		modelMessages, persistedUser, err = assembleCycleMessages(effectiveTranscript, input.Text, prepared.fragments)
		if err != nil {
			return runstate.EngineResult{}, err
		}
	}
	emitTrace(ctx, engine.trace, TraceEvent{
		Kind: TraceCycleStarted, Session: engine.key, RunID: string(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle,
	})
	emitTrace(ctx, engine.trace, TraceEvent{
		Kind: TraceModelStarted, Session: engine.key, RunID: string(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle,
	})
	middlewares := append([]Middleware(nil), prepared.definition.Middlewares...)
	permission := effectivePermissionPolicy(prepared.definition.Permission)
	middlewares = append(middlewares, &permissionMiddleware{
		BaseMiddleware: &BaseMiddleware{}, policy: permission,
		session: SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		run:     runViewForTurn(request.Snapshot),
	})
	loop, err := NewLoop(ctx, LoopConfig{
		Name: prepared.definition.Name, Description: prepared.definition.Description,
		Instruction: prepared.definition.Instructions, Model: prepared.definition.Model,
		Tools: prepared.tools, Middlewares: middlewares,
		Retry:           prepared.definition.Execution.Retry,
		MaxIterations:   prepared.definition.Execution.MaxIterations,
		ToolParallelism: prepared.definition.Execution.ToolParallelism,
	})
	if err != nil {
		return runstate.EngineResult{}, err
	}

	runOption, cancelLoop := WithCancel()
	completion := &runCompletionControl{cancel: cancelLoop}
	control := &engineControlState{}
	interactions := newEngineInteractionClient(effectiveInteractionPolicy(prepared.definition.Interaction), request.Snapshot.Interactions, emit)
	controlDone := make(chan struct{})
	watcherDone := make(chan struct{})
	safeGo(func() {
		defer close(watcherDone)
		watchEngineControls(ctx, request.Controls, controlDone, control, cancelLoop, interactions)
	}, func(panicErr error) {
		control.fail(panicErr)
		_, _ = cancelLoop()
	})

	capabilities := newCapabilityStateClient(request.Snapshot.Capabilities, emit)
	loopCtx := contextWithCapabilityState(ctx, capabilities)
	loopCtx = contextWithInteractionClient(loopCtx, interactions)
	loopCtx = context.WithValue(loopCtx, runCompletionControlKey{}, completion)
	scope, _ := agentsession.CanonicalKey(engine.key)
	loopCtx = ContextWithInvocationIdentity(loopCtx, InvocationIdentity{
		Scope: scope, OperationID: string(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle,
	})
	iterator := loop.Run(loopCtx, &AgentInput{Messages: modelMessages, EnableStreaming: true, ResumeToolCalls: resumeTools}, runOption)
	baseTranscript := cloneMessages(state.Messages)
	if persistedUser != nil {
		baseTranscript = append(baseTranscript, persistedUser)
	}
	transcript := cloneMessages(baseTranscript)
	startedTools := make(map[string]bool)
	var final *Message
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			close(controlDone)
			<-watcherDone
			if controlErr := control.err(); controlErr != nil {
				return runstate.EngineResult{}, controlErr
			}
			switch control.kind() {
			case runstate.EngineControlPreempt:
				return engine.controlledResult(runstate.EnginePreempted, prepared, baseTranscript, emit)
			case runstate.EngineControlAbort:
				return engine.controlledResult(runstate.EngineAborted, prepared, baseTranscript, emit)
			default:
				var cancelErr *CancelError
				if completion.requestedCompletion() && errors.As(event.Err, &cancelErr) && cancelErr.Info != nil && cancelErr.Info.Mode&CancelAfterToolCalls != 0 {
					goto loopControlsStopped
				}
				return runstate.EngineResult{}, event.Err
			}
		}
		if event.Output == nil {
			continue
		}
		source := runtimeEventSource(event)
		rootEvent := rootAgentEvent(event, prepared.definition.Name)
		if execution := event.Output.ToolExecution; execution != nil {
			if err := engine.emitToolExecution(ctx, request, execution, source, prepared.definition.Canonical, startedTools, emit); err != nil {
				close(controlDone)
				<-watcherDone
				return runstate.EngineResult{}, err
			}
		}
		if variant := event.Output.MessageOutput; variant != nil {
			message, err := consumeMessageVariant(variant, source, !rootEvent, emit)
			if err != nil {
				close(controlDone)
				<-watcherDone
				return runstate.EngineResult{}, err
			}
			if message == nil {
				continue
			}
			// Nested Agent messages are live display events. The enclosing task
			// tool returns the only result that belongs in the root transcript.
			if !rootEvent {
				continue
			}
			if message.Role == Assistant && variant.ModelResponseOrdinal > 0 {
				message.AgentMeta = &AgentMessageMeta{ModelResponseOrdinal: variant.ModelResponseOrdinal}
			}
			if message.Role == Assistant {
				usage := runstate.ModelUsage{}
				finishReason := ""
				if message.ResponseMeta != nil {
					finishReason = message.ResponseMeta.FinishReason
					if value := message.ResponseMeta.Usage; value != nil {
						usage = runstate.ModelUsage{
							PromptTokens: value.PromptTokens, CachedPromptTokens: value.PromptTokenDetails.CachedTokens,
							CompletionTokens: value.CompletionTokens, ReasoningTokens: value.CompletionTokensDetails.ReasoningTokens,
							TotalTokens: value.TotalTokens,
						}
					}
				}
				if err := emit(runstate.EngineModelCompleted{
					Usage: usage, FinishReason: finishReason,
					RequestedTools: modelRequestedToolNames(message.ToolCalls), Source: source,
				}); err != nil {
					close(controlDone)
					<-watcherDone
					return runstate.EngineResult{}, err
				}
			}
			transcript = append(transcript, CloneMessage(message))
			if message.Role == Assistant {
				final = CloneMessage(message)
				if len(message.ToolCalls) > 0 {
					checkpoint, checkpointErr := encodeEngineTranscript(prepared, transcript)
					if checkpointErr != nil {
						close(controlDone)
						<-watcherDone
						return runstate.EngineResult{}, checkpointErr
					}
					if err := emit(runstate.EngineStateCheckpoint{State: checkpoint}); err != nil {
						close(controlDone)
						<-watcherDone
						return runstate.EngineResult{}, err
					}
				}
			}
		}
	}

	close(controlDone)
	<-watcherDone

loopControlsStopped:
	if controlErr := control.err(); controlErr != nil {
		return runstate.EngineResult{}, controlErr
	}
	switch control.kind() {
	case runstate.EngineControlPreempt:
		return engine.controlledResult(runstate.EnginePreempted, prepared, baseTranscript, emit)
	case runstate.EngineControlAbort:
		return engine.controlledResult(runstate.EngineAborted, prepared, baseTranscript, emit)
	}
	if completion.requestedCompletion() && final != nil && len(final.ToolCalls) != 0 {
		final = final.Clone()
		final.ToolCalls = nil
		transcript = append(transcript, final.Clone())
	}
	if final == nil || len(final.ToolCalls) != 0 {
		return runstate.EngineResult{}, errors.New("Agent Loop completed without a final assistant message")
	}
	continuation, err := engine.goalContinuation(ctx, request, prepared, capabilities)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	emitTrace(ctx, engine.trace, TraceEvent{
		Kind: TraceModelFinished, Session: engine.key, RunID: string(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle,
	})
	final, err = engine.commitCanonicalOutput(ctx, request, final, prepared.definition.Canonical, emit)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	if len(transcript) == 0 || transcript[len(transcript)-1] == nil || transcript[len(transcript)-1].Role != Assistant {
		return runstate.EngineResult{}, errors.New("Agent transcript lost the final assistant message")
	}
	transcript[len(transcript)-1] = CloneMessage(final)
	encoded, err := encodeEngineTranscript(prepared, transcript)
	if err != nil {
		return runstate.EngineResult{}, fmt.Errorf("encode Agent transcript: %w", err)
	}
	if err := emit(runstate.EngineAssistantFinal{
		Content: final.Content, Thinking: final.ReasoningContent, State: encoded, Continuation: continuation,
	}); err != nil {
		return runstate.EngineResult{}, err
	}
	return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
}

func (engine *definitionEngine) goalContinuation(
	ctx context.Context,
	request runstate.EngineRequest,
	prepared preparedDefinition,
	capabilities *capabilityStateClient,
) (*runstate.EngineContinuation, error) {
	manager := prepared.definition.Goal
	if manager == nil {
		return nil, nil
	}
	state, present, err := capabilities.goal()
	if err != nil {
		return nil, err
	}
	decision, err := manager.AfterRun(ctx, GoalAfterRunRequest{
		Session: SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		Run:     runViewForTurn(request.Snapshot),
		State:   state, Present: present, Result: Result{Status: ResultCompleted},
	})
	if err != nil {
		return nil, fmt.Errorf("evaluate Goal continuation: %w", err)
	}
	if !decision.Continue {
		return nil, nil
	}
	if strings.TrimSpace(decision.Prompt) == "" {
		return nil, errors.New("Goal continuation requires a non-empty prompt")
	}
	input := Input{Text: decision.Prompt, HostData: cloneHostData(prepared.hostData)}
	encoded, runtimeInput, err := encodeInput(input)
	if err != nil {
		return nil, fmt.Errorf("encode Goal continuation: %w", err)
	}
	runtimeInput.RestoreDescriptor = encoded
	fingerprint, err := hashCanonical(struct {
		OperationID string
		Cycle       int
		GoalID      string
		Revision    uint64
		Prompt      string
	}{string(request.Snapshot.OperationID), request.Snapshot.Cycle, state.ID, state.Revision, decision.Prompt})
	if err != nil {
		return nil, err
	}
	return &runstate.EngineContinuation{
		CommandID: runstate.CommandID("goal-continuation-" + fingerprint[:32]),
		Input:     runtimeInput, Autonomous: true,
	}, nil
}

func (engine *definitionEngine) controlledResult(
	status runstate.EngineStatus,
	prepared preparedDefinition,
	messages []*Message,
	emit runstate.EngineEventSink,
) (runstate.EngineResult, error) {
	encoded, err := encodeEngineTranscript(prepared, messages)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	if err := emit(runstate.EngineStateCheckpoint{State: encoded}); err != nil {
		return runstate.EngineResult{}, err
	}
	return runstate.EngineResult{Status: status}, nil
}

func encodeEngineTranscript(prepared preparedDefinition, messages []*Message) (json.RawMessage, error) {
	encoded, err := json.Marshal(engineTranscript{
		Version: engineTranscriptVersion, DefinitionKey: prepared.definitionKey,
		RestoreKey: prepared.restoreKey, PrefixFingerprint: prepared.prefixFingerprint,
		Messages:      cloneMessages(messages),
		HostData:      cloneHostData(prepared.hostData),
		ClearRevision: prepared.clearRevision,
	})
	if err != nil {
		return nil, fmt.Errorf("encode Agent transcript: %w", err)
	}
	return encoded, nil
}

func decodeEngineTranscript(encoded json.RawMessage) (engineTranscript, error) {
	if len(encoded) == 0 || string(encoded) == "null" {
		return engineTranscript{Version: engineTranscriptVersion}, nil
	}
	var state engineTranscript
	if err := json.Unmarshal(encoded, &state); err != nil {
		return engineTranscript{}, fmt.Errorf("decode Agent transcript: %w", err)
	}
	if state.Version != engineTranscriptVersion {
		return engineTranscript{}, fmt.Errorf("unsupported Agent transcript version %d", state.Version)
	}
	state.Messages = cloneMessages(state.Messages)
	state.HostData = cloneHostData(state.HostData)
	return state, nil
}

func recoverableInteractionBoundary(messages []*Message, interactions []runstate.InteractionSnapshot) bool {
	if len(interactions) == 0 || len(messages) == 0 {
		return false
	}
	resolved := false
	for _, interaction := range interactions {
		if interaction.Resolved {
			resolved = true
			break
		}
	}
	last := messages[len(messages)-1]
	return resolved && last != nil && last.Role == Assistant && len(last.ToolCalls) > 0 &&
		last.AgentMeta != nil && last.AgentMeta.ModelResponseOrdinal > 0
}

func validatePersistentDefinition(definition Definition) error {
	if err := definition.ModelIdentity.validate("Model"); err != nil {
		return fmt.Errorf("durable Agent: %w", err)
	}
	if definition.Tools != nil {
		if err := definition.Tools.Identity().validate("Toolset"); err != nil {
			return fmt.Errorf("durable Agent: %w", err)
		}
	}
	if definition.Context != nil {
		if err := definition.Context.Identity().validate("Context"); err != nil {
			return fmt.Errorf("durable Agent: %w", err)
		}
	}
	if definition.Goal != nil {
		if err := definition.Goal.Identity().validate("Goal"); err != nil {
			return fmt.Errorf("durable Agent: %w", err)
		}
	}
	if definition.Compaction != nil {
		if err := definition.Compaction.Identity().validate("Compaction"); err != nil {
			return fmt.Errorf("durable Agent: %w", err)
		}
	}
	if err := effectivePermissionPolicy(definition.Permission).Identity().validate("Permission"); err != nil {
		return fmt.Errorf("durable Agent: %w", err)
	}
	if err := effectiveInteractionPolicy(definition.Interaction).Identity().validate("Interaction"); err != nil {
		return fmt.Errorf("durable Agent: %w", err)
	}
	if definition.Canonical != nil {
		if err := definition.Canonical.Identity().validate("Canonical"); err != nil {
			return fmt.Errorf("durable Agent: %w", err)
		}
	}
	if definition.Execution.Retry != nil {
		if err := definition.Execution.RetryIdentity.validate("Retry"); err != nil {
			return fmt.Errorf("durable Agent: %w", err)
		}
	}
	for index, middleware := range definition.Middlewares {
		identified, ok := middleware.(IdentifiedMiddleware)
		if !ok {
			return fmt.Errorf("durable Agent: Middleware %d has no stable capability identity", index)
		}
		if err := identified.Identity().validate(fmt.Sprintf("Middleware %d", index)); err != nil {
			return fmt.Errorf("durable Agent: %w", err)
		}
	}
	return nil
}
