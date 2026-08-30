package app

import (
	"context"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	appagentruntime "denova/internal/app/agentruntime"
	apptask "denova/internal/app/task"
	"fmt"
	"log/slog"
	"strings"
)

type AgentRuntimeRecoveryRequest = appagentruntime.RecoveryRequest
type AgentRuntimeRecoveryResult = appagentruntime.RecoveryResult

func recoveryActionKey(action agentexecution.RuntimeRecoveryAction) string {
	return appagentruntime.RecoveryActionKey(action)
}

func validateSelectedRecoveryAction(status agentrun.RuntimeStatus, selected agentexecution.RuntimeRecoveryAction) error {
	return appagentruntime.ValidateRecoveryAction(status, selected)
}

func (a *App) RecoverWritingAgent(ctx context.Context, request AgentRuntimeRecoveryRequest) (AgentRuntimeRecoveryResult, error) {
	return a.chat().RecoverAgentRuntime(ctx, "", request)
}

// RecoverWritingAgentForSession binds recovery to the exact foreground
// Writing Session selected when the browser requested the projection.
func (a *App) RecoverWritingAgentForSession(ctx context.Context, sessionID string, request AgentRuntimeRecoveryRequest) (AgentRuntimeRecoveryResult, error) {
	return a.chat().RecoverAgentRuntime(ctx, strings.TrimSpace(sessionID), request)
}

func (s *ChatAppService) RecoverAgentRuntime(ctx context.Context, expectedSessionID string, request AgentRuntimeRecoveryRequest) (AgentRuntimeRecoveryResult, error) {
	if s == nil || s.app == nil {
		return AgentRuntimeRecoveryResult{}, ErrNoWorkspace
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	a := s.app
	a.mu.RLock()
	workspace := strings.TrimSpace(a.workspace)
	sess := a.session
	executionRuntime := a.executionRuntime
	bookService := a.bookService
	existing := a.activeWritingRun
	stateRoot := ""
	projectID := ""
	if a.cfg != nil {
		stateRoot = a.cfg.ProjectStateDir
		projectID = a.cfg.ProjectID
	}
	a.mu.RUnlock()
	if expectedSessionID != "" && (sess == nil || strings.TrimSpace(sess.ID) != expectedSessionID) {
		return AgentRuntimeRecoveryResult{}, ErrAgentContextChanged
	}
	if workspace == "" || sess == nil || executionRuntime == nil {
		return AgentRuntimeRecoveryResult{}, ErrNoWorkspace
	}
	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	defer operation.Release()
	if existing != nil && existing.task != nil && existing.recovery != nil && existing.runtime.workspace == workspace && existing.runtime.sess == sess {
		current, currentErr := finishedRecoveryActionStillCurrent(operation.Context(), existing.task, existing.recovery, request.Action)
		if currentErr != nil {
			return AgentRuntimeRecoveryResult{}, currentErr
		}
		if !current {
			return resumeExistingWritingRecovery(operation.Context(), existing, request.Action)
		}
	}
	if existing != nil && existing.task != nil && !existing.task.Finished() {
		return AgentRuntimeRecoveryResult{}, ErrAgentOperationActive
	}

	runtime := ideChatRuntime{
		app: a, projectID: projectID, sess: sess, bookService: bookService,
		executionRuntime: executionRuntime, workspace: workspace, projectState: stateRoot,
	}
	options := runtime.agentOptions("")
	recovery, err := executionRuntime.OpenRecoveryObservation(operation.Context(), options)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	if err := validateSelectedRecoveryAction(recovery.InitialStatus(), request.Action); err != nil {
		recovery.Close()
		return AgentRuntimeRecoveryResult{}, err
	}
	key := recoveryActionKey(request.Action)
	var run *writingTaskRun
	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.workspaceTransition {
			return ErrWorkspaceTransition
		}
		if a.workspace != workspace || a.session != sess || a.executionRuntime != executionRuntime {
			return ErrAgentContextChanged
		}
		if a.activeWritingRun != existing && a.activeWritingRun != nil && a.activeWritingRun.task != nil && !a.activeWritingRun.task.Finished() {
			return ErrAgentOperationActive
		}
		if err := a.registerWorkspaceTaskLocked(task, workspace, true); err != nil {
			return err
		}
		run = &writingTaskRun{
			task: task, runtime: runtime, recovery: recovery,
			recoveryActions: make(map[string]agentrun.CommandReceipt),
		}
		a.activeTask = task
		a.activeWritingRun = run
		return nil
	})
	if err != nil {
		recovery.Close()
		return AgentRuntimeRecoveryResult{}, err
	}
	var receipt agentrun.CommandReceipt
	receipt, err = recovery.Resume(operation.Context(), request.Action, task.ID(), task.Emit)
	if err != nil {
		recovery.Close()
		rollbackWritingAttachTask(a, task, err)
		return AgentRuntimeRecoveryResult{}, err
	}
	run.recoveryActions[key] = receipt
	if err := task.Start(func(taskCtx context.Context, task *apptask.Task, emit func(agentrun.Event)) {
		defer a.unregisterWorkspaceTask(task)
		defer recovery.Close()
		outcome := recovery.Wait(taskCtx, emit)
		run.flushRecoveryMutations(taskCtx)
		slog.InfoContext(taskCtx, fmt.Sprintf("[agent-recovery] writing task settled task_id=%s action=%s command_id=%s operation_id=%s outcome=%s", task.ID(), request.Action.Kind, request.Action.CommandID, request.Action.OperationID, outcome.Status))
	}); err != nil {
		recovery.Close()
		rollbackWritingAttachTask(a, task, err)
		return AgentRuntimeRecoveryResult{}, err
	}
	return AgentRuntimeRecoveryResult{Task: task, Action: request.Action, Receipt: receipt}, nil
}

