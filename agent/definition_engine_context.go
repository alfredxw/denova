package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
	agentsession "github.com/alfredxw/denova/agent/session"
)

func (engine *definitionEngine) applyGoalPreparation(
	ctx context.Context,
	request runstate.EngineRequest,
	prepared *preparedDefinition,
) error {
	if prepared == nil || prepared.definition.Goal == nil {
		return nil
	}
	raw, present := request.Snapshot.Capabilities[goalCapability]
	var state GoalState
	var err error
	if present {
		state, err = decodeGoalState(raw)
		if err != nil {
			return err
		}
	}
	return applyPreparedGoal(
		ctx,
		prepared,
		SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		runViewForTurn(request.Snapshot),
		state,
		present,
	)
}

// applyPreparedGoal is the single model-facing Goal assembly seam shared by
// real runs and read-only Session inspection. Mutation and CAS remain
// in the lifecycle; this helper only materializes tools and context from an
// already selected Goal state.
func applyPreparedGoal(
	ctx context.Context,
	prepared *preparedDefinition,
	session SessionView,
	run RunView,
	state GoalState,
	present bool,
) error {
	if prepared == nil || prepared.definition.Goal == nil {
		return nil
	}
	goalPreparation, err := prepared.definition.Goal.Prepare(ctx, GoalPrepareRequest{
		Session: session, Run: run, State: state, Present: present,
	})
	if err != nil {
		return fmt.Errorf("prepare Goal capability: %w", err)
	}
	definitions := append([]ToolDefinition(nil), prepared.tools...)
	definitions = append(definitions, goalPreparation.Tools...)
	if goalPreparation.StandardTool {
		tool, err := standardGoalTool(prepared.definition.Goal, session, run)
		if err != nil {
			return err
		}
		definitions = append(definitions, tool)
	}
	registry, err := NewRegistry(ctx, definitions...)
	if err != nil {
		return fmt.Errorf("prepare Goal tools: %w", err)
	}
	prepared.tools = registry.Definitions()
	prepared.toolSnapshots = registry.Snapshots()
	prepared.goalFragments = append([]ContextFragment(nil), goalPreparation.Context...)
	prepared.fragments = append(prepared.fragments, prepared.goalFragments...)
	if err := validateContextFragments(prepared.fragments); err != nil {
		return err
	}
	return updatePreparedPrefixFingerprint(prepared)
}

func sessionKeyFromBinding(binding runstate.BindingRef) (SessionKey, error) {
	return agentsession.NormalizeKey(SessionKey{
		Namespace: binding.Kind, ID: binding.Key, Attributes: cloneStringMap(binding.Labels),
	})
}

func runViewForTurn(snapshot runstate.TurnSnapshot) RunView {
	return RunView{
		ID: string(snapshot.OperationID), CommandID: string(snapshot.CommandID), Cycle: snapshot.Cycle,
		StartedAt: snapshot.StartedAt,
		Delivery:  publicTurnDelivery(snapshot.Delivery), Autonomous: snapshot.Autonomous,
	}
}

func publicTurnDelivery(delivery runstate.DeliveryKind) TurnDelivery {
	switch delivery {
	case runstate.DeliveryStart:
		return TurnDeliveryStart
	case runstate.DeliverySteer:
		return TurnDeliverySteer
	case runstate.DeliveryFollowUp:
		return TurnDeliveryFollowUp
	case runstate.DeliveryNextTurn:
		return TurnDeliveryNextTurn
	default:
		return ""
	}
}

func turnReasonForSnapshot(snapshot runstate.TurnSnapshot) (TurnReason, error) {
	switch snapshot.Delivery {
	case runstate.DeliveryStart:
		return TurnReasonStart, nil
	case runstate.DeliverySteer:
		return TurnReasonSteer, nil
	case runstate.DeliveryFollowUp:
		return TurnReasonFollowUp, nil
	case runstate.DeliveryNextTurn:
		return TurnReasonNextTurn, nil
	default:
		return "", fmt.Errorf("unsupported Agent turn delivery %q", snapshot.Delivery)
	}
}

