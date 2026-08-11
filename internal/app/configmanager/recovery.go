package configmanager

import (
	"context"
	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
	apptask "denova/internal/app/task"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	appagentruntime "denova/internal/app/agentruntime"
)

// ActiveView binds a reconnectable display Task and the
// durable Runtime projection to one exact Config Manager scope.
type ActiveView struct {
	Task                *apptask.Snapshot
	CommandID           string
	StreamAttached      bool
	Runtime             agentrun.RuntimeStatus
	RuntimeProjectionOK bool
	PendingAsk          *session.AskInteraction
}

func runOptions(projectID, workspace, stateRoot, sessionID string) agentrun.Options {
	return agentrun.Options{
		AgentKind: agentrun.AgentKindConfigManager,
		ProjectID: projectID,
		StateRoot: stateRoot,
		Workspace: workspace,
		SessionID: sessionID,
		Mode:      RuntimeMode,
	}
}

func (service *Service) ActiveView(ctx context.Context, request Request) ActiveView {
	if service == nil || service.host == nil {
		return ActiveView{}
	}
	service.admission.Lock()
	defer service.admission.Unlock()
	sessionID, err := SessionID(request)
	if err != nil {
		return ActiveView{}
	}
	runtime, err := service.runtime(ctx, request)
	if err != nil {
		return ActiveView{}
	}
	projectID := runtime.ProjectID
	workspace := strings.TrimSpace(runtime.Workspace)
	if workspace == "" || runtime.ExecutionRuntime == nil {
		return ActiveView{}
	}
	operation, err := service.host.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return ActiveView{}
	}
	defer operation.Release()

	runtimeSnapshot, projected := projectRuntime(
		operation.Context(), runtime.ExecutionRuntime, runOptions(projectID, workspace, runtime.Config.ProjectStateDir, sessionID),
	)
	record, recoverySelected := selectDisplayRecord(
		latestStartTask(&service.starts, projectID, sessionID),
		service.recoveries.current(projectID, sessionID),
	)
	if recoverySelected {
		record.CommandID = runtimeCommandID(runtimeSnapshot)
	}

	if !service.host.IsCurrent(runtime) {
		return ActiveView{}
	}
	var taskSnapshot *apptask.Snapshot
	if record.Task != nil {
		snapshot := record.Task.Snapshot()
		taskSnapshot = &snapshot
	}
	var pendingAsk *session.AskInteraction
	if store := runtime.SessionStore; store != nil {
		if sess, loadErr := store.Get(sessionID); loadErr == nil {
			if projected {
				reconciled, reconcileErr := agentconversation.ReconcileColdPendingAsk(operation.Context(), sess, runtimeSnapshot)
				if reconcileErr != nil {
					slog.ErrorContext(ctx, fmt.Sprintf("[agent-ask-recovery] reconcile Config Manager Ask failed workspace=%s session_id=%s operation_id=%s cycle=%d err=%v", workspace, sessionID, runtimeSnapshot.ActiveOperation, runtimeSnapshot.ActiveCycle, reconcileErr))
				} else if reconciled {
					slog.InfoContext(ctx, fmt.Sprintf("[agent-ask-recovery] cancelled orphaned Config Manager Ask workspace=%s session_id=%s operation_id=%s cycle=%d", workspace, sessionID, runtimeSnapshot.ActiveOperation, runtimeSnapshot.ActiveCycle))
				}
			}
			// Never project a cold durable Ask as answerable unless this process
			// owns the waiter that resumes the exact model continuation.
			pendingAsk = sess.LivePendingAsk("")
		}
	}
	return ActiveView{
		Task: taskSnapshot, CommandID: record.CommandID,
		StreamAttached: displayOwnsRuntime(record, runtimeSnapshot),
		Runtime:        runtimeSnapshot, RuntimeProjectionOK: projected, PendingAsk: pendingAsk,
	}
}

