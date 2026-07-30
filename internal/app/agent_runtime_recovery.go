package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	agents "denova/internal/agents"
)

type AgentRuntimeRecoveryRequest struct {
	Action   agents.RuntimeRecoveryAction
	StoryID  string
	BranchID string
}

type AgentRuntimeRecoveryResult struct {
	Task    *Task
	Action  agents.RuntimeRecoveryAction
	Receipt agents.CommandReceipt
}

func recoveryActionKey(action agents.RuntimeRecoveryAction) string {
	return strings.Join([]string{string(action.Kind), string(action.CommandID), string(action.OperationID)}, "\x00")
}

func validateSelectedRecoveryAction(status agents.RuntimeStatus, selected agents.RuntimeRecoveryAction) error {
	for _, action := range agents.RuntimeRecoveryActions(status) {
		if action == selected {
			return nil
		}
	}
	return fmt.Errorf(
		"%w: kind=%q command_id=%q operation_id=%q",
		agents.ErrRecoveryActionChanged,
		selected.Kind,
		selected.CommandID,
		selected.OperationID,
	)
}

func recoveryStructuralAction(kind agents.RuntimeRecoveryActionKind) (agents.ContextStructuralAction, bool) {
	switch kind {
	case agents.RuntimeRecoveryCompactContext:
		return agents.ContextStructuralCompact, true
	case agents.RuntimeRecoveryRemoveCompaction:
		return agents.ContextStructuralRemove, true
	default:
		return "", false
	}
}

func (a *App) RecoverWritingAgent(ctx context.Context, request AgentRuntimeRecoveryRequest) (AgentRuntimeRecoveryResult, error) {
	return a.chat().RecoverAgentRuntime(ctx, request)
}