func resumeExistingWritingRecovery(ctx context.Context, run *writingTaskRun, action agentexecution.RuntimeRecoveryAction) (AgentRuntimeRecoveryResult, error) {
	key := recoveryActionKey(action)
	if receipt, ok := run.recoveryActions[key]; ok {
		return AgentRuntimeRecoveryResult{Task: run.task, Action: action, Receipt: receipt}, nil
	}
	if run.task.Finished() {
		return AgentRuntimeRecoveryResult{}, fmt.Errorf("%w: recovery display task is already settled", agentexecution.ErrRecoveryActionChanged)
	}
	receipt, err := run.recovery.Resume(ctx, action, run.task.ID(), run.task.Emit)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	run.recoveryActions[key] = receipt
	return AgentRuntimeRecoveryResult{Task: run.task, Action: action, Receipt: receipt}, nil
}

func (a *App) RecoverInteractiveAgent(ctx context.Context, request AgentRuntimeRecoveryRequest) (AgentRuntimeRecoveryResult, error) {
	return a.interactiveService().RecoverAgentRuntime(ctx, request)
}

func (s *InteractiveAppService) RecoverAgentRuntime(ctx context.Context, request AgentRuntimeRecoveryRequest) (AgentRuntimeRecoveryResult, error) {
	if s == nil || s.app == nil {
		return AgentRuntimeRecoveryResult{}, ErrNoWorkspace
	}
	request.StoryID = strings.TrimSpace(request.StoryID)
	request.BranchID = strings.TrimSpace(request.BranchID)
	if request.StoryID == "" {
		return AgentRuntimeRecoveryResult{}, fmt.Errorf("interactive story is required")
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	a := s.app
	a.mu.RLock()
	workspace := strings.TrimSpace(a.workspace)
	projectID := ""
	if a.cfg != nil {
		projectID = strings.TrimSpace(a.cfg.ProjectID)
	}
	executionRuntime := a.executionRuntime
	store := a.interactive
	existing := a.activeInteractiveRun
	a.mu.RUnlock()
	if workspace == "" || executionRuntime == nil || store == nil {
		return AgentRuntimeRecoveryResult{}, ErrNoWorkspace
	}
	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	defer operation.Release()
	branchID, err := resolveInteractiveProjectionBranch(store, request.StoryID, request.BranchID)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	if existing != nil && existing.task != nil && existing.recovery != nil && existing.info.Workspace == workspace && existing.info.StoryID == request.StoryID && existing.info.BranchID == branchID {
		current, currentErr := finishedRecoveryActionStillCurrent(operation.Context(), existing.task, existing.recovery, request.Action)
		if currentErr != nil {
			return AgentRuntimeRecoveryResult{}, currentErr
		}
		if !current {
			return resumeExistingInteractiveRecovery(operation.Context(), existing, request.Action)
		}
	}
	if existing != nil && existing.task != nil && !existing.task.Finished() {
		return AgentRuntimeRecoveryResult{}, ErrAgentOperationActive
	}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, ProjectID: projectID, Workspace: workspace,
		StoryID: request.StoryID, BranchID: branchID, Mode: "interactive",
	}
	recovery, err := executionRuntime.OpenRecoveryObservation(operation.Context(), options)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	if err := validateSelectedRecoveryAction(recovery.InitialStatus(), request.Action); err != nil {
		recovery.Close()
		return AgentRuntimeRecoveryResult{}, err
	}
	display, err := recovery.DisplayMetadata(operation.Context(), request.Action)
	if err != nil {
		recovery.Close()
		return AgentRuntimeRecoveryResult{}, err
	}
	info := InteractiveTaskInfo{
		CommandID: string(request.Action.CommandID), ProjectID: projectID, Workspace: workspace,
		StoryID: request.StoryID, BranchID: branchID,
		Message: display.Message, RegenerateFromTurnID: display.RegenerateFromTurnID,
		Attachments: display.Attachments,
	}
	var run *interactiveTaskRun
	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.workspaceTransition || a.workspace != workspace || a.interactive != store || a.executionRuntime != executionRuntime {
			return ErrAgentContextChanged
		}
		if a.activeInteractiveRun != existing && a.activeInteractiveRun != nil && a.activeInteractiveRun.task != nil && !a.activeInteractiveRun.task.Finished() {
			return ErrAgentOperationActive
		}
		if err := a.registerWorkspaceTaskLocked(task, workspace, true); err != nil {
			return err
		}
		info.TaskID = task.ID()
		run = &interactiveTaskRun{
			task: task, info: info, recovery: recovery,
			recoveryActions: make(map[string]agentrun.CommandReceipt),
		}
		a.activeInteractiveRun = run
		return nil
	})
	if err != nil {
		recovery.Close()
		return AgentRuntimeRecoveryResult{}, err
	}
	key := recoveryActionKey(request.Action)
	var receipt agentrun.CommandReceipt
	receipt, err = recovery.Resume(operation.Context(), request.Action, task.ID(), task.Emit)
	if err != nil {
		recovery.Close()
		rollbackInteractiveAttachTask(a, task, err)
		return AgentRuntimeRecoveryResult{}, err
	}
	run.recoveryActions[key] = receipt
	if err := task.Start(func(taskCtx context.Context, task *apptask.Task, emit func(agentrun.Event)) {
		defer a.unregisterWorkspaceTask(task)
		defer recovery.Close()
		outcome := recovery.Wait(taskCtx, emit)
		slog.InfoContext(taskCtx, fmt.Sprintf("[agent-recovery] game task settled task_id=%s story_id=%s branch_id=%s action=%s command_id=%s operation_id=%s outcome=%s", task.ID(), request.StoryID, branchID, request.Action.Kind, request.Action.CommandID, request.Action.OperationID, outcome.Status))
	}); err != nil {
		recovery.Close()
		rollbackInteractiveAttachTask(a, task, err)
		return AgentRuntimeRecoveryResult{}, err
	}
	return AgentRuntimeRecoveryResult{Task: task, Action: request.Action, Receipt: receipt}, nil
}

