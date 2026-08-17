package agentchat

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"

	chatagent "denova/internal/agents/chat"
	agentdelegation "denova/internal/agents/delegation"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	appagentruntime "denova/internal/app/agentruntime"
	conversationapp "denova/internal/app/conversation"
)

func (service *Service) PrepareChildDefinition(
	ctx context.Context,
	binding agentrun.RuntimeBinding,
	child string,
	parentRequest ...chatagent.ChatRequest,
) (agent.Definition, error) {
	scope, err := service.ResolveBinding(Binding{
		ProjectID: binding.ProjectID, Workspace: binding.Workspace, SessionID: binding.SessionID,
	})
	if err != nil {
		return agent.Definition{}, err
	}
	project, err := service.projectRuntime(ctx, scope.ProjectID)
	if err != nil {
		return agent.Definition{}, err
	}
	sess, _, err := getOrCreateConversation(project, scope)
	if err != nil {
		return agent.Definition{}, err
	}
	request := chatagent.ChatRequest{}
	if len(parentRequest) > 0 {
		request = parentRequest[0]
	}
	runtime, _, err := conversationapp.Prepare(ctx, project.conversation(sess), request)
	if err != nil {
		return agent.Definition{}, err
	}
	if runtime.ProjectID != scope.ProjectID || runtime.Session == nil || runtime.Session.ID != scope.SessionID {
		return agent.Definition{}, fmt.Errorf("%w: delegated AgentChat parent changed", agentexecution.ErrCyclePreparationUnavailable)
	}
	agentHost, err := service.host.HarnessAgentHostCapabilities(ctx, &runtime.Config, runtime.AgentKind)
	if err != nil {
		return agent.Definition{}, err
	}
	built, err := appagentruntime.BuildConversationAgent(
		ctx, &runtime.Config, runtime.State, runtime.IDETeller, runtime.AgentKind,
		agentHost,
	)
	if err != nil {
		return agent.Definition{}, err
	}
	return agentdelegation.ChildDefinition(built.Definition, child)
}