func runViewForStructural(snapshot runstate.StructuralOperationSnapshot) RunView {
	return RunView{
		ID: string(snapshot.OperationID), CommandID: string(snapshot.CommandID), Cycle: snapshot.Cycle,
	}
}

func assembleCycleMessages(
	transcript []*Message,
	userText string,
	fragments []ContextFragment,
) ([]*Message, *Message, error) {
	messages := make([]*Message, 0, len(transcript)+len(fragments)+1)
	messages = append(messages, leadingContextMessages(fragments)...)
	var prefixes []string
	var finalUserMessage string
	hasFinalUserMessage := false
	for _, fragment := range fragments {
		rendered := renderContextFragment(fragment)
		switch fragment.Placement {
		case ContextLeadingMessage:
		case ContextFinalUserPrefix:
			prefixes = append(prefixes, rendered)
		case ContextFinalUserMessage:
			finalUserMessage = rendered
			hasFinalUserMessage = true
		case ContextAuditOnly:
		}
	}
	messages = append(messages, cloneMessages(transcript)...)
	modelUserText := strings.TrimSpace(userText)
	if hasFinalUserMessage {
		modelUserText = finalUserMessage
	} else if len(prefixes) > 0 {
		modelUserText = strings.Join(prefixes, "\n\n---\n\n") + "\n\n---\n\n# User request\n\n" + modelUserText
	}
	user := UserMessage(modelUserText)
	messages = append(messages, user)
	return messages, CloneMessage(user), nil
}

// leadingContextMessages is the single assembly rule for lifecycle-owned
// stable fragments. Normal turns, retries, and structural Compaction snapshots
// must preserve the exact same role and bytes for provider cache identity.
func leadingContextMessages(fragments []ContextFragment) []*Message {
	messages := make([]*Message, 0, len(fragments))
	for _, fragment := range fragments {
		if fragment.Placement != ContextLeadingMessage {
			continue
		}
		rendered := renderContextFragment(fragment)
		switch effectiveContextRole(fragment) {
		case User:
			messages = append(messages, UserMessage(rendered))
		default:
			messages = append(messages, SystemMessage(rendered))
		}
	}
	return messages
}

func newPreparedDefinitionLoop(
	ctx context.Context,
	prepared preparedDefinition,
	middlewares []Middleware,
	permission *permissionMiddleware,
	gate modelCallGate,
) (*modelToolLoop, error) {
	return newModelToolLoop(ctx, loopConfig{
		Name: prepared.definition.Name, Description: prepared.definition.Description,
		Instruction: prepared.definition.Instructions, Model: prepared.definition.Model,
		Tools: prepared.tools, Middlewares: middlewares,
		ResultProcessor: prepared.definition.ResultProcessor, Artifacts: prepared.definition.Artifacts,
		Retry:           prepared.definition.Execution.Retry,
		MaxIterations:   prepared.definition.Execution.MaxIterations,
		IdleTimeout:     prepared.definition.Execution.IdleTimeout,
		ToolParallelism: prepared.definition.Execution.ToolParallelism,
		modelCallGate:   gate,
		permission:      permission,
	})
}

// prepareDefinitionModelRequest runs the exact provider-neutral assembly
// pipeline without invoking the provider. Structural operations and
// public read-only inspection share this seam so caller Middleware, tool
// schemas, cache routing, and stable-prefix authentication cannot drift.
func prepareDefinitionModelRequest(
	ctx context.Context,
	prepared preparedDefinition,
	session SessionView,
	run RunView,
	messages []*Message,
	stablePrefixMessages int,
) (*ModelRequestSnapshot, error) {
	permission := effectivePermissionPolicy(prepared.definition.Permission)
	permissionStage := &permissionMiddleware{
		BaseMiddleware: &BaseMiddleware{}, policy: permission, session: session, run: run,
	}
	loop, err := newPreparedDefinitionLoop(
		ctx,
		prepared,
		append([]Middleware(nil), prepared.definition.Middlewares...),
		permissionStage,
		nil,
	)
	if err != nil {
		return nil, err
	}
	return newLoopRunner(loopRunnerConfig{Agent: loop, EnableStreaming: true}).prepareModelRequest(
		ctx,
		messages,
		stablePrefixMessages,
	)
}

