package app

import (
	"context"

	"denova/config"
	agents "denova/internal/agents"
)

// HarnessAgentHostCapabilities is the application composition seam shared by
// Writing, Game, Agent Chat, and recovery paths.
func (a *App) HarnessAgentHostCapabilities(
	ctx context.Context,
	cfg *config.Config,
	agentKind string,
) (agents.AgentHostCapabilities, error) {
	if a == nil {
		return agents.AgentHostCapabilities{}, nil
	}
	return a.ContinualLearning().AgentHostCapabilities(ctx, cfg, agentKind)
}
