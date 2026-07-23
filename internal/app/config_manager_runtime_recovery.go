package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"denova/internal/agent"
	"denova/internal/agentruntime"
)

// ConfigManagerAgentActiveView binds a reconnectable display Task and the
// durable Runtime projection to one exact Config Manager scope.
type ConfigManagerAgentActiveView struct {
	Task                *TaskStateSnapshot
	CommandID           string
	StreamAttached      bool
	Runtime             agentruntime.StatusSnapshot
	RuntimeProjectionOK bool
}

func configManagerRunOptions(workspace, sessionID string) agent.RunOptions {
	return agent.RunOptions{
		AgentKind: agent.AgentKindConfigManager,
		Workspace: workspace,
		SessionID: sessionID,
		Mode:      "config_manager",
	}
}

func (a *App) ConfigManagerAgentActiveView(ctx context.Context, req ConfigManagerRequest) ConfigManagerAgentActiveView {
	return a.configManager().ActiveView(ctx, req)
}

func (s *ConfigManagerAppService) ActiveView(ctx context.Context, req ConfigManagerRequest) ConfigManagerAgentActiveView {
	if s == nil || s.app == nil {
		return ConfigManagerAgentActiveView{}
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	sessionID, err := configManagerSessionID(req)
	if err != nil {
		return ConfigManagerAgentActiveView{}
	}
	a := s.app
	a.mu.RLock()
	workspace := strings.TrimSpace(a.workspace)
	chatService := a.chatService
	a.mu.RUnlock()
	if workspace == "" || chatService == nil {
		return ConfigManagerAgentActiveView{}
	}
	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return ConfigManagerAgentActiveView{}
	}
	defer operation.Release()

	runtimeSnapshot, projected := projectConfigManagerRuntime(
		operation.Context(), chatService, configManagerRunOptions(workspace, sessionID),
	)
	record, recoverySelected := selectConfigManagerDisplayRecord(
		s.starts.latestConfigManagerTask(workspace, sessionID),
		s.recoveries.current(workspace, sessionID),
	)
	if recoverySelected {
		record.CommandID = configManagerRuntimeCommandID(runtimeSnapshot)
	}

	a.mu.RLock()
	stillCurrent := lifecycleWorkspaceKey(a.workspace) == lifecycleWorkspaceKey(workspace) && a.chatService == chatService
	a.mu.RUnlock()
	if !stillCurrent {
		return ConfigManagerAgentActiveView{}
	}
	var taskSnapshot *TaskStateSnapshot
	if record.Task != nil {
		snapshot := record.Task.Snapshot()
		taskSnapshot = &snapshot
	}
	return ConfigManagerAgentActiveView{
		Task: taskSnapshot, CommandID: record.CommandID,
		StreamAttached: configManagerDisplayOwnsRuntime(record, runtimeSnapshot),
		Runtime:        runtimeSnapshot, RuntimeProjectionOK: projected,
	}
}

// ConfigManagerDisplayTask resolves an already-authorized task ID within the
// exact Config Manager scope. It never falls back to another active workspace
// task, which prevents cross-panel stream attachment.
func (a *App) ConfigManagerDisplayTask(ctx context.Context, req ConfigManagerRequest, taskID string) *Task {
	return a.configManager().displayTask(ctx, req, taskID)
}

func (s *ConfigManagerAppService) displayTask(ctx context.Context, req ConfigManagerRequest, taskID string) *Task {
	if s == nil || s.app == nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	sessionID, err := configManagerSessionID(req)
	if err != nil {
		return nil
	}
	a := s.app
	a.mu.RLock()
	workspace := strings.TrimSpace(a.workspace)
	chatService := a.chatService
	a.mu.RUnlock()
	if workspace == "" || chatService == nil {
		return nil
	}
	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return nil
	}
	defer operation.Release()
	runtimeSnapshot, projected := projectConfigManagerRuntime(
		operation.Context(), chatService, configManagerRunOptions(workspace, sessionID),
	)
	if !projected {
		return nil
	}
	record, recoverySelected := selectConfigManagerDisplayRecord(
		s.starts.latestConfigManagerTask(workspace, sessionID),
		s.recoveries.current(workspace, sessionID),
	)
	if recoverySelected {
		record.CommandID = configManagerRuntimeCommandID(runtimeSnapshot)
	}
	task := record.Task
	if task != nil && task.ID() == taskID && configManagerDisplayOwnsRuntime(record, runtimeSnapshot) {
		return task
	}
	return nil
}

