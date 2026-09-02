package app

import (
	"context"
	agentexecution "denova/internal/agents/execution"
	apptask "denova/internal/app/task"
	"fmt"
	"strings"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	appagentruntime "denova/internal/app/agentruntime"
	"denova/internal/interactive"
)

// writingStructuralFence is the immutable admission snapshot used by session
// mutations. Slow cancellation and actor shutdown happen without App.mu; the
// caller must validate the fence again under App.mu immediately before write.
type writingStructuralFence struct {
	projectID           string
	workspace           string
	stateRoot           string
	workspaceGeneration uint64
	store               *session.Store
	selected            *session.Session
	chat                *agentexecution.Runtime
	sessionID           string
	task                *apptask.Task
}

func (s *ChatAppService) drainWritingBinding(ctx context.Context, sessionID string) (writingStructuralFence, error) {
	if s == nil || s.app == nil {
		return writingStructuralFence{}, ErrNoWorkspace
	}
	if ctx == nil {
		ctx = context.Background()
	}
	a := s.app
	a.mu.RLock()
	if a.sessionStore == nil {
		a.mu.RUnlock()
		return writingStructuralFence{}, ErrNoWorkspace
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" && a.session != nil {
		sessionID = a.session.ID
	}
	fence := writingStructuralFence{
		workspace:           a.workspace,
		workspaceGeneration: a.workspaceGeneration,
		store:               a.sessionStore,
		selected:            a.session,
		chat:                a.executionRuntime,
		sessionID:           sessionID,
	}
	if a.cfg != nil {
		fence.projectID = strings.TrimSpace(a.cfg.ProjectID)
		fence.stateRoot = a.cfg.ProjectStoreDir
	}
	fence.task = writingTaskForSessionLocked(a, fence.workspace, sessionID)
	a.mu.RUnlock()
	if sessionID == "" {
		return writingStructuralFence{}, ErrNoWorkspace
	}
	if err := abortAndWaitTask(ctx, fence.task); err != nil {
		return writingStructuralFence{}, err
	}
	if err := closeAgentBindings(fence.chat, func(chat *agentexecution.Runtime) error {
		return chat.CloseSessionBindings(ctx, agentrun.AgentKindIDE, fence.projectID, sessionID)
	}); err != nil {
		return writingStructuralFence{}, err
	}
	return fence, nil
}

func writingTaskForSessionLocked(a *App, workspace, sessionID string) *apptask.Task {
	if a == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	if run := a.activeWritingRun; run != nil && run.task != nil &&
		run.runtime.workspace == workspace && run.runtime.sess != nil && run.runtime.sess.ID == sessionID && !run.task.Finished() {
		return run.task
	}
	return nil
}

func (f writingStructuralFence) validateLocked(a *App, requireSelected bool) error {
	if a == nil || a.workspace != f.workspace || a.workspaceGeneration != f.workspaceGeneration || a.sessionStore != f.store || a.executionRuntime != f.chat {
		return ErrAgentContextChanged
	}
	if requireSelected && a.session != f.selected {
		return ErrAgentContextChanged
	}
	if task := writingTaskForSessionLocked(a, f.workspace, f.sessionID); task != nil {
		return ErrAgentOperationActive
	}
	return nil
}

// interactiveStructuralFence applies the same barrier to a whole story or one
// branch. A blank branchID intentionally means the exact story scope.
type interactiveStructuralFence struct {
	projectID           string
	workspace           string
	workspaceGeneration uint64
	store               *interactive.Store
	chat                *agentexecution.Runtime
	storyID             string
	branchID            string
	task                *apptask.Task
}

func (s *InteractiveAppService) drainInteractiveBinding(ctx context.Context, storyID, branchID string) (interactiveStructuralFence, error) {
	if s == nil || s.app == nil {
		return interactiveStructuralFence{}, ErrNoWorkspace
	}
	if ctx == nil {
		ctx = context.Background()
	}
	storyID = strings.TrimSpace(storyID)
	branchID = strings.TrimSpace(branchID)
	if storyID == "" {
		return interactiveStructuralFence{}, fmt.Errorf("story id is required")
	}
	a := s.app
	a.mu.RLock()
	if a.interactive == nil {
		a.mu.RUnlock()
		return interactiveStructuralFence{}, ErrNoWorkspace
	}
	fence := interactiveStructuralFence{
		workspace:           a.workspace,
		workspaceGeneration: a.workspaceGeneration,
		store:               a.interactive,
		chat:                a.executionRuntime,
		storyID:             storyID,
		branchID:            branchID,
	}
	if a.cfg != nil {
		fence.projectID = strings.TrimSpace(a.cfg.ProjectID)
	}
	fence.task = interactiveTaskForScopeLocked(a, fence.workspace, storyID, branchID)
	a.mu.RUnlock()

	if err := abortAndWaitTask(ctx, fence.task); err != nil {
		return interactiveStructuralFence{}, err
	}
	closeStoryBindings := func(chat *agentexecution.Runtime) error {
		return chat.CloseStoryBindings(ctx, fence.projectID, storyID, branchID)
	}
	if err := closeAgentBindings(fence.chat, closeStoryBindings); err != nil {
		return interactiveStructuralFence{}, err
	}
	return fence, nil
}

func interactiveTaskForScopeLocked(a *App, workspace, storyID, branchID string) *apptask.Task {
	if a == nil || a.activeInteractiveRun == nil || a.activeInteractiveRun.task == nil || a.activeInteractiveRun.task.Finished() {
		return nil
	}
	run := a.activeInteractiveRun
	if run.info.Workspace != workspace || run.info.StoryID != storyID {
		return nil
	}
	if branchID != "" && run.info.BranchID != branchID {
		return nil
	}
	return run.task
}

func (f interactiveStructuralFence) validateLocked(a *App) error {
	if a == nil || a.workspace != f.workspace || a.workspaceGeneration != f.workspaceGeneration || a.interactive != f.store || a.executionRuntime != f.chat {
		return ErrAgentContextChanged
	}
	if task := interactiveTaskForScopeLocked(a, f.workspace, f.storyID, f.branchID); task != nil {
		return ErrAgentOperationActive
	}
	return nil
}

func abortAndWaitTask(ctx context.Context, task *apptask.Task) error {
	return appagentruntime.AbortAndWait(ctx, task)
}

func closeAgentBindings(chat *agentexecution.Runtime, close func(*agentexecution.Runtime) error) error {
	return appagentruntime.CloseBindings(chat, close)
}
