package app

import (
	"context"

	"nova/internal/agent"
)

func (a *App) automationMutationCallback(source string) func(context.Context, []agent.ToolMutation, agent.PostRunVerification) {
	return func(ctx context.Context, mutations []agent.ToolMutation, _ agent.PostRunVerification) {
		paths := make([]string, 0, len(mutations))
		for _, mutation := range mutations {
			if mutation.Target == "" {
				continue
			}
			paths = append(paths, mutation.Target)
		}
		a.CheckAutomationTriggersAfterWorkspaceMutation(ctx, source, paths)
	}
}
