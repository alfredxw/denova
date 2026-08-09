package agentchat

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	agentconversation "denova/internal/agents/conversation"
	agentharness "denova/internal/agents/harness"
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
		return appagentruntime.RecoveryResult{}, fmt.Errorf("%w: AgentChat has no project structural recovery action", agentharness.ErrRecoveryActionChanged)
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
	recovery, err := project.chatService.OpenRecoveryObservation(ctx, options)
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

func (service *Service) resumeExistingRecovery(ctx context.Context, active *run, action agentharness.RuntimeRecoveryAction) (appagentruntime.RecoveryResult, error) {
	if active == nil || active.task == nil || active.recovery == nil {
		return appagentruntime.RecoveryResult{}, appagentruntime.ErrNoActiveOperation
	}
	key := appagentruntime.RecoveryActionKey(action)
	if receipt, ok := active.recoveryActions[key]; ok {
		return appagentruntime.RecoveryResult{Task: active.task, Action: action, Receipt: receipt}, nil
	}
	if active.task.Finished() {
		return appagentruntime.RecoveryResult{}, fmt.Errorf("%w: AgentChat recovery display task is settled", agentharness.ErrRecoveryActionChanged)
	}
	if _, structural := appagentruntime.StructuralRecoveryAction(action.Kind); structural {
		return appagentruntime.RecoveryResult{}, fmt.Errorf("%w: structural action cannot join AgentChat recovery", agentharness.ErrRecoveryActionChanged)
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

// RestoreTurn reconstructs process-local execution dependencies for a durable
// queued AgentChat command. The descriptor remains authoritative for input and
// binding identity.
func (service *Service) RestoreTurn(request agentharness.TurnRestoreRequest, binding agentrun.RuntimeBinding) agentharness.TurnSpec {
	spec := agentharness.TurnSpec{
		Request: request.Request,
		Options: request.Options,
		Prepare: func(ctx context.Context) (agentharness.TurnExecution, error) {
			scope := Binding{
				ProjectID: strings.TrimSpace(binding.ProjectID),
				Workspace: strings.TrimSpace(binding.Workspace),
				SessionID: strings.TrimSpace(binding.SessionID),
			}
			var err error
			scope, err = service.ResolveBinding(scope)
			if err != nil {
				return agentharness.TurnExecution{}, err
			}
			active := service.activeRun(scope)
			if active == nil || active.task == nil || active.task.Finished() {
				return agentharness.TurnExecution{}, agentharness.ErrTurnRestoreUnavailable
			}
			execution, err := service.prepareCommandExecution(ctx, active, request.Request)
			if err != nil {
				return agentharness.TurnExecution{}, err
			}
			if execution.Options.Mode != RuntimeMode || execution.Options.ProjectID != scope.ProjectID || execution.Options.Workspace != scope.Workspace || execution.Options.SessionID != scope.SessionID {
				return agentharness.TurnExecution{}, fmt.Errorf("%w: restored AgentChat runtime does not match durable binding", agentharness.ErrTurnRestoreUnavailable)
			}
			return execution, nil
		},
	}
	spec.Successor = func(ctx context.Context, parent agentrun.OperationID, outcome agentrun.Outcome) error {
		scope, err := service.ResolveBinding(Binding{
			ProjectID: strings.TrimSpace(binding.ProjectID), Workspace: strings.TrimSpace(binding.Workspace), SessionID: strings.TrimSpace(binding.SessionID),
		})
		if err != nil {
			return err
		}
		active := service.activeRun(scope)
		if active == nil {
			return appagentruntime.ErrContextChanged
		}
		return service.goalSuccessor(active)(ctx, parent, outcome)
	}
	return spec
}
