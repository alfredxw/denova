package agentchat

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	appagentruntime "denova/internal/app/agentruntime"
	apptask "denova/internal/app/task"
)

func (service *Service) Recover(ctx context.Context, binding Binding, request appagentruntime.RecoveryRequest) (appagentruntime.RecoveryResult, error) {
	service.admission.Lock()
	defer service.admission.Unlock()
	var err error
	binding, err = service.ResolveBinding(binding)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	if existing := service.activeRun(binding); existing != nil && existing.task != nil {
		if existing.recovery == nil {
			if !existing.task.Finished() {
				return appagentruntime.RecoveryResult{}, appagentruntime.ErrOperationActive
			}
		} else {
			return service.resumeExistingRecovery(ctx, existing, request.Action)
		}
	}
	if _, structural := appagentruntime.StructuralRecoveryAction(request.Action.Kind); structural {
		return appagentruntime.RecoveryResult{}, fmt.Errorf("%w: AgentChat has no project structural recovery action", agentexecution.ErrRecoveryActionChanged)
	}

	project, err := service.projectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	sess, err := project.store.Get(binding.SessionID)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	options := runtimeOptions(binding, "")
	recovery, err := project.executionRuntime.OpenRecoveryObservation(ctx, options)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	if err := appagentruntime.ValidateRecoveryAction(recovery.InitialStatus(), request.Action); err != nil {
		recovery.Close()
		return appagentruntime.RecoveryResult{}, err
	}
	if _, err := agentconversation.ReconcileColdPendingAsk(ctx, sess, recovery.InitialStatus()); err != nil {
		recovery.Close()
		return appagentruntime.RecoveryResult{}, fmt.Errorf("reconcile orphaned AgentChat Ask before recovery: %w", err)
	}
	active := &run{
		binding: binding, runtime: project.conversation(sess), recovery: recovery,
		recoveryActions: make(map[string]agentrun.CommandReceipt),
	}
	task, err := apptask.NewDeferredWithContext(ctx, func(task *apptask.Task) error {
		active.task = task
		return service.installActiveRun(active)
	})
	if err != nil {
		recovery.Close()
		return appagentruntime.RecoveryResult{}, err
	}
	receipt, err := recovery.Resume(ctx, request.Action, task.ID(), task.Emit)
	if err != nil {
		recovery.Close()
		task.RejectStart(err)
		service.releaseActiveRun(active)
		return appagentruntime.RecoveryResult{}, err
	}
	active.recoveryActions[appagentruntime.RecoveryActionKey(request.Action)] = receipt
	active.commandID = runtimeCommandID(recovery.InitialStatus())
	if err := task.Start(func(taskCtx context.Context, task *apptask.Task, emit func(agentrun.Event)) {
		defer service.releaseActiveRun(active)
		defer recovery.Close()
		outcome := recovery.Wait(taskCtx, emit)
		slog.InfoContext(taskCtx, fmt.Sprintf(
			"[app/agentchat] recovery settled task_id=%s project_id=%s workspace=%q session_id=%s action=%s outcome=%s",
			task.ID(), binding.ProjectID, binding.Workspace, binding.SessionID, request.Action.Kind, outcome.Status,
		))
	}); err != nil {
		recovery.Close()
		task.RejectStart(err)
		service.releaseActiveRun(active)
		return appagentruntime.RecoveryResult{}, err
	}
	return appagentruntime.RecoveryResult{Task: task, Action: request.Action, Receipt: receipt}, nil
}

func (service *Service) resumeExistingRecovery(ctx context.Context, active *run, action agentexecution.RuntimeRecoveryAction) (appagentruntime.RecoveryResult, error) {
	if active == nil || active.task == nil || active.recovery == nil {
		return appagentruntime.RecoveryResult{}, appagentruntime.ErrNoActiveOperation
	}
	key := appagentruntime.RecoveryActionKey(action)
	if receipt, ok := active.recoveryActions[key]; ok {
		return appagentruntime.RecoveryResult{Task: active.task, Action: action, Receipt: receipt}, nil
	}
	if active.task.Finished() {
		return appagentruntime.RecoveryResult{}, fmt.Errorf("%w: AgentChat recovery display task is settled", agentexecution.ErrRecoveryActionChanged)
	}
	if _, structural := appagentruntime.StructuralRecoveryAction(action.Kind); structural {
		return appagentruntime.RecoveryResult{}, fmt.Errorf("%w: structural action cannot join AgentChat recovery", agentexecution.ErrRecoveryActionChanged)
	}
	receipt, err := active.recovery.Resume(ctx, action, active.task.ID(), active.task.Emit)
	if err != nil {
		return appagentruntime.RecoveryResult{}, err
	}
	active.recoveryActions[key] = receipt
	return appagentruntime.RecoveryResult{Task: active.task, Action: action, Receipt: receipt}, nil
}

func runtimeCommandID(runtime agentrun.RuntimeStatus) string {
	if runtime.ActiveCommandID != "" {
		return string(runtime.ActiveCommandID)
	}
	if runtime.LastOperation != nil {
		return string(runtime.LastOperation.CommandID)
	}
	return ""
}

// PrepareCycle rebuilds one queued or recovered AgentChat cycle from the
// latest canonical Project state. The durable descriptor remains authoritative
// for caller input and binding identity.
func (service *Service) PrepareCycle(
	ctx context.Context,
	request agentexecution.CycleRestoreRequest,
	binding agentrun.RuntimeBinding,
) (agentexecution.Cycle, error) {
	scope := Binding{
		ProjectID: strings.TrimSpace(binding.ProjectID),
		Workspace: strings.TrimSpace(binding.Workspace),
		SessionID: strings.TrimSpace(binding.SessionID),
	}
	var err error
	scope, err = service.ResolveBinding(scope)
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	active := service.activeRun(scope)
	if active == nil || active.task == nil || active.task.Finished() {
		return agentexecution.Cycle{}, agentexecution.ErrCyclePreparationUnavailable
	}
	cycle, err := service.prepareCommandExecution(ctx, active, request.Request)
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	if service.activeRun(scope) != active || active.task.Finished() {
		return agentexecution.Cycle{}, appagentruntime.ErrContextChanged
	}
	if cycle.Options.Mode != RuntimeMode || cycle.Options.ProjectID != scope.ProjectID || cycle.Options.Workspace != scope.Workspace || cycle.Options.SessionID != scope.SessionID {
		return agentexecution.Cycle{}, fmt.Errorf("%w: prepared AgentChat runtime does not match durable binding", agentexecution.ErrCyclePreparationUnavailable)
	}
	cycle.Successor = func(ctx context.Context, parent agentrun.OperationID, outcome agentrun.Outcome) error {
		active := service.activeRun(scope)
		if active == nil {
			return appagentruntime.ErrContextChanged
		}
		return service.goalSuccessor(active)(ctx, parent, outcome)
	}
	return cycle, nil
}