// stableContextPrefixMessages returns the Definition-owned contiguous prefix
// assembled before canonical conversation body messages. The checkpoint is
// stable only when it replaces from raw index zero; a custom interior
// replacement cannot extend the provider cache prefix across mutable history.
func stableContextPrefixMessages(
	fragments []ContextFragment,
	compaction CompactionState,
	compactionPresent bool,
) int {
	count := 0
	for _, fragment := range fragments {
		if fragment.Placement == ContextLeadingMessage {
			count++
		}
	}
	if compactionPresent && !compaction.Removed && compaction.ReplacementFrom == 0 {
		count++
	}
	return count
}

func renderContextFragment(fragment ContextFragment) string {
	if effectiveContextRendering(fragment.Rendering) == ContextRenderVerbatim {
		return fragment.Content
	}
	provenance := fmt.Sprintf(
		"Source: %s\nPurpose: %s\nResource: %s",
		fragment.Source, fragment.Purpose, fragment.Resource,
	)
	if revision := strings.TrimSpace(fragment.Revision); revision != "" {
		provenance += "\nRevision: " + revision
	}
	return "# Context\n\n" + provenance + "\n\n" + fragment.Content
}

func consumeMessageVariant(variant *loopMessage, source runstate.EventSource, displayOnly bool, emit runstate.EngineEventSink) (*Message, error) {
	if variant == nil {
		return nil, nil
	}
	toolInputs := newToolInputProjector(variant, source)
	if !variant.IsStreaming {
		message := CloneMessage(variant.Message)
		if message != nil && message.Role == Assistant {
			if message.Content != "" {
				if err := emit(runstate.EngineAssistantDelta{Source: source, Delta: message.Content, DisplayOnly: displayOnly}); err != nil {
					return nil, err
				}
			}
			if message.ReasoningContent != "" {
				if err := emit(runstate.EngineThinkingDelta{Source: source, Delta: message.ReasoningContent, DisplayOnly: displayOnly}); err != nil {
					return nil, err
				}
			}
		}
		if err := toolInputs.observe(message, emit); err != nil {
			return nil, err
		}
		return message, nil
	}
	if variant.MessageStream == nil {
		return nil, errors.New("Agent modelToolLoop returned a nil Message stream")
	}
	defer variant.MessageStream.Close()
	assembler := NewMessageAssembler()
	for {
		chunk, err := variant.MessageStream.Recv()
		if errors.Is(err, io.EOF) {
			return assembler.Message()
		}
		if err != nil {
			return nil, err
		}
		if chunk == nil {
			return nil, errors.New("Agent modelToolLoop streamed a nil Message chunk")
		}
		if chunk.Content != "" {
			if err := emit(runstate.EngineAssistantDelta{Source: source, Delta: chunk.Content, DisplayOnly: displayOnly}); err != nil {
				return nil, err
			}
		}
		if chunk.ReasoningContent != "" {
			if err := emit(runstate.EngineThinkingDelta{Source: source, Delta: chunk.ReasoningContent, DisplayOnly: displayOnly}); err != nil {
				return nil, err
			}
		}
		if err := assembler.Append(chunk); err != nil {
			return nil, err
		}
		message, err := assembler.Message()
		if err != nil {
			return nil, err
		}
		if err := toolInputs.observe(message, emit); err != nil {
			return nil, err
		}
	}
}

