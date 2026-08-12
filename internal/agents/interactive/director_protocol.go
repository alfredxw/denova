package interactive

import (
	"context"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

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

func RequestDirectorPlanCompletion(ctx context.Context) bool {
	return agent.RequestCompletionAfterTools(ctx)
}
