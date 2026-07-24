package app

import agents "denova/internal/agents"

func (a *App) AgentRunTraces(limit int) ([]agents.RunTraceSummary, error) {
	if !a.HasWorkspace() {
		return []agents.RunTraceSummary{}, nil
	}
	return agents.ListRunTraces(a.Workspace(), limit)
}

func (a *App) AgentRunTrace(id string) (agents.RunTrace, error) {
	if !a.HasWorkspace() {
		return agents.RunTrace{}, ErrNoWorkspace
	}
	return agents.ReadRunTrace(a.Workspace(), id)
}

func (a *App) ExportAgentRunTrace(id string) (agents.RunTraceExport, error) {
	if !a.HasWorkspace() {
		return agents.RunTraceExport{}, ErrNoWorkspace
	}
	return agents.ExportRunTrace(a.Workspace(), id)
}
