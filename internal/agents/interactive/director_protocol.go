package interactive

import (
	"context"
	"errors"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

type interactiveDirectorPlanCancelKey struct{}

func IsDirectorPlanTask(task string) bool {
	switch strings.TrimSpace(task) {
	case "director_plan_update", "opening_plan":
		return true
	default:
		return false
	}
}

func IsDirectorPlanRun(agentKind, task string) bool {
	return agentKind == config.AgentKindInteractiveDirector && IsDirectorPlanTask(task)
}

func WithDirectorPlanCancel(ctx context.Context, cancel agent.AgentCancelFunc) context.Context {
	return context.WithValue(ctx, interactiveDirectorPlanCancelKey{}, cancel)
}

func RequestDirectorPlanCompletion(ctx context.Context) bool {
	cancel, _ := ctx.Value(interactiveDirectorPlanCancelKey{}).(agent.AgentCancelFunc)
	if cancel == nil {
		return false
	}
	_, contributed := cancel(agent.WithAgentCancelMode(agent.CancelAfterToolCalls))
	return contributed
}

func DirectorPlanCompletedByCancel(err error, agentKind, task string) bool {
	if err == nil || !IsDirectorPlanRun(agentKind, task) {
		return false
	}
	var cancelErr *agent.CancelError
	return errors.As(err, &cancelErr) && cancelErr.Info != nil && cancelErr.Info.Mode&agent.CancelAfterToolCalls != 0
}
