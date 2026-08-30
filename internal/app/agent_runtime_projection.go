package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	apptask "denova/internal/app/task"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	appagentruntime "denova/internal/app/agentruntime"
	"denova/internal/interactive"
)

type WritingAgentActiveView struct {
	SessionID             string
	Task                  *apptask.Snapshot
	Runtime               agentrun.RuntimeStatus
	RuntimeProjectionOK   bool
	PendingAsk            *session.AskInteraction
	PendingInterruptionID string
	// RecoveryActions can contain a process-local projection refresh action
	// after the durable runtime has already settled to Idle.
	RecoveryActions []agentexecution.RuntimeRecoveryAction
}

type InteractiveAgentActiveView struct {
	Task                  *apptask.Snapshot
	Info                  InteractiveTaskInfo
	Runtime               agentrun.RuntimeStatus
	RuntimeProjectionOK   bool
	PendingInterruptionID string
}

// WritingAgentActiveView returns task metadata and durable runtime state from
// one exact workspace/session generation. Task fields come from one Task lock.
func (a *App) WritingAgentActiveView(ctx context.Context) WritingAgentActiveView {
	if a == nil {
		return WritingAgentActiveView{}
	}
	a.mu.RLock()
	workspace := strings.TrimSpace(a.workspace)
	projectID := ""
	if a.cfg != nil {
		projectID = strings.TrimSpace(a.cfg.ProjectID)
	}
	a.mu.RUnlock()
	if workspace == "" {
		return WritingAgentActiveView{}
	}
	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return WritingAgentActiveView{}
	}
	defer operation.Release()

	a.mu.RLock()
	executionRuntime := a.executionRuntime
	selectedSession := a.session
	stateRoot := ""
	if a.cfg != nil {
		stateRoot = a.cfg.ProjectStateDir
	}
	task := activeWritingTaskLocked(a)
	a.mu.RUnlock()
	sessionID := ""
	if selectedSession != nil {
		sessionID = strings.TrimSpace(selectedSession.ID)
	}
	var runtimeSnapshot agentrun.RuntimeStatus
	projected := false
	if sessionID != "" && executionRuntime != nil {
		runtimeSnapshot, projected = projectAgentRuntime(operation.Context(), executionRuntime, agentrun.Options{
			AgentKind: agentrun.AgentKindIDE,
			ProjectID: projectID,
			StateRoot: stateRoot,
			Workspace: workspace,
			SessionID: sessionID,
		})
	}
	recoveryActions := agentexecution.RuntimeRecoveryActions(runtimeSnapshot)
	var pendingAsk *session.AskInteraction
	if projected && len(runtimeSnapshot.PendingInteractions) > 0 {
		pendingAsk = agentchat.ProjectPendingInteraction(runtimeSnapshot.PendingInteractions[0], runtimeSnapshot)
	}
	pendingInterruptionID := ""
	if selectedSession != nil {
		if pending := selectedSession.PendingInterruption(); pending != nil {
			pendingInterruptionID = strings.TrimSpace(pending.ID)
		}
	}

	a.mu.RLock()
	if lifecycleWorkspaceKey(a.workspace) != lifecycleWorkspaceKey(workspace) || a.executionRuntime != executionRuntime || a.session != selectedSession || activeWritingTaskLocked(a) != task {
		a.mu.RUnlock()
		return WritingAgentActiveView{}
	}
	var taskSnapshot *apptask.Snapshot
	if task != nil {
		snapshot := task.Snapshot()
		taskSnapshot = &snapshot
	}
	a.mu.RUnlock()
	return WritingAgentActiveView{
		SessionID: sessionID, Task: taskSnapshot, Runtime: runtimeSnapshot, RuntimeProjectionOK: projected,
		RecoveryActions: recoveryActions, PendingAsk: pendingAsk, PendingInterruptionID: pendingInterruptionID,
	}
}