// configManagerRuntimeProjector keeps active inspection actor-free for the
// overwhelmingly common idle case. A cold unfinished journal is the only
// state that needs canonical reconciliation before it can expose recovery
// identities.
type configManagerRuntimeProjector interface {
	RuntimeStatusProjection(context.Context, agent.RunOptions) (agentruntime.StatusSnapshot, error)
	RuntimeRecoveryStatusProjection(context.Context, agent.RunOptions) (agentruntime.StatusSnapshot, error)
}

func projectConfigManagerRuntime(
	ctx context.Context,
	projector configManagerRuntimeProjector,
	options agent.RunOptions,
) (agentruntime.StatusSnapshot, bool) {
	if projector == nil {
		return agentruntime.StatusSnapshot{}, false
	}
	snapshot, err := projector.RuntimeStatusProjection(ctx, options)
	if err != nil {
		logConfigManagerProjectionError(options, err)
		return agentruntime.StatusSnapshot{}, false
	}
	if !snapshot.RecoveryPending {
		return snapshot, true
	}
	snapshot, err = projector.RuntimeRecoveryStatusProjection(ctx, options)
	if err != nil {
		logConfigManagerProjectionError(options, err)
		return agentruntime.StatusSnapshot{}, false
	}
	return snapshot, true
}

func logConfigManagerProjectionError(options agent.RunOptions, err error) {
	if errors.Is(err, agent.ErrRuntimeProjectionUnavailable) {
		return
	}
	log.Printf("[config-manager-runtime] projection unavailable workspace=%s session_id=%s err=%v", options.Workspace, options.SessionID, err)
}

// selectConfigManagerDisplayRecord chooses one process-local display owner for
// a scope. Recovery records intentionally outlive settlement for replay, so an
// older recovered Task must not shadow a newer normal run.
func selectConfigManagerDisplayRecord(
	start configManagerTaskRecord,
	recovery *configManagerRecoveryRun,
) (configManagerTaskRecord, bool) {
	if recovery == nil || recovery.task == nil {
		return start, false
	}
	if start.Task == nil || start.Task == recovery.task {
		return configManagerTaskRecord{Task: recovery.task}, true
	}
	startSnapshot := start.Task.Snapshot()
	recoverySnapshot := recovery.task.Snapshot()
	if startSnapshot.Finished != recoverySnapshot.Finished {
		if !startSnapshot.Finished {
			return start, false
		}
		return configManagerTaskRecord{Task: recovery.task}, true
	}
	if start.Task.startedAt.After(recovery.task.startedAt) {
		return start, false
	}
	return configManagerTaskRecord{Task: recovery.task}, true
}

func configManagerDisplayOwnsRuntime(record configManagerTaskRecord, runtime agentruntime.StatusSnapshot) bool {
	if record.Task == nil || record.Task.Finished() {
		return false
	}
	commandID := strings.TrimSpace(record.CommandID)
	if commandID == "" {
		return false
	}
	if string(runtime.ActiveCommandID) == commandID {
		return true
	}
	if runtime.ActiveStructural != nil && string(runtime.ActiveStructural.CommandID) == commandID {
		return true
	}
	if runtime.InputRecovery != nil && string(runtime.InputRecovery.CommandID) == commandID {
		return true
	}
	if runtime.Phase == agentruntime.PhaseIdle && runtime.LastOperation != nil && string(runtime.LastOperation.CommandID) == commandID {
		return true
	}
	return false
}

func configManagerRuntimeCommandID(runtime agentruntime.StatusSnapshot) string {
	if runtime.ActiveCommandID != "" {
		return string(runtime.ActiveCommandID)
	}
	if runtime.ActiveStructural != nil && runtime.ActiveStructural.CommandID != "" {
		return string(runtime.ActiveStructural.CommandID)
	}
	if runtime.LastOperation != nil {
		return string(runtime.LastOperation.CommandID)
	}
	return ""
}

func (a *App) RecoverConfigManagerAgent(
	ctx context.Context,
	scope ConfigManagerRequest,
	request AgentRuntimeRecoveryRequest,
) (AgentRuntimeRecoveryResult, error) {
	return a.configManager().RecoverAgentRuntime(ctx, scope, request)
}

