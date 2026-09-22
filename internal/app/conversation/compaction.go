package conversationapp

import (
	"context"

	agentchat "denova/internal/agents/chat"
	agentcompaction "denova/internal/agents/context/compaction"
	agentstructural "denova/internal/agents/context/structural"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	"denova/internal/agents/runtime/external"
	agent "github.com/alfredxw/denova/agent"
)

// Compact dispatches maintenance to the selected peer runtime. Product history
// remains unchanged; only observations and reconstruction checkpoints are saved.
func (execution Execution) Compact(ctx context.Context, commandID string, options agentrun.Options) (agentcompaction.Result, error) {
	if execution.external == nil {
		result, err := execution.runtime.ExecutionRuntime.ExecuteStructuralOperation(ctx, agentexecution.Cycle{
			Definition: execution.native.Definition, Conversation: ProjectConversation(execution.runtime, agentchat.ChatRequest{}),
			BookService: execution.runtime.BookService, Options: options,
		}, agentstructural.Spec{CommandID: commandID, Action: agentstructural.Compact, Ref: agentrun.ContextCompactionRef{Force: true}})
		return result.Compaction, err
	}
	state, err := agentruntime.SessionState(options, execution.runtime.Session)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	control, err := execution.engines.ExternalControl(options, state)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	return control.Maintain(ctx, commandID, func() (agentcompaction.Result, error) {
		request := agentchat.ChatRequest{CommandID: commandID, Message: "Maintain the current conversation context."}
		prepared, err := prepareExternal(ctx, execution.runtime, request, ProjectConversation(execution.runtime, request), *execution.external, options, nil)
		if err != nil {
			return agentcompaction.Result{}, err
		}
		prepared.Runtime = execution.engines.ExternalRuntime(execution.runtime.Config)
		return external.CompactSession(ctx, prepared, func(ctx context.Context, preparation external.HistoryPreparation) (external.Input, error) {
			var usageErr error
			preparation.AddUsage = func(usage *agent.TokenUsage) {
				if usage != nil {
					usageErr = execution.runtime.Session.AppendDisplayEvent(external.UsageDisplay(usage))
				}
			}
			bounded, err := state.PrepareExternalHistory(ctx, preparation)
			if err == nil {
				err = usageErr
			}
			return bounded, err
		})
	})
}
