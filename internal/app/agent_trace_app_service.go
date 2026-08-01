package app

import "denova/internal/agents/run"

func (a *App) AgentRunTraces(limit int) ([]agentrun.RunTraceSummary, error) {
	if !a.HasWorkspace() {
		return []agentrun.RunTraceSummary{}, nil
	}
	return agentrun.ListRunTraces(a.Workspace(), limit)
}

func (a *App) AgentRunTrace(id string) (agentrun.RunTrace, error) {
	if !a.HasWorkspace() {
		return agentrun.RunTrace{}, ErrNoWorkspace
	}
	return agentrun.ReadRunTrace(a.Workspace(), id)
}

func (a *App) ExportAgentRunTrace(id string) (agentrun.RunTraceExport, error) {
	if !a.HasWorkspace() {
		return agentrun.RunTraceExport{}, ErrNoWorkspace
	}
	return agentrun.ExportRunTrace(a.Workspace(), id)
}