// ClearConfigManagerSessionContext drains the exact display/runtime binding
// before appending the Session clear marker. This prevents a late recovered
// tool/model result from repopulating a scope the user has already cleared.
func (a *App) ClearConfigManagerSessionContext(ctx context.Context, req ConfigManagerRequest) error {
	return a.configManager().ClearContext(ctx, req)
}

func (s *ConfigManagerAppService) ClearContext(ctx context.Context, req ConfigManagerRequest) error {
	if s == nil || s.app == nil {
		return ErrNoWorkspace
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	sessionID, err := configManagerSessionID(req)
	if err != nil {
		return err
	}
	a := s.app
	a.mu.RLock()
	workspace := strings.TrimSpace(a.workspace)
	store := a.sessionStore
	chatService := a.chatService
	a.mu.RUnlock()
	if workspace == "" || store == nil {
		return ErrNoWorkspace
	}
	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return err
	}
	defer operation.Release()
	recovery := s.recoveries.current(workspace, sessionID)
	startTask := s.starts.latestConfigManagerTask(workspace, sessionID).Task
	if recovery != nil && recovery.task != nil {
		if err := abortAndWaitTask(operation.Context(), recovery.task); err != nil {
			return err
		}
	}
	if startTask != nil && (recovery == nil || startTask != recovery.task) {
		if err := abortAndWaitTask(operation.Context(), startTask); err != nil {
			return err
		}
	}
	if err := closeRuntimeBinding(operation.Context(), chatService, agentruntime.BindingSelector{
		Kind: agentruntime.BindingWriting, Profile: agentruntime.ProfileConfigManager,
		Workspace: workspace, SessionID: sessionID,
	}); err != nil {
		return err
	}
	sess, err := store.GetOrCreate(sessionID)
	if err != nil {
		return err
	}
	if err := sess.Clear(); err != nil {
		return err
	}
	s.starts.releaseConfigManagerScope(workspace, sessionID)
	s.recoveries.releaseScope(workspace, sessionID)
	return nil
}