func (s *ChatAppService) RecoverAgentRuntime(ctx context.Context, request AgentRuntimeRecoveryRequest) (AgentRuntimeRecoveryResult, error) {
	if s == nil || s.app == nil {
		return AgentRuntimeRecoveryResult{}, ErrNoWorkspace
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	a := s.app
	a.mu.RLock()
	workspace := strings.TrimSpace(a.workspace)
	sess := a.session
	chatService := a.chatService
	bookService := a.bookService
	existing := a.activeWritingRun
	stateRoot := ""
	if a.cfg != nil {
		stateRoot = a.cfg.ProjectStateDir
	}
	a.mu.RUnlock()
	if workspace == "" || sess == nil || chatService == nil {
		return AgentRuntimeRecoveryResult{}, ErrNoWorkspace
	}
	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	defer operation.Release()
	if existing != nil && existing.task != nil && existing.recovery != nil && existing.runtime.workspace == workspace && existing.runtime.sess == sess {
		if retried, refreshErr := s.retryRecoveryRefresh(operation.Context(), workspace, sess.ID, request.Action, sess.RefreshCanonical); retried {
			if refreshErr != nil {
				return AgentRuntimeRecoveryResult{}, refreshErr
			}
			key := recoveryActionKey(request.Action)
			receipt, ok := existing.recoveryActions[key]
			if !ok {
				return AgentRuntimeRecoveryResult{}, fmt.Errorf("%w: recovered structural receipt is unavailable", agents.ErrRecoveryActionChanged)
			}
			existing.resolveRecoveryRefresh()
			return AgentRuntimeRecoveryResult{Task: existing.task, Action: request.Action, Receipt: receipt}, nil
		}
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

	options := agents.RunOptions{AgentKind: agents.AgentKindIDE, StateRoot: stateRoot, Workspace: workspace, SessionID: sess.ID, Mode: "ide"}
	recovery, err := chatService.OpenRecoveryObservation(operation.Context(), options)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	if err := validateSelectedRecoveryAction(recovery.InitialStatus(), request.Action); err != nil {
		recovery.Close()
		return AgentRuntimeRecoveryResult{}, err
	}
	if _, err := reconcileColdPendingAsk(operation.Context(), sess, recovery.InitialStatus()); err != nil {
		recovery.Close()
		return AgentRuntimeRecoveryResult{}, fmt.Errorf("reconcile orphaned Ask before writing recovery: %w", err)
	}
	runtime := ideChatRuntime{app: a, sess: sess, bookService: bookService, chatService: chatService, workspace: workspace, projectState: stateRoot}
	key := recoveryActionKey(request.Action)
	structural, isStructural := recoveryStructuralAction(request.Action.Kind)
	var run *writingTaskRun
	task, err := NewDeferredRegisteredTask(func(task *Task) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.workspaceTransition {
			return ErrWorkspaceTransition
		}
		if a.workspace != workspace || a.session != sess || a.chatService != chatService {
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
			recoveryActions:      make(map[string]agents.CommandReceipt),
			recoveryStructural:   isStructural,
			recoveryRefreshReady: make(chan struct{}),
		}
		a.activeTask = task
		a.activeWritingRun = run
		return nil
	})
	if err != nil {
		recovery.Close()
		return AgentRuntimeRecoveryResult{}, err
	}
	var receipt agents.CommandReceipt
	if !isStructural {
		receipt, err = recovery.Resume(operation.Context(), request.Action, task.ID(), task.emit)
		if err != nil {
			recovery.Close()
			rollbackWritingReplayTask(a, task, err)
			return AgentRuntimeRecoveryResult{}, err
		}
		run.recoveryActions[key] = receipt
	} else {
		run.recoveryActions[key] = agents.CommandReceipt{
			CommandID: request.Action.CommandID, OperationID: request.Action.OperationID,
			Cursor: recovery.InitialStatus().Cursor, Replayed: true,
		}
		receipt = run.recoveryActions[key]
	}
	if err := task.Start(func(taskCtx context.Context, task *Task, emit func(agents.Event)) {
		defer a.unregisterWorkspaceTask(task)
		defer recovery.Close()
		if isStructural {
			_, resumed, resumeErr := chatService.ResumeRecoveredContextStructuralOperation(taskCtx, options, structural)
			if resumed {
				s.markRecoveryRefreshPending(workspace, sess.ID, request.Action)
			}
			if resumeErr != nil {
				emit(agents.Event{Type: "error", Data: map[string]string{"message": resumeErr.Error()}})
				return
			} else if !resumed {
				emit(agents.Event{Type: "error", Data: map[string]string{"message": "Agent recovery action changed"}})
				return
			}
			if _, refreshErr := s.retryRecoveryRefresh(taskCtx, workspace, sess.ID, request.Action, sess.RefreshCanonical); refreshErr != nil {
				log.Printf("[agent-recovery] recovered structural session refresh pending task_id=%s workspace=%s session_id=%s command_id=%s operation_id=%s err=%v", task.ID(), workspace, sess.ID, request.Action.CommandID, request.Action.OperationID, refreshErr)
				emitWritingRecoveryRefreshRequired(emit, request.Action, recovery.InitialStatus().Cursor)
				if !run.waitForRecoveryRefresh(taskCtx) {
					return
				}
			}
		}
		outcome := recovery.Wait(taskCtx, emit)
		run.flushRecoveryMutations(taskCtx)
		log.Printf("[agent-recovery] writing task settled task_id=%s action=%s command_id=%s operation_id=%s outcome=%s", task.ID(), request.Action.Kind, request.Action.CommandID, request.Action.OperationID, outcome.Status)
	}); err != nil {
		recovery.Close()
		rollbackWritingReplayTask(a, task, err)
		return AgentRuntimeRecoveryResult{}, err
	}
	return AgentRuntimeRecoveryResult{Task: task, Action: request.Action, Receipt: receipt}, nil
}

func resumeExistingWritingRecovery(ctx context.Context, run *writingTaskRun, action agents.RuntimeRecoveryAction) (AgentRuntimeRecoveryResult, error) {
	key := recoveryActionKey(action)
	if receipt, ok := run.recoveryActions[key]; ok {
		return AgentRuntimeRecoveryResult{Task: run.task, Action: action, Receipt: receipt}, nil
	}
	if run.task.Finished() {
		return AgentRuntimeRecoveryResult{}, fmt.Errorf("%w: recovery display task is already settled", agents.ErrRecoveryActionChanged)
	}
	if _, structural := recoveryStructuralAction(action.Kind); structural {
		return AgentRuntimeRecoveryResult{}, fmt.Errorf("%w: a structural recovery action cannot join an existing display task", agents.ErrRecoveryActionChanged)
	}
	receipt, err := run.recovery.Resume(ctx, action, run.task.ID(), run.task.emit)
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
	chatService := a.chatService
	store := a.interactive
	existing := a.activeInteractiveRun
	a.mu.RUnlock()
	if workspace == "" || chatService == nil || store == nil {
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
	options := agents.RunOptions{
		AgentKind: agents.AgentKindInteractiveStory, Workspace: workspace,
		StoryID: request.StoryID, BranchID: branchID, Mode: "interactive",
	}
	recovery, err := chatService.OpenRecoveryObservation(operation.Context(), options)
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
		CommandID: string(request.Action.CommandID), Workspace: workspace,
		StoryID: request.StoryID, BranchID: branchID,
		Message: display.Message, RegenerateFromTurnID: display.RegenerateFromTurnID,
	}
	var run *interactiveTaskRun
	task, err := NewDeferredRegisteredTask(func(task *Task) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.workspaceTransition || a.workspace != workspace || a.interactive != store || a.chatService != chatService {
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
			recoveryActions: make(map[string]agents.CommandReceipt),
		}
		a.activeInteractiveRun = run
		return nil
	})
	if err != nil {
		recovery.Close()
		return AgentRuntimeRecoveryResult{}, err
	}
	key := recoveryActionKey(request.Action)
	structural, isStructural := recoveryStructuralAction(request.Action.Kind)
	var receipt agents.CommandReceipt
	if !isStructural {
		receipt, err = recovery.Resume(operation.Context(), request.Action, task.ID(), task.emit)
		if err != nil {
			recovery.Close()
			rollbackInteractiveReplayTask(a, task, err)
			return AgentRuntimeRecoveryResult{}, err
		}
		run.recoveryActions[key] = receipt
	} else {
		receipt = agents.CommandReceipt{
			CommandID: request.Action.CommandID, OperationID: request.Action.OperationID,
			Cursor: recovery.InitialStatus().Cursor, Replayed: true,
		}
		run.recoveryActions[key] = receipt
	}
	if err := task.Start(func(taskCtx context.Context, task *Task, emit func(agents.Event)) {
		defer a.unregisterWorkspaceTask(task)
		defer recovery.Close()
		if isStructural {
			if _, resumed, resumeErr := chatService.ResumeRecoveredContextStructuralOperation(taskCtx, options, structural); resumeErr != nil {
				emit(agents.Event{Type: "error", Data: map[string]string{"message": resumeErr.Error()}})
				return
			} else if !resumed {
				emit(agents.Event{Type: "error", Data: map[string]string{"message": "Agent recovery action changed"}})
				return
			}
		}
		outcome := recovery.Wait(taskCtx, emit)
		log.Printf("[agent-recovery] game task settled task_id=%s story_id=%s branch_id=%s action=%s command_id=%s operation_id=%s outcome=%s", task.ID(), request.StoryID, branchID, request.Action.Kind, request.Action.CommandID, request.Action.OperationID, outcome.Status)
	}); err != nil {
		recovery.Close()
		rollbackInteractiveReplayTask(a, task, err)
		return AgentRuntimeRecoveryResult{}, err
	}
	return AgentRuntimeRecoveryResult{Task: task, Action: request.Action, Receipt: receipt}, nil
}

