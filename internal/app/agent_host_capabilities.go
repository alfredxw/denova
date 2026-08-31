package app

import (
	"context"
	"log/slog"

	"denova/config"
	agents "denova/internal/agents"
	producttools "denova/internal/agents/tools"
	"denova/internal/agents/trajectory"
)

// AgentHostCapabilities is the shared host surface for ordinary Writing,
// Game, and General Agents. Agent Profiles are already resolved in Config, so
// no global state is injected into the prompt or context.
func (a *App) AgentHostCapabilities(
	_ context.Context,
	_ *config.Config,
	_ string,
) (agents.AgentHostCapabilities, error) {
	return agents.AgentHostCapabilities{Interactive: true}, nil
}

// AgentsProjectAgentHostCapabilities adds only the on-demand, read-only
// trajectory:// adapter to the Agents Project's ordinary General Agent.
func (a *App) AgentsProjectAgentHostCapabilities(
	_ context.Context,
	_ *config.Config,
	_ string,
) (agents.AgentHostCapabilities, error) {
	host := agents.AgentHostCapabilities{Interactive: true}
	if a == nil {
		return host, nil
	}
	adapter, err := trajectory.NewReadAdapter(trajectory.Catalog{
		Sources: func(ctx context.Context) ([]trajectory.Source, error) {
			sources, issues, sourceErr := a.globalTrajectorySources(ctx)
			for _, issue := range issues {
				slog.WarnContext(ctx, "[trajectory] skip unavailable Project source",
					"project_id", issue.ProjectID,
					"error", issue.Message,
				)
			}
			return sources, sourceErr
		},
		Outcomes: a.trajectoryOutcomes,
		Limit:    100,
	})
	if err != nil {
		return agents.AgentHostCapabilities{}, err
	}
	binding, err := producttools.NewReadAdapterBinding(config.AgentToolTrajectory, adapter)
	if err != nil {
		return agents.AgentHostCapabilities{}, err
	}
	host.ReadAdapters = []producttools.ReadAdapterBinding{binding}
	return host, nil
}
