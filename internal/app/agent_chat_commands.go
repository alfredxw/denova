package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	"fmt"
)

// SubmitAgentChatCommand targets one exact project conversation. Commands for
// another tab cannot steer, queue into, or abort this binding.
func (a *App) SubmitAgentChatCommand(
	ctx context.Context,
	binding AgentChatBinding,
	command ChatAgentCommand,
) (agentrun.CommandReceipt, error) {
	return a.agentChat().SubmitCommand(ctx, binding, command)
}

func (s *AgentChatAppService) SubmitCommand(
	ctx context.Context,
	binding AgentChatBinding,
	command ChatAgentCommand,
) (agentrun.CommandReceipt, error) {
	var err error
	binding, err = s.resolveBinding(binding)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	run := s.activeRun(binding)
	if run == nil || run.task == nil || run.task.Finished() {
		return agentrun.CommandReceipt{}, ErrNoActiveAgentOperation
	}

	options := agentChatRunOptions(binding, run.task.ID())
	switch command.Kind {
	case agentharness.CommandAbort, agentharness.CommandSteerQueued, agentharness.CommandCancelQueued:
		return run.runtime.chatService.SubmitCommand(ctx, agentharness.CommandSpec{
			Kind: command.Kind, CommandID: command.CommandID,
			OperationID: command.OperationID, TargetCommandID: command.TargetCommandID, Reason: command.Reason,
			Options: options,
		})
	case agentharness.CommandSteer, agentharness.CommandFollowUp, agentharness.CommandNextTurn:
		// handled below with a deferred project-scoped turn preparation
	default:
		return agentrun.CommandReceipt{}, fmt.Errorf("%w: unsupported AgentChat command %q", agentrun.ErrInvalidCommand, command.Kind)
	}

	prepare := func(prepareCtx context.Context) (agentharness.TurnExecution, error) {
		if s.activeRun(binding) != run || run.task.Finished() {
			return agentharness.TurnExecution{}, ErrAgentContextChanged
		}
		execution, err := s.prepareCommandExecution(prepareCtx, run, command.Input)
		if err != nil {
			return agentharness.TurnExecution{}, err
		}
		if s.activeRun(binding) != run || run.task.Finished() {
			return agentharness.TurnExecution{}, ErrAgentContextChanged
		}
		return execution, nil
	}
	return run.runtime.chatService.SubmitCommand(ctx, agentharness.CommandSpec{
		Kind: command.Kind, CommandID: command.CommandID,
		OperationID: command.OperationID, AfterOperationID: command.OperationID,
		Request: command.Input, Emit: run.task.Emit, Prepare: prepare, Options: options,
	})
}

func (s *AgentChatAppService) prepareCommandExecution(
	ctx context.Context,
	run *agentChatRun,
	request agentchat.ChatRequest,
) (agentharness.TurnExecution, error) {
	runtime, resolved, err := s.app.chat().prepareProjectChatRuntimeSnapshot(ctx, run.runtime, request)
	if err != nil {
		return agentharness.TurnExecution{}, err
	}
	runner, systemPrompt, err := buildProjectAgentRunnerWithComposition(ctx, runtime)
	if err != nil {
		return agentharness.TurnExecution{}, err
	}
	conversation := projectSessionConversation(runtime, resolved)
	options := agentChatRunOptions(run.binding, run.task.ID())
	options.IdleTimeout = agentIdleTimeout(runtime.cfg)
	options.ToolResultMaxBytes = agentToolResultMaxBytes(runtime.cfg)
	options.SystemPromptLog = systemPrompt
	options.OnMutationsVerified = s.app.verifiedWorkspaceMutationCallback(
		"agent_chat_post_run", runtime.versionService, versionAutoSettingsForConfig(&runtime.cfg),
	)
	options = s.app.chat().bindReviewFeedbackInputCommit(options, runtime, resolved)
	return agentharness.TurnExecution{
		Runner: runner, Conversation: conversation, BookService: runtime.bookService,
		Request: resolved, Options: options,
	}, nil
}
