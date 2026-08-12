package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	runstate "github.com/alfredxw/denova/agent/runtime"
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
	session := SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)}
	run := runViewForTurn(request.Snapshot)
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
	var prefixes []string
	var finalUserMessage string
	hasFinalUserMessage := false
	for _, fragment := range fragments {
		rendered := renderContextFragment(fragment)
		switch fragment.Placement {
		case ContextLeadingMessage:
			switch effectiveContextRole(fragment) {
			case User:
				messages = append(messages, UserMessage(rendered))
			default:
				messages = append(messages, SystemMessage(rendered))
			}
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

func consumeMessageVariant(variant *MessageVariant, source runstate.EventSource, displayOnly bool, emit runstate.EngineEventSink) (*Message, error) {
	if variant == nil {
		return nil, nil
	}
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
		return message, nil
	}
	if variant.MessageStream == nil {
		return nil, errors.New("Agent Loop returned a nil Message stream")
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
			return nil, errors.New("Agent Loop streamed a nil Message chunk")
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
	}
}

func (engine *definitionEngine) emitToolExecution(
	ctx context.Context,
	request runstate.EngineRequest,
	execution *ToolExecutionEvent,
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
	if execution.Phase == ToolExecutionStarted {
		if !started[callID] {
			emitTrace(ctx, engine.trace, TraceEvent{
				Kind: TraceToolStarted, Session: engine.key, RunID: string(request.Snapshot.OperationID),
				Cycle: request.Snapshot.Cycle, ToolCallID: execution.ExecutionID, ToolName: execution.ToolName,
			})
			if err := emit(runstate.EngineToolStarted{
				CallID: callID, Name: execution.ToolName, Arguments: append(json.RawMessage(nil), execution.Arguments...), Source: source,
			}); err != nil {
				return err
			}
			started[callID] = true
		}
		return nil
	}
	switch execution.Phase {
	case ToolExecutionProgress:
		return emit(runstate.EngineToolProgress{CallID: callID, Delta: execution.Delta, Source: source})
	case ToolExecutionFinished:
		// Policy denial and invalid preflight can finish before concrete Tool.Run.
		// Record a paired zero-side-effect start immediately before the result so
		// runtime recovery remains structurally complete.
		if !started[callID] {
			if err := emit(runstate.EngineToolStarted{
				CallID: callID, Name: execution.ToolName, Arguments: append(json.RawMessage(nil), execution.Arguments...), Source: source,
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
		hostEffects := make([]runstate.HostEffect, len(result.Effects))
		if len(result.Effects) != 0 && canonical == nil {
			return errors.New("Tool produced canonical Effects but Definition has no Canonical Adapter")
		}
		for index, effect := range result.Effects {
			payload, err := json.Marshal(effect)
			if err != nil {
				return fmt.Errorf("encode Tool effect %d: %w", index, err)
			}
			hostEffect, err := runstate.NewToolHostEffect(
				request.Binding, request.Snapshot.OperationID, request.Snapshot.Cycle,
				callID, index, runstate.HostEffectKind(effect.Kind), payload,
			)
			if err != nil {
				return err
			}
			hostEffects[index] = hostEffect
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
			CallID: callID, Name: execution.ToolName, Result: result.DisplayContent,
			IsError: result.IsError(), RetrySafety: retrySafety(execution.Definition.Descriptor.Recovery),
			Source: source, Projection: encodedProjection, HostEffects: hostEffects,
		})
	default:
		return fmt.Errorf("unsupported Tool execution phase %q", execution.Phase)
	}
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

func runtimeEventSource(event *AgentEvent) runstate.EventSource {
	if event == nil {
		return runstate.EventSource{}
	}
	source := runstate.EventSource{Name: strings.TrimSpace(event.AgentName)}
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

func rootAgentEvent(event *AgentEvent, rootName string) bool {
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

func retrySafety(recovery ToolRecoveryClass) runstate.RetrySafety {
	switch recovery {
	case ToolRecoveryReadOnly, ToolRecoveryIdempotent:
		return runstate.RetrySafe
	case ToolRecoveryNonIdempotent:
		return runstate.RetryUnsafe
	default:
		return runstate.RetryUnknown
	}
}
