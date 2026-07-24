package app

import (
	"context"
	"errors"
	"log"
	"strings"

	agents "denova/internal/agents"
	"denova/internal/interactive"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

type WritingAgentActiveView struct {
	Task                *TaskStateSnapshot
	Runtime             runstate.StatusSnapshot
	RuntimeProjectionOK bool
	// RecoveryActions can contain a process-local projection refresh action
	// after the durable runtime has already settled to Idle.
	RecoveryActions []agents.RuntimeRecoveryAction
}

type InteractiveAgentActiveView struct {
	Task                *TaskStateSnapshot
	Info                InteractiveTaskInfo
	Runtime             runstate.StatusSnapshot
	RuntimeProjectionOK bool
}

// WritingAgentActiveView returns task metadata and durable runtime state from
// one exact workspace/session generation. Task fields come from one Task lock.
func (a *App) WritingAgentActiveView(ctx context.Context) WritingAgentActiveView {
	if a == nil {
		return WritingAgentActiveView{}
	}
	chatApp := a.chat()
	a.mu.RLock()
	workspace := strings.TrimSpace(a.workspace)
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
	chatService := a.chatService
	selectedSession := a.session
	task := activeWritingTaskLocked(a)
	a.mu.RUnlock()
	sessionID := ""
	if selectedSession != nil {
		sessionID = strings.TrimSpace(selectedSession.ID)
	}
	var runtimeSnapshot runstate.StatusSnapshot
	projected := false
	if sessionID != "" && chatService != nil {
		runtimeSnapshot, projected = projectAgentRuntime(operation.Context(), chatService, agents.RunOptions{
			AgentKind: agents.AgentKindIDE,
			Workspace: workspace,
			SessionID: sessionID,
		})
	}
	var recoveryActions []agents.RuntimeRecoveryAction
	if action, pending := chatApp.pendingRecoveryRefreshAction(workspace, sessionID); pending {
		recoveryActions = []agents.RuntimeRecoveryAction{action}
		// The selected projection remains paused at the application boundary even
		// though the durable actor is already Idle: clients must exact-retry the
		// refresh before this Session can be used again.
		runtimeSnapshot.RecoveryPaused = true
		projected = true
	}

	a.mu.RLock()
	if lifecycleWorkspaceKey(a.workspace) != lifecycleWorkspaceKey(workspace) || a.chatService != chatService || a.session != selectedSession || activeWritingTaskLocked(a) != task {
		a.mu.RUnlock()
		return WritingAgentActiveView{}
	}
	var taskSnapshot *TaskStateSnapshot
	if task != nil {
		snapshot := task.Snapshot()
		taskSnapshot = &snapshot
	}
	a.mu.RUnlock()
	return WritingAgentActiveView{
		Task: taskSnapshot, Runtime: runtimeSnapshot, RuntimeProjectionOK: projected,
		RecoveryActions: recoveryActions,
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
	chatService := a.chatService
	store := a.interactive
	task, info := activeInteractiveTaskLocked(a, storyID, branchID)
	a.mu.RUnlock()
	projectionBranch := branchID
	if task != nil && strings.TrimSpace(info.BranchID) != "" {
		projectionBranch = info.BranchID
	}
	resolved, err := resolveInteractiveProjectionBranch(store, storyID, projectionBranch)
	if err != nil {
		log.Printf("[agent-runtime-projection] resolve game binding failed workspace=%s story_id=%s branch_id=%s err=%v", workspace, storyID, projectionBranch, err)
		resolved = ""
	}
	var runtimeSnapshot runstate.StatusSnapshot
	projected := false
	if resolved != "" && chatService != nil {
		runtimeSnapshot, projected = projectAgentRuntime(operation.Context(), chatService, agents.RunOptions{
			AgentKind: agents.AgentKindInteractiveStory,
			Workspace: workspace,
			StoryID:   storyID,
			BranchID:  resolved,
		})
	}

	a.mu.RLock()
	currentTask, currentInfo := activeInteractiveTaskLocked(a, storyID, branchID)
	if lifecycleWorkspaceKey(a.workspace) != lifecycleWorkspaceKey(workspace) || a.chatService != chatService || a.interactive != store || currentTask != task || currentInfo != info {
		a.mu.RUnlock()
		return InteractiveAgentActiveView{}
	}
	var taskSnapshot *TaskStateSnapshot
	if task != nil {
		snapshot := task.Snapshot()
		taskSnapshot = &snapshot
	}
	a.mu.RUnlock()
	return InteractiveAgentActiveView{Task: taskSnapshot, Info: info, Runtime: runtimeSnapshot, RuntimeProjectionOK: projected}
}

// Projection-only methods remain useful to non-active callers while sharing
// the same immutable view construction as the HTTP active endpoints.
func (a *App) WritingAgentRuntimeProjection(ctx context.Context) (runstate.StatusSnapshot, bool) {
	view := a.WritingAgentActiveView(ctx)
	return view.Runtime, view.RuntimeProjectionOK
}

func (a *App) InteractiveAgentRuntimeProjection(ctx context.Context, storyID, branchID string) (runstate.StatusSnapshot, bool) {
	view := a.InteractiveAgentActiveView(ctx, storyID, branchID)
	return view.Runtime, view.RuntimeProjectionOK
}

func activeWritingTaskLocked(a *App) *Task {
	if a == nil || (a.activeWritingRun != nil && !a.activeWritingRun.matchesCurrent(a)) {
		return nil
	}
	return a.activeTask
}

func activeInteractiveTaskLocked(a *App, storyID, branchID string) (*Task, InteractiveTaskInfo) {
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

func projectAgentRuntime(ctx context.Context, chatService *agents.ChatService, options agents.RunOptions) (runstate.StatusSnapshot, bool) {
	snapshot, err := chatService.RuntimeRecoveryStatusProjection(ctx, options)
	if err == nil {
		return snapshot, true
	}
	if !errors.Is(err, agents.ErrRuntimeProjectionUnavailable) {
		log.Printf("[agent-runtime-projection] projection unavailable kind=%s workspace=%s session_id=%s story_id=%s branch_id=%s err=%v", options.AgentKind, options.Workspace, options.SessionID, options.StoryID, options.BranchID, err)
	}
	return runstate.StatusSnapshot{}, false
}
