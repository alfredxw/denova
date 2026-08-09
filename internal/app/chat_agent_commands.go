package app

import (
	"context"
	"fmt"

	"denova/config"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	appagentruntime "denova/internal/app/agentruntime"
	apptask "denova/internal/app/task"
)

var ErrNoActiveAgentOperation = appagentruntime.ErrNoActiveOperation

type ChatAgentCommand = appagentruntime.Command

// SubmitChatAgentCommand adapts a transport command to the active writing
// binding. Workspace/session identity is captured from App state and never
// accepted from the client.
func (a *App) SubmitChatAgentCommand(ctx context.Context, command ChatAgentCommand) (agentrun.CommandReceipt, error) {
	return a.chat().SubmitAgentCommand(ctx, command)
}

func (s *ChatAppService) SubmitAgentCommand(ctx context.Context, command ChatAgentCommand) (agentrun.CommandReceipt, error) {
	return s.submitAgentCommand(ctx, command)
}

// SubmitChatAgentCommandForSession prevents a control intended for the
// visible Session from being delivered to a different foreground runtime.
func (a *App) SubmitChatAgentCommandForSession(ctx context.Context, sessionID string, command ChatAgentCommand) (agentrun.CommandReceipt, error) {
	return a.chat().SubmitAgentCommandForSession(ctx, sessionID, command)
}

func (s *ChatAppService) SubmitAgentCommandForSession(ctx context.Context, sessionID string, command ChatAgentCommand) (agentrun.CommandReceipt, error) {
	s.admission.RLock()
	defer s.admission.RUnlock()
	if err := s.confirmSelectedSessionID(sessionID); err != nil {
		return agentrun.CommandReceipt{}, err
	}
	return s.submitAgentCommand(ctx, command)
}

func (s *ChatAppService) submitAgentCommand(ctx context.Context, command ChatAgentCommand) (agentrun.CommandReceipt, error) {
	if command.Kind == agentharness.CommandAbort || command.Kind == agentharness.CommandSteerQueued || command.Kind == agentharness.CommandCancelQueued {
		runtime, task, err := s.activeCommandRuntime()
		if err != nil {
			return agentrun.CommandReceipt{}, err
		}
		return runtime.chatService.SubmitCommand(ctx, agentharness.CommandSpec{
			Kind: command.Kind, CommandID: command.CommandID,
			OperationID: command.OperationID, TargetCommandID: command.TargetCommandID, Reason: command.Reason,
			Options: agentrun.Options{
				AgentKind: agentrun.AgentKindIDE, TaskID: task.ID(),
				StateRoot: runtime.projectState,
				SessionID: runtime.sess.ID, Workspace: runtime.workspace, Mode: "ide",
			},
		})
	}
	if command.Kind != agentharness.CommandSteer && command.Kind != agentharness.CommandFollowUp && command.Kind != agentharness.CommandNextTurn {
		return agentrun.CommandReceipt{}, fmt.Errorf("%w: unsupported writing command %q", agentrun.ErrInvalidCommand, command.Kind)
	}
	activeRuntime, task, err := s.activeCommandRuntime()
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	prepare := func(prepareCtx context.Context) (agentharness.TurnExecution, error) {
		if err := s.confirmActiveCommandRuntime(activeRuntime, task); err != nil {
			return agentharness.TurnExecution{}, err
		}
		execution, runtime, err := s.prepareWritingHarnessTurn(prepareCtx, command.Input, task.ID())
		if err != nil {
			return agentharness.TurnExecution{}, err
		}
		if runtime.workspace != activeRuntime.workspace || runtime.sess != activeRuntime.sess || runtime.chatService != activeRuntime.chatService {
			return agentharness.TurnExecution{}, ErrAgentContextChanged
		}
		if err := s.confirmActiveCommandRuntime(activeRuntime, task); err != nil {
			return agentharness.TurnExecution{}, err
		}
		return execution, nil
	}
	return activeRuntime.chatService.SubmitCommand(ctx, agentharness.CommandSpec{
		Kind: command.Kind, CommandID: command.CommandID,
		OperationID: command.OperationID, AfterOperationID: command.OperationID,
		Request: command.Input, Emit: task.Emit, Prepare: prepare,
		Successor: s.writingGoalSuccessor(activeRuntime, task, command.Input.Locale),
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindIDE,
			StateRoot: activeRuntime.projectState,
			TaskID:    task.ID(),
			SessionID: activeRuntime.sess.ID,
			Workspace: activeRuntime.workspace,
			Mode:      "ide",
		},
	})
}

func (s *ChatAppService) writingGoalSuccessor(runtime ideChatRuntime, task *apptask.Task, locale string) agentharness.SuccessorPolicy {
	var successor agentharness.SuccessorPolicy
	successor = func(ctx context.Context, parent agentrun.OperationID, _ agentrun.Outcome) error {
		if !config.ResolveAgentTools(&runtime.cfg, config.AgentKindIDE).Allows(config.AgentToolGoal) {
			return nil
		}
		current, ok, err := runtime.sess.Goal(ctx)
		if err != nil || !ok || !current.IsActive() {
			return err
		}
		commandID, input := appagentruntime.GoalContinuationRequest(current, parent, locale)
		prepare := func(prepareCtx context.Context) (agentharness.TurnExecution, error) {
			if err := s.confirmActiveCommandRuntime(runtime, task); err != nil {
				return agentharness.TurnExecution{}, err
			}
			execution, preparedRuntime, err := s.prepareWritingHarnessTurn(prepareCtx, input, task.ID())
			if err != nil {
				return agentharness.TurnExecution{}, err
			}
			if preparedRuntime.workspace != runtime.workspace || preparedRuntime.sess != runtime.sess || preparedRuntime.chatService != runtime.chatService {
				return agentharness.TurnExecution{}, ErrAgentContextChanged
			}
			return execution, s.confirmActiveCommandRuntime(runtime, task)
		}
		_, err = runtime.chatService.SubmitCommand(ctx, agentharness.CommandSpec{
			Kind: agentharness.CommandNextTurn, CommandID: commandID,
			AfterOperationID: parent, Request: input, Emit: task.Emit, Prepare: prepare,
			Successor: successor,
			Options: agentrun.Options{
				AgentKind: agentrun.AgentKindIDE, StateRoot: runtime.projectState,
				TaskID: task.ID(), SessionID: runtime.sess.ID, Workspace: runtime.workspace, Mode: "ide",
			},
		})
		return err
	}
	return successor
}

func (s *ChatAppService) confirmActiveCommandRuntime(expected ideChatRuntime, task *apptask.Task) error {
	current, currentTask, err := s.activeCommandRuntime()
	if err != nil {
		return err
	}
	if currentTask != task || current.workspace != expected.workspace || current.sess != expected.sess || current.state != expected.state || current.chatService != expected.chatService {
		return ErrAgentContextChanged
	}
	return nil
}

func (s *ChatAppService) activeCommandRuntime() (ideChatRuntime, *apptask.Task, error) {
	if s == nil || s.app == nil {
		return ideChatRuntime{}, nil, ErrNoWorkspace
	}
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.workspaceTransition {
		return ideChatRuntime{}, nil, ErrWorkspaceTransition
	}
	if a.session == nil || a.bookState == nil || a.chatService == nil || a.cfg == nil {
		return ideChatRuntime{}, nil, ErrNoWorkspace
	}
	run := a.activeWritingRun
	if run == nil || run.task == nil || run.task.Finished() {
		return ideChatRuntime{}, nil, ErrNoActiveAgentOperation
	}
	return run.runtime, run.task, nil
}
