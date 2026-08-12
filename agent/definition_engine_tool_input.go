package agent

import (
	"fmt"
	"strings"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

type projectedToolInput struct {
	name      string
	arguments string
	started   bool
}

// toolInputProjector observes the same fully merged tool-call view consumed by
// the native loop, so live display deltas and eventual execution share one
// execution identity and one append-only argument stream.
type toolInputProjector struct {
	variant *MessageVariant
	source  runstate.EventSource
	calls   map[int]projectedToolInput
}

func newToolInputProjector(variant *MessageVariant, source runstate.EventSource) *toolInputProjector {
	return &toolInputProjector{
		variant: variant, source: source, calls: make(map[int]projectedToolInput),
	}
}

func (projector *toolInputProjector) observe(message *Message, emit runstate.EngineEventSink) error {
	if projector == nil || message == nil || (message.Role != Assistant && projector.variant.Role != Assistant) {
		return nil
	}
	for ordinal, call := range message.ToolCalls {
		name := call.Function.Name
		if strings.TrimSpace(name) == "" {
			continue
		}
		state := projector.calls[ordinal]
		if state.name != "" && state.name != name {
			return fmt.Errorf("streamed tool input at ordinal %d changed name from %q to %q", ordinal, state.name, name)
		}
		if !strings.HasPrefix(call.Function.Arguments, state.arguments) {
			return fmt.Errorf("streamed tool input for %q at ordinal %d is not append-only", name, ordinal)
		}
		callID := projector.variant.ToolExecutionID(ordinal)
		if strings.TrimSpace(callID) == "" {
			return fmt.Errorf("assistant tool %q at ordinal %d is missing an execution ID", name, ordinal)
		}
		if !state.started {
			if err := emit(runstate.EngineToolInputStarted{
				CallID: callID, ProviderCallID: call.ID, Name: name, Source: projector.source,
			}); err != nil {
				return err
			}
			state.started = true
			state.name = name
		}
		if delta := strings.TrimPrefix(call.Function.Arguments, state.arguments); delta != "" {
			if err := emit(runstate.EngineToolInputDelta{
				CallID: callID, ProviderCallID: call.ID, Name: name, Delta: delta, Source: projector.source,
			}); err != nil {
				return err
			}
		}
		state.arguments = call.Function.Arguments
		projector.calls[ordinal] = state
	}
	return nil
}
