package app

import (
	"context"

	"denova/internal/agent"
)

func (a *App) automationMutationCallback(source string) func(context.Context, []agent.ToolMutation, agent.PostRunVerification) {
	scoped := a.automationSnapshot()
	if scoped != nil {
		return scoped.automationMutationCallback(source)
	}
	return func(context.Context, []agent.ToolMutation, agent.PostRunVerification) {}
}
