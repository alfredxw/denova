package app

import (
	"context"
	agentharness "denova/internal/agents/harness"
	apptask "denova/internal/app/task"
	"errors"
	"fmt"
	"strings"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/interactive"
)

// writingStructuralFence is the immutable admission snapshot used by session
// mutations. Slow cancellation and actor shutdown happen without App.mu; the
// caller must validate the fence again under App.mu immediately before write.
type writingStructuralFence struct {
	workspace           string
	stateRoot           string
	workspaceGeneration uint64
	store               *session.Store
	selected            *session.Session
	chat                *agentharness.Service
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
		chat:                a.chatService,
		sessionID:           sessionID,
	}
	if a.cfg != nil {
		fence.stateRoot = a.cfg.ProjectStateDir
	}
	fence.task = writingTaskForSessionLocked(a, fence.workspace, sessionID)
	a.mu.RUnlock()
	if sessionID == "" {
		return writingStructuralFence{}, ErrNoWorkspace
	}
	if err := abortAndWaitTask(ctx, fence.task); err != nil {
		return writingStructuralFence{}, err
	}
	if err := s.retryPendingWritingRecoveryRefresh(ctx, fence.workspace, fence.selected); err != nil {
		return writingStructuralFence{}, err
	}
	if err := closeAgentBindings(fence.chat, func(chat *agentharness.Service) error {
		return chat.CloseSessionBindings(ctx, agentrun.AgentKindIDE, fence.workspace, sessionID)
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
	// Preserve safe behavior for tests and legacy registrations that predate
	// writingTaskRun: the only global writing task belongs to the selected
	// session while that selection has not changed.
	if a.activeTask != nil && !a.activeTask.Finished() && a.workspace == workspace && a.session != nil && a.session.ID == sessionID {
		return a.activeTask
	}
	return nil
}

func (f writingStructuralFence) validateLocked(a *App, requireSelected bool) error {
	if a == nil || a.workspace != f.workspace || a.workspaceGeneration != f.workspaceGeneration || a.sessionStore != f.store || a.chatService != f.chat {
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
	workspace           string
	workspaceGeneration uint64
	store               *interactive.Store
	chat                *agentharness.Service
	directorTasks       *workspaceDirectorTaskGroup
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
		chat:                a.chatService,
		directorTasks:       a.workspaceDirectorTasks,
		storyID:             storyID,
		branchID:            branchID,
	}
	fence.task = interactiveTaskForScopeLocked(a, fence.workspace, storyID, branchID)
	a.mu.RUnlock()

	if err := abortAndWaitTask(ctx, fence.task); err != nil {
		return interactiveStructuralFence{}, err
	}
	closeStoryBindings := func(chat *agentharness.Service) error {
		return chat.CloseStoryBindings(ctx, fence.workspace, storyID, branchID)
	}
	if err := closeAgentBindings(fence.chat, closeStoryBindings); err != nil {
		return interactiveStructuralFence{}, err
	}
	if err := fence.waitDirectorMaintenance(ctx); err != nil {
		return interactiveStructuralFence{}, err
	}
	// A maintenance item that was queued but had not opened its Director actor
	// can do so after the first scope close releases. Evict once more after the
	// queue drains so the returned fence has no late-open binding behind it.
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

func (f interactiveStructuralFence) waitDirectorMaintenance(ctx context.Context) error {
	if f.directorTasks == nil {
		return nil
	}
	branchIDs := []string{f.branchID}
	if f.branchID == "" {
		branches, err := f.store.Branches(f.storyID)
		if err != nil {
			return err
		}
		branchIDs = make([]string, 0, len(branches))
		for _, branch := range branches {
			branchIDs = append(branchIDs, branch.ID)
		}
	}
	for _, branchID := range branchIDs {
		key := f.storyID + ":" + strings.TrimSpace(branchID) + ":derived"
		if err := f.directorTasks.WaitKey(ctx, key); err != nil {
			return fmt.Errorf("wait interactive Director maintenance for %s/%s: %w", f.storyID, branchID, err)
		}
	}
	return nil
}

func (f interactiveStructuralFence) validateLocked(a *App) error {
	if a == nil || a.workspace != f.workspace || a.workspaceGeneration != f.workspaceGeneration || a.interactive != f.store || a.chatService != f.chat || a.workspaceDirectorTasks != f.directorTasks {
		return ErrAgentContextChanged
	}
	if task := interactiveTaskForScopeLocked(a, f.workspace, f.storyID, f.branchID); task != nil {
		return ErrAgentOperationActive
	}
	return nil
}

func abortAndWaitTask(ctx context.Context, task *apptask.Task) error {
	if task == nil {
		return nil
	}
	task.Abort()
	select {
	case <-task.Done():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func closeAgentBindings(chat *agentharness.Service, close func(*agentharness.Service) error) error {
	if chat == nil {
		return nil
	}
	err := close(chat)
	if err == nil || errors.Is(err, agentharness.ErrRuntimeProjectionUnavailable) {
		return nil
	}
	return fmt.Errorf("close Agent runtime binding: %w", err)
}
