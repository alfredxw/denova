package app

import (
	"context"
	"fmt"

	"denova/config"
	agents "denova/internal/agents"
)

// SubmitAgentChatCommand targets one exact project conversation. Commands for
// another tab cannot steer, queue into, or abort this binding.
func (a *App) SubmitAgentChatCommand(
	ctx context.Context,
	binding AgentChatBinding,
	command ChatAgentCommand,
) (agents.CommandReceipt, error) {
	return a.agentChat().SubmitCommand(ctx, binding, command)
}

func (s *AgentChatAppService) SubmitCommand(
	ctx context.Context,
	binding AgentChatBinding,
	command ChatAgentCommand,
) (agents.CommandReceipt, error) {
	var err error
	binding, err = s.resolveBinding(binding)
	if err != nil {
		return agents.CommandReceipt{}, err
	}
	run := s.activeRun(binding)
	if run == nil || run.task == nil || run.task.Finished() {
		return agents.CommandReceipt{}, ErrNoActiveAgentOperation
	}

	options := agentChatRunOptions(binding, run.task.ID())
	switch command.Kind {
	case agents.AgentCommandAbort, agents.AgentCommandSteerQueued, agents.AgentCommandCancelQueued:
		return run.runtime.chatService.SubmitCommand(ctx, agents.AgentCommandSpec{
			Kind: command.Kind, CommandID: command.CommandID,
			OperationID: command.OperationID, TargetCommandID: command.TargetCommandID, Reason: command.Reason,
			Options: options,
		})
	case agents.AgentCommandSteer, agents.AgentCommandFollowUp, agents.AgentCommandNextTurn:
		// handled below with a deferred project-scoped turn preparation
	default:
		return agents.CommandReceipt{}, fmt.Errorf("%w: unsupported AgentChat command %q", agents.ErrInvalidCommand, command.Kind)
	}

	prepare := func(prepareCtx context.Context) (agents.HarnessTurnExecution, error) {
		if s.activeRun(binding) != run || run.task.Finished() {
			return agents.HarnessTurnExecution{}, ErrAgentContextChanged
		}
		execution, err := s.prepareCommandExecution(prepareCtx, run, command.Input)
		if err != nil {
			return agents.HarnessTurnExecution{}, err
		}
		if s.activeRun(binding) != run || run.task.Finished() {
			return agents.HarnessTurnExecution{}, ErrAgentContextChanged
		}
		return execution, nil
	}
	return run.runtime.chatService.SubmitCommand(ctx, agents.AgentCommandSpec{
		Kind: command.Kind, CommandID: command.CommandID,
		OperationID: command.OperationID, AfterOperationID: command.OperationID,
		Request: command.Input, Emit: run.task.emit, Prepare: prepare, Options: options,
	})
}

func (s *AgentChatAppService) prepareCommandExecution(
	ctx context.Context,
	run *agentChatRun,
	request agents.ChatRequest,
) (agents.HarnessTurnExecution, error) {
	runtime, resolved, err := s.app.chat().prepareIDEChatRuntimeSnapshot(ctx, run.runtime, request)
	if err != nil {
		return agents.HarnessTurnExecution{}, err
	}
	runner, systemPrompt, err := buildAgentRunnerWithComposition(ctx, &runtime.cfg, runtime.state, runtime.ideTeller)
	if err != nil {
		return agents.HarnessTurnExecution{}, err
	}
	runtimeContexts := agents.IDEWorkspaceRuntimeContextsForRequest(runtime.state, resolved)
	conversation := agents.NewSessionConversationForAgentWithRuntimeContexts(
		runtime.sess, &runtime.cfg, config.AgentKindIDE,
		runtimeContexts.StableTitle, runtimeContexts.Stable,
		runtimeContexts.DynamicTitle, runtimeContexts.Dynamic,
	)
	options := agentChatRunOptions(run.binding, run.task.ID())
	options.IdleTimeout = agentIdleTimeout(runtime.cfg)
	options.ToolResultMaxBytes = agentToolResultMaxBytes(runtime.cfg)
	options.SystemPromptLog = systemPrompt
	options.OnMutationsVerified = s.app.verifiedWorkspaceMutationCallback(
		"agent_chat_post_run", runtime.versionService, versionAutoSettingsForConfig(&runtime.cfg),
	)
	options = s.app.chat().bindReviewFeedbackInputCommit(options, runtime, resolved)
	return agents.HarnessTurnExecution{
		Runner: runner, Conversation: conversation, BookService: runtime.bookService,
		Request: resolved, Options: options,
	}, nil
}
