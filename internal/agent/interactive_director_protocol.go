package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/eino/adk"

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

func withInteractiveDirectorPlanCancel(ctx context.Context, cancel adk.AgentCancelFunc) context.Context {
	return context.WithValue(ctx, interactiveDirectorPlanCancelKey{}, cancel)
}

func requestInteractiveDirectorPlanCompletion(ctx context.Context) bool {
	cancel, _ := ctx.Value(interactiveDirectorPlanCancelKey{}).(adk.AgentCancelFunc)
	if cancel == nil {
		return false
	}
	_, contributed := cancel(adk.WithAgentCancelMode(adk.CancelAfterToolCalls))
	return contributed
}

func interactiveDirectorPlanCompletedByCancel(err error, agentKind, task string) bool {
	if err == nil || !isInteractiveDirectorPlanRun(agentKind, task) {
		return false
	}
	var cancelErr *adk.CancelError
	return errors.As(err, &cancelErr) && cancelErr.Info != nil && cancelErr.Info.Mode&adk.CancelAfterToolCalls != 0
}
