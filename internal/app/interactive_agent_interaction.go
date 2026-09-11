package app

import (
	"context"
	"strings"

	agentrun "denova/internal/agents/run"
)

// ResolveInteractiveAsk uses the exact Story branch binding even while its
// display task is closed. Answering never resumes the underlying Agent Run.
func (a *App) ResolveInteractiveAsk(ctx context.Context, storyID, branchID, askID, status string, answers []AgentAskAnswer, cancelReason string) (AgentAskResolution, error) {
	if a == nil {
		return AgentAskResolution{}, ErrNoWorkspace
	}
	a.mu.RLock()
	workspace := a.workspace
	a.mu.RUnlock()
	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return AgentAskResolution{}, err
	}
	defer operation.Release()
	a.mu.RLock()
	store, runtime := a.interactive, a.executionRuntime
	projectID := ""
	if a.cfg != nil {
		projectID = a.cfg.ProjectID
	}
	a.mu.RUnlock()
	if store == nil || runtime == nil {
		return AgentAskResolution{}, ErrNoWorkspace
	}
	storyID = strings.TrimSpace(storyID)
	branchID, err = resolveInteractiveProjectionBranch(store, storyID, strings.TrimSpace(branchID))
	if err != nil {
		return AgentAskResolution{}, err
	}
	return runtime.ResolveAsk(operation.Context(), agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, ProjectID: projectID,
		Workspace: workspace, StoryID: storyID, BranchID: branchID, Mode: "interactive",
	}, askID, status, answers, cancelReason)
}