func (s *ConfigManagerAppService) RecoverAgentRuntime(
	ctx context.Context,
	scope ConfigManagerRequest,
	request AgentRuntimeRecoveryRequest,
) (AgentRuntimeRecoveryResult, error) {
	if s == nil || s.app == nil {
		return AgentRuntimeRecoveryResult{}, ErrNoWorkspace
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	sessionID, err := configManagerSessionID(scope)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	a := s.app
	a.mu.RLock()
	workspace := strings.TrimSpace(a.workspace)
	chatService := a.chatService
	store := a.sessionStore
	a.mu.RUnlock()
	if workspace == "" || chatService == nil || store == nil {
		return AgentRuntimeRecoveryResult{}, ErrNoWorkspace
	}
	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	defer operation.Release()
	sess, err := store.GetOrCreate(sessionID)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}

	existing := s.recoveries.current(workspace, sessionID)
	if existing != nil && existing.task != nil && existing.recovery != nil {
		current, currentErr := finishedRecoveryActionStillCurrent(
			operation.Context(), existing.task, existing.recovery, request.Action,
		)
		if currentErr != nil {
			return AgentRuntimeRecoveryResult{}, currentErr
		}
		if !current {
			return resumeExistingConfigManagerRecovery(operation.Context(), existing, request.Action)
		}
	}
	if existing != nil && existing.task != nil && !existing.task.Finished() {
		return AgentRuntimeRecoveryResult{}, ErrAgentOperationActive
	}
	if active := s.starts.latestConfigManagerTask(workspace, sessionID).Task; active != nil && !active.Finished() {
		return AgentRuntimeRecoveryResult{}, ErrAgentOperationActive
	}

	options := configManagerRunOptions(workspace, sessionID)
	recovery, err := chatService.OpenRecoveryObservation(operation.Context(), options)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	if err := validateSelectedRecoveryAction(recovery.InitialStatus(), request.Action); err != nil {
		recovery.Close()
		return AgentRuntimeRecoveryResult{}, err
	}
	structural, isStructural := recoveryStructuralAction(request.Action.Kind)
	run := &configManagerRecoveryRun{
		workspace: workspace, sessionID: sessionID, recovery: recovery,
		recoveryActions: make(map[string]agentruntime.Receipt),
	}
	task, err := NewDeferredRegisteredTask(func(task *Task) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.workspaceTransition || lifecycleWorkspaceKey(a.workspace) != lifecycleWorkspaceKey(workspace) || a.chatService != chatService || a.sessionStore != store {
			return ErrAgentContextChanged
		}
		return a.registerWorkspaceTaskLocked(task, workspace, true)
	})
	if err != nil {
		recovery.Close()
		return AgentRuntimeRecoveryResult{}, err
	}
	run.task = task
	if err := s.recoveries.install(run); err != nil {
		recovery.Close()
		rollbackConfigManagerRecoveryTask(a, s, run, err)
		return AgentRuntimeRecoveryResult{}, err
	}

	key := recoveryActionKey(request.Action)
	var receipt agentruntime.Receipt
	if !isStructural {
		receipt, err = recovery.Resume(operation.Context(), request.Action, task.ID(), task.emit)
		if err != nil {
			recovery.Close()
			rollbackConfigManagerRecoveryTask(a, s, run, err)
			return AgentRuntimeRecoveryResult{}, err
		}
	} else {
		receipt = agentruntime.Receipt{
			CommandID: request.Action.CommandID, OperationID: request.Action.OperationID,
			Cursor: recovery.InitialStatus().Cursor, Replayed: true,
		}
	}
	run.recoveryActions[key] = receipt
	if err := task.Start(func(taskCtx context.Context, task *Task, emit func(agent.Event)) {
		defer a.unregisterWorkspaceTask(task)
		defer recovery.Close()
		if isStructural {
			if _, resumed, resumeErr := chatService.ResumeRecoveredContextStructuralOperation(taskCtx, options, structural); resumeErr != nil {
				emit(agent.Event{Type: "error", Data: map[string]string{"message": resumeErr.Error()}})
				return
			} else if !resumed {
				emit(agent.Event{Type: "error", Data: map[string]string{"message": "Agent 恢复操作已变化 / Agent recovery action changed"}})
				return
			}
			if refreshErr := sess.RefreshCanonical(taskCtx); refreshErr != nil {
				emit(agent.Event{Type: "error", Data: map[string]string{"message": fmt.Sprintf("配置会话刷新失败 / Failed to refresh Config Manager session: %v", refreshErr)}})
				return
			}
		}
		outcome := recovery.Wait(taskCtx, emit)
		log.Printf("[config-manager-recovery] task settled task_id=%s session_id=%s action=%s command_id=%s operation_id=%s outcome=%s", task.ID(), sessionID, request.Action.Kind, request.Action.CommandID, request.Action.OperationID, outcome.Status)
	}); err != nil {
		recovery.Close()
		rollbackConfigManagerRecoveryTask(a, s, run, err)
		return AgentRuntimeRecoveryResult{}, err
	}
	return AgentRuntimeRecoveryResult{Task: task, Action: request.Action, Receipt: receipt}, nil
}

func resumeExistingConfigManagerRecovery(
	ctx context.Context,
	run *configManagerRecoveryRun,
	action agent.RuntimeRecoveryAction,
) (AgentRuntimeRecoveryResult, error) {
	key := recoveryActionKey(action)
	if receipt, ok := run.recoveryActions[key]; ok {
		return AgentRuntimeRecoveryResult{Task: run.task, Action: action, Receipt: receipt}, nil
	}
	if run.task.Finished() {
		return AgentRuntimeRecoveryResult{}, fmt.Errorf("%w: recovery display task is already settled", agent.ErrRecoveryActionChanged)
	}
	if _, structural := recoveryStructuralAction(action.Kind); structural {
		return AgentRuntimeRecoveryResult{}, fmt.Errorf("%w: a structural recovery action cannot join an existing display task", agent.ErrRecoveryActionChanged)
	}
	receipt, err := run.recovery.Resume(ctx, action, run.task.ID(), run.task.emit)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	run.recoveryActions[key] = receipt
	return AgentRuntimeRecoveryResult{Task: run.task, Action: action, Receipt: receipt}, nil
}

func rollbackConfigManagerRecoveryTask(
	a *App,
	s *ConfigManagerAppService,
	run *configManagerRecoveryRun,
	err error,
) {
	if s != nil {
		s.recoveries.rollback(run)
	}
	if run == nil || run.task == nil {
		return
	}
	run.task.failBeforeStart(err)
	if a != nil {
		a.unregisterWorkspaceTask(run.task)
	}
}
