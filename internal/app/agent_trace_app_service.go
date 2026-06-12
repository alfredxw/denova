package app

import "nova/internal/agent"

func (a *App) AgentRunTraces(limit int) ([]agent.RunTraceSummary, error) {
	if !a.HasWorkspace() {
		return []agent.RunTraceSummary{}, nil
	}
	return agent.ListRunTraces(a.Workspace(), limit)
}

func (a *App) AgentRunTrace(id string) (agent.RunTrace, error) {
	if !a.HasWorkspace() {
		return agent.RunTrace{}, ErrNoWorkspace
	}
	return agent.ReadRunTrace(a.Workspace(), id)
}