// InteractiveAgentActiveView binds reconnect metadata and runtime projection
// to one exact workspace/story/branch generation.
func (a *App) InteractiveAgentActiveView(ctx context.Context, storyID, branchID string) InteractiveAgentActiveView {
	if a == nil {
		return InteractiveAgentActiveView{}
	}
	storyID = strings.TrimSpace(storyID)
	branchID = strings.TrimSpace(branchID)
	a.mu.RLock()
	workspace := strings.TrimSpace(a.workspace)
	projectID := ""
	if a.cfg != nil {
		projectID = strings.TrimSpace(a.cfg.ProjectID)
	}
	a.mu.RUnlock()
	if workspace == "" || storyID == "" {
		return InteractiveAgentActiveView{}
	}
	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return InteractiveAgentActiveView{}
	}
	defer operation.Release()

	a.mu.RLock()
	executionRuntime := a.executionRuntime
	store := a.interactive
	task, info := activeInteractiveTaskLocked(a, storyID, branchID)
	a.mu.RUnlock()
	projectionBranch := branchID
	if task != nil && strings.TrimSpace(info.BranchID) != "" {
		projectionBranch = info.BranchID
	}
	resolved, err := resolveInteractiveProjectionBranch(store, storyID, projectionBranch)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[agent-runtime-projection] resolve game binding failed workspace=%s story_id=%s branch_id=%s err=%v", workspace, storyID, projectionBranch, err))
		resolved = ""
	}
	var runtimeSnapshot agentrun.RuntimeStatus
	projected := false
	if resolved != "" && executionRuntime != nil {
		runtimeSnapshot, projected = projectAgentRuntime(operation.Context(), executionRuntime, agentrun.Options{
			AgentKind: agentrun.AgentKindInteractiveStory,
			ProjectID: projectID,
			Workspace: workspace,
			StoryID:   storyID,
			BranchID:  resolved,
		})
	}
	pendingInterruptionID := ""
	if resolved != "" && store != nil {
		pending, pendingErr := store.PendingTurnInterruption(storyID, resolved)
		if pendingErr != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("[agent-runtime-projection] load paused game turn failed workspace=%s story_id=%s branch_id=%s err=%v", workspace, storyID, resolved, pendingErr))
		} else if pending != nil {
			pendingInterruptionID = strings.TrimSpace(pending.ID)
		}
	}

	a.mu.RLock()
	currentTask, currentInfo := activeInteractiveTaskLocked(a, storyID, branchID)
	if lifecycleWorkspaceKey(a.workspace) != lifecycleWorkspaceKey(workspace) || a.executionRuntime != executionRuntime || a.interactive != store || currentTask != task || !sameInteractiveTaskInfo(currentInfo, info) {
		a.mu.RUnlock()
		return InteractiveAgentActiveView{}
	}
	var taskSnapshot *apptask.Snapshot
	if task != nil {
		snapshot := task.Snapshot()
		taskSnapshot = &snapshot
	}
	a.mu.RUnlock()
	return InteractiveAgentActiveView{
		Task: taskSnapshot, Info: info, Runtime: runtimeSnapshot, RuntimeProjectionOK: projected,
		PendingInterruptionID: pendingInterruptionID,
	}
}

func sameInteractiveTaskInfo(left, right InteractiveTaskInfo) bool {
	if left.TaskID != right.TaskID || left.CommandID != right.CommandID || left.ProjectID != right.ProjectID || left.Workspace != right.Workspace ||
		left.StoryID != right.StoryID || left.BranchID != right.BranchID || left.Message != right.Message ||
		left.RegenerateFromTurnID != right.RegenerateFromTurnID || len(left.Attachments) != len(right.Attachments) {
		return false
	}
	for index := range left.Attachments {
		if left.Attachments[index] != right.Attachments[index] {
			return false
		}
	}
	return true
}

// Projection-only methods remain useful to non-active callers while sharing
// the same immutable view construction as the HTTP active endpoints.
func (a *App) WritingAgentRuntimeProjection(ctx context.Context) (agentrun.RuntimeStatus, bool) {
	view := a.WritingAgentActiveView(ctx)
	return view.Runtime, view.RuntimeProjectionOK
}

func (a *App) InteractiveAgentRuntimeProjection(ctx context.Context, storyID, branchID string) (agentrun.RuntimeStatus, bool) {
	view := a.InteractiveAgentActiveView(ctx, storyID, branchID)
	return view.Runtime, view.RuntimeProjectionOK
}

func activeWritingTaskLocked(a *App) *apptask.Task {
	if a == nil || (a.activeWritingRun != nil && !a.activeWritingRun.matchesCurrent(a)) {
		return nil
	}
	return a.activeTask
}

func activeInteractiveTaskLocked(a *App, storyID, branchID string) (*apptask.Task, InteractiveTaskInfo) {
	if a == nil || a.activeInteractiveRun == nil || a.activeInteractiveRun.task == nil {
		return nil, InteractiveTaskInfo{}
	}
	run := a.activeInteractiveRun
	if run.info.Workspace == "" || run.info.Workspace != a.workspace || (storyID != "" && run.info.StoryID != storyID) || (branchID != "" && run.info.BranchID != branchID) {
		return nil, InteractiveTaskInfo{}
	}
	return run.task, run.info
}

func resolveInteractiveProjectionBranch(store *interactive.Store, storyID, requestedBranch string) (string, error) {
	if store == nil {
		return "", ErrNoWorkspace
	}
	branches, err := store.Branches(storyID)
	if err != nil {
		return "", err
	}
	for _, branch := range branches {
		if requestedBranch != "" && branch.ID == requestedBranch {
			return strings.TrimSpace(branch.ID), nil
		}
		if requestedBranch == "" && branch.Current {
			return strings.TrimSpace(branch.ID), nil
		}
	}
	if requestedBranch != "" {
		return "", errors.New("interactive story branch does not exist")
	}
	return "", errors.New("interactive story has no current branch")
}

func projectAgentRuntime(ctx context.Context, executionRuntime *agentexecution.Runtime, options agentrun.Options) (agentrun.RuntimeStatus, bool) {
	return appagentruntime.RuntimeProjection(ctx, executionRuntime, options)
}