// DisplayTask resolves an already-authorized task ID within the
// exact Config Manager scope. It never falls back to another active workspace
// task, which prevents cross-panel stream attachment.
func (service *Service) DisplayTask(ctx context.Context, request Request, taskID string) *apptask.Task {
	if service == nil || service.host == nil || strings.TrimSpace(taskID) == "" {
		return nil
	}
	service.admission.Lock()
	defer service.admission.Unlock()
	sessionID, err := SessionID(request)
	if err != nil {
		return nil
	}
	runtime, err := service.runtime(ctx, request)
	if err != nil {
		return nil
	}
	projectID := runtime.ProjectID
	workspace := strings.TrimSpace(runtime.Workspace)
	if workspace == "" || runtime.ExecutionRuntime == nil {
		return nil
	}
	operation, err := service.host.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return nil
	}
	defer operation.Release()
	runtimeSnapshot, projected := projectRuntime(
		operation.Context(), runtime.ExecutionRuntime, runOptions(projectID, workspace, runtime.Config.ProjectStateDir, sessionID),
	)
	if !projected {
		return nil
	}
	record, recoverySelected := selectDisplayRecord(
		latestStartTask(&service.starts, projectID, sessionID),
		service.recoveries.current(projectID, sessionID),
	)
	if recoverySelected {
		record.CommandID = runtimeCommandID(runtimeSnapshot)
	}
	task := record.Task
	if task != nil && task.ID() == taskID && displayOwnsRuntime(record, runtimeSnapshot) {
		return task
	}
	return nil
}

// runtimeProjector keeps active inspection actor-free for the
// overwhelmingly common idle case. A cold unfinished journal is the only
// state that needs canonical reconciliation before it can expose recovery
// identities.
type runtimeProjector interface {
	RuntimeStatusProjection(context.Context, agentrun.Options) (agentrun.RuntimeStatus, error)
	RuntimeRecoveryStatusProjection(context.Context, agentrun.Options) (agentrun.RuntimeStatus, error)
}

func projectRuntime(
	ctx context.Context,
	projector runtimeProjector,
	options agentrun.Options,
) (agentrun.RuntimeStatus, bool) {
	if projector == nil {
		return agentrun.RuntimeStatus{}, false
	}
	snapshot, err := projector.RuntimeStatusProjection(ctx, options)
	if err != nil {
		logProjectionError(options, err)
		return agentrun.RuntimeStatus{}, false
	}
	if !snapshot.RecoveryPending {
		return snapshot, true
	}
	snapshot, err = projector.RuntimeRecoveryStatusProjection(ctx, options)
	if err != nil {
		logProjectionError(options, err)
		return agentrun.RuntimeStatus{}, false
	}
	return snapshot, true
}

func logProjectionError(options agentrun.Options, err error) {
	if errors.Is(err, agentexecution.ErrRuntimeProjectionUnavailable) {
		return
	}
	slog.WarnContext(context.Background(), fmt.Sprintf("[config-manager-runtime] projection unavailable workspace=%s session_id=%s err=%v", options.Workspace, options.SessionID, err))
}

// selectConfigManagerDisplayRecord chooses one process-local display owner for
// a scope. Recovery records intentionally outlive settlement for replay, so an
// older recovered Task must not shadow a newer normal run.
func selectDisplayRecord(start taskRecord, recovery *recoveryRun) (taskRecord, bool) {
	if recovery == nil || recovery.task == nil {
		return start, false
	}
	if start.Task == nil || start.Task == recovery.task {
		return taskRecord{Task: recovery.task}, true
	}
	startSnapshot := start.Task.Snapshot()
	recoverySnapshot := recovery.task.Snapshot()
	if startSnapshot.Finished != recoverySnapshot.Finished {
		if !startSnapshot.Finished {
			return start, false
		}
		return taskRecord{Task: recovery.task}, true
	}
	if start.Task.StartedAt().After(recovery.task.StartedAt()) {
		return start, false
	}
	return taskRecord{Task: recovery.task}, true
}

