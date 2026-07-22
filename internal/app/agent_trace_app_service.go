package app

import "denova/internal/agent"

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

func (a *App) ExportAgentRunTrace(id string) (agent.RunTraceExport, error) {
	if !a.HasWorkspace() {
		return agent.RunTraceExport{}, ErrNoWorkspace
	}
	return agent.ExportRunTrace(a.Workspace(), id)
}
