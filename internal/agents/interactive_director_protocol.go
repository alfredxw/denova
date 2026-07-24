package agents

import (
	"context"
	"errors"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

type interactiveDirectorPlanCancelKey struct{}

func isInteractiveDirectorPlanTask(task string) bool {
	switch strings.TrimSpace(task) {
	case "director_plan_update", "opening_plan":
		return true
	default:
		return false
	}
}

func isInteractiveDirectorPlanRun(agentKind, task string) bool {
	return agentKind == config.AgentKindInteractiveDirector && isInteractiveDirectorPlanTask(task)
}

func withInteractiveDirectorPlanCancel(ctx context.Context, cancel agent.AgentCancelFunc) context.Context {
	return context.WithValue(ctx, interactiveDirectorPlanCancelKey{}, cancel)
}

func requestInteractiveDirectorPlanCompletion(ctx context.Context) bool {
	cancel, _ := ctx.Value(interactiveDirectorPlanCancelKey{}).(agent.AgentCancelFunc)
	if cancel == nil {
		return false
	}
	_, contributed := cancel(agent.WithAgentCancelMode(agent.CancelAfterToolCalls))
	return contributed
}

func interactiveDirectorPlanCompletedByCancel(err error, agentKind, task string) bool {
	if err == nil || !isInteractiveDirectorPlanRun(agentKind, task) {
		return false
	}
	var cancelErr *agent.CancelError
	return errors.As(err, &cancelErr) && cancelErr.Info != nil && cancelErr.Info.Mode&agent.CancelAfterToolCalls != 0
}