func (engine *definitionEngine) emitToolExecution(
	ctx context.Context,
	request runstate.EngineRequest,
	execution *toolExecutionEvent,
	source runstate.EventSource,
	canonical CanonicalAdapter,
	started map[string]bool,
	emit runstate.EngineEventSink,
) error {
	if execution == nil {
		return nil
	}
	callID := execution.ExecutionID
	if callID == "" {
		callID = execution.ProviderCallID
	}
	metadata, err := toolExecutionMetadata(execution.Definition.Descriptor)
	if err != nil {
		return err
	}
	if execution.ParentCallID != "" && !started[callID] {
		if err := emit(runstate.EngineToolInputStarted{
			CallID: callID, ParentCallID: execution.ParentCallID, Name: execution.ToolName,
			Index: execution.Index, Metadata: metadata, Source: source,
		}); err != nil {
			return err
		}
	}
	if execution.Phase == toolExecutionStarted {
		if !started[callID] {
			emitTrace(ctx, engine.trace, TraceEvent{
				Kind: TraceToolStarted, Session: engine.key, RunID: string(request.Snapshot.OperationID),
				Cycle: request.Snapshot.Cycle, ToolCallID: execution.ExecutionID, ToolName: execution.ToolName,
			})
			if err := emit(runstate.EngineToolStarted{
				CallID: callID, ProviderCallID: execution.ProviderCallID, Name: execution.ToolName, Index: execution.Index,
				Arguments: append(json.RawMessage(nil), execution.Arguments...), Metadata: metadata, Source: source,
				ExecutionAuthorized: true,
			}); err != nil {
				return err
			}
			started[callID] = true
		}
		return nil
	}
	switch execution.Phase {
	case toolExecutionProgress:
		return emit(runstate.EngineToolProgress{
			CallID: callID, ProviderCallID: execution.ProviderCallID, Name: execution.ToolName, Index: execution.Index,
			Delta: execution.Delta, Metadata: metadata, Source: source,
		})
	case toolExecutionFinished:
		// Policy denial and invalid preflight can finish before concrete Tool.Run.
		// Record a paired zero-side-effect start immediately before the result so
		// the tool lifecycle remains structurally complete.
		if !started[callID] {
			if err := emit(runstate.EngineToolStarted{
				CallID: callID, ProviderCallID: execution.ProviderCallID, Name: execution.ToolName, Index: execution.Index,
				Arguments: append(json.RawMessage(nil), execution.Arguments...), Metadata: metadata, Source: source,
			}); err != nil {
				return err
			}
			started[callID] = true
		}
		result := ToolResult{}
		if execution.Result != nil {
			result = *execution.Result
		}
		projection := result
		projection.Effects = nil
		encodedProjection, err := json.Marshal(projection)
		if err != nil {
			return fmt.Errorf("encode bounded Tool result projection: %w", err)
		}
		if len(result.Effects) != 0 && canonical == nil {
			return errors.New("Tool produced canonical Effects but Definition has no Canonical Adapter")
		}
		if err := engine.applyCanonicalEffects(ctx, request, canonical, callID, result.Effects); err != nil {
			return err
		}
		for index, artifact := range result.Artifacts {
			encoded, err := json.Marshal(artifact)
			if err != nil {
				return fmt.Errorf("encode Tool artifact %d: %w", index, err)
			}
			if err := emit(runstate.EngineArtifactProduced{CallID: callID, Artifact: encoded}); err != nil {
				return err
			}
		}
		emitTrace(ctx, engine.trace, TraceEvent{
			Kind: TraceToolFinished, Session: engine.key, RunID: string(request.Snapshot.OperationID),
			Cycle: request.Snapshot.Cycle, ToolCallID: callID, ToolName: execution.ToolName,
		})
		return emit(runstate.EngineToolFinished{
			CallID: callID, ProviderCallID: execution.ProviderCallID, Name: execution.ToolName, Index: execution.Index,
			Result:   result.DisplayContent,
			IsError:  result.IsError(),
			Metadata: metadata, Source: source, Projection: encodedProjection,
		})
	default:
		return fmt.Errorf("unsupported Tool execution phase %q", execution.Phase)
	}
}

