package agentchat

import (
	"context"
	"denova/config"
	agentruntime "denova/internal/agents/runtime"
)

func (service *Service) agentSession(ctx context.Context, binding Binding, taskID string) (*agentruntime.Session, error) {
	project, err := service.projectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return nil, err
	}
	sess, err := project.store.Get(binding.SessionID)
	if err != nil {
		return nil, err
	}
	options := runtimeOptions(binding, taskID)
	snapshot, _ := sess.RuntimeConfig()
	return service.host.AgentEngines().ConversationSession(project.executionRuntime, options, sess, snapshot.Engine())
}

func (service *Service) externalController(ctx context.Context, binding Binding) (*agentruntime.ExternalController, bool, error) {
	project, err := service.projectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return nil, false, err
	}
	sess, err := project.store.Get(binding.SessionID)
	if err != nil {
		return nil, false, err
	}
	snapshot, found := sess.RuntimeConfig()
	if !found || snapshot.Engine().Kind == config.RuntimeNative {
		return nil, false, nil
	}
	state, err := agentruntime.SessionState(runtimeOptions(binding, ""), sess)
	if err != nil {
		return nil, true, err
	}
	control, err := service.host.AgentEngines().ExternalControl(runtimeOptions(binding, ""), state)
	return control, true, err
}
