package agentchat

import (
	"context"
	"errors"
	"fmt"

	"denova/config"
	"denova/internal/agents/goal"
)

func (service *Service) ConversationGoal(ctx context.Context, binding Binding) (goal.State, bool, error) {
	service.admission.Lock()
	defer service.admission.Unlock()
	resolved, project, _, err := service.conversationRuntime(ctx, binding)
	if err != nil {
		return goal.State{}, false, err
	}
	if !project.store.Exists(resolved.SessionID) {
		return goal.State{}, false, nil
	}
	sess, err := project.store.Get(resolved.SessionID)
	if err != nil {
		return goal.State{}, false, err
	}
	return sess.Goal(ctx)
}

func (service *Service) MutateConversationGoal(ctx context.Context, binding Binding, action string, objective string, expectedRevision uint64) (goal.State, error) {
	service.admission.Lock()
	defer service.admission.Unlock()
	resolved, project, runtimeCfg, err := service.conversationRuntime(ctx, binding)
	if err != nil {
		return goal.State{}, err
	}
	if (action == "set" || action == "resume") && !config.ResolveAgentTools(&runtimeCfg, resolved.agentKind).Allows(config.AgentToolGoal) {
		return goal.State{}, errors.New("conversation goal is disabled for Agent Chat")
	}
	sess, _, err := getOrCreateConversation(project, resolved)
	if err != nil {
		return goal.State{}, err
	}
	switch action {
	case "set":
		return sess.SetGoal(ctx, objective, expectedRevision)
	case "pause":
		return sess.PauseGoal(ctx, expectedRevision)
	case "resume":
		return sess.ResumeGoal(ctx, expectedRevision)
	case "clear":
		return sess.ClearGoal(ctx, expectedRevision)
	default:
		return goal.State{}, fmt.Errorf("unsupported goal action %q", action)
	}
}

func IsGoalRevisionConflict(err error) bool { return errors.Is(err, goal.ErrRevisionConflict) }