func (engine *definitionEngine) applyCanonicalEffects(
	ctx context.Context,
	request runstate.EngineRequest,
	adapter CanonicalAdapter,
	callID string,
	effects []Effect,
) error {
	if len(effects) == 0 {
		return nil
	}
	requests := make([]EffectRequest, len(effects))
	for index, effect := range effects {
		digest, err := hashCanonical(struct {
			Version int
			Session SessionKey
			RunID   string
			Cycle   int
			CallID  string
			Index   int
		}{1, engine.key, string(request.Snapshot.OperationID), request.Snapshot.Cycle, callID, index})
		if err != nil {
			return err
		}
		requests[index] = EffectRequest{
			ID: "effect-" + digest,
			Identity: CommitIdentity{
				Session: engine.key, CommandID: string(request.Snapshot.CommandID),
				RunID: string(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle, Stage: CommitOutput,
			},
			CallID: callID, Index: index, Effect: effect,
		}
	}
	results, err := adapter.ApplyEffects(ctx, requests)
	if err != nil {
		return err
	}
	byID := make(map[string]EffectResult, len(results))
	for _, result := range results {
		byID[result.ID] = result
	}
	for _, request := range requests {
		result, ok := byID[request.ID]
		if !ok {
			return fmt.Errorf("Canonical Adapter omitted Tool effect %q", request.ID)
		}
		if result.Error != "" {
			return fmt.Errorf("apply canonical Tool effect %q: %s", request.ID, result.Error)
		}
		if strings.TrimSpace(result.Revision) == "" {
			return fmt.Errorf("Canonical Adapter Tool effect %q has no revision", request.ID)
		}
	}
	return nil
}

func toolExecutionMetadata(descriptor ToolDescriptor) (json.RawMessage, error) {
	if descriptor.Execution == "" {
		return nil, nil
	}
	presentation, err := descriptor.Presentation.Normalize()
	if err != nil {
		return nil, fmt.Errorf("normalize tool live presentation: %w", err)
	}
	// Keep descriptor fields flat so existing event metadata readers can
	// continue decoding execution semantics while presentation remains a
	// separate, model-invisible concern.
	metadata, err := json.Marshal(struct {
		ToolDescriptor
		Presentation ToolPresentation `json:"presentation"`
	}{ToolDescriptor: descriptor, Presentation: presentation})
	if err != nil {
		return nil, fmt.Errorf("encode tool live metadata: %w", err)
	}
	return metadata, nil
}

func modelRequestedToolNames(calls []ToolCall) []string {
	if len(calls) == 0 {
		return nil
	}
	result := make([]string, 0, len(calls))
	seen := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result
}

func runtimeEventSource(event *loopEvent) runstate.EventSource {
	if event == nil {
		return runstate.EventSource{}
	}
	source := runstate.EventSource{
		Name: strings.TrimSpace(event.AgentName), InvocationID: strings.TrimSpace(event.InvocationID),
		InvocationType: strings.TrimSpace(event.InvocationType),
	}
	for _, step := range event.RunPath {
		if name := strings.TrimSpace(step.String()); name != "" {
			source.Path = append(source.Path, name)
		}
	}
	if len(source.Path) == 0 && source.Name != "" {
		source.Path = []string{source.Name}
	}
	return source
}

func rootAgentEvent(event *loopEvent, rootName string) bool {
	if event == nil {
		return false
	}
	name := strings.TrimSpace(event.AgentName)
	rootName = strings.TrimSpace(rootName)
	if name != "" && rootName != "" && name != rootName {
		return false
	}
	return len(event.RunPath) <= 1
}