func resumeExistingInteractiveRecovery(ctx context.Context, run *interactiveTaskRun, action agentexecution.RuntimeRecoveryAction) (AgentRuntimeRecoveryResult, error) {
	key := recoveryActionKey(action)
	if receipt, ok := run.recoveryActions[key]; ok {
		return AgentRuntimeRecoveryResult{Task: run.task, Action: action, Receipt: receipt}, nil
	}
	if run.task.Finished() {
		return AgentRuntimeRecoveryResult{}, fmt.Errorf("%w: recovery display task is already settled", agentexecution.ErrRecoveryActionChanged)
	}
	receipt, err := run.recovery.Resume(ctx, action, run.task.ID(), run.task.Emit)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	run.recoveryActions[key] = receipt
	return AgentRuntimeRecoveryResult{Task: run.task, Action: action, Receipt: receipt}, nil
}

func rollbackWritingAttachTask(a *App, task *apptask.Task, err error) {
	task.RejectStart(err)
	a.unregisterWorkspaceTask(task)
	a.mu.Lock()
	if a.activeTask == task {
		a.activeTask = nil
	}
	if a.activeWritingRun != nil && a.activeWritingRun.task == task {
		a.activeWritingRun = nil
	}
	a.mu.Unlock()
}

func rollbackInteractiveAttachTask(a *App, task *apptask.Task, err error) {
	task.RejectStart(err)
	a.unregisterWorkspaceTask(task)
	a.mu.Lock()
	if a.activeInteractiveRun != nil && a.activeInteractiveRun.task == task {
		a.activeInteractiveRun = nil
	}
	a.mu.Unlock()
}

// finishedRecoveryActionStillCurrent distinguishes a repeated display attach
// from a new attach after the previous display task failed.
func finishedRecoveryActionStillCurrent(
	ctx context.Context,
	task *apptask.Task,
	recovery *agentexecution.RecoveryObservation,
	action agentexecution.RuntimeRecoveryAction,
) (bool, error) {
	return appagentruntime.FinishedRecoveryActionStillCurrent(ctx, task, recovery, action)
}
