package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	agents "denova/internal/agents"
)

func (a *App) RecoverAgentChat(
	ctx context.Context,
	binding AgentChatBinding,
	request AgentRuntimeRecoveryRequest,
) (AgentRuntimeRecoveryResult, error) {
	return a.agentChat().Recover(ctx, binding, request)
}

func (s *AgentChatAppService) Recover(
	ctx context.Context,
	binding AgentChatBinding,
	request AgentRuntimeRecoveryRequest,
) (AgentRuntimeRecoveryResult, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	var err error
	binding, err = s.resolveBinding(binding)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	if existing := s.activeRun(binding); existing != nil && existing.task != nil {
		if existing.recovery == nil {
			if !existing.task.Finished() {
				return AgentRuntimeRecoveryResult{}, ErrAgentOperationActive
			}
		} else {
			return resumeExistingAgentChatRecovery(ctx, existing, request.Action)
		}
	}
	if _, structural := recoveryStructuralAction(request.Action.Kind); structural {
		return AgentRuntimeRecoveryResult{}, fmt.Errorf("%w: AgentChat has no project structural recovery action", agents.ErrRecoveryActionChanged)
	}

	project, err := s.projectRuntime(ctx, binding.Workspace)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	sess, err := project.store.Get(binding.SessionID)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	options := agentChatRunOptions(binding, "")
	recovery, err := project.chatService.OpenRecoveryObservation(ctx, options)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	if err := validateSelectedRecoveryAction(recovery.InitialStatus(), request.Action); err != nil {
		recovery.Close()
		return AgentRuntimeRecoveryResult{}, err
	}
	if _, err := reconcileColdPendingAsk(ctx, sess, recovery.InitialStatus()); err != nil {
		recovery.Close()
		return AgentRuntimeRecoveryResult{}, fmt.Errorf("reconcile orphaned AgentChat Ask before recovery: %w", err)
	}
	runtime := ideChatRuntime{
		app: s.app, sess: sess, state: project.state, bookService: project.bookService,
		chatService: project.chatService, workspace: project.workspace,
		versionService: project.versionService, cfg: project.cfg,
	}
	run := &agentChatRun{
		binding: binding, runtime: runtime, recovery: recovery,
		recoveryActions: make(map[string]agents.CommandReceipt),
	}
	task, err := NewDeferredRegisteredTask(func(task *Task) error {
		run.task = task
		return s.installActiveRun(run)
	})
	if err != nil {
		recovery.Close()
		return AgentRuntimeRecoveryResult{}, err
	}
	receipt, err := recovery.Resume(ctx, request.Action, task.ID(), task.emit)
	if err != nil {
		recovery.Close()
		task.failBeforeStart(err)
		s.releaseActiveRun(run)
		return AgentRuntimeRecoveryResult{}, err
	}
	run.recoveryActions[recoveryActionKey(request.Action)] = receipt
	run.commandID = agentChatRuntimeCommandID(recovery.InitialStatus())
	if err := task.Start(func(taskCtx context.Context, task *Task, emit func(agents.Event)) {
		defer s.releaseActiveRun(run)
		defer recovery.Close()
		outcome := recovery.Wait(taskCtx, emit)
		log.Printf("[agent-chat-recovery] settled task_id=%s workspace=%q session_id=%s action=%s outcome=%s", task.ID(), binding.Workspace, binding.SessionID, request.Action.Kind, outcome.Status)
	}); err != nil {
		recovery.Close()
		task.failBeforeStart(err)
		s.releaseActiveRun(run)
		return AgentRuntimeRecoveryResult{}, err
	}
	return AgentRuntimeRecoveryResult{Task: task, Action: request.Action, Receipt: receipt}, nil
}

func resumeExistingAgentChatRecovery(
	ctx context.Context,
	run *agentChatRun,
	action agents.RuntimeRecoveryAction,
) (AgentRuntimeRecoveryResult, error) {
	if run == nil || run.task == nil || run.recovery == nil {
		return AgentRuntimeRecoveryResult{}, ErrNoActiveAgentOperation
	}
	key := recoveryActionKey(action)
	if receipt, ok := run.recoveryActions[key]; ok {
		return AgentRuntimeRecoveryResult{Task: run.task, Action: action, Receipt: receipt}, nil
	}
	if run.task.Finished() {
		return AgentRuntimeRecoveryResult{}, fmt.Errorf("%w: AgentChat recovery display task is settled", agents.ErrRecoveryActionChanged)
	}
	if _, structural := recoveryStructuralAction(action.Kind); structural {
		return AgentRuntimeRecoveryResult{}, fmt.Errorf("%w: structural action cannot join AgentChat recovery", agents.ErrRecoveryActionChanged)
	}
	receipt, err := run.recovery.Resume(ctx, action, run.task.ID(), run.task.emit)
	if err != nil {
		return AgentRuntimeRecoveryResult{}, err
	}
	run.recoveryActions[key] = receipt
	return AgentRuntimeRecoveryResult{Task: run.task, Action: action, Receipt: receipt}, nil
}

func agentChatRuntimeCommandID(runtime agents.RuntimeStatus) string {
	if runtime.ActiveCommandID != "" {
		return string(runtime.ActiveCommandID)
	}
	if runtime.LastOperation != nil {
		return string(runtime.LastOperation.CommandID)
	}
	return ""
}

func (s *AgentChatAppService) restoreHarnessTurn(
	request agents.HarnessTurnRestoreRequest,
	binding agents.RuntimeBinding,
) agents.HarnessTurnSpec {
	return agents.HarnessTurnSpec{
		Request: request.Request,
		Options: request.Options,
		Prepare: func(ctx context.Context) (agents.HarnessTurnExecution, error) {
			scope := AgentChatBinding{
				Workspace: strings.TrimSpace(binding.Workspace),
				SessionID: strings.TrimSpace(binding.SessionID),
			}
			run := s.activeRun(scope)
			if run == nil || run.task == nil || run.task.Finished() {
				return agents.HarnessTurnExecution{}, agents.ErrHarnessTurnRestoreUnavailable
			}
			execution, err := s.prepareCommandExecution(ctx, run, request.Request)
			if err != nil {
				return agents.HarnessTurnExecution{}, err
			}
			if execution.Options.Mode != agentChatRuntimeMode || execution.Options.Workspace != scope.Workspace || execution.Options.SessionID != scope.SessionID {
				return agents.HarnessTurnExecution{}, fmt.Errorf("%w: restored AgentChat runtime does not match durable binding", agents.ErrHarnessTurnRestoreUnavailable)
			}
			return execution, nil
		},
	}
}