func resumeExistingInteractiveRecovery(ctx context.Context, run *interactiveTaskRun, action agents.RuntimeRecoveryAction) (AgentRuntimeRecoveryResult, error) {
	key := recoveryActionKey(action)
	if receipt, ok := run.recoveryActions[key]; ok {
		return AgentRuntimeRecoveryResult{Task: run.task, Action: action, Receipt: receipt}, nil
	}
	if run.task.Finished() {
		return AgentRuntimeRecoveryResult{}, fmt.Errorf("%w: recovery display task is already settled", agents.ErrRecoveryActionChanged)
	}
	if _, structural := recoveryStructuralAction(action.Kind); structural {
		return AgentRuntimeRecoveryResult{}, fmt.Errorf("%w: a structural recovery action cannot join an existing display task", agents.ErrRecoveryActionChanged)
	}
	receipt, err := run.recovery.Resume(ctx, action, run.task.ID(), run.task.emit)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	run.recoveryActions[key] = receipt
	return AgentRuntimeRecoveryResult{Task: run.task, Action: action, Receipt: receipt}, nil
}

// finishedRecoveryActionStillCurrent distinguishes an idempotent replay of a
// settled action from retrying work whose previous display task failed before
// changing durable state. The latter must receive a fresh task/observer.
func finishedRecoveryActionStillCurrent(
	ctx context.Context,
	task *Task,
	recovery *agents.RecoveryObservation,
	action agents.RuntimeRecoveryAction,
) (bool, error) {
	if task == nil || recovery == nil || !task.Finished() {
		return false, nil
	}
	status, err := recovery.CurrentStatus(ctx)
	if err != nil {
		return false, err
	}
	for _, current := range agents.RuntimeRecoveryActions(status) {
		if current == action {
			return true, nil
		}
	}
	return false, nil
}
