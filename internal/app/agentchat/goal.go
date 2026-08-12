package agentchat

import (
	"context"
	"errors"
	"fmt"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
	publicgoal "github.com/alfredxw/denova/agent/goal"
)

func (service *Service) ConversationGoal(ctx context.Context, binding Binding) (agent.GoalState, bool, error) {
	service.admission.Lock()
	defer service.admission.Unlock()
	resolved, project, _, err := service.conversationRuntime(ctx, binding)
	if err != nil {
		return agent.GoalState{}, false, err
	}
	if !project.store.Exists(resolved.SessionID) {
		return agent.GoalState{}, false, nil
	}
	return project.executionRuntime.Goal(ctx, runtimeOptions(resolved, ""))
}

func (service *Service) MutateConversationGoal(ctx context.Context, binding Binding, action string, objective string, expectedRevision uint64) (agent.GoalState, error) {
	service.admission.Lock()
	defer service.admission.Unlock()
	resolved, project, runtimeCfg, err := service.conversationRuntime(ctx, binding)
	if err != nil {
		return agent.GoalState{}, err
	}
	if (action == "set" || action == "resume") && !config.ResolveAgentTools(&runtimeCfg, resolved.agentKind).Allows(config.AgentToolGoal) {
		return agent.GoalState{}, errors.New("conversation goal is disabled for Agent Chat")
	}
	_, _, err = getOrCreateConversation(project, resolved)
	if err != nil {
		return agent.GoalState{}, err
	}
	mutation := agent.GoalMutation{ExpectedRevision: expectedRevision}
	switch action {
	case "set":
		mutation.Kind, mutation.Objective = agent.GoalSet, objective
	case "pause":
		mutation.Kind = agent.GoalPause
	case "resume":
		mutation.Kind = agent.GoalResume
	case "clear":
		mutation.Kind = agent.GoalClear
	default:
		return agent.GoalState{}, fmt.Errorf("unsupported goal action %q", action)
	}
	return project.executionRuntime.UpdateGoal(ctx, runtimeOptions(resolved, ""), mutation)
}

func IsGoalRevisionConflict(err error) bool { return errors.Is(err, publicgoal.ErrRevisionConflict) }