func displayOwnsRuntime(record taskRecord, runtime agentrun.RuntimeStatus) bool {
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
	if runtime.Phase == agentrun.RunPhaseIdle && runtime.LastOperation != nil && string(runtime.LastOperation.CommandID) == commandID {
		return true
	}
	return false
}

func runtimeCommandID(runtime agentrun.RuntimeStatus) string {
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

// ClearContext drains the exact display/runtime binding
// before appending the Session clear marker. This prevents a late recovered
// tool/model result from repopulating a scope the user has already cleared.
func (service *Service) ClearContext(ctx context.Context, request Request) error {
	if service == nil || service.host == nil {
		return appagentruntime.ErrNoWorkspace
	}
	service.admission.Lock()
	defer service.admission.Unlock()
	sessionID, err := SessionID(request)
	if err != nil {
		return err
	}
	runtime, err := service.runtime(ctx, request)
	if err != nil {
		return err
	}
	projectID := runtime.ProjectID
	workspace := strings.TrimSpace(runtime.Workspace)
	if workspace == "" || runtime.SessionStore == nil {
		return appagentruntime.ErrNoWorkspace
	}
	operation, err := service.host.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return err
	}
	defer operation.Release()
	recovery := service.recoveries.current(projectID, sessionID)
	startTask := latestStartTask(&service.starts, projectID, sessionID).Task
	if recovery != nil && recovery.task != nil {
		if err := appagentruntime.AbortAndWait(operation.Context(), recovery.task); err != nil {
			return err
		}
	}
	if startTask != nil && (recovery == nil || startTask != recovery.task) {
		if err := appagentruntime.AbortAndWait(operation.Context(), startTask); err != nil {
			return err
		}
	}
	if err := appagentruntime.CloseBindings(runtime.ExecutionRuntime, func(chat *agentexecution.Runtime) error {
		return chat.CloseSessionBindings(operation.Context(), agentrun.AgentKindConfigManager, workspace, sessionID)
	}); err != nil {
		return err
	}
	sess, err := runtime.SessionStore.GetOrCreate(sessionID)
	if err != nil {
		return err
	}
	if err := sess.Clear(); err != nil {
		return err
	}
	service.starts.ReleaseScope(projectID, sessionID)
	service.recoveries.releaseScope(projectID, sessionID)
	return nil
}

