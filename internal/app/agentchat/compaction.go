package agentchat

import (
	"context"
	"fmt"

	chatagent "denova/internal/agents/chat"
	agentcompaction "denova/internal/agents/context/compaction"
	agentstructural "denova/internal/agents/context/structural"
	compactionapp "denova/internal/app/compaction"
	conversationapp "denova/internal/app/conversation"
)

// CompactContext admits maintenance for exactly the selected Project/Session.
// It shares executor preparation with turns, without creating a user input.
func (service *Service) CompactContext(ctx context.Context, binding Binding, commandID string) (agentcompaction.Result, error) {
	service.admission.Lock()
	defer service.admission.Unlock()
	binding, err := service.ResolveBinding(binding)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	if err := service.requireIdle(binding); err != nil {
		return agentcompaction.Result{}, err
	}
	project, err := service.projectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	sess, err := project.store.Get(binding.SessionID)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	commandID, err = compactionapp.ResolveCommandID(commandID, agentstructural.CommandID("agent-chat-compact", binding.ProjectID, binding.SessionID, fmt.Sprint(sess.ContextCursor().Revision)))
	if err != nil {
		return agentcompaction.Result{}, err
	}
	runtime, _, err := conversationapp.Prepare(ctx, project.conversation(sess), chatagent.ChatRequest{})
	if err != nil {
		return agentcompaction.Result{}, err
	}
	host, err := service.host.ProjectAgentHostCapabilities(ctx, runtime.ProjectType, &runtime.Config, runtime.AgentKind)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	execution, err := conversationapp.BuildExecution(ctx, runtime, host, service.host.AgentEngines(), "")
	if err != nil {
		return agentcompaction.Result{}, err
	}
	return execution.Compact(ctx, commandID, runtimeOptions(binding, ""))
}
