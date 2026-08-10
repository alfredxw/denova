package app

import (
	"context"
	"fmt"
	"strings"

	agentstructural "denova/internal/agents/context/structural"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	compactionapp "denova/internal/app/compaction"
)

func (service *ChatAppService) resumeWritingContextStructuralOperation(ctx context.Context, action agentstructural.Action) (agentstructural.Result, bool, error) {
	if service == nil || service.app == nil {
		return agentstructural.Result{}, false, nil
	}
	app := service.app
	app.mu.RLock()
	chat := app.executionRuntime
	workspace := app.workspace
	stateRoot := ""
	if app.cfg != nil {
		stateRoot = app.cfg.ProjectStateDir
	}
	sessionID := ""
	selected := app.session
	if app.session != nil {
		sessionID = app.session.ID
	}
	app.mu.RUnlock()
	if chat == nil || strings.TrimSpace(workspace) == "" || strings.TrimSpace(sessionID) == "" {
		return agentstructural.Result{}, false, nil
	}
	result, resumed, err := chat.ResumeRecoveredStructuralOperation(ctx, agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, StateRoot: stateRoot, Workspace: workspace, SessionID: sessionID, Mode: "ide",
	}, action)
	if !resumed || selected == nil {
		return result, resumed, err
	}
	// Record the refresh obligation before refreshing independently restored
	// Session state, so later starts remain fenced after any read failure.
	recoveryAction := agentexecution.RuntimeRecoveryAction{Kind: compactionapp.RecoveryActionFor(action)}
	service.markRecoveryRefreshPending(workspace, sessionID, recoveryAction)
	if err != nil {
		return result, true, err
	}
	matched, refreshErr := service.retryRecoveryRefresh(ctx, workspace, sessionID, recoveryAction, selected.RefreshCanonical)
	if !matched && refreshErr == nil {
		_, refreshErr = service.retryAnyRecoveryRefresh(ctx, workspace, sessionID, selected.RefreshCanonical)
	}
	if refreshErr != nil {
		return result, true, fmt.Errorf("refresh recovered writing context: %w", refreshErr)
	}
	return result, true, nil
}

func (service *InteractiveAppService) resumeStoryContextStructuralOperation(
	ctx context.Context,
	workspace, storyID, branchID string,
	action agentstructural.Action,
) (agentstructural.Result, bool, error) {
	if service == nil || service.app == nil {
		return agentstructural.Result{}, false, nil
	}
	service.app.mu.RLock()
	chat := service.app.executionRuntime
	service.app.mu.RUnlock()
	if chat == nil {
		return agentstructural.Result{}, false, nil
	}
	return chat.ResumeRecoveredStructuralOperation(ctx, agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace,
		StoryID: storyID, BranchID: branchID, Mode: "interactive",
	}, action)
}

func (app *App) restoreContextStructuralOperation(ctx context.Context, request agentexecution.StructuralRestoreRequest) (agentstructural.Spec, error) {
	return compactionapp.RestoreSpec(ctx, request, app.sessionDirectoryForBinding)
}