func (service *Service) Recover(
	ctx context.Context,
	scope Request,
	request appagentruntime.RecoveryRequest,
) (appagentruntime.RecoveryResult, error) {
	if service == nil || service.host == nil {
		return appagentruntime.RecoveryResult{}, appagentruntime.ErrNoWorkspace
	}
	service.admission.Lock()
	defer service.admission.Unlock()
	sessionID, err := SessionID(scope)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	runtime, err := service.runtime(ctx, scope)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	projectID := runtime.ProjectID
	workspace := strings.TrimSpace(runtime.Workspace)
	if workspace == "" || runtime.ExecutionRuntime == nil || runtime.SessionStore == nil {
		return appagentruntime.RecoveryResult{}, appagentruntime.ErrNoWorkspace
	}
	operation, err := service.host.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	defer operation.Release()
	sess, err := runtime.SessionStore.GetOrCreate(sessionID)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}

	existing := service.recoveries.current(projectID, sessionID)
	if existing != nil && existing.task != nil && existing.recovery != nil {
		current, currentErr := appagentruntime.FinishedRecoveryActionStillCurrent(
			operation.Context(), existing.task, existing.recovery, request.Action,
		)
		if currentErr != nil {
			return appagentruntime.RecoveryResult{}, currentErr
		}
		if !current {
			return resumeExistingRecovery(operation.Context(), existing, request.Action)
		}
	}
	if existing != nil && existing.task != nil && !existing.task.Finished() {
		return appagentruntime.RecoveryResult{}, appagentruntime.ErrOperationActive
	}
	if active := latestStartTask(&service.starts, projectID, sessionID).Task; active != nil && !active.Finished() {
		return appagentruntime.RecoveryResult{}, appagentruntime.ErrOperationActive
	}

	options := runOptions(projectID, workspace, runtime.Config.ProjectStateDir, sessionID)
	recovery, err := runtime.ExecutionRuntime.OpenRecoveryObservation(operation.Context(), options)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	if err := appagentruntime.ValidateRecoveryAction(recovery.InitialStatus(), request.Action); err != nil {
		recovery.Close()
		return appagentruntime.RecoveryResult{}, err
	}
	if _, err := agentconversation.ReconcileColdPendingAsk(operation.Context(), sess, recovery.InitialStatus()); err != nil {
		recovery.Close()
		return appagentruntime.RecoveryResult{}, fmt.Errorf("reconcile orphaned Ask before Config Manager recovery: %w", err)
	}
	run := &recoveryRun{
		projectID: projectID, sessionID: sessionID, recovery: recovery,
		recoveryActions: make(map[string]agentrun.CommandReceipt),
	}
	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
		return service.host.RegisterTask(task, runtime)
	})
	if err != nil {
		recovery.Close()
		return appagentruntime.RecoveryResult{}, err
	}
	run.task = task
	if err := service.recoveries.install(run); err != nil {
		recovery.Close()
		service.rollbackRecoveryTask(run, err)
		return appagentruntime.RecoveryResult{}, err
	}

	key := appagentruntime.RecoveryActionKey(request.Action)
	receipt, err := recovery.Resume(operation.Context(), request.Action, task.ID(), task.Emit)
	if err != nil {
		recovery.Close()
		service.rollbackRecoveryTask(run, err)
		return appagentruntime.RecoveryResult{}, err
	}
	run.recoveryActions[key] = receipt
	if err := task.Start(func(taskCtx context.Context, task *apptask.Task, emit func(agentrun.Event)) {
		defer service.host.UnregisterTask(task)
		defer recovery.Close()
		outcome := recovery.Wait(taskCtx, emit)
		slog.InfoContext(taskCtx, fmt.Sprintf("[config-manager-recovery] task settled task_id=%s session_id=%s action=%s command_id=%s operation_id=%s outcome=%s", task.ID(), sessionID, request.Action.Kind, request.Action.CommandID, request.Action.OperationID, outcome.Status))
	}); err != nil {
		recovery.Close()
		service.rollbackRecoveryTask(run, err)
		return appagentruntime.RecoveryResult{}, err
	}
	return appagentruntime.RecoveryResult{Task: task, Action: request.Action, Receipt: receipt}, nil
}

func resumeExistingRecovery(
	ctx context.Context,
	run *recoveryRun,
	action agentexecution.RuntimeRecoveryAction,
) (appagentruntime.RecoveryResult, error) {
	key := appagentruntime.RecoveryActionKey(action)
	if receipt, ok := run.recoveryActions[key]; ok {
		return appagentruntime.RecoveryResult{Task: run.task, Action: action, Receipt: receipt}, nil
	}
	if run.task.Finished() {
		return appagentruntime.RecoveryResult{}, fmt.Errorf("%w: recovery display task is already settled", agentexecution.ErrRecoveryActionChanged)
	}
	receipt, err := run.recovery.Resume(ctx, action, run.task.ID(), run.task.Emit)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	run.recoveryActions[key] = receipt
	return appagentruntime.RecoveryResult{Task: run.task, Action: action, Receipt: receipt}, nil
}

func (service *Service) rollbackRecoveryTask(run *recoveryRun, err error) {
	if service != nil {
		service.recoveries.rollback(run)
	}
	if run == nil || run.task == nil {
		return
	}
	run.task.RejectStart(err)
	if service != nil && service.host != nil {
		service.host.UnregisterTask(run.task)
	}
}
